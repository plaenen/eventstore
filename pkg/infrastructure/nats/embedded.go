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

// Option is a functional option for configuring the embedded NATS server.
type Option func(*server.Options)

// WithPort sets a specific port for the NATS server.
// Use -1 for a random available port (recommended for testing).
func WithPort(port int) Option {
	return func(opts *server.Options) {
		opts.Port = port
	}
}

// WithHost sets the host address for the NATS server.
// Default is "127.0.0.1".
func WithHost(host string) Option {
	return func(opts *server.Options) {
		opts.Host = host
	}
}

// WithStoreDir sets the directory for JetStream storage.
// Empty string uses a temporary directory (default).
func WithStoreDir(dir string) Option {
	return func(opts *server.Options) {
		opts.StoreDir = dir
	}
}

// WithJetStream enables or disables JetStream.
// Default is enabled (true).
func WithJetStream(enabled bool) Option {
	return func(opts *server.Options) {
		opts.JetStream = enabled
	}
}

// WithMaxPayload sets the maximum message payload size.
// Default is 1MB.
func WithMaxPayload(bytes int32) Option {
	return func(opts *server.Options) {
		opts.MaxPayload = bytes
	}
}

// WithWriteDeadline sets the write deadline for connections.
// Default is 10 seconds.
func WithWriteDeadline(duration time.Duration) Option {
	return func(opts *server.Options) {
		opts.WriteDeadline = duration
	}
}

// WithMaxConnections sets the maximum number of client connections.
// Default is 64K.
func WithMaxConnections(max int) Option {
	return func(opts *server.Options) {
		opts.MaxConn = max
	}
}

// WithMaxSubscriptions sets the maximum number of subscriptions per connection.
// Default is 0 (unlimited).
func WithMaxSubscriptions(max int) Option {
	return func(opts *server.Options) {
		opts.MaxSubs = max
	}
}

// WithDebug enables debug logging.
// Default is false.
func WithDebug(enabled bool) Option {
	return func(opts *server.Options) {
		opts.Debug = enabled
	}
}

// WithTrace enables trace logging.
// Default is false.
func WithTrace(enabled bool) Option {
	return func(opts *server.Options) {
		opts.Trace = enabled
	}
}

// WithLogFile sets the log file path.
// Empty string logs to stdout (default).
func WithLogFile(path string) Option {
	return func(opts *server.Options) {
		opts.LogFile = path
	}
}

// WithServerName sets the server name.
// Useful for server identification in clusters.
func WithServerName(name string) Option {
	return func(opts *server.Options) {
		opts.ServerName = name
	}
}

// StartEmbeddedServer starts an embedded NATS server with JetStream enabled.
// Perfect for testing without external dependencies.
//
// Example:
//
//	// Default configuration (random port, JetStream enabled)
//	srv, err := StartEmbeddedServer()
//
//	// Custom configuration
//	srv, err := StartEmbeddedServer(
//	    WithPort(4222),
//	    WithStoreDir("/tmp/nats"),
//	    WithDebug(true),
//	)
func StartEmbeddedServer(options ...Option) (*EmbeddedServer, error) {
	// Default options
	opts := &server.Options{
		Host:      "127.0.0.1",
		Port:      -1, // Random port
		JetStream: true,
		StoreDir:  "", // Use temp directory
	}

	// Apply custom options
	for _, opt := range options {
		opt(opts)
	}

	// Create server
	s, err := server.NewServer(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create embedded server: %w", err)
	}

	// Start server in goroutine
	go s.Start()

	// Wait for server to be ready
	if !s.ReadyForConnections(5 * time.Second) {
		return nil, fmt.Errorf("server not ready within 5 seconds")
	}

	url := s.ClientURL()

	return &EmbeddedServer{
		server: s,
		url:    url,
	}, nil
}

// Server returns the underlying NATS server.
// Useful for advanced configuration or monitoring.
func (e *EmbeddedServer) Server() *server.Server {
	return e.server
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

// ShutdownWithTimeout stops the embedded server gracefully with a custom timeout.
// Safe to call multiple times - only the first call will perform shutdown.
func (e *EmbeddedServer) ShutdownWithTimeout(timeout time.Duration) {
	e.shutdownOnce.Do(func() {
		if e.server != nil {
			// Shutdown the server
			e.server.Shutdown()

			// Wait for shutdown with custom timeout
			timer := time.After(timeout)
			shutdownDone := make(chan struct{})

			go func() {
				e.server.WaitForShutdown()
				close(shutdownDone)
			}()

			select {
			case <-shutdownDone:
				// Shutdown completed successfully
			case <-timer:
				// Timeout - log but don't block
				fmt.Printf("Warning: NATS server shutdown timed out after %v\n", timeout)
			}
		}
	})
}

// ConnectToEmbedded connects to an embedded NATS server and returns a client.
// Useful for testing.
//
// Example:
//
//	srv, _ := StartEmbeddedServer()
//	defer srv.Shutdown()
//
//	nc, err := ConnectToEmbedded(srv)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer nc.Close()
func ConnectToEmbedded(srv *EmbeddedServer) (*nats.Conn, error) {
	return nats.Connect(srv.URL())
}

// ConnectToEmbeddedWithOptions connects to an embedded NATS server with custom options.
//
// Example:
//
//	srv, _ := StartEmbeddedServer()
//	defer srv.Shutdown()
//
//	nc, err := ConnectToEmbeddedWithOptions(srv,
//	    nats.Name("my-client"),
//	    nats.MaxReconnects(5),
//	    nats.ReconnectWait(time.Second),
//	)
func ConnectToEmbeddedWithOptions(srv *EmbeddedServer, opts ...nats.Option) (*nats.Conn, error) {
	return nats.Connect(srv.URL(), opts...)
}
