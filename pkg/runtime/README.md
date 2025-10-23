# Runnable Package

Service adapters that bridge core packages with the `runner` lifecycle manager.

## Architecture Philosophy

The `pkg/runnable` package follows the **Adapter Pattern** to keep core packages independent:

```
Core Packages (Independent)
├── pkg/runner/        (generic service lifecycle)
├── pkg/nats/          (NATS EventBus implementation)
└── pkg/observability/ (OpenTelemetry integration)
        ↓
    Adapters (Integration Layer)
        ↓
pkg/runnable/
├── embeddednats/      (adapts nats.EmbeddedServer → runner.Service)
├── eventbus/          (adapts nats.EventBus → runner.Service)
└── httpserver/        (future: adapts HTTP server → runner.Service)
```

### Why This Design?

**Without adapters** (bad):
```go
// pkg/nats imports pkg/runner - creates tight coupling
package nats
import "pkg/runner"

type EventBus struct {
    // ... now depends on runner
}
```

**With adapters** (good):
```go
// pkg/nats stays pure - no runner dependency
package nats
type EventBus struct { /* ... */ }

// pkg/runnable/eventbus adapts it
package eventbus
import (
    "pkg/nats"
    "pkg/runner"
)
type Service struct {
    bus *nats.EventBus
}
```

**Benefits**:
- ✅ Core packages stay independent and reusable
- ✅ No circular dependencies
- ✅ Easy to add observability at integration layer
- ✅ Clear separation: implementation vs lifecycle management
- ✅ Each adapter can have service-specific telemetry

## Available Services

### 1. `embeddednats` - Embedded NATS Server

Wraps `nats.EmbeddedServer` as a `runner.Service`.

**Use when**: You need just a NATS server without EventBus integration.

```go
import (
    "github.com/plaenen/eventstore/pkg/runnable/embeddednats"
    "github.com/plaenen/eventstore/pkg/runner"
)

// Basic usage
service := embeddednats.New()

// With options
service := embeddednats.New(
    embeddednats.WithLogger(logger),
    embeddednats.WithTracer(tracer),
)

// Use with runner
r := runner.New([]runner.Service{service})
r.Run(ctx)

// Access server after startup
url := service.URL()
server := service.Server()
```

**Features**:
- OpenTelemetry tracing (optional)
- Structured logging via `slog`
- Health checks
- Graceful shutdown with timeout

### 2. `eventbus` - NATS EventBus with Embedded Server

Wraps both `nats.EmbeddedServer` + `nats.EventBus` as a `runner.Service`.

**Use when**: You need a complete event bus solution for event sourcing.

```go
import (
    "github.com/plaenen/eventstore/pkg/runnable/eventbus"
    "github.com/plaenen/eventstore/pkg/nats"
    "github.com/plaenen/eventstore/pkg/runner"
)

// Basic usage (defaults)
service := eventbus.New()

// With custom config
config := nats.DefaultConfig()
config.StreamName = "PROD_EVENTS"
config.MaxAge = 30 * 24 * time.Hour // 30 days

service := eventbus.New(
    eventbus.WithConfig(config),
    eventbus.WithLogger(logger),
    eventbus.WithTracer(tracer),
)

// Use with runner
r := runner.New([]runner.Service{service})
r.Run(ctx)

// Access EventBus after startup
bus := service.EventBus()
bus.Publish(events)
```

**Features**:
- OpenTelemetry tracing with span attributes
- Structured logging (startup, shutdown, drain)
- Health checks (server + bus)
- Proper shutdown order (bus → drain → server)
- Configurable stream settings

## Complete Example

```go
package main

import (
    "context"
    "log"
    "log/slog"

    "github.com/plaenen/eventstore/pkg/nats"
    "github.com/plaenen/eventstore/pkg/observability"
    "github.com/plaenen/eventstore/pkg/runnable/eventbus"
    "github.com/plaenen/eventstore/pkg/runner"
    "go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
)

func main() {
    // Setup observability
    exporter, _ := stdouttrace.New()
    tel, _ := observability.Init(context.Background(), observability.Config{
        ServiceName:    "my-service",
        ServiceVersion: "1.0.0",
        TraceExporter:  exporter,
        TraceSampleRate: 1.0,
    })
    defer tel.Shutdown(context.Background())

    logger := slog.Default()
    tracer := tel.Tracer("main")

    // Create EventBus service with observability
    config := nats.DefaultConfig()
    config.StreamName = "PRODUCTION_EVENTS"

    eventBusService := eventbus.New(
        eventbus.WithConfig(config),
        eventbus.WithLogger(logger),
        eventbus.WithTracer(tracer),
    )

    // Create your other services
    projectionService := NewProjectionService(
        eventBusService.EventBus(),
        WithLogger(logger),
        WithTracer(tracer),
    )

    // Create runner with all services
    r := runner.New(
        []runner.Service{
            eventBusService,
            projectionService,
        },
        runner.WithLogger(logger),
        runner.WithShutdownTimeout(30 * time.Second),
    )

    // Run (blocks until signal or error)
    // Handles SIGTERM/SIGINT automatically
    if err := r.Run(context.Background()); err != nil {
        log.Fatal(err)
    }
}
```

