# Proposed Package Structure (Clean Slate)

## Philosophy

Organize by **domain concepts** and **architectural patterns**, not by transport mechanism.

## Proposed Structure

```
pkg/
  domain/                    # Core DDD/ES concepts (pure Go, no dependencies)
    ├── aggregate.go         # Aggregate root interface
    ├── event.go             # Domain event types
    ├── command.go           # Command envelope
    ├── metadata.go          # Metadata types
    └── errors.go            # Domain errors

  store/                     # Event Storage (Event Sourcing pattern)
    ├── eventstore.go        # EventStore interface
    ├── repository.go        # Repository[T Aggregate] generic
    ├── snapshot.go          # Snapshot support
    ├── sqlite/              # SQLite implementation
    │   ├── store.go
    │   └── store_test.go
    └── memory/              # In-memory implementation (testing)
        └── store.go

  cqrs/                      # CQRS pattern (Commands & Queries)
    ├── handler.go           # Handler interfaces
    ├── server.go            # Server interface (handles requests)
    ├── transport.go         # Transport interface (sends requests)
    ├── middleware.go        # Middleware support
    └── nats/                # NATS implementation
        ├── server.go        # NATS request/reply server
        ├── transport.go     # NATS request/reply client
        └── commandbus.go    # Distributed command bus

  messaging/                 # Event-Driven architecture (Pub/Sub)
    ├── publisher.go         # Publisher interface
    ├── subscriber.go        # Subscriber interface
    ├── projection.go        # Projection base types
    └── nats/                # NATS JetStream implementation
        ├── publisher.go
        ├── subscriber.go
        └── eventbus.go      # Combined pub/sub

  # Infrastructure & Cross-cutting
  infrastructure/
    └── nats/                # Pure NATS utilities
        ├── embedded.go      # Embedded server for testing
        └── config.go        # Connection helpers

  observability/             # OpenTelemetry integration
    ├── telemetry.go
    ├── middleware.go
    └── sqlite/              # SQLite exporters

  runtime/                   # Service lifecycle
    ├── service.go           # Service interface
    ├── runner.go            # Runner implementation
    └── adapters/            # Adapters for services
        ├── nats.go
        ├── eventbus.go
        └── http.go
```

## Key Changes

### 1. `pkg/domain` - Pure Domain Concepts

**What it contains:**
- Aggregate interface
- Event types (Event, EventEnvelope)
- Command types (Command, CommandEnvelope, CommandMetadata)
- Metadata types
- Domain errors (AppError)

**Why:**
- Zero dependencies (pure Go)
- Shared by all other packages
- Clear ubiquitous language
- Easy to reason about

**Example:**
```go
// pkg/domain/aggregate.go
type Aggregate interface {
    AggregateID() string
    AggregateType() string
    Version() int
}

// pkg/domain/event.go
type Event struct {
    ID            string
    AggregateID   string
    AggregateType string
    EventType     string
    Data          []byte
    Version       int
    Timestamp     time.Time
}

// pkg/domain/command.go
type CommandMetadata struct {
    CommandID   string
    AggregateID string
    Timestamp   time.Time
    TenantID    string
    UserID      string
    Custom      map[string]string
}
```

### 2. `pkg/store` - Event Storage (Event Sourcing)

**What it contains:**
- EventStore interface (Append, Load, LoadEvents)
- Repository[T Aggregate] generic interface
- Snapshot support
- Implementations: sqlite/, memory/

**Why:**
- Clear separation: this is about **persistence**
- Event Sourcing pattern is isolated
- Easy to add new storage backends (Postgres, DynamoDB, etc.)

**Example:**
```go
// pkg/store/eventstore.go
type EventStore interface {
    Append(ctx context.Context, events []*domain.Event) error
    Load(ctx context.Context, aggregateID string) ([]*domain.Event, error)
    LoadFrom(ctx context.Context, aggregateID string, version int) ([]*domain.Event, error)
}

// pkg/store/repository.go
type Repository[T domain.Aggregate] interface {
    Load(ctx context.Context, id string) (T, error)
    Save(ctx context.Context, aggregate T, events []*domain.Event) error
}

// Implementation
// pkg/store/sqlite/store.go
type EventStore struct { ... }
```

