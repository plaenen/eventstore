package cqrs

import (
	"context"
	"time"

	"github.com/plaenen/eventstore/pkg/domain"
	"github.com/plaenen/eventstore/pkg/eventsourcing"
	"google.golang.org/protobuf/proto"
)

// Transport handles request/reply communication between client and server
// Implementations include NATS, HTTP, gRPC, etc.
type Transport interface {
	// Request sends a request and waits for a response
	// subject: The topic/subject to send to (e.g., "account.v1.AccountCommandService.OpenAccount")
	// request: The command or query message
	// Returns the Response wrapper
	Request(ctx context.Context, subject string, request proto.Message) (*eventsourcing.Response, error)

	// Close cleans up resources
	Close() error
}

// TransportConfig holds common transport configuration
type TransportConfig struct {
	// Timeout for request/reply operations
	Timeout time.Duration

	// MaxReconnectAttempts for connection retry
	MaxReconnectAttempts int

	// ReconnectWait time between reconnection attempts
	ReconnectWait time.Duration

	// MaxRetries for request retry on version conflicts (0 = no retries, default 3)
	MaxRetries int
}

// DefaultTransportConfig returns sensible defaults
func DefaultTransportConfig() *TransportConfig {
	return &TransportConfig{
		Timeout:              30 * time.Second,
		MaxReconnectAttempts: 5,
		ReconnectWait:        2 * time.Second,
		MaxRetries:           3, // Retry up to 3 times on version conflicts
	}
}

// HandlerFunc processes a request and returns a response
// This is used by the server-side to handle incoming requests
type HandlerFunc = eventsourcing.HandlerFunc

// Server handles incoming requests from a transport
type Server interface {
	// RegisterHandler registers a handler for a specific subject
	// subject: The topic/subject to listen on (e.g., "account.v1.AccountCommandService.OpenAccount")
	// handler: The function that processes requests
	RegisterHandler(subject string, handler HandlerFunc) error

	// Start begins listening for requests
	Start(ctx context.Context) error

	// Close stops the server and cleans up resources
	Close() error
}

// ServerConfig holds server configuration
type ServerConfig struct {
	// QueueGroup for load balancing across multiple server instances
	QueueGroup string

	// MaxConcurrent limits concurrent handler executions
	MaxConcurrent int

	// Timeout for handler execution
	HandlerTimeout time.Duration
}

// DefaultServerConfig returns sensible defaults
func DefaultServerConfig() *ServerConfig {
	return &ServerConfig{
		QueueGroup:     "default-handlers",
		MaxConcurrent:  100,
		HandlerTimeout: 30 * time.Second,
	}
}

// CommandHandler processes a command and returns produced events.
type CommandHandler interface {
	// Handle processes the command and returns events produced.
	Handle(ctx context.Context, cmd *domain.CommandEnvelope) ([]*domain.Event, error)
}

// CommandHandlerFunc is a function adapter for CommandHandler.
type CommandHandlerFunc func(ctx context.Context, cmd *domain.CommandEnvelope) ([]*domain.Event, error)

// Handle implements CommandHandler.
func (f CommandHandlerFunc) Handle(ctx context.Context, cmd *domain.CommandEnvelope) ([]*domain.Event, error) {
	return f(ctx, cmd)
}

// CommandBus routes commands to their handlers.
type CommandBus interface {
	// Send sends a command to its handler.
	Send(ctx context.Context, cmd *domain.CommandEnvelope) error

	// Register registers a handler for a command type.
	Register(commandType string, handler CommandHandler)

	// Use adds middleware to the command processing pipeline.
	Use(middleware CommandMiddleware)
}

// CommandMiddleware wraps command handlers with cross-cutting concerns.
type CommandMiddleware func(CommandHandler) CommandHandler
