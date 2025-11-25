-- Rollback initial schema

DROP INDEX IF EXISTS idx_commands_expires;
DROP TABLE IF EXISTS processed_commands;

DROP INDEX IF EXISTS idx_constraints_aggregate;
DROP TABLE IF EXISTS unique_constraints;

DROP INDEX IF EXISTS idx_events_position;
DROP INDEX IF EXISTS idx_events_type;
DROP INDEX IF EXISTS idx_events_aggregate;
DROP TABLE IF EXISTS events;
