-- Fix duplicate positions and add constraints
-- This migration fixes the critical position assignment bug

-- Step 1: Backup existing events table
CREATE TABLE events_backup_pre_position_fix AS SELECT * FROM events;

-- Step 2: Recalculate ALL positions correctly using ROW_NUMBER()
-- This ensures sequential, unique positions based on timestamp and event_id
UPDATE events
SET position = (
    SELECT row_num
    FROM (
        SELECT event_id,
               ROW_NUMBER() OVER (ORDER BY timestamp ASC, event_id ASC) as row_num
        FROM events
    ) numbered
    WHERE numbered.event_id = events.event_id
);

-- Step 3: Verify no NULL positions remain
-- This should not fail unless there's a data integrity issue
UPDATE events
SET position = (
    SELECT MAX(position) + 1 FROM events
)
WHERE position IS NULL;

-- Step 4: Create new events table with proper constraints
CREATE TABLE events_new (
    event_id TEXT PRIMARY KEY CHECK(length(event_id) > 0),
    aggregate_id TEXT NOT NULL CHECK(length(aggregate_id) > 0),
    aggregate_type TEXT NOT NULL CHECK(length(aggregate_type) > 0),
    event_type TEXT NOT NULL CHECK(length(event_type) > 0),
    version INTEGER NOT NULL,
    timestamp INTEGER NOT NULL,
    data BLOB NOT NULL,
    metadata TEXT NOT NULL,
    constraints TEXT,
    position INTEGER NOT NULL UNIQUE,  -- NOW WITH NOT NULL AND UNIQUE!
    UNIQUE (aggregate_id, version)
);

-- Step 5: Migrate data to new table
INSERT INTO events_new
SELECT event_id, aggregate_id, aggregate_type, event_type,
       version, timestamp, data, metadata, constraints, position
FROM events
ORDER BY position ASC;

-- Step 6: Drop old table and rename new one
DROP TABLE events;
ALTER TABLE events_new RENAME TO events;

-- Step 7: Recreate indexes
CREATE INDEX IF NOT EXISTS idx_events_aggregate
    ON events(aggregate_id, version);

CREATE INDEX IF NOT EXISTS idx_events_type
    ON events(event_type);

CREATE INDEX IF NOT EXISTS idx_events_position
    ON events(position);

CREATE INDEX IF NOT EXISTS idx_events_timestamp
    ON events(timestamp);

-- Step 8: Verify integrity
-- Count unique positions - should equal total events
CREATE TEMP TABLE position_check AS
SELECT
    (SELECT COUNT(*) FROM events) as total_events,
    (SELECT COUNT(DISTINCT position) FROM events) as unique_positions,
    (SELECT MIN(position) FROM events) as min_position,
    (SELECT MAX(position) FROM events) as max_position;

-- This will fail the migration if positions are not unique
-- Note: SQLite doesn't have a great way to fail migrations,
-- but the UNIQUE constraint will prevent insertions if there are duplicates
