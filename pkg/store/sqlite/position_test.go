package sqlite_test

import (
	"testing"
	"time"

	"github.com/plaenen/eventstore/pkg/domain"
	"github.com/plaenen/eventstore/pkg/store/sqlite"
)

// TestAtomicPositionAssignment verifies that positions are unique and sequential,
// even when events have identical timestamps.
func TestAtomicPositionAssignment(t *testing.T) {
	store, err := sqlite.NewEventStore(
		sqlite.WithDSN(":memory:"),
		sqlite.WithWALMode(false),
	)
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	defer store.Close()

	// Create multiple events with the SAME timestamp
	now := time.Now()
	events := []*domain.Event{
		{
			ID:            "event-1",
			AggregateID:   "agg-1",
			AggregateType: "TestAggregate",
			EventType:     "TestEvent",
			Version:       1,
			Timestamp:     now,
			Data:          []byte("data1"),
		},
		{
			ID:            "event-2",
			AggregateID:   "agg-1",
			AggregateType: "TestAggregate",
			EventType:     "TestEvent",
			Version:       2,
			Timestamp:     now, // SAME timestamp!
			Data:          []byte("data2"),
		},
		{
			ID:            "event-3",
			AggregateID:   "agg-1",
			AggregateType: "TestAggregate",
			EventType:     "TestEvent",
			Version:       3,
			Timestamp:     now, // SAME timestamp!
			Data:          []byte("data3"),
		},
	}

	// Append events
	err = store.AppendEvents("agg-1", 0, events)
	if err != nil {
		t.Fatalf("failed to append events: %v", err)
	}

	// Load all events
	loaded, err := store.LoadAllEvents(0, 100)
	if err != nil {
		t.Fatalf("failed to load events: %v", err)
	}

	// Verify we got 3 events
	if len(loaded) != 3 {
		t.Fatalf("expected 3 events, got %d", len(loaded))
	}

	// Verify positions are unique and sequential (1, 2, 3)
	expectedPositions := []int64{1, 2, 3}
	for i, event := range loaded {
		if event.Position != expectedPositions[i] {
			t.Errorf("event[%d] has position %d, expected %d", i, event.Position, expectedPositions[i])
		}
	}

	// Verify no position is 0 (unassigned)
	for i, event := range loaded {
		if event.Position == 0 {
			t.Errorf("event[%d] has position 0 (unassigned)", i)
		}
	}

	// Verify positions are strictly sequential (no gaps)
	for i := 1; i < len(loaded); i++ {
		if loaded[i].Position != loaded[i-1].Position+1 {
			t.Errorf("gap in positions: event[%d].Position=%d, event[%d].Position=%d",
				i-1, loaded[i-1].Position, i, loaded[i].Position)
		}
	}

	t.Logf("✅ All positions are unique and sequential: %v", []int64{
		loaded[0].Position,
		loaded[1].Position,
		loaded[2].Position,
	})
}

// TestMultipleBatchPositions verifies positions continue correctly across multiple appends.
func TestMultipleBatchPositions(t *testing.T) {
	store, err := sqlite.NewEventStore(
		sqlite.WithDSN(":memory:"),
		sqlite.WithWALMode(false),
	)
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	defer store.Close()

	// First batch
	batch1 := []*domain.Event{
		{
			ID:            "e1",
			AggregateID:   "agg-1",
			AggregateType: "Test",
			EventType:     "Created",
			Version:       1,
			Timestamp:     time.Now(),
			Data:          []byte("data"),
		},
		{
			ID:            "e2",
			AggregateID:   "agg-1",
			AggregateType: "Test",
			EventType:     "Updated",
			Version:       2,
			Timestamp:     time.Now(),
			Data:          []byte("data"),
		},
	}

	err = store.AppendEvents("agg-1", 0, batch1)
	if err != nil {
		t.Fatalf("failed to append batch1: %v", err)
	}

	// Second batch (different aggregate)
	batch2 := []*domain.Event{
		{
			ID:            "e3",
			AggregateID:   "agg-2",
			AggregateType: "Test",
			EventType:     "Created",
			Version:       1,
			Timestamp:     time.Now(),
			Data:          []byte("data"),
		},
		{
			ID:            "e4",
			AggregateID:   "agg-2",
			AggregateType: "Test",
			EventType:     "Updated",
			Version:       2,
			Timestamp:     time.Now(),
			Data:          []byte("data"),
		},
	}

	err = store.AppendEvents("agg-2", 0, batch2)
	if err != nil {
		t.Fatalf("failed to append batch2: %v", err)
	}

	// Load all events
	all, err := store.LoadAllEvents(0, 100)
	if err != nil {
		t.Fatalf("failed to load all events: %v", err)
	}

	// Verify positions are 1, 2, 3, 4 (continuous across aggregates)
	expectedPositions := []int64{1, 2, 3, 4}
	if len(all) != len(expectedPositions) {
		t.Fatalf("expected %d events, got %d", len(expectedPositions), len(all))
	}

	for i, event := range all {
		if event.Position != expectedPositions[i] {
			t.Errorf("event[%d] has position %d, expected %d", i, event.Position, expectedPositions[i])
		}
	}

	t.Logf("✅ Positions span correctly across aggregates: %v", expectedPositions)
}
