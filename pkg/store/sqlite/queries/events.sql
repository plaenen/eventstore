-- name: GetAggregateVersion :one
SELECT COALESCE(MAX(version), 0) AS version
FROM events
WHERE aggregate_id = ?;

-- name: InsertEvent :exec
-- Position is now assigned atomically at insertion time, not calculated afterward
INSERT INTO events (
    event_id, aggregate_id, aggregate_type, event_type,
    version, timestamp, data, metadata, constraints, position
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: LoadEventByID :one
SELECT event_id, aggregate_id, aggregate_type, event_type,
       version, timestamp, data, metadata, constraints
FROM events
WHERE event_id = ?;

-- name: LoadEvents :many
SELECT event_id, aggregate_id, aggregate_type, event_type,
       version, timestamp, data, metadata, constraints
FROM events
WHERE aggregate_id = ? AND version > ?
ORDER BY version ASC;

-- name: LoadAllEvents :many
SELECT event_id, aggregate_id, aggregate_type, event_type,
       version, timestamp, data, metadata, constraints, position
FROM events
WHERE position >= ?
ORDER BY position ASC
LIMIT ?;

-- UpdateEventPositions is no longer needed - positions are assigned atomically during INSERT
-- See AppendEvents method which uses: SELECT MAX(position) + 1
