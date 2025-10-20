-- name: GetAggregateVersion :one
SELECT COALESCE(MAX(version), 0) AS version
FROM events
WHERE aggregate_id = ?;

-- name: InsertEvent :exec
INSERT INTO events (
    event_id, aggregate_id, aggregate_type, event_type,
    version, timestamp, data, metadata, constraints, position
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL);

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

-- name: UpdateEventPositions :exec
UPDATE events
SET position = (
    SELECT COUNT(*)
    FROM events e2
    WHERE e2.timestamp < events.timestamp
       OR (e2.timestamp = events.timestamp AND e2.event_id <= events.event_id)
)
WHERE position IS NULL;
