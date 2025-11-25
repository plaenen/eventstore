-- Rollback initial schema for projection metadata

DROP INDEX IF EXISTS idx_metadata_updated;
DROP INDEX IF EXISTS idx_metadata_key;
DROP INDEX IF EXISTS idx_metadata_projection;
DROP TABLE IF EXISTS projection_metadata;
