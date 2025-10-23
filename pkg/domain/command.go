package domain

import (
	"time"

	"google.golang.org/protobuf/proto"
)

// Command represents an intention to change the system state.
type Command interface {
	// ID returns the unique identifier for this command.
	// Must be provided by the client for idempotency.
	ID() string

	// AggregateID returns the ID of the aggregate this command targets.
	AggregateID() string

	// CommandType returns the fully qualified type name of the command.
	CommandType() string
}

// CommandMetadata contains contextual information about a command.
type CommandMetadata struct {
	// CommandID is the unique identifier for this command (for idempotency)
	CommandID string

	// CorrelationID is used to trace related commands and events
	CorrelationID string

	// PrincipalID is the identifier of the principal executing this command
	PrincipalID string

	// TenantID is the identifier of the tenant this command belongs to (for multi-tenancy)
	TenantID string

	// Timestamp is when the command was created
	Timestamp time.Time

	// Custom allows for application-specific metadata
	Custom map[string]string
}

// CommandEnvelope wraps a command with its metadata.
type CommandEnvelope struct {
	Command  proto.Message
	Metadata CommandMetadata
}

// CommandResult represents the result of processing a command.
type CommandResult struct {
	// CommandID is the ID of the command that was processed
	CommandID string

	// Events are the events produced by the command
	Events []*Event

	// AlreadyProcessed indicates if this was a duplicate command
	AlreadyProcessed bool

	// ProcessedAt is when the command was originally processed
	ProcessedAt time.Time
}
