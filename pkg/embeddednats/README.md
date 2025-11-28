# NATS Package

Pure NATS infrastructure utilities - embedded server for testing and local development.

## Overview

This package provides **infrastructure-only** NATS utilities with **no domain logic**. It's focused on providing a convenient embedded NATS server for testing.

**For domain-specific implementations, see:**
- `pkg/eventbus/nats` - Event sourcing (events)
- `pkg/cqrs/nats` - CQRS (commands & queries)

## Components

### Embedded NATS Server

In-process NATS server for testing and local development:

```go
import "github.com/plaenen/eventstore/pkg/nats"

// Start embedded server
srv, err := nats.StartEmbeddedServer()
if err != nil {
    log.Fatal(err)
}
defer srv.Shutdown()

// Get connection URL
url := srv.URL()  // e.g., "nats://127.0.0.1:54321"

// Connect clients
nc, err := nats.ConnectToEmbedded(srv)
if err != nil {
    log.Fatal(err)
}
defer nc.Close()
```

**Features:**
- Random port allocation
- JetStream enabled by default
- Graceful shutdown with timeout (5 seconds max)
- Thread-safe shutdown (safe to call multiple times)
- Perfect for testing without external dependencies

**Helper Function:**

```go
// Connect to embedded server
nc, err := nats.ConnectToEmbedded(srv)
```

## What's NOT Here?

This package is **pure infrastructure**. Domain-specific implementations have been moved:

### Event Sourcing (Events)

**Moved to** `pkg/eventbus/nats`:

```go
// Old (deprecated)
import "github.com/plaenen/eventstore/pkg/nats"
bus, err := nats.NewEventBus(config)

// New (current)
import natseventbus "github.com/plaenen/eventstore/pkg/eventbus/nats"
bus, err := natseventbus.NewEventBus(config)
```

See `pkg/eventbus/README.md` for documentation.

### CQRS (Commands & Queries)

**Moved to** `pkg/cqrs/nats`:

```go
// Old (deprecated)
import "github.com/plaenen/eventstore/pkg/nats"
server, err := nats.NewServer(config)
transport, err := nats.NewTransport(config)

// New (current)
import cqrsnats "github.com/plaenen/eventstore/pkg/cqrs/nats"
server, err := cqrsnats.NewServer(config)
transport, err := cqrsnats.NewTransport(config)
```

See `pkg/cqrs/README.md` for documentation.

## Runner Integration

For lifecycle management, use the adapters:

```go
// Embedded NATS server as a service
import "github.com/plaenen/eventstore/pkg/runnable/embeddednats"

service := embeddednats.New()
runner.New([]runner.Service{service}).Run(ctx)
```

See `pkg/runnable/README.md` for details.

## Configuration

```go
config := nats.Config{
    URL:            nats.DefaultURL,           // NATS server URL
    StreamName:     "EVENTS",                  // JetStream stream name
    StreamSubjects: []string{"events.>"},     // Subject patterns
    MaxAge:         7 * 24 * time.Hour,       // Event retention (7 days)
    MaxBytes:       1024 * 1024 * 1024,       // Max storage (1 GB)
}
```

## Testing Support

### Embedded Server

```go
func TestMyCode(t *testing.T) {
    // Start embedded server
    srv, err := nats.StartEmbeddedServer()
    if err != nil {
        t.Fatal(err)
    }
    defer srv.Shutdown()

    // Connect your code
    nc, _ := nats.Connect(srv.URL())
    defer nc.Close()

    // Run tests...
}
```

### Complete EventBus

```go
func TestProjections(t *testing.T) {
    bus, srv, err := nats.NewEmbeddedEventBus()
    if err != nil {
        t.Fatal(err)
    }
    defer nats.ShutdownWithBus(bus, srv)

    // Test with EventBus...
}
```

## Health Checks

For health check support with runner integration, see `pkg/runnable` services which implement `runner.HealthChecker`.

## Shutdown Behavior

### With Runner (Recommended)

See `pkg/runnable` for automatic shutdown management with runner integration.

### Manual Shutdown

```go
// Always close EventBus before server
bus.Close()
time.Sleep(100 * time.Millisecond)  // Drain connections
srv.Shutdown()  // Max 5 second timeout

// Or use helper
nats.ShutdownWithBus(bus, srv)
```

## Examples

See:
- `examples/cmd/runner-nats/main.go` - Runner integration examples
- `examples/cmd/projection-nats/main.go` - Projection with NATS
- `pkg/runnable/README.md` - Runnable service adapters

## Architecture

```
┌─────────────────────────────────┐
│ pkg/nats                        │
│ Pure Infrastructure             │
│ ┌─────────────────────────────┐ │
│ │ Embedded NATS Server        │ │
│ │ - JetStream enabled         │ │
│ │ - Random port               │ │
│ │ - Graceful shutdown         │ │
│ └─────────────────────────────┘ │
└─────────────────────────────────┘
         ↓ (used by)
┌─────────────────────────────────┐
│ Domain Implementations          │
│ - pkg/eventbus/nats             │
│ - pkg/cqrs/nats                 │
│ - pkg/runnable/embeddednats     │
└─────────────────────────────────┘
```

## Thread Safety

- ✅ `Shutdown()` is thread-safe (uses `sync.Once`)
- ✅ Multiple `Shutdown()` calls are safe (no-op after first)
- ✅ Concurrent `StartEmbeddedServer()` calls create independent servers

## Production Notes

**This package is for testing only.** For production:

1. **Use external NATS cluster** with proper configuration
2. **Deploy NATS with JetStream** for persistence and reliability
3. **Configure monitoring** and health checks
4. **Set up clustering** for high availability

Example production connection:
```go
nc, err := nats.Connect(
    "nats://prod-nats:4222",
    nats.Name("my-service"),
    nats.Token(os.Getenv("NATS_TOKEN")),
)
```

## License

Part of the eventstore framework.
