package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/plaenen/eventstore/pkg/eventsourcing"
	"github.com/plaenen/eventstore/pkg/sqlite/sqlcgen"
)

// CheckpointStore is a SQLite-based implementation of eventsourcing.CheckpointStore.
// It supports both standalone operations and transactional operations when used
// with SaveInTx to ensure atomic updates with projections.
//
// The CheckpointStore can use either:
// 1. The same database as EventStore (for co-located deployments)
// 2. A separate database (for independent scaling of reads/projections)
type CheckpointStore struct {
	db      *sql.DB
	queries *sqlcgen.Queries
}

// checkpointStoreConfig holds internal configuration for the checkpoint store.
type checkpointStoreConfig struct {
	// autoMigrate automatically runs pending migrations on startup
	autoMigrate bool
}

// defaultCheckpointStoreConfig returns sensible defaults.
func defaultCheckpointStoreConfig() checkpointStoreConfig {
	return checkpointStoreConfig{
		autoMigrate: true,
	}
}

// CheckpointStoreOption is a function that configures a CheckpointStore.
type CheckpointStoreOption func(*checkpointStoreConfig)

// WithCheckpointAutoMigrate enables automatic migration on startup.
// When enabled, the checkpoint store will automatically run pending migrations.
func WithCheckpointAutoMigrate(enabled bool) CheckpointStoreOption {
	return func(c *checkpointStoreConfig) {
		c.autoMigrate = enabled
	}
}

// NewCheckpointStore creates a new SQLite checkpoint store with the given database and options.
// By default, it will auto-migrate the database schema.
//
// The CheckpointStore can use:
// - The same database as EventStore (pass eventStore.DB())
// - A separate database (create a new sql.DB instance)
//
// Example usage:
//
//	// Using the same database as EventStore
//	checkpointStore, err := sqlite.NewCheckpointStore(eventStore.DB())
//
//	// Using a separate database without auto-migration
//	db, _ := sql.Open("sqlite", "projections.db")
//	checkpointStore, err := sqlite.NewCheckpointStore(
//	    db,
//	    sqlite.WithCheckpointAutoMigrate(false),
//	)
func NewCheckpointStore(db *sql.DB, opts ...CheckpointStoreOption) (*CheckpointStore, error) {
	// Start with defaults and apply options
	config := defaultCheckpointStoreConfig()
	for _, opt := range opts {
		opt(&config)
	}

	store := &CheckpointStore{
		db:      db,
		queries: sqlcgen.New(db),
	}

	// Run migrations if auto-migrate is enabled
	if config.autoMigrate {
		if err := runCheckpointMigrations(db); err != nil {
			return nil, fmt.Errorf("failed to run checkpoint migrations: %w", err)
		}
	}

	return store, nil
}

// DB returns the underlying database connection for creating transactions.
func (s *CheckpointStore) DB() *sql.DB {
	return s.db
}

// Save saves a checkpoint in its own transaction.
// WARNING: For atomic projection updates, use SaveInTx instead to avoid dual-write issues.
func (s *CheckpointStore) Save(checkpoint *eventsourcing.ProjectionCheckpoint) error {
	ctx := context.Background()
	err := s.queries.SaveCheckpoint(ctx, sqlcgen.SaveCheckpointParams{
		ProjectionName: checkpoint.ProjectionName,
		Position:       checkpoint.Position,
		LastEventID:    checkpoint.LastEventID,
		UpdatedAt:      checkpoint.UpdatedAt.Unix(),
	})

	if err != nil {
		return fmt.Errorf("failed to save checkpoint: %w", err)
	}

	return nil
}

// SaveInTx saves a checkpoint within the provided transaction.
// This should be used when updating projections to ensure atomicity:
// the projection update and checkpoint save happen in the same transaction,
// avoiding dual-write issues.
//
// Example usage:
//
//	tx, err := checkpointStore.DB().Begin()
//	if err != nil {
//	    return err
//	}
//	defer tx.Rollback()
//
//	// Update your projection tables
//	_, err = tx.Exec("UPDATE my_projection SET ...")
//	if err != nil {
//	    return err
//	}
//
//	// Save checkpoint in same transaction
//	err = checkpointStore.SaveInTx(tx, checkpoint)
//	if err != nil {
//	    return err
//	}
//
//	return tx.Commit()
func (s *CheckpointStore) SaveInTx(tx *sql.Tx, checkpoint *eventsourcing.ProjectionCheckpoint) error {
	ctx := context.Background()
	queries := sqlcgen.New(tx)

	err := queries.SaveCheckpoint(ctx, sqlcgen.SaveCheckpointParams{
		ProjectionName: checkpoint.ProjectionName,
		Position:       checkpoint.Position,
		LastEventID:    checkpoint.LastEventID,
		UpdatedAt:      checkpoint.UpdatedAt.Unix(),
	})

	if err != nil {
		return fmt.Errorf("failed to save checkpoint in transaction: %w", err)
	}

	return nil
}

// Load loads a checkpoint for a projection.
func (s *CheckpointStore) Load(projectionName string) (*eventsourcing.ProjectionCheckpoint, error) {
	ctx := context.Background()
	row, err := s.queries.LoadCheckpoint(ctx, projectionName)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("checkpoint not found for projection %s", projectionName)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load checkpoint: %w", err)
	}

	checkpoint := eventsourcing.ProjectionCheckpoint{
		ProjectionName: row.ProjectionName,
		Position:       row.Position,
		LastEventID:    row.LastEventID,
		UpdatedAt:      time.Unix(row.UpdatedAt, 0),
	}

	return &checkpoint, nil
}

// Delete deletes a checkpoint (for rebuilding).
func (s *CheckpointStore) Delete(projectionName string) error {
	ctx := context.Background()
	err := s.queries.DeleteCheckpoint(ctx, projectionName)

	if err != nil {
		return fmt.Errorf("failed to delete checkpoint: %w", err)
	}

	return nil
}

// DeleteInTx deletes a checkpoint within the provided transaction.
// This should be used when resetting projections atomically.
func (s *CheckpointStore) DeleteInTx(tx *sql.Tx, projectionName string) error {
	ctx := context.Background()
	queries := sqlcgen.New(tx)

	err := queries.DeleteCheckpoint(ctx, projectionName)
	if err != nil {
		return fmt.Errorf("failed to delete checkpoint in transaction: %w", err)
	}

	return nil
}
