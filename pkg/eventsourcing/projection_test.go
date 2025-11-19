package eventsourcing_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/plaenen/eventstore/pkg/domain"
	"github.com/plaenen/eventstore/pkg/eventsourcing"
	"github.com/plaenen/eventstore/pkg/messaging"
	"github.com/plaenen/eventstore/pkg/store"
)

var ErrCheckpointNotFound = errors.New("checkpoint not found")

// Mock implementations for testing

type mockProjection struct {
	name           string
	handleFunc     func(ctx context.Context, event *domain.EventEnvelope) error
	resetFunc      func(ctx context.Context) error
	processedCount int
	mu             sync.Mutex
}

func (m *mockProjection) Name() string {
	return m.name
}

func (m *mockProjection) Handle(ctx context.Context, event *domain.EventEnvelope) error {
	m.mu.Lock()
	m.processedCount++
	m.mu.Unlock()

	if m.handleFunc != nil {
		return m.handleFunc(ctx, event)
	}
	return nil
}

func (m *mockProjection) Reset(ctx context.Context) error {
	m.mu.Lock()
	m.processedCount = 0
	m.mu.Unlock()

	if m.resetFunc != nil {
		return m.resetFunc(ctx)
	}
	return nil
}

func (m *mockProjection) GetProcessedCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.processedCount
}

type mockCheckpointStore struct {
	checkpoints map[string]*store.ProjectionCheckpoint
	mu          sync.RWMutex
}

func newMockCheckpointStore() *mockCheckpointStore {
	return &mockCheckpointStore{
		checkpoints: make(map[string]*store.ProjectionCheckpoint),
	}
}

func (m *mockCheckpointStore) Save(checkpoint *store.ProjectionCheckpoint) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Clone to avoid race conditions
	clone := *checkpoint
	m.checkpoints[checkpoint.ProjectionName] = &clone
	return nil
}

func (m *mockCheckpointStore) Load(projectionName string) (*store.ProjectionCheckpoint, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	checkpoint, exists := m.checkpoints[projectionName]
	if !exists {
		return nil, ErrCheckpointNotFound
	}

	// Clone to avoid race conditions
	clone := *checkpoint
	return &clone, nil
}

func (m *mockCheckpointStore) Delete(projectionName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.checkpoints, projectionName)
	return nil
}

type mockEventStore struct {
	events []*domain.Event
	mu     sync.RWMutex
}

func newMockEventStore(events []*domain.Event) *mockEventStore {
	return &mockEventStore{
		events: events,
	}
}

func (m *mockEventStore) LoadAllEvents(afterPosition int64, limit int) ([]*domain.Event, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*domain.Event
	for _, event := range m.events {
		if event.Position > afterPosition {
			result = append(result, event)
			if len(result) >= limit {
				break
			}
		}
	}
	return result, nil
}

func (m *mockEventStore) AppendEvents(aggregateID string, expectedVersion int64, events []*domain.Event) error {
	return nil
}

func (m *mockEventStore) AppendEventsIdempotent(aggregateID string, expectedVersion int64, events []*domain.Event, commandID string, ttl time.Duration) (*domain.CommandResult, error) {
	return &domain.CommandResult{}, nil
}

func (m *mockEventStore) GetCommandResult(commandID string) (*domain.CommandResult, error) {
	return nil, nil
}

func (m *mockEventStore) LoadEvents(aggregateID string, afterVersion int64) ([]*domain.Event, error) {
	return nil, nil
}

func (m *mockEventStore) GetAggregateVersion(aggregateID string) (int64, error) {
	return 0, nil
}

func (m *mockEventStore) CheckUniqueness(indexName, value string) (bool, string, error) {
	return true, "", nil
}

func (m *mockEventStore) GetConstraintOwner(indexName, value string) (string, error) {
	return "", nil
}

func (m *mockEventStore) RebuildConstraints() error {
	return nil
}

func (m *mockEventStore) SeedEvents(aggregateID string, expectedVersion int64, events []*domain.Event, opts *domain.SeedOptions) (*domain.SeedResult, error) {
	return &domain.SeedResult{}, nil
}

func (m *mockEventStore) Close() error {
	return nil
}

func (m *mockEventStore) LoadUnpublishedEvents(limit int) ([]*domain.EventEnvelope, error) {
	return nil, nil
}

func (m *mockEventStore) MarkEventsPublished(eventIDs []string) error {
	return nil
}

func (m *mockEventStore) RecordPublishFailure(eventID string, err error) error {
	return nil
}

type mockEventBus struct {
	subscribeFunc func(filter messaging.EventFilter, handler messaging.EventHandler, opts ...messaging.SubscribeOption) (messaging.Subscription, error)
}

func (m *mockEventBus) Subscribe(filter messaging.EventFilter, handler messaging.EventHandler, opts ...messaging.SubscribeOption) (messaging.Subscription, error) {
	if m.subscribeFunc != nil {
		return m.subscribeFunc(filter, handler, opts...)
	}
	return &mockSubscription{}, nil
}

