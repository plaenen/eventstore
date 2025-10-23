-- Initial schema for projection checkpoints

-- Projection checkpoints table
CREATE TABLE IF NOT EXISTS projection_checkpoints (
    projection_name TEXT PRIMARY KEY,
    position INTEGER NOT NULL,
    last_event_id TEXT NOT NULL,
    updated_at INTEGER NOT NULL
);

-- Index for checkpoint updates
CREATE INDEX IF NOT EXISTS idx_checkpoints_updated
    ON projection_checkpoints(updated_at);
