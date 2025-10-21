package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/plaenen/eventstore/pkg/eventsourcing"
	"github.com/plaenen/eventstore/pkg/sqlite/sqlcgen"
	_ "modernc.org/sqlite" // Pure Go SQLite driver
)

// EventStore is a SQLite-based implementation of eventsourcing.EventStore.
// It provides ACID guarantees for event persistence with no CGo dependencies.
type EventStore struct {
	db      *sql.DB
	queries *sqlcgen.Queries
	mu      sync.RWMutex // Protects concurrent access to connection pool
}

// eventStoreConfig holds internal configuration for the SQLite event store.
type eventStoreConfig struct {
	// dsn is the data source name (file path or ":memory:" for in-memory)
	dsn string

	// maxOpenConns sets the maximum number of open connections
	maxOpenConns int

	// maxIdleConns sets the maximum number of idle connections
	maxIdleConns int

	// walMode enables write-ahead logging for better concurrency
	walMode bool

	// autoMigrate automatically runs pending migrations on startup
	autoMigrate bool
}

// defaultEventStoreConfig returns sensible defaults.
func defaultEventStoreConfig() eventStoreConfig {
	return eventStoreConfig{
		dsn:          "eventstore.db",
		maxOpenConns: 25,
		maxIdleConns: 5,
		walMode:      true,
		autoMigrate:  true,
	}
}

// EventStoreOption is a function that configures an EventStore.
type EventStoreOption func(*eventStoreConfig)

// WithDSN sets the data source name (file path or ":memory:" for in-memory).
func WithDSN(dsn string) EventStoreOption {
	return func(c *eventStoreConfig) {
		c.dsn = dsn
	}
}

// With MemoryDatabase sets the database to an in-memory database.
func WithMemoryDatabase() EventStoreOption {
	return func(c *eventStoreConfig) {
		c.dsn = ":memory:"
	}
}

// With Filename sets the filename for the database.
func WithFilename(filename string) EventStoreOption {
	return func(c *eventStoreConfig) {
		c.dsn = filename
	}
}

// WithMaxOpenConns sets the maximum number of open connections to the database.
func WithMaxOpenConns(n int) EventStoreOption {
	return func(c *eventStoreConfig) {
		c.maxOpenConns = n
	}
}

// WithMaxIdleConns sets the maximum number of idle connections in the pool.
func WithMaxIdleConns(n int) EventStoreOption {
	return func(c *eventStoreConfig) {
		c.maxIdleConns = n
	}
}

// WithWALMode enables write-ahead logging for better concurrency.
// This is recommended for production use but not available for :memory: databases.
func WithWALMode(enabled bool) EventStoreOption {
	return func(c *eventStoreConfig) {
		c.walMode = enabled
	}
}

// WithAutoMigrate enables automatic migration on startup.
// When enabled, the event store will automatically run pending migrations.
func WithAutoMigrate(enabled bool) EventStoreOption {
	return func(c *eventStoreConfig) {
		c.autoMigrate = enabled
	}
}

// NewEventStore creates a new SQLite event store with the given options.
//
// Example usage:
//
//	// Use defaults (eventstore.db, WAL mode, auto-migrate)
//	store, err := sqlite.NewEventStore()
//
//	// In-memory database for testing
//	store, err := sqlite.NewEventStore(
//	    sqlite.WithDSN(":memory:"),
//	)
//
//	// Custom configuration
//	store, err := sqlite.NewEventStore(
//	    sqlite.WithDSN("/path/to/db"),
//	    sqlite.WithMaxOpenConns(50),
//	    sqlite.WithWALMode(true),
//	)
func NewEventStore(opts ...EventStoreOption) (*EventStore, error) {
	// Start with defaults and apply options
	config := defaultEventStoreConfig()
	for _, opt := range opts {
		opt(&config)
	}

	db, err := sql.Open("sqlite", config.dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// For :memory: databases, we need to ensure we use a single connection
	// Otherwise each connection gets its own isolated in-memory database
	if config.dsn == ":memory:" {
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
	} else {
		// Configure connection pool
		db.SetMaxOpenConns(config.maxOpenConns)
		db.SetMaxIdleConns(config.maxIdleConns)
	}
	db.SetConnMaxLifetime(time.Hour)

	store := &EventStore{
		db:      db,
		queries: sqlcgen.New(db),
	}

	// Configure WAL mode if enabled
	if config.walMode {
		if err := store.setWALMode(); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to set WAL mode: %w", err)
		}
	}

	// Run migrations if auto-migrate is enabled
	if config.autoMigrate {
		if err := runMigrations(db); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to run migrations: %w", err)
		}
	}

	return store, nil
}

