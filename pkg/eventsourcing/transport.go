package eventsourcing

import (
	"context"
	"time"

	"google.golang.org/protobuf/proto"
)

// Transport handles request/reply communication between client and server
// Implementations include NATS, HTTP, gRPC, etc.
type Transport interface {
	// Request sends a request and waits for a response
	// subject: The topic/subject to send to (e.g., "account.v1.AccountCommandService.OpenAccount")
	// request: The command or query message
	// Returns the Response wrapper
	Request(ctx context.Context, subject string, request proto.Message) (*Response, error)

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
type HandlerFunc func(ctx context.Context, request proto.Message) (*Response, error)

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
