# Agent Instructions for Event Sourcing Framework

This file contains instructions for AI coding agents working on this Event Sourcing and CQRS framework for Go. It complements the human-focused README and CONTRIBUTING documents with agent-specific guidance.

## Quick Reference

- **Language**: Go 1.25.0+
- **Build Tool**: Task (Taskfile.yml)
- **Key Command**: `task generate` (regenerates all code from proto files)
- **Test Command**: `task test` (runs all tests)
- **Check Command**: `task dev:check` (format, lint, test)

## Project Overview

This is an event sourcing and CQRS framework for Go that generates code from Protocol Buffer definitions. The framework provides event stores, snapshot stores, middleware pipelines, and projection support.

**Core Pattern**: Define aggregates, commands, and events in `.proto` files → Run `task generate` → Implement business logic in generated helper files → Wire up infrastructure.

## Setup Instructions

### Initial Setup

```bash
# 1. Install dependencies
go install github.com/go-task/task/v3/cmd/task@latest
go install github.com/bufbuild/buf/cmd/buf@latest
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest

# 2. Install the custom code generator
go install ./cmd/protoc-gen-eventsourcing

# 3. Generate all code
task generate

# 4. Run tests to verify
task test
```

### Regeneration After Changes

**CRITICAL**: Always run `task generate` after modifying any `.proto` file. The framework generates:
- Aggregate types and event handlers
- Repository implementations
- Event serialization/deserialization
- Type-safe SQL queries (via SQLC)

```bash
# Full regeneration (proto + SQL)
task generate

# Proto only
task generate:proto

# SQL only (for schema changes)
task generate:sqlc
```

## Build and Test

### Build Commands

```bash
# List all available tasks
task --list

# Generate all code (most common during development)
task generate

# Build the code generator plugin
task build:protoc-gen

# Run formatter
task fmt

# Run linter
task lint

# Full check (format + lint + test)
task dev:check
```

### Test Commands

```bash
# All tests
task test

# Unit tests only
task test:unit

# Integration tests only
task test:integration

# Specific package
go test -v ./pkg/eventsourcing/...

# Specific test
go test -v ./pkg/eventsourcing -run TestEventStore

# With coverage
task test:coverage

# With race detector
go test -race ./...
```

### Example Commands

```bash
# Run bank account example
go run examples/bankaccount/main.go

# Test bank account example
go test -v ./examples/bankaccount/...

# Test specific scenario
go test -v ./examples/bankaccount -run TestFullStack_AccountLifecycle
```

## Project Structure

### DO NOT EDIT

These directories contain generated code that will be overwritten:

- `gen/` - All generated Go code from proto files
- `examples/pb/**/*_aggregate.pb.go` - Generated aggregate wrappers (e.g., `AccountAggregate`)
- `examples/pb/**/*_sdk.pb.go` - **Generated SDK clients** (e.g., `AccountClient`)
- `pkg/sqlite/sqlc.go` - Generated SQL queries
- Any file with "Code generated" comment at the top

**Key generated types:**
- `AccountAggregate` - Wrapper struct embedding proto `Account` message
- `AccountClient` - **Type-safe SDK client with command/query methods**
- `NewAccount()` - Constructor returning `*AccountAggregate`
- `NewAccountClient()` - **Constructor for SDK client**
- `EmitXxxEvent()` - Event emitter helpers
- `applyXxxEvent()` - Auto-generated event appliers
- `AccountRepository` - Type-safe repository
- `MarshalSnapshot()` / `UnmarshalSnapshot()` - Automatic proto serialization

### Safe to Edit

- `pkg/eventsourcing/` - Core framework interfaces and implementations
- `pkg/middleware/` - Middleware implementations
- `pkg/sqlite/` - Event store and snapshot store (except `sqlc.go`)
- `pkg/nats/` - NATS event bus and command bus implementation
- `pkg/sdk/` - **Unified SDK client for commands/events/queries**
- `cmd/protoc-gen-eventsourcing/` - Code generator plugin
- `examples/bankaccount/domain/` - Example business logic
- `examples/pb/account/v1/account.go` - Business logic for generated aggregate
- `proto/` - Protocol Buffer definitions
- `*.sql` files in `pkg/sqlite/` - Database schemas

### Key Files

**Build and Configuration:**
- `Taskfile.yml` - Build automation (30+ tasks)
- `buf.work.yaml` - Buf workspace configuration
- `buf.gen.yaml` - Buf code generation config
- `sqlc.yaml` - SQLC query generation config
- `go.mod` - Go module dependencies

