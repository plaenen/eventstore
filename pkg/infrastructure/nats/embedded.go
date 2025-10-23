package nats

import (
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

// EmbeddedServer wraps an embedded NATS server for testing.
type EmbeddedServer struct {
	server       *server.Server
	url          string
	shutdownOnce sync.Once
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

// Shutdown stops the embedded server gracefully with a timeout.
// Safe to call multiple times - only the first call will perform shutdown.
func (e *EmbeddedServer) Shutdown() {
	e.shutdownOnce.Do(func() {
		if e.server != nil {
			// Shutdown the server
			e.server.Shutdown()

			// Wait for shutdown with timeout (5 seconds max)
			// This prevents hanging if shutdown doesn't complete
			timeout := time.After(5 * time.Second)
			shutdownDone := make(chan struct{})

			go func() {
				e.server.WaitForShutdown()
				close(shutdownDone)
			}()

			select {
			case <-shutdownDone:
				// Shutdown completed successfully
			case <-timeout:
				// Timeout - log but don't block
				// Note: In production you might want to use a proper logger
				fmt.Println("Warning: NATS server shutdown timed out after 5 seconds")
			}
		}
	})
}

// ConnectToEmbedded connects to an embedded NATS server and returns a client.
// Useful for testing.
func ConnectToEmbedded(srv *EmbeddedServer) (*nats.Conn, error) {
	return nats.Connect(srv.URL())
}
