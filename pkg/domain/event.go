package domain

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

// EventEnvelope wraps an event with its deserialized payload.
type EventEnvelope struct {
	Event
	Payload proto.Message
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
