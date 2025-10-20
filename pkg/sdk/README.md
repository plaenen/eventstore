# Event Sourcing SDK

A unified, developer-friendly SDK for building event-sourced applications with distributed command processing.

## Features

- **Unified Interface**: Single client for commands, events, and persistence
- **Two Modes**: Development (in-memory) and Production (NATS-based)
- **Type-Safe**: Full Go generics and protobuf support
- **Distributed by Default**: Commands and events flow through NATS in production
- **Builder Pattern**: Fluent API for configuration
- **Zero Config**: Sensible defaults for quick starts

## Quick Start

### Development Mode (In-Memory)

Perfect for local development, testing, and prototyping:

```go
package main

import (
    "context"
    "github.com/plaenen/eventsourcing/pkg/sdk"
    "github.com/plaenen/eventsourcing/pkg/eventsourcing"
    accountv1 "github.com/plaenen/eventsourcing/examples/pb/account/v1"
)

func main() {
    // Create client in development mode
    client, _ := sdk.NewBuilder().
        WithMode(sdk.DevelopmentMode).
        WithSQLiteDSN(":memory:").
        Build()
    defer client.Close()

    // Register command handler
    client.RegisterCommandHandler("account.v1.OpenAccountCommand",
        eventsourcing.CommandHandlerFunc(func(ctx context.Context, cmd *eventsourcing.CommandEnvelope) ([]*eventsourcing.Event, error) {
            // Your business logic here
            return events, nil
        }),
    )

    // Send command
    ctx := context.Background()
    client.SendCommand(ctx, "acc-123", &accountv1.OpenAccountCommand{
        AccountId:      "acc-123",
        OwnerName:      "John Doe",
        InitialBalance: "1000.00",
    }, eventsourcing.CommandMetadata{
        PrincipalID: "user-123",
    })

    // Subscribe to events
    client.SubscribeToEvents(
        eventsourcing.EventFilter{AggregateTypes: []string{"Account"}},
        func(event *eventsourcing.EventEnvelope) error {
            // Handle event
            return nil
        },
    )
}
```

### Production Mode (NATS)

For distributed, production-ready deployments:

```go
client, _ := sdk.NewBuilder().
    WithMode(sdk.ProductionMode).
    WithNATSURL("nats://nats-cluster:4222").
    WithSQLiteDSN("./data/events.db").
    WithWALMode(true).
    WithCommandTimeout(30 * time.Second).
    Build()
defer client.Close()

// Same API - commands now flow through NATS!
client.SendCommand(ctx, aggregateID, command, metadata)
```

## Architecture

### Development Mode

```
┌─────────────────┐
│   Your App      │
│                 │
│  ┌───────────┐  │
│  │   SDK     │  │
│  ├───────────┤  │
│  │ In-Memory │  │  Commands processed locally
│  │ Cmd Bus   │  │
│  ├───────────┤  │
│  │   NATS    │  │  Events published to NATS
│  │ Event Bus │  │
│  ├───────────┤  │
│  │  SQLite   │  │  Events persisted locally
│  │Event Store│  │
│  └───────────┘  │
└─────────────────┘
```

### Production Mode

```
┌──────────────┐         ┌──────────────┐         ┌──────────────┐
│  Service A   │         │  Service B   │         │  Service C   │
│              │         │              │         │              │
│ ┌──────────┐ │         │ ┌──────────┐ │         │ ┌──────────┐ │
│ │   SDK    │ │         │ │   SDK    │ │         │ │   SDK    │ │
│ └──────────┘ │         │ └──────────┘ │         │ └──────────┘ │
└──────┬───────┘         └──────┬───────┘         └──────┬───────┘
       │                        │                        │
       └────────────────────────┼────────────────────────┘
                                │
                    ┌───────────▼───────────┐
                    │                       │
                    │     NATS Cluster      │
                    │                       │
                    │  ┌─────────────────┐  │
                    │  │ Commands Stream │  │  Request-reply pattern
                    │  ├─────────────────┤  │
                    │  │  Events Stream  │  │  Pub-sub pattern
                    │  └─────────────────┘  │
                    └───────────────────────┘
```

## Key Concepts

### Command Flow

**Development Mode:**
1. `SendCommand()` → In-memory handler
2. Handler executes business logic
3. Events saved to SQLite
4. Events published to NATS

**Production Mode:**
1. `SendCommand()` → Published to NATS
2. Handler service receives from NATS
3. Handler executes business logic
4. Events saved to SQLite
5. Events published to NATS
6. Response sent back via NATS

### Event Flow

Both modes:
1. Events saved to event store (SQLite)
2. Events published to NATS JetStream
3. Subscribers consume from NATS
4. Durable consumers for reliable processing

## Configuration

### Builder Methods

```go
sdk.NewBuilder().
    WithMode(sdk.ProductionMode).           // Development or Production
    WithNATSURL("nats://localhost:4222").  // NATS server
    WithSQLiteDSN("./events.db").          // Event store path
    WithWALMode(true).                     // SQLite WAL mode
    WithCommandTimeout(30 * time.Second).  // Command timeout
    Build()
```

### Default Configuration

```go
Mode:           DevelopmentMode
NATS URL:       nats://localhost:4222
SQLite DSN:     :memory:
WAL Mode:       false
CommandTimeout: 30s
```

## API Reference

### Client Methods

```go
// Send a command
SendCommand(ctx context.Context, aggregateID string, command proto.Message, metadata CommandMetadata) error

// Register command handler
RegisterCommandHandler(commandType string, handler CommandHandler)

// Add command middleware
UseCommandMiddleware(middleware CommandMiddleware)

// Subscribe to events
SubscribeToEvents(filter EventFilter, handler EventHandler) (Subscription, error)

// Access underlying components
EventStore() EventStore
EventBus() EventBus
CommandBus() CommandBus

// Cleanup
Close() error
```

## Examples

See `examples/sdk/main.go` for complete working examples demonstrating:
- Development mode usage
- Production mode usage
- Command handling
- Event subscription
- State verification

## When to Use Each Mode

### Development Mode
- ✅ Local development
- ✅ Unit/integration tests
- ✅ Prototyping
- ✅ Single-service deployments
- ✅ Quick feedback loops

### Production Mode
- ✅ Multi-service architectures
- ✅ Microservices
- ✅ Distributed teams
- ✅ Scalability requirements
- ✅ Service decoupling

## Migration Path

Start with **Development Mode** for rapid iteration:
```go
client, _ := sdk.NewBuilder().
    WithMode(sdk.DevelopmentMode).
    Build()
```

Switch to **Production Mode** when ready to scale:
```go
client, _ := sdk.NewBuilder().
    WithMode(sdk.ProductionMode).  // Only change needed!
    WithNATSURL(os.Getenv("NATS_URL")).
    Build()
```

**No code changes required** - the SDK handles everything!

## Best Practices

1. **Use the Builder**: Fluent API makes configuration clear
2. **Environment-based mode**: Set mode via environment variables
3. **Graceful shutdown**: Always `defer client.Close()`
4. **Error handling**: Check errors from `SendCommand()`
5. **Idempotency**: Always provide CommandID for retry safety
6. **Middleware**: Use for cross-cutting concerns (logging, tracing, auth)

## Benefits

✅ **Single API** - Same code works in dev and production
✅ **Type-Safe** - Full Go type safety with protobuf
✅ **Distributed** - Built for microservices from day one
✅ **Fast Development** - In-memory mode for quick iteration
✅ **Production-Ready** - NATS mode for real deployments
✅ **Testable** - Easy to test with in-memory mode
✅ **Flexible** - Access underlying components when needed
