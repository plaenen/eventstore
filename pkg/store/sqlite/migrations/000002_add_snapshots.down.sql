-- Rollback snapshots table

DROP INDEX IF EXISTS idx_snapshots_type;
DROP INDEX IF EXISTS idx_snapshots_created_at;
DROP INDEX IF EXISTS idx_snapshots_aggregate_version;
DROP TABLE IF EXISTS snapshots;
