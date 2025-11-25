package store_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/plaenen/eventstore/pkg/eventsourcing"
	"github.com/plaenen/eventstore/pkg/eventsourcing/store"
	"github.com/plaenen/eventstore/pkg/eventsourcing/store/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// MockAggregate for testing snapshots
type MockAggregate struct {
	eventsourcing.AggregateRoot
	State *MockAggregateState
}

type MockAggregateState struct {
	Value string
	Count int
}

func NewMockAggregate(id string) *MockAggregate {
	return &MockAggregate{
		AggregateRoot: eventsourcing.NewAggregateRoot(id, "MockAggregate"),
		State:         &MockAggregateState{},
	}
}

// Implement Snapshotable
func (m *MockAggregate) MarshalSnapshot() ([]byte, error) {
	return json.Marshal(m.State)
}

func (m *MockAggregate) UnmarshalSnapshot(data []byte) error {
	return json.Unmarshal(data, &m.State)
}

// Implement domain.Aggregate interface
func (m *MockAggregate) ApplyEvent(event proto.Message) error {
	// Not used in this test, we use the applier function instead
	return nil
}

// Apply event
func (m *MockAggregate) ApplyMockEvent(eventType string) {
	m.State.Count++
	m.State.Value = eventType
}

// Mock applier function
func mockApplier(agg *MockAggregate, event *eventsourcing.Event) error {
	agg.ApplyMockEvent(event.EventType)
	return nil
}

func TestRepository_SnapshotWithAnalytics(t *testing.T) {
	// Create event store and snapshot store
	eventStore, err := sqlite.NewEventStore(
		sqlite.WithMemoryDatabase(),
		sqlite.WithWALMode(false),
	)
	require.NoError(t, err)
	defer eventStore.Close()

	snapshotStore := sqlite.NewSnapshotStore(eventStore.DB())

	// Create repository with snapshot support
	repo := store.NewRepository(
		eventStore,
		"MockAggregate",
		NewMockAggregate,
		mockApplier,
	).WithSnapshotStore(snapshotStore)

	// Create aggregate and apply some events
	agg := NewMockAggregate("test-123")

	// Simulate events
	events := []*eventsourcing.Event{
		{
			ID:            eventsourcing.GenerateID(),
			AggregateID:   "test-123",
			AggregateType: "MockAggregate",
			EventType:     "EventA",
			Version:       1,
			Timestamp:     time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			Data:          []byte(`{}`),
		},
		{
			ID:            eventsourcing.GenerateID(),
			AggregateID:   "test-123",
			AggregateType: "MockAggregate",
			EventType:     "EventB",
			Version:       2,
			Timestamp:     time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
			Data:          []byte(`{}`),
		},
		{
			ID:            eventsourcing.GenerateID(),
			AggregateID:   "test-123",
			AggregateType: "MockAggregate",
			EventType:     "EventA",
			Version:       3,
			Timestamp:     time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC),
			Data:          []byte(`{}`),
		},
	}

	// Apply events and load history to build analytics
	for _, event := range events {
		err := mockApplier(agg, event)
		require.NoError(t, err)
	}

	err = agg.LoadFromHistory(events)
	require.NoError(t, err)

	// Verify analytics before snapshot
	assert.Equal(t, int64(3), agg.Analytics().TotalEvents)
	assert.Equal(t, int64(2), agg.Analytics().GetCount("EventA"))
	assert.Equal(t, int64(1), agg.Analytics().GetCount("EventB"))

	// Save snapshot
	err = repo.SaveSnapshot(agg)
	require.NoError(t, err)

	// Verify snapshot was saved
	snapshot, err := snapshotStore.GetLatestSnapshot("test-123")
	require.NoError(t, err)
	require.NotNil(t, snapshot)
	assert.Equal(t, int64(3), snapshot.Version)

	// Verify analytics in snapshot metadata
	require.NotNil(t, snapshot.Metadata)
	assert.NotEmpty(t, snapshot.Metadata.Analytics)

	// Load from snapshot
	loaded, err := repo.LoadWithSnapshot("test-123")
	require.NoError(t, err)

	// Verify state was restored
	assert.Equal(t, "EventA", loaded.State.Value)
	assert.Equal(t, 3, loaded.State.Count)

	// Verify analytics were restored from snapshot
	analytics := loaded.Analytics()
	assert.Equal(t, int64(3), analytics.TotalEvents)
	assert.Equal(t, int64(2), analytics.GetCount("EventA"))
	assert.Equal(t, int64(1), analytics.GetCount("EventB"))
}

