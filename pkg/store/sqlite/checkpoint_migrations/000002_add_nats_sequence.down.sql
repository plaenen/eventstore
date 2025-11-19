-- Rollback: Remove NATS sequence and rebuild state columns

-- SQLite doesn't support DROP COLUMN directly, so we recreate the table

-- Create backup table with old schema
CREATE TABLE projection_checkpoints_backup (
    projection_name TEXT PRIMARY KEY,
    position INTEGER NOT NULL,
    last_event_id TEXT NOT NULL,
    updated_at INTEGER NOT NULL
);

-- Copy existing data (excluding new columns)
INSERT INTO projection_checkpoints_backup (projection_name, position, last_event_id, updated_at)
SELECT projection_name, position, last_event_id, updated_at
FROM projection_checkpoints;

-- Drop new table
DROP TABLE projection_checkpoints;

-- Rename backup to original
ALTER TABLE projection_checkpoints_backup
RENAME TO projection_checkpoints;

-- Recreate original index
CREATE INDEX IF NOT EXISTS idx_checkpoints_updated
    ON projection_checkpoints(updated_at);
