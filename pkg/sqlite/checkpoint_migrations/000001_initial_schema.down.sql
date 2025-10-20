-- Rollback initial schema for projection checkpoints

DROP INDEX IF EXISTS idx_checkpoints_updated;
DROP TABLE IF EXISTS projection_checkpoints;
