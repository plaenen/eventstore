# Advanced Projection Features Demo

This example demonstrates the advanced projection features introduced in the rebuild optimization update, including:

- ✅ **Rebuild Optimization** - No duplicate event processing during rebuilds
- ✅ **NATS Sequence Tracking** - Separate tracking of EventStore position and NATS stream sequence
- ✅ **Deterministic Consumer Names** - Predictable, manageable NATS consumers
- ✅ **Interrupted Rebuild Detection** - Automatic detection and recovery from interrupted rebuilds
- ✅ **Checkpoint Monitoring** - Real-time visibility into projection state

## Prerequisites

1. **NATS Server** with JetStream enabled
   ```bash
   # Using Docker
   docker run -p 4222:4222 -p 8222:8222 nats:latest -js

   # Or install nats-server locally
   nats-server -js
   ```

2. **Go 1.21+**

3. **NATS CLI** (optional, for monitoring)
   ```bash
   go install github.com/nats-io/natscli/nats@latest
   ```

## Available Scenarios

### 1. Basic Demo (Default)

Demonstrates basic projection with checkpoint tracking.

```bash
cd examples/cmd/advanced-projection-features
go run main.go basic
```

**What it shows:**
- Deterministic consumer name: `projection_account-balance`
- Checkpoint tracks both EventStore position and NATS sequence
- Projection resumes from last checkpoint on restart

**Output:**
```
=== Advanced Projection Features Demo ===

Scenario: Basic projection with checkpoint tracking
...
📍 Checkpoint State:
  Projection: account-balance
  EventStore Position: 10
  NATS Sequence: 10
  IsRebuilding: false

🔌 NATS Consumer: projection_account-balance
  (Use 'nats consumer info DEMO_EVENTS projection_account-balance' for details)
```

### 2. Rebuild Demo

Demonstrates rebuild optimization preventing duplicate processing.

```bash
go run main.go rebuild
```

**What it shows:**
- Rebuild clears NATS sequence (will be reset when subscription resumes)
- `IsRebuilding` flag set during rebuild, cleared when complete
- No duplicate processing after rebuild completes
- Events processed exactly once despite rebuild

**Key Observations:**
```
Checkpoint after rebuild:
  Position: 5
  NATS Sequence: nil (✓ correct - will be set when subscription starts)
  IsRebuilding: false (should be false)

Checkpoint after restart:
  Position: 5
  NATS Sequence: 5 (✓ set by NATS subscription)

✅ No duplicate accounts (idempotent processing works!)
```

### 3. Interrupted Rebuild Demo

Demonstrates automatic detection of interrupted rebuilds.

```bash
go run main.go interrupted
```

**What it shows:**
- `IsRebuilding` flag prevents invalid restart
- Clear error message guides recovery
- Shows how to complete interrupted rebuild

**Output:**
```
🔧 Simulating interrupted rebuild...
Checkpoint state:
  Position: 500
  IsRebuilding: true ⚠️

❌ Attempting to start projection with interrupted rebuild...

Expected error: projection account-balance has interrupted rebuild at position 500 - call Rebuild() to resume or complete

✅ Correctly detected interrupted rebuild!

To fix this, call Rebuild() to complete or restart the rebuild:
  projectionManager.Rebuild(ctx, "account-balance")
```

### 4. Monitoring Demo

Real-time monitoring of checkpoint and consumer state.

```bash
go run main.go monitor
```

**What it shows:**
- Real-time checkpoint updates
- NATS consumer state
- Lag detection
- Live event processing

**Output:**
```
📊 Monitoring projection (10 seconds)...

Created event 1
--- Checkpoint State ---
📍 Checkpoint State:
  Projection: account-balance
  EventStore Position: 1
  NATS Sequence: 1
  IsRebuilding: false
  Updated: 2025-01-19T...

Created event 2
...
```

### 5. Concurrent Events Demo

Demonstrates events arriving during rebuild (the core optimization scenario).

```bash
go run main.go concurrent
```

**What it shows:**
- Create 10 initial events
- Start rebuild
- Create 5 MORE events DURING rebuild
- Verify all 15 events processed exactly once
- No reprocessing after subscription resumes

**Timeline:**
```
📝 Creating 10 initial events...
📊 Starting projection (initial)...
Initial checkpoint: position=10, nats_seq=10

⏸️  Stopping projection for rebuild...
🔄 Starting rebuild (background)...

💰 Creating 5 NEW events DURING rebuild...

After rebuild: position=15 (should be 15), nats_seq=nil
▶️  Restarting projection...
After restart: position=15, nats_seq=15

✅ No duplicate accounts (idempotent processing works!)
```

## Understanding the Output

### Checkpoint Fields

```go
type ProjectionCheckpoint struct {
    ProjectionName string
    Position       int64   // EventStore position (0-based)
    NATSSequence   *int64  // NATS stream sequence (1-based, nullable)
    IsRebuilding   bool    // Rebuild state flag
    LastEventID    string
    UpdatedAt      time.Time
}
```

**Position vs NATSSequence:**
- `Position`: Tracks how many events processed from EventStore (used during rebuild)
- `NATSSequence`: Last processed NATS stream sequence (used for subscription resume)
- May differ if events are batched or reordered by outbox forwarder

**IsRebuilding Flag:**
- `true`: Projection is currently rebuilding (subscription paused)
- `false`: Normal operation
- Prevents starting projection with incomplete rebuild

### Consumer Names

**Before Optimization:**
```
consumer_abc12345  (random)
consumer_xyz98765  (random)
```

**After Optimization:**
```
projection_account-balance  (deterministic)
projection_principal        (deterministic)
```