func (m *mockEventBus) Publish(events []*domain.Event) error {
	return nil
}

func (m *mockEventBus) Close() error {
	return nil
}

type mockSubscription struct {
	unsubscribed bool
	mu           sync.Mutex
}

func (m *mockSubscription) Unsubscribe() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.unsubscribed = true
	return nil
}

// Helper function to create test events
func createTestEvents(count int) []*domain.Event {
	events := make([]*domain.Event, count)
	for i := 0; i < count; i++ {
		events[i] = &domain.Event{
			ID:            fmt.Sprintf("event-%d", i+1),
			AggregateID:   "test-aggregate",
			AggregateType: "TestAggregate",
			EventType:     "test.Event",
			Version:       int64(i + 1),
			Position:      int64(i + 1),
			Timestamp:     time.Now(),
			Data:          []byte("test"),
		}
	}
	return events
}

// REGRESSION TEST 1: Fresh Rebuild - Position should not double-count
func TestProjectionCheckpoint_FreshRebuild(t *testing.T) {
	ctx := context.Background()

	// Given: 4 events in EventStore (positions 1-4)
	events := createTestEvents(4)
	eventStore := newMockEventStore(events)
	checkpointStore := newMockCheckpointStore()
	eventBus := &mockEventBus{}

	projection := &mockProjection{name: "test-projection"}
	manager := eventsourcing.NewProjectionManager(checkpointStore, eventStore, eventBus)
	manager.Register(projection)

	// When: Projection rebuilds
	err := manager.Rebuild(ctx, "test-projection")
	if err != nil {
		t.Fatalf("Rebuild failed: %v", err)
	}

	// Then: Checkpoint position should be 4, not 8
	checkpoint, err := checkpointStore.Load("test-projection")
	if err != nil {
		t.Fatalf("Failed to load checkpoint: %v", err)
	}

	if checkpoint.Position != 4 {
		t.Errorf("Expected checkpoint position 4, got %d", checkpoint.Position)
	}

	// And: All 4 events should have been processed exactly once
	if projection.GetProcessedCount() != 4 {
		t.Errorf("Expected 4 events processed, got %d", projection.GetProcessedCount())
	}
}

// REGRESSION TEST 2: Resume After Rebuild - Event 5 should not be skipped
func TestProjectionCheckpoint_ResumeAfterRebuild(t *testing.T) {
	ctx := context.Background()

	// Given: Projection rebuilt from 4 events (checkpoint=4)
	events := createTestEvents(4)
	eventStore := newMockEventStore(events)
	checkpointStore := newMockCheckpointStore()

	// Mock event bus that simulates delivering all 4 events + new event 5
	deliveredEvents := make([]*domain.EventEnvelope, 0)
	eventBus := &mockEventBus{
		subscribeFunc: func(filter messaging.EventFilter, handler messaging.EventHandler, opts ...messaging.SubscribeOption) (messaging.Subscription, error) {
			// Deliver events 1-4 (already processed during rebuild)
			for i := 0; i < 4; i++ {
				envelope := &domain.EventEnvelope{
					Event: *events[i],
					NATSMetadata: &domain.NATSMetadata{
						StreamSequence: uint64(i + 1),
					},
				}
				deliveredEvents = append(deliveredEvents, envelope)
				handler(envelope)
			}

			// Deliver new event 5
			newEvent := &domain.Event{
				ID:            "event-5",
				AggregateID:   "test-aggregate",
				AggregateType: "TestAggregate",
				EventType:     "test.Event",
				Version:       5,
				Position:      5,
				Timestamp:     time.Now(),
				Data:          []byte("test"),
			}
			envelope := &domain.EventEnvelope{
				Event: *newEvent,
				NATSMetadata: &domain.NATSMetadata{
					StreamSequence: 5,
				},
			}
			deliveredEvents = append(deliveredEvents, envelope)
			handler(envelope)

			return &mockSubscription{}, nil
		},
	}

	projection := &mockProjection{name: "test-projection"}
	manager := eventsourcing.NewProjectionManager(checkpointStore, eventStore, eventBus)
	manager.Register(projection)

	// Rebuild first (processes events 1-4)
	err := manager.Rebuild(ctx, "test-projection")
	if err != nil {
		t.Fatalf("Rebuild failed: %v", err)
	}

	// Verify checkpoint after rebuild
	checkpoint, _ := checkpointStore.Load("test-projection")
	if checkpoint.Position != 4 {
		t.Fatalf("Expected checkpoint 4 after rebuild, got %d", checkpoint.Position)
	}

	// Reset processed count to track NATS events separately
	projection.mu.Lock()
	projection.processedCount = 0
	projection.mu.Unlock()

	// When: Start projection (subscribes to NATS, receives events 1-5)
	err = manager.Start(ctx, "test-projection")
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Give time for async processing
	time.Sleep(100 * time.Millisecond)

	// Then: Checkpoint should be 5 (not 8 or 9!)
	checkpoint, err = checkpointStore.Load("test-projection")
	if err != nil {
		t.Fatalf("Failed to load checkpoint: %v", err)
	}

	if checkpoint.Position != 5 {
		t.Errorf("Expected checkpoint position 5, got %d (double-counting bug if 8 or 9)", checkpoint.Position)
	}

	// And: Only event 5 should have been processed (events 1-4 were skipped as already processed)
	processedCount := projection.GetProcessedCount()
	if processedCount != 1 {
		t.Errorf("Expected 1 new event processed (event 5), got %d", processedCount)
	}

	// And: NATS sequence should be 5
	if checkpoint.NATSSequence == nil || *checkpoint.NATSSequence != 5 {
		t.Errorf("Expected NATS sequence 5, got %v", checkpoint.NATSSequence)
	}
}

