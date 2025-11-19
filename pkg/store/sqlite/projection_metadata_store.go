package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/plaenen/eventstore/pkg/domain"
	"github.com/plaenen/eventstore/pkg/store/sqlite/sqlcgen"
)

// ProjectionMetadataStore is a SQLite-based implementation of store.ProjectionMetadataStore.
// It manages projection lifecycle metadata including schema versions, rebuild flags,
// and custom configuration properties.
//
// The ProjectionMetadataStore can use either:
// 1. The same database as EventStore (for co-located deployments)
// 2. The same database as CheckpointStore (recommended - keeps projection data together)
// 3. A separate database (for independent scaling)
type ProjectionMetadataStore struct {
	db      *sql.DB
	queries *sqlcgen.Queries
}

// metadataStoreConfig holds internal configuration for the metadata store.
type metadataStoreConfig struct {
	// autoMigrate automatically runs pending migrations on startup
	autoMigrate bool
}

// defaultMetadataStoreConfig returns sensible defaults.
func defaultMetadataStoreConfig() metadataStoreConfig {
	return metadataStoreConfig{
		autoMigrate: true,
	}
}

// MetadataStoreOption is a function that configures a ProjectionMetadataStore.
type MetadataStoreOption func(*metadataStoreConfig)

// WithMetadataAutoMigrate enables automatic migration on startup.
// When enabled, the metadata store will automatically run pending migrations.
func WithMetadataAutoMigrate(enabled bool) MetadataStoreOption {
	return func(c *metadataStoreConfig) {
		c.autoMigrate = enabled
	}
}

// NewProjectionMetadataStore creates a new SQLite projection metadata store with the given database and options.
// By default, it will auto-migrate the database schema.
//
// The ProjectionMetadataStore can use:
// - The same database as CheckpointStore (recommended - keeps projection data together)
// - The same database as EventStore (pass eventStore.DB())
// - A separate database (create a new sql.DB instance)
//
// Example usage:
//
//	// Using the same database as CheckpointStore (recommended)
//	metadataStore, err := sqlite.NewProjectionMetadataStore(checkpointStore.DB())
//
//	// Using the same database as EventStore
//	metadataStore, err := sqlite.NewProjectionMetadataStore(eventStore.DB())
//
//	// Using a separate database without auto-migration
//	db, _ := sql.Open("sqlite", "projections.db")
//	metadataStore, err := sqlite.NewProjectionMetadataStore(
//	    db,
//	    sqlite.WithMetadataAutoMigrate(false),
//	)
func NewProjectionMetadataStore(db *sql.DB, opts ...MetadataStoreOption) (*ProjectionMetadataStore, error) {
	// Start with defaults and apply options
	config := defaultMetadataStoreConfig()
	for _, opt := range opts {
		opt(&config)
	}

	store := &ProjectionMetadataStore{
		db:      db,
		queries: sqlcgen.New(db),
	}

	// Run migrations if auto-migrate is enabled
	if config.autoMigrate {
		if err := runMetadataMigrations(db); err != nil {
			return nil, fmt.Errorf("failed to run metadata migrations: %w", err)
		}
	}

	return store, nil
}

// DB returns the underlying database connection for creating transactions.
func (s *ProjectionMetadataStore) DB() *sql.DB {
	return s.db
}

// Get retrieves a metadata value by key.
// Returns empty string if key doesn't exist.
func (s *ProjectionMetadataStore) Get(projectionName, key string) (string, error) {
	ctx := context.Background()

	value, err := s.queries.GetMetadata(ctx, sqlcgen.GetMetadataParams{
		ProjectionName: projectionName,
		Key:            key,
	})

	if err == sql.ErrNoRows {
		return "", nil // Key doesn't exist - return empty string
	}

	if err != nil {
		return "", fmt.Errorf("failed to get metadata: %w", err)
	}

	return value, nil
}

// Set saves a metadata key-value pair.
// Creates or updates the value atomically.
func (s *ProjectionMetadataStore) Set(projectionName, key, value string) error {
	ctx := context.Background()

	err := s.queries.SetMetadata(ctx, sqlcgen.SetMetadataParams{
		ProjectionName: projectionName,
		Key:            key,
		Value:          value,
		UpdatedAt:      domain.Now().Unix(),
	})

	if err != nil {
		return fmt.Errorf("failed to set metadata: %w", err)
	}

	return nil
}

// SetInTx saves a metadata key-value pair within a transaction.
// Useful for atomic operations with projection updates.
func (s *ProjectionMetadataStore) SetInTx(tx *sql.Tx, projectionName, key, value string) error {
	ctx := context.Background()

	txQueries := s.queries.WithTx(tx)
	err := txQueries.SetMetadata(ctx, sqlcgen.SetMetadataParams{
		ProjectionName: projectionName,
		Key:            key,
		Value:          value,
		UpdatedAt:      domain.Now().Unix(),
	})

	if err != nil {
		return fmt.Errorf("failed to set metadata in transaction: %w", err)
	}

	return nil
}

// Delete removes a metadata key.
// No-op if key doesn't exist.
func (s *ProjectionMetadataStore) Delete(projectionName, key string) error {
	ctx := context.Background()

	err := s.queries.DeleteMetadata(ctx, sqlcgen.DeleteMetadataParams{
		ProjectionName: projectionName,
		Key:            key,
	})

	if err != nil {
		return fmt.Errorf("failed to delete metadata: %w", err)
	}

	return nil
}

// DeleteInTx removes a metadata key within a transaction.
func (s *ProjectionMetadataStore) DeleteInTx(tx *sql.Tx, projectionName, key string) error {
	ctx := context.Background()

	txQueries := s.queries.WithTx(tx)
	err := txQueries.DeleteMetadata(ctx, sqlcgen.DeleteMetadataParams{
		ProjectionName: projectionName,
		Key:            key,
	})

	if err != nil {
		return fmt.Errorf("failed to delete metadata in transaction: %w", err)
	}

	return nil
}

// GetAll retrieves all metadata for a projection as a map.
// Returns empty map if projection has no metadata.
func (s *ProjectionMetadataStore) GetAll(projectionName string) (map[string]string, error) {
	ctx := context.Background()

	rows, err := s.queries.GetAllMetadata(ctx, projectionName)
	if err != nil {
		return nil, fmt.Errorf("failed to get all metadata: %w", err)
	}

	// Convert rows to map
	metadata := make(map[string]string, len(rows))
	for _, row := range rows {
		metadata[row.Key] = row.Value
	}

	return metadata, nil
}

// DeleteAll removes all metadata for a projection.
// Useful during projection cleanup/removal.
func (s *ProjectionMetadataStore) DeleteAll(projectionName string) error {
	ctx := context.Background()

	err := s.queries.DeleteAllMetadata(ctx, projectionName)
	if err != nil {
		return fmt.Errorf("failed to delete all metadata: %w", err)
	}

	return nil
}

// DeleteAllInTx removes all metadata for a projection within a transaction.
func (s *ProjectionMetadataStore) DeleteAllInTx(tx *sql.Tx, projectionName string) error {
	ctx := context.Background()

	txQueries := s.queries.WithTx(tx)
	err := txQueries.DeleteAllMetadata(ctx, projectionName)
	if err != nil {
		return fmt.Errorf("failed to delete all metadata in transaction: %w", err)
	}

	return nil
}

// ListProjections returns a list of all projection names that have metadata.
// Useful for discovering which projections are configured.
func (s *ProjectionMetadataStore) ListProjections() ([]string, error) {
	ctx := context.Background()

	projections, err := s.queries.ListProjectionsWithMetadata(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list projections: %w", err)
	}

	return projections, nil
}
