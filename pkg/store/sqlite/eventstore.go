package sqlite

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/plaenen/eventstore/pkg/domain"
	"github.com/plaenen/eventstore/pkg/store/sqlite/sqlcgen"
	"github.com/plaenen/eventstore/pkg/validation"

	// _ "modernc.org/sqlite" // Pure Go SQLite driver
	//
	// LibSQL driver (github.com/tursodatabase/go-libsql) provides enhanced capabilities:
	//
	// Schema Evolution:
	//   - ✅ ALTER TABLE ADD COLUMN (standard SQLite)
	//   - ✅ ALTER TABLE RENAME COLUMN (LibSQL extension)
	//   - ✅ ALTER TABLE DROP COLUMN (LibSQL extension)
	//   - ✅ ALTER TABLE RENAME TO (standard SQLite)
	//
	// Deployment Modes:
	//   - ✅ Local files (traditional SQLite)
	//   - ✅ Remote databases (Turso cloud)
	//   - ✅ Embedded replicas (local-first with cloud sync)
	//
	// Performance & Features:
	//   - ✅ Randomized ROWID for better performance
	//   - ✅ Virtual write-ahead log interface
	//   - ✅ Native vector search
	//   - ✅ FTS5 full-text search
	//   - ✅ R*Tree spatial indexing
	//   - ✅ JSON functions
	//   - ✅ Encryption at rest
	//   - ✅ Crypto, Fuzzy, Math, Stats, Text, UUID extensions from SQLean
	//   - ✅ WebAssembly User Defined Functions
	_ "github.com/tursodatabase/go-libsql"
)

// EventStore is a SQLite-based implementation of domain.EventStore.
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

	// connector is an optional driver.Connector for advanced LibSQL features
	// When set, this takes precedence over dsn
	connector driver.Connector

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

// WithLibSQLConnector sets a custom driver.Connector for advanced LibSQL features.
// This allows full control over embedded replicas, encryption, and other LibSQL features.
//
// Example with embedded replica:
//
//	import libsql "github.com/tursodatabase/go-libsql"
//
//	connector := libsql.NewEmbeddedReplicaConnector(
//	    "local.db",
//	    "libsql://mydb.turso.io",
//	    libsql.WithAuthToken(os.Getenv("TURSO_AUTH_TOKEN")),
//	    libsql.WithSyncInterval(time.Minute),
//	)
//	store, err := sqlite.NewEventStore(sqlite.WithLibSQLConnector(connector))
func WithLibSQLConnector(connector driver.Connector) EventStoreOption {
	return func(c *eventStoreConfig) {
		c.connector = connector
	}
}

// WithLibSQLRemote configures the event store to connect to a remote LibSQL/Turso database.
// This is useful for cloud-hosted databases like Turso.
//
// Example:
//
//	store, err := sqlite.NewEventStore(
//	    sqlite.WithLibSQLRemote(
//	        "libsql://mydb.turso.io",
//	        os.Getenv("TURSO_AUTH_TOKEN"),
//	    ),
//	)
func WithLibSQLRemote(url, authToken string) EventStoreOption {
	return func(c *eventStoreConfig) {
		if authToken != "" {
			c.dsn = fmt.Sprintf("%s?authToken=%s", url, authToken)
		} else {
			c.dsn = url
		}
	}
}

// WithLibSQLEmbeddedReplica configures the event store with local-first embedded replica.
// This provides offline-first capabilities with automatic synchronization to a remote database.
//
// The embedded replica maintains a local copy of the database for fast reads/writes,
// and periodically syncs with the remote server.
//
// Example:
//
//	store, err := sqlite.NewEventStore(
//	    sqlite.WithLibSQLEmbeddedReplica(
//	        "local-events.db",
//	        "libsql://mydb.turso.io",
//	        os.Getenv("TURSO_AUTH_TOKEN"),
//	    ),
//	)
//
// Note: For advanced features like custom sync intervals, use WithLibSQLConnector instead.
func WithLibSQLEmbeddedReplica(localPath, remoteURL, authToken string) EventStoreOption {
	return func(c *eventStoreConfig) {
		// Build DSN with embedded replica parameters
		// Format: file:local.db?_sync_url=libsql://remote&_auth_token=xxx
		dsn := fmt.Sprintf("file:%s?_embedded_replica=1&_sync_url=%s", localPath, remoteURL)
		if authToken != "" {
			dsn = fmt.Sprintf("%s&_auth_token=%s", dsn, authToken)
		}
		c.dsn = dsn
	}
}

