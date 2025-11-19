# Projection Rebuild Optimization - Implementation Plan

## Overview

This plan implements the fix for duplicate event processing during projection rebuilds by:
1. Adding NATS stream sequence tracking to checkpoints
2. Implementing deterministic consumer names
3. Using checkpoint position to resume NATS subscriptions
4. Adding proper delivery policy configuration

## Current State Analysis

### Files Involved
- `pkg/store/checkpoint.go` - Checkpoint interface
- `pkg/store/sqlite/checkpoint_store.go` - SQLite implementation
- `pkg/store/sqlite/queries/checkpoints.sql` - SQL queries (sqlc)
- `pkg/store/sqlite/checkpoint_migrations/` - Migration files
- `pkg/messaging/eventbus.go` - EventBus interface
- `pkg/messaging/nats/eventbus.go` - NATS implementation
- `pkg/eventsourcing/projection.go` - ProjectionManager
- `pkg/domain/event.go` - Event structures

### Current Issues
1. **Random consumer names** (`nats/eventbus.go:159`)
   ```go
   consumerName := fmt.Sprintf("consumer_%s", domain.GenerateID()[:8])
   ```

2. **Checkpoint loaded but ignored** (`projection.go:79-93`)
   ```go
   checkpoint, err := m.checkpointStore.Load(projectionName)
   // ... later ...
   subscription, err := m.eventBus.Subscribe(messaging.EventFilter{}, handler)
   // ❌ Checkpoint not passed to Subscribe!
   ```

3. **No NATS sequence tracking**
   - Checkpoint only stores EventStore position
   - No mapping between EventStore position and NATS stream sequence

4. **No delivery policy options**
   - Always delivers all messages from beginning
   - No way to resume from specific sequence

## Implementation Steps

### Phase 1: Data Model Changes

#### 1.1 Update Checkpoint Structure

**File**: `pkg/store/checkpoint.go`

**Changes**:
```go
// ProjectionCheckpoint tracks the progress of a projection.
type ProjectionCheckpoint struct {
	ProjectionName string

	// EventStore position (used during rebuild)
	Position int64

	// NATS stream sequence (used during normal subscription)
	// This may differ from Position due to outbox batching/ordering
	NATSSequence *int64 // Nullable to support legacy checkpoints

	LastEventID string
	UpdatedAt   time.Time

	// Rebuild state tracking
	IsRebuilding bool
}
```

#### 1.2 Create Database Migration

**File**: `pkg/store/sqlite/checkpoint_migrations/000002_add_nats_sequence.up.sql`

```sql
-- Add NATS sequence tracking and rebuild state
ALTER TABLE projection_checkpoints
ADD COLUMN nats_sequence INTEGER;

ALTER TABLE projection_checkpoints
ADD COLUMN is_rebuilding INTEGER NOT NULL DEFAULT 0;

-- Index for finding rebuilding projections
CREATE INDEX IF NOT EXISTS idx_checkpoints_rebuilding
    ON projection_checkpoints(is_rebuilding)
    WHERE is_rebuilding = 1;
```

**File**: `pkg/store/sqlite/checkpoint_migrations/000002_add_nats_sequence.down.sql`

```sql
-- Rollback migration (SQLite doesn't support DROP COLUMN easily)
-- Instead, recreate table without new columns

CREATE TABLE projection_checkpoints_backup (
    projection_name TEXT PRIMARY KEY,
    position INTEGER NOT NULL,
    last_event_id TEXT NOT NULL,
    updated_at INTEGER NOT NULL
);

INSERT INTO projection_checkpoints_backup
SELECT projection_name, position, last_event_id, updated_at
FROM projection_checkpoints;

DROP TABLE projection_checkpoints;

ALTER TABLE projection_checkpoints_backup
RENAME TO projection_checkpoints;

CREATE INDEX IF NOT EXISTS idx_checkpoints_updated
    ON projection_checkpoints(updated_at);
```

#### 1.3 Update SQL Queries

**File**: `pkg/store/sqlite/queries/checkpoints.sql`

