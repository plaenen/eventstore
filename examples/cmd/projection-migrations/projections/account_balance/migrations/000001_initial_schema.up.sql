-- Migration: Initial schema for account_balance projection
-- Creates the base table for tracking account balances

CREATE TABLE IF NOT EXISTS account_balance (
    account_id TEXT PRIMARY KEY,
    balance INTEGER NOT NULL DEFAULT 0,
    updated_at INTEGER NOT NULL
);
