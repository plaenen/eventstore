-- Rollback migration - remove CHECK constraints
-- This reverts tables to their original schema without CHECK constraints

-- ============================================================================
-- 1. EVENTS TABLE
-- ============================================================================

-- 1.1. Create events table without CHECK constraints
CREATE TABLE events_new (
    event_id TEXT PRIMARY KEY,
    aggregate_id TEXT NOT NULL,
    aggregate_type TEXT NOT NULL,
    event_type TEXT NOT NULL,
    version INTEGER NOT NULL,
    timestamp INTEGER NOT NULL,
    data BLOB NOT NULL,
    metadata TEXT NOT NULL,
    constraints TEXT,
    position INTEGER,
    UNIQUE (aggregate_id, version)
);

-- 1.2. Copy data back
INSERT INTO events_new (event_id, aggregate_id, aggregate_type, event_type, version, timestamp, data, metadata, constraints, position)
SELECT event_id, aggregate_id, aggregate_type, event_type, version, timestamp, data, metadata, constraints, position
FROM events;

-- 1.3. Drop old table
DROP TABLE events;

-- 1.4. Rename new table
ALTER TABLE events_new RENAME TO events;

-- 1.5. Recreate indexes
CREATE INDEX IF NOT EXISTS idx_events_aggregate
    ON events(aggregate_id, version);

CREATE INDEX IF NOT EXISTS idx_events_type
    ON events(event_type);

CREATE INDEX IF NOT EXISTS idx_events_position
    ON events(position);

-- ============================================================================
-- 2. UNIQUE CONSTRAINTS TABLE
-- ============================================================================

-- 2.1. Create unique_constraints table without CHECK constraints
CREATE TABLE unique_constraints_new (
    index_name TEXT NOT NULL,
    value TEXT NOT NULL,
    aggregate_id TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    PRIMARY KEY (index_name, value)
);

-- 2.2. Copy data back
INSERT INTO unique_constraints_new (index_name, value, aggregate_id, created_at)
SELECT index_name, value, aggregate_id, created_at
FROM unique_constraints;

-- 2.3. Drop old table
DROP TABLE unique_constraints;

-- 2.4. Rename new table
ALTER TABLE unique_constraints_new RENAME TO unique_constraints;

-- 2.5. Recreate index
CREATE INDEX IF NOT EXISTS idx_constraints_aggregate
    ON unique_constraints(aggregate_id);