```sql
-- name: SaveCheckpoint :exec
INSERT OR REPLACE INTO projection_checkpoints (
    projection_name,
    position,
    nats_sequence,
    last_event_id,
    updated_at,
    is_rebuilding
)
VALUES (?, ?, ?, ?, ?, ?);

-- name: LoadCheckpoint :one
SELECT
    projection_name,
    position,
    nats_sequence,
    last_event_id,
    updated_at,
    is_rebuilding
FROM projection_checkpoints
WHERE projection_name = ?;

-- name: DeleteCheckpoint :exec
DELETE FROM projection_checkpoints
WHERE projection_name = ?;

-- name: SetRebuildingFlag :exec
UPDATE projection_checkpoints
SET is_rebuilding = ?, updated_at = ?
WHERE projection_name = ?;

-- name: GetRebuildingProjections :many
SELECT projection_name, position, nats_sequence, last_event_id, updated_at, is_rebuilding
FROM projection_checkpoints
WHERE is_rebuilding = 1;
```

#### 1.4 Update CheckpointStore Implementation

**File**: `pkg/store/sqlite/checkpoint_store.go`

**Changes to Save method**:
```go
func (s *CheckpointStore) Save(checkpoint *store.ProjectionCheckpoint) error {
	ctx := context.Background()

	var natsSequence sql.NullInt64
	if checkpoint.NATSSequence != nil {
		natsSequence = sql.NullInt64{
			Int64: *checkpoint.NATSSequence,
			Valid: true,
		}
	}

	err := s.queries.SaveCheckpoint(ctx, sqlcgen.SaveCheckpointParams{
		ProjectionName: checkpoint.ProjectionName,
		Position:       checkpoint.Position,
		NatsSequence:   natsSequence,
		LastEventID:    checkpoint.LastEventID,
		UpdatedAt:      checkpoint.UpdatedAt.Unix(),
		IsRebuilding:   boolToInt(checkpoint.IsRebuilding),
	})

	if err != nil {
		return fmt.Errorf("failed to save checkpoint: %w", err)
	}

	return nil
}
```

**Changes to Load method**:
```go
func (s *CheckpointStore) Load(projectionName string) (*store.ProjectionCheckpoint, error) {
	ctx := context.Background()
	row, err := s.queries.LoadCheckpoint(ctx, projectionName)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("checkpoint not found for projection %s", projectionName)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load checkpoint: %w", err)
	}

	checkpoint := store.ProjectionCheckpoint{
		ProjectionName: row.ProjectionName,
		Position:       row.Position,
		LastEventID:    row.LastEventID,
		UpdatedAt:      time.Unix(row.UpdatedAt, 0),
		IsRebuilding:   intToBool(row.IsRebuilding),
	}

	// Handle nullable NATS sequence
	if row.NatsSequence.Valid {
		seq := row.NatsSequence.Int64
		checkpoint.NATSSequence = &seq
	}

	return &checkpoint, nil
}

// Helper functions
func boolToInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func intToBool(i int64) bool {
	return i != 0
}
```

### Phase 2: EventBus Interface Changes

#### 2.1 Add Subscribe Options Pattern

**File**: `pkg/messaging/eventbus.go`

```go
package messaging

import "github.com/plaenen/eventstore/pkg/domain"

// EventBus defines the interface for publishing and subscribing to events.
type EventBus interface {
	// Publish publishes events to all subscribers.
	Publish(events []*domain.Event) error

	// Subscribe subscribes to events matching the filter.
	// The handler is called for each event.
	Subscribe(filter EventFilter, handler EventHandler, opts ...SubscribeOption) (Subscription, error)

	// Close closes the event bus and releases resources.
	Close() error
}

// EventFilter defines criteria for filtering events.
type EventFilter struct {
	// AggregateTypes filters by aggregate type (empty = all types)
	AggregateTypes []string

	// EventTypes filters by event type (empty = all types)
	EventTypes []string
}

// EventHandler processes an event.
// Return an error to nack the event (it will be retried based on bus configuration).
type EventHandler func(event *domain.EventEnvelope) error

// Subscription represents an active event subscription.
type Subscription interface {
	// Unsubscribe stops receiving events and cleans up resources.
	Unsubscribe() error
}

// SubscribeOption configures subscription behavior.
type SubscribeOption func(*SubscribeConfig)

// SubscribeConfig holds subscription configuration.
type SubscribeConfig struct {
	// ConsumerName is the durable consumer name (if empty, generates random name)
	ConsumerName string

	// StartSequence is the NATS stream sequence to start from (0 = deliver all)
	// This is INCLUSIVE (starts at this sequence, not after)
	StartSequence uint64

	// DeliverAll overrides StartSequence and delivers all messages from beginning
	DeliverAll bool
}

// WithConsumerName sets a deterministic consumer name for durable subscriptions.
func WithConsumerName(name string) SubscribeOption {
	return func(c *SubscribeConfig) {
		c.ConsumerName = name
	}
}

// WithStartSequence sets the stream sequence to start from (inclusive).
// Use this when resuming from a checkpoint.
func WithStartSequence(sequence uint64) SubscribeOption {
	return func(c *SubscribeConfig) {
		c.StartSequence = sequence
		c.DeliverAll = false
	}
}

// WithDeliverAll sets the subscription to deliver all messages from the beginning,
// ignoring any start sequence.
func WithDeliverAll() SubscribeOption {
	return func(c *SubscribeConfig) {
		c.DeliverAll = true
	}
}
```

