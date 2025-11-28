# Event Sourcing Framework for Go

An alpha version Event Sourcing and CQRS framework for Go with Protocol Buffers code generation, built-in observability, and flexible storage backends.

[![Go Version](https://img.shields.io/badge/go-1.25%2B-blue)](https://golang.org/dl/)
[![License](https://img.shields.io/badge/license-MIT-green)](LICENSE)

## ⚠️ Security Notice

**This project is in Beta.** Significant security features have been implemented, but production deployments require careful configuration.

### ✅ Security Improvements
- ✅ **Secure credentials management** - `pkg/security/credentials` with encryption support (AWS, GCP, Azure, Vault)
- ✅ **SQL injection protection** - Comprehensive input validation and parameterized queries
- ✅ **Error Handling** - `pkg/errorx` provides safe error propagation and sanitization patterns
- ✅ **Input validation** - Defense-in-depth validation across event store operations
- ✅ **Encrypted Storage** - Support for encrypted key stores and multi-tenant data

### 🛡️ Production Checklist
- ⚠️ **TLS Configuration** - Ensure NATS and Database connections use TLS (requires explicit setup)
- ⚠️ **Authorization** - Implement ABAC/RBAC using your preferred policy engine (e.g., OPA)
- ⚠️ **Rate Limiting** - Configure rate limits in `pkg/config` and ensure enforcement at the gateway level
- ⚠️ **Secret Management** - Use a production secret manager (AWS Secrets Manager, Vault) instead of local files

**📚 See [Security Credentials Guide](docs/SECURITY_CREDENTIALS.md) for configuration details.**


---

## Overview

This framework provides everything you need to build event-sourced systems in Go:

- **Type-safe code generation** from Protocol Buffers definitions
- **Clean CQRS patterns** with automatic command/query routing
- **Flexible projections** with built-in checkpoint management
- **Multiple storage backends** (SQLite/LibSQL: local, Turso cloud, embedded replica; PostgreSQL planned)
- **Event streaming** via NATS JetStream
- **Built-in observability** with OpenTelemetry integration
- **Service lifecycle management** for production deployments
- **Event analytics** for debugging and insights (automatic tracking, persisted in snapshots)
- **Snapshots** for 20-100x performance improvements
- **Event seeding** for migrations and bootstrapping (idempotent, deterministic)
- **Secure credentials** with AWS/GCP/Azure/Vault integration

## Quick Start

### Prerequisites

- Go 1.25+
- `buf` (for Protobuf generation)
- `protoc-gen-go`, `protoc-gen-connect-go`
- NATS server (for messaging)

### Installation

```bash
# Clone the repository
git clone https://github.com/plaenen/eventstore.git
cd eventstore

# Install dependencies
go mod download

# Generate code
task generate

# Run tests
task test
```

### Building Your First Application

This guide walks through building a scalable, multi-tenant application using Event Sourcing and CQRS.

#### 1. Define Service & Domain (Proto)

Define your service, commands, events, and aggregate state in `.proto` files.

**`proto/account/domain/v1/account.proto`** (Domain Model):
```protobuf
syntax = "proto3";
package account.domain.v1;

import "eventsourcing/options.proto";

// Aggregate State
message Account {
  option (eventsourcing.aggregate_root) = { id_field: "account_id" };
  string account_id = 1;
  string balance = 2;
  string status = 3;
}

// Events
message AccountOpenedEvent {
  option (eventsourcing.event) = { aggregate_name: "Account" };
  string account_id = 1;
  string owner_name = 2;
}

message MoneyDepositedEvent {
  option (eventsourcing.event) = { aggregate_name: "Account" };
  string account_id = 1;
  string amount = 2;
  string new_balance = 3;
}
```

**`proto/account/service/v1/account.proto`** (API Definition):
```protobuf
syntax = "proto3";
package account.service.v1;

import "eventsourcing/options.proto";
import "cqrs/options.proto";
import "account/domain/v1/account.proto";

service AccountCommandService {
  option (cqrs.service) = {
    service_type: SERVICE_TYPE_COMMAND
    generate_client: true
  };

  rpc OpenAccount(OpenAccountCommand) returns (OpenAccountResponse);
  rpc Deposit(DepositCommand) returns (DepositResponse);
}

message OpenAccountCommand { ... }
message OpenAccountResponse { ... }
message DepositCommand { ... }
message DepositResponse { ... }
```

#### 2. Generate Code

Run `buf generate` to create type-safe Go code, including:
*   `AccountAggregate`: Domain object with `Apply*` methods.
*   `AccountRepository`: For loading/saving aggregates.
*   `AccountEventApplier`: Interface for domain logic.
*   `AccountCommandServiceHandler`: Interface for service implementation.

#### 3. Implement Domain Logic (Appliers)

Implement `AccountEventApplier` to define how events mutate state. This is **pure domain logic**.

```go
type AccountAppliers struct{}

func (a *AccountAppliers) ApplyAccountOpenedEvent(agg *accountdomainv1.Account, e *accountdomainv1.AccountOpenedEvent) error {
    agg.AccountId = e.AccountId
    agg.Status = accountdomainv1.AccountStatus_ACCOUNT_STATUS_OPEN
    agg.Balance = "0"
    return nil
}

func (a *AccountAppliers) ApplyMoneyDepositedEvent(agg *accountdomainv1.Account, e *accountdomainv1.MoneyDepositedEvent) error {
    agg.Balance = e.NewBalance
    return nil
}
```

#### 4. Implement Command Handler

Implement `AccountCommandServiceHandler` to coordinate loading, validating, and saving.

```go
type AccountHandler struct {
    repo *accountservicev1.AccountRepository
}

func (h *AccountHandler) Deposit(ctx context.Context, cmd *accountservicev1.DepositCommand) (*accountservicev1.DepositResponse, error) {
    // 1. Validate Command
    if cmd.Amount <= 0 { return nil, fmt.Errorf("invalid amount") }

    // 2. Load & Mutate (with optimistic locking retry)
    err := h.repo.RetryOnConflict(cmd.AccountId, 3, func(agg *accountdomainv1.Account) error {
        // Business Rule Check
        if agg.Status != accountdomainv1.AccountStatus_ACCOUNT_STATUS_OPEN { return fmt.Errorf("account closed") }

        // Calculate new state & Emit Event
        event := &accountdomainv1.MoneyDepositedEvent{
            AccountId: cmd.AccountId,
            Amount: cmd.Amount,
            NewBalance: newBalance,
        }
        
        // Apply Event (updates in-memory state)
        return agg.ApplyMoneyDepositedEvent(event)
    })

    return &accountservicev1.DepositResponse{...}, err
}
```

#### 5. Create Projections (Read Models)

Projections listen to the event stream and update a read-optimized database.

```go
func (p *AccountProjection) HandleEvent(ctx context.Context, event *eventsourcing.Event) error {
    switch e := event.Payload.(type) {
    case *accountv1.AccountOpenedEvent:
        _, err := p.db.Exec("INSERT INTO accounts (id, balance) VALUES (?, ?)", e.AccountId, 0)
        return err
    case *accountv1.MoneyDepositedEvent:
        _, err := p.db.Exec("UPDATE accounts SET balance = ? WHERE id = ?", e.NewBalance, e.AccountId)
        return err
    }
    return nil
}
```

#### 6. Wiring It All Together

```go
func main() {
    // 1. Initialize Infrastructure
    nc, _ := nats.Connect("nats://localhost:4222")
    eventStore, _ := sqlite.NewEventStore(sqlite.WithDSN("events.db"))
    
    // 2. Initialize Components
    repo := accountservicev1.NewAccountRepository(eventStore, domain.NewAccountAppliers())
    handler := handlers.NewAccountHandler(repo)
    
    // 3. Start NATS Server
    server, _ := nats.NewServer(&nats.ServerConfig{Connection: nc})
    server.RegisterHandler("commands.account.deposit", handler.Deposit)
    server.Start(context.Background())
}
```

### Secure Credential Management

Use the credentials provider for secure authentication across cloud providers:

```go
import "github.com/plaenen/eventstore/pkg/security/credentials"

// Production: AWS Secrets Manager
provider, err := credentials.NewSecretProvider(ctx,
    "awskms://arn:aws:secretsmanager:us-east-1:123456789:secret:nats-creds")

// Get credentials with automatic caching
creds, err := provider.GetCredentials(ctx)

// Use with NATS
nc, err := nats.Connect(
    natsURL,
    nats.UserInfo(creds.Username, creds.Password),
)
defer provider.Close()
```

**Supported Backends:**
- AWS Secrets Manager
- GCP Secret Manager
- Azure Key Vault
- HashiCorp Vault
- Local files (development)
- Environment variables (simple cases)

**📚 See [Security Credentials Guide](docs/SECURITY_CREDENTIALS.md) for complete examples**

## Core Concepts

### Architecture

The framework follows clean architecture principles with clear separation of concerns:

```
┌─────────────────────────────────────────────────────────┐
│                    Application Layer                     │
│  ┌─────────────┐  ┌──────────────┐  ┌───────────────┐  │
│  │   Commands  │  │   Queries    │  │  Projections  │  │
│  └─────────────┘  └──────────────┘  └───────────────┘  │
└────────────────────────┬────────────────────────────────┘
                         │
┌────────────────────────┴────────────────────────────────┐
│                    Domain Layer                          │
│  ┌─────────────┐  ┌──────────────┐  ┌───────────────┐  │
│  │ Aggregates  │  │    Events    │  │   Commands    │  │
│  └─────────────┘  └──────────────┘  └───────────────┘  │
└────────────────────────┬────────────────────────────────┘
                         │
┌────────────────────────┴────────────────────────────────┐
│                Infrastructure Layer                      │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  │
│  │ Event Store  │  │   Messaging  │  │     CQRS     │  │
│  │  (SQLite)    │  │    (NATS)    │  │    (NATS)    │  │
│  └──────────────┘  └──────────────┘  └──────────────┘  │
└─────────────────────────────────────────────────────────┘
```

### Package Structure

```
pkg/
├── domain/           # Pure domain types (Event, Command, Aggregate)
├── store/            # Event persistence (EventStore, Repository, Snapshots)
│   └── sqlite/      # SQLite implementation
├── cqrs/            # Command/Query handling (request/reply)
│   └── nats/        # NATS implementation
├── messaging/       # Event publishing/subscription (pub/sub)
│   └── nats/        # NATS JetStream implementation
├── infrastructure/  # Pure infrastructure utilities
│   └── nats/        # Embedded NATS server
├── observability/   # OpenTelemetry integration
├── runtime/         # Service lifecycle management
└── multitenancy/    # Multi-tenant support
```

## Key Features

### 1. Code Generation

Generate type-safe, idiomatic Go code from Protocol Buffers:

```bash
# Generate everything
buf generate

# Generated files include:
# - Aggregate implementations with event sourcing
# - Command/query handlers
# - Client SDKs
# - Event appliers
# - NATS service integrations
```

### 2. Projections

Build read models with automatic transaction and checkpoint management:

```go
projection, err := sqlite.NewSQLiteProjectionBuilder(
    "account-balance",
    db,
    checkpointStore,
    eventStore,
).
    WithSchema(func(ctx context.Context, db *sql.DB) error {
        _, err := db.Exec(`CREATE TABLE IF NOT EXISTS account_balance (...)`)
        return err
    }).
    On(accountv1.OnAccountOpened(func(ctx context.Context, event *accountv1.AccountOpenedEvent, envelope *domain.EventEnvelope) error {
        tx, _ := sqlite.TxFromContext(ctx)
        _, err := tx.Exec("INSERT INTO account_balance ...")
        return err
    })).
    Build()
```

### 3. Event Streaming

Real-time event processing with NATS JetStream:

```go
// Publish events
bus, _ := natseventbus.NewEventBus(config)
bus.Publish(events)

// Subscribe to events
filter := eventsourcing.EventFilter{
    AggregateTypes: []string{"Account"},
}
bus.Subscribe(filter, func(event *eventsourcing.EventEnvelope) error {
    // Handle event
    return nil
})
```

### 4. Observability

Built-in OpenTelemetry support for traces and metrics:

```go
tel, _ := observability.Init(ctx, observability.Config{
    ServiceName:     "account-service",
    ServiceVersion:  "1.0.0",
    TraceExporter:   exporter,
    TraceSampleRate: 1.0,
})
defer tel.Shutdown(ctx)

// Automatic tracing for commands, queries, and events
```

### 5. Service Management

Production-ready service lifecycle management:

```go
runner := runner.New(
    []runner.Service{
        eventBusService,
        commandService,
        projectionService,
    },
    runner.WithLogger(logger),
    runner.WithShutdownTimeout(30 * time.Second),
)

// Handles SIGTERM/SIGINT gracefully
runner.Run(ctx)
```

### 6. Database Options

Multiple deployment modes for different use cases:

#### SQLite (Local Development)
```go
eventStore, err := sqlite.NewEventStore(
    sqlite.WithFilename("events.db"),
)
```

#### LibSQL Remote (Turso Cloud)
```go
eventStore, err := sqlite.NewEventStore(
    sqlite.WithLibSQLRemote(
        "libsql://your-db.turso.io",
        os.Getenv("TURSO_AUTH_TOKEN"),
    ),
)
```

#### LibSQL Embedded Replica (Local-First + Cloud Sync)
```go
eventStore, err := sqlite.NewEventStore(
    sqlite.WithLibSQLEmbeddedReplica(
        "./local.db",
        "libsql://your-db.turso.io",
        os.Getenv("TURSO_AUTH_TOKEN"),
    ),
)
```

**📚 See [LibSQL Usage Guide](pkg/store/sqlite/LIBSQL_USAGE.md) for complete configuration options**

### 7. Event Analytics

Automatic event tracking for debugging and insights:

```go
// Load aggregate
order, _ := repo.Load("order-123")

// Get analytics (automatically tracked)
analytics := order.Analytics()
fmt.Printf("Total events: %d\n", analytics.TotalEvents)
fmt.Printf("OrderPlaced: %d times\n", analytics.GetCount("OrderPlaced"))

// Detailed stats with timestamps
stats := analytics.GetStats("OrderPlaced")
fmt.Printf("First: %s, Last: %s\n", stats.FirstApplied, stats.LastApplied)

// Event distribution analysis
distribution := analytics.GetDistribution()
for eventType, pct := range distribution {
    fmt.Printf("%s: %.1f%%\n", eventType, pct)
}
```

**Features:**
- Automatic tracking during event replay
- Persisted in snapshots
- No performance overhead
- Useful for debugging and optimization

**📚 See [Event Analytics Guide](docs/EVENT_ANALYTICS_GUIDE.md)**

### 8. Snapshots for Performance

Optimize aggregate loading with automatic snapshots:

```go
// Enable snapshots
snapshotStore := sqlite.NewSnapshotStore(eventStore.DB())
repo := store.NewRepository(...).WithSnapshotStore(snapshotStore)

// Normal loading (uses snapshots automatically)
order, _ := repo.Load("order-123") // 20-100x faster!

// Save snapshots periodically
if order.Version() % 100 == 0 {
    repo.SaveSnapshot(order)
}
```

**Performance Gains:**
- 10,000 events: 500ms → 25ms (20x faster)
- 100,000 events: 5,000ms → 50ms (100x faster)
- Analytics automatically preserved in snapshots

**📚 See [Snapshot Guide](docs/SNAPSHOT_GUIDE.md)**

### 9. Event Seeding for Migrations

Deterministic, idempotent event seeding for migrations and bootstrapping:

```go
// Bootstrap admin user
admin := NewUser("admin-001")
admin.Create("admin@example.com", "Admin")
admin.AssignRole("super_admin")

// Seed with default options (idempotent)
opts := domain.DefaultSeedOptions()
opts.CustomTags = map[string]string{
    "migration": "v1.0.0",
    "source":    "bootstrap",
}

result, err := repo.SeedAggregate(admin, 0, opts)
fmt.Printf("Saved: %d, Skipped: %d\n", result.Saved, result.Skipped)
```

**Features:**
- Idempotent (safe to run multiple times)
- Deterministic ID generation
- Constraint ownership checking
- Custom metadata for data lineage

**Use Cases:**
- Database migrations (historical data import)
- Bootstrap data (admin users, system configs)
- Test fixtures (deterministic test data)

**📚 See [Event Seeding Guide](docs/SEEDING_GUIDE.md)**

## Examples

### Complete Examples

See the `examples/` directory for complete, runnable examples:

- **[bankaccount-observability](examples/cmd/bankaccount-observability/)** - Full CQRS with observability
- **[generic-projection](examples/cmd/generic-projection/)** - Cross-domain projections
- **[projection-migrations](examples/cmd/projection-migrations/)** - Schema evolution
- **[sqlite-projection](examples/cmd/sqlite-projection/)** - Basic projections
- **[projection-nats](examples/cmd/projection-nats/)** - Real-time event processing

Run any example:

```bash
go run ./examples/cmd/bankaccount-observability
```

## Documentation

### Getting Started

- **[Examples Guide](examples/README.md)** - Understanding examples structure
- **[Release Notes](docs/releases/)** - What's new in each version

### Guides

- **[Projection Patterns](docs/guides/projections.md)** - Building read models (Generic, SQLite, NATS)
- **[Event Upcasting](docs/guides/event-upcasting.md)** - Schema evolution and backward compatibility
- **[SDK Generation](docs/guides/sdk-generation.md)** - Generating unified SDKs



- **[Domain Layer](pkg/domain/)** - Core interfaces for Aggregates, Events, and Commands. Defines the `AggregateRoot` and `EventEnvelope` types.
- **[Event Store](pkg/store/)** - Persistence layer for storing events. Includes:
    - `pkg/store/sqlite`: SQLite/LibSQL implementation with support for local files, Turso, and embedded replicas.
- **[CQRS](pkg/cqrs/)** - Command Query Responsibility Segregation framework.
    - `pkg/cqrs/nats`: NATS-based transport for command routing and query handling.
- **[Messaging](pkg/messaging/)** - Event publishing and subscription infrastructure.
    - `pkg/messaging/nats`: JetStream implementation for reliable event streaming.
- **[Identity](pkg/identity/)** - Identity and Access Management (IAM) service.
    - `pkg/identity/store/sqlite`: Secure credential storage using SQLite.
- **[Runtime Services](pkg/runtime/)** - Service lifecycle management, graceful shutdown, and dependency injection.
- **[Observability](pkg/observability/)** - OpenTelemetry integration for distributed tracing and metrics.
- **[Security](pkg/security/)** - Security utilities including encryption and credential management.


### All Documentation

See the **[Documentation Index](docs/README.md)** for a complete guide to all documentation, organized by topic and learning path.

## Contributing

We welcome contributions! Please see our [Contributing Guide](CONTRIBUTING.md) for details on:

- Setting up your development environment
- Code style and conventions
- Testing requirements
- Pull request process

## Community

- **GitHub Issues** - Bug reports and feature requests
- **GitHub Discussions** - Questions and community support

## License

MIT License - see [LICENSE](LICENSE) file for details

## Acknowledgments

Built with:
- [Protocol Buffers](https://protobuf.dev/) - Schema definition
- [NATS](https://nats.io/) - Event streaming
- [OpenTelemetry](https://opentelemetry.io/) - Observability
- [SQLite](https://sqlite.org/) - Event storage

---

**Ready to build event-sourced systems?** Explore the [examples](examples/) to get started!
