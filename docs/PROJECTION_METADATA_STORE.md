# Projection Metadata Store

The `ProjectionMetadataStore` is a key-value store for managing projection lifecycle metadata including schema versions, rebuild flags, and custom configuration properties.

## Overview

The eventstore package provides a complete projection management system with three complementary stores:

1. **ProjectionCheckpoint** - Tracks event position and NATS sequence for resuming projections
2. **ProjectionStatus** - Monitors projection health (READY/REBUILDING/FAILED/PAUSED)
3. **ProjectionMetadataStore** - Manages projection lifecycle (schema versions, rebuild triggers, custom config) ✨ NEW

## Features

- ✅ **Schema version tracking** - Trigger automatic rebuilds after migrations
- ✅ **Manual rebuild requests** - Operator-initiated rebuild flags
- ✅ **Custom configuration** - Store projection-specific settings (batch sizes, filters, etc.)
- ✅ **Projection versioning** - Track logic versions for rebuild detection
- ✅ **Transaction support** - Atomic updates with projection data
- ✅ **Zero boilerplate** - Auto-migrations, no manual table creation
- ✅ **Standard pattern** - Consistent across all eventstore projects

## Usage

### Basic Operations

```go
import (
    "github.com/plaenen/eventstore/pkg/store/sqlite"
)

// Create metadata store (auto-migrates)
metadataStore, err := sqlite.NewProjectionMetadataStore(db)
if err != nil {
    return fmt.Errorf("failed to create metadata store: %w", err)
}

// Set a value
err = metadataStore.Set("user-projection", "schema_version", "3")

// Get a value (returns empty string if not found)
version, err := metadataStore.Get("user-projection", "schema_version")

// Get all metadata for a projection
metadata, err := metadataStore.GetAll("user-projection")
// Returns: map[string]string{"schema_version": "3", "batch_size": "1000", ...}

// Delete a specific key
err = metadataStore.Delete("user-projection", "rebuild_requested")

// Delete all metadata for a projection
err = metadataStore.DeleteAll("user-projection")

// List all projections with metadata
projections, err := metadataStore.ListProjections()
// Returns: []string{"user-projection", "analytics-projection", ...}
```

### Integration with SQLiteProjectionBuilder

The metadata store is automatically created and integrated when using `SQLiteProjectionBuilder`:

```go
projection := sqlite.NewSQLiteProjectionBuilder(
    "user-projection",
    db,
    checkpointStore,
    eventStore,
).
    WithMigrations(migrationsFS, "migrations").
    On(userv1.OnUserCreated(handleUserCreated)).
    Build()

// Access metadata store
metadataStore := projection.(*sqlite.SQLiteProjection).MetadataStore()
metadataStore.Set("user-projection", "schema_version", "1")
```

## Common Use Cases

### 1. Schema Version Tracking (Automatic Rebuilds)

Track projection schema versions to automatically trigger rebuilds after migrations:

```go
const CurrentSchemaVersion = 3

// On application startup
func initializeProjection(projection *SQLiteProjection) error {
    metadataStore := projection.MetadataStore()

    // Check current schema version
    savedVersion, err := metadataStore.Get(projection.Name(), "schema_version")
    if err != nil {
        return err
    }

    // Convert to int for comparison
    saved := 0
    if savedVersion != "" {
        saved, _ = strconv.Atoi(savedVersion)
    }

    // Rebuild if schema changed
    if saved < CurrentSchemaVersion {
        log.Printf("Schema version changed (%d -> %d), rebuilding projection...",
            saved, CurrentSchemaVersion)

        if err := projection.Rebuild(ctx); err != nil {
            return fmt.Errorf("rebuild failed: %w", err)
        }

        // Update schema version
        metadataStore.Set(projection.Name(), "schema_version",
            fmt.Sprintf("%d", CurrentSchemaVersion))
    }

    return nil
}
```

### 2. Manual Rebuild Requests

Support operator-initiated rebuilds via CLI or API:

```go
// CLI command: steward projection rebuild user-projection
func RebuildProjectionCommand(projectionName string) error {
    metadataStore := getMetadataStore()

    // Set rebuild flag
    err := metadataStore.Set(projectionName, "rebuild_requested", "true")
    if err != nil {
        return fmt.Errorf("failed to set rebuild flag: %w", err)
    }

    log.Printf("Rebuild requested for %s. Will trigger on next restart.", projectionName)
    return nil
}

// On application startup
func startProjection(projection *SQLiteProjection) error {
    metadataStore := projection.MetadataStore()

    // Check rebuild flag
    rebuildRequested, _ := metadataStore.Get(projection.Name(), "rebuild_requested")
    if rebuildRequested == "true" {
        log.Printf("Rebuild requested for %s, starting rebuild...", projection.Name())

        if err := projection.Rebuild(ctx); err != nil {
            return fmt.Errorf("rebuild failed: %w", err)
        }

        // Clear flag after successful rebuild
        metadataStore.Delete(projection.Name(), "rebuild_requested")
        log.Printf("Rebuild completed for %s", projection.Name())
    }

    return nil
}
```

