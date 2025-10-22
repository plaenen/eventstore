-- Migration: Add owner_name column to account_balance
-- Schema evolution example: adding a new field

ALTER TABLE account_balance ADD COLUMN owner_name TEXT;