func TestRepository_SnapshotWithNewEvents(t *testing.T) {
	// Create stores
	eventStore, err := sqlite.NewEventStore(
		sqlite.WithMemoryDatabase(),
		sqlite.WithWALMode(false),
	)
	require.NoError(t, err)
	defer eventStore.Close()

	snapshotStore := sqlite.NewSnapshotStore(eventStore.DB())

	repo := store.NewRepository(
		eventStore,
		"MockAggregate",
		NewMockAggregate,
		mockApplier,
	).WithSnapshotStore(snapshotStore)

	// Create aggregate with 3 events
	agg := NewMockAggregate("test-456")

	events1 := []*eventsourcing.Event{
		{
			ID:            eventsourcing.GenerateID(),
			AggregateID:   "test-456",
			AggregateType: "MockAggregate",
			EventType:     "EventA",
			Version:       1,
			Timestamp:     time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			Data:          []byte(`{}`),
		},
		{
			ID:            eventsourcing.GenerateID(),
			AggregateID:   "test-456",
			AggregateType: "MockAggregate",
			EventType:     "EventB",
			Version:       2,
			Timestamp:     time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
			Data:          []byte(`{}`),
		},
		{
			ID:            eventsourcing.GenerateID(),
			AggregateID:   "test-456",
			AggregateType: "MockAggregate",
			EventType:     "EventC",
			Version:       3,
			Timestamp:     time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC),
			Data:          []byte(`{}`),
		},
	}

	// Save events to store
	for _, event := range events1 {
		err := mockApplier(agg, event)
		require.NoError(t, err)
	}
	err = agg.LoadFromHistory(events1)
	require.NoError(t, err)
	err = eventStore.AppendEvents("test-456", 0, events1)
	require.NoError(t, err)

	// Create snapshot at version 3
	err = repo.SaveSnapshot(agg)
	require.NoError(t, err)

	// Add more events AFTER snapshot
	events2 := []*eventsourcing.Event{
		{
			ID:            eventsourcing.GenerateID(),
			AggregateID:   "test-456",
			AggregateType: "MockAggregate",
			EventType:     "EventA",
			Version:       4,
			Timestamp:     time.Date(2025, 1, 4, 0, 0, 0, 0, time.UTC),
			Data:          []byte(`{}`),
		},
		{
			ID:            eventsourcing.GenerateID(),
			AggregateID:   "test-456",
			AggregateType: "MockAggregate",
			EventType:     "EventD",
			Version:       5,
			Timestamp:     time.Date(2025, 1, 5, 0, 0, 0, 0, time.UTC),
			Data:          []byte(`{}`),
		},
	}

	err = eventStore.AppendEvents("test-456", 3, events2)
	require.NoError(t, err)

	// Load from snapshot (should load snapshot + new events)
	loaded, err := repo.LoadWithSnapshot("test-456")
	require.NoError(t, err)

	// Verify version includes new events
	assert.Equal(t, int64(5), loaded.Version())

	// Verify analytics include BOTH snapshot analytics AND new events
	analytics := loaded.Analytics()
	assert.Equal(t, int64(5), analytics.TotalEvents) // 3 from snapshot + 2 new

	// Event counts
	assert.Equal(t, int64(2), analytics.GetCount("EventA")) // 1 in snapshot + 1 new
	assert.Equal(t, int64(1), analytics.GetCount("EventB")) // From snapshot
	assert.Equal(t, int64(1), analytics.GetCount("EventC")) // From snapshot
	assert.Equal(t, int64(1), analytics.GetCount("EventD")) // New event

	// Verify timestamps - EventD should be most recent
	statsD := analytics.GetStats("EventD")
	require.NotNil(t, statsD)
	expectedD := time.Date(2025, 1, 5, 0, 0, 0, 0, time.UTC)
	assert.True(t, statsD.FirstApplied.UTC().Equal(expectedD), "EventD FirstApplied: expected %v, got %v", expectedD, statsD.FirstApplied.UTC())

	// Verify EventA has updated LastApplied
	statsA := analytics.GetStats("EventA")
	require.NotNil(t, statsA)
	expectedAFirst := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	expectedALast := time.Date(2025, 1, 4, 0, 0, 0, 0, time.UTC)
	assert.True(t, statsA.FirstApplied.UTC().Equal(expectedAFirst), "EventA FirstApplied: expected %v, got %v", expectedAFirst, statsA.FirstApplied.UTC())
	assert.True(t, statsA.LastApplied.UTC().Equal(expectedALast), "EventA LastApplied: expected %v, got %v", expectedALast, statsA.LastApplied.UTC())
}

