package sdk

import (
	"context"
	"fmt"
	"time"

	"github.com/plaenen/eventsourcing/pkg/eventsourcing"
	natspkg "github.com/plaenen/eventsourcing/pkg/nats"
	"github.com/plaenen/eventsourcing/pkg/sqlite"
	"google.golang.org/protobuf/proto"
)

// Client is a unified SDK for event sourcing that provides a great developer experience.
// It combines command bus, event bus, and event store in a single interface.
type Client struct {
	commandBus   eventsourcing.CommandBus
	eventBus     eventsourcing.EventBus
	eventStore   eventsourcing.EventStore
	embeddedNATS interface{} // *nats.EmbeddedServer - stores embedded NATS for cleanup
	config       *Config
}

// Config holds configuration for the SDK client.
type Config struct {
	// Mode determines if the client runs in development or production mode
	Mode Mode

	// NATS configuration (used in production mode)
	NATS NATSConfig

	// SQLite configuration (event store)
	SQLite SQLiteConfig

	// Timeouts
	CommandTimeout time.Duration
}

// Mode represents the operational mode of the client.
type Mode string

const (
	// DevelopmentMode uses in-memory command bus for local development
	DevelopmentMode Mode = "development"

	// ProductionMode uses NATS for distributed command processing
	ProductionMode Mode = "production"
)

// NATSConfig holds NATS-specific configuration.
type NATSConfig struct {
	URL            string
	StreamName     string
	StreamSubjects []string
	MaxAge         time.Duration
	MaxBytes       int64
}

// SQLiteConfig holds SQLite event store configuration.
type SQLiteConfig struct {
	DSN     string
	WALMode bool
}

// DefaultConfig returns sensible defaults for the SDK.
func DefaultConfig() *Config {
	return &Config{
		Mode: DevelopmentMode,
		NATS: NATSConfig{
			URL:            "nats://localhost:4222",
			StreamName:     "EVENTS",
			StreamSubjects: []string{"events.>"},
			MaxAge:         7 * 24 * time.Hour,
			MaxBytes:       1024 * 1024 * 1024,
		},
		SQLite: SQLiteConfig{
			DSN:     ":memory:",
			WALMode: false,
		},
		CommandTimeout: 30 * time.Second,
	}
}