**Proto Definitions:**
- `proto/eventsourcing/options.proto` - Framework options for code generation
- `proto/account/v1/account.proto` - Example aggregate definition

**Core Framework:**
- `pkg/eventsourcing/aggregate.go` - Base aggregate implementation
- `pkg/eventsourcing/event_store.go` - Event store interface
- `pkg/eventsourcing/snapshot.go` - Snapshot support
- `pkg/eventsourcing/command_bus.go` - Command bus with middleware

**Examples:**
- `examples/bankaccount/` - Complete working example
- `examples/bankaccount/domain/` - Business logic implementation
- `examples/bankaccount/fullstack_test.go` - End-to-end tests

## Coding Conventions

### Proto Files

**File Structure:**
```protobuf
syntax = "proto3";
package domain.v1;

import "eventsourcing/options.proto";

option go_package = "github.com/plaenen/eventsourcing/gen/pb/domain/v1;domainv1";

// 1. Enums first
enum Status { ... }

// 2. Aggregate state (🆕 single source of truth)
message Entity {
  option (eventsourcing.aggregate_root) = {
    id_field: "entity_id"
    type_name: "Entity"
  };
  // State fields...
}

// 3. Commands (imperative verbs)
message CreateEntityCommand { ... }

// 4. Events (past tense)
message EntityCreatedEvent { ... }

// 5. Queries (if needed)
message GetEntityQuery { ... }
```

**Naming:**
- Commands: Imperative verbs ending with `Command` (e.g., `OpenAccountCommand`)
- Events: Past tense ending with `Event` (e.g., `AccountOpenedEvent`)
- Aggregates: Singular nouns (e.g., `Account`, `Order`)
- Fields: snake_case (e.g., `account_id`, `created_at`)
- Enums: SCREAMING_SNAKE_CASE with type prefix (e.g., `ACCOUNT_STATUS_OPEN`)

**Required Options:**

Every command MUST specify:
```protobuf
message MyCommand {
  option (eventsourcing.aggregate_options) = {
    aggregate: "EntityName"
    produces_events: "EventName"  // comma-separated if multiple
  };
  // fields...
}
```

Every event MUST specify:
```protobuf
message MyEvent {
  option (eventsourcing.event_options) = {
    aggregate: "EntityName"
    applies_to_state: ["field1", "field2"]  // fields to update on aggregate
  };
  // fields...
}
```

Every aggregate MUST specify:
```protobuf
message MyAggregate {
  option (eventsourcing.aggregate_root) = {
    id_field: "entity_id"
    type_name: "EntityName"
  };
  // fields...
}
```

### Go Code

**Imports Organization:**
```go
import (
    // 1. Standard library
    "context"
    "fmt"
    "time"

    // 2. External dependencies
    "google.golang.org/protobuf/proto"

    // 3. Internal framework
    "github.com/plaenen/eventsourcing/pkg/eventsourcing"

    // 4. Generated code
    accountv1 "github.com/plaenen/eventsourcing/examples/pb/account/v1"
)
```

**Error Handling:**
```go
// Always wrap errors with context
if err != nil {
    return fmt.Errorf("failed to save account: %w", err)
}

// Use early returns (guard clauses)
if a.Status != AccountStatus_ACCOUNT_STATUS_OPEN {
    return fmt.Errorf("account is not open")
}

// Domain errors should be explicit
if currentBalance.Cmp(amount) < 0 {
    return fmt.Errorf("insufficient balance: have %s, need %s", a.Balance, cmd.Amount)
}
```

**Business Logic Pattern:**
```go
func (a *Account) HandleCommand(ctx context.Context, cmd *Command, metadata eventsourcing.EventMetadata) error {
    // 1. Validate aggregate state
    if a.Status != DesiredStatus {
        return fmt.Errorf("invalid state")
    }

    // 2. Parse and validate command data
    value := new(big.Float)
    if _, ok := value.SetString(cmd.Value); !ok {
        return fmt.Errorf("invalid value: %s", cmd.Value)
    }

    // 3. Apply business rules
    if !businessRuleCheck() {
        return fmt.Errorf("business rule violation")
    }

    // 4. Create event with all necessary data
    event := &SomethingHappenedEvent{
        Field1: cmd.Field1,
        Field2: calculatedValue,
        Timestamp: time.Now().Unix(),
    }

    // 5. Emit event (don't modify state directly)
    return a.EmitSomethingHappenedEvent(event, metadata)
}
```

