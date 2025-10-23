package migrate

import (
	"database/sql"
	"embed"
	"testing"

	_ "modernc.org/sqlite"
)

//go:embed testdata/*.sql
var testMigrationsFS embed.FS

func TestMigrator(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	m := New(db, "test_migrations")

	// Test ensure table creation
	err = m.ensureMigrationTable()
	if err != nil {
		t.Fatalf("failed to ensure migration table: %v", err)
	}

	// Test getting version from empty table
	version, err := m.getCurrentVersion()
	if err != nil {
		t.Fatalf("failed to get current version: %v", err)
	}
	if version != 0 {
		t.Errorf("expected version 0, got %d", version)
	}

	t.Log("Basic migration table operations work!")
}

func TestMigratorWithFS(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	m := New(db, "test_migrations")

	// Load migrations from embedded FS
	err = m.LoadFromFS(testMigrationsFS, "testdata")
	if err != nil {
		t.Fatalf("failed to load migrations: %v", err)
	}

	if len(m.migrations) == 0 {
		t.Fatal("no migrations loaded")
	}

	t.Logf("Loaded %d migration(s)", len(m.migrations))

	// Run migrations
	err = m.Up()
	if err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	// Verify version
	version, err := m.Version()
	if err != nil {
		t.Fatalf("failed to get version: %v", err)
	}

	if version != 1 {
		t.Errorf("expected version 1, got %d", version)
	}

	// Verify table was created
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM test_table").Scan(&count)
	if err != nil {
		t.Fatalf("test table not created: %v", err)
	}

	t.Log("Migration system works end-to-end!")
}
