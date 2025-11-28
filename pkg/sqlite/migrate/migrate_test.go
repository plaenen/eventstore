package migrate

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestMigrator(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open db: %v", err)
	}
	defer db.Close()

	m := New(db, "test_migrations")

	// Test initialization
	if m.tableName != "test_migrations" {
		t.Errorf("Expected table name test_migrations, got %s", m.tableName)
	}

	// Test ensureMigrationTable
	if err := m.ensureMigrationTable(); err != nil {
		t.Fatalf("Failed to ensure migration table: %v", err)
	}

	// Test getCurrentVersion (should be 0)
	version, err := m.getCurrentVersion()
	if err != nil {
		t.Fatalf("Failed to get current version: %v", err)
	}
	if version != 0 {
		t.Errorf("Expected version 0, got %d", version)
	}
}

func TestSplitSQL(t *testing.T) {
	sql := `
	-- This is a comment
	CREATE TABLE foo (id INT);
	-- Another comment
	INSERT INTO foo VALUES (1);
	`
	stmts := splitSQL(sql)
	if len(stmts) != 2 {
		t.Errorf("Expected 2 statements, got %d", len(stmts))
	}
	if stmts[0] != "CREATE TABLE foo (id INT)" {
		t.Errorf("Unexpected statement 1: %s", stmts[0])
	}
	if stmts[1] != "INSERT INTO foo VALUES (1)" {
		t.Errorf("Unexpected statement 2: %s", stmts[1])
	}
}