**Testing Pattern:**
```go
func TestAggregate_Command(t *testing.T) {
    tests := []struct {
        name        string
        setup       func(*Aggregate)
        cmd         *Command
        expectError bool
        checkResult func(*testing.T, *Aggregate)
    }{
        {
            name: "success case",
            setup: func(a *Aggregate) {
                a.Status = StatusReady
            },
            cmd: &Command{Field: "value"},
            expectError: false,
            checkResult: func(t *testing.T, a *Aggregate) {
                // assertions
            },
        },
        {
            name: "error case",
            setup: func(a *Aggregate) {
                a.Status = StatusClosed
            },
            cmd: &Command{Field: "value"},
            expectError: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            aggregate := NewAggregate("test-id")
            if tt.setup != nil {
                tt.setup(aggregate)
            }

            err := aggregate.HandleCommand(context.Background(), tt.cmd, eventsourcing.EventMetadata{})

            if tt.expectError && err == nil {
                t.Errorf("expected error, got nil")
            }
            if !tt.expectError && err != nil {
                t.Errorf("unexpected error: %v", err)
            }
            if tt.checkResult != nil {
                tt.checkResult(t, aggregate)
            }
        })
    }
}
```

## SDK and Distributed Architecture

### Generated SDK Client

The code generator automatically creates type-safe SDK clients for each aggregate:

**Generated Files:**
- `*_sdk.pb.go` - Contains `{Aggregate}Client` with type-safe command/query methods

**Example Generated Client:**
```go
// Generated in examples/pb/account/v1/account_sdk.pb.go
type AccountClient struct {
    sdk *sdk.Client
}

func NewAccountClient(sdkClient *sdk.Client) *AccountClient

func (c *AccountClient) OpenAccount(ctx context.Context, req *OpenAccountCommand, principalID string) (*OpenAccountResponse, error)
func (c *AccountClient) Deposit(ctx context.Context, req *DepositCommand, principalID string) (*DepositResponse, error)
func (c *AccountClient) GetAccount(ctx context.Context, req *GetAccountRequest) (*AccountView, error)
```

**Usage:**
```go
// 1. Create SDK client
client, _ := sdk.NewBuilder().
    WithMode(sdk.DevelopmentMode).
    WithSQLiteDSN(":memory:").
    Build()
defer client.Close()

// 2. Use generated client
accountClient := accountv1.NewAccountClient(client)

// 3. Send commands with type safety
accountClient.OpenAccount(ctx, &accountv1.OpenAccountCommand{
    AccountId:      "acc-123",
    OwnerName:      "Alice",
    InitialBalance: "1000.00",
}, "user-alice")
```

### SDK Modes

**Development Mode (In-Memory Command Bus):**
- Commands processed in-memory within the same process
- Events published to NATS for distribution
- Perfect for local development and testing
- Fast feedback loops

```go
client, _ := sdk.NewBuilder().
    WithMode(sdk.DevelopmentMode).
    Build()
```

**Production Mode (NATS Command Bus):**
- Commands published to NATS with request-reply pattern
- Enables distributed command processing across services
- Commands routed via NATS subjects: `commands.{CommandType}`
- Queue groups for load balancing handlers

```go
client, _ := sdk.NewBuilder().
    WithMode(sdk.ProductionMode).
    WithNATSURL("nats://cluster:4222").
    Build()
```

### Command Bus Architecture

**Development Mode Flow:**
```
Client → In-Memory CommandBus → Handler → EventStore → NATS EventBus
```

**Production Mode Flow:**
```
Client → NATS CommandBus → NATS (request-reply) → Handler Service → EventStore → NATS EventBus
```

**Key Files:**
- `pkg/eventsourcing/commandbus.go` - In-memory command bus
- `pkg/nats/commandbus.go` - NATS-based distributed command bus
- `pkg/nats/eventbus.go` - NATS JetStream event bus
- `pkg/sdk/client.go` - Unified SDK client

### Registering Command Handlers

**With SDK (Development Mode):**
```go
client.RegisterCommandHandler("account.v1.OpenAccountCommand",
    eventsourcing.CommandHandlerFunc(func(ctx context.Context, cmd *eventsourcing.CommandEnvelope) ([]*eventsourcing.Event, error) {
        // Handler logic
        return events, nil
    }),
)
```

**With SDK (Production Mode):**
Same code! The SDK automatically subscribes to NATS when in production mode.

**Command Subject Pattern:**
- Subject: `commands.{CommandType}`
- Example: `commands.account.v1.OpenAccountCommand`
- Queue group: `command-handlers` (load balancing)

