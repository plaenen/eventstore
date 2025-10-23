-- name: SaveCheckpoint :exec
INSERT OR REPLACE INTO projection_checkpoints (projection_name, position, last_event_id, updated_at)
VALUES (?, ?, ?, ?);

-- name: LoadCheckpoint :one
SELECT projection_name, position, last_event_id, updated_at
FROM projection_checkpoints
WHERE projection_name = ?;

-- name: DeleteCheckpoint :exec
DELETE FROM projection_checkpoints
WHERE projection_name = ?;
