# Projection Patterns Guide

This guide shows all the different ways to build projections in the event sourcing framework, from simple to advanced.

## Table of Contents

1. [Basic Concepts](#basic-concepts)
2. [Pattern 1: Generic Projection Builder](#pattern-1-generic-projection-builder)
3. [Pattern 2: SQLite Projection Builder](#pattern-2-sqlite-projection-builder)
4. [Pattern 3: Projections with NATS EventBus](#pattern-3-projections-with-nats-eventbus)
5. [Comparison Table](#comparison-table)
6. [Best Practices](#best-practices)

---

## Basic Concepts

### What is a Projection?

A projection is a read model built from events. It transforms the event stream into a queryable format optimized for specific queries.

### Key Components

- **Projection**: Processes events and updates a read model
- **CheckpointStore**: Tracks projection progress for resumability
- **EventStore**: Source of events for rebuilding
- **EventBus**: Real-time event delivery (NATS)
- **ProjectionManager**: Coordinates multiple projections

---

## Pattern 1: Generic Projection Builder

**Use when**: You need cross-domain projections that listen to events from multiple aggregates.

### Features

✅ Mix events from multiple aggregates/domains
✅ Type-safe event handlers
✅ Full IDE code completion
✅ Pick and choose specific events

### Example

```go
projection := eventsourcing.NewProjectionBuilder("customer-360").
    // Account domain events
    On(accountv1.OnAccountOpened(func(ctx context.Context, event *accountv1.AccountOpenedEvent, envelope *eventsourcing.EventEnvelope) error {
        // Handle account opened
        // Manual transaction management required
        tx, _ := db.Begin()
        defer tx.Rollback()

        _, err := tx.Exec("INSERT INTO customer_activity ...")
        if err != nil {
            return err
        }

        // Manual checkpoint update
        checkpoint := &eventsourcing.ProjectionCheckpoint{...}
        checkpointStore.SaveInTx(tx, checkpoint)

        return tx.Commit()
    })).
    // Order domain events
    On(orderv1.OnOrderPlaced(func(ctx, event, envelope) error {
        // Handle order placed
        return nil
    })).
    // User domain events
    On(userv1.OnUserRegistered(func(ctx, event, envelope) error {
        // Handle user registered
        return nil
    })).
    OnReset(func(ctx context.Context) error {
        _, err := db.Exec("DELETE FROM customer_activity")
        return err
    }).
    Build()
```

### Pros & Cons

**Pros:**
- Cross-domain event handling
- Maximum flexibility
- Type-safe handlers

**Cons:**
- Manual transaction management
- Manual checkpoint updates
- More boilerplate code

---

## Pattern 2: SQLite Projection Builder

**Use when**: You're using SQLite and want automatic transaction/checkpoint management.

### Features

✅ Automatic transaction management (begin/commit/rollback)
✅ Automatic checkpoint updates (atomic)
✅ Built-in rebuild functionality
✅ Schema initialization support
✅ Minimal boilerplate

### Example

```go
projection, err := sqlite.NewSQLiteProjectionBuilder(
    "account-balance",
    db,
    checkpointStore,
    eventStore,
).
    // Schema runs automatically during Build()
    WithSchema(func(ctx context.Context, db *sql.DB) error {
        _, err := db.Exec(`
            CREATE TABLE IF NOT EXISTS account_balance (
                account_id TEXT PRIMARY KEY,
                balance TEXT NOT NULL
            )
        `)
        return err
    }).
    // Handlers with automatic transaction management
    On(accountv1.OnAccountOpened(func(ctx context.Context, event *accountv1.AccountOpenedEvent, envelope *eventsourcing.EventEnvelope) error {
        // Transaction automatically provided via context
        tx, _ := sqlite.TxFromContext(ctx)

        // Just do your SQL - transaction and checkpoint handled automatically!
        _, err := tx.Exec("INSERT INTO account_balance VALUES (?, ?)",
            event.AccountId, event.InitialBalance)
        return err
        // Automatic commit and checkpoint update!
    })).
    On(accountv1.OnMoneyDeposited(func(ctx, event, envelope) error {
        tx, _ := sqlite.TxFromContext(ctx)
        _, err := tx.Exec("UPDATE account_balance SET balance = ? WHERE account_id = ?",
            event.NewBalance, event.AccountId)
        return err
    })).
    OnReset(func(ctx context.Context, tx *sql.Tx) error {
        _, err := tx.Exec("DELETE FROM account_balance")
        return err
    }).
    Build()

if err != nil {
    log.Fatal(err)
}
```

### Transaction Context

The transaction is available via context:

```go
On(accountv1.OnAccountOpened(func(ctx context.Context, event *accountv1.AccountOpenedEvent, envelope *eventsourcing.EventEnvelope) error {
    // Get transaction from context
    tx, ok := sqlite.TxFromContext(ctx)
    if !ok {
        return fmt.Errorf("no transaction in context")
    }

    // Use transaction
    _, err := tx.Exec("INSERT INTO ...")
    return err
}))
```

### Rebuilding

SQLite projections support rebuilding:

```go
sqliteProj := projection.(*sqlite.SQLiteProjection)

// Rebuild from event store
if err := sqliteProj.Rebuild(ctx); err != nil {
    log.Fatal(err)
}
```

### Pros & Cons

**Pros:**
- Zero transaction boilerplate
- Automatic checkpoint management
- Built-in rebuild support
- Schema initialization
- Clean, simple code

**Cons:**
- SQLite-specific
- Less flexible than generic builder
- Single domain only (can't mix events from different aggregates)

---

## Pattern 3: Projections with NATS EventBus

**Use when**: You want real-time projection updates as events are published.

### Features

✅ Real-time event processing
✅ Multiple projections running concurrently
✅ Automatic checkpoint tracking
✅ Resume from last checkpoint on restart
✅ Rebuild support from EventStore

### Architecture

```
┌─────────────┐
│   Command   │
│   Handler   │
└──────┬──────┘
       │ publishes events
       ▼
┌─────────────┐       ┌──────────────┐
│    NATS     │──────▶│  Projection  │
│  EventBus   │       │   Manager    │
└─────────────┘       └──────┬───────┘
                             │ dispatches
                             ▼
                    ┌────────────────┐
                    │  Projection 1  │
                    │  Projection 2  │
                    │  Projection 3  │
                    └────────────────┘
```

### Example

```go
// 1. Setup infrastructure
eventStore, _ := sqlite.NewEventStore(...)
checkpointStore, _ := sqlite.NewCheckpointStore(eventStore.DB())

// NATS server
natsServer, _ := natspkg.NewServer(&natspkg.ServerConfig{
    Port: -1, // Random port
})
defer natsServer.Shutdown()

// NATS EventBus
eventBus, _ := natspkg.NewEventBus(natspkg.EventBusConfig{
    URL:    natsServer.ClientURL(),
    Stream: "events",
})
defer eventBus.Close()

// 2. Build projections (using SQLite builder for convenience)
accountBalanceProjection, _ := sqlite.NewSQLiteProjectionBuilder(
    "account-balance",
    eventStore.DB(),
    checkpointStore,
    eventStore,
).
    WithSchema(func(ctx context.Context, db *sql.DB) error {
        _, err := db.Exec("CREATE TABLE IF NOT EXISTS account_balance (...)")
        return err
    }).
    On(accountv1.OnAccountOpened(func(ctx, event, envelope) error {
        tx, _ := sqlite.TxFromContext(ctx)
        _, err := tx.Exec("INSERT INTO account_balance ...")
        return err
    })).
    Build()

activityLogProjection, _ := sqlite.NewSQLiteProjectionBuilder(
    "activity-log",
    eventStore.DB(),
    checkpointStore,
    eventStore,
).
    WithSchema(func(ctx context.Context, db *sql.DB) error {
        _, err := db.Exec("CREATE TABLE IF NOT EXISTS activity_log (...)")
        return err
    }).
    On(accountv1.OnAccountOpened(func(ctx, event, envelope) error {
        tx, _ := sqlite.TxFromContext(ctx)
        _, err := tx.Exec("INSERT INTO activity_log ...")
        return err
    })).
    Build()

// 3. Create ProjectionManager with EventBus
projectionManager := eventsourcing.NewProjectionManager(
    checkpointStore,
    eventStore,
    eventBus, // ← EventBus for real-time events
)

// 4. Register projections
projectionManager.Register(accountBalanceProjection)
projectionManager.Register(activityLogProjection)

// 5. Start projections (begins listening to NATS)
ctx := context.Background()
projectionManager.Start(ctx, "account-balance")
projectionManager.Start(ctx, "activity-log")

// 6. Events published to NATS are automatically processed!
events := []*eventsourcing.Event{...}
eventBus.Publish(events)

// Projections automatically:
// - Receive events from NATS
// - Process events with registered handlers
// - Update read models transactionally
// - Save checkpoints atomically

// 7. Query read models
var balance string
db.QueryRow("SELECT balance FROM account_balance WHERE account_id = ?", id).Scan(&balance)

// 8. Stop projections
defer projectionManager.StopAll()
```

### Rebuilding with ProjectionManager

```go
// Rebuild a specific projection from event store
if err := projectionManager.Rebuild(ctx, "account-balance"); err != nil {
    log.Fatal(err)
}
```

### Pros & Cons

**Pros:**
- Real-time updates
- Multiple projections run concurrently
- Automatic checkpoint management
- Resume on restart
- Scalable architecture

**Cons:**
- Requires NATS infrastructure
- More moving parts
- Network dependency

---

## Comparison Table

| Feature | Generic Builder | SQLite Builder | NATS EventBus |
|---------|----------------|----------------|---------------|
| **Cross-domain events** | ✅ Yes | ❌ Single aggregate | ✅ Any event |
| **Auto transactions** | ❌ Manual | ✅ Automatic | ✅ Automatic (with SQLite builder) |
| **Auto checkpoints** | ❌ Manual | ✅ Automatic | ✅ Automatic |
| **Real-time updates** | ❌ Batch only | ❌ Batch only | ✅ Real-time |
| **Rebuild support** | ⚠️ Manual | ✅ Built-in | ✅ Built-in |
| **Schema init** | ❌ Manual | ✅ Automatic | ✅ Automatic (with SQLite builder) |
| **Boilerplate** | ⚠️ Medium | ✅ Minimal | ✅ Minimal |
| **Flexibility** | ✅ Maximum | ⚠️ SQLite-specific | ⚠️ Requires NATS |

---

## Best Practices

### 1. Use SQLite Builder for Simple Projections

For single-aggregate projections with SQLite, use the SQLite builder:

```go
projection, _ := sqlite.NewSQLiteProjectionBuilder(...).
    WithSchema(...).
    On(accountv1.OnAccountOpened(...)).
    Build()
```

### 2. Use Generic Builder for Cross-Domain Projections

For projections that need events from multiple aggregates:

```go
projection := eventsourcing.NewProjectionBuilder("customer-360").
    On(accountv1.OnAccountOpened(...)).
    On(orderv1.OnOrderPlaced(...)).
    On(userv1.OnUserRegistered(...)).
    Build()
```

### 3. Use NATS for Real-Time Updates

In production, use ProjectionManager with NATS EventBus:

```go
manager := eventsourcing.NewProjectionManager(
    checkpointStore,
    eventStore,
    eventBus, // ← NATS EventBus
)
manager.Register(projection)
manager.Start(ctx, "projection-name")
```

### 4. Always Use Transactions

Projection updates and checkpoint saves must be atomic:

```go
// ✅ Good: Transaction ensures atomicity
tx, _ := db.Begin()
tx.Exec("UPDATE projection ...")
checkpointStore.SaveInTx(tx, checkpoint)
tx.Commit()

// ❌ Bad: Dual-write problem!
db.Exec("UPDATE projection ...")
checkpointStore.Save(checkpoint) // Separate transaction!
```

### 5. Schema Initialization

Always initialize your projection schema:

```go
WithSchema(func(ctx context.Context, db *sql.DB) error {
    _, err := db.Exec(`
        CREATE TABLE IF NOT EXISTS my_projection (...)
    `)
    return err
})
```

### 6. Idempotent Handlers

Make handlers idempotent to handle duplicate events safely:

```go
On(accountv1.OnAccountOpened(func(ctx, event, envelope) error {
    tx, _ := sqlite.TxFromContext(ctx)

    // Use INSERT OR IGNORE / UPSERT
    _, err := tx.Exec(`
        INSERT INTO accounts (id, balance) VALUES (?, ?)
        ON CONFLICT(id) DO NOTHING
    `, event.AccountId, event.Balance)

    return err
}))
```

### 7. Error Handling

Let errors bubble up - the framework will handle retries:

```go
On(accountv1.OnAccountOpened(func(ctx, event, envelope) error {
    tx, _ := sqlite.TxFromContext(ctx)

    _, err := tx.Exec("INSERT ...")
    if err != nil {
        return err // Framework will retry
    }

    return nil
}))
```

---

## Complete Example

See the demo files:
- `generic_projection_demo.go` - Generic cross-domain projection
- `sqlite_projection_demo.go` - SQLite projection with automatic management
- `projection_with_nats_demo.go` - Real-time projections with NATS EventBus
