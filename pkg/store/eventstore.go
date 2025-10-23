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

	// Close closes the event store and releases resources.
	Close() error
}
