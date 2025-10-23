# EventBus Package

Event bus implementations for event sourcing. Each sub-package provides a different transport mechanism for publishing and subscribing to domain events.

## Architecture

```
pkg/eventsourcing/          # EventBus interface definition
        ↓ (implemented by)
pkg/eventbus/
  ├── nats/                 # NATS JetStream implementation
  ├── kafka/                # Future: Apache Kafka implementation
  └── memory/               # Future: In-memory for testing
```

## Why This Structure?

**Separation of Concerns:**
- `pkg/eventsourcing` = Domain interfaces (what)
- `pkg/eventbus/*` = Transport implementations (how)
- `pkg/nats` = Pure NATS infrastructure (embedded server, utilities)

**Benefits:**
- ✅ EventBus implementations grouped by transport mechanism
- ✅ NATS package stays focused on infrastructure (not domain)
- ✅ Easy to add new transports (Kafka, RabbitMQ, etc.)
- ✅ Clear dependency direction (implementations depend on interfaces)

## Available Implementations

### NATS JetStream (`pkg/eventbus/nats`)

Production-ready event bus using NATS JetStream with:
- At-least-once delivery guarantees
- Durable streaming
- Message deduplication
- Consumer groups
- Stream retention policies

**Quick Start:**

```go
import natseventbus "github.com/plaenen/eventstore/pkg/eventbus/nats"

// Connect to NATS server
config := natseventbus.DefaultConfig()
config.URL = "nats://localhost:4222"
config.StreamName = "PRODUCTION_EVENTS"

bus, err := natseventbus.NewEventBus(config)
if err != nil {
    log.Fatal(err)
}
defer bus.Close()

// Publish events
events := []*eventsourcing.Event{...}
if err := bus.Publish(events); err != nil {
    log.Fatal(err)
}

// Subscribe to events
filter := eventsourcing.EventFilter{
    AggregateTypes: []string{"Account"},
}

sub, err := bus.Subscribe(filter, func(envelope *eventsourcing.EventEnvelope) error {
    // Handle event
    return nil
})
```

**Configuration:**

```go
config := natseventbus.Config{
    URL:            "nats://prod-nats:4222",  // NATS server URL
    StreamName:     "EVENTS",                  // JetStream stream name
    StreamSubjects: []string{"events.>"},     // Subject patterns
    MaxAge:         7 * 24 * time.Hour,       // Retention period
    MaxBytes:       1024 * 1024 * 1024,       // Max storage (1GB)
}
```

**For testing with embedded NATS:**

```go
import (
    natseventbus "github.com/plaenen/eventstore/pkg/eventbus/nats"
    "github.com/plaenen/eventstore/pkg/nats"
)

// Start embedded server
srv, err := nats.StartEmbeddedServer()
if err != nil {
    log.Fatal(err)
}
defer srv.Shutdown()

// Create EventBus using embedded server
config := natseventbus.DefaultConfig()
config.URL = srv.URL()

bus, err := natseventbus.NewEventBus(config)
if err != nil {
    log.Fatal(err)
}
defer bus.Close()
```

**Runner Integration:**

For production with lifecycle management, use `pkg/runnable/eventbus`:

```go
import (
    "github.com/plaenen/eventstore/pkg/runnable/eventbus"
    "github.com/plaenen/eventstore/pkg/runner"
)

service := eventbus.New()  // Uses embedded NATS + EventBus

runner := runner.New([]runner.Service{service})
runner.Run(ctx)

// Access EventBus after startup
bus := service.EventBus()
```

See `pkg/runnable/README.md` for details.

## Future Implementations

### Kafka (Planned)

```go
import "github.com/plaenen/eventstore/pkg/eventbus/kafka"

config := kafka.DefaultConfig()
config.Brokers = []string{"localhost:9092"}
config.Topic = "events"

bus, err := kafka.NewEventBus(config)
```

### In-Memory (Planned)

For testing without external dependencies:

```go
import "github.com/plaenen/eventstore/pkg/eventbus/memory"

bus := memory.NewEventBus()
defer bus.Close()

// Same EventBus interface
bus.Publish(events)
bus.Subscribe(filter, handler)
```

## EventBus Interface

All implementations satisfy the `eventsourcing.EventBus` interface:

```go
type EventBus interface {
    // Publish events to the bus
    Publish(events []*Event) error

    // Subscribe to events matching the filter
    Subscribe(filter EventFilter, handler EventHandler) (Subscription, error)

    // Close the event bus and clean up resources
    Close() error
}

type EventFilter struct {
    AggregateTypes []string  // Filter by aggregate type
    EventTypes     []string  // Filter by event type
}

type EventHandler func(*EventEnvelope) error

type Subscription interface {
    Unsubscribe() error
}
```

## Package Dependencies

```
pkg/eventbus/nats
    → pkg/eventsourcing   (EventBus interface)
    → github.com/nats-io/nats.go  (NATS client)

pkg/eventbus/kafka (future)
    → pkg/eventsourcing   (EventBus interface)
    → github.com/segmentio/kafka-go (Kafka client)

pkg/eventbus/memory (future)
    → pkg/eventsourcing   (EventBus interface)
```

No dependencies on `pkg/nats` infrastructure.

## Testing

Each implementation includes comprehensive tests:

```bash
# Test NATS EventBus
go test github.com/plaenen/eventstore/pkg/eventbus/nats

# Test all EventBus implementations
go test github.com/plaenen/eventstore/pkg/eventbus/...
```

## Examples

- `examples/cmd/runner-nats` - EventBus with runner integration
- `examples/cmd/projection-nats` - Projections consuming events from EventBus
- `pkg/eventbus/nats/eventbus_test.go` - Detailed usage examples

## Design Principles

1. **Interface Segregation**: Each transport implements only `EventBus` interface
2. **Dependency Inversion**: Implementations depend on abstractions (interfaces)
3. **Single Responsibility**: Each package handles one transport mechanism
4. **Open/Closed**: Easy to add new transports without modifying existing code

## Migration from pkg/nats

If you were using:

```go
import "github.com/plaenen/eventstore/pkg/nats"

bus, err := nats.NewEventBus(config)
```

Update to:

```go
import natseventbus "github.com/plaenen/eventstore/pkg/eventbus/nats"

bus, err := natseventbus.NewEventBus(config)
```

## See Also

- `pkg/eventsourcing` - Core event sourcing interfaces
- `pkg/nats` - NATS infrastructure (embedded server, utilities)
- `pkg/runnable/eventbus` - Runner service adapter
- `pkg/store/sqlite` - SQLite event store implementation

## License

Part of the eventstore framework.