### 3. `pkg/cqrs` - CQRS Pattern

**What it contains:**
- Server interface (handles incoming commands/queries)
- Transport interface (sends commands/queries to server)
- Handler types
- Middleware support
- Implementation: nats/

**Why:**
- CQRS is about **request/reply** pattern
- Separates command/query handling from event storage
- Transport-agnostic (can add HTTP, gRPC, etc.)

**Example:**
```go
// pkg/cqrs/server.go
type Server interface {
    RegisterHandler(subject string, handler HandlerFunc) error
    Start(ctx context.Context) error
    Close() error
}

// pkg/cqrs/transport.go
type Transport interface {
    Request(ctx context.Context, subject string, req proto.Message) (*Response, error)
    Close() error
}

// pkg/cqrs/nats/server.go - Implementation
type Server struct { ... }
```

### 4. `pkg/messaging` - Event-Driven Architecture (Pub/Sub)

**What it contains:**
- Publisher interface
- Subscriber interface
- Projection base types
- Implementation: nats/

**Why:**
- Clear name: "messaging" = pub/sub, not request/reply
- Separates event **distribution** from event **storage**
- Events from store can be published here
- Projections subscribe here

**Example:**
```go
// pkg/messaging/publisher.go
type Publisher interface {
    Publish(ctx context.Context, event *domain.Event) error
    PublishBatch(ctx context.Context, events []*domain.Event) error
}

// pkg/messaging/subscriber.go
type Subscriber interface {
    Subscribe(ctx context.Context, subject string, handler EventHandler) error
    Close() error
}

// pkg/messaging/projection.go
type Projection interface {
    Name() string
    Handle(ctx context.Context, event *domain.Event) error
}

// pkg/messaging/nats/eventbus.go - Combined implementation
type EventBus struct {
    // Implements both Publisher and Subscriber
}
```

### 5. `pkg/infrastructure/nats` - Pure NATS

**What it contains:**
- Embedded server for testing
- Connection helpers
- Configuration utilities

**Why:**
- Pure infrastructure
- No domain logic
- Used by cqrs/nats and messaging/nats

### 6. `pkg/runtime` - Service Lifecycle

**What it contains:**
- Service interface (Start, Stop, HealthCheck)
- Runner (manages multiple services)
- Adapters for different service types

**Why:**
- Better name than "runnable"
- Clear separation from domain logic
- All lifecycle management in one place

## Package Dependencies

```
domain/
  ↓
store/ ──→ domain/
  ↓
cqrs/ ──→ domain/
  ↓
messaging/ ──→ domain/
  ↓
infrastructure/nats (pure infrastructure, no domain deps)
  ↓
runtime/ ──→ all packages (orchestration layer)
```

## Benefits of This Structure

### 1. Clear Separation of Concerns
- **Storage** (store/) vs **Messaging** (messaging/) vs **Request/Reply** (cqrs/)
- Each package has ONE responsibility

### 2. Better Names
- `messaging` instead of `eventbus` (pub/sub is messaging)
- `store` instead of `eventsourcing` (focuses on storage aspect)
- `runtime` instead of `runnable` (more standard term)

### 3. Transport-Agnostic
- `cqrs/` can have: nats/, http/, grpc/
- `messaging/` can have: nats/, kafka/, rabbitmq/
- `store/` can have: sqlite/, postgres/, dynamodb/

### 4. Testing
- Pure domain types in `domain/` (easy to test)
- In-memory implementations: `store/memory/`
- Embedded NATS: `infrastructure/nats/`

### 5. Discoverability
- Want to store events? → `pkg/store`
- Want to handle commands? → `pkg/cqrs`
- Want to publish events? → `pkg/messaging`
- Want domain types? → `pkg/domain`

## Migration Path

