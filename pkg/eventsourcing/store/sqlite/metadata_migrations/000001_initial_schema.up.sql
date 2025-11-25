-- Initial schema for projection metadata

-- Projection metadata table for lifecycle management
CREATE TABLE IF NOT EXISTS projection_metadata (
    projection_name TEXT NOT NULL,
    key TEXT NOT NULL,
    value TEXT NOT NULL,
    updated_at INTEGER NOT NULL,

    PRIMARY KEY (projection_name, key),

    CHECK (length(projection_name) > 0),
    CHECK (length(key) > 0)
);

-- Index for querying all metadata for a projection
CREATE INDEX IF NOT EXISTS idx_metadata_projection
    ON projection_metadata(projection_name);

-- Index for querying by specific key across projections
CREATE INDEX IF NOT EXISTS idx_metadata_key
    ON projection_metadata(key);

-- Index for tracking recent updates
CREATE INDEX IF NOT EXISTS idx_metadata_updated
    ON projection_metadata(updated_at DESC);