// NewEventStore creates a new LibSQL-powered event store with the given options.
//
// Supports three deployment modes:
//   1. Local file - Traditional SQLite file on disk
//   2. Remote - Cloud-hosted LibSQL/Turso database
//   3. Embedded Replica - Local-first with cloud sync
//
// Example usage:
//
//	// Local file (default)
//	store, err := sqlite.NewEventStore()
//
//	// In-memory database for testing
//	store, err := sqlite.NewEventStore(
//	    sqlite.WithMemoryDatabase(),
//	)
//
//	// Remote Turso database
//	store, err := sqlite.NewEventStore(
//	    sqlite.WithLibSQLRemote(
//	        "libsql://mydb.turso.io",
//	        os.Getenv("TURSO_AUTH_TOKEN"),
//	    ),
//	)
//
//	// Embedded replica (local-first with cloud sync)
//	store, err := sqlite.NewEventStore(
//	    sqlite.WithLibSQLEmbeddedReplica(
//	        "local-events.db",
//	        "libsql://mydb.turso.io",
//	        os.Getenv("TURSO_AUTH_TOKEN"),
//	    ),
//	)
//
//	// Advanced: Custom connector
//	connector := libsql.NewEmbeddedReplicaConnector(
//	    "local.db", "libsql://mydb.turso.io",
//	    libsql.WithAuthToken(token),
//	    libsql.WithSyncInterval(time.Minute),
//	)
//	store, err := sqlite.NewEventStore(
//	    sqlite.WithLibSQLConnector(connector),
//	)
func NewEventStore(opts ...EventStoreOption) (*EventStore, error) {
	// Start with defaults and apply options
	config := defaultEventStoreConfig()
	for _, opt := range opts {
		opt(&config)
	}

	var db *sql.DB
	var err error

	// Prefer connector over DSN if both are provided
	if config.connector != nil {
		db = sql.OpenDB(config.connector)
	} else {
		db, err = sql.Open("libsql", config.dsn)
		if err != nil {
			return nil, fmt.Errorf("failed to open database: %w", err)
		}
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
	// LibSQL requires executing PRAGMAs individually and may return results
	// Use Query() instead of Exec() for PRAGMAs that return values
	pragmas := []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA foreign_keys = ON",
	}

	for _, pragma := range pragmas {
		// Some PRAGMAs return rows, so use Query() then close
		rows, err := s.db.Query(pragma)
		if err != nil {
			return fmt.Errorf("failed to execute %s: %w", pragma, err)
		}
		rows.Close()
	}

	return nil
}

// AppendEvents appends events to an aggregate's stream atomically.
func (s *EventStore) AppendEvents(aggregateID string, expectedVersion int64, events []*domain.Event) error {
	if len(events) == 0 {
		return nil
	}

	// Validate inputs before any database operations (SCOPE: Infrastructure/Input Validation)
	if err := validateAppendEventsInput(aggregateID, events); err != nil {
		return fmt.Errorf("input validation failed: %w", err)
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
		return domain.ErrConcurrencyConflict
	}

	// Get next position atomically
	var maxPos int64
	err = tx.QueryRow("SELECT COALESCE(MAX(position), 0) FROM events").Scan(&maxPos)
	if err != nil {
		return fmt.Errorf("failed to get max position: %w", err)
	}

	// Assign positions to events before insertion
	nextPos := maxPos + 1
	for i := range events {
		events[i].Position = nextPos + int64(i)
	}

	// Validate and insert unique constraints
	for _, event := range events {
		if err := s.validateConstraints(tx, event, aggregateID); err != nil {
			return err
		}
	}

	// Insert events with pre-assigned positions
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
			Position:      sql.NullInt64{Int64: event.Position, Valid: true}, // Position assigned atomically above
		})
		if err != nil {
			return fmt.Errorf("failed to insert event: %w", err)
		}

		// Insert into outbox for publishing (transactional outbox pattern)
		if err := s.insertOutbox(tx, event); err != nil {
			return fmt.Errorf("failed to insert into outbox: %w", err)
		}
	}

	// Position assignment is now atomic - no need for updatePositions()

	return tx.Commit()
}

