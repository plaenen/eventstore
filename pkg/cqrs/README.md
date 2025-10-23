# CQRS Package

CQRS (Command Query Responsibility Segregation) transport implementations for command and query handling.

## Architecture

```
pkg/eventsourcing/          # CQRS interfaces (Server, Transport, CommandBus)
        ↓ (implemented by)
pkg/cqrs/
  ├── nats/                 # NATS-based CQRS transport
  ├── http/                 # Future: HTTP/REST transport
  └── grpc/                 # Future: gRPC transport
```

## Why This Structure?

**Separation of Concerns:**
- `pkg/eventsourcing` = Domain interfaces (what)
- `pkg/cqrs/*` = Transport implementations (how)
- `pkg/nats` = Pure NATS infrastructure (not domain-specific)

**Benefits:**
- ✅ CQRS implementations grouped by transport mechanism
- ✅ Clear separation from event sourcing (commands ≠ events)
- ✅ Easy to add new transports (HTTP, gRPC, Kafka, etc.)
- ✅ Consistent with `pkg/eventbus` pattern

## CQRS vs Event Sourcing

**Commands & Queries (CQRS):**
- Request/Reply pattern
- Synchronous communication
- Direct responses expected
- Example: `ExecuteCommand` → `Result`

**Events (Event Sourcing):**
- Publish/Subscribe pattern
- Asynchronous communication
- Fire-and-forget or at-least-once delivery
- Example: `PublishEvent` → (multiple subscribers)

**Both are important but serve different purposes!**

## Available Implementations

### NATS CQRS (`pkg/cqrs/nats`)

Production-ready CQRS transport using NATS with:
- Request/reply for commands and queries
- NATS microservices for service discovery
- Queue groups for load balancing
- Observability (OpenTelemetry)
- Timeout and retry handling

#### Server-Side (Command/Query Handlers)

```go
import cqrsnats "github.com/plaenen/eventstore/pkg/cqrs/nats"

// Create server
server, err := cqrsnats.NewServer(&cqrsnats.ServerConfig{
    ServerConfig: &eventsourcing.ServerConfig{
        QueueGroup:     "my-service-handlers",
        MaxConcurrent:  10,
        HandlerTimeout: 5 * time.Second,
    },
    URL:         "nats://localhost:4222",
    Name:        "MyService",
    Version:     "1.0.0",
    Description: "My service description",
    Telemetry:   telemetry,  // Optional: OpenTelemetry
})
if err != nil {
    log.Fatal(err)
}
defer server.Close()

// Register command service (generated code)
commandService := myv1.NewMyCommandServiceServer(server, commandHandler)
if err := commandService.Start(ctx); err != nil {
    log.Fatal(err)
}

// Register query service (generated code)
queryService := myv1.NewMyQueryServiceServer(server, queryHandler)
if err := queryService.Start(ctx); err != nil {
    log.Fatal(err)
}
```

#### Client-Side (Sending Commands/Queries)

```go
import cqrsnats "github.com/plaenen/eventstore/pkg/cqrs/nats"

// Create transport
transport, err := cqrsnats.NewTransport(&cqrsnats.TransportConfig{
    TransportConfig: &eventsourcing.TransportConfig{
        Timeout:              5 * time.Second,
        MaxReconnectAttempts: 3,
        ReconnectWait:        1 * time.Second,
    },
    URL:       "nats://localhost:4222",
    Name:      "my-client",
    Telemetry: telemetry,  // Optional: OpenTelemetry
})
if err != nil {
    log.Fatal(err)
}
defer transport.Close()

// Create client (generated code)
client := myv1.NewMyServiceClient(transport)

// Execute command
result, err := client.ExecuteCommand(ctx, &myv1.MyCommand{...})
if err != nil {
    log.Fatal(err)
}

// Execute query
data, err := client.ExecuteQuery(ctx, &myv1.MyQuery{...})
if err != nil {
    log.Fatal(err)
}
```

#### Command Bus (Distributed Commands)

```go
import cqrsnats "github.com/plaenen/eventstore/pkg/cqrs/nats"

// Create command bus
bus, err := cqrsnats.NewCommandBus(cqrsnats.DefaultCommandBusConfig())
if err != nil {
    log.Fatal(err)
}
defer bus.Close()

// Register handler
bus.RegisterHandler("MyCommand", func(ctx context.Context, cmd proto.Message) (proto.Message, error) {
    // Handle command
    return &MyResponse{...}, nil
})

// Send command
response, err := bus.Send(ctx, "MyCommand", &MyCommand{...})
if err != nil {
    log.Fatal(err)
}
```

### Features

**Server:**
- NATS microservices integration
- Service discovery and metadata
- Queue groups for load balancing
- Concurrent request handling
- Timeout protection
- OpenTelemetry tracing

**Transport (Client):**
- Request/reply pattern
- Connection pooling
- Automatic reconnection
- Timeout and retry logic
- OpenTelemetry tracing
- Context propagation