## Common Workflows

### Adding a New Command

1. **Define in proto:**
   ```protobuf
   message NewCommand {
     option (eventsourcing.aggregate_options) = {
       aggregate: "Account"
       produces_events: "NewEvent"
     };
     string account_id = 1;
     string value = 2;
   }

   message NewEvent {
     option (eventsourcing.event_options) = {
       aggregate: "Account"
       applies_to_state: ["new_field"]
     };
     string account_id = 1;
     string value = 2;
     string new_field = 3;
   }
   ```

2. **Regenerate code:**
   ```bash
   task generate:proto
   ```

3. **Implement business logic** in `examples/pb/account/v1/account.go`:
   ```go
   func (a *Account) HandleNew(ctx context.Context, cmd *NewCommand, metadata eventsourcing.EventMetadata) error {
       // validation and business logic
       event := &NewEvent{
           AccountId: cmd.AccountId,
           Value: cmd.Value,
           NewField: calculatedValue,
       }
       return a.EmitNewEvent(event, metadata)
   }
   ```

4. **Add test:**
   ```go
   func TestAccount_HandleNew(t *testing.T) {
       // test implementation
   }
   ```

### Adding Unique Constraints

1. **Add to command proto:**
   ```protobuf
   message CreateEntityCommand {
     option (eventsourcing.aggregate_options) = {
       aggregate: "Entity"
       produces_events: "EntityCreatedEvent"
       unique_constraints: {
         index_name: "email"
         field: "email"
         operation: CONSTRAINT_OPERATION_CLAIM
       }
     };
     string email = 1;
   }
   ```

2. **Release in corresponding event:**
   ```protobuf
   message EntityDeletedEvent {
     option (eventsourcing.event_options) = {
       aggregate: "Entity"
       applies_to_state: ["status"]
       unique_constraints: {
         index_name: "email"
         field: "email"
         operation: CONSTRAINT_OPERATION_RELEASE
       }
     };
     string email = 1;
   }
   ```

3. **Regenerate and test:**
   ```bash
   task generate
   go test -v ./examples/... -run UniqueConstraints
   ```

### Adding Middleware

1. **Create middleware file** in `pkg/middleware/`:
   ```go
   package middleware

   import (
       "context"
       "github.com/plaenen/eventsourcing/pkg/eventsourcing"
   )

   type MyMiddleware struct {
       config Config
   }

   func NewMyMiddleware(config Config) *MyMiddleware {
       return &MyMiddleware{config: config}
   }

   func (m *MyMiddleware) Handle(ctx context.Context, cmd interface{}, next eventsourcing.CommandHandlerFunc) (*eventsourcing.CommandResult, error) {
       // Before command execution

       result, err := next(ctx, cmd)

       // After command execution

       return result, err
   }
   ```

2. **Add tests** in `pkg/middleware/my_middleware_test.go`

3. **Document usage** in README.md

4. **Add example** in `examples/bankaccount/`

### Modifying Database Schema

1. **Update SQL file** in `pkg/sqlite/`:
   - `schema.sql` - Event store tables
   - `snapshots.sql` - Snapshot tables

2. **Regenerate SQLC:**
   ```bash
   task generate:sqlc
   ```

3. **Update migration code** if needed in `pkg/sqlite/migrate.go`

4. **Test migrations:**
   ```bash
   go test -v ./pkg/sqlite/migrate -run TestMigrations
   ```

## Troubleshooting

### "cannot find package" errors

**Cause**: Generated code not present or out of date.

**Fix**:
```bash
task generate
go mod tidy
```

### "event type not found" errors

**Cause**: Event deserialization switch statement needs updating.

**Fix**: Check `account_repository_snapshot.go` `deserializeEvent()` function and add new event types:
```go
case "accountv1.NewEvent":
    msg := &accountv1.NewEvent{}
    if err := proto.Unmarshal(event.Data, msg); err != nil {
        return nil, err
    }
    return msg, nil
```

### Tests failing after proto changes

**Cause**: Generated code doesn't match proto definitions.

**Fix**:
```bash
# Clean and rebuild
rm -rf gen/
task generate
go test ./...
```

### "aggregate_options not defined"

**Cause**: Missing import of eventsourcing options.

**Fix**: Add to proto file:
```protobuf
import "eventsourcing/options.proto";
```

### Buf errors

**Cause**: Proto file not following buf conventions.

