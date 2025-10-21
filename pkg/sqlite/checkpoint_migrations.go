package sqlite

import (
	"database/sql"
	"embed"
	"fmt"

	"github.com/plaenen/eventstore/pkg/sqlite/migrate"
)

//go:embed checkpoint_migrations/*.sql
var checkpointMigrationsFS embed.FS

// runCheckpointMigrations runs all pending checkpoint migrations using our custom migrator.
func runCheckpointMigrations(db *sql.DB) error {
	m := migrate.New(db, "checkpoint_schema_migrations")

	if err := m.LoadFromFS(checkpointMigrationsFS, "checkpoint_migrations"); err != nil {
		return fmt.Errorf("failed to load checkpoint migrations: %w", err)
	}

	if err := m.Up(); err != nil {
		return fmt.Errorf("failed to run checkpoint migrations: %w", err)
	}

	return nil
}
