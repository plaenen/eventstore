package eventsourcing

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"
)

// Event represents a domain event that has occurred in the system.
// Events are immutable facts about state changes.
type Event struct {
	// ID is the unique identifier for this event (deterministic)
	ID string

	// AggregateID is the identifier of the aggregate this event belongs to
	AggregateID string

	// AggregateType is the type name of the aggregate (e.g., "Account", "Order")
	AggregateType string

	// EventType is the fully qualified type name of the event (e.g., "example.AccountCreated")
	EventType string

	// Version is the version number of the aggregate after applying this event
	Version int64

	// Timestamp is when the event was created
	Timestamp time.Time

	// Data is the serialized protobuf payload of the event
	Data []byte

	// Metadata contains additional contextual information
	Metadata EventMetadata

	// UniqueConstraints are the unique constraints claimed or released by this event.
	// These are validated atomically with event persistence.
	UniqueConstraints []UniqueConstraint
}

// EventMetadata contains contextual information about an event.
type EventMetadata struct {
	// CausationID is the ID of the command that caused this event
	CausationID string

	// CorrelationID is used to trace related events across aggregates
	CorrelationID string

	// PrincipalID is the identifier of the principal (user, service, system) who triggered this event
	PrincipalID string

	// Custom allows for application-specific metadata
	Custom map[string]string
}

// UniqueConstraint represents a uniqueness claim or release on a value.
type UniqueConstraint struct {
	// IndexName identifies this constraint (e.g., "user_email", "account_number")
	IndexName string

	// Value is the unique value being claimed or released (e.g., "user@example.com")
	Value string

	// Operation specifies whether to "claim" or "release" this value
	Operation ConstraintOperation
}

// ConstraintOperation defines operations on unique constraints.
type ConstraintOperation string

const (
	// ConstraintClaim claims a unique value for this aggregate
	ConstraintClaim ConstraintOperation = "claim"

	// ConstraintRelease releases a unique value previously claimed
	ConstraintRelease ConstraintOperation = "release"
)

// EventEnvelope wraps an event with its deserialized payload.
type EventEnvelope struct {
	Event
	Payload proto.Message
}

// EventStore defines the interface for persisting and retrieving events.
type EventStore interface {
	// AppendEvents appends events to an aggregate's stream atomically.
	// Validates unique constraints before persisting.
	// Returns ErrConcurrencyConflict if expectedVersion doesn't match current version.
	// Returns ErrUniqueConstraintViolation if any constraint would be violated.
	AppendEvents(aggregateID string, expectedVersion int64, events []*Event) error

	// AppendEventsIdempotent appends events with command-level idempotency.
	// If commandID was already processed, returns cached result without appending.
	// TTL specifies how long to remember processed commands (default 7 days).
	AppendEventsIdempotent(
		aggregateID string,
		expectedVersion int64,
		events []*Event,
		commandID string,
		ttl time.Duration,
	) (*CommandResult, error)

	// GetCommandResult retrieves the result of a previously processed command.
	// Returns nil if command hasn't been processed or TTL expired.
	GetCommandResult(commandID string) (*CommandResult, error)

	// LoadEvents loads all events for an aggregate starting from afterVersion.
	LoadEvents(aggregateID string, afterVersion int64) ([]*Event, error)

	// LoadAllEvents loads all events from all aggregates for projection building.
	// Returns events in the order they were appended.
	LoadAllEvents(fromPosition int64, limit int) ([]*Event, error)

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

// EventBus defines the interface for publishing and subscribing to events.
type EventBus interface {
	// Publish publishes events to all subscribers.
	Publish(events []*Event) error

	// Subscribe subscribes to events matching the filter.
	// The handler is called for each event.
	Subscribe(filter EventFilter, handler EventHandler) (Subscription, error)

	// Close closes the event bus and releases resources.
	Close() error
}

// EventFilter defines criteria for filtering events.
type EventFilter struct {
	// AggregateTypes filters by aggregate type (empty = all types)
	AggregateTypes []string

	// EventTypes filters by event type (empty = all types)
	EventTypes []string

	// FromPosition starts consuming from this position (0 = from beginning)
	FromPosition int64
}

// EventHandler processes an event.
// Return an error to nack the event (it will be retried based on bus configuration).
type EventHandler func(event *EventEnvelope) error

// Subscription represents an active event subscription.
type Subscription interface {
	// Unsubscribe stops receiving events and cleans up resources.
	Unsubscribe() error
}

// GenerateDeterministicEventID generates a deterministic event ID from command context.
// This ensures the same command always produces the same event IDs (idempotency).
func GenerateDeterministicEventID(commandID, aggregateID string, sequence int) string {
	h := sha256.New()
	h.Write([]byte(fmt.Sprintf("%s:%s:%d", commandID, aggregateID, sequence)))
	return hex.EncodeToString(h.Sum(nil))[:32] // Use first 32 chars (128 bits)
}

// DefaultCommandTTL is the default time to remember processed commands.
const DefaultCommandTTL = 7 * 24 * time.Hour // 7 days
