-- Rollback position uniqueness fix
-- WARNING: This will restore the old schema with nullable, non-unique positions

-- Step 1: Create old schema table
CREATE TABLE events_old (
    event_id TEXT PRIMARY KEY CHECK(length(event_id) > 0),
    aggregate_id TEXT NOT NULL CHECK(length(aggregate_id) > 0),
    aggregate_type TEXT NOT NULL CHECK(length(aggregate_type) > 0),
    event_type TEXT NOT NULL CHECK(length(event_type) > 0),
    version INTEGER NOT NULL,
    timestamp INTEGER NOT NULL,
    data BLOB NOT NULL,
    metadata TEXT NOT NULL,
    constraints TEXT,
    position INTEGER,  -- Back to nullable, non-unique
    UNIQUE (aggregate_id, version)
);

-- Step 2: Migrate data back
INSERT INTO events_old
SELECT event_id, aggregate_id, aggregate_type, event_type,
       version, timestamp, data, metadata, constraints, position,
       aggregate_id, version
FROM events;

-- Step 3: Drop new table and rename old one
DROP TABLE events;
ALTER TABLE events_old RENAME TO events;

-- Step 4: Recreate indexes
CREATE INDEX IF NOT EXISTS idx_events_aggregate
    ON events(aggregate_id, version);

CREATE INDEX IF NOT EXISTS idx_events_type
    ON events(event_type);

CREATE INDEX IF NOT EXISTS idx_events_position
    ON events(position);

-- Step 5: Restore backup if it exists
-- The backup table (events_backup_pre_position_fix) is left intact for manual recovery if needed
