# Event Sourcing Framework for Go

An alpha version Event Sourcing and CQRS framework for Go with Protocol Buffers code generation, built-in observability, and flexible storage backends.

[![Go Version](https://img.shields.io/badge/go-1.25%2B-blue)](https://golang.org/dl/)
[![License](https://img.shields.io/badge/license-MIT-green)](LICENSE)

## Overview

This framework provides everything you need to build event-sourced systems in Go:

- **Type-safe code generation** from Protocol Buffers definitions
- **Clean CQRS patterns** with automatic command/query routing
- **Flexible projections** with built-in checkpoint management
- **Multiple storage backends** (SQLite, with PostgreSQL planned)
- **Event streaming** via NATS JetStream
- **Built-in observability** with OpenTelemetry integration
- **Service lifecycle management** for production deployments

## Quick Start

### Prerequisites

- Go 1.25 or later
- Protocol Buffers compiler (`protoc`)
- Task runner (`go install github.com/go-task/task/v3/cmd/task@latest`)
- Buf (`go install github.com/bufbuild/buf/cmd/buf@latest`)

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

### Your First Event-Sourced Aggregate

**1. Define your proto schema** (`proto/account/v1/account.proto`):

```protobuf
syntax = "proto3";
package account.v1;

import "eventsourcing/options.proto";

// Declare the aggregate
service AccountCommandService {
  option (eventsourcing.service) = {
    aggregate_name: "Account"
    aggregate_root_message: "Account"
  };
  rpc OpenAccount(OpenAccountCommand) returns (OpenAccountResponse);
  rpc Deposit(DepositCommand) returns (DepositResponse);
}

// Commands
message OpenAccountCommand {
  string account_id = 1;
  string owner_name = 2;
  string initial_balance = 3;
}

// Events
message AccountOpenedEvent {
  option (eventsourcing.event) = {aggregate_name: "Account"};
  string account_id = 1;
  string owner_name = 2;
  string initial_balance = 3;
  int64 timestamp = 4;
}

// Aggregate state
message Account {
  option (eventsourcing.aggregate_root) = {id_field: "account_id"};
  string account_id = 1;
  string owner_name = 2;
  string balance = 3;
  AccountStatus status = 4;
}

enum AccountStatus {
  ACCOUNT_STATUS_UNSPECIFIED = 0;
  ACCOUNT_STATUS_OPEN = 1;
  ACCOUNT_STATUS_CLOSED = 2;
}
```

**2. Generate code**:

```bash
buf generate
```

**3. Implement business logic** (`domain/account.go`):

```go
package domain

import (
    "context"
    "math/big"
    "time"

    accountv1 "github.com/your-org/your-app/gen/pb/account/v1"
    "github.com/plaenen/eventstore/pkg/eventsourcing"
)

// Event appliers - update aggregate state
func (a *accountv1.AccountAggregate) ApplyAccountOpenedEvent(e *accountv1.AccountOpenedEvent) error {
    a.AccountId = e.AccountId
    a.OwnerName = e.OwnerName
    a.Balance = e.InitialBalance
    a.Status = accountv1.AccountStatus_ACCOUNT_STATUS_OPEN
    return nil
}

// Command handler - business logic
type AccountHandler struct {
    repo Repository[*accountv1.AccountAggregate]
}

func (h *AccountHandler) OpenAccount(
    ctx context.Context,
    cmd *accountv1.OpenAccountCommand,
) (*accountv1.OpenAccountResponse, *eventsourcing.AppError) {
    // Validation
    if cmd.OwnerName == "" {
        return nil, &eventsourcing.AppError{
            Code: "INVALID_OWNER",
            Message: "Owner name is required",
        }
    }

    // Create aggregate
    account := accountv1.NewAccount(cmd.AccountId)

    // Create and emit event
    event := &accountv1.AccountOpenedEvent{
        AccountId: cmd.AccountId,
        OwnerName: cmd.OwnerName,
        InitialBalance: cmd.InitialBalance,
        Timestamp: time.Now().Unix(),
    }

    account.AggregateRoot.ApplyChange(
        event,
        "accountv1.AccountOpenedEvent",
        eventsourcing.EventMetadata{},
    )

    // Save
    if _, err := h.repo.Save(ctx, account); err != nil {
        return nil, &eventsourcing.AppError{
            Code: "SAVE_FAILED",
            Message: err.Error(),
        }
    }

    return &accountv1.OpenAccountResponse{
        AccountId: account.AccountId,
    }, nil
}
```

**4. Wire it up**:

```go
package main

import (
    "context"
    "log"

    "github.com/plaenen/eventstore/pkg/store/sqlite"
    accountv1 "github.com/your-org/your-app/gen/pb/account/v1"
    "github.com/your-org/your-app/domain"
)

func main() {
    ctx := context.Background()

    // Setup event store
    eventStore, err := sqlite.NewEventStore(
        sqlite.WithDSN("events.db"),
    )
    if err != nil {
        log.Fatal(err)
    }
    defer eventStore.Close()

    // Create repository
    repo := sqlite.NewRepository[*accountv1.AccountAggregate](
        eventStore,
        accountv1.NewAccount,
    )

    // Create handler
    handler := &domain.AccountHandler{repo: repo}

    // Execute command
    cmd := &accountv1.OpenAccountCommand{
        AccountId: "acc-001",
        OwnerName: "Alice",
        InitialBalance: "1000.00",
    }

    response, appErr := handler.OpenAccount(ctx, cmd)
    if appErr != nil {
        log.Fatal(appErr)
    }

    log.Printf("Account opened: %s", response.AccountId)
}
```

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

### Package Documentation

- [Domain Layer](pkg/domain/) - Core domain types
- [Event Store](pkg/store/) - Event persistence
- [CQRS](pkg/cqrs/) - Command/Query handling
- [Messaging](pkg/messaging/) - Event pub/sub
- [Runtime Services](pkg/runtime/) - Service lifecycle
- [Observability](pkg/observability/) - OpenTelemetry

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
