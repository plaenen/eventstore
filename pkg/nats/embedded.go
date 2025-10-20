package nats

import (
	"fmt"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

// EmbeddedServer wraps an embedded NATS server for testing.
type EmbeddedServer struct {
	server *server.Server
	url    string
}

// StartEmbeddedServer starts an embedded NATS server with JetStream enabled.
// Perfect for testing without external dependencies.
func StartEmbeddedServer() (*EmbeddedServer, error) {
	opts := &server.Options{
		Host:      "127.0.0.1",
		Port:      -1, // Random port
		JetStream: true,
		StoreDir:  "", // Use temp directory
	}

	s, err := server.NewServer(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create embedded server: %w", err)
	}

	// Start server in goroutine
	go s.Start()

	// Wait for server to be ready
	if !s.ReadyForConnections(5e9) { // 5 seconds
		return nil, fmt.Errorf("server not ready")
	}

	url := s.ClientURL()

	return &EmbeddedServer{
		server: s,
		url:    url,
	}, nil
}

// URL returns the connection URL for the embedded server.
func (e *EmbeddedServer) URL() string {
	return e.url
}

// Shutdown stops the embedded server.
func (e *EmbeddedServer) Shutdown() {
	if e.server != nil {
		e.server.Shutdown()
		e.server.WaitForShutdown()
	}
}

// NewEmbeddedEventBus creates an event bus with an embedded NATS server.
// This is a convenience function for testing.
func NewEmbeddedEventBus() (*EventBus, *EmbeddedServer, error) {
	// Start embedded server
	srv, err := StartEmbeddedServer()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to start embedded server: %w", err)
	}

	// Create event bus
	config := DefaultConfig()
	config.URL = srv.URL()

	bus, err := NewEventBus(config)
	if err != nil {
		srv.Shutdown()
		return nil, nil, fmt.Errorf("failed to create event bus: %w", err)
	}

	return bus, srv, nil
}

// TestConfig returns a config suitable for testing with embedded NATS.
func TestConfig(serverURL string) Config {
	return Config{
		URL:            serverURL,
		StreamName:     "TEST_EVENTS",
		StreamSubjects: []string{"events.>"},
		MaxAge:         time.Minute,     // 1 minute for tests
		MaxBytes:       10 * 1024 * 1024, // 10 MB for tests
	}
}

// ConnectToEmbedded connects to an embedded NATS server and returns a client.
// Useful for testing.
func ConnectToEmbedded(srv *EmbeddedServer) (*nats.Conn, error) {
	return nats.Connect(srv.URL())
}