// setWALMode configures the database for WAL mode.
func (s *EventStore) setWALMode() error {
	_, err := s.db.Exec(`
		PRAGMA journal_mode = WAL;
		PRAGMA synchronous = NORMAL;
		PRAGMA foreign_keys = ON;
	`)
	return err
}

// AppendEvents appends events to an aggregate's stream atomically.
func (s *EventStore) AppendEvents(aggregateID string, expectedVersion int64, events []*eventsourcing.Event) error {
	if len(events) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Check optimistic concurrency
	ctx := context.Background()
	queries := sqlcgen.New(tx)
	currentVersionRaw, err := queries.GetAggregateVersion(ctx, aggregateID)
	if err != nil {
		return fmt.Errorf("failed to check current version: %w", err)
	}
	currentVersion := currentVersionRaw.(int64)

	if currentVersion != expectedVersion {
		return eventsourcing.ErrConcurrencyConflict
	}

	// Validate and insert unique constraints
	for _, event := range events {
		if err := s.validateConstraints(tx, event, aggregateID); err != nil {
			return err
		}
	}

	// Insert events
	for _, event := range events {
		metadataJSON, _ := json.Marshal(event.Metadata)
		constraintsJSON, _ := json.Marshal(event.UniqueConstraints)

		err = queries.InsertEvent(ctx, sqlcgen.InsertEventParams{
			EventID:       event.ID,
			AggregateID:   event.AggregateID,
			AggregateType: event.AggregateType,
			EventType:     event.EventType,
			Version:       event.Version,
			Timestamp:     event.Timestamp.Unix(),
			Data:          event.Data,
			Metadata:      string(metadataJSON),
			Constraints:   sql.NullString{String: string(constraintsJSON), Valid: len(constraintsJSON) > 0},
		})
		if err != nil {
			return fmt.Errorf("failed to insert event: %w", err)
		}
	}

	// Update global position
	if err := s.updatePositions(tx); err != nil {
		return fmt.Errorf("failed to update positions: %w", err)
	}

	return tx.Commit()
}

