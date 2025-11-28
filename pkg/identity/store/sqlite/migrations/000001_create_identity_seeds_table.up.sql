-- Migration: create_identity_seeds_table
CREATE TABLE IF NOT EXISTS identity_seeds (
    id TEXT PRIMARY KEY,
    seed TEXT NOT NULL,
    created_at DATETIME NOT NULL
);