func TestRepository_LoadUsesSnapshotAutomatically(t *testing.T) {
	// Create stores
	eventStore, err := sqlite.NewEventStore(
		sqlite.WithMemoryDatabase(),
		sqlite.WithWALMode(false),
	)
	require.NoError(t, err)
	defer eventStore.Close()

	snapshotStore := sqlite.NewSnapshotStore(eventStore.DB())

	repo := store.NewRepository(
		eventStore,
		"MockAggregate",
		NewMockAggregate,
		mockApplier,
	).WithSnapshotStore(snapshotStore)

	// Create and save aggregate with snapshot
	agg := NewMockAggregate("test-789")
	events := []*eventsourcing.Event{
		{
			ID:            eventsourcing.GenerateID(),
			AggregateID:   "test-789",
			AggregateType: "MockAggregate",
			EventType:     "EventA",
			Version:       1,
			Timestamp:     time.Now(),
			Data:          []byte(`{}`),
		},
	}

	for _, event := range events {
		err := mockApplier(agg, event)
		require.NoError(t, err)
	}
	err = agg.LoadFromHistory(events)
	require.NoError(t, err)
	err = eventStore.AppendEvents("test-789", 0, events)
	require.NoError(t, err)
	err = repo.SaveSnapshot(agg)
	require.NoError(t, err)

	// Load using regular Load() method - should use snapshot automatically
	loaded, err := repo.Load("test-789")
	require.NoError(t, err)

	// Verify analytics were restored
	assert.Equal(t, int64(1), loaded.Analytics().TotalEvents)
	assert.Equal(t, int64(1), loaded.Analytics().GetCount("EventA"))
}

func TestRepository_NoSnapshotFallsBackToEvents(t *testing.T) {
	// Create stores
	eventStore, err := sqlite.NewEventStore(
		sqlite.WithMemoryDatabase(),
		sqlite.WithWALMode(false),
	)
	require.NoError(t, err)
	defer eventStore.Close()

	snapshotStore := sqlite.NewSnapshotStore(eventStore.DB())

	repo := store.NewRepository(
		eventStore,
		"MockAggregate",
		NewMockAggregate,
		mockApplier,
	).WithSnapshotStore(snapshotStore)

	// Create aggregate but DON'T save snapshot (just save events directly)
	events := []*eventsourcing.Event{
		{
			ID:            eventsourcing.GenerateID(),
			AggregateID:   "test-no-snap",
			AggregateType: "MockAggregate",
			EventType:     "EventA",
			Version:       1,
			Timestamp:     time.Now(),
			Data:          []byte(`{}`),
		},
	}

	err = eventStore.AppendEvents("test-no-snap", 0, events)
	require.NoError(t, err)

	// Load should fall back to full event replay
	loaded, err := repo.LoadWithSnapshot("test-no-snap")
	require.NoError(t, err)

	// Verify analytics were built from events
	assert.Equal(t, int64(1), loaded.Analytics().TotalEvents)
	assert.Equal(t, int64(1), loaded.Analytics().GetCount("EventA"))
}
