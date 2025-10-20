-- name: GetProcessedCommand :one
SELECT aggregate_id, processed_at, event_ids
FROM processed_commands
WHERE command_id = ? AND expires_at > ?;

-- name: CheckCommandExists :one
SELECT command_id
FROM processed_commands
WHERE command_id = ?;

-- name: InsertProcessedCommand :exec
INSERT INTO processed_commands (command_id, aggregate_id, processed_at, expires_at, event_ids)
VALUES (?, ?, ?, ?, ?);

-- name: CleanExpiredCommands :execrows
DELETE FROM processed_commands
WHERE expires_at < ?;
