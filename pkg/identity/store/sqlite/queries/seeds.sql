-- name: SaveSeed :exec
INSERT INTO identity_seeds (id, seed, created_at)
VALUES (?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    seed = excluded.seed,
    created_at = excluded.created_at;

-- name: GetSeed :one
SELECT seed FROM identity_seeds WHERE id = ?;
