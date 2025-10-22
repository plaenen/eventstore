-- Remove owner_name column
-- Note: SQLite doesn't support DROP COLUMN directly in older versions
-- This is a simplified rollback
CREATE TABLE account_balance_backup (
    account_id TEXT PRIMARY KEY,
    balance TEXT NOT NULL,
    updated_at INTEGER NOT NULL
);

INSERT INTO account_balance_backup SELECT account_id, balance, updated_at FROM account_balance;
DROP TABLE account_balance;
ALTER TABLE account_balance_backup RENAME TO account_balance;
