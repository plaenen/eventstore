-- name: SaveSnapshot :exec
INSERT INTO snapshots (aggregate_id, aggregate_type, version, data, created_at, metadata)
VALUES (?, ?, ?, ?, ?, ?);

-- name: GetLatestSnapshot :one
SELECT aggregate_id, aggregate_type, version, data, created_at, metadata
FROM snapshots
WHERE aggregate_id = ?
ORDER BY version DESC
LIMIT 1;

-- name: GetLatestSnapshotBeforeVersion :one
SELECT aggregate_id, aggregate_type, version, data, created_at, metadata
FROM snapshots
WHERE aggregate_id = ? AND version <= ?
ORDER BY version DESC
LIMIT 1;

-- name: GetSnapshotAtVersion :one
SELECT aggregate_id, aggregate_type, version, data, created_at, metadata
FROM snapshots
WHERE aggregate_id = ? AND version = ?;

-- name: ListSnapshotsForAggregate :many
SELECT aggregate_id, aggregate_type, version, data, created_at, metadata
FROM snapshots
WHERE aggregate_id = ?
ORDER BY version DESC;

-- name: DeleteOldSnapshots :exec
-- Deletes snapshots older than a specific version for an aggregate
DELETE FROM snapshots
WHERE aggregate_id = ? AND version < ?;

-- name: DeleteSnapshotsOlderThan :exec
DELETE FROM snapshots
WHERE created_at < ?;

-- name: CountSnapshotsForAggregate :one
SELECT COUNT(*) FROM snapshots
WHERE aggregate_id = ?;

-- name: GetSnapshotStats :one
SELECT
    COUNT(*) as total_snapshots,
    COUNT(DISTINCT aggregate_id) as unique_aggregates,
    SUM(LENGTH(data)) as total_size_bytes,
    AVG(LENGTH(data)) as avg_size_bytes,
    MIN(created_at) as oldest_snapshot,
    MAX(created_at) as newest_snapshot
FROM snapshots;
