-- name: GetMetadata :one
SELECT value
FROM projection_metadata
WHERE projection_name = ? AND key = ?;

-- name: SetMetadata :exec
INSERT INTO projection_metadata (projection_name, key, value, updated_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(projection_name, key) DO UPDATE SET
    value = excluded.value,
    updated_at = excluded.updated_at;

-- name: DeleteMetadata :exec
DELETE FROM projection_metadata
WHERE projection_name = ? AND key = ?;

-- name: GetAllMetadata :many
SELECT key, value
FROM projection_metadata
WHERE projection_name = ?
ORDER BY key;

-- name: DeleteAllMetadata :exec
DELETE FROM projection_metadata
WHERE projection_name = ?;

-- name: ListProjectionsWithMetadata :many
SELECT DISTINCT projection_name
FROM projection_metadata
ORDER BY projection_name;