// REGRESSION TEST 3: Multiple Restart Cycles - No accumulation
func TestProjectionCheckpoint_MultipleRestarts(t *testing.T) {
	ctx := context.Background()

	// Given: Events created across multiple server lifecycles
	events := createTestEvents(5)
	eventStore := newMockEventStore(events)
	checkpointStore := newMockCheckpointStore()
	eventBus := &mockEventBus{}

	projection := &mockProjection{name: "test-projection"}
	manager := eventsourcing.NewProjectionManager(checkpointStore, eventStore, eventBus)
	manager.Register(projection)

	// First rebuild: events 1-2
	eventStore.events = events[:2]
	err := manager.Rebuild(ctx, "test-projection")
	if err != nil {
		t.Fatalf("First rebuild failed: %v", err)
	}

	checkpoint1, _ := checkpointStore.Load("test-projection")
	if checkpoint1.Position != 2 {
		t.Errorf("After first rebuild: expected position 2, got %d", checkpoint1.Position)
	}

	// Second rebuild: events 1-5 (full rebuild)
	eventStore.events = events
	err = manager.Rebuild(ctx, "test-projection")
	if err != nil {
		t.Fatalf("Second rebuild failed: %v", err)
	}

	checkpoint2, _ := checkpointStore.Load("test-projection")
	if checkpoint2.Position != 5 {
		t.Errorf("After second rebuild: expected position 5, got %d (NOT 10!)", checkpoint2.Position)
	}
}

// REGRESSION TEST 4: Out-of-order event delivery protection
func TestProjectionCheckpoint_OutOfOrderEvents(t *testing.T) {
	ctx := context.Background()

	events := createTestEvents(5)
	eventStore := newMockEventStore(events)
	checkpointStore := newMockCheckpointStore()

	// Mock event bus that delivers events out of order
	eventBus := &mockEventBus{
		subscribeFunc: func(filter messaging.EventFilter, handler messaging.EventHandler, opts ...messaging.SubscribeOption) (messaging.Subscription, error) {
			// Deliver events in order: 1, 2, 3, 4, 5
			for i := 0; i < 5; i++ {
				envelope := &domain.EventEnvelope{
					Event: *events[i],
					NATSMetadata: &domain.NATSMetadata{
						StreamSequence: uint64(i + 1),
					},
				}
				handler(envelope)
			}

			// Now deliver old event 3 again (out of order)
			oldEnvelope := &domain.EventEnvelope{
				Event: *events[2], // Event with position 3
				NATSMetadata: &domain.NATSMetadata{
					StreamSequence: 6, // New NATS sequence
				},
			}
			handler(oldEnvelope)

			return &mockSubscription{}, nil
		},
	}

	projection := &mockProjection{name: "test-projection"}
	manager := eventsourcing.NewProjectionManager(checkpointStore, eventStore, eventBus)
	manager.Register(projection)

	// When: Start projection
	err := manager.Start(ctx, "test-projection")
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Then: Checkpoint should be 5 (not moved backwards to 3)
	checkpoint, err := checkpointStore.Load("test-projection")
	if err != nil {
		t.Fatalf("Failed to load checkpoint: %v", err)
	}

	if checkpoint.Position != 5 {
		t.Errorf("Expected checkpoint position 5 (not moved backwards), got %d", checkpoint.Position)
	}

	// And: NATS sequence should be 6 (acknowledging the redelivered message)
	if checkpoint.NATSSequence == nil || *checkpoint.NATSSequence != 6 {
		t.Errorf("Expected NATS sequence 6, got %v", checkpoint.NATSSequence)
	}

	// And: Only 5 events should have been processed (old event 3 was skipped)
	processedCount := projection.GetProcessedCount()
	if processedCount != 5 {
		t.Errorf("Expected 5 events processed (old event skipped), got %d", processedCount)
	}
}
