package store

import (
	"time"

	"github.com/plaenen/eventstore/pkg/domain"
)

// EventStore defines the interface for persisting and retrieving events.
type EventStore interface {
	// AppendEvents appends events to an aggregate's stream atomically.
	// Validates unique constraints before persisting.
	// Returns domain.ErrConcurrencyConflict if expectedVersion doesn't match current version.
	// Returns domain.ErrUniqueConstraintViolation if any constraint would be violated.
	AppendEvents(aggregateID string, expectedVersion int64, events []*domain.Event) error

	// AppendEventsIdempotent appends events with command-level idempotency.
	// If commandID was already processed, returns cached result without appending.
	// TTL specifies how long to remember processed commands (default 7 days).
	AppendEventsIdempotent(
		aggregateID string,
		expectedVersion int64,
		events []*domain.Event,
		commandID string,
		ttl time.Duration,
	) (*domain.CommandResult, error)

	// GetCommandResult retrieves the result of a previously processed command.
	// Returns nil if command hasn't been processed or TTL expired.
	GetCommandResult(commandID string) (*domain.CommandResult, error)

	// LoadEvents loads all events for an aggregate starting from afterVersion.
	LoadEvents(aggregateID string, afterVersion int64) ([]*domain.Event, error)

	// LoadAllEvents loads all events from all aggregates for projection building.
	// Returns events in the order they were appended.
	LoadAllEvents(fromPosition int64, limit int) ([]*domain.Event, error)

	// GetAggregateVersion returns the current version of an aggregate.
	// Returns 0 if the aggregate doesn't exist.
	GetAggregateVersion(aggregateID string) (int64, error)

	// CheckUniqueness checks if a value is available for claiming.
	// Returns true if available, false if already claimed.
	// Returns the ownerID if the value is claimed by another aggregate.
	CheckUniqueness(indexName, value string) (available bool, ownerID string, error error)

	// GetConstraintOwner returns the aggregate ID that owns a unique value.
	// Returns empty string if the value is not claimed.
	GetConstraintOwner(indexName, value string) (string, error)

	// RebuildConstraints rebuilds the unique constraint index from the event stream.
	// This is used for recovery or migration scenarios.
	RebuildConstraints() error

	// SeedEvents appends events with special semantics for migrations and bootstrapping.
	//
	// Unlike AppendEvents, SeedEvents:
	//   - Is idempotent (skips events that already exist)
	//   - Generates deterministic IDs for events without IDs
	//   - Optionally skips version checking
	//   - Checks constraint ownership rather than failing on conflicts
	//   - Adds metadata tracking for data lineage
	//
	// All events for a single aggregate are processed in one atomic transaction.
	//
	// Use cases:
	//   - Database migrations (historical data import)
	//   - Bootstrap data (admin users, system configs)
	//   - Test data setup (deterministic test fixtures)
	//
	// Returns SeedResult with counts of saved/skipped/failed events.
	SeedEvents(
		aggregateID string,
		expectedVersion int64,
		events []*domain.Event,
		opts *domain.SeedOptions,
	) (*domain.SeedResult, error)

	// Close closes the event store and releases resources.
	Close() error

	// Outbox methods for transactional outbox pattern

	// LoadUnpublishedEvents loads events from the outbox that haven't been published yet.
	// Returns events ordered by created_at (oldest first) up to the specified limit.
	// Use this in a background worker to poll for events that need publishing.
	LoadUnpublishedEvents(limit int) ([]*domain.EventEnvelope, error)

	// MarkEventsPublished marks events as successfully published in the outbox.
	// This should be called after events are successfully published to the message bus.
	MarkEventsPublished(eventIDs []string) error

	// RecordPublishFailure records a failed publish attempt for an event.
	// Increments the attempts counter and stores the error message for debugging.
	RecordPublishFailure(eventID string, err error) error
}
