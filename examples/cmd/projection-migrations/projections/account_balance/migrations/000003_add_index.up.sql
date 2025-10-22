-- Migration: Add index on updated_at for efficient time-based queries

CREATE INDEX IF NOT EXISTS idx_account_balance_updated_at ON account_balance(updated_at);
