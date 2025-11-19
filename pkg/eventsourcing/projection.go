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
	projection, exists := m.projections[projectionName]
	if !exists {
		m.mu.Unlock()
		return fmt.Errorf("projection %s not found", projectionName)
	}

	// Check if already running
	if _, running := m.running[projectionName]; running {
		m.mu.Unlock()
		return fmt.Errorf("projection %s already running", projectionName)
	}
	m.mu.Unlock()

	// Load checkpoint
	checkpoint, err := m.checkpointStore.Load(projectionName)
	if err != nil {
		// No checkpoint, start from beginning
		checkpoint = &store.ProjectionCheckpoint{
			ProjectionName: projectionName,
			Position:       0,
			NATSSequence:   nil,
			IsRebuilding:   false,
		}
	}

	// Check if interrupted rebuild
	if checkpoint.IsRebuilding {
		return fmt.Errorf(
			"projection %s has interrupted rebuild at position %d - call Rebuild() to resume or complete",
			projectionName, checkpoint.Position,
		)
	}

	// Create cancellable context
	projCtx, cancel := context.WithCancel(ctx)

	// Build subscription options
	subscribeOpts := []messaging.SubscribeOption{
		// Use deterministic consumer name
		messaging.WithConsumerName(fmt.Sprintf("projection_%s", projectionName)),
	}

	// If we have a NATS sequence checkpoint, resume from there
	if checkpoint.NATSSequence != nil {
		// Resume from NEXT sequence (checkpoint is last processed)
		nextSequence := uint64(*checkpoint.NATSSequence + 1)
		subscribeOpts = append(subscribeOpts, messaging.WithStartSequence(nextSequence))
	} else {
		// No NATS checkpoint - deliver all (initial build or legacy checkpoint)
		subscribeOpts = append(subscribeOpts, messaging.WithDeliverAll())
	}

	// Subscribe to event bus
	subscription, err := m.eventBus.Subscribe(
		messaging.EventFilter{},
		func(event *domain.EventEnvelope) error {
			// CRITICAL FIX: Check if we've already processed this event position
			// This prevents double-counting when events are reprocessed from NATS after rebuild
			if event.Event.Position <= checkpoint.Position {
				// Already processed - skip handler but acknowledge to NATS
				if event.NATSMetadata != nil {
					seq := int64(event.NATSMetadata.StreamSequence)
					checkpoint.NATSSequence = &seq
					checkpoint.UpdatedAt = domain.Now()
					if err := m.checkpointStore.Save(checkpoint); err != nil {
						return fmt.Errorf("failed to save checkpoint: %w", err)
					}
				}
				return nil // Skip processing
			}

			// Process new event
			if err := projection.Handle(projCtx, event); err != nil {
				return fmt.Errorf("projection %s failed to handle event: %w", projectionName, err)
			}

			// Update checkpoint with new position
			if event.NATSMetadata != nil {
				// Event from NATS - track stream sequence
				seq := int64(event.NATSMetadata.StreamSequence)
				checkpoint.NATSSequence = &seq
			}

			checkpoint.Position = event.Event.Position
			checkpoint.LastEventID = event.Event.ID
			checkpoint.UpdatedAt = domain.Now()

			if err := m.checkpointStore.Save(checkpoint); err != nil {
				return fmt.Errorf("failed to save checkpoint: %w", err)
			}

			return nil
		},
		subscribeOpts...,
	)

	if err != nil {
		cancel()
		return fmt.Errorf("failed to subscribe: %w", err)
	}

	// Mark as running
	m.mu.Lock()
	m.running[projectionName] = cancel
	m.mu.Unlock()

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

	// Set rebuilding flag FIRST (before reset, in case of crash during reset)
	if err := m.checkpointStore.Save(&store.ProjectionCheckpoint{
		ProjectionName: projectionName,
		Position:       0,
		NATSSequence:   nil,
		LastEventID:    "",
		IsRebuilding:   true,
		UpdatedAt:      domain.Now(),
	}); err != nil {
		return fmt.Errorf("failed to set rebuilding flag: %w", err)
	}

	// Reset projection
	if err := projection.Reset(ctx); err != nil {
		return fmt.Errorf("failed to reset projection: %w", err)
	}

	// Replay all events from EventStore
	position := int64(0)
	batchSize := 1000

	for {
		events, err := m.eventStore.LoadAllEvents(position, batchSize)
		if err != nil {
			return fmt.Errorf("failed to load events at position %d: %w", position, err)
		}

		if len(events) == 0 {
			break
		}

		for _, event := range events {
			envelope := &domain.EventEnvelope{
				Event:        *event,
				NATSMetadata: nil, // No NATS metadata during rebuild (from EventStore)
			}

			if err := projection.Handle(ctx, envelope); err != nil {
				return fmt.Errorf("failed to handle event during rebuild: %w", err)
			}
			position++
		}

		// Save checkpoint periodically (still marked as rebuilding)
		lastEventID := ""
		if len(events) > 0 {
			lastEventID = events[len(events)-1].ID
		}

		if err := m.checkpointStore.Save(&store.ProjectionCheckpoint{
			ProjectionName: projectionName,
			Position:       position,
			NATSSequence:   nil, // NATS sequence not set during rebuild
			LastEventID:    lastEventID,
			UpdatedAt:      domain.Now(),
			IsRebuilding:   true, // Still rebuilding
		}); err != nil {
			return fmt.Errorf("failed to save checkpoint: %w", err)
		}

		if len(events) < batchSize {
			break
		}
	}

	// Clear rebuilding flag (rebuild complete)
	if err := m.checkpointStore.Save(&store.ProjectionCheckpoint{
		ProjectionName: projectionName,
		Position:       position,
		NATSSequence:   nil, // Will be set when subscription starts
		LastEventID:    "",  // Will be updated by first NATS message
		UpdatedAt:      domain.Now(),
		IsRebuilding:   false, // Rebuild complete
	}); err != nil {
		return fmt.Errorf("failed to clear rebuilding flag: %w", err)
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