### Phase 1: Create New Structure
1. Create `pkg/domain/` and move types from `pkg/eventsourcing`
2. Create `pkg/store/` and move EventStore + Repository
3. Rename `pkg/eventbus/` → `pkg/messaging/`
4. Keep `pkg/cqrs/` as-is
5. Move `pkg/nats/` → `pkg/infrastructure/nats/`

### Phase 2: Update Implementations
1. Update `pkg/store/sqlite/` imports
2. Update `pkg/cqrs/nats/` imports
3. Update `pkg/messaging/nats/` imports
4. Update examples

### Phase 3: Runtime
1. Rename `pkg/runnable/` → `pkg/runtime/`
2. Move services to `pkg/runtime/adapters/`

## Example Usage After Refactor

### Command Handler (CQRS)
```go
import (
    "github.com/plaenen/eventstore/pkg/domain"
    "github.com/plaenen/eventstore/pkg/store"
    "github.com/plaenen/eventstore/pkg/cqrs"
    cqrsnats "github.com/plaenen/eventstore/pkg/cqrs/nats"
)

// Repository uses store
repo := store.NewRepository[*Account](eventStore, NewAccount)

// CQRS server handles commands
server, _ := cqrsnats.NewServer(config)
commandService := accountv1.NewAccountCommandServiceServer(server, handler)
commandService.Start(ctx)
```

### Event Publishing (Messaging)
```go
import (
    "github.com/plaenen/eventstore/pkg/domain"
    "github.com/plaenen/eventstore/pkg/messaging"
    natsmsg "github.com/plaenen/eventstore/pkg/messaging/nats"
)

// Create publisher
publisher, _ := natsmsg.NewPublisher(config)

// Publish domain events
event := &domain.Event{...}
publisher.Publish(ctx, event)
```

### Projection (Messaging Subscriber)
```go
import (
    "github.com/plaenen/eventstore/pkg/domain"
    "github.com/plaenen/eventstore/pkg/messaging"
    natsmsg "github.com/plaenen/eventstore/pkg/messaging/nats"
)

// Create subscriber
subscriber, _ := natsmsg.NewSubscriber(config)

// Subscribe to events
subscriber.Subscribe(ctx, "events.account.*", func(ctx context.Context, event *domain.Event) error {
    // Handle event for projection
    return nil
})
```

### Testing with Embedded NATS
```go
import (
    infraNats "github.com/plaenen/eventstore/pkg/infrastructure/nats"
)

srv, _ := infraNats.StartEmbeddedServer()
defer srv.Shutdown()

// Use srv.URL() for cqrs/nats and messaging/nats
```

## Comparison: Current vs Proposed

| Aspect | Current | Proposed |
|--------|---------|----------|
| Event types | `pkg/eventsourcing` | `pkg/domain` |
| Command types | `pkg/eventsourcing` | `pkg/domain` |
| EventStore | `pkg/eventsourcing` + `pkg/sqlite` | `pkg/store` + `pkg/store/sqlite` |
| Repository | `pkg/eventsourcing` | `pkg/store` |
| CQRS Server | `pkg/cqrs/nats` | `pkg/cqrs/nats` (same) |
| CQRS Transport | `pkg/cqrs/nats` | `pkg/cqrs/nats` (same) |
| Event Bus | `pkg/eventbus/nats` | `pkg/messaging/nats` |
| Embedded NATS | `pkg/nats` | `pkg/infrastructure/nats` |
| Services | `pkg/runnable` | `pkg/runtime/adapters` |

## Summary

**Core Insight:** The current `pkg/eventsourcing` mixes:
1. Domain types (Event, Command, Aggregate)
2. Storage interfaces (EventStore, Repository)
3. CQRS interfaces (Server, Transport)

**Solution:** Split into:
1. `pkg/domain` - Pure domain types
2. `pkg/store` - Event storage (Event Sourcing persistence)
3. `pkg/cqrs` - Commands/Queries (request/reply)
4. `pkg/messaging` - Events (pub/sub)

**Result:**
- ✅ Each package has ONE clear purpose
- ✅ Better discoverability
- ✅ Transport-agnostic
- ✅ Easier to add new implementations
- ✅ Clear dependencies (domain → store → cqrs/messaging)
