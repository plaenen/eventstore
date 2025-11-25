-- name: SaveCheckpoint :exec
INSERT OR REPLACE INTO projection_checkpoints (
    projection_name,
    position,
    nats_sequence,
    last_event_id,
    updated_at,
    is_rebuilding
)
VALUES (?, ?, ?, ?, ?, ?);

-- name: LoadCheckpoint :one
SELECT
    projection_name,
    position,
    nats_sequence,
    last_event_id,
    updated_at,
    is_rebuilding
FROM projection_checkpoints
WHERE projection_name = ?;

-- name: DeleteCheckpoint :exec
DELETE FROM projection_checkpoints
WHERE projection_name = ?;

-- name: SetRebuildingFlag :exec
UPDATE projection_checkpoints
SET is_rebuilding = ?, updated_at = ?
WHERE projection_name = ?;

-- name: GetRebuildingProjections :many
SELECT
    projection_name,
    position,
    nats_sequence,
    last_event_id,
    updated_at,
    is_rebuilding
FROM projection_checkpoints
WHERE is_rebuilding = 1;
