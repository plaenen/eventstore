# LibSQL Event Store Usage Guide

The event store now fully supports LibSQL with three deployment modes: **local files**, **remote Turso**, and **embedded replicas**.

## Deployment Modes

### 1. Local File (Default)

Traditional SQLite file on disk:

```go
import "github.com/plaenen/eventstore/pkg/store/sqlite"

// Use default filename (eventstore.db)
store, err := sqlite.NewEventStore()

// Or specify custom filename
store, err := sqlite.NewEventStore(
    sqlite.WithFilename("events.db"),
)

// In-memory for testing
store, err := sqlite.NewEventStore(
    sqlite.WithMemoryDatabase(),
    sqlite.WithWALMode(false), // WAL not supported for :memory:
)
```

### 2. Remote Turso Database

Cloud-hosted LibSQL database with full ACID guarantees:

```go
import (
    "os"
    "github.com/plaenen/eventstore/pkg/store/sqlite"
)

store, err := sqlite.NewEventStore(
    sqlite.WithLibSQLRemote(
        "libsql://mydb.turso.io",
        os.Getenv("TURSO_AUTH_TOKEN"),
    ),
)
```

**Features:**
- ✅ Globally distributed
- ✅ Automatic backups
- ✅ Point-in-time recovery
- ✅ Built-in authentication

### 3. Embedded Replica (Local-First)

Local database with automatic cloud synchronization:

```go
store, err := sqlite.NewEventStore(
    sqlite.WithLibSQLEmbeddedReplica(
        "local-events.db",           // Local cache
        "libsql://prod.turso.io",    // Remote sync URL
        os.Getenv("TURSO_AUTH_TOKEN"),
    ),
)
```

**Features:**
- ✅ Offline-first: Works without internet
- ✅ Low latency: Reads from local disk
- ✅ Auto-sync: Periodically syncs with remote
- ✅ Conflict-free: Event sourcing is naturally append-only

### 4. Advanced: Custom Connector

For full control over LibSQL features:

```go
import (
    libsql "github.com/tursodatabase/go-libsql"
    "github.com/plaenen/eventstore/pkg/store/sqlite"
)

connector := libsql.NewEmbeddedReplicaConnector(
    "local.db",
    "libsql://remote.turso.io",
    libsql.WithAuthToken(os.Getenv("TURSO_TOKEN")),
    libsql.WithSyncInterval(time.Minute),
    libsql.WithEncryption("encryption-key"),
)

store, err := sqlite.NewEventStore(
    sqlite.WithLibSQLConnector(connector),
)
```

## Schema Migrations

LibSQL supports advanced ALTER TABLE operations beyond standard SQLite:

```sql
-- Standard SQLite (supported)
ALTER TABLE events ADD COLUMN source_system TEXT;
ALTER TABLE events RENAME TO events_backup;

-- LibSQL Extensions (also supported)
ALTER TABLE events RENAME COLUMN source_system TO origin_system;
ALTER TABLE events DROP COLUMN origin_system;
```

### Creating Migrations

Add new numbered migration files to `pkg/store/sqlite/migrations/`:

```
migrations/
├── 000001_initial_schema.up.sql
├── 000001_initial_schema.down.sql
├── 000002_add_source_tracking.up.sql    ← New migration
└── 000002_add_source_tracking.down.sql  ← Rollback
```

Example migration:

```sql
-- 000002_add_source_tracking.up.sql
ALTER TABLE events ADD COLUMN source_system TEXT;

CREATE INDEX IF NOT EXISTS idx_events_source
    ON events(source_system) WHERE source_system IS NOT NULL;
```

Migrations are automatically applied on startup when `WithAutoMigrate(true)` (default).

## Configuration Options

