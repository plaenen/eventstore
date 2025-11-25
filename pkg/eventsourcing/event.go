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

	// Position is the global sequence number in the event store.
	// This is assigned atomically during event persistence and is used for:
	// - Global event ordering across all aggregates
	// - Projection checkpointing and resume
	// - Event replay from a specific point in time
	// Position is guaranteed to be unique, sequential, and never null.
	Position int64

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

	// TenantID is the identifier of the tenant this event belongs to (for multi-tenancy)
	TenantID string

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

// NATSMetadata contains NATS JetStream metadata attached to events.
// This is populated when events are consumed from NATS subscriptions.
type NATSMetadata struct {
	// StreamSequence is the sequence number in the NATS stream
	StreamSequence uint64

	// ConsumerSequence is the sequence number for the consumer
	ConsumerSequence uint64

	// Timestamp is when NATS received the message
	Timestamp time.Time

	// NumDelivered is how many times this message has been delivered
	NumDelivered uint64
}

// EventEnvelope wraps an event with its deserialized payload and optional NATS metadata.
type EventEnvelope struct {
	Event
	Payload proto.Message

	// NATSMetadata is populated when event comes from NATS subscription.
	// It is nil when event comes from EventStore during rebuild.
	NATSMetadata *NATSMetadata
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

// CommandResult represents the result of processing a command idempotently.
// Used by AppendEventsIdempotent to track whether a command was already processed.
type CommandResult struct {
	CommandID        string
	Events           []*Event
	AlreadyProcessed bool
	ProcessedAt      time.Time
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

	// SeedEvents seeds events with special idempotency and versioning semantics.
	// Used for database migrations and bootstrap data. See SeedOptions for configuration.
	SeedEvents(aggregateID string, expectedVersion int64, events []*Event, opts *SeedOptions) (*SeedResult, error)

	// LoadUnpublishedEvents loads events that haven't been published to the event bus yet.
	// Used by the outbox pattern forwarder.
	LoadUnpublishedEvents(limit int) ([]*EventEnvelope, error)

	// MarkEventsPublished marks events as published after successful delivery.
	// Used by the outbox pattern forwarder.
	MarkEventsPublished(eventIDs []string) error

	// RecordPublishFailure records a failed publish attempt for an event.
	// Used by the outbox pattern forwarder for retry logic.
	RecordPublishFailure(eventID string, err error) error

	// Close closes the event store and releases resources.
	Close() error
}

// EventBus defines the interface for publishing and subscribing to events.
type EventBus interface {
	// Publish publishes events to all subscribers.
	Publish(events []*Event) error

	// Subscribe subscribes to events matching the filter with optional configuration.
	// The handler is called for each event.
	// Variadic opts parameter is backward compatible (existing code can pass zero options).
	Subscribe(filter EventFilter, handler EventHandler, opts ...SubscribeOption) (Subscription, error)

	// Close closes the event bus and releases resources.
	Close() error
}

// EventFilter defines criteria for filtering events.
type EventFilter struct {
	// AggregateTypes filters by aggregate type (empty = all types)
	AggregateTypes []string

	// EventTypes filters by event type (empty = all types)
	EventTypes []string
}

// EventHandler processes an event.
// Return an error to nack the event (it will be retried based on bus configuration).
type EventHandler func(event *EventEnvelope) error

// Subscription represents an active event subscription.
type Subscription interface {
	// Unsubscribe stops receiving events and cleans up resources.
	Unsubscribe() error
}

// SubscribeOption configures subscription behavior.
type SubscribeOption func(*SubscribeConfig)

// SubscribeConfig holds subscription configuration.
type SubscribeConfig struct {
	// ConsumerName is the durable consumer name (if empty, generates random name)
	ConsumerName string

	// StartSequence is the NATS stream sequence to start from (0 = deliver all from beginning)
	// This is INCLUSIVE (starts at this sequence, not after)
	StartSequence uint64

	// DeliverAll overrides StartSequence and delivers all messages from beginning
	DeliverAll bool
}

// WithConsumerName sets a deterministic consumer name for durable subscriptions.
// This is critical for maintaining subscription state across restarts.
func WithConsumerName(name string) SubscribeOption {
	return func(c *SubscribeConfig) {
		c.ConsumerName = name
	}
}

// WithStartSequence sets the stream sequence to start from (inclusive).
// Use this when resuming from a checkpoint.
// The sequence is INCLUSIVE, meaning it will start AT this sequence, not after it.
func WithStartSequence(sequence uint64) SubscribeOption {
	return func(c *SubscribeConfig) {
		c.StartSequence = sequence
		c.DeliverAll = false
	}
}

// WithDeliverAll sets the subscription to deliver all messages from the beginning,
// ignoring any start sequence. This is the default behavior when no options are provided.
func WithDeliverAll() SubscribeOption {
	return func(c *SubscribeConfig) {
		c.DeliverAll = true
	}
}