### 3. Custom Projection Configuration

Store and retrieve projection-specific configuration:

```go
// Configure projection settings
func configureProjection(projection *SQLiteProjection) error {
    metadataStore := projection.MetadataStore()

    // Store configuration
    config := map[string]string{
        "batch_size":       "1000",
        "retention_days":   "90",
        "filter_tenant_id": "tenant-123",
        "algorithm":        "collaborative-filtering-v2",
    }

    for key, value := range config {
        if err := metadataStore.Set(projection.Name(), key, value); err != nil {
            return fmt.Errorf("failed to set %s: %w", key, err)
        }
    }

    return nil
}

// Read configuration at runtime
func getProjectionConfig(projection *SQLiteProjection) (map[string]string, error) {
    metadataStore := projection.MetadataStore()

    config, err := metadataStore.GetAll(projection.Name())
    if err != nil {
        return nil, fmt.Errorf("failed to get config: %w", err)
    }

    // Use configuration
    batchSize, _ := strconv.Atoi(config["batch_size"])
    tenantFilter := config["filter_tenant_id"]

    log.Printf("Projection configured with batch_size=%d, tenant=%s",
        batchSize, tenantFilter)

    return config, nil
}
```

### 4. Projection Logic Versioning

Track projection logic versions to trigger rebuilds when algorithms change:

```go
const ProjectionLogicVersion = "2.1.0"

func checkLogicVersion(projection *SQLiteProjection) error {
    metadataStore := projection.MetadataStore()

    // Get deployed version
    deployedVersion, _ := metadataStore.Get(projection.Name(), "logic_version")

    // Rebuild if logic changed
    if deployedVersion != ProjectionLogicVersion {
        log.Printf("Projection logic changed (%s -> %s), rebuilding...",
            deployedVersion, ProjectionLogicVersion)

        if err := projection.Rebuild(ctx); err != nil {
            return fmt.Errorf("rebuild failed: %w", err)
        }

        // Update logic version
        metadataStore.Set(projection.Name(), "logic_version", ProjectionLogicVersion)

        // Also track what changed
        metadataStore.Set(projection.Name(), "algorithm", "collaborative-filtering-v2")
        metadataStore.Set(projection.Name(), "last_rebuild_reason", "logic_upgrade")
    }

    return nil
}
```

### 5. Transactional Updates

Atomically update metadata with projection data:

```go
func handleUserCreatedWithMetadata(ctx context.Context, event *userv1.UserCreatedEvent,
    envelope *domain.EventEnvelope) error {

    tx, ok := sqlite.TxFromContext(ctx)
    if !ok {
        return fmt.Errorf("transaction not found in context")
    }

    // Update projection data
    _, err := tx.Exec(`
        INSERT INTO users (id, email, created_at)
        VALUES (?, ?, ?)
    `, event.UserId, event.Email, event.CreatedAt)
    if err != nil {
        return err
    }

    // Update metadata atomically in same transaction
    metadataStore := getMetadataStore()

    // Track last processed user ID
    err = metadataStore.SetInTx(tx, "user-projection", "last_user_id", event.UserId)
    if err != nil {
        return err
    }

    // Increment user count
    userCount, _ := metadataStore.Get("user-projection", "total_users")
    count, _ := strconv.Atoi(userCount)
    metadataStore.SetInTx(tx, "user-projection", "total_users",
        fmt.Sprintf("%d", count+1))

    return nil
}
```

## Database Schema

The metadata store automatically creates the following schema via migrations:

```sql
CREATE TABLE projection_metadata (
    projection_name TEXT NOT NULL,
    key TEXT NOT NULL,
    value TEXT NOT NULL,
    updated_at INTEGER NOT NULL,

    PRIMARY KEY (projection_name, key),

    CHECK (length(projection_name) > 0),
    CHECK (length(key) > 0)
);

CREATE INDEX idx_metadata_projection ON projection_metadata(projection_name);
CREATE INDEX idx_metadata_key ON projection_metadata(key);
CREATE INDEX idx_metadata_updated ON projection_metadata(updated_at DESC);
```

## Configuration Options

```go
// Disable auto-migrations (not recommended)
metadataStore, err := sqlite.NewProjectionMetadataStore(
    db,
    sqlite.WithMetadataAutoMigrate(false),
)
```

## Database Options

The metadata store can share a database with other stores or use a separate database:

