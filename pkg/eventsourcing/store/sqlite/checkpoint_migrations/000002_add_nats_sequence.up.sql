-- Add NATS sequence tracking and rebuild state

-- Add NATS stream sequence column (nullable for backward compatibility)
ALTER TABLE projection_checkpoints
ADD COLUMN nats_sequence INTEGER;

-- Add rebuild state tracking
ALTER TABLE projection_checkpoints
ADD COLUMN is_rebuilding INTEGER NOT NULL DEFAULT 0;

-- Index for finding rebuilding projections
CREATE INDEX IF NOT EXISTS idx_checkpoints_rebuilding
    ON projection_checkpoints(is_rebuilding)
    WHERE is_rebuilding = 1;
