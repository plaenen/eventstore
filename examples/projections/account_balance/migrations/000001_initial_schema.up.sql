-- Initial schema for account balance projection
CREATE TABLE IF NOT EXISTS account_balance (
    account_id TEXT PRIMARY KEY,
    balance TEXT NOT NULL,
    updated_at INTEGER NOT NULL
);
