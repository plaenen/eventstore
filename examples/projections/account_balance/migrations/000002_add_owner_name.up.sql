-- Add owner_name column to track account owner
ALTER TABLE account_balance ADD COLUMN owner_name TEXT NOT NULL DEFAULT '';
