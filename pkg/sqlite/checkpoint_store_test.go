package sqlite_test

import (
	"testing"
	"time"

	"github.com/plaenen/eventsourcing/pkg/eventsourcing"
	"github.com/plaenen/eventsourcing/pkg/sqlite"
)

func TestCheckpointStore(t *testing.T) {
	// Create in-memory store
	eventStore, err := sqlite.NewEventStore(
		sqlite.WithDSN(":memory:"),
		sqlite.WithWALMode(false),
	)
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	defer eventStore.Close()

	checkpointStore, err := sqlite.NewCheckpointStore(eventStore.DB())
	if err != nil {
		t.Fatalf("failed to create checkpoint store: %v", err)
	}

	t.Run("BasicCheckpointSaveLoad", func(t *testing.T) {
		checkpoint := &eventsourcing.ProjectionCheckpoint{
			ProjectionName: "test-projection",
			Position:       42,
			LastEventID:    "event-123",
			UpdatedAt:      time.Now(),
		}

		err := checkpointStore.Save(checkpoint)
		if err != nil {
			t.Fatalf("failed to save checkpoint: %v", err)
		}

		loaded, err := checkpointStore.Load("test-projection")
		if err != nil {
			t.Fatalf("failed to load checkpoint: %v", err)
		}

		if loaded.Position != checkpoint.Position {
			t.Errorf("expected position %d, got %d", checkpoint.Position, loaded.Position)
		}

		if loaded.LastEventID != checkpoint.LastEventID {
			t.Errorf("expected last event ID %s, got %s", checkpoint.LastEventID, loaded.LastEventID)
		}
	})

	t.Run("TransactionalCheckpointUpdate", func(t *testing.T) {
		db := checkpointStore.DB()

		// Create a projection table
		_, err := db.Exec(`
			CREATE TABLE test_projection (
				id INTEGER PRIMARY KEY,
				value TEXT
			)
		`)
		if err != nil {
			t.Fatalf("failed to create projection table: %v", err)
		}

		// Test successful transaction
		t.Run("SuccessfulTransaction", func(t *testing.T) {
			tx, err := db.Begin()
			if err != nil {
				t.Fatalf("failed to begin transaction: %v", err)
			}
			defer tx.Rollback()

			// Update projection
			_, err = tx.Exec("INSERT INTO test_projection (id, value) VALUES (?, ?)", 1, "test-value")
			if err != nil {
				t.Fatalf("failed to insert into projection: %v", err)
			}

			// Save checkpoint in same transaction
			checkpoint := &eventsourcing.ProjectionCheckpoint{
				ProjectionName: "test-projection-tx",
				Position:       100,
				LastEventID:    "event-tx-100",
				UpdatedAt:      time.Now(),
			}
			err = checkpointStore.SaveInTx(tx, checkpoint)
			if err != nil {
				t.Fatalf("failed to save checkpoint in transaction: %v", err)
			}

			// Commit transaction
			err = tx.Commit()
			if err != nil {
				t.Fatalf("failed to commit transaction: %v", err)
			}

			// Verify both projection and checkpoint were saved
			var value string
			err = db.QueryRow("SELECT value FROM test_projection WHERE id = 1").Scan(&value)
			if err != nil {
				t.Fatalf("projection data not found: %v", err)
			}
			if value != "test-value" {
				t.Errorf("expected value 'test-value', got '%s'", value)
			}

			loaded, err := checkpointStore.Load("test-projection-tx")
			if err != nil {
				t.Fatalf("checkpoint not found: %v", err)
			}
			if loaded.Position != 100 {
				t.Errorf("expected position 100, got %d", loaded.Position)
			}
		})

		// Test rollback scenario
		t.Run("RollbackTransaction", func(t *testing.T) {
			tx, err := db.Begin()
			if err != nil {
				t.Fatalf("failed to begin transaction: %v", err)
			}
			defer tx.Rollback()

			// Update projection
			_, err = tx.Exec("INSERT INTO test_projection (id, value) VALUES (?, ?)", 2, "rollback-test")
			if err != nil {
				t.Fatalf("failed to insert into projection: %v", err)
			}

			// Save checkpoint in same transaction
			checkpoint := &eventsourcing.ProjectionCheckpoint{
				ProjectionName: "test-projection-rollback",
				Position:       200,
				LastEventID:    "event-rollback-200",
				UpdatedAt:      time.Now(),
			}
			err = checkpointStore.SaveInTx(tx, checkpoint)
			if err != nil {
				t.Fatalf("failed to save checkpoint in transaction: %v", err)
			}

			// Rollback transaction (simulating an error)
			err = tx.Rollback()
			if err != nil {
				t.Fatalf("failed to rollback transaction: %v", err)
			}

			// Verify neither projection nor checkpoint were saved
			var count int
			err = db.QueryRow("SELECT COUNT(*) FROM test_projection WHERE id = 2").Scan(&count)
			if err != nil {
				t.Fatalf("failed to query projection: %v", err)
			}
			if count != 0 {
				t.Error("projection data should have been rolled back")
			}

			_, err = checkpointStore.Load("test-projection-rollback")
			if err == nil {
				t.Error("checkpoint should not exist after rollback")
			}
		})
	})

	t.Run("DeleteCheckpoint", func(t *testing.T) {
		checkpoint := &eventsourcing.ProjectionCheckpoint{
			ProjectionName: "delete-test",
			Position:       999,
			LastEventID:    "event-999",
			UpdatedAt:      time.Now(),
		}

		err := checkpointStore.Save(checkpoint)
		if err != nil {
			t.Fatalf("failed to save checkpoint: %v", err)
		}

		err = checkpointStore.Delete("delete-test")
		if err != nil {
			t.Fatalf("failed to delete checkpoint: %v", err)
		}

		_, err = checkpointStore.Load("delete-test")
		if err == nil {
			t.Error("checkpoint should not exist after deletion")
		}
	})
}
