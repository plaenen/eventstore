package sqlite_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/plaenen/eventstore/pkg/eventsourcing"
	"github.com/plaenen/eventstore/pkg/store/sqlite"
	_ "modernc.org/sqlite"
)

func TestCheckpointStoreSeparateDatabase(t *testing.T) {
	t.Run("SameDatabase", func(t *testing.T) {
		// Create event store
		eventStore, err := sqlite.NewEventStore(
			sqlite.WithDSN(":memory:"),
			sqlite.WithWALMode(false),
		)
		if err != nil {
			t.Fatalf("failed to create event store: %v", err)
		}
		defer eventStore.Close()

		// Create checkpoint store using the same database
		checkpointStore, err := sqlite.NewCheckpointStore(eventStore.DB())
		if err != nil {
			t.Fatalf("failed to create checkpoint store: %v", err)
		}

		// Verify checkpoint operations work
		checkpoint := &eventsourcing.ProjectionCheckpoint{
			ProjectionName: "same-db-test",
			Position:       100,
			LastEventID:    "event-100",
			UpdatedAt:      time.Now(),
		}

		err = checkpointStore.Save(checkpoint)
		if err != nil {
			t.Fatalf("failed to save checkpoint: %v", err)
		}

		loaded, err := checkpointStore.Load("same-db-test")
		if err != nil {
			t.Fatalf("failed to load checkpoint: %v", err)
		}

		if loaded.Position != 100 {
			t.Errorf("expected position 100, got %d", loaded.Position)
		}

		t.Log("Checkpoint store works with same database as event store")
	})

	t.Run("SeparateDatabase", func(t *testing.T) {
		// Create event store in one database
		eventStore, err := sqlite.NewEventStore(
			sqlite.WithDSN(":memory:"),
			sqlite.WithWALMode(false),
		)
		if err != nil {
			t.Fatalf("failed to create event store: %v", err)
		}
		defer eventStore.Close()

		// Create a completely separate database for checkpoints
		checkpointDB, err := sql.Open("sqlite", ":memory:")
		if err != nil {
			t.Fatalf("failed to open checkpoint database: %v", err)
		}
		defer checkpointDB.Close()

		// :memory: databases need single connection
		checkpointDB.SetMaxOpenConns(1)
		checkpointDB.SetMaxIdleConns(1)

		// Create checkpoint store using separate database
		checkpointStore, err := sqlite.NewCheckpointStore(checkpointDB)
		if err != nil {
			t.Fatalf("failed to create checkpoint store: %v", err)
		}

		// Verify checkpoint operations work in separate database
		checkpoint := &eventsourcing.ProjectionCheckpoint{
			ProjectionName: "separate-db-test",
			Position:       200,
			LastEventID:    "event-200",
			UpdatedAt:      time.Now(),
		}

		err = checkpointStore.Save(checkpoint)
		if err != nil {
			t.Fatalf("failed to save checkpoint: %v", err)
		}

		loaded, err := checkpointStore.Load("separate-db-test")
		if err != nil {
			t.Fatalf("failed to load checkpoint: %v", err)
		}

		if loaded.Position != 200 {
			t.Errorf("expected position 200, got %d", loaded.Position)
		}

		// Verify the checkpoint table doesn't exist in event store database
		var count int
		err = eventStore.DB().QueryRow(`
			SELECT COUNT(*) FROM sqlite_master
			WHERE type='table' AND name='projection_checkpoints'
		`).Scan(&count)
		if err != nil {
			t.Fatalf("failed to query event store: %v", err)
		}

		if count != 0 {
			t.Error("projection_checkpoints table should NOT exist in event store database")
		}

		// Verify the checkpoint table DOES exist in checkpoint database
		err = checkpointDB.QueryRow(`
			SELECT COUNT(*) FROM sqlite_master
			WHERE type='table' AND name='projection_checkpoints'
		`).Scan(&count)
		if err != nil {
			t.Fatalf("failed to query checkpoint database: %v", err)
		}

		if count != 1 {
			t.Error("projection_checkpoints table SHOULD exist in checkpoint database")
		}

		t.Log("Checkpoint store works independently with separate database!")
	})

	t.Run("SeparateDatabaseWithTransaction", func(t *testing.T) {
		// Create projection database
		projectionDB, err := sql.Open("sqlite", ":memory:")
		if err != nil {
			t.Fatalf("failed to open projection database: %v", err)
		}
		defer projectionDB.Close()

		// :memory: databases need single connection
		projectionDB.SetMaxOpenConns(1)
		projectionDB.SetMaxIdleConns(1)

		// Initialize checkpoint store with separate database
		checkpointStore, err := sqlite.NewCheckpointStore(projectionDB)
		if err != nil {
			t.Fatalf("failed to create checkpoint store: %v", err)
		}

		// Create projection table in projection database
		_, err = projectionDB.Exec(`
			CREATE TABLE user_balances (
				user_id TEXT PRIMARY KEY,
				balance INTEGER
			)
		`)
		if err != nil {
			t.Fatalf("failed to create projection table: %v", err)
		}

		// Update projection and checkpoint atomically
		tx, err := projectionDB.Begin()
		if err != nil {
			t.Fatalf("failed to begin transaction: %v", err)
		}
		defer tx.Rollback()

		// Update projection
		_, err = tx.Exec("INSERT INTO user_balances (user_id, balance) VALUES (?, ?)", "user-1", 1000)
		if err != nil {
			t.Fatalf("failed to insert projection: %v", err)
		}

		// Save checkpoint in same transaction
		checkpoint := &eventsourcing.ProjectionCheckpoint{
			ProjectionName: "user-balances",
			Position:       50,
			LastEventID:    "event-50",
			UpdatedAt:      time.Now(),
		}
		err = checkpointStore.SaveInTx(tx, checkpoint)
		if err != nil {
			t.Fatalf("failed to save checkpoint in tx: %v", err)
		}

		err = tx.Commit()
		if err != nil {
			t.Fatalf("failed to commit: %v", err)
		}

		// Verify both were saved
		var balance int
		err = projectionDB.QueryRow("SELECT balance FROM user_balances WHERE user_id = ?", "user-1").Scan(&balance)
		if err != nil {
			t.Fatalf("failed to query projection: %v", err)
		}
		if balance != 1000 {
			t.Errorf("expected balance 1000, got %d", balance)
		}

		loaded, err := checkpointStore.Load("user-balances")
		if err != nil {
			t.Fatalf("failed to load checkpoint: %v", err)
		}
		if loaded.Position != 50 {
			t.Errorf("expected position 50, got %d", loaded.Position)
		}

		t.Log("Transactional updates work correctly in separate projection database!")
	})
}
