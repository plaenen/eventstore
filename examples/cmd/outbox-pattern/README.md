# Transactional Outbox Pattern with Projections

This example demonstrates the complete event-driven architecture using:
- **Transactional Outbox Pattern** - Guaranteed event publishing
- **NATS Event Bus** - Real-time event streaming
- **Projections** - Read model updates from events

## Architecture Overview

```
┌─────────────┐     ┌──────────────┐     ┌─────────────┐     ┌──────────────┐
│  Command    │────▶│  Event Store │────▶│   Outbox    │────▶│  NATS Bus    │
│  Handler    │     │  (SQLite)    │     │  Forwarder  │     │ (JetStream)  │
└─────────────┘     └──────────────┘     └─────────────┘     └──────────────┘
                           │                                          │
                           │                                          ▼
                           │                                  ┌───────────────┐
                           │                                  │  Projection   │
                           │                                  │   Manager     │
                           │                                  └───────────────┘
                           │                                          │
                           │                                          ▼
                           │                                  ┌───────────────┐
                           │                                  │  Read Models  │
                           └─────────────────────────────────▶│  (Rebuild)    │
                                    (For rebuilds)            └───────────────┘
```

## How to Use Projections with NATS

### 1. Build Your Projection

You can use either the generic builder or the SQLite-specific builder:

**Option A: Generic Projection Builder**
```go
import (
    "github.com/plaenen/eventstore/pkg/eventsourcing"
    accountv1 "github.com/plaenen/eventstore/examples/pb/account/v1"
)

projection := eventsourcing.NewProjectionBuilder("account-summary").
    On(accountv1.OnAccountOpened(func(ctx context.Context, event *accountv1.AccountOpenedEvent, envelope *domain.EventEnvelope) error {
        // Update your read model
        return nil
    })).
    On(accountv1.OnMoneyDeposited(func(ctx context.Context, event *accountv1.MoneyDepositedEvent, envelope *domain.EventEnvelope) error {
        // Update your read model
        return nil
    })).
    OnReset(func(ctx context.Context) error {
        // Reset your read model
        return nil
    }).
    Build()
```

**Option B: SQLite Projection Builder (Recommended)**
```go
import (
    "github.com/plaenen/eventstore/pkg/eventsourcing/store/sqlite"
)

projection, err := sqlite.NewSQLiteProjectionBuilder(
    "account-summary",
    db,
    checkpointStore,
    eventStore,
).
    WithSchema(func(ctx context.Context, db *sql.DB) error {
        // Create your read model tables
        _, err := db.Exec(`CREATE TABLE IF NOT EXISTS account_summary (...)`)
        return err
    }).
    On(accountv1.OnAccountOpened(func(ctx context.Context, event *accountv1.AccountOpenedEvent, envelope *domain.EventEnvelope) error {
        tx, _ := sqlite.TxFromContext(ctx)
        // Transaction is automatically managed!
        _, err := tx.Exec("INSERT INTO account_summary ...")
        return err
    })).
    OnReset(func(ctx context.Context, tx *sql.Tx) error {
        _, err := tx.Exec("DELETE FROM account_summary")
        return err
    }).
    Build()
```

### 2. Register with ProjectionManager

```go
import (
    "github.com/plaenen/eventstore/pkg/eventsourcing"
)

// Create projection manager
projectionManager := eventsourcing.NewProjectionManager(
    checkpointStore,  // Tracks projection progress
    eventStore,       // For rebuilds
    eventBus,         // NATS event bus for real-time updates
)

// Register your projections
projectionManager.Register(projection)
```

### 3. Start Listening to NATS Events

```go
// Start projection - it will now listen to NATS events in real-time
if err := projectionManager.Start(ctx, "account-summary"); err != nil {
    log.Fatal(err)
}

// Events published to NATS are automatically processed by the projection!
```

### 4. Complete Flow with Outbox Pattern

```go
// 1. Command handler stores events
err := eventStore.AppendEvents(aggregateID, expectedVersion, events)
// ↓ Events are stored in both `events` and `event_outbox` tables atomically

// 2. Outbox forwarder polls and publishes
forwarder := messaging.NewOutboxForwarder(eventStore, eventBus, config)
forwarder.Start(ctx)
// ↓ Unpublished events are automatically sent to NATS

// 3. Projections receive events from NATS
// ↓ Your projection handlers are automatically called

// 4. Read models are updated
// ✓ Your database tables are updated with the latest state
```

## Key Features

### Automatic Checkpoint Management
The ProjectionManager automatically:
- Tracks which events have been processed
- Saves checkpoints after each event
- Resumes from last checkpoint on restart

### Rebuild Support
You can rebuild projections from scratch:

```go
// Rebuild from event store history
if err := projectionManager.Rebuild(ctx, "account-summary"); err != nil {
    log.Fatal(err)
}
```

This is useful for:
- Initial projection build
- Recovering from errors
- Changing read model schema
- Testing with production data

### Multiple Projections
Register multiple projections that listen to the same events:

```go
projectionManager.Register(accountSummaryProjection)
projectionManager.Register(activityLogProjection)
projectionManager.Register(analyticsProjection)

// Start all projections
projectionManager.Start(ctx, "account-summary")
projectionManager.Start(ctx, "activity-log")
projectionManager.Start(ctx, "analytics")
```

Each projection:
- Has its own checkpoint
- Can be started/stopped independently
- Can be rebuilt independently
- Receives the same events

## Event Flow Guarantees

1. **At-Least-Once Delivery**: Events are guaranteed to be published to NATS (via outbox pattern)
2. **Ordered Processing**: Events are processed in the order they were stored
3. **Idempotency**: Checkpoints ensure events aren't reprocessed after crashes
4. **Durability**: Both events and projections survive restarts

## Running the Example

```bash
# Build the example
go build -o outbox-demo ./examples/cmd/outbox-pattern

# Run it
./outbox-demo
```

The demo shows:
1. Events being stored with outbox entries
2. Outbox forwarder publishing to NATS
3. Automatic event delivery guarantee
4. Clean shutdown with no data loss

## See Also

- **`examples/cmd/projection-nats`** - Full projection example with NATS
- **`examples/cmd/sqlite-projection`** - SQLite projection builder details
- **`pkg/eventsourcing/projection.go`** - ProjectionManager API
- **`pkg/messaging/outbox_forwarder.go`** - Outbox forwarder implementation
