package eventsourcing

import (
	"context"
	"fmt"
	"sync"

	"github.com/plaenen/eventstore/pkg/domain"
	"github.com/plaenen/eventstore/pkg/messaging"
	"github.com/plaenen/eventstore/pkg/store"
)

// Projection defines the interface for building read models from events.
// Projections consume events from the EventBus in real-time and can be rebuilt from EventStore.
type Projection interface {
	// Name returns the unique name of this projection.
	Name() string

	// Handle processes an event and updates the read model.
	Handle(ctx context.Context, event *domain.EventEnvelope) error

	// Reset resets the projection state (useful for rebuilding).
	Reset(ctx context.Context) error
}

// Deprecated: Use store.ProjectionCheckpoint instead
type ProjectionCheckpoint = store.ProjectionCheckpoint

// Deprecated: Use store.CheckpointStore instead
type CheckpointStore = store.CheckpointStore

// ProjectionManager coordinates running projections.
// Uses hybrid approach: EventBus for real-time, EventStore for rebuilds.
type ProjectionManager struct {
	projections     map[string]Projection
	checkpointStore store.CheckpointStore
	eventStore      store.EventStore // For rebuilds
	eventBus        messaging.EventBus   // For real-time
	mu              sync.RWMutex
	running         map[string]context.CancelFunc
	wg              sync.WaitGroup
}

// NewProjectionManager creates a new projection manager.
func NewProjectionManager(checkpointStore store.CheckpointStore, eventStore store.EventStore, eventBus messaging.EventBus) *ProjectionManager {
	return &ProjectionManager{
		projections:     make(map[string]Projection),
		checkpointStore: checkpointStore,
		eventStore:      eventStore,
		eventBus:        eventBus,
		running:         make(map[string]context.CancelFunc),
	}
}

// Register registers a projection with the manager.
func (m *ProjectionManager) Register(projection Projection) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.projections[projection.Name()] = projection
}

// Start starts a projection consuming events from EventBus (real-time).
func (m *ProjectionManager) Start(ctx context.Context, projectionName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	projection, exists := m.projections[projectionName]
	if !exists {
		return fmt.Errorf("projection %s not found", projectionName)
	}

	// Check if already running
	if _, running := m.running[projectionName]; running {
		return fmt.Errorf("projection %s already running", projectionName)
	}

	// Load checkpoint
	checkpoint, err := m.checkpointStore.Load(projectionName)
	if err != nil {
		// No checkpoint, start from beginning
		checkpoint = &store.ProjectionCheckpoint{
			ProjectionName: projectionName,
			Position:       0,
		}
	}

	// Create cancellable context
	projCtx, cancel := context.WithCancel(ctx)
	m.running[projectionName] = cancel

	// Subscribe to event bus (real-time events)
	subscription, err := m.eventBus.Subscribe(messaging.EventFilter{}, func(event *domain.EventEnvelope) error {
		// Process event
		if err := projection.Handle(projCtx, event); err != nil {
			return fmt.Errorf("projection %s failed to handle event: %w", projectionName, err)
		}

		// Update checkpoint
		checkpoint.Position++
		checkpoint.LastEventID = event.Event.ID
		checkpoint.UpdatedAt = domain.Now()

		if err := m.checkpointStore.Save(checkpoint); err != nil {
			return fmt.Errorf("failed to save checkpoint: %w", err)
		}

		return nil
	})

	if err != nil {
		cancel()
		delete(m.running, projectionName)
		return fmt.Errorf("failed to subscribe: %w", err)
	}

	// Start projection in background
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		<-projCtx.Done()
		subscription.Unsubscribe()
	}()

	return nil
}

// Stop stops a running projection.
func (m *ProjectionManager) Stop(projectionName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cancel, running := m.running[projectionName]
	if !running {
		return fmt.Errorf("projection %s not running", projectionName)
	}

	cancel()
	delete(m.running, projectionName)

	return nil
}

// Rebuild rebuilds a projection from EventStore history (batch processing).
// This is useful for:
// - Initial projection build
// - Recovering from errors
// - Schema changes in read model
func (m *ProjectionManager) Rebuild(ctx context.Context, projectionName string) error {
	m.mu.Lock()
	projection, exists := m.projections[projectionName]
	if !exists {
		m.mu.Unlock()
		return fmt.Errorf("projection %s not found", projectionName)
	}

	// Stop if running
	if cancel, running := m.running[projectionName]; running {
		cancel()
		delete(m.running, projectionName)
	}
	m.mu.Unlock()

	// Reset projection
	if err := projection.Reset(ctx); err != nil {
		return fmt.Errorf("failed to reset projection: %w", err)
	}

	// Delete checkpoint
	if err := m.checkpointStore.Delete(projectionName); err != nil {
		return fmt.Errorf("failed to delete checkpoint: %w", err)
	}

	// Replay all events from EventStore
	position := int64(0)
	batchSize := 1000

	for {
		events, err := m.eventStore.LoadAllEvents(position, batchSize)
		if err != nil {
			return fmt.Errorf("failed to load events: %w", err)
		}

		if len(events) == 0 {
			break
		}

		for _, event := range events {
			envelope := &domain.EventEnvelope{Event: *event}
			if err := projection.Handle(ctx, envelope); err != nil {
				return fmt.Errorf("failed to handle event during rebuild: %w", err)
			}
			position++
		}

		// Save checkpoint periodically
		if err := m.checkpointStore.Save(&store.ProjectionCheckpoint{
			ProjectionName: projectionName,
			Position:       position,
			LastEventID:    events[len(events)-1].ID,
			UpdatedAt:      domain.Now(),
		}); err != nil {
			return fmt.Errorf("failed to save checkpoint: %w", err)
		}

		if len(events) < batchSize {
			break
		}
	}

	return nil
}

// StopAll stops all running projections.
func (m *ProjectionManager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, cancel := range m.running {
		cancel()
		delete(m.running, name)
	}

	m.wg.Wait()
}

// GetCheckpoint returns the current checkpoint for a projection.
func (m *ProjectionManager) GetCheckpoint(projectionName string) (*ProjectionCheckpoint, error) {
	return m.checkpointStore.Load(projectionName)
}
