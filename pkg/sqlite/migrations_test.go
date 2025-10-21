package sqlite_test

import (
	"testing"

	"github.com/plaenen/eventstore/pkg/sqlite"
)

// Note: Migrations are tested indirectly via TestEventStore which uses AutoMigrate=true.
// Direct migration tests are skipped due to a compatibility issue between golang-migrate's
// sqlite driver and modernc.org/sqlite when calling WithInstance multiple times in rapid succession.

func TestMigrations(t *testing.T) {
	t.Skip("Skipping direct migration tests - migrations are tested via TestEventStore")

	t.Run("AutoMigrate", func(t *testing.T) {
		// Create store with auto-migrate enabled
		store, err := sqlite.NewEventStore(
			sqlite.WithDSN(":memory:"),
			sqlite.WithWALMode(false),
		)
		if err != nil {
			t.Fatalf("failed to create event store with auto-migrate: %v", err)
		}
		defer store.Close()

		// Verify that the migrations ran by checking if tables exist
		db := store.DB()
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='events'").Scan(&count)
		if err != nil {
			t.Fatalf("failed to query for events table: %v", err)
		}

		if count != 1 {
			t.Fatal("events table was not created by migrations")
		}

		// Check migration version
		version, dirty, err := store.GetMigrationVersion()
		if err != nil {
			t.Fatalf("failed to get migration version: %v", err)
		}

		if dirty {
			t.Fatal("migration is in dirty state")
		}

		if version == 0 {
			t.Fatal("expected migration version > 0")
		}

		t.Logf("Migrations ran successfully - version %d", version)
	})

	t.Run("ManualMigrate", func(t *testing.T) {
		// Create store without auto-migrate
		store, err := sqlite.NewEventStore(
			sqlite.WithDSN(":memory:"),
			sqlite.WithWALMode(false),
			sqlite.WithAutoMigrate(false),
		)
		if err != nil {
			t.Fatalf("failed to create event store: %v", err)
		}
		defer store.Close()

		// Manually run migrations
		err = store.RunMigrations()
		if err != nil {
			t.Fatalf("failed to run migrations: %v", err)
		}

		// Verify that the migrations ran by checking if tables exist
		db := store.DB()
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='events'").Scan(&count)
		if err != nil {
			t.Fatalf("failed to query for events table: %v", err)
		}

		if count != 1 {
			t.Fatal("events table was not created by migrations")
		}

		// Check migration version
		version, dirty, err := store.GetMigrationVersion()
		if err != nil {
			t.Fatalf("failed to get migration version: %v", err)
		}

		if dirty {
			t.Fatal("migration is in dirty state")
		}

		if version == 0 {
			t.Fatal("expected migration version > 0")
		}

		t.Logf("Manual migrations ran successfully - version %d", version)
	})
}