## Creating Custom Runnable Services

To create your own runnable service:

```go
package myservice

import (
    "context"
    "log/slog"

    "github.com/plaenen/eventstore/pkg/observability"
    "github.com/plaenen/eventstore/pkg/runner"
    "go.opentelemetry.io/otel/trace"
)

type Service struct {
    // Your dependencies
    db     *sql.DB
    logger *slog.Logger
    tracer trace.Tracer
}

type Option func(*Service)

func WithLogger(logger *slog.Logger) Option {
    return func(s *Service) { s.logger = logger }
}

func WithTracer(tracer trace.Tracer) Option {
    return func(s *Service) { s.tracer = tracer }
}

func New(db *sql.DB, opts ...Option) *Service {
    s := &Service{
        db:     db,
        logger: slog.Default(),
        tracer: trace.NewNoopTracerProvider().Tracer("myservice"),
    }
    for _, opt := range opts {
        opt(s)
    }
    return s
}

func (s *Service) Name() string {
    return "myservice"
}

func (s *Service) Start(ctx context.Context) error {
    ctx, span := s.tracer.Start(ctx, "myservice.Start")
    defer span.End()

    s.logger.Info("starting myservice")

    // Your startup logic
    if err := s.initialize(); err != nil {
        observability.SetSpanError(ctx, err)
        return err
    }

    s.logger.Info("myservice started")
    return nil
}

func (s *Service) Stop(ctx context.Context) error {
    ctx, span := s.tracer.Start(ctx, "myservice.Stop")
    defer span.End()

    s.logger.Info("stopping myservice")

    // Your shutdown logic
    if err := s.cleanup(); err != nil {
        observability.SetSpanError(ctx, err)
        return err
    }

    s.logger.Info("myservice stopped")
    return nil
}

func (s *Service) HealthCheck(ctx context.Context) error {
    ctx, span := s.tracer.Start(ctx, "myservice.HealthCheck")
    defer span.End()

    // Your health check logic
    if err := s.db.PingContext(ctx); err != nil {
        observability.SetSpanError(ctx, err)
        return err
    }

    return nil
}

// Ensure compliance
var _ runner.Service = (*Service)(nil)
var _ runner.HealthChecker = (*Service)(nil)
```

## Observability Integration

All runnable services support optional OpenTelemetry integration:

### Tracing

```go
// Create service with tracer
tracer := tel.Tracer("myapp")
service := eventbus.New(eventbus.WithTracer(tracer))

// Automatic spans for:
// - eventbus.Start
// - eventbus.Stop
// - eventbus.HealthCheck

// Span attributes include:
// - nats.url
// - stream.name
// - stream.max_bytes
// - stream.max_age
// - healthy (for health checks)
```

### Logging

```go
// Create service with logger
logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
service := eventbus.New(eventbus.WithLogger(logger))

// Automatic structured logs for:
// - Service startup
// - EventBus creation
// - Connection draining
// - Server shutdown
// - Errors and warnings
```

### Metrics (Future)

```go
// Future: Add metrics support
meter := tel.Meter("myapp")
service := eventbus.New(eventbus.WithMeter(meter))

// Planned metrics:
// - eventbus_start_duration_seconds
// - eventbus_shutdown_duration_seconds
// - eventbus_health_check_total
// - eventbus_health_check_failures_total
```

## Testing

Each runnable service includes comprehensive tests:

```bash
# Test embeddednats
go test github.com/plaenen/eventstore/pkg/runnable/embeddednats

# Test eventbus
go test github.com/plaenen/eventstore/pkg/runnable/eventbus

# All tests
go test github.com/plaenen/eventstore/pkg/runnable/...
```

## Design Principles

1. **Dependency Inversion**: Adapters depend on abstractions, not implementations
2. **Single Responsibility**: Each adapter does one thing (lifecycle management)
3. **Open/Closed**: Easy to add new services without modifying existing ones
4. **Interface Segregation**: Services implement only what they need
5. **No Circular Dependencies**: One-way dependency flow

## Package Dependencies

```
pkg/runnable/embeddednats
    → pkg/nats          (EmbeddedServer)
    → pkg/runner        (Service interface)
    → pkg/observability (tracing helpers)

pkg/runnable/eventbus
    → pkg/nats          (EmbeddedServer, EventBus, Config)
    → pkg/runner        (Service interface)
    → pkg/observability (tracing helpers)
```

Core packages (`pkg/nats`, `pkg/runner`, `pkg/observability`) have **no** dependencies on `pkg/runnable`.

## Future Services

Planned adapters:

- `pkg/runnable/httpserver` - HTTP server with graceful shutdown
- `pkg/runnable/grpcserver` - gRPC server adapter
- `pkg/runnable/projection` - Projection manager adapter
- `pkg/runnable/worker` - Background worker adapter

## See Also

- `pkg/runner` - Service lifecycle manager
- `pkg/nats` - NATS EventBus implementation
- `pkg/observability` - OpenTelemetry integration
- `examples/cmd/runner-nats` - Complete example

## License

Part of the eventstore framework.