// AppendEventsIdempotent appends events with command-level idempotency.
func (s *EventStore) AppendEventsIdempotent(
	aggregateID string,
	expectedVersion int64,
	events []*domain.Event,
	commandID string,
	ttl time.Duration,
) (*domain.CommandResult, error) {
	if commandID == "" {
		return nil, domain.ErrInvalidCommand
	}

	if len(events) == 0 {
		return &domain.CommandResult{
			CommandID: commandID,
			Events:    nil,
		}, nil
	}

	// Validate inputs before any database operations (SCOPE: Infrastructure/Input Validation)
	if err := validateAppendEventsInput(aggregateID, events); err != nil {
		return nil, fmt.Errorf("input validation failed: %w", err)
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
		return nil, domain.ErrConcurrencyConflict
	}

	// Get next position atomically
	var maxPos int64
	err = tx.QueryRow("SELECT COALESCE(MAX(position), 0) FROM events").Scan(&maxPos)
	if err != nil {
		return nil, fmt.Errorf("failed to get max position: %w", err)
	}

	// Assign positions to events before insertion
	nextPos := maxPos + 1
	for i := range events {
		events[i].Position = nextPos + int64(i)
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
			Position:      sql.NullInt64{Int64: event.Position, Valid: true}, // Position assigned atomically above
		})
		if err != nil {
			return nil, fmt.Errorf("failed to insert event: %w", err)
		}
		eventIDs[i] = event.ID
	}

	// Position assignment is now atomic - no need for updatePositions()

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

	return &domain.CommandResult{
		CommandID:        commandID,
		Events:           events,
		AlreadyProcessed: false,
		ProcessedAt:      now,
	}, nil
}

