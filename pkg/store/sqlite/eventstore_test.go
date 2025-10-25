package sqlite_test

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/plaenen/eventstore/pkg/domain"
	"github.com/plaenen/eventstore/pkg/eventsourcing"
	storelib "github.com/plaenen/eventstore/pkg/store"
	"github.com/plaenen/eventstore/pkg/store/sqlite"
)

func TestEventStore(t *testing.T) {
	// Create in-memory store
	store, err := sqlite.NewEventStore(
		sqlite.WithDSN(":memory:"),
		sqlite.WithWALMode(false),
	)
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	defer store.Close()

	// Test basic event appending
	t.Run("AppendAndLoadEvents", func(t *testing.T) {
		aggregateID := "test-aggregate-1"
		events := []*domain.Event{
			{
				ID:            "event-1",
				AggregateID:   aggregateID,
				AggregateType: "TestAggregate",
				EventType:     "test.Created",
				Version:       1,
				Timestamp:     time.Now(),
				Data:          []byte("test data"),
				Metadata: domain.EventMetadata{
					PrincipalID: "test-user",
				},
			},
		}

		err := store.AppendEvents(aggregateID, 0, events)
		if err != nil {
			t.Fatalf("failed to append events: %v", err)
		}

		loaded, err := store.LoadEvents(aggregateID, 0)
		if err != nil {
			t.Fatalf("failed to load events: %v", err)
		}

		if len(loaded) != 1 {
			t.Fatalf("expected 1 event, got %d", len(loaded))
		}

		if loaded[0].ID != "event-1" {
			t.Errorf("expected event ID 'event-1', got '%s'", loaded[0].ID)
		}
	})

	// Test optimistic concurrency
	t.Run("ConcurrencyConflict", func(t *testing.T) {
		aggregateID := "test-aggregate-2"

		// First event
		err := store.AppendEvents(aggregateID, 0, []*domain.Event{
			{
				ID:            "event-2",
				AggregateID:   aggregateID,
				AggregateType: "TestAggregate",
				EventType:     "test.Created",
				Version:       1,
				Timestamp:     time.Now(),
				Data:          []byte("test"),
				Metadata:      domain.EventMetadata{},
			},
		})
		if err != nil {
			t.Fatalf("failed to append first event: %v", err)
		}

		// Try to append with wrong expected version
		err = store.AppendEvents(aggregateID, 0, []*domain.Event{
			{
				ID:            "event-3",
				AggregateID:   aggregateID,
				AggregateType: "TestAggregate",
				EventType:     "test.Updated",
				Version:       2,
				Timestamp:     time.Now(),
				Data:          []byte("test"),
				Metadata:      domain.EventMetadata{},
			},
		})

		if !errors.Is(err, domain.ErrConcurrencyConflict) {
			t.Errorf("expected concurrency conflict, got %v", err)
		}
	})

	// Test unique constraints
	t.Run("UniqueConstraints", func(t *testing.T) {
		aggregateID1 := "test-aggregate-3"
		aggregateID2 := "test-aggregate-4"

		// Claim unique email
		err := store.AppendEvents(aggregateID1, 0, []*domain.Event{
			{
				ID:            "event-4",
				AggregateID:   aggregateID1,
				AggregateType: "User",
				EventType:     "user.Created",
				Version:       1,
				Timestamp:     time.Now(),
				Data:          []byte("test"),
				Metadata:      domain.EventMetadata{},
				UniqueConstraints: []domain.UniqueConstraint{
					{
						IndexName: "user_email",
						Value:     "test@example.com",
						Operation: domain.ConstraintClaim,
					},
				},
			},
		})
		if err != nil {
			t.Fatalf("failed to claim unique constraint: %v", err)
		}

		// Try to claim same email with different aggregate
		err = store.AppendEvents(aggregateID2, 0, []*domain.Event{
			{
				ID:            "event-5",
				AggregateID:   aggregateID2,
				AggregateType: "User",
				EventType:     "user.Created",
				Version:       1,
				Timestamp:     time.Now(),
				Data:          []byte("test"),
				Metadata:      domain.EventMetadata{},
				UniqueConstraints: []domain.UniqueConstraint{
					{
						IndexName: "user_email",
						Value:     "test@example.com",
						Operation: domain.ConstraintClaim,
					},
				},
			},
		})

		if err == nil {
			t.Error("expected unique constraint violation")
		}

		// Check uniqueness
		available, ownerID, err := store.CheckUniqueness("user_email", "test@example.com")
		if err != nil {
			t.Fatalf("failed to check uniqueness: %v", err)
		}
		if available {
			t.Error("expected email to be unavailable")
		}
		if ownerID != aggregateID1 {
			t.Errorf("expected owner %s, got %s", aggregateID1, ownerID)
		}
	})

	// Test idempotency
	t.Run("Idempotency", func(t *testing.T) {
		aggregateID := "test-aggregate-5"
		commandID := "test-command-1"

		events := []*domain.Event{
			{
				ID:            eventsourcing.GenerateDeterministicEventID(commandID, aggregateID, 0),
				AggregateID:   aggregateID,
				AggregateType: "TestAggregate",
				EventType:     "test.Created",
				Version:       1,
				Timestamp:     time.Now(),
				Data:          []byte("test"),
				Metadata: domain.EventMetadata{
					CausationID: commandID,
				},
			},
		}

		// First append
		result1, err := store.AppendEventsIdempotent(aggregateID, 0, events, commandID, 24*time.Hour)
		if err != nil {
			t.Fatalf("failed first append: %v", err)
		}
		if result1.AlreadyProcessed {
			t.Error("first append should not be marked as already processed")
		}

		// Second append with same command ID
		result2, err := store.AppendEventsIdempotent(aggregateID, 0, events, commandID, 24*time.Hour)
		if err != nil {
			t.Fatalf("failed second append: %v", err)
		}
		if !result2.AlreadyProcessed {
			t.Error("second append should be marked as already processed")
		}

		// Verify only one event was persisted
		loaded, err := store.LoadEvents(aggregateID, 0)
		if err != nil {
			t.Fatalf("failed to load events: %v", err)
		}
		if len(loaded) != 1 {
			t.Errorf("expected 1 event, got %d", len(loaded))
		}
	})

	// Test checkpoint store with transactions
	t.Run("CheckpointStoreTransactions", func(t *testing.T) {
		checkpointStore, err := sqlite.NewCheckpointStore(store.DB())
		if err != nil {
			t.Fatalf("failed to create checkpoint store: %v", err)
		}

		// Create a projection table for testing
		_, err = store.DB().Exec(`
			CREATE TABLE IF NOT EXISTS test_projection (
				id INTEGER PRIMARY KEY,
				value TEXT
			)
		`)
		if err != nil {
			t.Fatalf("failed to create projection table: %v", err)
		}

		// Test transactional update
		tx, err := checkpointStore.DB().Begin()
		if err != nil {
			t.Fatalf("failed to begin transaction: %v", err)
		}

		// Update projection
		_, err = tx.Exec("INSERT INTO test_projection (id, value) VALUES (?, ?)", 1, "atomic-test")
		if err != nil {
			tx.Rollback()
			t.Fatalf("failed to insert into projection: %v", err)
		}

		// Save checkpoint in same transaction
		checkpoint := &storelib.ProjectionCheckpoint{
			ProjectionName: "test-projection",
			Position:       42,
			LastEventID:    "event-42",
			UpdatedAt:      time.Now(),
		}
		err = checkpointStore.SaveInTx(tx, checkpoint)
		if err != nil {
			tx.Rollback()
			t.Fatalf("failed to save checkpoint in transaction: %v", err)
		}

		// Commit - both projection and checkpoint should be saved atomically
		err = tx.Commit()
		if err != nil {
			t.Fatalf("failed to commit transaction: %v", err)
		}

		// Verify both were saved
		var value string
		err = store.DB().QueryRow("SELECT value FROM test_projection WHERE id = 1").Scan(&value)
		if err != nil {
			t.Fatalf("projection data not found: %v", err)
		}
		if value != "atomic-test" {
			t.Errorf("expected value 'atomic-test', got '%s'", value)
		}

		loaded, err := checkpointStore.Load("test-projection")
		if err != nil {
			t.Fatalf("checkpoint not found: %v", err)
		}
		if loaded.Position != 42 {
			t.Errorf("expected position 42, got %d", loaded.Position)
		}

		t.Log("Transactional checkpoint update successful - no dual-write issue!")
	})
}

func TestMain(m *testing.M) {
	// Override time function for deterministic testing
	eventsourcing.TimeFunc = func() time.Time {
		return time.Unix(1234567890, 0)
	}

	code := m.Run()

	// Restore original time function
	eventsourcing.TimeFunc = time.Now

	os.Exit(code)
}
