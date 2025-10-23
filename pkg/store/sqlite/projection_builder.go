package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"strings"

	"github.com/plaenen/eventstore/pkg/domain"
	"github.com/plaenen/eventstore/pkg/store"
	"github.com/plaenen/eventstore/pkg/store/sqlite/migrate"
)

// TransactionalEventHandler is a handler that receives a transaction to work with.
// The transaction, checkpoint update, and commit are handled automatically.
type TransactionalEventHandler func(ctx context.Context, tx *sql.Tx, envelope *domain.EventEnvelope) error

// SQLiteProjectionBuilder provides a high-level builder for SQLite projections
// with automatic transaction handling, checkpoint management, and rebuild support.
type SQLiteProjectionBuilder struct {
	name            string
	db              *sql.DB
	checkpointStore *CheckpointStore
	statusStore     *ProjectionStatusStore
	eventStore      store.EventStore
	handlers        map[string]TransactionalEventHandler
	resetFunc       func(context.Context, *sql.Tx) error
	schemaFunc      func(context.Context, *sql.DB) error
	migrationsFS    fs.FS
	migrationsPath  string
}

// NewSQLiteProjectionBuilder creates a new SQLite-specific projection builder.
//
// This builder provides:
//   - Automatic transaction management
//   - Automatic checkpoint updates (atomic with projection updates)
//   - Built-in rebuild functionality
//   - Schema initialization support
//
// Example:
//
//	projection := sqlite.NewSQLiteProjectionBuilder("account-balance", db, checkpointStore, eventStore).
//	    WithSchema(func(ctx context.Context, db *sql.DB) error {
//	        _, err := db.Exec("CREATE TABLE IF NOT EXISTS accounts (...)")
//	        return err
//	    }).
//	    On(accountv1.OnAccountOpened(func(ctx context.Context, event *accountv1.AccountOpenedEvent, envelope *domain.EventEnvelope) error {
//	        // Just return the SQL - transaction handling is automatic!
//	        return tx.Exec("INSERT INTO accounts (...) VALUES (...)")
//	    })).
//	    Build()
func NewSQLiteProjectionBuilder(
	name string,
	db *sql.DB,
	checkpointStore *CheckpointStore,
	eventStore store.EventStore,
) *SQLiteProjectionBuilder {
	// Create status store
	statusStore, _ := NewProjectionStatusStore(db)

	return &SQLiteProjectionBuilder{
		name:            name,
		db:              db,
		checkpointStore: checkpointStore,
		statusStore:     statusStore,
		eventStore:      eventStore,
		handlers:        make(map[string]TransactionalEventHandler),
	}
}

// WithSchema registers a function to initialize the projection schema.
// This is called during Build() to ensure tables exist.
// Deprecated: Use WithMigrations for version-controlled schema evolution.
func (b *SQLiteProjectionBuilder) WithSchema(schemaFunc func(context.Context, *sql.DB) error) *SQLiteProjectionBuilder {
	b.schemaFunc = schemaFunc
	return b
}

// WithMigrations registers a migrations directory for schema evolution.
// The migrations are run automatically during Build() using the same migration
// system as the event store.
//
// Supports embedded file systems for compiled-in migrations.
//
// Example with embedded migrations:
//
//	//go:embed migrations/*.sql
//	var migrationsFS embed.FS
//
//	projection := sqlite.NewSQLiteProjectionBuilder("account-balance", db, checkpointStore, eventStore).
//	    WithMigrations(migrationsFS, "migrations").
//	    On(accountv1.OnAccountOpened(...)).
//	    Build()
//
// Migration files should follow the naming convention:
//   - 001_initial_schema.sql
//   - 002_add_index.sql
//   - 003_add_column.sql
func (b *SQLiteProjectionBuilder) WithMigrations(migrationsFS fs.FS, path string) *SQLiteProjectionBuilder {
	b.migrationsFS = migrationsFS
	b.migrationsPath = path
	return b
}

// On registers an event handler registration with automatic transaction handling.
//
// The handler can access the transaction via sqlite.TxFromContext(ctx).
// Transaction begin, checkpoint update, and commit are handled automatically.
//
// Example:
//
//	builder.On(accountv1.OnAccountOpened(func(ctx, event, envelope) error {
//	    tx, _ := sqlite.TxFromContext(ctx)
//	    _, err := tx.Exec("INSERT INTO accounts VALUES (?, ?)", event.AccountId, event.Balance)
//	    return err
//	}))
func (b *SQLiteProjectionBuilder) On(registration store.EventHandlerRegistration) *SQLiteProjectionBuilder {
	// Wrap the handler to inject transaction
	b.handlers[registration.EventType] = func(ctx context.Context, tx *sql.Tx, envelope *domain.EventEnvelope) error {
		// Create a context that carries the transaction
		txCtx := context.WithValue(ctx, txContextKey{}, tx)

		// Call the original handler with the transaction context
		return registration.Handler(txCtx, envelope)
	}
	return b
}

