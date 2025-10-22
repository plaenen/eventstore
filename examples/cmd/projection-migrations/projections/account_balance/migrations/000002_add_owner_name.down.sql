-- Migration: Remove owner_name column
-- SQLite doesn't support DROP COLUMN, so we need to recreate the table

BEGIN TRANSACTION;

-- Create new table without owner_name
CREATE TABLE account_balance_new (
    account_id TEXT PRIMARY KEY,
    balance INTEGER NOT NULL DEFAULT 0,
    updated_at INTEGER NOT NULL
);

-- Copy data
INSERT INTO account_balance_new (account_id, balance, updated_at)
SELECT account_id, balance, updated_at FROM account_balance;

-- Replace old table
DROP TABLE account_balance;
ALTER TABLE account_balance_new RENAME TO account_balance;

COMMIT;
