package migrate

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/plaenen/eventstore/pkg/validation"
)

// Migration represents a single database migration.
type Migration struct {
	Version int
	Name    string
	Up      string
	Down    string
}

// Migrator handles database migrations.
type Migrator struct {
	db         *sql.DB
	migrations []Migration
	tableName  string
}

// New creates a new migrator instance.
// tableName is the name of the table used to track migrations (e.g., "schema_migrations").
//
// SECURITY: The tableName is validated to prevent SQL injection attacks.
// It must be a valid SQL identifier (alphanumeric and underscores only).
// If invalid, the migrator will panic during initialization.
func New(db *sql.DB, tableName string) *Migrator {
	// Validate table name to prevent SQL injection
	// This is critical because tableName is used in fmt.Sprintf for SQL queries
	if err := validateTableName(tableName); err != nil {
		panic(fmt.Sprintf("invalid migration table name %q: %v", tableName, err))
	}

	return &Migrator{
		db:         db,
		migrations: []Migration{},
		tableName:  tableName,
	}
}

// LoadFromFS loads migrations from an embedded filesystem.
// The directory should contain files named like: 000001_name.up.sql, 000001_name.down.sql
func (m *Migrator) LoadFromFS(fsys embed.FS, dir string) error {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return fmt.Errorf("failed to read migration directory: %w", err)
	}

	migrationMap := make(map[int]*Migration)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}

		// Parse filename: 000001_name.up.sql or 000001_name.down.sql
		parts := strings.SplitN(name, "_", 2)
		if len(parts) != 2 {
			continue
		}

		version, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}

		// Read file content
		content, err := fs.ReadFile(fsys, filepath.Join(dir, name))
		if err != nil {
			return fmt.Errorf("failed to read migration file %s: %w", name, err)
		}

		// Get or create migration entry
		migration, exists := migrationMap[version]
		if !exists {
			migration = &Migration{Version: version}
			migrationMap[version] = migration
		}

		// Extract name and direction
		remainder := parts[1]
		if strings.HasSuffix(remainder, ".up.sql") {
			migration.Name = strings.TrimSuffix(remainder, ".up.sql")
			migration.Up = string(content)
		} else if strings.HasSuffix(remainder, ".down.sql") {
			migration.Down = string(content)
		}
	}

	// Convert map to sorted slice
	for _, migration := range migrationMap {
		m.migrations = append(m.migrations, *migration)
	}

	sort.Slice(m.migrations, func(i, j int) bool {
		return m.migrations[i].Version < m.migrations[j].Version
	})

	return nil
}

// ensureMigrationTable creates the migration tracking table if it doesn't exist.
func (m *Migrator) ensureMigrationTable() error {
	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			version INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at INTEGER NOT NULL
		)
	`, m.tableName)
	_, err := m.db.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to create table %s: %w", m.tableName, err)
	}
	return nil
}

// getCurrentVersion returns the current migration version.
// Returns 0 if no migrations have been applied.
func (m *Migrator) getCurrentVersion() (int, error) {
	var version int
	err := m.db.QueryRow(fmt.Sprintf(
		"SELECT COALESCE(MAX(version), 0) FROM %s", m.tableName,
	)).Scan(&version)
	if err != nil {
		return 0, err
	}
	return version, nil
}

// Up runs all pending migrations.
func (m *Migrator) Up() error {
	if err := m.ensureMigrationTable(); err != nil {
		return fmt.Errorf("failed to ensure migration table: %w", err)
	}

	currentVersion, err := m.getCurrentVersion()
	if err != nil {
		return fmt.Errorf("failed to get current version: %w", err)
	}

	// Find migrations to apply
	var toApply []Migration
	for _, migration := range m.migrations {
		if migration.Version > currentVersion {
			toApply = append(toApply, migration)
		}
	}

	if len(toApply) == 0 {
		return nil // No migrations to apply
	}

	// Apply each migration in a transaction
	for _, migration := range toApply {
		if err := m.applyMigration(migration); err != nil {
			return fmt.Errorf("failed to apply migration %d: %w", migration.Version, err)
		}
	}

	return nil
}

// applyMigration applies a single migration.
func (m *Migrator) applyMigration(migration Migration) error {
	tx, err := m.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Split SQL into individual statements and execute each one
	// LibSQL doesn't support multi-statement execution in a single Exec()
	statements := splitSQL(migration.Up)
	for i, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("failed to execute statement %d: %w\nStatement: %s", i+1, err, stmt)
		}
	}

	// Record migration
	_, err = tx.Exec(fmt.Sprintf(
		"INSERT INTO %s (version, name, applied_at) VALUES (?, ?, ?)",
		m.tableName,
	), migration.Version, migration.Name, currentTimestamp())
	if err != nil {
		return fmt.Errorf("failed to record migration: %w", err)
	}

	return tx.Commit()
}

// splitSQL splits a SQL string into individual statements by semicolons.
// Removes SQL comments (-- style) before splitting.
func splitSQL(sql string) []string {
	// Remove single-line comments (--)
	lines := strings.Split(sql, "\n")
	var cleanedLines []string
	for _, line := range lines {
		// Find comment start
		if idx := strings.Index(line, "--"); idx >= 0 {
			line = line[:idx]
		}
		cleanedLines = append(cleanedLines, line)
	}

	cleaned := strings.Join(cleanedLines, "\n")

	// Split by semicolon
	parts := strings.Split(cleaned, ";")

	var statements []string
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			statements = append(statements, trimmed)
		}
	}

	return statements
}

// Down rolls back the last migration.
func (m *Migrator) Down() error {
	if err := m.ensureMigrationTable(); err != nil {
		return fmt.Errorf("failed to ensure migration table: %w", err)
	}

	currentVersion, err := m.getCurrentVersion()
	if err != nil {
		return fmt.Errorf("failed to get current version: %w", err)
	}

	if currentVersion == 0 {
		return fmt.Errorf("no migrations to roll back")
	}

	// Find migration to roll back
	var toRollback *Migration
	for i := range m.migrations {
		if m.migrations[i].Version == currentVersion {
			toRollback = &m.migrations[i]
			break
		}
	}

	if toRollback == nil {
		return fmt.Errorf("migration %d not found", currentVersion)
	}

	if toRollback.Down == "" {
		return fmt.Errorf("migration %d has no down script", currentVersion)
	}

	tx, err := m.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Execute rollback SQL
	if _, err := tx.Exec(toRollback.Down); err != nil {
		return fmt.Errorf("failed to execute rollback SQL: %w", err)
	}

	// Remove migration record
	_, err = tx.Exec(fmt.Sprintf(
		"DELETE FROM %s WHERE version = ?",
		m.tableName,
	), currentVersion)
	if err != nil {
		return fmt.Errorf("failed to remove migration record: %w", err)
	}

	return tx.Commit()
}

// Version returns the current migration version.
func (m *Migrator) Version() (int, error) {
	if err := m.ensureMigrationTable(); err != nil {
		return 0, err
	}
	return m.getCurrentVersion()
}

// currentTimestamp returns the current Unix timestamp.
func currentTimestamp() int64 {
	return time.Now().Unix()
}

// validateTableName validates that a table name is a safe SQL identifier.
// This prevents SQL injection attacks when table names are used in dynamic SQL.
//
// The validation uses pkg/validation.ValidateTableName which ensures:
// - Only alphanumeric characters and underscores
// - Must start with a letter or underscore
// - Maximum length of 128 characters
// - Not a reserved SQL keyword
// - No SQL injection patterns
func validateTableName(tableName string) error {
	return validation.ValidateTableName(tableName)
}