#### 2.2 Add NATS Metadata to EventEnvelope

**File**: `pkg/domain/event.go`

**Add after EventEnvelope definition**:
```go
// EventEnvelope struct already exists, add this:

// NATSMetadata contains NATS JetStream metadata attached to events.
type NATSMetadata struct {
	// StreamSequence is the sequence number in the NATS stream
	StreamSequence uint64

	// ConsumerSequence is the sequence number for the consumer
	ConsumerSequence uint64

	// Timestamp is when NATS received the message
	Timestamp time.Time

	// NumDelivered is how many times this message has been delivered
	NumDelivered uint64
}

// Update EventEnvelope:
type EventEnvelope struct {
	Event
	Payload proto.Message

	// NATSMetadata is populated when event comes from NATS subscription (nil when from EventStore)
	NATSMetadata *NATSMetadata
}
```

### Phase 3: NATS EventBus Implementation

#### 3.1 Implement Subscribe Options

**File**: `pkg/messaging/nats/eventbus.go`

**Replace Subscribe method** (line 151):
```go
// Subscribe subscribes to events matching the filter with optional configuration.
func (b *EventBus) Subscribe(
	filter messaging.EventFilter,
	handler messaging.EventHandler,
	opts ...messaging.SubscribeOption,
) (messaging.Subscription, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Build configuration from options
	config := &messaging.SubscribeConfig{
		DeliverAll: true, // Default: deliver all
	}
	for _, opt := range opts {
		opt(config)
	}

	// Generate consumer name if not provided
	consumerName := config.ConsumerName
	if consumerName == "" {
		consumerName = fmt.Sprintf("consumer_%s", domain.GenerateID()[:8])
	}

	// Build NATS subject from filter
	subject := b.buildSubject(filter)

	// Create consumer configuration
	consumerConfig := &nats.ConsumerConfig{
		Durable:           consumerName,
		AckPolicy:         nats.AckExplicitPolicy,
		AckWait:           30 * time.Second,
		MaxDeliver:        10,
		InactiveThreshold: 24 * time.Hour, // Prevent auto-deletion
	}

	// Configure delivery policy based on options
	if config.DeliverAll {
		consumerConfig.DeliverPolicy = nats.DeliverAllPolicy
	} else if config.StartSequence > 0 {
		consumerConfig.DeliverPolicy = nats.DeliverByStartSequencePolicy
		consumerConfig.OptStartSeq = config.StartSequence

		// Validate sequence is within stream bounds
		if err := b.validateSequence(config.StartSequence); err != nil {
			return nil, fmt.Errorf("invalid start sequence: %w", err)
		}
	} else {
		// StartSequence == 0 means deliver new messages only
		consumerConfig.DeliverPolicy = nats.DeliverNewPolicy
	}

	// Try to create consumer, update if it already exists
	consumer, err := b.js.AddConsumer(b.streamName, consumerConfig)
	if err != nil {
		// Check if consumer already exists
		if strings.Contains(err.Error(), "already exists") ||
		   strings.Contains(err.Error(), "consumer name already in use") {
			// Consumer exists, update it
			consumer, err = b.js.UpdateConsumer(b.streamName, consumerConfig)
			if err != nil {
				return nil, fmt.Errorf("failed to update existing consumer %s: %w", consumerName, err)
			}
		} else {
			return nil, fmt.Errorf("failed to create consumer %s: %w", consumerName, err)
		}
	}

	// Subscribe to the consumer
	sub, err := consumer.Subscribe(func(msg *nats.Msg) {
		// Deserialize event
		event, err := b.deserializeEvent(msg.Data)
		if err != nil {
			// Log error and nack
			msg.Nak()
			return
		}

		// Get NATS metadata
		meta, err := msg.Metadata()
		if err != nil {
			// Metadata unavailable (shouldn't happen with JetStream)
			msg.Nak()
			return
		}

		// Create event envelope with NATS metadata
		envelope := &domain.EventEnvelope{
			Event: *event,
			NATSMetadata: &domain.NATSMetadata{
				StreamSequence:   meta.Sequence.Stream,
				ConsumerSequence: meta.Sequence.Consumer,
				Timestamp:        meta.Timestamp,
				NumDelivered:     meta.NumDelivered,
			},
		}

		// Call handler
		if err := handler(envelope); err != nil {
			// Handler failed, nack for retry
			msg.Nak()
			return
		}

		// Handler succeeded, ack
		msg.Ack()
	}, nats.ManualAck())

	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to consumer: %w", err)
	}

	// Store subscription
	b.subs[consumerName] = sub

	return &subscription{
		bus:          b,
		sub:          sub,
		consumerName: consumerName,
	}, nil
}

// validateSequence validates that a sequence number is within stream bounds.
func (b *EventBus) validateSequence(sequence uint64) error {
	streamInfo, err := b.js.StreamInfo(b.streamName)
	if err != nil {
		return fmt.Errorf("failed to get stream info: %w", err)
	}

	// Check if sequence is too old (stream purged)
	if sequence < streamInfo.State.FirstSeq && streamInfo.State.FirstSeq > 1 {
		return fmt.Errorf(
			"sequence %d is before stream first sequence %d (stream may have been purged)",
			sequence, streamInfo.State.FirstSeq,
		)
	}

	// Check if sequence is ahead of stream
	if sequence > streamInfo.State.LastSeq {
		// This is OK - means we're caught up, consumer will wait for new messages
		// But log it for visibility
		fmt.Printf("Start sequence %d is ahead of stream last sequence %d (caught up)\n",
			sequence, streamInfo.State.LastSeq)
	}

	return nil
}
```

