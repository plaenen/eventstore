# Event Sourcing Framework for Go

[![Go Version](https://img.shields.io/badge/Go-1.25%2B-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/plaenen/eventsourcing)](https://goreportcard.com/report/github.com/plaenen/eventsourcing)

A production-ready, type-safe CQRS and Event Sourcing framework for Go with code generation from Protocol Buffers.

## ✨ Features

### Core Capabilities
- **🔒 Type-Safe** - Leverages Go generics for compile-time safety across aggregates and repositories
- **⚡ Pure Go** - Zero CGo dependencies using modernc.org/sqlite for portability
- **🎯 Code Generation** - Auto-generate aggregates, handlers, and repositories from proto files
- **🔄 Hybrid Idempotency** - Three-layer protection: command deduplication, deterministic event IDs, and idempotent projections

### Event Sourcing
- **📦 SQLite Event Store** - ACID transactions with WAL mode, optimistic concurrency via versioning
- **📸 Snapshots** - Performance optimization with multi-version snapshot storage and configurable strategies
- **🔗 Unique Constraints** - Database-enforced uniqueness integrated with event sourcing (claim/release)
- **⏱️ Command TTL** - Configurable time-to-live for command deduplication (default 7 days)

### Distribution & Integration
- **🚀 NATS JetStream** - Distributed event bus with at-least-once delivery and consumer groups
- **📡 NATS Command Bus** - Distributed command processing with request-reply pattern (Production mode)
- **🎁 Unified SDK** - Single client for commands/events/queries with Dev/Prod modes
- **🤖 Auto-Generated Clients** - Type-safe SDK clients generated from proto definitions
- **📊 Projection Management** - Hybrid approach using EventBus for real-time updates and EventStore for rebuilds
- **🔌 Middleware Pipeline** - Logging (slog), validation, OpenTelemetry tracing, RBAC authorization, panic recovery
- **🌐 Connect RPC** - Modern RPC with HTTP/JSON and gRPC support via connectrpc.com

## 📋 Table of Contents

- [Quick Start](#-quick-start)
- [Architecture](#-architecture)
- [Core Concepts](#-core-concepts)
- [Code Generation](#-code-generation)
- [Middleware](#-middleware)
- [Testing](#-testing)
- [Performance](#-performance)
- [Examples](#-examples)
- [Best Practices](#-best-practices)
- [Contributing](#-contributing)

## 🚀 Quick Start

### Prerequisites

**Required:**
- Go 1.25.0 or higher
- [Task](https://taskfile.dev) - Build automation tool

**Install Task:**

```bash
# macOS
brew install go-task

# Linux/WSL
sh -c "$(curl --location https://taskfile.dev/install.sh)" -- -d -b /usr/local/bin

# Or use Go
go install github.com/go-task/task/v3/cmd/task@latest
```

### Installation

```bash
# Clone repository
git clone https://github.com/plaenen/eventsourcing.git
cd eventsourcing

# Complete setup (install dependencies, generate code, run tests)
task dev:setup

# Show all available tasks
task --list
```

**Add to your project:**

```bash
go get github.com/plaenen/eventsourcing
```

### 1️⃣ Define Your Domain (Proto)

Create your domain model using Protocol Buffers with event sourcing annotations:

```protobuf
syntax = "proto3";

package account.v1;

import "eventsourcing/options.proto";

// Command service defines write operations
service AccountCommandService {
  option (eventsourcing.aggregate_name) = "Account";

  rpc OpenAccount(OpenAccountCommand) returns (OpenAccountResponse);
  rpc Deposit(DepositCommand) returns (DepositResponse);
  rpc Withdraw(WithdrawCommand) returns (WithdrawResponse);
}

// Query service defines read operations
service AccountQueryService {
  rpc GetAccount(GetAccountRequest) returns (AccountView);
  rpc ListAccounts(ListAccountsRequest) returns (ListAccountsResponse);
}

// 🆕 Aggregate State - Single source of truth for aggregate structure
message Account {
  option (eventsourcing.aggregate_root) = {
    id_field: "account_id"
    type_name: "Account"
  };

  string account_id = 1;
  string owner_name = 2;
  string balance = 3;
  AccountStatus status = 4;
}

// Commands represent intentions
message OpenAccountCommand {
  option (eventsourcing.aggregate_options) = {
    aggregate: "Account"
    produces_events: "AccountOpenedEvent"
    unique_constraints: {
      index_name: "account_id"
      field: "account_id"
      operation: CONSTRAINT_OPERATION_CLAIM
    }
  };

  string account_id = 1;
  string owner_name = 2;
  string initial_balance = 3;
}

message OpenAccountResponse {
  string account_id = 1;
  int64 version = 2;
}

// Events represent facts (past tense)
message AccountOpenedEvent {
  option (eventsourcing.event_options) = {
    aggregate: "Account"
    applies_to_state: ["account_id", "owner_name", "balance"]
    field_mapping: {
      key: "initial_balance"
      value: "balance"
    }
  };

  string account_id = 1;
  string owner_name = 2;
  string initial_balance = 3;  // Maps to "balance" on aggregate
  int64 timestamp = 4;
}

// Views represent read models
message AccountView {
  string account_id = 1;
  string owner_name = 2;
  string balance = 3;
  AccountStatus status = 4;
  int64 version = 5;
}

enum AccountStatus {
  ACCOUNT_STATUS_UNSPECIFIED = 0;
  ACCOUNT_STATUS_OPEN = 1;
  ACCOUNT_STATUS_CLOSED = 2;
}
```

### 2️⃣ Generate Code

```bash
# Generate all proto code and SQLC queries
task generate

# Or manually with buf
cd examples
buf generate
```

The code generator creates:
- ✅ `AccountAggregate` struct embedding the proto `Account` message
- ✅ Event emitter helper methods (`EmitAccountOpenedEvent`, etc.)
- ✅ Event applier methods that update aggregate state
- ✅ Type-safe repository (`AccountRepository`)
- ✅ **`AccountClient` SDK** with type-safe command/query methods
- ✅ Automatic snapshot serialization via proto marshal
- ✅ Event serialization/deserialization helpers

### 3️⃣ Implement Business Logic

Add your business rules to the generated aggregate:

```go
package domain

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/plaenen/eventsourcing/pkg/eventsourcing"
	accountv1 "github.com/yourorg/project/pb/account/v1"
)

// Generated by protoc-gen-eventsourcing:
// - Account aggregate struct
// - AccountRepository
// - Method stubs for OpenAccount, Deposit, Withdraw

// OpenAccount implements business logic for opening an account
func (a *Account) OpenAccount(ctx context.Context, cmd *accountv1.OpenAccountCommand, metadata eventsourcing.EventMetadata) error {
	// Business validation
	balance := new(big.Float)
	if _, ok := balance.SetString(cmd.InitialBalance); !ok {
		return fmt.Errorf("invalid initial balance: %s", cmd.InitialBalance)
	}
	if balance.Cmp(big.NewFloat(0)) < 0 {
		return fmt.Errorf("initial balance cannot be negative")
	}

	// Create domain event
	event := &accountv1.AccountOpenedEvent{
		AccountId:      cmd.AccountId,
		OwnerName:      cmd.OwnerName,
		InitialBalance: cmd.InitialBalance,
		Timestamp:      time.Now().Unix(),
	}

	// Apply event with unique constraint (account_id must be unique)
	return a.EmitAccountOpenedEvent(event, metadata)
}

func (a *Account) Deposit(ctx context.Context, cmd *accountv1.DepositCommand, metadata eventsourcing.EventMetadata) error {
	// Validation
	if a.Status != accountv1.AccountStatus_ACCOUNT_STATUS_OPEN {
		return fmt.Errorf("account is not open")
	}

	amount := new(big.Float)
	if _, ok := amount.SetString(cmd.Amount); !ok || amount.Cmp(big.NewFloat(0)) <= 0 {
		return fmt.Errorf("invalid deposit amount: %s", cmd.Amount)
	}

	// Calculate new balance
	currentBalance := new(big.Float)
	currentBalance.SetString(a.Balance)
	newBalance := new(big.Float).Add(currentBalance, amount)

	event := &accountv1.MoneyDepositedEvent{
		AccountId:  cmd.AccountId,
		Amount:     cmd.Amount,
		NewBalance: newBalance.String(),
		Timestamp:  time.Now().Unix(),
	}

	return a.EmitMoneyDepositedEvent(event, metadata)
}
```

### 4️⃣ Use the Unified SDK (Recommended)

The framework automatically generates a **unified SDK** that aggregates all service clients into a single entry point:

```go
package main

import (
	"context"
	"log"

	"github.com/plaenen/eventsourcing/pkg/unifiedsdk"
	"github.com/plaenen/eventsourcing/pkg/sdk"
	accountv1 "github.com/yourorg/project/pb/account/v1"
)

func main() {
	// Create unified SDK - single entry point!
	s, err := unifiedsdk.New(
		unifiedsdk.WithMode(sdk.DevelopmentMode),
		unifiedsdk.WithSQLiteDSN("./events.db"),
		unifiedsdk.WithWALMode(true),
	)
	if err != nil {
		log.Fatalf("Failed to create SDK: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// Access all services via properties!
	_, err = s.Account.OpenAccount(ctx, &accountv1.OpenAccountCommand{
		AccountId:      "acc-123",
		OwnerName:      "Alice Johnson",
		InitialBalance: "1000.00",
	}, "user-alice")
	if err != nil {
		log.Fatalf("Failed: %v", err)
	}

	// All service clients accessible from one SDK instance
	s.Account.Deposit(ctx, depositCmd, principalID)
	s.Order.CreateOrder(ctx, orderCmd, principalID)     // Auto-added when you define Order service
	s.User.RegisterUser(ctx, userCmd, principalID)      // Auto-added when you define User service
}
```

**Benefits:**
- ✅ **Single entry point** - `unifiedsdk.New()` for all services
- ✅ **Property-based access** - `s.Account.OpenAccount()`, `s.Order.CreateOrder()`
- ✅ **Auto-discovery** - New services automatically appear when you add proto definitions
- ✅ **Type-safe API** - Compiler catches errors, full IntelliSense support
- ✅ **Multi-file support** - Works across all your proto service definitions
- ✅ **Dev/Prod modes** - Same API for local development and distributed production

**Production Mode (NATS):**

```go
// Switch to production mode for distributed command processing
s, _ := unifiedsdk.New(
	unifiedsdk.WithMode(sdk.ProductionMode),       // Commands via NATS!
	unifiedsdk.WithNATSURL("nats://cluster:4222"), // NATS cluster
	unifiedsdk.WithSQLiteDSN("./events.db"),
)

// Same API - commands now distributed across services!
s.Account.OpenAccount(ctx, cmd, principalID)
s.Order.CreateOrder(ctx, cmd, principalID)
```

**How It Works:**

1. Each proto service generates an individual client (e.g., `AccountClient`, `OrderClient`)
2. After proto generation, a post-processing tool scans for all generated clients
3. The unified SDK is automatically generated with all clients as properties
4. When you add new services, they're automatically included in the next generation

See `examples/unified_sdk/` for a complete working example.

**Alternative: Individual Service Clients**

If you prefer to use individual service clients directly:

```go
client, _ := sdk.NewClient(config)
accountClient := accountv1.NewAccountClient(client)
accountClient.OpenAccount(ctx, cmd, principalID)
```

### 5️⃣ Wire Up Infrastructure (Manual Approach)

Connect all components in your main application:

```go
package main

import (
	"context"
	"log"
	"log/slog"

	"github.com/plaenen/eventsourcing/pkg/eventsourcing"
	"github.com/plaenen/eventsourcing/pkg/middleware"
	natspkg "github.com/plaenen/eventsourcing/pkg/nats"
	"github.com/plaenen/eventsourcing/pkg/sqlite"
	accountv1 "github.com/yourorg/project/pb/account/v1"
)

func main() {
	ctx := context.Background()
	logger := slog.Default()

	// 1. Event Store (SQLite)
	eventStore, err := sqlite.NewEventStore(
		sqlite.WithDSN("eventstore.db"),
		sqlite.WithWALMode(true),
	)
	if err != nil {
		log.Fatalf("failed to create event store: %v", err)
	}
	defer eventStore.Close()

	// 2. Event Bus (NATS JetStream)
	eventBus, natsServer, err := natspkg.NewEmbeddedEventBus() // For development
	if err != nil {
		log.Fatalf("failed to create event bus: %v", err)
	}
	defer natsServer.Shutdown()
	defer eventBus.Close()

	// For production, use external NATS:
	// eventBus, err := natspkg.NewEventBus(natspkg.Config{
	//     URL: "nats://localhost:4222",
	// })

	// 3. Command Bus with Middleware Pipeline
	commandBus := eventsourcing.NewCommandBusWithEventBus(eventBus)

	// Add middleware (executes in order)
	commandBus.Use(middleware.RecoveryMiddleware(logger))           // Panic recovery
	commandBus.Use(middleware.LoggingMiddleware(logger))            // Request logging
	commandBus.Use(middleware.MetadataValidationMiddleware())       // Validate metadata
	commandBus.Use(middleware.OpenTelemetryMiddleware("myapp"))     // Distributed tracing
	// commandBus.Use(middleware.AuthorizationMiddleware(authorizer)) // RBAC

	// 4. Register Command Handlers
	repo := accountv1.NewAccountRepository(eventStore)

	commandBus.Register("account.v1.OpenAccountCommand",
		eventsourcing.CommandHandlerFunc(func(ctx context.Context, envelope *eventsourcing.CommandEnvelope) ([]*eventsourcing.Event, error) {
			cmd := envelope.Command.(*accountv1.OpenAccountCommand)

			// Load or create aggregate
			account := accountv1.NewAccount(cmd.AccountId)
			account.SetCommandID(envelope.Metadata.CommandID)

			// Execute business logic
			if err := account.OpenAccount(ctx, cmd, envelope.Metadata); err != nil {
				return nil, err
			}

			// Persist events with idempotency
			result, err := repo.SaveWithCommand(account, envelope.Metadata.CommandID)
			if err != nil {
				return nil, err
			}

			return result.Events, nil
		}),
	)

	// 5. Projection Management
	checkpointStore, _ := sqlite.NewCheckpointStore(eventStore.DB())
	projectionMgr := eventsourcing.NewProjectionManager(checkpointStore, eventStore, eventBus)

	// Register projections
	projectionMgr.Register(&AccountViewProjection{db: eventStore.DB()})

	// Start projection (subscribes to EventBus and processes events)
	if err := projectionMgr.Start(ctx, "account_view"); err != nil {
		log.Fatalf("failed to start projection: %v", err)
	}

	// 6. Start your API server (Connect RPC, gRPC, HTTP, etc.)
	log.Println("Application started successfully")
	select {} // Keep running
}
```

## 🏗️ Architecture

### Write Side (Command Flow)

```
┌────────┐     ┌─────────────┐     ┌────────────┐     ┌───────────┐     ┌────────────┐     ┌──────────┐
│ Client │────▶│ Command Bus │────▶│ Middleware │────▶│  Handler  │────▶│ Aggregate  │────▶│  Events  │
└────────┘     └─────────────┘     └────────────┘     └───────────┘     └────────────┘     └──────────┘
                                          │                                      │                  │
                                          │                                      │                  │
                                     ┌────▼────┐                           ┌────▼────┐        ┌────▼────┐
                                     │ Logging │                           │Business │        │ Event   │
                                     │ Tracing │                           │  Rules  │        │  Store  │
                                     │ AuthZ   │                           │Uniqueness        └─────────┘
                                     └─────────┘                           │Snapshots│             │
                                                                           └─────────┘             │
                                                                                              ┌────▼────┐
                                                                                              │  Event  │
                                                                                              │   Bus   │
                                                                                              └─────────┘
```

### Read Side (Query Flow)

```
┌──────────┐     ┌─────────────┐     ┌─────────────┐     ┌───────────┐
│  Event   │────▶│ Projections │────▶│ Read Models │◀────│  Queries  │
│   Bus    │     └─────────────┘     │    (SQL)    │     └───────────┘
└──────────┘            │             └─────────────┘           ▲
     ▲                  │                    │                  │
     │                  │                    │                  │
┌────┴────┐        ┌────▼────┐         ┌────▼────┐       ┌────┴────┐
│  Event  │        │Checkpoint        │ Indexes │       │ Client  │
│  Store  │────────▶│ Tracking │        │ Views   │       └─────────┘
└─────────┘        └─────────┘         └─────────┘
    │
    │ (For rebuilds)
    └──────────────────────────▶
```

### Key Components

| Component | Purpose | Implementation |
|-----------|---------|----------------|
| **Aggregate** | Domain model with business logic | Base class + generated code |
| **Event Store** | Persistent event stream | SQLite with WAL mode |
| **Event Bus** | Event distribution | NATS JetStream |
| **Command Bus** | Command routing & middleware | In-memory with plugin pipeline |
| **Projection** | Read model updater | EventBus subscriber + checkpoint tracking |
| **Repository** | Aggregate persistence | Type-safe generic wrapper |
| **Snapshot** | Performance optimization | Multi-version storage |

## 🧩 Core Concepts

### Commands vs Events

**Commands** (Intentions):
- Imperative verbs: `OpenAccount`, `Deposit`, `Withdraw`
- Can be rejected (validation)
- Contain user intent
- Generate events when successful

**Events** (Facts):
- Past tense: `AccountOpened`, `MoneyDeposited`, `MoneyWithdrawn`
- Immutable history
- Cannot be rejected
- Represent state changes

### Aggregates

Aggregates are consistency boundaries that:
- Enforce business invariants
- Generate events from commands
- Reconstruct state from events
- Maintain internal consistency

**Proto-based aggregate state** (single source of truth):

```protobuf
message Account {
  option (eventsourcing.aggregate_root) = {
    id_field: "account_id"
    type_name: "Account"
  };

  string account_id = 1;
  string owner_name = 2;
  string balance = 3;
  AccountStatus status = 4;
}
```

**Generated aggregate wrapper:**

```go
type AccountAggregate struct {
	eventsourcing.AggregateRoot
	*Account  // Embeds the proto-defined state
}
```

**Key generated methods:**
- `EmitAccountOpenedEvent()` - Event emitter helper
- `applyAccountOpenedEvent()` - Event applier (auto-generated state updates)
- `MarshalSnapshot()` / `UnmarshalSnapshot()` - Automatic proto serialization
- `ID()` / `Type()` - Aggregate metadata

**Benefits of proto-defined state:**
- ✅ Single source of truth (no duplication)
- ✅ Automatic serialization for snapshots
- ✅ Language-agnostic state definition
- ✅ Schema evolution via protobuf versioning
- ✅ No manual field mapping needed

### Unique Constraints

Enforce uniqueness at database level integrated with event sourcing:

```go
constraints := []eventsourcing.UniqueConstraint{
	{
		IndexName: "user_email",
		Value:     "alice@example.com",
		Operation: eventsourcing.ConstraintClaim,  // Claim ownership
	},
}

a.ApplyChangeWithConstraints(event, eventType, metadata, constraints)
```

**Operations:**
- `ConstraintClaim` - Reserve a unique value
- `ConstraintRelease` - Free a previously claimed value

**Features:**
- ✅ Atomic with event persistence
- ✅ Survives projection rebuilds
- ✅ Returns detailed violation errors
- ✅ Supports natural keys (email, account ID, SKU, etc.)

### Idempotency (Three Layers)

#### 1. Command-Level Deduplication

```go
commandID := uuid.New().String() // Client generates
result, err := repo.SaveWithCommand(aggregate, commandID)

if result.AlreadyProcessed {
	// Command was already executed, return cached events
	return result.Events, nil
}
```

- Prevents duplicate command execution
- Default TTL: 7 days
- Stored in `commands` table

#### 2. Deterministic Event IDs

```go
eventID := eventsourcing.GenerateDeterministicEventID(commandID, aggregateID, sequence)
// Example: cmd-123_acc-456_0, cmd-123_acc-456_1, ...
```

- Same command → same event IDs
- Enables safe retries
- Database constraint prevents duplicates

#### 3. Idempotent Projections

```go
func (p *AccountViewProjection) Handle(ctx context.Context, event *eventsourcing.Event) error {
	// Upsert (INSERT OR REPLACE) for natural idempotency
	_, err := p.db.Exec(`
		INSERT INTO account_views (account_id, owner_name, balance, version)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(account_id) DO UPDATE SET
			owner_name = excluded.owner_name,
			balance = excluded.balance,
			version = excluded.version
	`, ...)
	return err
}
```

### Snapshots

Snapshots provide performance optimization by storing aggregate state at specific versions. Instead of replaying all events from the beginning, load the latest snapshot and only replay events since that snapshot.

#### Quick Start

```go
// 1. Configure snapshot strategy
strategy := eventsourcing.NewIntervalSnapshotStrategy(100) // Every 100 events

// 2. Create snapshot store (uses same DB as event store)
snapshotStore := sqlite.NewSnapshotStore(eventStore.DB())

// 3. Create repository with snapshot support
repo := NewAccountRepositoryWithSnapshots(eventStore, snapshotStore, strategy)

// 4. Load uses snapshot + events since snapshot
account, err := repo.Load(aggregateID)
```

#### Implementation

**1. Make your aggregate snapshotable:**

```go
// MarshalSnapshot serializes the aggregate state
func (a *Account) MarshalSnapshot() ([]byte, error) {
    return proto.Marshal(&accountv1.AccountSnapshot{
        AccountId: a.AccountId,
        OwnerName: a.OwnerName,
        Balance:   a.Balance,
        Status:    a.Status,
    })
}

// UnmarshalSnapshot deserializes the aggregate state
func (a *Account) UnmarshalSnapshot(data []byte) error {
    var snapshot accountv1.AccountSnapshot
    if err := proto.Unmarshal(data, &snapshot); err != nil {
        return err
    }

    a.AccountId = snapshot.AccountId
    a.OwnerName = snapshot.OwnerName
    a.Balance = snapshot.Balance
    a.Status = snapshot.Status

    return nil
}
```

**2. Custom snapshot strategies:**

```go
// Interval-based (every N events)
strategy := eventsourcing.NewIntervalSnapshotStrategy(100)

// Custom logic
type CustomSnapshotStrategy struct{}

func (s *CustomSnapshotStrategy) ShouldCreateSnapshot(currentVersion int64, eventsSinceLastSnapshot int64) bool {
    // Snapshot every 50 events OR at every 1000th version
    return eventsSinceLastSnapshot >= 50 || currentVersion%1000 == 0
}
```

**3. Repository with automatic snapshotting:**

```go
type AccountRepositoryWithSnapshots struct {
    *accountv1.AccountRepository
    eventStore          eventsourcing.EventStore
    snapshotStore       eventsourcing.SnapshotStore
    snapshotStrategy    eventsourcing.SnapshotStrategy
    lastSnapshotVersion map[string]int64
}

// Load with snapshot optimization
func (r *AccountRepositoryWithSnapshots) Load(id string) (*Account, error) {
    snapshot, err := r.snapshotStore.GetLatestSnapshot(id)

    account := NewAccount(id)
    fromVersion := int64(0)

    if err == nil {
        // Restore from snapshot
        if err := account.UnmarshalSnapshot(snapshot.Data); err != nil {
            log.Printf("Snapshot unmarshal failed, falling back to full replay: %v", err)
        } else {
            account.SetVersion(snapshot.Version)
            fromVersion = snapshot.Version
        }
    }

    // Load events since snapshot (or from beginning if no snapshot)
    events, err := r.eventStore.LoadEvents(id, fromVersion)
    if err != nil {
        return nil, err
    }

    // Apply remaining events
    for _, event := range events {
        if err := account.ApplyEvent(event); err != nil {
            return nil, err
        }
    }

    return account, nil
}

// Save with automatic snapshot creation
func (r *AccountRepositoryWithSnapshots) SaveWithCommand(aggregate *Account, commandID string) (*CommandResult, error) {
    result, err := r.AccountRepository.SaveWithCommand(aggregate, commandID)
    if err != nil {
        return nil, err
    }

    // Don't snapshot if this was idempotent replay
    if result.AlreadyProcessed {
        return result, nil
    }

    // Check if we should create a snapshot
    lastVersion := r.lastSnapshotVersion[aggregate.ID()]
    eventsSinceSnapshot := aggregate.Version() - lastVersion

    if r.snapshotStrategy.ShouldCreateSnapshot(aggregate.Version(), eventsSinceSnapshot) {
        if err := r.createSnapshot(aggregate); err != nil {
            log.Printf("Failed to create snapshot: %v", err) // Log but don't fail
        } else {
            r.lastSnapshotVersion[aggregate.ID()] = aggregate.Version()

            // Cleanup old snapshots (keep last 3 versions)
            if strategy, ok := r.snapshotStrategy.(*eventsourcing.IntervalSnapshotStrategy); ok {
                retentionVersion := aggregate.Version() - (3 * strategy.Interval)
                if retentionVersion > 0 {
                    r.snapshotStore.DeleteOldSnapshots(aggregate.ID(), retentionVersion)
                }
            }
        }
    }

    return result, nil
}
```

#### Storage Architecture

Snapshots are stored in SQLite with **multi-version support**:

```sql
CREATE TABLE snapshots (
    aggregate_id TEXT NOT NULL,
    aggregate_type TEXT NOT NULL,
    version INTEGER NOT NULL,
    data BLOB NOT NULL,
    created_at INTEGER NOT NULL,
    metadata TEXT,
    PRIMARY KEY (aggregate_id, version)
);
```

**Why keep multiple versions?**
1. **Safety**: If snapshot creation fails, previous snapshot remains available
2. **Debugging**: Compare aggregate state across versions
3. **Rollback**: Recover from corrupted snapshots
4. **Time-Travel**: Load aggregate state at historical points

#### Performance Guidelines

**When to use snapshots:**
- ✅ Aggregates with >50-100 events
- ✅ Load time is noticeable (>100ms)
- ✅ Aggregates are frequently loaded
- ✅ Events are large in size

**When NOT to use snapshots:**
- ❌ Aggregates typically have <20 events
- ❌ Load time is acceptable
- ❌ Storage space is constrained
- ❌ Aggregate state is very large (>10MB)

**Snapshot frequency recommendations:**

| Interval | Use Case | Trade-offs |
|----------|----------|------------|
| Every 50 events | Aggressive optimization | Faster loads, more storage |
| Every 100 events | Balanced (recommended) | Good performance/storage ratio |
| Every 500 events | Conservative | Less storage, slower loads |

**Performance impact example:**

| Scenario | Load Time | Storage Overhead |
|----------|-----------|-----------------|
| 1000 events, no snapshot | ~500ms | 0 MB |
| 1000 events, snapshot every 100 | ~50ms | ~10KB per snapshot |
| **Improvement** | **10x faster** | Minimal |

#### Maintenance Operations

**Cleanup old snapshots:**

```go
// Keep only last 3 snapshots per aggregate
// Delete snapshots older than version (currentVersion - 300)
err := snapshotStore.DeleteOldSnapshots(aggregateID, currentVersion-300)
```

**Monitor snapshot statistics:**

```go
stats, err := snapshotStore.GetSnapshotStats()
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Total Snapshots: %d\n", stats.TotalSnapshots)
fmt.Printf("Unique Aggregates: %d\n", stats.UniqueAggregates)
fmt.Printf("Total Size: %d MB\n", stats.TotalSizeBytes/(1024*1024))
fmt.Printf("Average Size: %d KB\n", stats.AvgSizeBytes/1024)
```

#### Migration to Snapshots

Add snapshots to existing system:

```bash
# 1. Schema is auto-created on first use
# 2. Optionally create initial snapshots for existing aggregates (background job)
for aggregateID in $(getAllAggregateIDs); do
    aggregate := repo.Load(aggregateID)
    repo.createSnapshot(aggregate)
done
```

#### Best Practices

1. **Start Conservative**: Begin with snapshot every 100 events
2. **Monitor Performance**: Track snapshot creation time and storage growth
3. **Handle Failures Gracefully**: Log snapshot errors, never fail command execution
4. **Clean Up Regularly**: Schedule cleanup job to remove old snapshots
5. **Version Your Schema**: Include schema version in metadata for migrations
6. **Test Serialization**: Ensure MarshalSnapshot/UnmarshalSnapshot handle all fields
7. **Keep Multiple Versions**: Maintain 2-3 recent snapshots for safety and debugging

## 🛠️ Code Generation

The `protoc-gen-eventsourcing` plugin generates type-safe boilerplate from proto definitions.

### Naming Conventions

| Proto | Generated Go |
|-------|--------------|
| `OpenAccountCommand` | `OpenAccount(ctx, cmd, metadata)` method |
| `AccountOpenedEvent` | `applyAccountOpenedEvent(event)` method |
| `Account` aggregate | `Account` struct + `NewAccount(id)` constructor |

### What Gets Generated

**Aggregate Struct:**
```go
type Account struct {
	eventsourcing.AggregateRoot
	AccountId string
	OwnerName string
	Balance   string
	Status    AccountStatus
}

func NewAccount(id string) *Account {
	return &Account{
		AggregateRoot: eventsourcing.NewAggregateRoot(id, "Account"),
	}
}
```

**Command Handler Stub:**
```go
func (a *Account) OpenAccount(ctx context.Context, cmd *OpenAccountCommand, metadata eventsourcing.EventMetadata) error {
	// TODO: Add business logic validation
	event := &AccountOpenedEvent{
		AccountId:      cmd.AccountId,
		OwnerName:      cmd.OwnerName,
		InitialBalance: cmd.InitialBalance,
		Timestamp:      time.Now().Unix(),
	}
	return a.EmitAccountOpenedEvent(event, metadata)
}
```

**Event Applier:**
```go
func (a *Account) applyAccountOpenedEvent(e *AccountOpenedEvent) error {
	a.AccountId = e.AccountId
	a.OwnerName = e.OwnerName
	a.Balance = e.InitialBalance
	return nil
}
```

**Type-Safe Repository:**
```go
type AccountRepository struct {
	*eventsourcing.BaseRepository[*Account]
}

func NewAccountRepository(eventStore eventsourcing.EventStore) *AccountRepository {
	return &AccountRepository{
		BaseRepository: eventsourcing.NewRepository[*Account](
			eventStore,
			"Account",
			func(id string) *Account { return NewAccount(id) },
			func(agg *Account, event *eventsourcing.Event) error {
				// Deserialize and apply
			},
		),
	}
}
```

### Safe Regeneration Pattern

Generated code lives in `*_aggregate.pb.go` files. Your business logic lives in separate files:

```
account.proto           → account_aggregate.pb.go (generated, don't edit)
                       → account.pb.go (protobuf, generated)

account.go             → Your business logic (safe to edit)
account_test.go        → Your tests (safe to edit)
```

Regenerating code won't overwrite your business logic!

## 🔌 Middleware

Build processing pipelines by chaining middleware:

### Available Middleware

#### Logging (slog)
```go
commandBus.Use(middleware.LoggingMiddleware(slog.Default()))
```
Logs command execution, timing, and errors using structured logging.

#### Validation
```go
// Validate protobuf message structure
commandBus.Use(middleware.ValidationMiddleware(&middleware.ProtobufValidator{}))

// Validate required metadata fields
commandBus.Use(middleware.MetadataValidationMiddleware())
```

#### OpenTelemetry Tracing
```go
commandBus.Use(middleware.OpenTelemetryMiddleware("account-service"))
```
Creates spans for distributed tracing with command type, aggregate ID, and result status.

#### Authorization (RBAC)
```go
authorizer := middleware.NewRoleBasedAuthorizer(
	map[string][]string{
		"DeleteAccount": {"admin"},
		"TransferMoney": {"admin", "accountant"},
	},
	func(ctx context.Context, principalID string) ([]string, error) {
		// Fetch user roles from your auth system
		return getRolesFromDatabase(principalID)
	},
)
commandBus.Use(middleware.AuthorizationMiddleware(authorizer))
```

#### Panic Recovery
```go
commandBus.Use(middleware.RecoveryMiddleware(logger))
```
Catches panics and converts to errors, preventing process crashes.

### Custom Middleware

```go
func AuditMiddleware(logger *slog.Logger) eventsourcing.Middleware {
	return func(next eventsourcing.CommandHandler) eventsourcing.CommandHandler {
		return eventsourcing.CommandHandlerFunc(func(ctx context.Context, cmd *eventsourcing.CommandEnvelope) ([]*eventsourcing.Event, error) {
			// Before
			logger.Info("command received",
				"type", cmd.CommandType,
				"principal", cmd.Metadata.PrincipalID)

			// Execute
			events, err := next.Handle(ctx, cmd)

			// After
			if err != nil {
				logger.Error("command failed", "error", err)
			} else {
				logger.Info("command succeeded", "events", len(events))
			}

			return events, err
		})
	}
}

commandBus.Use(AuditMiddleware(logger))
```

## 🧪 Testing

### Unit Tests (Aggregate Logic)

Test business rules without infrastructure:

```go
func TestAccount_OpenAccount_Success(t *testing.T) {
	account := accountv1.NewAccount("acc-123")

	cmd := &accountv1.OpenAccountCommand{
		AccountId:      "acc-123",
		OwnerName:      "Alice",
		InitialBalance: "1000.00",
	}

	err := account.OpenAccount(context.Background(), cmd, eventsourcing.EventMetadata{})

	assert.NoError(t, err)
	assert.Equal(t, 1, len(account.UncommittedEvents()))
	assert.Equal(t, int64(1), account.Version())
}

func TestAccount_Withdraw_InsufficientFunds(t *testing.T) {
	account := accountv1.NewAccount("acc-123")
	account.Balance = "100.00"
	account.Status = accountv1.AccountStatus_ACCOUNT_STATUS_OPEN

	cmd := &accountv1.WithdrawCommand{
		AccountId: "acc-123",
		Amount:    "200.00",
	}

	err := account.Withdraw(context.Background(), cmd, eventsourcing.EventMetadata{})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "insufficient balance")
}
```

### Integration Tests (Full Stack)

Test with real infrastructure using embedded NATS:

```go
func TestFullStack_AccountLifecycle(t *testing.T) {
	ctx := context.Background()

	// Setup (zero external dependencies!)
	eventStore, _ := sqlite.NewEventStore(sqlite.WithDSN(":memory:"))
	defer eventStore.Close()

	eventBus, natsServer, _ := natspkg.NewEmbeddedEventBus()
	defer natsServer.Shutdown()
	defer eventBus.Close()

	repo := accountv1.NewAccountRepository(eventStore)

	// 1. Open account
	account := accountv1.NewAccount("acc-test")
	account.SetCommandID(eventsourcing.GenerateID())

	cmd := &accountv1.OpenAccountCommand{
		AccountId:      "acc-test",
		OwnerName:      "Bob",
		InitialBalance: "500.00",
	}

	err := account.OpenAccount(ctx, cmd, eventsourcing.EventMetadata{
		CommandID:     eventsourcing.GenerateID(),
		CorrelationID: eventsourcing.GenerateID(),
		PrincipalID:   "test-user",
	})
	assert.NoError(t, err)

	result, err := repo.SaveWithCommand(account, account.CommandID())
	assert.NoError(t, err)
	assert.False(t, result.AlreadyProcessed)

	// 2. Load and verify
	loaded, err := repo.Load("acc-test")
	assert.NoError(t, err)
	assert.Equal(t, "Bob", loaded.OwnerName)
	assert.Equal(t, "500.00", loaded.Balance)

	// 3. Publish events to event bus
	err = eventBus.Publish(result.Events)
	assert.NoError(t, err)
}
```

### Projection Tests

```go
func TestProjection_AccountView(t *testing.T) {
	db := setupTestDB(t)
	projection := &AccountViewProjection{db: db}

	event := &eventsourcing.Event{
		ID:            "evt-1",
		AggregateID:   "acc-1",
		AggregateType: "Account",
		EventType:     "account.v1.AccountOpenedEvent",
		Version:       1,
		Timestamp:     time.Now().Unix(),
		Data:          marshalEvent(&accountv1.AccountOpenedEvent{
			AccountId:      "acc-1",
			OwnerName:      "Charlie",
			InitialBalance: "1000.00",
		}),
	}

	err := projection.Handle(context.Background(), event)
	assert.NoError(t, err)

	// Verify read model
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM account_views WHERE account_id = ?", "acc-1").Scan(&count)
	assert.NoError(t, err)
	assert.Equal(t, 1, count)
}
```

## 📊 Performance

Benchmarked on Apple M1 Pro, 16GB RAM:

| Operation | Throughput/Latency | Details |
|-----------|-------------------|---------|
| **Event Write** | ~10,000 events/sec | Single aggregate, SQLite WAL mode |
| **Event Read** | ~100,000 events/sec | Projection rebuilds from history |
| **Query Latency** | <1ms | Indexed projection queries |
| **NATS Latency** | <10ms | Event propagation to subscribers |
| **Snapshot Load** | ~50ms | 1000 events with snapshot at 100 |
| **Full Replay** | ~500ms | 1000 events without snapshot |

**Optimization Tips:**
1. Use snapshots for aggregates with >100 events
2. Add database indexes to projection tables
3. Batch event publishes when possible
4. Use WAL mode for SQLite (enabled by default)
5. Consider sharding for >1M events/day

## 📚 Examples

### Bank Account (Complete CQRS)

Location: `examples/bankaccount/`

**Features demonstrated:**
- ✅ Command handlers with validation
- ✅ Event sourcing with full audit trail
- ✅ Unique constraints (account ID)
- ✅ Projections for read models
- ✅ Balance calculations with big.Float precision
- ✅ Snapshot support for performance
- ✅ Integration tests with embedded NATS

**Run the example:**

```bash
# Generate code
task generate

# Run tests
task test:examples

# Run with coverage
task test:coverage
```

**View detailed walkthrough:**
- [examples/EXAMPLE.md](examples/EXAMPLE.md) - Step-by-step tutorial
- [examples/SETUP.md](examples/SETUP.md) - Development environment setup

## 💡 Best Practices

### Domain Modeling

1. **Commands are intentions, events are facts**
   - ✅ `OpenAccountCommand` → `AccountOpenedEvent`
   - ❌ `OpenAccountEvent` (events should be past tense)

2. **Keep aggregates small and focused**
   - ✅ One aggregate = one consistency boundary
   - ❌ Don't create mega-aggregates with multiple concerns

3. **Use value objects for business concepts**
   - ✅ `Money`, `Email`, `AccountNumber` types
   - ❌ Primitive obsession (strings everywhere)

4. **Validate in commands, not events**
   - Commands can fail → validate business rules
   - Events represent facts → always succeed

### Event Sourcing

5. **Events are immutable**
   - Never modify historical events
   - Use new event types for corrections (e.g., `AccountCorrectedEvent`)

6. **Use unique constraints for natural keys**
   - Email addresses, account numbers, SKUs
   - Enforced at database level for consistency

7. **Design events for rehydration**
   - Events should contain all data needed to rebuild state
   - Don't rely on external lookups during event replay

### CQRS

8. **Separate read and write models**
   - Queries use projections (SQL views), not event store
   - Optimize read models for query patterns

9. **Embrace eventual consistency**
   - Projections update asynchronously via EventBus
   - Design UI for "processing..." states

10. **Idempotent everything**
    - Client provides command IDs
    - Projections use upsert semantics
    - Safe to retry failed operations

### Performance

11. **Use snapshots strategically**
    - Aggregates with >100 events benefit most
    - Configure interval based on load patterns

12. **Index projection tables properly**
    - Add indexes for common query patterns
    - Use `EXPLAIN QUERY PLAN` to verify

13. **Batch operations when possible**
    - Publish events in batches
    - Process multiple commands in pipeline

## 📖 Documentation

- [CONTRIBUTING.md](CONTRIBUTING.md) - Contribution guidelines
- [AGENTS.md](AGENTS.md) - AI coding agent instructions
- [examples/EXAMPLE.md](examples/EXAMPLE.md) - Step-by-step tutorial
- [examples/SETUP.md](examples/SETUP.md) - Development setup

## 🤝 Contributing

We welcome contributions! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for:
- Development setup
- Code style guidelines
- Testing requirements
- Pull request process

Quick start for contributors:

```bash
git clone https://github.com/plaenen/eventsourcing.git
cd eventsourcing
task dev:setup      # Install dependencies and generate code
task dev:check      # Run checks before committing
```

## 📄 License

MIT License - see [LICENSE](LICENSE) file for details.

## 🙏 Credits

Built with inspiration from:
- [EventStoreDB](https://www.eventstore.com/) - Event store patterns
- [Axon Framework](https://axoniq.io/) - Command/event routing
- [Greg Young's CQRS/ES teachings](https://cqrs.files.wordpress.com/2010/11/cqrs_documents.pdf)
- [Domain-Driven Design](https://www.domainlanguage.com/ddd/) - Eric Evans, Vaughn Vernon

## 🌟 Support

- ⭐ Star this repo if you find it useful
- 🐛 [Report bugs](https://github.com/plaenen/eventsourcing/issues)
- 💡 [Request features](https://github.com/plaenen/eventsourcing/issues)
- 📖 [Read the docs](https://github.com/plaenen/eventsourcing)

---

Built with ❤️ for the Go community
