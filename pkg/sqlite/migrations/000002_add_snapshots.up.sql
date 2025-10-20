-- Add snapshots table for performance optimization

-- Snapshots table: stores aggregate state at specific versions
CREATE TABLE IF NOT EXISTS snapshots (
    aggregate_id TEXT NOT NULL,
    aggregate_type TEXT NOT NULL,
    version INTEGER NOT NULL,
    data BLOB NOT NULL,              -- Serialized aggregate state (protobuf or JSON)
    created_at INTEGER NOT NULL,
    metadata TEXT,                    -- JSON metadata (snapshot creation info, size, etc.)
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
