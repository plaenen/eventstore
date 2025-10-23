package sqlite

import (
	"database/sql"
	"embed"
	"fmt"

	"github.com/plaenen/eventstore/pkg/store/sqlite/migrate"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// runMigrations runs all pending migrations using our custom migrator.
func runMigrations(db *sql.DB) error {
	m := migrate.New(db, "schema_migrations")

	if err := m.LoadFromFS(migrationsFS, "migrations"); err != nil {
		return fmt.Errorf("failed to load migrations: %w", err)
	}

	if err := m.Up(); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	return nil
}