**Command Bus:**
- Distributed command processing
- Queue-based load balancing
- Middleware support
- Type-safe protobuf messages

## Configuration

### Server Config

```go
config := &cqrsnats.ServerConfig{
    ServerConfig: &eventsourcing.ServerConfig{
        QueueGroup:     "service-handlers",  // For load balancing
        MaxConcurrent:  10,                   // Max parallel requests
        HandlerTimeout: 5 * time.Second,     // Request timeout
    },
    URL:         "nats://prod-nats:4222",    // NATS server
    Name:        "AccountService",           // Service name
    Version:     "2.0.0",                    // Semantic version
    Description: "Account management",       // Human-readable
    Telemetry:   telemetry,                  // Optional observability
}
```

### Transport Config

```go
config := &cqrsnats.TransportConfig{
    TransportConfig: &eventsourcing.TransportConfig{
        Timeout:              5 * time.Second,  // Request timeout
        MaxReconnectAttempts: 3,                // Retry attempts
        ReconnectWait:        1 * time.Second,  // Retry delay
    },
    URL:       "nats://prod-nats:4222",  // NATS server
    Name:      "web-client",             // Client identifier
    Telemetry: telemetry,                // Optional observability
}
```

## Observability

Both server and transport support OpenTelemetry:

### Tracing

```go
// Automatic traces for:
// - cqrs.server.handle_request
// - cqrs.transport.send_request

// With span attributes:
// - service.name, service.version
// - command.type, query.type
// - request.timeout
// - response.status
```

### Example with Telemetry

```go
import (
    "github.com/plaenen/eventstore/pkg/observability"
    cqrsnats "github.com/plaenen/eventstore/pkg/cqrs/nats"
)

// Initialize observability
tel, err := observability.Init(ctx, observability.Config{
    ServiceName:    "my-service",
    ServiceVersion: "1.0.0",
    TraceExporter:  exporter,
    TraceSampleRate: 1.0,
})
defer tel.Shutdown(ctx)

// Create server with telemetry
server, err := cqrsnats.NewServer(&cqrsnats.ServerConfig{
    // ... other config
    Telemetry: tel,
})
```

## Future Implementations

### HTTP/REST (Planned)

```go
import "github.com/plaenen/eventstore/pkg/cqrs/http"

server, err := http.NewServer(&http.ServerConfig{
    Address: ":8080",
    // ... config
})
```

### gRPC (Planned)

```go
import "github.com/plaenen/eventstore/pkg/cqrs/grpc"

server, err := grpc.NewServer(&grpc.ServerConfig{
    Port: 9090,
    // ... config
})
```

## Package Dependencies

```
pkg/cqrs/nats
    → pkg/eventsourcing        (Server, Transport interfaces)
    → pkg/observability        (OpenTelemetry)
    → github.com/nats-io/nats.go  (NATS client)
```

No dependencies on `pkg/nats` infrastructure or `pkg/eventbus`.

## Testing

```bash
# Test NATS CQRS
go test github.com/plaenen/eventstore/pkg/cqrs/nats

# Test all CQRS implementations
go test github.com/plaenen/eventstore/pkg/cqrs/...
```

## Examples

- `examples/cmd/bankaccount-observability` - Complete CQRS example with observability
- Generated code examples in `examples/pb/*/` packages

## CQRS Interfaces

All implementations satisfy interfaces from `pkg/eventsourcing`:

```go
// Server handles incoming commands and queries
type Server interface {
    RegisterHandler(subject string, handler HandlerFunc) error
    Start(ctx context.Context) error
    Close() error
}

// Transport sends commands and queries
type Transport interface {
    Request(ctx context.Context, subject string, request proto.Message) (proto.Message, error)
    Close() error
}

// CommandBus for distributed command processing
type CommandBus interface {
    RegisterHandler(commandType string, handler CommandHandler) error
    Send(ctx context.Context, command proto.Message) (proto.Message, error)
    Close() error
}
```

## Design Principles

1. **Interface Segregation**: Each transport implements only CQRS interfaces
2. **Dependency Inversion**: Implementations depend on abstractions
3. **Single Responsibility**: Each package handles one transport
4. **Open/Closed**: Easy to add new transports without modifying existing code

## Migration from pkg/nats

**Old (deprecated):**
```go
import "github.com/plaenen/eventstore/pkg/nats"

server, err := nats.NewServer(config)
transport, err := nats.NewTransport(config)
```

**New (current):**
```go
import cqrsnats "github.com/plaenen/eventstore/pkg/cqrs/nats"

server, err := cqrsnats.NewServer(config)
transport, err := cqrsnats.NewTransport(config)
```

## See Also

- `pkg/eventsourcing` - Core interfaces
- `pkg/eventbus` - Event publishing/subscription (different pattern)
- `pkg/observability` - OpenTelemetry integration
- `pkg/nats` - NATS infrastructure utilities

## License

Part of the eventstore framework.