// NewClient creates a new SDK client based on the configuration.
func NewClient(config *Config) (*Client, error) {
	if config == nil {
		config = DefaultConfig()
	}

	client := &Client{
		config: config,
	}

	// 1. Initialize Event Store (always SQLite)
	eventStore, err := sqlite.NewEventStore(
		sqlite.WithDSN(config.SQLite.DSN),
		sqlite.WithWALMode(config.SQLite.WALMode),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create event store: %w", err)
	}
	client.eventStore = eventStore

	// 2. Initialize Event Bus (always NATS for distribution)
	var eventBus *natspkg.EventBus
	var embeddedNATS *natspkg.EmbeddedServer

	if config.Mode == DevelopmentMode {
		// Use embedded NATS for development
		eventBus, embeddedNATS, err = natspkg.NewEmbeddedEventBus()
		if err != nil {
			return nil, fmt.Errorf("failed to create embedded event bus: %w", err)
		}
		client.embeddedNATS = embeddedNATS
	} else {
		// Use external NATS for production
		eventBus, err = natspkg.NewEventBus(natspkg.Config{
			URL:            config.NATS.URL,
			StreamName:     config.NATS.StreamName,
			StreamSubjects: config.NATS.StreamSubjects,
			MaxAge:         config.NATS.MaxAge,
			MaxBytes:       config.NATS.MaxBytes,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create event bus: %w", err)
		}
	}
	client.eventBus = eventBus

	// 3. Initialize Command Bus (depends on mode)
	switch config.Mode {
	case DevelopmentMode:
		// In-memory command bus for local development
		client.commandBus = eventsourcing.NewCommandBusWithEventBus(eventBus)

	case ProductionMode:
		// NATS-based command bus for distributed architecture
		commandBus, err := natspkg.NewCommandBus(natspkg.CommandBusConfig{
			URL:        config.NATS.URL,
			Timeout:    config.CommandTimeout,
			QueueGroup: "command-handlers",
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create command bus: %w", err)
		}
		client.commandBus = commandBus

	default:
		return nil, fmt.Errorf("invalid mode: %s", config.Mode)
	}

	return client, nil
}

// SendCommand sends a command and waits for it to be processed.
func (c *Client) SendCommand(ctx context.Context, aggregateID string, command proto.Message, metadata eventsourcing.CommandMetadata) error {
	// Ensure command type is set
	if metadata.Custom == nil {
		metadata.Custom = make(map[string]string)
	}
	metadata.Custom["command_type"] = string(command.ProtoReflect().Descriptor().FullName())
	metadata.Custom["aggregate_id"] = aggregateID

	// Generate IDs if not provided
	if metadata.CommandID == "" {
		metadata.CommandID = eventsourcing.GenerateID()
	}
	if metadata.CorrelationID == "" {
		metadata.CorrelationID = eventsourcing.GenerateID()
	}
	if metadata.Timestamp.IsZero() {
		metadata.Timestamp = time.Now()
	}

	envelope := &eventsourcing.CommandEnvelope{
		Command:  command,
		Metadata: metadata,
	}

	return c.commandBus.Send(ctx, envelope)
}

// SubscribeToEvents subscribes to events matching the filter.
func (c *Client) SubscribeToEvents(filter eventsourcing.EventFilter, handler eventsourcing.EventHandler) (eventsourcing.Subscription, error) {
	return c.eventBus.Subscribe(filter, handler)
}

// RegisterCommandHandler registers a command handler.
// In development mode, this registers with the in-memory bus.
// In production mode, this subscribes to NATS for distributed handling.
func (c *Client) RegisterCommandHandler(commandType string, handler eventsourcing.CommandHandler) {
	c.commandBus.Register(commandType, handler)
}

// UseCommandMiddleware adds middleware to the command processing pipeline.
func (c *Client) UseCommandMiddleware(middleware eventsourcing.CommandMiddleware) {
	c.commandBus.Use(middleware)
}

// EventStore returns the underlying event store.
func (c *Client) EventStore() eventsourcing.EventStore {
	return c.eventStore
}

// EventBus returns the underlying event bus.
func (c *Client) EventBus() eventsourcing.EventBus {
	return c.eventBus
}

// CommandBus returns the underlying command bus.
func (c *Client) CommandBus() eventsourcing.CommandBus {
	return c.commandBus
}

// Close closes all connections and releases resources.
func (c *Client) Close() error {
	var errs []error

	if c.eventStore != nil {
		if err := c.eventStore.Close(); err != nil {
			errs = append(errs, fmt.Errorf("event store close error: %w", err))
		}
	}

	if c.eventBus != nil {
		if eb, ok := c.eventBus.(*natspkg.EventBus); ok {
			if err := eb.Close(); err != nil {
				errs = append(errs, fmt.Errorf("event bus close error: %w", err))
			}
		}
	}

	if c.commandBus != nil {
		if cb, ok := c.commandBus.(*natspkg.CommandBus); ok {
			if err := cb.Close(); err != nil {
				errs = append(errs, fmt.Errorf("command bus close error: %w", err))
			}
		}
	}

	// Shutdown embedded NATS if present
	if c.embeddedNATS != nil {
		if srv, ok := c.embeddedNATS.(*natspkg.EmbeddedServer); ok {
			srv.Shutdown()
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors during close: %v", errs)
	}

	return nil
}

// Builder provides a fluent API for building SDK clients.
type Builder struct {
	config *Config
}

// NewBuilder creates a new builder with default configuration.
func NewBuilder() *Builder {
	return &Builder{
		config: DefaultConfig(),
	}
}

// WithMode sets the operational mode.
func (b *Builder) WithMode(mode Mode) *Builder {
	b.config.Mode = mode
	return b
}

// WithNATSURL sets the NATS server URL.
func (b *Builder) WithNATSURL(url string) *Builder {
	b.config.NATS.URL = url
	return b
}

// WithSQLiteDSN sets the SQLite database DSN.
func (b *Builder) WithSQLiteDSN(dsn string) *Builder {
	b.config.SQLite.DSN = dsn
	return b
}

// WithWALMode enables or disables WAL mode for SQLite.
func (b *Builder) WithWALMode(enabled bool) *Builder {
	b.config.SQLite.WALMode = enabled
	return b
}

// WithCommandTimeout sets the command timeout.
func (b *Builder) WithCommandTimeout(timeout time.Duration) *Builder {
	b.config.CommandTimeout = timeout
	return b
}

// Build creates the client.
func (b *Builder) Build() (*Client, error) {
	return NewClient(b.config)
}
