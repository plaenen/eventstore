-- Event outbox table: tracks events that need to be published to message bus
-- This enables the transactional outbox pattern for reliable event publishing

CREATE TABLE IF NOT EXISTS event_outbox (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    event_id TEXT NOT NULL CHECK(length(event_id) > 0),
    aggregate_id TEXT NOT NULL CHECK(length(aggregate_id) > 0),
    aggregate_type TEXT NOT NULL CHECK(length(aggregate_type) > 0),
    event_type TEXT NOT NULL CHECK(length(event_type) > 0),
    version INTEGER NOT NULL,
    created_at INTEGER NOT NULL,
    published_at INTEGER DEFAULT NULL,
    attempts INTEGER NOT NULL DEFAULT 0,
    last_error TEXT,
    FOREIGN KEY (event_id) REFERENCES events(event_id)
);

-- Index for finding unpublished events (most common query)
CREATE INDEX IF NOT EXISTS idx_outbox_unpublished
    ON event_outbox(published_at, created_at)
    WHERE published_at IS NULL;

-- Index for aggregate lookups
CREATE INDEX IF NOT EXISTS idx_outbox_aggregate
    ON event_outbox(aggregate_id);

-- Index for event type filtering
CREATE INDEX IF NOT EXISTS idx_outbox_event_type
    ON event_outbox(event_type);

-- Index for published events (for analytics/cleanup)
CREATE INDEX IF NOT EXISTS idx_outbox_published_at
    ON event_outbox(published_at)
    WHERE published_at IS NOT NULL;