### Phase 4: ProjectionManager Updates

#### 4.1 Update Start Method

**File**: `pkg/eventsourcing/projection.go`

**Replace Start method** (line 64):
```go
// Start starts a projection consuming events from EventBus (real-time).
func (m *ProjectionManager) Start(ctx context.Context, projectionName string) error {
	m.mu.Lock()
	projection, exists := m.projections[projectionName]
	if !exists {
		m.mu.Unlock()
		return fmt.Errorf("projection %s not found", projectionName)
	}

	// Check if already running
	if _, running := m.running[projectionName]; running {
		m.mu.Unlock()
		return fmt.Errorf("projection %s already running", projectionName)
	}
	m.mu.Unlock()

	// Load checkpoint
	checkpoint, err := m.checkpointStore.Load(projectionName)
	if err != nil {
		// No checkpoint, start from beginning
		checkpoint = &store.ProjectionCheckpoint{
			ProjectionName: projectionName,
			Position:       0,
			NATSSequence:   nil,
			IsRebuilding:   false,
		}
	}

	// Check if interrupted rebuild
	if checkpoint.IsRebuilding {
		return fmt.Errorf(
			"projection %s has interrupted rebuild at position %d - call Rebuild() to resume or complete",
			projectionName, checkpoint.Position,
		)
	}

	// Create cancellable context
	projCtx, cancel := context.WithCancel(ctx)

	// Build subscription options
	subscribeOpts := []messaging.SubscribeOption{
		// Use deterministic consumer name
		messaging.WithConsumerName(fmt.Sprintf("projection_%s", projectionName)),
	}

	// If we have a NATS sequence checkpoint, resume from there
	if checkpoint.NATSSequence != nil {
		// Resume from NEXT sequence (checkpoint is last processed)
		nextSequence := uint64(*checkpoint.NATSSequence + 1)
		subscribeOpts = append(subscribeOpts, messaging.WithStartSequence(nextSequence))
	} else {
		// No NATS checkpoint - deliver all (initial build or legacy checkpoint)
		subscribeOpts = append(subscribeOpts, messaging.WithDeliverAll())
	}

	// Subscribe to event bus
	subscription, err := m.eventBus.Subscribe(
		messaging.EventFilter{},
		func(event *domain.EventEnvelope) error {
			// Process event
			if err := projection.Handle(projCtx, event); err != nil {
				return fmt.Errorf("projection %s failed to handle event: %w", projectionName, err)
			}

			// Update checkpoint
			if event.NATSMetadata != nil {
				// Event from NATS - track stream sequence
				seq := int64(event.NATSMetadata.StreamSequence)
				checkpoint.NATSSequence = &seq
			}

			checkpoint.Position++
			checkpoint.LastEventID = event.Event.ID
			checkpoint.UpdatedAt = domain.Now()

			if err := m.checkpointStore.Save(checkpoint); err != nil {
				return fmt.Errorf("failed to save checkpoint: %w", err)
			}

			return nil
		},
		subscribeOpts...,
	)

	if err != nil {
		cancel()
		return fmt.Errorf("failed to subscribe: %w", err)
	}

	// Mark as running
	m.mu.Lock()
	m.running[projectionName] = cancel
	m.mu.Unlock()

	// Start projection in background
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		<-projCtx.Done()
		subscription.Unsubscribe()
	}()

	return nil
}
```

