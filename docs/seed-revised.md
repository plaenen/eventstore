# Event Seeding Design - Revised Proposal

## Overview

Add deterministic, idempotent event seeding to the EventStore for migrations, bootstrapping, and testing scenarios.

## API Design

### Core Function

```go
// SeedEvents appends events with special migration/bootstrap semantics:
//   - Idempotent: skips events that already exist
//   - Deterministic: uses provided event IDs or generates deterministic ones
//   - Historical: accepts pre-set timestamps
//   - Bypass checks: optionally skips version and constraint checks
//
// Returns detailed results about what was saved, skipped, or failed.
func (es *EventStore) SeedEvents(
    aggregateID string,
    expectedVersion int64,
    events []*domain.Event,
    opts *SeedOptions,
) (*SeedResult, error)
```

### Options

```go
type SeedOptions struct {
    // SkipExisting makes seeding idempotent by skipping events with IDs that already exist.
    // Default: true
    SkipExisting bool

    // SkipVersionCheck disables optimistic concurrency checking.
    // When true, expectedVersion is ignored.
    // Default: true
    SkipVersionCheck bool

    // RespectConstraints determines if unique constraints should be enforced.
    // When false, constraint claims/releases are skipped entirely.
    // When true, constraints are checked but won't fail if owner matches aggregateID.
    // Default: false (skip constraints)
    RespectConstraints bool

    // GenerateDeterministicIDs auto-generates IDs for events without IDs set.
    // IDs are generated as: sha256(aggregateID + eventType + version + data)[:16]
    // Default: true
    GenerateDeterministicIDs bool

    // TrackAsSeeded adds metadata to indicate the event was seeded.
    // Adds: {"_seeded": "true", "_seeded_at": "<timestamp>"}
    // Default: true
    TrackAsSeeded bool

    // SeedVersion tracks which version of seed script created this event.
    // Added to metadata as: {"_seed_version": "<version>"}
    // Default: "" (not tracked)
    SeedVersion string

    // BatchSize determines how many events to process in a single transaction.
    // 0 = all events in one transaction
    // Default: 0
    BatchSize int
}

// DefaultSeedOptions returns sensible defaults for seeding
func DefaultSeedOptions() *SeedOptions {
    return &SeedOptions{
        SkipExisting:             true,
        SkipVersionCheck:         true,
        RespectConstraints:       false,
        GenerateDeterministicIDs: true,
        TrackAsSeeded:            true,
        SeedVersion:              "",
        BatchSize:                0,
    }
}
```

### Result Types

```go
type SeedResult struct {
    // Saved is the count of events successfully saved
    Saved int

    // Skipped is the count of events skipped (already existed)
    Skipped int

    // Failed is the count of events that failed to save
    Failed int

    // Errors contains detailed error information for failed events
    Errors []SeedError

    // EventIDs contains all event IDs (generated or provided)
    EventIDs []string
}

type SeedError struct {
    // EventID is the ID of the event that failed (if available)
    EventID string

    // EventType is the type of event that failed
    EventType string

    // Version is the version number that failed
    Version int64

    // Reason is a human-readable explanation
    Reason string

    // Error is the underlying error
    Error error
}

func (r *SeedResult) Success() bool {
    return r.Failed == 0
}

func (r *SeedResult) HasErrors() bool {
    return len(r.Errors) > 0
}
```

## Usage Examples

### Example 1: Bootstrap Admin User

```go
func seedAdminUser(store *sqlite.EventStore) error {
    ctx := context.Background()

    adminID := "bootstrap-admin"
    hashedPassword := bcrypt.HashPassword("changeme")

    events := []*domain.Event{
        {
            ID:            "seed-admin-created-v1",  // Deterministic ID
            AggregateID:   adminID,
            AggregateType: "User",
            EventType:     "UserCreated",
            Version:       1,
            Timestamp:     time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
            Data:          marshalProto(&UserCreated{...}),
            Metadata: domain.EventMetadata{
                "purpose": "bootstrap",
            },
        },
        {
            ID:            "seed-admin-role-v1",
            AggregateID:   adminID,
            AggregateType: "User",
            EventType:     "RoleAssigned",
            Version:       2,
            Timestamp:     time.Date(2025, 1, 1, 0, 0, 1, 0, time.UTC),
            Data:          marshalProto(&RoleAssigned{Role: "admin"}),
        },
    }

    result, err := store.SeedEvents(adminID, 0, events, &SeedOptions{
        SkipExisting:   true,
        SeedVersion:    "v1.0",
        TrackAsSeeded:  true,
    })

    if err != nil {
        return fmt.Errorf("seed failed: %w", err)
    }

    if result.HasErrors() {
        return fmt.Errorf("seed had errors: %v", result.Errors)
    }

    log.Printf("Seeded %d events, skipped %d", result.Saved, result.Skipped)
    return nil
}
```

### Example 2: Historical Data Migration

```go
func migrateHistoricalOrders(store *sqlite.EventStore, orders []LegacyOrder) error {
    ctx := context.Background()

    for _, order := range orders {
        events := convertLegacyOrder(order) // Convert to events

        result, err := store.SeedEvents(
            order.ID,
            0,
            events,
            &SeedOptions{
                SkipExisting:             true,
                SkipVersionCheck:         true,
                GenerateDeterministicIDs: true,  // Auto-generate if missing
                SeedVersion:              "legacy-import-v1",
                BatchSize:                100,   // Process in batches
            },
        )

        if err != nil {
            log.Printf("Failed to seed order %s: %v", order.ID, err)
            continue
        }

        log.Printf("Order %s: saved=%d, skipped=%d", order.ID, result.Saved, result.Skipped)
    }

    return nil
}
```

