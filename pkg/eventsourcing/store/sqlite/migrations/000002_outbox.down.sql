-- Rollback outbox table

DROP INDEX IF EXISTS idx_outbox_published_at;
DROP INDEX IF EXISTS idx_outbox_event_type;
DROP INDEX IF EXISTS idx_outbox_aggregate;
DROP INDEX IF EXISTS idx_outbox_unpublished;
DROP TABLE IF EXISTS event_outbox;