// AppendEventsIdempotent appends events with command-level idempotency.
func (s *EventStore) AppendEventsIdempotent(
	aggregateID string,
	expectedVersion int64,
	events []*eventsourcing.Event,
	commandID string,
	ttl time.Duration,
) (*eventsourcing.CommandResult, error) {
	if commandID == "" {
		return nil, eventsourcing.ErrInvalidCommand
	}

	if len(events) == 0 {
		return &eventsourcing.CommandResult{
			CommandID: commandID,
			Events:    nil,
		}, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if command already processed
	result, err := s.getCommandResultNoLock(commandID)
	if err == nil && result != nil {
		return result, nil // Idempotent return
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Double-check within transaction
	ctx := context.Background()
	queries := sqlcgen.New(tx)
	existingCommand, err := queries.CheckCommandExists(ctx, commandID)
	if err == nil && existingCommand != "" {
		// Command was processed between our check and tx start
		tx.Rollback()
		return s.getCommandResultNoLock(commandID)
	} else if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to check processed commands: %w", err)
	}

	// Check optimistic concurrency
	currentVersionRaw, err := queries.GetAggregateVersion(ctx, aggregateID)
	if err != nil {
		return nil, fmt.Errorf("failed to check current version: %w", err)
	}
	currentVersion := currentVersionRaw.(int64)

	if currentVersion != expectedVersion {
		return nil, eventsourcing.ErrConcurrencyConflict
	}

	// Validate and insert unique constraints
	for _, event := range events {
		if err := s.validateConstraints(tx, event, aggregateID); err != nil {
			return nil, err
		}
	}

	// Insert events
	eventIDs := make([]string, len(events))
	for i, event := range events {
		metadataJSON, _ := json.Marshal(event.Metadata)
		constraintsJSON, _ := json.Marshal(event.UniqueConstraints)

		err = queries.InsertEvent(ctx, sqlcgen.InsertEventParams{
			EventID:       event.ID,
			AggregateID:   event.AggregateID,
			AggregateType: event.AggregateType,
			EventType:     event.EventType,
			Version:       event.Version,
			Timestamp:     event.Timestamp.Unix(),
			Data:          event.Data,
			Metadata:      string(metadataJSON),
			Constraints:   sql.NullString{String: string(constraintsJSON), Valid: len(constraintsJSON) > 0},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to insert event: %w", err)
		}
		eventIDs[i] = event.ID
	}

	// Update global position
	if err := s.updatePositions(tx); err != nil {
		return nil, fmt.Errorf("failed to update positions: %w", err)
	}

	// Record processed command
	eventIDsJSON, _ := json.Marshal(eventIDs)
	now := time.Now()
	expiresAt := now.Add(ttl)

	err = queries.InsertProcessedCommand(ctx, sqlcgen.InsertProcessedCommandParams{
		CommandID:   commandID,
		AggregateID: aggregateID,
		ProcessedAt: now.Unix(),
		ExpiresAt:   expiresAt.Unix(),
		EventIds:    string(eventIDsJSON),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to record command: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return &eventsourcing.CommandResult{
		CommandID:        commandID,
		Events:           events,
		AlreadyProcessed: false,
		ProcessedAt:      now,
	}, nil
}

// validateConstraints validates and applies unique constraints.
func (s *EventStore) validateConstraints(tx *sql.Tx, event *eventsourcing.Event, aggregateID string) error {
	ctx := context.Background()
	queries := sqlcgen.New(tx)

	for _, constraint := range event.UniqueConstraints {
		switch constraint.Operation {
		case eventsourcing.ConstraintClaim:
			// Check if value already claimed
			ownerID, err := queries.GetConstraintOwner(ctx, sqlcgen.GetConstraintOwnerParams{
				IndexName: constraint.IndexName,
				Value:     constraint.Value,
			})

			if err == nil && ownerID != aggregateID {
				// Value already claimed by different aggregate
				return eventsourcing.NewUniqueConstraintError(constraint.IndexName, constraint.Value, ownerID)
			} else if err != nil && err != sql.ErrNoRows {
				return fmt.Errorf("failed to check uniqueness: %w", err)
			}

			// Claim the value
			err = queries.ClaimConstraint(ctx, sqlcgen.ClaimConstraintParams{
				IndexName:   constraint.IndexName,
				Value:       constraint.Value,
				AggregateID: aggregateID,
				CreatedAt:   time.Now().Unix(),
			})
			if err != nil {
				return fmt.Errorf("failed to claim constraint: %w", err)
			}

		case eventsourcing.ConstraintRelease:
			// Release the value
			err := queries.ReleaseConstraint(ctx, sqlcgen.ReleaseConstraintParams{
				IndexName:   constraint.IndexName,
				Value:       constraint.Value,
				AggregateID: aggregateID,
			})
			if err != nil {
				return fmt.Errorf("failed to release constraint: %w", err)
			}
		}
	}
	return nil
}

// updatePositions updates the global position for events.
func (s *EventStore) updatePositions(tx *sql.Tx) error {
	ctx := context.Background()
	queries := sqlcgen.New(tx)
	return queries.UpdateEventPositions(ctx)
}

// Continue in next file...