```go
store, err := sqlite.NewEventStore(
    // Database location
    sqlite.WithFilename("events.db"),
    // OR
    sqlite.WithMemoryDatabase(),
    // OR
    sqlite.WithLibSQLRemote("libsql://...", "token"),
    // OR
    sqlite.WithLibSQLEmbeddedReplica("local.db", "libsql://...", "token"),

    // Connection pool
    sqlite.WithMaxOpenConns(25),
    sqlite.WithMaxIdleConns(5),

    // Write-ahead logging (recommended for production)
    sqlite.WithWALMode(true),  // Default: true

    // Automatic migrations
    sqlite.WithAutoMigrate(true),  // Default: true
)
```

## Production Recommendations

### Local Development
```go
store, err := sqlite.NewEventStore(
    sqlite.WithFilename("dev-events.db"),
    sqlite.WithWALMode(true),
)
```

### Production (Cloud)
```go
store, err := sqlite.NewEventStore(
    sqlite.WithLibSQLRemote(
        os.Getenv("TURSO_URL"),
        os.Getenv("TURSO_TOKEN"),
    ),
    sqlite.WithMaxOpenConns(100),
)
```

### Production (Edge/Distributed)
```go
store, err := sqlite.NewEventStore(
    sqlite.WithLibSQLEmbeddedReplica(
        "/var/lib/events/local.db",
        os.Getenv("TURSO_URL"),
        os.Getenv("TURSO_TOKEN"),
    ),
    sqlite.WithMaxOpenConns(50),
)
```

## LibSQL Features

### Schema Evolution
- ✅ `ALTER TABLE ADD COLUMN` (standard SQLite)
- ✅ `ALTER TABLE RENAME COLUMN` (LibSQL extension)
- ✅ `ALTER TABLE DROP COLUMN` (LibSQL extension)
- ✅ `ALTER TABLE RENAME TO` (standard SQLite)

### Search & Indexing
- ✅ FTS5 full-text search
- ✅ R*Tree spatial indexing
- ✅ Vector search (native)
- ✅ JSON functions

### Performance
- ✅ Randomized ROWID for better performance
- ✅ Virtual write-ahead log interface
- ✅ Connection pooling

### Security
- ✅ Encryption at rest
- ✅ Token-based authentication (remote/replicas)
- ✅ SQL injection protection (parameterized queries)

## Turso Setup

1. **Install Turso CLI:**
   ```bash
   curl -sSfL https://get.tur.so/install.sh | bash
   ```

2. **Create Database:**
   ```bash
   turso db create my-eventstore
   ```

3. **Get Connection String:**
   ```bash
   turso db show my-eventstore --url
   # Output: libsql://my-eventstore-username.turso.io
   ```

4. **Create Auth Token:**
   ```bash
   turso db tokens create my-eventstore
   # Output: eyJhbGc...
   ```

5. **Use in Application:**
   ```go
   store, err := sqlite.NewEventStore(
       sqlite.WithLibSQLRemote(
           "libsql://my-eventstore-username.turso.io",
           "eyJhbGc...",
       ),
   )
   ```

## Testing

All deployment modes are fully tested:

```bash
# Run all tests
go test ./pkg/store/sqlite/...

# Run specific test
go test -v ./pkg/store/sqlite -run TestEventStore
```

## Troubleshooting

### Connection Issues
```go
// Enable debug logging
import "log"

store, err := sqlite.NewEventStore(
    sqlite.WithLibSQLRemote(url, token),
)
if err != nil {
    log.Fatalf("Failed to connect: %v", err)
}
```

### Migration Failures
Migrations are applied in transactions. If a migration fails:
1. Check `schema_migrations` table for current version
2. Fix the failing migration SQL
3. Restart the application (migrations resume from last successful version)

### WAL Mode Issues
WAL mode is not supported for `:memory:` databases:
```go
store, err := sqlite.NewEventStore(
    sqlite.WithMemoryDatabase(),
    sqlite.WithWALMode(false),  // Must disable for :memory:
)
```

## Resources

- [LibSQL GitHub](https://github.com/tursodatabase/libsql)
- [Turso Documentation](https://docs.turso.tech/)
- [go-libsql Package](https://pkg.go.dev/github.com/tursodatabase/go-libsql)