// OnWithTx registers a handler that directly receives the transaction.
// This is useful when you need more control over the transaction.
func (b *SQLiteProjectionBuilder) OnWithTx(eventType string, handler TransactionalEventHandler) *SQLiteProjectionBuilder {
	b.handlers[eventType] = handler
	return b
}

// OnReset registers a function to reset the projection state.
// The function receives a transaction to perform the reset.
func (b *SQLiteProjectionBuilder) OnReset(resetFunc func(context.Context, *sql.Tx) error) *SQLiteProjectionBuilder {
	b.resetFunc = resetFunc
	return b
}

// Build creates the final Projection implementation with full SQLite integration.
func (b *SQLiteProjectionBuilder) Build() (store.Projection, error) {
	// Run migrations if provided (preferred approach)
	if b.migrationsFS != nil {
		if err := runProjectionMigrations(b.db, b.migrationsFS, b.migrationsPath, b.name); err != nil {
			return nil, fmt.Errorf("failed to run migrations: %w", err)
		}
	}

	// Initialize schema if provided (deprecated, but still supported)
	if b.schemaFunc != nil {
		if err := b.schemaFunc(context.Background(), b.db); err != nil {
			return nil, fmt.Errorf("failed to initialize schema: %w", err)
		}
	}

	projection := &SQLiteProjection{
		name:            b.name,
		db:              b.db,
		checkpointStore: b.checkpointStore,
		statusStore:     b.statusStore,
		eventStore:      b.eventStore,
		handlers:        b.handlers,
		resetFunc:       b.resetFunc,
	}

	// Set initial status to READY
	_ = b.statusStore.Save(&store.ProjectionState{
		ProjectionName: b.name,
		Status:         store.ProjectionStatusReady,
		UpdatedAt:      domain.Now(),
	})

	return projection, nil
}

// SQLiteProjection implements store.Projection with SQLite-specific features.
type SQLiteProjection struct {
	name            string
	db              *sql.DB
	checkpointStore *CheckpointStore
	statusStore     *ProjectionStatusStore
	eventStore      store.EventStore
	handlers        map[string]TransactionalEventHandler
	resetFunc       func(context.Context, *sql.Tx) error
}

// Name returns the projection name.
func (p *SQLiteProjection) Name() string {
	return p.name
}