### Example 3: Test Data Setup

```go
func TestAccountProjection(t *testing.T) {
    store := setupTestStore(t)

    // Seed test data with deterministic IDs
    accountID := "test-account-1"
    events := []*domain.Event{
        {
            ID:            "test-account-created",
            AggregateID:   accountID,
            AggregateType: "Account",
            EventType:     "AccountCreated",
            Version:       1,
            Timestamp:     time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
            Data:          marshalProto(&AccountCreated{...}),
        },
        {
            ID:            "test-deposit-100",
            AggregateID:   accountID,
            AggregateType: "Account",
            EventType:     "MoneyDeposited",
            Version:       2,
            Timestamp:     time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
            Data:          marshalProto(&MoneyDeposited{Amount: 100}),
        },
    }

    result, err := store.SeedEvents(accountID, 0, events, DefaultSeedOptions())
    require.NoError(t, err)
    assert.Equal(t, 2, result.Saved)

    // Re-run should be idempotent
    result, err = store.SeedEvents(accountID, 0, events, DefaultSeedOptions())
    require.NoError(t, err)
    assert.Equal(t, 0, result.Saved)
    assert.Equal(t, 2, result.Skipped)
}
```

## Implementation Considerations

### 1. Event ID Generation

If `GenerateDeterministicIDs` is true and event has no ID:

```go
func generateDeterministicID(event *domain.Event) string {
    h := sha256.New()
    h.Write([]byte(event.AggregateID))
    h.Write([]byte(event.EventType))
    h.Write([]byte(fmt.Sprintf("%d", event.Version)))
    h.Write(event.Data)

    // Use first 16 bytes of hash as hex
    hash := h.Sum(nil)
    return fmt.Sprintf("seed-%x", hash[:16])
}
```

### 2. Idempotency Check

Before inserting, check which events already exist:

```go
func (es *EventStore) checkExistingEvents(ctx context.Context, eventIDs []string) (map[string]bool, error) {
    // Query: SELECT event_id FROM events WHERE event_id IN (...)
    existing := make(map[string]bool)
    // ... populate from query
    return existing, nil
}
```

### 3. Metadata Augmentation

If `TrackAsSeeded` is true:

```go
func augmentSeedMetadata(event *domain.Event, opts *SeedOptions) {
    if event.Metadata == nil {
        event.Metadata = make(domain.EventMetadata)
    }

    event.Metadata["_seeded"] = "true"
    event.Metadata["_seeded_at"] = time.Now().Format(time.RFC3339)

    if opts.SeedVersion != "" {
        event.Metadata["_seed_version"] = opts.SeedVersion
    }
}
```

### 4. Constraint Handling

When `RespectConstraints` is false:
- Don't insert into `unique_constraints` table
- Don't check for constraint conflicts

When `RespectConstraints` is true:
- Check if constraint exists
- If exists and owner != aggregateID, fail
- If exists and owner == aggregateID, skip (idempotent)

### 5. Transaction Batching

If `BatchSize > 0`, process events in batches:

```go
for i := 0; i < len(events); i += batchSize {
    end := min(i+batchSize, len(events))
    batch := events[i:end]

    // Process batch in transaction
    if err := processBatch(batch); err != nil {
        // Record errors but continue
    }
}
```

### 6. Position Assignment

Seeded events should be appended at the current max position:

```go
func (es *EventStore) getNextPosition() (int64, error) {
    var maxPos int64
    err := es.db.QueryRow("SELECT COALESCE(MAX(position), 0) FROM events").Scan(&maxPos)
    return maxPos + 1, err
}
```

## Migration from Normal AppendEvents

Existing code using `AppendEvents` for seeding:

```go
// OLD: Not idempotent, version conflicts
err := store.AppendEvents(id, 0, events)
```

Becomes:

```go
// NEW: Idempotent, no version conflicts
result, err := store.SeedEvents(id, 0, events, DefaultSeedOptions())
```

## Testing Strategy

1. **Idempotency**: Run same seed twice, verify `Skipped` count
2. **Deterministic IDs**: Verify same events generate same IDs
3. **Constraint handling**: Test with/without `RespectConstraints`
4. **Batch processing**: Test large seed sets with batching
5. **Error recovery**: Partial failure scenarios
6. **Metadata tracking**: Verify `_seeded` metadata added

## Security Considerations

1. **Audit Trail**: All seeded events tracked via metadata
2. **Constraint Bypass**: Document that `RespectConstraints=false` bypasses uniqueness
3. **Access Control**: Seeding should require elevated permissions
4. **Validation**: Still validate event data even when seeding

## Performance Considerations

1. **Batch Size**: Tune based on event size and database limits
2. **Index Impact**: Positions still need indexing
3. **Transaction Length**: Large seeds may hold locks
4. **Existence Check**: Querying for existing events adds overhead

## Open Questions

1. Should seeding bypass the `processed_commands` table entirely?
2. Should there be a separate audit log for seed operations?
3. Should we support "upsert" mode for events (update if exists)?
4. Should deterministic ID generation be pluggable (custom hash function)?
5. Should we support seeding across multiple aggregates in one call?

## References

- Original proposal: `docs/seed.md`
- Related: Event sourcing migration patterns
- Related: Database seeding best practices
