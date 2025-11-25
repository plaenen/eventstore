-- Complete initial schema for event sourcing
-- This combines all schema elements with proper CHECK constraints

-- Events table: append-only log of all events
CREATE TABLE IF NOT EXISTS events (
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

-- Index for loading aggregate events
CREATE INDEX IF NOT EXISTS idx_events_aggregate
    ON events(aggregate_id, version);

-- Index for event type filtering
CREATE INDEX IF NOT EXISTS idx_events_type
    ON events(event_type);

-- Index for global event stream
CREATE INDEX IF NOT EXISTS idx_events_position
    ON events(position);

-- Unique constraints table: enforces uniqueness
CREATE TABLE IF NOT EXISTS unique_constraints (
    index_name TEXT NOT NULL CHECK(length(index_name) > 0),
    value TEXT NOT NULL CHECK(length(value) > 0),
    aggregate_id TEXT NOT NULL CHECK(length(aggregate_id) > 0),
    created_at INTEGER NOT NULL,
    PRIMARY KEY (index_name, value)
);

-- Index for looking up constraint owner
CREATE INDEX IF NOT EXISTS idx_constraints_aggregate
    ON unique_constraints(aggregate_id);

-- Processed commands table: idempotency tracking
CREATE TABLE IF NOT EXISTS processed_commands (
    command_id TEXT PRIMARY KEY,
    aggregate_id TEXT NOT NULL,
    processed_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL,
    event_ids TEXT NOT NULL
);

-- Index for command expiration cleanup
CREATE INDEX IF NOT EXISTS idx_commands_expires
    ON processed_commands(expires_at);

-- Snapshots table: stores aggregate state at specific versions
CREATE TABLE IF NOT EXISTS snapshots (
    aggregate_id TEXT NOT NULL,
    aggregate_type TEXT NOT NULL,
    version INTEGER NOT NULL,
    data BLOB NOT NULL,
    created_at INTEGER NOT NULL,
    metadata TEXT,
    PRIMARY KEY (aggregate_id, version)
);

-- Index for finding latest snapshot before a version
CREATE INDEX IF NOT EXISTS idx_snapshots_aggregate_version
    ON snapshots(aggregate_id, version DESC);

-- Index for cleanup queries (finding old snapshots)
CREATE INDEX IF NOT EXISTS idx_snapshots_created_at
    ON snapshots(created_at);

-- Index for aggregate type queries
CREATE INDEX IF NOT EXISTS idx_snapshots_type
    ON snapshots(aggregate_type);