```go
// Option 1: Same database as CheckpointStore (recommended - keeps projection data together)
metadataStore, err := sqlite.NewProjectionMetadataStore(checkpointStore.DB())

// Option 2: Same database as EventStore
metadataStore, err := sqlite.NewProjectionMetadataStore(eventStore.DB())

// Option 3: Separate database
db, err := sql.Open("sqlite", "projection_metadata.db")
metadataStore, err := sqlite.NewProjectionMetadataStore(db)
```

## Testing

The metadata store includes comprehensive tests covering:

- ✅ Basic CRUD operations
- ✅ Transactional updates
- ✅ Multi-projection isolation
- ✅ All real-world use cases

See `pkg/store/sqlite/projection_metadata_store_test.go` for examples.

## Migration from Manual Implementation

If you're currently managing metadata manually:

### Before (Manual Implementation)

```go
// Manual table creation
db.Exec(`CREATE TABLE IF NOT EXISTS projection_metadata (...)`)

// Manual operations
db.Exec(`INSERT OR REPLACE INTO projection_metadata ...`)
```

### After (Using ProjectionMetadataStore)

```go
// Auto-migrated, standardized interface
metadataStore, _ := sqlite.NewProjectionMetadataStore(db)
metadataStore.Set("projection-name", "key", "value")
```

## Best Practices

1. **Schema Versioning**: Always track schema versions to enable automatic rebuilds
2. **Rebuild Flags**: Support operator-initiated rebuilds for troubleshooting
3. **Configuration**: Store projection-specific settings for runtime configuration
4. **Transactional Updates**: Use `*InTx` methods when updating metadata with projection data
5. **Naming Convention**: Use consistent key names across projections (e.g., `schema_version`, `logic_version`)

## Common Metadata Keys

Recommended standard keys for consistency across projections:

- `schema_version` - Database schema version (integer as string)
- `logic_version` - Projection logic version (semver string)
- `rebuild_requested` - Manual rebuild flag ("true"/"false")
- `last_rebuild_at` - Unix timestamp of last rebuild
- `last_rebuild_reason` - Why the last rebuild occurred
- `batch_size` - Processing batch size
- `filter_*` - Various filters (e.g., `filter_tenant_id`)
- `algorithm` - Algorithm or strategy name
- `retention_days` - Data retention policy

## API Reference

### ProjectionMetadataStore Interface

```go
type ProjectionMetadataStore interface {
    // Get retrieves a metadata value by key (returns empty string if not found)
    Get(projectionName, key string) (string, error)

    // Set saves a metadata key-value pair (creates or updates)
    Set(projectionName, key, value string) error

    // Delete removes a metadata key (no-op if doesn't exist)
    Delete(projectionName, key string) error

    // GetAll retrieves all metadata for a projection as a map
    GetAll(projectionName string) (map[string]string, error)

    // DeleteAll removes all metadata for a projection
    DeleteAll(projectionName string) error

    // ListProjections returns projection names that have metadata
    ListProjections() ([]string, error)
}
```

### SQLite-Specific Methods

```go
// Transaction-aware methods for atomic updates
SetInTx(tx *sql.Tx, projectionName, key, value string) error
DeleteInTx(tx *sql.Tx, projectionName, key string) error
DeleteAllInTx(tx *sql.Tx, projectionName string) error

// Database access
DB() *sql.DB
```

## Troubleshooting

### Metadata not persisting

Check that auto-migrations are enabled (default):

```go
metadataStore, err := sqlite.NewProjectionMetadataStore(
    db,
    sqlite.WithMetadataAutoMigrate(true), // This is the default
)
```

### Values not updating

Ensure you're using `Set()` which upserts (creates or updates):

```go
// This always works (creates or updates)
metadataStore.Set("proj", "key", "new-value")
```

### Transaction rollbacks

When using `*InTx` methods, ensure the transaction is committed:

```go
tx, _ := db.Begin()
metadataStore.SetInTx(tx, "proj", "key", "value")
tx.Commit() // Don't forget this!
```

## Performance Considerations

- **Indexes**: The metadata store creates indexes on `projection_name`, `key`, and `updated_at`
- **Batch Operations**: Use `GetAll()` to retrieve multiple keys in one query
- **Transactions**: Use `*InTx` methods to avoid separate round-trips
- **Caching**: Consider caching frequently-accessed metadata in memory

## Future Enhancements

Potential future additions:

- [ ] TTL/expiration support for temporary metadata
- [ ] Metadata change history/audit log
- [ ] Typed accessors for common metadata (version as int, etc.)
- [ ] Metadata validation rules
- [ ] Bulk operations for multiple projections

## Contributing

Found a bug or have a feature request? Please open an issue at:
https://github.com/plaenen/eventstore/issues

---

**Added in**: EventStore v0.0.31+
**Author**: Pascal Laenen (@plaenen)
**Related**: ProjectionCheckpoint, ProjectionStatus, SQLiteProjectionBuilder