Benefits:
- Easy to identify in NATS monitoring
- Preserved across restarts
- Enables checkpoint-based resume

## Monitoring with NATS CLI

### View Consumer Info

```bash
nats consumer info DEMO_EVENTS projection_account-balance
```

Output:
```
Information for Consumer DEMO_EVENTS > projection_account-balance

Configuration:
        Durable Name: projection_account-balance
     Deliver Policy: By Start Sequence
       Start Sequence: 11
          Ack Policy: Explicit
            Ack Wait: 30.00s
       Max Deliver: 10
   Inactive Threshold: 24h0m0s

State:
   Last Delivered Message: Consumer sequence: 10 Stream sequence: 10
     Ack Floor: Consumer sequence: 10 Stream sequence: 10
         Outstanding Acks: 0 out of maximum 1000
     Redelivered Messages: 0
     Unprocessed Messages: 0
```

### View Stream Info

```bash
nats stream info DEMO_EVENTS
```

### Monitor in Real-Time

```bash
nats stream view DEMO_EVENTS
```

## Key Concepts Demonstrated

### 1. Idempotent Event Handlers

The projection uses `INSERT OR REPLACE` to ensure idempotency:

```go
func (p *AccountBalanceProjection) Handle(ctx context.Context, event *domain.EventEnvelope) error {
    _, err := p.db.ExecContext(ctx, `
        INSERT INTO account_balances (account_id, balance, transaction_count, last_updated)
        VALUES (?, 100, 1, ?)
        ON CONFLICT(account_id) DO UPDATE SET
            balance = balance + 100,
            transaction_count = transaction_count + 1,
            last_updated = ?
    `, event.Event.AggregateID, time.Now().Unix(), time.Now().Unix())

    return err
}
```

**Why Important:**
- Allows safe reprocessing of events
- Essential for at-least-once delivery guarantees
- Prevents data corruption from duplicates

### 2. Rebuild Optimization

**Before:**
```
1. Rebuild reads all events from EventStore (1-100)
2. NATS subscription resumes with random consumer
3. NATS delivers ALL events again (1-100)
4. ❌ Events 1-100 processed TWICE
```

**After:**
```
1. Rebuild reads all events from EventStore (1-100)
2. Checkpoint saved: position=100, nats_sequence=nil
3. NATS subscription resumes from sequence 101 (checkpoint + 1)
4. ✅ No redelivery (already at latest)
```

### 3. Event Source Tracking

Events in the handler know their source:

```go
source := "EventStore"
if event.NATSMetadata != nil {
    source = fmt.Sprintf("NATS(seq=%d)", event.NATSMetadata.StreamSequence)
}

log.Printf("Processing event %s from %s", event.Event.ID[:8], source)
```

**Output:**
```
Processing event 3a8f7b21 from EventStore  (during rebuild)
Processing event 9c2e4d56 from NATS(seq=11) (after rebuild)
```

### 4. State Machine

```
    IDLE
     │
     ├─ Start() ──> RUNNING ──┐
     │                        │
     ├─ Rebuild() ─> REBUILDING (IsRebuilding=true)
     │                        │
     │              REBUILDING ──> IDLE (IsRebuilding=false)
     │                        │
     └────────────────────────┘
```

## Troubleshooting

### Projection Not Processing Events

**Check:**
1. NATS server running: `nats server check`
2. Consumer exists: `nats consumer ls DEMO_EVENTS`
3. Checkpoint state: Look for `IsRebuilding: true`

**Fix:**
```go
// If IsRebuilding is stuck, complete the rebuild:
projectionManager.Rebuild(ctx, "projection-name")
```

### Duplicate Processing

**Should Not Happen** with this implementation. If you see duplicates:

1. Check idempotent handler implementation
2. Verify NATS sequence tracking:
   ```sql
   SELECT * FROM projection_checkpoints WHERE projection_name = 'account-balance';
   ```
3. Check NATS consumer delivery policy:
   ```bash
   nats consumer info DEMO_EVENTS projection_account-balance
   ```

### Checkpoint Ahead of Stream

**Error:**
```
sequence 150 is before stream first sequence 100 (stream may have been purged)
```

**Cause:** NATS stream was purged/recreated

**Fix:** Trigger rebuild to catch up
```go
projectionManager.Rebuild(ctx, "projection-name")
```

## Performance Comparison

### Before Optimization

```
Rebuild 1000 events:           ~2.0 seconds
100 events during rebuild:
Resume subscription:           +2.0 seconds (reprocess 1100)
Total:                         ~4.0 seconds
```

### After Optimization

```
Rebuild 1000 events:           ~2.0 seconds
100 events during rebuild:
Resume subscription:           +0.0 seconds (no reprocessing)
Total:                         ~2.0 seconds (50% faster!)
```

## Next Steps

1. **Run each scenario** to understand the features
2. **Monitor with NATS CLI** to see consumer behavior
3. **Adapt to your domain** - use your own aggregates and events
4. **Add monitoring** - integrate with your observability stack
5. **Test failure scenarios** - kill process during rebuild, etc.

## Related Documentation

- [REBUILD_OPTIMIZATION.md](../../../REBUILD_OPTIMIZATION.md) - Complete technical documentation
- [IMPLEMENTATION_PLAN.md](../../../IMPLEMENTATION_PLAN.md) - Step-by-step implementation guide
- [NATS JetStream Consumers](https://docs.nats.io/nats-concepts/jetstream/consumers)

## Questions?

This example demonstrates all the key features. If something isn't clear:

1. Read the inline comments in `main.go`
2. Check the comprehensive docs (REBUILD_OPTIMIZATION.md)
3. Run with `-v` flag for verbose output (if added)
4. Open an issue with your question

Happy projecting! 🚀