#### 4.2 Update Rebuild Method

**File**: `pkg/eventsourcing/projection.go`

**Replace Rebuild method** (line 149):
```go
// Rebuild rebuilds a projection from EventStore history (batch processing).
// This is useful for:
// - Initial projection build
// - Recovering from errors
// - Schema changes in read model
func (m *ProjectionManager) Rebuild(ctx context.Context, projectionName string) error {
	m.mu.Lock()
	projection, exists := m.projections[projectionName]
	if !exists {
		m.mu.Unlock()
		return fmt.Errorf("projection %s not found", projectionName)
	}

	// Stop if running
	if cancel, running := m.running[projectionName]; running {
		cancel()
		delete(m.running, projectionName)
	}
	m.mu.Unlock()

	// Set rebuilding flag FIRST (atomic with reset if using transactions)
	if err := m.checkpointStore.Save(&store.ProjectionCheckpoint{
		ProjectionName: projectionName,
		Position:       0,
		NATSSequence:   nil,
		IsRebuilding:   true,
		UpdatedAt:      domain.Now(),
	}); err != nil {
		return fmt.Errorf("failed to set rebuilding flag: %w", err)
	}

	// Reset projection
	if err := projection.Reset(ctx); err != nil {
		return fmt.Errorf("failed to reset projection: %w", err)
	}

	// Replay all events from EventStore
	position := int64(0)
	batchSize := 1000

	for {
		events, err := m.eventStore.LoadAllEvents(position, batchSize)
		if err != nil {
			return fmt.Errorf("failed to load events at position %d: %w", position, err)
		}

		if len(events) == 0 {
			break
		}

		for _, event := range events {
			envelope := &domain.EventEnvelope{
				Event:        *event,
				NATSMetadata: nil, // No NATS metadata during rebuild (from EventStore)
			}

			if err := projection.Handle(ctx, envelope); err != nil {
				return fmt.Errorf("failed to handle event during rebuild: %w", err)
			}
			position++
		}

		// Save checkpoint periodically (still marked as rebuilding)
		if err := m.checkpointStore.Save(&store.ProjectionCheckpoint{
			ProjectionName: projectionName,
			Position:       position,
			NATSSequence:   nil, // NATS sequence not set during rebuild
			LastEventID:    events[len(events)-1].ID,
			UpdatedAt:      domain.Now(),
			IsRebuilding:   true,
		}); err != nil {
			return fmt.Errorf("failed to save checkpoint: %w", err)
		}

		if len(events) < batchSize {
			break
		}
	}

	// IMPORTANT: Wait for outbox to catch up before clearing rebuild flag
	// This ensures NATS stream has all events we just processed
	if err := m.waitForOutboxCatchup(ctx, position); err != nil {
		return fmt.Errorf("outbox catchup failed: %w", err)
	}

	// Clear rebuilding flag (rebuild complete)
	if err := m.checkpointStore.Save(&store.ProjectionCheckpoint{
		ProjectionName: projectionName,
		Position:       position,
		NATSSequence:   nil, // Will be set when subscription starts
		LastEventID:    "", // Will be updated by first NATS message
		UpdatedAt:      domain.Now(),
		IsRebuilding:   false,
	}); err != nil {
		return fmt.Errorf("failed to clear rebuilding flag: %w", err)
	}

	return nil
}

// waitForOutboxCatchup waits for the outbox forwarder to publish events up to the given position.
// This prevents race conditions where rebuild completes but NATS stream is behind.
func (m *ProjectionManager) waitForOutboxCatchup(ctx context.Context, position int64) error {
	// TODO: Implement outbox status checking
	// For now, just wait a reasonable amount of time
	// In production, this should query the event store for unpublished events

	timeout := 30 * time.Second
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		// Check if we have unpublished events
		// This requires adding a method to EventStore interface:
		// HasUnpublishedEvents() (bool, error)

		// For now, just sleep
		time.Sleep(500 * time.Millisecond)

		// In production:
		// hasUnpublished, err := m.eventStore.HasUnpublishedEvents()
		// if err != nil {
		//     return err
		// }
		// if !hasUnpublished {
		//     return nil // All caught up
		// }
	}

	// Timeout - log warning but don't fail
	// The subscription will catch up eventually
	fmt.Printf("Warning: Outbox catchup timeout after %v, proceeding anyway\n", timeout)
	return nil
}
```

