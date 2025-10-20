package eventsourcing

import (
	"context"
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

// CommandHandler processes a command and returns produced events.
type CommandHandler interface {
	// Handle processes the command and returns events produced.
	// Returns ErrCommandAlreadyProcessed if command was already handled (idempotent).
	Handle(ctx context.Context, cmd *CommandEnvelope) ([]*Event, error)
}

// CommandHandlerFunc is a function adapter for CommandHandler.
type CommandHandlerFunc func(ctx context.Context, cmd *CommandEnvelope) ([]*Event, error)

// Handle implements CommandHandler.
func (f CommandHandlerFunc) Handle(ctx context.Context, cmd *CommandEnvelope) ([]*Event, error) {
	return f(ctx, cmd)
}

// CommandBus routes commands to their handlers.
type CommandBus interface {
	// Send sends a command to its handler.
	// Automatically handles idempotency via command ID.
	Send(ctx context.Context, cmd *CommandEnvelope) error

	// Register registers a handler for a command type.
	Register(commandType string, handler CommandHandler)

	// Use adds middleware to the command processing pipeline.
	Use(middleware CommandMiddleware)
}

// CommandMiddleware wraps command handlers with cross-cutting concerns.
type CommandMiddleware func(CommandHandler) CommandHandler

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
