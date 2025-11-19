package sqlite

import (
	"database/sql"
	"embed"
	"fmt"

	"github.com/plaenen/eventstore/pkg/store/sqlite/migrate"
)

//go:embed metadata_migrations/*.sql
var metadataMigrationsFS embed.FS

// runMetadataMigrations runs all pending metadata migrations using our custom migrator.
func runMetadataMigrations(db *sql.DB) error {
	m := migrate.New(db, "metadata_schema_migrations")

	if err := m.LoadFromFS(metadataMigrationsFS, "metadata_migrations"); err != nil {
		return fmt.Errorf("failed to load metadata migrations: %w", err)
	}

	if err := m.Up(); err != nil {
		return fmt.Errorf("failed to run metadata migrations: %w", err)
	}

	return nil
}