### Phase 5: Testing

#### 5.1 Update Unit Tests

**File**: `pkg/eventsourcing/projection_test.go` (create if doesn't exist)

```go
package eventsourcing_test

import (
	"context"
	"testing"

	"github.com/plaenen/eventstore/pkg/domain"
	"github.com/plaenen/eventstore/pkg/eventsourcing"
	"github.com/plaenen/eventstore/pkg/messaging"
	"github.com/plaenen/eventstore/pkg/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// Test that checkpoint is used when starting projection
func TestProjectionManager_Start_UsesCheckpoint(t *testing.T) {
	// Mock dependencies
	checkpointStore := new(MockCheckpointStore)
	eventStore := new(MockEventStore)
	eventBus := new(MockEventBus)

	// Setup: checkpoint at position 10, NATS sequence 15
	natsSeq := int64(15)
	checkpoint := &store.ProjectionCheckpoint{
		ProjectionName: "test-projection",
		Position:       10,
		NATSSequence:   &natsSeq,
	}
	checkpointStore.On("Load", "test-projection").Return(checkpoint, nil)

	// Verify Subscribe is called with correct options
	eventBus.On("Subscribe",
		mock.Anything,
		mock.Anything,
		mock.MatchedBy(func(opts []messaging.SubscribeOption) bool {
			config := &messaging.SubscribeConfig{}
			for _, opt := range opts {
				opt(config)
			}
			// Should use consumer name and start from sequence 16 (checkpoint + 1)
			return config.ConsumerName == "projection_test-projection" &&
			       config.StartSequence == 16
		}),
	).Return(&MockSubscription{}, nil)

	// Create manager and projection
	manager := eventsourcing.NewProjectionManager(checkpointStore, eventStore, eventBus)
	projection := &TestProjection{name: "test-projection"}
	manager.Register(projection)

	// Start projection
	err := manager.Start(context.Background(), "test-projection")
	assert.NoError(t, err)

	// Verify Subscribe was called with correct options
	eventBus.AssertExpectations(t)
}

// Test that first-time start (no checkpoint) uses DeliverAll
func TestProjectionManager_Start_NoCheckpoint_DeliversAll(t *testing.T) {
	// Mock dependencies
	checkpointStore := new(MockCheckpointStore)
	eventStore := new(MockEventStore)
	eventBus := new(MockEventBus)

	// Setup: no checkpoint
	checkpointStore.On("Load", "test-projection").Return(nil, fmt.Errorf("not found"))

	// Verify Subscribe is called with DeliverAll
	eventBus.On("Subscribe",
		mock.Anything,
		mock.Anything,
		mock.MatchedBy(func(opts []messaging.SubscribeOption) bool {
			config := &messaging.SubscribeConfig{}
			for _, opt := range opts {
				opt(config)
			}
			return config.DeliverAll == true
		}),
	).Return(&MockSubscription{}, nil)

	// Create manager and projection
	manager := eventsourcing.NewProjectionManager(checkpointStore, eventStore, eventBus)
	projection := &TestProjection{name: "test-projection"}
	manager.Register(projection)

	// Start projection
	err := manager.Start(context.Background(), "test-projection")
	assert.NoError(t, err)

	eventBus.AssertExpectations(t)
}
```

#### 5.2 Integration Test Script

**File**: `test_rebuild_optimization.sh`

```bash
#!/bin/bash
set -euo pipefail

echo "=== Projection Rebuild Optimization Test ==="

# This script tests that events arriving during rebuild are not reprocessed

# Cleanup
rm -f test_eventstore.db test_projections.db
rm -rf test_nats_data

echo "1. Starting NATS server..."
nats-server -js -sd test_nats_data > nats.log 2>&1 &
NATS_PID=$!
sleep 2

echo "2. Running test program..."
go run ./test/rebuild_test/main.go

echo "3. Cleaning up..."
kill $NATS_PID
rm -f test_eventstore.db test_projections.db nats.log
rm -rf test_nats_data

echo "=== Test Complete ✅ ==="
```

## Migration Path for Existing Systems

### Step 1: Deploy New Code
- All changes are backward compatible
- New nullable `nats_sequence` and `is_rebuilding` columns
- Old checkpoints continue to work (NULL nats_sequence triggers DeliverAll)

### Step 2: Verify Subscriptions
```bash
# Check that consumers use deterministic names
nats consumer ls DOMAIN_EVENTS

# Should see:
# projection_principal-projection
# projection_account-projection
# etc.

# NOT random names like:
# consumer_abc12345
```

### Step 3: Monitor Rebuild
- First rebuild after deployment will still process some duplicates
- Second rebuild will use new checkpoint logic (no duplicates)

### Step 4: Cleanup Old Consumers (Optional)
```bash
# List old random consumers
nats consumer ls DOMAIN_EVENTS | grep "consumer_"

# Delete them
nats consumer rm DOMAIN_EVENTS consumer_abc12345
```

## Rollback Procedure

If issues occur:

1. **Revert Code**: Deploy previous version
2. **Data Migration**: Down migration recreates old table structure
3. **NATS Consumers**: Old random consumers will be created again

**No data loss** - checkpoint position remains valid.

## Success Criteria

✅ **Functional**:
- [ ] Projections start from checkpoint after restart
- [ ] No duplicate processing during rebuild
- [ ] Deterministic consumer names in NATS
- [ ] Rebuild completes successfully
- [ ] Interrupted rebuilds can resume

✅ **Performance**:
- [ ] Rebuild time reduced by ~50% (no reprocessing)
- [ ] No increase in CPU/memory during normal operation
- [ ] Checkpoint save time unchanged

✅ **Reliability**:
- [ ] All tests pass
- [ ] No events skipped or lost
- [ ] Idempotent handlers still work correctly
- [ ] Stream purge detection works

## Timeline

- **Phase 1** (Data Model): 2 hours
- **Phase 2** (EventBus Interface): 1 hour
- **Phase 3** (NATS Implementation): 3 hours
- **Phase 4** (ProjectionManager): 2 hours
- **Phase 5** (Testing): 4 hours

**Total**: ~12 hours (1.5 days)

## Post-Implementation Tasks

1. Update documentation
2. Add monitoring metrics for:
   - Checkpoint lag (EventStore position vs NATS sequence)
   - Rebuild duration
   - Duplicate processing rate (should be 0)
3. Performance benchmarking
4. Production smoke test

## Dependencies

- `github.com/nats-io/nats.go` v1.31.0+
- `github.com/sqlc-dev/sqlc` v1.25.0+ (for regenerating queries)
- Go 1.21+

## Breaking Changes

**None** - All changes are backward compatible:
- New checkpoint fields are nullable
- EventBus.Subscribe accepts variadic options (existing code passes none)
- EventEnvelope.NATSMetadata is nullable (existing code sets nil)
