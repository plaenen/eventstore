-- Migration to add CHECK constraints to prevent empty string values
-- This ensures critical fields are never empty strings (SQLite's NOT NULL allows "")
--
-- SCOPE: Infrastructure / Global Event Store Layer
-- This migration applies database-level constraints to the event store schema.
-- These are foundational data integrity constraints that protect the entire system.
--
-- Fields protected:
--
--   events table (SCOPE: Global event store):
--     - event_id: Unique identifier for each event (global scope)
--     - aggregate_id: Identifier linking events to aggregates (aggregate scope)
--     - aggregate_type: Type classification of aggregates (domain scope)
--     - event_type: Type classification of events (domain scope)
--
--   unique_constraints table (SCOPE: Domain constraints):
--     - index_name: Constraint identifier (domain scope)
--     - value: The unique value being protected (domain/aggregate scope)
--     - aggregate_id: Owner of the constraint (aggregate scope)
--
-- SQLite doesn't support ALTER TABLE ADD CONSTRAINT directly on existing tables,
-- so we need to recreate the tables with the constraints

-- ============================================================================
-- 1. EVENTS TABLE
-- ============================================================================

-- 1.1. Create new events table with CHECK constraints
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
    position INTEGER,
    UNIQUE (aggregate_id, version)
);

-- 1.2. Copy existing data (filtering out bad rows with empty required fields)
INSERT INTO events_new (event_id, aggregate_id, aggregate_type, event_type, version, timestamp, data, metadata, constraints, position)
SELECT event_id, aggregate_id, aggregate_type, event_type, version, timestamp, data, metadata, constraints, position
FROM events
WHERE length(event_id) > 0
  AND length(aggregate_id) > 0
  AND length(aggregate_type) > 0
  AND length(event_type) > 0;

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

-- 2.1. Create new unique_constraints table with CHECK constraints
CREATE TABLE unique_constraints_new (
    index_name TEXT NOT NULL CHECK(length(index_name) > 0),
    value TEXT NOT NULL CHECK(length(value) > 0),
    aggregate_id TEXT NOT NULL CHECK(length(aggregate_id) > 0),
    created_at INTEGER NOT NULL,
    PRIMARY KEY (index_name, value)
);

-- 2.2. Copy existing data (filtering out bad rows with empty fields)
INSERT INTO unique_constraints_new (index_name, value, aggregate_id, created_at)
SELECT index_name, value, aggregate_id, created_at
FROM unique_constraints
WHERE length(index_name) > 0
  AND length(value) > 0
  AND length(aggregate_id) > 0;

-- 2.3. Drop old table
DROP TABLE unique_constraints;

-- 2.4. Rename new table
ALTER TABLE unique_constraints_new RENAME TO unique_constraints;

-- 2.5. Recreate index
CREATE INDEX IF NOT EXISTS idx_constraints_aggregate
    ON unique_constraints(aggregate_id);

-- ============================================================================
-- NOTES
-- ============================================================================
-- This migration will DROP any rows that have empty strings in required fields.
-- This is intentional as those entries are invalid and would cause the constraints to fail.
--
-- Protected fields:
--   events:
--     - event_id (must not be empty)
--     - aggregate_id (must not be empty)
--     - aggregate_type (must not be empty)
--     - event_type (must not be empty)
--
--   unique_constraints:
--     - index_name (must not be empty)
--     - value (must not be empty)
--     - aggregate_id (must not be empty)