**Fix**:
```bash
# Format proto files
buf format -w

# Check for breaking changes
buf breaking --against '.git#branch=main'
```

## Performance Considerations

### Event Store

- SQLite event store handles ~10,000 writes/sec
- Use WAL mode for concurrent reads (enabled by default)
- Batch event writes when possible (single transaction)

### Snapshots

- Enable snapshots for aggregates with >50 events
- Use `IntervalSnapshotStrategy` with interval of 50-100
- Snapshots reduce load time by ~90% for large aggregates

```go
strategy := eventsourcing.NewIntervalSnapshotStrategy(50)
repo := bankaccount.NewAccountRepositoryWithSnapshots(
    eventStore,
    snapshotStore,
    strategy,
)
```

### Projections

- Use checkpoint store to track progress
- Process events in batches for better throughput
- Use NATS for distributed projections

### Middleware

- Keep middleware lightweight (< 1ms overhead each)
- Avoid I/O in middleware hot path
- Use context for request-scoped data

## Security Considerations

### Command Validation

Always validate:
- User permissions (use authorization middleware)
- Input data types and formats
- Business rule constraints
- Aggregate state before mutations

### Event Store

- Never expose raw SQL interfaces to clients
- Use parameterized queries (handled by SQLC)
- Validate aggregate IDs to prevent injection
- Events are immutable - never allow deletion

### Sensitive Data

- Don't log sensitive fields (use middleware to redact)
- Consider encrypting snapshot data
- Use correlation IDs for audit trails
- Implement GDPR-compliant event deletion if needed

## Dependencies

### Core Dependencies

```go
// Protocol Buffers
google.golang.org/protobuf v1.36.10

// RPC framework
connectrpc.com/connect v1.17.0

// Event bus
github.com/nats-io/nats.go v1.47.0

// Database
modernc.org/sqlite v1.39.1  // Pure Go, no CGo

// Observability
go.opentelemetry.io/otel v1.38.0
```

### Development Tools

```bash
# Task runner
go install github.com/go-task/task/v3/cmd/task@latest

# Proto tools
go install github.com/bufbuild/buf/cmd/buf@latest
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install connectrpc.com/connect/cmd/protoc-gen-connect-go@latest

# SQL tools
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest

# Linting
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

## Key Abstractions

### Event Store

Primary interface for event persistence:
```go
type EventStore interface {
    SaveEvents(events []*Event) error
    LoadEvents(aggregateID string, afterVersion int64) ([]*Event, error)
    Close() error
}
```

### Snapshot Store

Optional optimization for large aggregates:
```go
type SnapshotStore interface {
    SaveSnapshot(snapshot *Snapshot) error
    GetLatestSnapshot(aggregateID string) (*Snapshot, error)
    DeleteOldSnapshots(aggregateID string, beforeVersion int64) error
}
```

### Command Bus

Handles command routing with middleware:
```go
type CommandBus interface {
    RegisterHandler(commandType string, handler CommandHandler)
    Use(middleware Middleware)
    Dispatch(ctx context.Context, cmd interface{}) (*CommandResult, error)
}
```

### Event Bus

Publishes events for projections:
```go
type EventBus interface {
    Publish(ctx context.Context, events []*Event) error
    Subscribe(ctx context.Context, handler EventHandler) error
}
```

## Testing Checklist

Before submitting changes:

- [ ] `task generate` completes without errors
- [ ] `task fmt` applied
- [ ] `task lint` passes with no warnings
- [ ] `task test` passes all tests
- [ ] New code has test coverage >80%
- [ ] Integration tests pass
- [ ] Example tests demonstrate new features
- [ ] Documentation updated (README.md, CONTRIBUTING.md)
- [ ] This AGENTS.md updated if build process changed

## Release Process

**Note**: This is for maintainers, but useful context for contributors.

1. Update version in code
2. Run full test suite: `task test`
3. Update CHANGELOG.md
4. Tag release: `git tag v1.x.x`
5. Push tags: `git push --tags`
6. GitHub Actions builds and publishes

## Additional Resources

- **README.md** - Human-focused project overview (includes snapshot guide)
- **CONTRIBUTING.md** - Detailed contribution guidelines
- **examples/EXAMPLE.md** - Step-by-step tutorial
- **go.dev/doc** - Go language documentation
- **buf.build/docs** - Protocol Buffers tooling
- **docs.nats.io** - NATS event bus documentation

---

**Last Updated**: 2025-01-20

This file follows the [agents.md](https://agents.md/) specification for AI coding agent instructions.
