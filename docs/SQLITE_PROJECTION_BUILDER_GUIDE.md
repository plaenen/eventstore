# SQLiteProjectionBuilder User Guide

A comprehensive guide to building SQLite projections with automatic transaction management, checkpoint tracking, and schema migrations.

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Understanding Migrations](#understanding-migrations)
- [Building Your First Projection](#building-your-first-projection)
- [Migration Management](#migration-management)
- [Advanced Features](#advanced-features)
- [Best Practices](#best-practices)
- [Troubleshooting](#troubleshooting)

## Overview

The `SQLiteProjectionBuilder` is a high-level builder for creating SQLite-based event projections with:

- ✅ **Automatic transaction management** - No manual tx.Begin()/Commit()
- ✅ **Atomic checkpoint updates** - Checkpoint saved with projection data in same transaction
- ✅ **Built-in rebuild functionality** - Replay events from EventStore
- ✅ **Status tracking** - Monitor projection health (READY/REBUILDING/FAILED/PAUSED)
- ✅ **Metadata management** - Track schema versions, rebuild flags, custom config
- ✅ **Schema migrations** - Version-controlled schema evolution

## Quick Start

```go
import (
    "context"
    "database/sql"

    "github.com/plaenen/eventstore/pkg/store/sqlite"
    "github.com/plaenen/eventstore/pkg/domain"
    userv1 "your/app/proto/user/v1"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Create projection
projection := sqlite.NewSQLiteProjectionBuilder(
    "user-projection",          // Unique projection name
    db,                          // *sql.DB
    checkpointStore,             // Checkpoint store
    eventStore,                  // Event store for rebuilds
).
    WithMigrations(migrationsFS, "migrations").  // Schema migrations
    On(userv1.OnUserCreated(handleUserCreated)). // Event handlers
    On(userv1.OnUserUpdated(handleUserUpdated)).
    OnReset(resetProjection).                    // Reset handler
    Build()

// Event handler with automatic transaction management
func handleUserCreated(ctx context.Context, event *userv1.UserCreatedEvent,
    envelope *domain.EventEnvelope) error {

    // Get transaction from context (automatically managed)
    tx, _ := sqlite.TxFromContext(ctx)

    // Just write SQL - transaction is handled automatically!
    _, err := tx.Exec(`
        INSERT INTO users (id, email, name, created_at)
        VALUES (?, ?, ?, ?)
    `, event.UserId, event.Email, event.Name, event.CreatedAt)

    return err  // Rollback on error, commit on success
}
```

## Understanding Migrations

The `SQLiteProjectionBuilder` manages **two distinct types of migrations**:

### 1. Infrastructure Migrations (Automatic)

These are **managed automatically** by the eventstore package and track the projection management infrastructure:

```
Internal Tables Created Automatically:
├── projection_checkpoints           (tracks event positions)
├── projection_status               (tracks health status)
├── projection_metadata             (tracks schema versions, config)
│
Migration Tracking Tables:
├── checkpoint_schema_migrations    (tracks checkpoint table schema version)
├── metadata_schema_migrations      (tracks metadata table schema version)
└── projection_status_schema_migrations (tracks status table schema version)
```

**You don't need to do anything** - these migrations run automatically when you create a `CheckpointStore`, `ProjectionMetadataStore`, or `ProjectionStatusStore`.

### 2. Projection-Specific Migrations (User-Provided)

These are **your custom migrations** for the projection's read model schema:

```
Your Project:
├── projections/
│   └── user_projection/
│       ├── migrations/
│       │   ├── 001_initial_schema.sql      // Your migration
│       │   ├── 002_add_user_index.sql      // Your migration
│       │   └── 003_add_user_status.sql     // Your migration
│       └── projection.go
```

**Migration Tracking:**
Each projection gets its own tracking table:
- `user-projection` → `projection_user_projection_schema_migrations`
- `analytics-projection` → `projection_analytics_projection_schema_migrations`
- etc.

This means **each projection can have different schema versions** and evolve independently!

## Building Your First Projection

### Step 1: Create Migration Files

Create a `migrations/` directory with your schema migrations:

**migrations/001_initial_schema.sql:**
```sql
-- Create users table for user-projection
CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    email TEXT NOT NULL,
    name TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
CREATE INDEX IF NOT EXISTS idx_users_created_at ON users(created_at);
```

**migrations/002_add_user_status.sql:**
```sql
-- Add status column to users
ALTER TABLE users ADD COLUMN status TEXT DEFAULT 'active';

CREATE INDEX IF NOT EXISTS idx_users_status ON users(status);
```

### Step 2: Embed Migrations

```go
package projections

import (
    _ "embed"
)

//go:embed migrations/*.sql
var userProjectionMigrations embed.FS
```

### Step 3: Create Event Handlers

```go
func handleUserCreated(ctx context.Context, event *userv1.UserCreatedEvent,
    envelope *domain.EventEnvelope) error {

    tx, _ := sqlite.TxFromContext(ctx)

    _, err := tx.Exec(`
        INSERT INTO users (id, email, name, created_at, updated_at, status)
        VALUES (?, ?, ?, ?, ?, ?)
    `, event.UserId, event.Email, event.Name,
       envelope.Timestamp.Unix(), envelope.Timestamp.Unix(), "active")

    return err
}

func handleUserUpdated(ctx context.Context, event *userv1.UserUpdatedEvent,
    envelope *domain.EventEnvelope) error {

    tx, _ := sqlite.TxFromContext(ctx)

    _, err := tx.Exec(`
        UPDATE users
        SET email = ?, name = ?, updated_at = ?
        WHERE id = ?
    `, event.Email, event.Name, envelope.Timestamp.Unix(), event.UserId)

    return err
}

func handleUserDeleted(ctx context.Context, event *userv1.UserDeletedEvent,
    envelope *domain.EventEnvelope) error {

    tx, _ := sqlite.TxFromContext(ctx)

    _, err := tx.Exec(`
        UPDATE users
        SET status = 'deleted', updated_at = ?
        WHERE id = ?
    `, envelope.Timestamp.Unix(), event.UserId)

    return err
}
```

### Step 4: Create Reset Handler

The reset handler is called during projection rebuilds to clear existing data:

```go
func resetUserProjection(ctx context.Context, tx *sql.Tx) error {
    // Delete all data - schema remains intact
    _, err := tx.Exec(`DELETE FROM users`)
    return err
}
```

### Step 5: Build the Projection

```go
func NewUserProjection(db *sql.DB, checkpointStore *sqlite.CheckpointStore,
    eventStore store.EventStore) (store.Projection, error) {

    projection := sqlite.NewSQLiteProjectionBuilder(
        "user-projection",
        db,
        checkpointStore,
        eventStore,
    ).
        WithMigrations(userProjectionMigrations, "migrations").
        On(userv1.OnUserCreated(handleUserCreated)).
        On(userv1.OnUserUpdated(handleUserUpdated)).
        On(userv1.OnUserDeleted(handleUserDeleted)).
        OnReset(resetUserProjection).
        Build()

    return projection, nil
}
```

### Step 6: Use the Projection

```go
func main() {
    // Create database
    db, _ := sql.Open("sqlite", "app.db")

    // Create stores
    eventStore, _ := sqlite.NewEventStore(db)
    checkpointStore, _ := sqlite.NewCheckpointStore(db)
    eventBus, _ := nats.NewEventBus(...)

    // Create projection
    userProjection, _ := NewUserProjection(db, checkpointStore, eventStore)

    // Register with projection manager
    projectionManager := eventsourcing.NewProjectionManager(
        checkpointStore,
        eventStore,
        eventBus,
    )
    projectionManager.Register(userProjection)

    // Start projection (subscribes to event bus)
    projectionManager.Start(ctx, "user-projection")

    // Rebuild if needed
    // projectionManager.Rebuild(ctx, "user-projection")
}
```

## Migration Management

### How Migrations are Tracked

When you build a projection, here's what happens:

1. **Infrastructure tables are created** (if not exist):
   ```
   ✓ projection_checkpoints
   ✓ projection_status
   ✓ projection_metadata
   ✓ checkpoint_schema_migrations
   ✓ metadata_schema_migrations
   ✓ projection_status_schema_migrations
   ```

2. **Projection-specific migration tracker is created**:
   ```
   ✓ projection_user_projection_schema_migrations
   ```

3. **Your migrations are executed in order**:
   ```
   Running migration 001_initial_schema.sql...
   Running migration 002_add_user_status.sql...
   ```

4. **Migration tracker is updated**:
   ```sql
   SELECT * FROM projection_user_projection_schema_migrations;
   -- version: 2
   -- name: 002_add_user_status.sql
   ```

### Database State with Multiple Projections

Example with 3 projections in the same database:

```sql
-- Shared infrastructure tables (one per database)
projection_checkpoints
projection_status
projection_metadata

-- Shared infrastructure migration tracking (one per database)
checkpoint_schema_migrations         -- version: 2
metadata_schema_migrations           -- version: 1
projection_status_schema_migrations  -- version: 1

-- Projection-specific migration tracking (one per projection)
projection_user_projection_schema_migrations      -- version: 3
projection_analytics_projection_schema_migrations -- version: 1
projection_order_projection_schema_migrations     -- version: 5

-- Projection-specific tables (created by your migrations)
users                    -- Created by user-projection migration 001
user_sessions            -- Created by user-projection migration 002
analytics_events         -- Created by analytics-projection migration 001
orders                   -- Created by order-projection migration 001
order_items              -- Created by order-projection migration 002
```

### Migration Naming Convention

Follow this naming pattern for clarity:

```
001_initial_schema.sql
002_add_indexes.sql
003_add_user_preferences.sql
004_add_audit_columns.sql
```

**Rules:**
- ✅ Use sequential numbering (001, 002, 003...)
- ✅ Use descriptive names
- ✅ Keep migrations small and focused
- ✅ Never modify existing migrations after they've been applied
- ✅ Always create new migrations for schema changes

### Checking Migration Status

```go
// Query migration status
func checkMigrationStatus(db *sql.DB, projectionName string) {
    sanitizedName := validation.SanitizeIdentifier(projectionName)
    tableName := fmt.Sprintf("projection_%s_schema_migrations", sanitizedName)

    var version int
    var name string
    err := db.QueryRow(
        fmt.Sprintf("SELECT version, name FROM %s ORDER BY version DESC LIMIT 1", tableName),
    ).Scan(&version, &name)

    if err == sql.ErrNoRows {
        fmt.Printf("Projection %s: No migrations applied yet\n", projectionName)
    } else {
        fmt.Printf("Projection %s: version %d (%s)\n", projectionName, version, name)
    }
}
```

## Advanced Features

### 1. Schema Version Tracking with Metadata

Track your projection schema version for automatic rebuilds:

```go
const UserProjectionSchemaVersion = 3

func NewUserProjection(db *sql.DB, checkpointStore *sqlite.CheckpointStore,
    eventStore store.EventStore) (store.Projection, error) {

    projection := sqlite.NewSQLiteProjectionBuilder(
        "user-projection",
        db,
        checkpointStore,
        eventStore,
    ).
        WithMigrations(userProjectionMigrations, "migrations").
        On(userv1.OnUserCreated(handleUserCreated)).
        Build()

    // Cast to get access to metadata store
    sqliteProj := projection.(*sqlite.SQLiteProjection)
    metadataStore := sqliteProj.MetadataStore()

    // Check schema version
    savedVersion, _ := metadataStore.Get("user-projection", "schema_version")
    saved := 0
    if savedVersion != "" {
        saved, _ = strconv.Atoi(savedVersion)
    }

    // Rebuild if schema changed
    if saved < UserProjectionSchemaVersion {
        log.Printf("Schema version changed (%d -> %d), rebuilding...",
            saved, UserProjectionSchemaVersion)

        if err := sqliteProj.Rebuild(context.Background()); err != nil {
            return nil, fmt.Errorf("rebuild failed: %w", err)
        }

        metadataStore.Set("user-projection", "schema_version",
            fmt.Sprintf("%d", UserProjectionSchemaVersion))
    }

    return projection, nil
}
```

### 2. Conditional Event Handling

Skip certain events based on conditions:

```go
func handleUserCreated(ctx context.Context, event *userv1.UserCreatedEvent,
    envelope *domain.EventEnvelope) error {

    // Skip test users in production
    if strings.HasPrefix(event.Email, "test+") {
        return nil
    }

    tx, _ := sqlite.TxFromContext(ctx)

    _, err := tx.Exec(`
        INSERT INTO users (id, email, name, created_at)
        VALUES (?, ?, ?, ?)
    `, event.UserId, event.Email, event.Name, event.CreatedAt)

    return err
}
```

### 3. Denormalized Queries

Build denormalized read models for fast queries:

```go
func handleOrderPlaced(ctx context.Context, event *orderv1.OrderPlacedEvent,
    envelope *domain.EventEnvelope) error {

    tx, _ := sqlite.TxFromContext(ctx)

    // Insert order
    _, err := tx.Exec(`
        INSERT INTO orders (id, user_id, total, status, created_at)
        VALUES (?, ?, ?, ?, ?)
    `, event.OrderId, event.UserId, event.Total, "placed", event.PlacedAt)
    if err != nil {
        return err
    }

    // Insert denormalized order items
    for _, item := range event.Items {
        _, err := tx.Exec(`
            INSERT INTO order_items (order_id, product_id, quantity, price)
            VALUES (?, ?, ?, ?)
        `, event.OrderId, item.ProductId, item.Quantity, item.Price)
        if err != nil {
            return err
        }
    }

    // Update user statistics
    _, err = tx.Exec(`
        INSERT INTO user_stats (user_id, total_orders, total_spent)
        VALUES (?, 1, ?)
        ON CONFLICT(user_id) DO UPDATE SET
            total_orders = total_orders + 1,
            total_spent = total_spent + excluded.total_spent
    `, event.UserId, event.Total)

    return err
}
```

### 4. Maintaining Aggregates

Keep pre-computed aggregates for analytics:

```go
func handleProductViewed(ctx context.Context, event *analyticsv1.ProductViewedEvent,
    envelope *domain.EventEnvelope) error {

    tx, _ := sqlite.TxFromContext(ctx)

    // Record raw event
    _, err := tx.Exec(`
        INSERT INTO product_views (product_id, user_id, viewed_at)
        VALUES (?, ?, ?)
    `, event.ProductId, event.UserId, event.ViewedAt)
    if err != nil {
        return err
    }

    // Update hourly aggregate
    hour := time.Unix(event.ViewedAt, 0).Truncate(time.Hour).Unix()
    _, err = tx.Exec(`
        INSERT INTO product_views_hourly (product_id, hour, view_count)
        VALUES (?, ?, 1)
        ON CONFLICT(product_id, hour) DO UPDATE SET
            view_count = view_count + 1
    `, event.ProductId, hour)

    return err
}
```

### 5. Multiple Databases

Run projections in separate databases for scaling:

```go
// User projection in users.db
userDB, _ := sql.Open("sqlite", "users.db")
userCheckpointStore, _ := sqlite.NewCheckpointStore(userDB)
userProjection := NewUserProjection(userDB, userCheckpointStore, eventStore)

// Analytics projection in analytics.db
analyticsDB, _ := sql.Open("sqlite", "analytics.db")
analyticsCheckpointStore, _ := sqlite.NewCheckpointStore(analyticsDB)
analyticsProjection := NewAnalyticsProjection(analyticsDB, analyticsCheckpointStore, eventStore)
```

## Best Practices

### 1. Migration Best Practices

✅ **DO:**
- Keep migrations small and focused
- Use descriptive names
- Test migrations on a copy of production data
- Create indexes for query performance
- Add `IF NOT EXISTS` for idempotency

❌ **DON'T:**
- Modify existing migrations after deployment
- Skip version numbers
- Make breaking changes without data migration
- Forget to add indexes

### 2. Event Handler Best Practices

✅ **DO:**
- Keep handlers simple and focused
- Use transactions (automatic with SQLiteProjectionBuilder)
- Handle all event types your projection cares about
- Return errors for failures (automatic rollback)
- Use prepared statements via tx.Exec()

❌ **DON'T:**
- Make external API calls in handlers (breaks idempotency)
- Modify multiple projections in one handler
- Ignore errors
- Use time.Now() (use envelope.Timestamp)

### 3. Schema Design Best Practices

✅ **DO:**
- Denormalize for read performance
- Add indexes for common queries
- Use appropriate column types
- Store timestamps as INTEGER (Unix time)
- Keep event data in projection if needed

❌ **DON'T:**
- Normalize like OLTP (this is OLAP/read model)
- Create foreign keys between projections
- Store binary data without good reason
- Forget about query patterns

### 4. Rebuilding Best Practices

```go
// Safe rebuild with downtime window
func safeRebuild(projectionManager *eventsourcing.ProjectionManager) error {
    // 1. Stop projection
    projectionManager.Stop("user-projection")

    // 2. Rebuild (takes time)
    if err := projectionManager.Rebuild(ctx, "user-projection"); err != nil {
        return err
    }

    // 3. Restart projection
    return projectionManager.Start(ctx, "user-projection")
}

// Zero-downtime rebuild using shadow table
func zeroDowntimeRebuild(db *sql.DB) error {
    // 1. Create shadow projection with different name
    shadowProjection := sqlite.NewSQLiteProjectionBuilder(
        "user-projection-v2",  // Different name
        db,
        checkpointStore,
        eventStore,
    ).
        WithMigrations(newMigrations, "migrations").
        Build()

    // 2. Rebuild shadow projection (old projection still serving reads)
    shadowProjection.(*sqlite.SQLiteProjection).Rebuild(ctx)

    // 3. Atomically swap tables
    tx, _ := db.Begin()
    tx.Exec("ALTER TABLE users RENAME TO users_old")
    tx.Exec("ALTER TABLE users_v2 RENAME TO users")
    tx.Commit()

    // 4. Update projection name and restart
    // ...
}
```

## Troubleshooting

### Problem: Migrations Don't Run

**Symptom:** Tables not created, migrations skipped

**Solution:**
```go
// Check that migrations are embedded correctly
//go:embed migrations/*.sql
var migrationsFS embed.FS

// Verify path matches embed directive
.WithMigrations(migrationsFS, "migrations")  // Must match embed path
```

### Problem: Duplicate Events Processed

**Symptom:** Rows inserted twice, counts are doubled

**Possible Causes:**
1. Projection rebuilt while still running
2. NATS redelivery without checkpoint

**Solution:**
```go
// Always stop projection before rebuild
projectionManager.Stop("user-projection")
projectionManager.Rebuild(ctx, "user-projection")
projectionManager.Start(ctx, "user-projection")
```

### Problem: Migration Fails Mid-Way

**Symptom:** Some tables created, others missing

**Solution:**
```sql
-- Check migration status
SELECT * FROM projection_user_projection_schema_migrations;

-- Manually fix issue, then increment version
INSERT INTO projection_user_projection_schema_migrations (version, name, applied_at)
VALUES (2, '002_manual_fix.sql', unixepoch());
```

### Problem: Projection Falls Behind

**Symptom:** Checkpoint position not advancing

**Check:**
1. Event handler errors (check logs)
2. Transaction deadlocks
3. Slow queries

**Solution:**
```go
// Add logging to handlers
func handleUserCreated(ctx context.Context, event *userv1.UserCreatedEvent,
    envelope *domain.EventEnvelope) error {

    start := time.Now()
    defer func() {
        log.Printf("handleUserCreated took %v", time.Since(start))
    }()

    // ... handler code
}

// Add indexes for slow queries
CREATE INDEX idx_users_email ON users(email);
```

### Problem: Different Schema Versions Across Projections

**This is NORMAL!** Each projection evolves independently.

```sql
-- user-projection at version 5
SELECT * FROM projection_user_projection_schema_migrations;
-- version: 5

-- analytics-projection at version 2
SELECT * FROM projection_analytics_projection_schema_migrations;
-- version: 2
```

This is intentional and allows projections to evolve at their own pace.

## Complete Example

See a full working example: [examples/sqlite_projection/](../examples/sqlite_projection/)

## API Reference

### SQLiteProjectionBuilder

```go
// Create builder
builder := sqlite.NewSQLiteProjectionBuilder(
    name string,              // Unique projection name
    db *sql.DB,               // Database connection
    checkpointStore *CheckpointStore,
    eventStore store.EventStore,
)

// Configuration methods (chainable)
builder.WithMigrations(fs.FS, path string) *SQLiteProjectionBuilder
builder.On(registration store.EventHandlerRegistration) *SQLiteProjectionBuilder
builder.OnWithTx(eventType string, handler TransactionalEventHandler) *SQLiteProjectionBuilder
builder.OnReset(resetFunc func(context.Context, *sql.Tx) error) *SQLiteProjectionBuilder

// Build projection
projection, err := builder.Build()
```

### SQLiteProjection

```go
// Projection interface methods
projection.Name() string
projection.Handle(ctx context.Context, envelope *domain.EventEnvelope) error
projection.Reset(ctx context.Context) error

// SQLite-specific methods
projection.Rebuild(ctx context.Context) error
projection.GetCheckpoint(ctx context.Context) (*store.ProjectionCheckpoint, error)
projection.GetStatus(ctx context.Context) (*store.ProjectionState, error)
projection.MetadataStore() *ProjectionMetadataStore
projection.IsReady(ctx context.Context) bool
```

### Helper Functions

```go
// Get transaction from context
tx, ok := sqlite.TxFromContext(ctx)
```

## Related Documentation

- [Projection Metadata Store Guide](./PROJECTION_METADATA_STORE.md)
- [Projection Checkpoint System](./PROJECTION_CHECKPOINTS.md)
- [Event Sourcing Patterns](./EVENT_SOURCING_PATTERNS.md)

---

**Version:** EventStore v0.0.31+
**Last Updated:** 2025-11-20
**Author:** Pascal Laenen (@plaenen)
