-- name: GetConstraintOwner :one
SELECT aggregate_id
FROM unique_constraints
WHERE index_name = ? AND value = ?;

-- name: ClaimConstraint :exec
INSERT OR REPLACE INTO unique_constraints (index_name, value, aggregate_id, created_at)
VALUES (?, ?, ?, ?);

-- name: ReleaseConstraint :exec
DELETE FROM unique_constraints
WHERE index_name = ? AND value = ? AND aggregate_id = ?;

-- name: DeleteAllConstraints :exec
DELETE FROM unique_constraints;

-- name: GetAllConstraints :many
SELECT aggregate_id, constraints
FROM events
ORDER BY position ASC;
