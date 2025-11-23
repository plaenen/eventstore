# AUTOINCREMENT Position Assignment - Better Solution

**Question**: Why not use SQLite AUTOINCREMENT for position assignment?

**Answer**: You're absolutely right! This would be cleaner. Here are the options:

---

## Current Approach (What We Fixed)

```go
// In AppendEvents (line 376-386)
InsertEvent(...) // Sets position = NULL

// Later (line 398)
updatePositions(tx) // Runs complex ROW_NUMBER() query
```

**Problems**:
- ❌ Two-step process (INSERT then UPDATE)
- ❌ Complex query with ROW_NUMBER()
- ❌ Extra database round-trip

**Benefits**:
- ✅ Works with current schema
- ✅ Allows reordering/recalculation

---

## Better Option 1: AUTOINCREMENT Column (Simplest)

### Schema Change

```sql
CREATE TABLE events (
    event_id TEXT PRIMARY KEY,
    aggregate_id TEXT NOT NULL,
    ...
    position INTEGER NOT NULL,  -- Remove this
    ...
);

-- Add new auto-increment column
ALTER TABLE events ADD COLUMN position INTEGER PRIMARY KEY AUTOINCREMENT;
```

**Problem**: ❌ SQLite only allows ONE PRIMARY KEY, and we're using `event_id`

### Workaround: Separate Sequence Table

```sql
-- Create a sequence table
CREATE TABLE event_position_seq (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    reserved INTEGER DEFAULT 0
);

-- Trigger to auto-assign position
CREATE TRIGGER auto_assign_position
AFTER INSERT ON events
WHEN NEW.position IS NULL
BEGIN
    INSERT INTO event_position_seq DEFAULT VALUES;
    UPDATE events
    SET position = (SELECT last_insert_rowid())
    WHERE event_id = NEW.event_id;
END;
```

**Pros**:
- ✅ True AUTOINCREMENT behavior
- ✅ No application logic needed

**Cons**:
- ❌ Trigger complexity
- ❌ Still does UPDATE after INSERT
- ❌ Doesn't really save much

---

## ✅ BEST Option: Application-Level Atomic Assignment

Assign position **during INSERT**, not after:

### Code Change

```go
// In AppendEvents, BEFORE inserting events:
func (s *EventStore) AppendEvents(aggregateID string, expectedVersion int64, events []*domain.Event) error {
    // ... existing validation ...

    tx, err := s.db.Begin()
    defer tx.Rollback()

    // ✅ GET NEXT POSITION ATOMICALLY
    var maxPos int64
    err = tx.QueryRow("SELECT COALESCE(MAX(position), 0) FROM events").Scan(&maxPos)
    if err != nil {
        return fmt.Errorf("failed to get max position: %w", err)
    }

    // Assign positions to events
    for i, event := range events {
        event.Position = maxPos + int64(i) + 1  // ← NEW!
    }

    // Insert events WITH positions already set
    for _, event := range events {
        err = queries.InsertEvent(ctx, sqlcgen.InsertEventParams{
            EventID:       event.ID,
            // ... other fields ...
            Position:      event.Position,  // ← Pass actual position, not NULL!
        })
    }

    // ✅ NO NEED FOR updatePositions() anymore!

    return tx.Commit()
}
```

### Query Change

```sql
-- name: InsertEvent :exec
INSERT INTO events (
    event_id, aggregate_id, aggregate_type, event_type,
    version, timestamp, data, metadata, constraints, position
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);  -- ← Pass position directly!
```

### Migration

```sql
-- 000004_atomic_position_assignment.up.sql

-- Position already has UNIQUE constraint from previous migration
-- Just need to make it NOT NULL (already done in 000003)

-- Nothing to change in schema!
-- The change is purely in application code
```

---

## Comparison

| Approach | INSERT/UPDATE | Complexity | Performance | Correctness |
|----------|---------------|------------|-------------|-------------|
| **Current (NULL + UPDATE)** | 2 ops | High (ROW_NUMBER) | Slower | ✅ Fixed |
| **AUTOINCREMENT Column** | N/A | N/A | N/A | ❌ Can't have 2 PKs |
| **Trigger-based** | 2 ops | High (Triggers) | Slower | ✅ Works |
| **✅ Atomic App-Level** | 1 op | Low | Fast | ✅✅✅ Best |

---

## Recommended Implementation

### Step 1: Update InsertEvent Query

