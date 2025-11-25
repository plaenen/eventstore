package cqrs

import (
	"time"
)

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

// Commands are now sent via the generic Transport layer.
// The CommandBus abstraction has been removed in favor of using Transport directly.
// See transport.go for the Transport, Server, and HandlerFunc interfaces.