// validateConstraints validates and applies unique constraints.
func (s *EventStore) validateConstraints(tx *sql.Tx, event *domain.Event, aggregateID string) error {
	ctx := context.Background()
	queries := sqlcgen.New(tx)

	for _, constraint := range event.UniqueConstraints {
		switch constraint.Operation {
		case domain.ConstraintClaim:
			// Check if value already claimed
			ownerID, err := queries.GetConstraintOwner(ctx, sqlcgen.GetConstraintOwnerParams{
				IndexName: constraint.IndexName,
				Value:     constraint.Value,
			})

			if err == nil && ownerID != aggregateID {
				// Value already claimed by different aggregate
				return domain.NewUniqueConstraintError(constraint.IndexName, constraint.Value, ownerID)
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

		case domain.ConstraintRelease:
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

// validateAppendEventsInput validates event data before database operations.
// This provides defense-in-depth validation at the infrastructure layer.
//
// SCOPE: Infrastructure/Input Validation Layer
// This ensures data integrity before any database operations are performed.
//
// Validates:
//   - aggregate_id: must not be empty (aggregate scope)
//   - For each event:
//   - event_id: must not be empty (global scope)
//   - aggregate_id: must match the parameter and not be empty (aggregate scope)
//   - aggregate_type: must not be empty (domain scope)
//   - event_type: must not be empty (domain scope)
//   - For each unique constraint:
//   - index_name: must not be empty (domain scope)
//   - value: must not be empty (domain/aggregate scope)
func validateAppendEventsInput(aggregateID string, events []*domain.Event) error {
	// Validate aggregate ID (aggregate scope)
	if err := validation.ValidateStringNotEmpty(aggregateID, "aggregate_id"); err != nil {
		return err
	}

	// Validate each event
	for i, event := range events {
		if event == nil {
			return fmt.Errorf("event at index %d is nil", i)
		}

		// Validate event ID (global scope)
		if err := validation.ValidateStringNotEmpty(event.ID, "event_id"); err != nil {
			return fmt.Errorf("event[%d]: %w", i, err)
		}

		// Validate aggregate ID matches (aggregate scope)
		if err := validation.ValidateStringNotEmpty(event.AggregateID, "aggregate_id"); err != nil {
			return fmt.Errorf("event[%d]: %w", i, err)
		}
		if event.AggregateID != aggregateID {
			return fmt.Errorf("event[%d]: aggregate_id mismatch: event has %q, expected %q", i, event.AggregateID, aggregateID)
		}

		// Validate aggregate type (domain scope)
		if strings.TrimSpace(event.AggregateType) == "" {
			return fmt.Errorf("event[%d]: aggregate_type cannot be empty", i)
		}

		// Validate event type (domain scope)
		if strings.TrimSpace(event.EventType) == "" {
			return fmt.Errorf("event[%d]: event_type cannot be empty", i)
		}

		// Validate version (aggregate scope)
		if err := validation.ValidateVersion(event.Version); err != nil {
			return fmt.Errorf("event[%d]: %w", i, err)
		}

		// Validate unique constraints (domain scope)
		for j, constraint := range event.UniqueConstraints {
			if err := validation.ValidateStringNotEmpty(constraint.IndexName, "constraint.index_name"); err != nil {
				return fmt.Errorf("event[%d].constraint[%d]: %w", i, j, err)
			}
			if err := validation.ValidateStringNotEmpty(constraint.Value, "constraint.value"); err != nil {
				return fmt.Errorf("event[%d].constraint[%d]: %w", i, j, err)
			}
		}
	}

	return nil
}

// SeedEvents appends events with special semantics for migrations and bootstrapping.
// See domain.SeedOptions for configuration details.
func (s *EventStore) SeedEvents(
	aggregateID string,
	expectedVersion int64,
	events []*domain.Event,
	opts *domain.SeedOptions,
) (*domain.SeedResult, error) {
	if len(events) == 0 {
		return &domain.SeedResult{}, nil
	}

	// Use defaults if opts not provided
	if opts == nil {
		opts = domain.DefaultSeedOptions()
	}

	result := &domain.SeedResult{
		EventIDs: make([]string, 0, len(events)),
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Step 1: Prepare events (generate IDs, augment metadata)
	preparedEvents := make([]*domain.Event, len(events))
	for i, event := range events {
		// Create a copy to avoid modifying original
		preparedEvent := *event

		// Generate deterministic ID if missing
		if preparedEvent.ID == "" && opts.GenerateDeterministicIDs {
			preparedEvent.ID = domain.GenerateDeterministicSeedID(&preparedEvent)
		}

		// Augment with seed metadata
		domain.AugmentSeedMetadata(&preparedEvent, opts)

		preparedEvents[i] = &preparedEvent
		result.EventIDs = append(result.EventIDs, preparedEvent.ID)
	}

	// Step 2: Validate inputs (after ID generation)
	if err := validateAppendEventsInput(aggregateID, preparedEvents); err != nil {
		return nil, fmt.Errorf("input validation failed: %w", err)
	}

	// Step 2: Check which events already exist (for idempotency)
	existingEvents := make(map[string]bool)
	if opts.SkipExisting {
		existing, err := s.checkExistingEventIDs(preparedEvents)
		if err != nil {
			return nil, fmt.Errorf("failed to check existing events: %w", err)
		}
		existingEvents = existing
	}

	// Step 3: Filter out existing events
	eventsToSave := make([]*domain.Event, 0, len(preparedEvents))
	for _, event := range preparedEvents {
		if existingEvents[event.ID] {
			result.Skipped++
		} else {
			eventsToSave = append(eventsToSave, event)
		}
	}

	// If all events already exist, return early
	if len(eventsToSave) == 0 {
		return result, nil
	}

	// Step 4: Begin transaction and save events
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	ctx := context.Background()
	queries := sqlcgen.New(tx)

	// Check version if required
	if !opts.SkipVersionCheck {
		currentVersionRaw, err := queries.GetAggregateVersion(ctx, aggregateID)
		if err != nil {
			return nil, fmt.Errorf("failed to check current version: %w", err)
		}
		currentVersion := currentVersionRaw.(int64)

		if currentVersion != expectedVersion {
			return nil, domain.ErrConcurrencyConflict
		}
	}

	// Get next position atomically
	var maxPos int64
	err = tx.QueryRow("SELECT COALESCE(MAX(position), 0) FROM events").Scan(&maxPos)
	if err != nil {
		return nil, fmt.Errorf("failed to get max position: %w", err)
	}

	// Assign positions to events before insertion
	nextPos := maxPos + 1
	for i := range eventsToSave {
		eventsToSave[i].Position = nextPos + int64(i)
	}

	// Process each event
	for _, event := range eventsToSave {
		// Handle constraints
		if err := s.validateSeedConstraints(tx, event, aggregateID, opts); err != nil {
			result.Failed++
			result.Errors = append(result.Errors, domain.SeedError{
				EventID:   event.ID,
				EventType: event.EventType,
				Version:   event.Version,
				Reason:    "constraint validation failed",
				Error:     err,
			})
			continue
		}

		// Insert event
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
			Position:      sql.NullInt64{Int64: event.Position, Valid: true}, // Position assigned atomically above
		})
		if err != nil {
			result.Failed++
			result.Errors = append(result.Errors, domain.SeedError{
				EventID:   event.ID,
				EventType: event.EventType,
				Version:   event.Version,
				Reason:    "database insert failed",
				Error:     err,
			})
			continue
		}

		result.Saved++
	}

	// Position assignment is now atomic - no need for updatePositions()

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return result, nil
}

// checkExistingEventIDs checks which event IDs already exist in the database.
func (s *EventStore) checkExistingEventIDs(events []*domain.Event) (map[string]bool, error) {
	if len(events) == 0 {
		return make(map[string]bool), nil
	}

	// Build list of event IDs to check
	eventIDs := make([]string, len(events))
	for i, event := range events {
		eventIDs[i] = event.ID
	}

	// Query database for existing IDs
	// Build IN clause: SELECT event_id FROM events WHERE event_id IN (?,?,...)
	placeholders := make([]string, len(eventIDs))
	args := make([]any, len(eventIDs))
	for i, id := range eventIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf("SELECT event_id FROM events WHERE event_id IN (%s)",
		strings.Join(placeholders, ","))

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query existing events: %w", err)
	}
	defer rows.Close()

	// Build map of existing IDs
	existing := make(map[string]bool)
	for rows.Next() {
		var eventID string
		if err := rows.Scan(&eventID); err != nil {
			return nil, fmt.Errorf("failed to scan event ID: %w", err)
		}
		existing[eventID] = true
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return existing, nil
}

// validateSeedConstraints validates constraints with seed-specific logic.
func (s *EventStore) validateSeedConstraints(tx *sql.Tx, event *domain.Event, aggregateID string, opts *domain.SeedOptions) error {
	// Skip constraint checking if disabled
	if !opts.CheckConstraintOwnership && len(event.UniqueConstraints) > 0 {
		return nil
	}

	ctx := context.Background()
	queries := sqlcgen.New(tx)

	for _, constraint := range event.UniqueConstraints {
		switch constraint.Operation {
		case domain.ConstraintClaim:
			// Check if value already claimed
			ownerID, err := queries.GetConstraintOwner(ctx, sqlcgen.GetConstraintOwnerParams{
				IndexName: constraint.IndexName,
				Value:     constraint.Value,
			})

			if err == nil {
				// Value is claimed
				if ownerID != aggregateID {
					// Claimed by different aggregate - this is an error
					return domain.NewUniqueConstraintError(constraint.IndexName, constraint.Value, ownerID)
				}
				// Claimed by same aggregate - skip (idempotent)
				continue
			} else if err != sql.ErrNoRows {
				return fmt.Errorf("failed to check uniqueness: %w", err)
			}

			// Value not claimed - claim it
			err = queries.ClaimConstraint(ctx, sqlcgen.ClaimConstraintParams{
				IndexName:   constraint.IndexName,
				Value:       constraint.Value,
				AggregateID: aggregateID,
				CreatedAt:   time.Now().Unix(),
			})
			if err != nil {
				return fmt.Errorf("failed to claim constraint: %w", err)
			}

		case domain.ConstraintRelease:
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

// ============================================================================
// Outbox Methods - Transactional Outbox Pattern
// ============================================================================

// insertOutbox inserts an event into the outbox table within an existing transaction.
// This is called by AppendEvents to ensure events are atomically added to both
// the events table and the outbox table.
func (s *EventStore) insertOutbox(tx *sql.Tx, event *domain.Event) error {
	_, err := tx.Exec(`
		INSERT INTO event_outbox (
			event_id, aggregate_id, aggregate_type, event_type, version, created_at
		) VALUES (?, ?, ?, ?, ?, ?)
	`, event.ID, event.AggregateID, event.AggregateType, event.EventType, event.Version, time.Now().Unix())

	return err
}

// LoadUnpublishedEvents loads events from the outbox that haven't been published yet.
// Returns events ordered by created_at (oldest first) up to the specified limit.
// This is used by the OutboxForwarder to poll for events that need publishing.
func (s *EventStore) LoadUnpublishedEvents(limit int) ([]*domain.EventEnvelope, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT
			o.event_id,
			e.aggregate_id,
			e.aggregate_type,
			e.event_type,
			e.version,
			e.timestamp,
			e.data,
			e.metadata
		FROM event_outbox o
		INNER JOIN events e ON o.event_id = e.event_id
		WHERE o.published_at IS NULL
		ORDER BY o.created_at ASC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query unpublished events: %w", err)
	}
	defer rows.Close()

	var envelopes []*domain.EventEnvelope
	for rows.Next() {
		var (
			eventID       string
			aggregateID   string
			aggregateType string
			eventType     string
			version       int64
			timestamp     int64
			data          []byte
			metadataJSON  string
		)

		if err := rows.Scan(
			&eventID, &aggregateID, &aggregateType, &eventType,
			&version, &timestamp, &data, &metadataJSON,
		); err != nil {
			return nil, fmt.Errorf("failed to scan event: %w", err)
		}

		event := domain.Event{
			ID:            eventID,
			AggregateID:   aggregateID,
			AggregateType: aggregateType,
			EventType:     eventType,
			Version:       version,
			Timestamp:     time.Unix(timestamp, 0),
			Data:          data,
		}

		// Unmarshal metadata directly into the Event struct
		if err := json.Unmarshal([]byte(metadataJSON), &event.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}

		envelope := &domain.EventEnvelope{
			Event: event,
		}

		envelopes = append(envelopes, envelope)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return envelopes, nil
}

// MarkEventsPublished marks events as successfully published in the outbox.
// This should be called after events are successfully published to the message bus.
func (s *EventStore) MarkEventsPublished(eventIDs []string) error {
	if len(eventIDs) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Build placeholders for IN clause
	placeholders := make([]string, len(eventIDs))
	args := make([]interface{}, len(eventIDs)+1)
	args[0] = time.Now().Unix()

	for i, id := range eventIDs {
		placeholders[i] = "?"
		args[i+1] = id
	}

	query := fmt.Sprintf(`
		UPDATE event_outbox
		SET published_at = ?, attempts = attempts + 1
		WHERE event_id IN (%s)
	`, strings.Join(placeholders, ","))

	_, err := s.db.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("failed to mark events as published: %w", err)
	}

	return nil
}

// RecordPublishFailure records a failed publish attempt for an event.
// Increments the attempts counter and stores the error message for debugging.
func (s *EventStore) RecordPublishFailure(eventID string, err error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, execErr := s.db.Exec(`
		UPDATE event_outbox
		SET attempts = attempts + 1,
		    last_error = ?
		WHERE event_id = ?
	`, err.Error(), eventID)

	if execErr != nil {
		return fmt.Errorf("failed to record publish failure: %w", execErr)
	}

	return nil
}

// Continue in next file...