// Handle processes an event with automatic transaction and checkpoint management.
func (p *SQLiteProjection) Handle(ctx context.Context, envelope *domain.EventEnvelope) error {
	handler, exists := p.handlers[envelope.EventType]
	if !exists {
		// No handler registered for this event type - skip it
		return nil
	}

	// Begin transaction
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() // Rollback if we don't commit

	// Call handler with transaction
	if err := handler(ctx, tx, envelope); err != nil {
		return fmt.Errorf("handler failed: %w", err)
	}

	// Update checkpoint in same transaction (atomic!)
	checkpoint := &store.ProjectionCheckpoint{
		ProjectionName: p.name,
		Position:       envelope.Version,
		LastEventID:    envelope.ID,
		UpdatedAt:      domain.Now(),
	}
	if err := p.checkpointStore.SaveInTx(tx, checkpoint); err != nil {
		return fmt.Errorf("failed to save checkpoint: %w", err)
	}

	// Commit transaction (both projection update and checkpoint atomically)
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// Reset resets the projection state.
func (p *SQLiteProjection) Reset(ctx context.Context) error {
	if p.resetFunc == nil {
		return nil // No reset function registered
	}

	// Reset in a transaction
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	if err := p.resetFunc(ctx, tx); err != nil {
		return fmt.Errorf("reset failed: %w", err)
	}

	// Delete checkpoint
	if err := p.checkpointStore.DeleteInTx(tx, p.name); err != nil {
		return fmt.Errorf("failed to delete checkpoint: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit reset: %w", err)
	}

	return nil
}

// Rebuild rebuilds the projection from the event store with status tracking.
func (p *SQLiteProjection) Rebuild(ctx context.Context) error {
	// Set status to REBUILDING
	rebuildState := &store.ProjectionState{
		ProjectionName: p.name,
		Status:         store.ProjectionStatusRebuilding,
		Message:        "Starting rebuild from event store",
		UpdatedAt:      domain.Now(),
		Progress: &store.RebuildProgress{
			EventsProcessed: 0,
			TotalEvents:     0, // Unknown
			StartedAt:       domain.Now(),
		},
	}
	if err := p.statusStore.Save(rebuildState); err != nil {
		return fmt.Errorf("failed to save rebuilding status: %w", err)
	}

	// Reset projection state
	if err := p.Reset(ctx); err != nil {
		// Set status to FAILED
		_ = p.statusStore.Save(&store.ProjectionState{
			ProjectionName: p.name,
			Status:         store.ProjectionStatusFailed,
			Message:        fmt.Sprintf("Reset failed: %v", err),
			UpdatedAt:      domain.Now(),
		})
		return fmt.Errorf("failed to reset projection: %w", err)
	}

	// Replay all events from EventStore
	position := int64(0)
	batchSize := 1000
	eventsProcessed := int64(0)

	for {
		events, err := p.eventStore.LoadAllEvents(position, batchSize)
		if err != nil {
			// Set status to FAILED
			_ = p.statusStore.Save(&store.ProjectionState{
				ProjectionName: p.name,
				Status:         store.ProjectionStatusFailed,
				Message:        fmt.Sprintf("Failed to load events: %v", err),
				UpdatedAt:      domain.Now(),
			})
			return fmt.Errorf("failed to load events: %w", err)
		}

		if len(events) == 0 {
			break
		}

		for _, event := range events {
			envelope := &domain.EventEnvelope{Event: *event}
			if err := p.Handle(ctx, envelope); err != nil {
				// Set status to FAILED
				_ = p.statusStore.Save(&store.ProjectionState{
					ProjectionName: p.name,
					Status:         store.ProjectionStatusFailed,
					Message:        fmt.Sprintf("Failed to handle event: %v", err),
					UpdatedAt:      domain.Now(),
				})
				return fmt.Errorf("failed to handle event during rebuild: %w", err)
			}
			position++
			eventsProcessed++

			// Update progress every 100 events
			if eventsProcessed%100 == 0 {
				_ = p.statusStore.UpdateProgress(p.name, &store.RebuildProgress{
					EventsProcessed: eventsProcessed,
					TotalEvents:     0, // Unknown
					StartedAt:       rebuildState.Progress.StartedAt,
				})
			}
		}

		if len(events) < batchSize {
			break
		}
	}

	// Set status to READY
	_ = p.statusStore.Save(&store.ProjectionState{
		ProjectionName: p.name,
		Status:         store.ProjectionStatusReady,
		Message:        fmt.Sprintf("Rebuild complete - processed %d events", eventsProcessed),
		UpdatedAt:      domain.Now(),
	})

	return nil
}

// GetCheckpoint returns the current checkpoint position.
func (p *SQLiteProjection) GetCheckpoint(ctx context.Context) (*store.ProjectionCheckpoint, error) {
	return p.checkpointStore.Load(p.name)
}

// GetStatus returns the current projection status.
func (p *SQLiteProjection) GetStatus(ctx context.Context) (*store.ProjectionState, error) {
	return p.statusStore.Load(p.name)
}

// IsReady returns true if the projection is ready to serve queries.
func (p *SQLiteProjection) IsReady(ctx context.Context) bool {
	status, err := p.GetStatus(ctx)
	if err != nil {
		return false
	}
	return status.Status == store.ProjectionStatusReady
}

// txContextKey is used to pass the transaction through context
type txContextKey struct{}

// TxFromContext extracts the transaction from the context.
// This allows handlers to access the transaction when needed.
func TxFromContext(ctx context.Context) (*sql.Tx, bool) {
	tx, ok := ctx.Value(txContextKey{}).(*sql.Tx)
	return tx, ok
}

// runProjectionMigrations runs migrations for a projection using the same
// migration system as the event store.
//
// Projection migrations are tracked separately using a table named:
// projection_{projectionName}_schema_migrations
func runProjectionMigrations(db *sql.DB, migrationsFS fs.FS, path string, projectionName string) error {
	// Use the existing migration runner with a custom table name
	// This ensures projection migrations are tracked separately from event store migrations
	// Sanitize projection name for use in table name (replace hyphens with underscores)
	sanitizedName := sanitizeTableName(projectionName)
	tableName := fmt.Sprintf("projection_%s_schema_migrations", sanitizedName)

	// Create migrator
	migrator := migrate.New(db, tableName)

	// Load migrations from FS
	// We need to convert fs.FS to embed.FS for the migrator
	embedFS, ok := migrationsFS.(embed.FS)
	if !ok {
		return fmt.Errorf("migrationsFS must be an embed.FS")
	}

	if err := migrator.LoadFromFS(embedFS, path); err != nil {
		return fmt.Errorf("failed to load migrations: %w", err)
	}

	// Run migrations
	if err := migrator.Up(); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	return nil
}

// sanitizeTableName replaces characters that are invalid in SQLite table names.
// Specifically, replaces hyphens with underscores.
func sanitizeTableName(name string) string {
	return strings.ReplaceAll(name, "-", "_")
}
