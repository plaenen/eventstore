-- Migration: Drop index on updated_at

DROP INDEX IF EXISTS idx_account_balance_updated_at;