```sql
-- pkg/store/sqlite/queries/events.sql

-- name: InsertEvent :exec
INSERT INTO events (
    event_id, aggregate_id, aggregate_type, event_type,
    version, timestamp, data, metadata, constraints, position
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);  -- position is now required param
```

### Step 2: Update AppendEvents Method

```go
// pkg/store/sqlite/eventstore.go

func (s *EventStore) AppendEvents(aggregateID string, expectedVersion int64, events []*domain.Event) error {
    if len(events) == 0 {
        return nil
    }

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

    ctx := context.Background()
    queries := sqlcgen.New(tx)

    // Check optimistic concurrency
    currentVersionRaw, err := queries.GetAggregateVersion(ctx, aggregateID)
    if err != nil {
        return fmt.Errorf("failed to check current version: %w", err)
    }
    currentVersion := currentVersionRaw.(int64)

    if currentVersion != expectedVersion {
        return domain.ErrConcurrencyConflict
    }

    // ✅ NEW: Get next position atomically
    var maxPos int64
    err = tx.QueryRow("SELECT COALESCE(MAX(position), 0) FROM events").Scan(&maxPos)
    if err != nil {
        return fmt.Errorf("failed to get max position: %w", err)
    }

    // ✅ NEW: Assign positions before insertion
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

    // Insert events
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
            Position:      event.Position,  // ✅ NEW: Pass position directly!
        })
        if err != nil {
            return fmt.Errorf("failed to insert event: %w", err)
        }

        // Insert into outbox
        if err := s.insertOutbox(tx, event); err != nil {
            return fmt.Errorf("failed to insert into outbox: %w", err)
        }
    }

    // ✅ REMOVED: No need for updatePositions() anymore!

    return tx.Commit()
}
```

### Step 3: Add Position to domain.Event

```go
// pkg/domain/event.go

type Event struct {
    ID                string
    AggregateID       string
    AggregateType     string
    EventType         string
    Version           int64
    Timestamp         time.Time
    Data              []byte
    Metadata          EventMetadata
    UniqueConstraints []UniqueConstraint
    Position          int64  // ✅ NEW: Position assigned at insertion time
}
```

### Step 4: Update sqlcgen Parameters

```go
// Regenerate sqlc
sqlc generate
```

This will update `InsertEventParams` to include `Position int64`.

---

## Benefits of Atomic Assignment

### ✅ Performance
- **Before**: INSERT (NULL) + UPDATE (ROW_NUMBER subquery)
- **After**: SELECT MAX + INSERT (with value)
- **Savings**: Eliminates complex UPDATE query

### ✅ Simplicity
```sql
-- Before
ROW_NUMBER() OVER (ORDER BY timestamp, event_id)
-- 15 lines of SQL

-- After
SELECT MAX(position) + 1
-- 1 line!
```

### ✅ Correctness
- Position assigned in same transaction as INSERT
- No window for inconsistency
- UNIQUE constraint enforced immediately

### ✅ Maintainability
- Easier to understand
- No complex ROW_NUMBER logic
- Follows "assign ID at creation" pattern

---

## Migration Path

If you want to switch to atomic assignment:

### Option A: Clean Break (Recommended for New Projects)
1. Apply the code changes above
2. Remove `UpdateEventPositions` query
3. Remove `updatePositions()` method

### Option B: Gradual Migration (For Existing Systems)
1. Keep both methods temporarily
2. Use atomic assignment for NEW events
3. Run `UpdateEventPositions` as cleanup job
4. Remove after migration complete

---

## Conclusion

**Your intuition was correct!** The AUTOINCREMENT-style atomic assignment is:
- ✅ Simpler
- ✅ Faster
- ✅ More reliable
- ✅ Industry standard pattern

The ROW_NUMBER() fix we implemented is **good enough** and fixes the critical bug, but **atomic assignment is the ideal solution** for greenfield projects or major refactors.

---

## Recommendation

**For your current situation**:
1. ✅ Keep the ROW_NUMBER() fix (it works and fixes the bug)
2. ✅ Plan to migrate to atomic assignment in next major version
3. ✅ Document both approaches

**For new projects**:
- ✅ Start with atomic assignment from day 1
- ✅ Never set position to NULL
- ✅ Use `SELECT MAX(position) + 1` pattern

---

**The key insight**: SQLite doesn't need traditional AUTOINCREMENT because you can implement it cleanly at the application level with better control and no triggers!
