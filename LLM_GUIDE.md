# LLM Guide: Event Sourcing & CQRS Framework

**Last Updated**: 2025-01-24
**Architecture**: Go-idiomatic CQRS + Event Sourcing with Protocol Buffers

---

## 🎯 Repository Overview

This is a **Go-based Event Sourcing and CQRS framework** that uses Protocol Buffers for type-safe message definitions and code generation. The framework follows strict separation of concerns with idiomatic Go error handling.

### Core Philosophy

1. **CQRS handles transport** - All command/query routing and network communication
2. **Event Sourcing handles domain** - Pure business logic with aggregates and events
3. **Protocol provides shared types** - Common error types for wire protocol
4. **Go-idiomatic patterns** - Standard `(result, error)` signatures, no wrapper types

---

## 📦 Package Structure

```
pkg/
├── cqrs/              # Command Query Responsibility Segregation (Transport Layer)
│   ├── transport.go   # Transport interface: Request(ctx, subject, msg) (proto.Message, error)
│   ├── server.go      # Server interface for registering handlers
│   ├── client.go      # Client options and subject builders
│   └── nats/          # NATS implementation of transport
│       ├── transport.go  # NATS client transport with header-based error signaling
│       └── server.go     # NATS microservices server
│
├── eventsourcing/     # Event Sourcing Domain Layer (NO TRANSPORT)
│   ├── aggregate.go   # Base aggregate root implementation
│   ├── event.go       # Event definitions and interfaces
│   ├── projection.go  # Read model projection interfaces
│   ├── snapshot.go    # Snapshot support for aggregate state
│   ├── commandbus.go  # In-memory command bus
│   └── errors.go      # Domain errors (ErrAggregateNotFound, ErrConcurrencyConflict)
│
├── store/             # Event Store Persistence
│   ├── eventstore.go  # EventStore interface
│   └── postgres/      # PostgreSQL event store implementation
│
├── domain/            # Core domain types
│   ├── aggregate.go   # AggregateRoot interface
│   ├── event.go       # Event, EventMetadata, EventEnvelope
│   └── constraint.go  # UniqueConstraint support
│
├── protocol/          # Wire Protocol Shared Types
│   └── error.go       # AppError type implementing Go error interface
│
└── observability/     # Tracing and Metrics
    └── middleware.go  # OpenTelemetry middleware for CQRS handlers

cmd/
├── protoc-gen-cqrs/          # Protocol Buffer plugin for CQRS code generation
└── protoc-gen-eventsourcing/ # Protocol Buffer plugin for event sourcing domain
```

---

## 🔧 Code Generation Pattern

### Two Separate Generators

#### 1. **protoc-gen-cqrs** (Transport/Routing)

Generates:
- `*_client.cqrs.pb.go` - Type-safe CQRS clients
- `*_server.cqrs.pb.go` - CQRS server handlers

**Example Generated Interface:**
```go
// Handler interface developers implement
type CqrsAccountCommandServiceHandler interface {
    OpenAccount(ctx context.Context, cmd *OpenAccountCommand) (*OpenAccountResponse, error)
    Deposit(ctx context.Context, cmd *DepositCommand) (*DepositResponse, error)
}

// Client for sending commands
type CqrsAccountCommandServiceClient struct {
    transport cqrs.Transport
}

func (c *CqrsAccountCommandServiceClient) OpenAccount(
    ctx context.Context,
    cmd *OpenAccountCommand,
) (*OpenAccountResponse, error) {
    subject := c.subjectBuilder.BuildSubject(ctx, "account.v1", "AccountCommandService", "OpenAccount")
    return c.transport.Request(ctx, subject, cmd)
}
```

#### 2. **protoc-gen-eventsourcing** (Domain Model)

Generates:
- `*_aggregate.es.pb.go` - Aggregate roots, event appliers, repositories, projections

**Example Generated Code:**
```go
// Aggregate root
type AccountAggregate struct {
    domain.AggregateRoot
    *Account  // Embeds proto-defined state
    applier AccountEventApplier
}

// Event applier interface (implement in domain layer)
type AccountEventApplier interface {
    ApplyAccountOpenedEvent(agg *AccountAggregate, e *AccountOpenedEvent) error
    ApplyMoneyDepositedEvent(agg *AccountAggregate, e *MoneyDepositedEvent) error
}

// Type-safe event application
func (a *AccountAggregate) ApplyAccountOpenedEvent(
    event *AccountOpenedEvent,
    opts ...ApplyEventOption,
) error {
    return a.AggregateRoot.ApplyChange(event, "account.v1.AccountOpenedEvent", metadata)
}

// Repository
type AccountRepository struct {
    *store.BaseRepository[*AccountAggregate]
}
```

---

## 🎨 Design Patterns

### Pattern 1: CQRS Handler Implementation

**Purpose**: Implement command/query handlers that receive messages via CQRS transport

**Best Practice:**
```go
package handlers

import (
    "context"
    "fmt"

    accountv1 "github.com/example/pb/account/v1"
    "github.com/plaenen/eventstore/pkg/protocol"
)

// Handler implements CQRS-generated interfaces
type AccountHandler struct {
    repo *accountv1.AccountRepository
}

// ✅ CORRECT: Idiomatic Go signature
func (h *AccountHandler) OpenAccount(
    ctx context.Context,
    cmd *accountv1.OpenAccountCommand,
) (*accountv1.OpenAccountResponse, error) {
    // Validation
    if cmd.AccountId == "" {
        return nil, protocol.ErrInvalidArgument("Account ID is required")
    }

    // Load or create aggregate
    agg := domain.NewAccount(cmd.AccountId)

    // Apply event
    event := &accountv1.AccountOpenedEvent{
        AccountId: cmd.AccountId,
        OwnerName: cmd.OwnerName,
    }

    if err := agg.ApplyAccountOpenedEvent(event); err != nil {
        return nil, fmt.Errorf("failed to apply event: %w", err)
    }

    // Persist
    if err := h.repo.Save(agg); err != nil {
        return nil, fmt.Errorf("failed to save: %w", err)
    }

    return &accountv1.OpenAccountResponse{
        AccountId: cmd.AccountId,
        Version:   agg.Version(),
    }, nil
}

// ✅ CORRECT: Ensure interface compliance at compile time
var _ accountv1.CqrsAccountCommandServiceHandler = (*AccountHandler)(nil)
var _ accountv1.CqrsAccountQueryServiceHandler = (*AccountHandler)(nil)
```

**❌ WRONG - Old Pattern (No Longer Supported):**
```go
// DON'T USE: eventsourcing.MethodOption is REMOVED
func (h *AccountHandler) OpenAccount(
    ctx context.Context,
    cmd *accountv1.OpenAccountCommand,
    opts ...eventsourcing.MethodOption,  // ❌ REMOVED
) (*accountv1.OpenAccountResponse, error)

// DON'T USE: eventsourcing.AppError return type is REMOVED
func (h *AccountHandler) OpenAccount(
    ctx context.Context,
    cmd *accountv1.OpenAccountCommand,
) (*accountv1.OpenAccountResponse, *eventsourcing.AppError)  // ❌ REMOVED
```

---

### Pattern 2: Event Applier Implementation

**Purpose**: Implement business logic that applies events to aggregate state

**Best Practice:**
```go
package domain

import (
    accountv1 "github.com/example/pb/account/v1"
    "github.com/shopspring/decimal"
)

// Applier implementation (in domain layer, outside pb/)
type AccountAppliers struct{}

// ✅ CORRECT: Implement the generated interface
func (ap *AccountAppliers) ApplyAccountOpenedEvent(
    agg *accountv1.AccountAggregate,
    e *accountv1.AccountOpenedEvent,
) error {
    // Update aggregate state from event
    agg.AccountId = e.AccountId
    agg.OwnerName = e.OwnerName
    agg.Balance = e.InitialBalance
    agg.Status = accountv1.AccountStatus_ACCOUNT_STATUS_OPEN
    return nil
}

func (ap *AccountAppliers) ApplyMoneyDepositedEvent(
    agg *accountv1.AccountAggregate,
    e *accountv1.MoneyDepositedEvent,
) error {
    // Business logic: update balance
    agg.Balance = e.NewBalance
    return nil
}

// Factory function that injects appliers
func NewAccount(id string) *accountv1.AccountAggregate {
    applier := &AccountAppliers{}
    return accountv1.NewAccount(id, applier)
}
```

**Key Points:**
- Appliers are **pure functions** that update state
- They are **idempotent** - applying the same event multiple times produces same result
- They **never fail** business validation (validation happens in command handlers)
- They are implemented **outside the pb/ directory** (in your domain layer)

---

### Pattern 3: CQRS Server Setup

**Purpose**: Wire up CQRS handlers to receive commands/queries over NATS

**Best Practice:**
```go
package main

import (
    "context"
    "time"

    accountv1 "github.com/example/pb/account/v1"
    "github.com/example/handlers"
    cqrsnats "github.com/plaenen/eventstore/pkg/cqrs/nats"
    "github.com/plaenen/eventstore/pkg/store/postgres"
)

func main() {
    ctx := context.Background()

    // 1. Create event store
    eventStore, _ := postgres.NewEventStore(dbConfig)

    // 2. Create repository with factory
    repo := accountv1.NewAccountRepository(
        eventStore,
        func(id string) *accountv1.AccountAggregate {
            return domain.NewAccount(id) // Injects appliers
        },
    )

    // 3. Create handler
    handler := handlers.NewAccountHandler(repo)

    // 4. Create NATS server
    server, _ := cqrsnats.NewServer(&cqrsnats.ServerConfig{
        ServerConfig: &cqrs.ServerConfig{
            QueueGroup:     "account-service",
            MaxConcurrent:  10,
            HandlerTimeout: 5 * time.Second,
        },
        URL:  "nats://localhost:4222",
        Name: "AccountService",
    })
    defer server.Close()

    // 5. Register command and query handlers
    commandServer := accountv1.NewCqrsAccountCommandServiceServer(server, handler)
    commandServer.Start(ctx)

    queryServer := accountv1.NewCqrsAccountQueryServiceServer(server, handler)
    queryServer.Start(ctx)

    // Server now listens on NATS subjects like:
    // - account.v1.AccountCommandService.OpenAccount
    // - account.v1.AccountQueryService.GetAccount
}
```

---

### Pattern 4: CQRS Client Usage

**Purpose**: Send commands/queries from client code

**Best Practice:**
```go
package main

import (
    "context"

    accountv1 "github.com/example/pb/account/v1"
    cqrsnats "github.com/plaenen/eventstore/pkg/cqrs/nats"
)

func main() {
    ctx := context.Background()

    // Create transport
    transport, _ := cqrsnats.NewTransport(&cqrsnats.TransportConfig{
        URL:  "nats://localhost:4222",
        Name: "account-client",
    })
    defer transport.Close()

    // Create client (uses default subject builder)
    client := accountv1.NewCqrsAccountCommandServiceClient(transport)

    // Send command - returns (response, error) idiomatic Go style
    resp, err := client.OpenAccount(ctx, &accountv1.OpenAccountCommand{
        AccountId:      "acc-123",
        OwnerName:      "John Doe",
        InitialBalance: "1000.00",
    })
    if err != nil {
        // err is a standard Go error or protocol.AppError
        log.Printf("Failed to open account: %v", err)
        return
    }

    log.Printf("Account opened: %s, version: %d", resp.AccountId, resp.Version)
}
```

---

### Pattern 5: Multi-Tenancy with Custom SubjectBuilder

**Purpose**: Route messages to tenant-specific subjects for multi-tenant deployments

**Best Practice:**
```go
package main

import (
    "context"
    "fmt"

    "github.com/plaenen/eventstore/pkg/cqrs"
)

// Custom subject builder for multi-tenancy
type TenantSubjectBuilder struct{}

func (t *TenantSubjectBuilder) BuildSubject(
    ctx context.Context,
    packageName, serviceName, methodName string,
) string {
    tenantID := GetTenantIDFromContext(ctx)
    // Format: tenant-{id}.package.service.method
    // Example: "tenant-acme.account.v1.AccountCommandService.OpenAccount"
    return fmt.Sprintf("tenant-%s.%s.%s.%s", tenantID, packageName, serviceName, methodName)
}

func main() {
    transport, _ := cqrsnats.NewTransport(&cqrsnats.TransportConfig{
        URL: "nats://localhost:4222",
    })

    // Use custom subject builder
    client := accountv1.NewCqrsAccountCommandServiceClient(
        transport,
        cqrs.WithClientSubjectBuilder(&TenantSubjectBuilder{}),
    )

    // Add tenant to context
    ctx := context.WithValue(context.Background(), "tenant_id", "acme")

    // Command is routed to: tenant-acme.account.v1.AccountCommandService.OpenAccount
    resp, err := client.OpenAccount(ctx, &accountv1.OpenAccountCommand{
        AccountId: "acc-123",
    })
}
```

---

### Pattern 6: Projections (Read Models)

**Purpose**: Build read models by subscribing to events

**Best Practice:**
```go
package projections

import (
    "context"
    "database/sql"

    accountv1 "github.com/example/pb/account/v1"
    "github.com/plaenen/eventstore/pkg/domain"
)

// Projection builder pattern
func NewAccountBalanceProjection(db *sql.DB) eventsourcing.Projection {
    return accountv1.NewAccountProjectionBuilder("account-balance").
        OnAccountOpened(func(ctx context.Context, e *accountv1.AccountOpenedEvent, env *domain.EventEnvelope) error {
            _, err := db.ExecContext(ctx, `
                INSERT INTO account_balances (account_id, balance, version)
                VALUES ($1, $2, $3)
            `, e.AccountId, e.InitialBalance, env.Version)
            return err
        }).
        OnMoneyDeposited(func(ctx context.Context, e *accountv1.MoneyDepositedEvent, env *domain.EventEnvelope) error {
            _, err := db.ExecContext(ctx, `
                UPDATE account_balances
                SET balance = $1, version = $2
                WHERE account_id = $3
            `, e.NewBalance, env.Version, e.AccountId)
            return err
        }).
        OnReset(func(ctx context.Context) error {
            _, err := db.ExecContext(ctx, `TRUNCATE TABLE account_balances`)
            return err
        }).
        Build()
}

// Usage
func main() {
    projection := NewAccountBalanceProjection(db)

    // Subscribe to event stream
    eventStore.Subscribe(ctx, projection, store.SubscribeOptions{
        FromBeginning: true,
    })
}
```

---

### Pattern 7: Unique Constraints

**Purpose**: Enforce uniqueness across aggregates (e.g., unique email, unique username)

**Best Practice:**
```go
func (h *AccountHandler) OpenAccount(
    ctx context.Context,
    cmd *accountv1.OpenAccountCommand,
) (*accountv1.OpenAccountResponse, error) {
    agg := domain.NewAccount(cmd.AccountId)

    event := &accountv1.AccountOpenedEvent{
        AccountId: cmd.AccountId,
        Email:     cmd.Email,
    }

    // Apply event with unique constraint
    err := agg.ApplyAccountOpenedEvent(event,
        accountv1.WithUniqueConstraints(
            domain.UniqueConstraint{
                IndexName: "account_id",
                Value:     cmd.AccountId,
                Operation: domain.ConstraintClaim,  // Claim this value
            },
            domain.UniqueConstraint{
                IndexName: "email",
                Value:     cmd.Email,
                Operation: domain.ConstraintClaim,
            },
        ),
    )
    if err != nil {
        // Will return eventsourcing.ErrUniqueConstraintViolation if violated
        return nil, err
    }

    return h.repo.Save(agg)
}

// When closing account, release constraints
func (h *AccountHandler) CloseAccount(
    ctx context.Context,
    cmd *accountv1.CloseAccountCommand,
) (*accountv1.CloseAccountResponse, error) {
    agg, _ := h.repo.Load(cmd.AccountId)

    event := &accountv1.AccountClosedEvent{
        AccountId: cmd.AccountId,
    }

    // Release the email so it can be reused
    err := agg.ApplyAccountClosedEvent(event,
        accountv1.WithUniqueConstraints(
            domain.UniqueConstraint{
                IndexName: "email",
                Value:     agg.Email,
                Operation: domain.ConstraintRelease,  // Release this value
            },
        ),
    )

    return h.repo.Save(agg)
}
```

---

### Pattern 8: Optimistic Concurrency Control

**Purpose**: Handle concurrent updates to the same aggregate

**Best Practice:**
```go
func (h *AccountHandler) Deposit(
    ctx context.Context,
    cmd *accountv1.DepositCommand,
) (*accountv1.DepositResponse, error) {
    var response *accountv1.DepositResponse

    // Retry on concurrency conflicts (up to 3 times)
    err := h.repo.RetryOnConflict(cmd.AccountId, 3, func(agg *accountv1.AccountAggregate) error {
        // This closure may be called multiple times if there's a conflict

        // Business logic
        currentBalance, _ := decimal.NewFromString(agg.Balance)
        newBalance := currentBalance.Add(amount)

        event := &accountv1.MoneyDepositedEvent{
            AccountId:  cmd.AccountId,
            Amount:     cmd.Amount,
            NewBalance: newBalance.String(),
        }

        if err := agg.ApplyMoneyDepositedEvent(event); err != nil {
            return err
        }

        // Save will fail with eventsourcing.ErrConcurrencyConflict if version changed
        if err := h.repo.Save(agg); err != nil {
            return err  // Return as-is for retry detection
        }

        response = &accountv1.DepositResponse{
            NewBalance: newBalance.String(),
            Version:    agg.Version(),
        }
        return nil
    })

    if err != nil {
        return nil, fmt.Errorf("deposit failed after retries: %w", err)
    }

    return response, nil
}
```

---

## 🚫 Common Mistakes to Avoid

### ❌ Mistake 1: Using Old eventsourcing.Response Wrapper
```go
// WRONG - Response wrapper is REMOVED
func (h *Handler) DoSomething() (*eventsourcing.Response, error) {
    result := &MyResponse{}
    return eventsourcing.NewSuccessResponse(result)  // ❌ REMOVED
}

// CORRECT - Return proto message directly
func (h *Handler) DoSomething() (*MyResponse, error) {
    return &MyResponse{}, nil  // ✅ Idiomatic Go
}
```

### ❌ Mistake 2: Using eventsourcing.MethodOption
```go
// WRONG - MethodOption is REMOVED
func (h *Handler) OpenAccount(
    ctx context.Context,
    cmd *OpenAccountCommand,
    opts ...eventsourcing.MethodOption,  // ❌ REMOVED
) (*OpenAccountResponse, error)

// CORRECT - Use standard context for metadata
func (h *Handler) OpenAccount(
    ctx context.Context,
    cmd *OpenAccountCommand,
) (*OpenAccountResponse, error) {
    // Extract metadata from context if needed
    tenantID, _ := ctx.Value("tenant_id").(string)
}
```

### ❌ Mistake 3: Mixing CQRS and EventSourcing Transport
```go
// WRONG - eventsourcing.Transport is REMOVED
import "github.com/plaenen/eventstore/pkg/eventsourcing"

transport := eventsourcing.NewTransport()  // ❌ REMOVED

// CORRECT - Use cqrs.Transport
import cqrsnats "github.com/plaenen/eventstore/pkg/cqrs/nats"

transport, _ := cqrsnats.NewTransport(&cqrsnats.TransportConfig{
    URL: "nats://localhost:4222",
})
```

### ❌ Mistake 4: Returning *eventsourcing.AppError
```go
// WRONG - eventsourcing.AppError is REMOVED from eventsourcing package
func (h *Handler) DoSomething() (*Response, *eventsourcing.AppError) {  // ❌
    return nil, &eventsourcing.AppError{Code: "ERROR"}
}

// CORRECT - Use protocol.AppError or standard errors
import "github.com/plaenen/eventstore/pkg/protocol"

func (h *Handler) DoSomething() (*Response, error) {  // ✅
    return nil, protocol.ErrInvalidArgument("Something is invalid")
}
```

### ❌ Mistake 5: Implementing Old Handler Interfaces
```go
// WRONG - UnimplementedAccountHandler is NO LONGER GENERATED
type Handler struct {
    accountv1.UnimplementedAccountHandler  // ❌ NOT GENERATED
}

// CORRECT - Implement CQRS interfaces directly
type Handler struct {
    repo *accountv1.AccountRepository
}

// Add compile-time checks
var _ accountv1.CqrsAccountCommandServiceHandler = (*Handler)(nil)
var _ accountv1.CqrsAccountQueryServiceHandler = (*Handler)(nil)
```

---

## 📝 Proto File Annotations

### CQRS Service Definition

```protobuf
syntax = "proto3";
package account.v1;

import "cqrs/options.proto";

// Mark service for CQRS code generation
service AccountCommandService {
  option (cqrs.service) = {
    generate_client: true
    generate_server: true
  };

  rpc OpenAccount(OpenAccountCommand) returns (OpenAccountResponse);
  rpc Deposit(DepositCommand) returns (DepositResponse);
}

service AccountQueryService {
  option (cqrs.service) = {
    generate_client: true
    generate_server: true
  };

  rpc GetAccount(GetAccountRequest) returns (AccountView);
  rpc ListAccounts(ListAccountsRequest) returns (ListAccountsResponse);
}
```

### Event Sourcing Definitions

```protobuf
syntax = "proto3";
package account.v1;

import "eventsourcing/options.proto";

// Aggregate root
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

// Events
message AccountOpenedEvent {
  option (eventsourcing.event) = {
    aggregate_name: "Account"
  };

  string account_id = 1;
  string owner_name = 2;
  string initial_balance = 3;
  int64 timestamp = 4;
}

message MoneyDepositedEvent {
  option (eventsourcing.event) = {
    aggregate_name: "Account"
  };

  string account_id = 1;
  string amount = 2;
  string new_balance = 3;
  int64 timestamp = 4;
}
```

---

## 🔍 Error Handling Reference

### Protocol Error Helpers

```go
import "github.com/plaenen/eventstore/pkg/protocol"

// Standard error codes
protocol.ErrInvalidArgument("Account ID is required")
protocol.ErrNotFound("Account not found")
protocol.ErrConflict("Account already exists")
protocol.ErrInternal("Internal server error")
protocol.ErrTimeout("Request timed out")
protocol.ErrUnauthenticated("Invalid credentials")
protocol.ErrPermissionDenied("Permission denied")
```

### Domain Error Constants

```go
import "github.com/plaenen/eventstore/pkg/eventsourcing"

// Check for specific domain errors
if errorx.Is(err, eventsourcing.ErrAggregateNotFound) {
    // Handle not found
}
if errorx.Is(err, eventsourcing.ErrConcurrencyConflict) {
    // Retry the operation
}
if errorx.Is(err, eventsourcing.ErrUniqueConstraintViolation) {
    // Handle constraint violation
}
```

### Wire Protocol Error Signaling

NATS messages use headers to indicate success/error:

**Success Response:**
```
Headers:
  Response-Type: account.v1.OpenAccountResponse
Body: <protobuf serialized response>
```

**Error Response:**
```
Headers:
  Error: true
Body: <JSON serialized protocol.AppError>
{
  "code": "INVALID_ARGUMENT",
  "message": "Account ID is required",
  "solution": "Provide a valid account ID",
  "details": {}
}
```

---

## 🏗️ Project Structure Best Practices

### Recommended Layout

```
myproject/
├── proto/                      # Proto definitions
│   ├── account/
│   │   └── v1/
│   │       ├── account.proto          # Aggregates, events
│   │       ├── commands.proto         # Command messages
│   │       └── queries.proto          # Query messages
│   └── buf.gen.yaml                   # Buf code generation config
│
├── pb/                         # Generated proto code (gitignored)
│   └── account/
│       └── v1/
│           ├── account.pb.go
│           ├── account_aggregate.es.pb.go    # Generated by protoc-gen-eventsourcing
│           ├── account_client.cqrs.pb.go     # Generated by protoc-gen-cqrs
│           └── account_server.cqrs.pb.go     # Generated by protoc-gen-cqrs
│
├── domain/                     # Domain layer (business logic)
│   └── account/
│       ├── appliers.go               # Event appliers implementation
│       ├── factory.go                # Aggregate factory
│       └── validators.go             # Business validation
│
├── handlers/                   # CQRS handlers (application layer)
│   └── account/
│       ├── command_handler.go        # Command handler
│       └── query_handler.go          # Query handler
│
├── projections/                # Read models
│   └── account/
│       └── balance_projection.go
│
├── cmd/
│   ├── server/                # CQRS server (commands/queries)
│   │   └── main.go
│   └── projector/             # Projection runner
│       └── main.go
│
└── migrations/                # Database migrations
    └── postgres/
```

---

## 🧪 Testing Patterns

### Unit Testing Handlers

```go
func TestOpenAccount(t *testing.T) {
    // Setup
    eventStore := store.NewInMemoryEventStore()
    repo := accountv1.NewAccountRepository(eventStore, domain.NewAccount)
    handler := handlers.NewAccountHandler(repo)

    ctx := context.Background()

    // Execute
    resp, err := handler.OpenAccount(ctx, &accountv1.OpenAccountCommand{
        AccountId:      "acc-123",
        OwnerName:      "John Doe",
        InitialBalance: "1000.00",
    })

    // Assert
    assert.NoError(t, err)
    assert.Equal(t, "acc-123", resp.AccountId)
    assert.Equal(t, int64(1), resp.Version)

    // Verify events
    events, _ := eventStore.LoadEvents(ctx, "Account", "acc-123", 0)
    assert.Len(t, events, 1)
    assert.Equal(t, "account.v1.AccountOpenedEvent", events[0].EventType)
}
```

### Integration Testing with NATS

```go
func TestAccountService_Integration(t *testing.T) {
    // Start embedded NATS
    ns, _ := server.NewServer(&server.Options{Port: 14222})
    go ns.Start()
    defer ns.Shutdown()

    // Start server
    handler := setupHandler(t)
    srv := setupCQRSServer(t, handler)
    defer srv.Close()

    // Create client
    transport, _ := cqrsnats.NewTransport(&cqrsnats.TransportConfig{
        URL: "nats://localhost:14222",
    })
    client := accountv1.NewCqrsAccountCommandServiceClient(transport)

    // Test
    resp, err := client.OpenAccount(ctx, &accountv1.OpenAccountCommand{
        AccountId: "acc-123",
    })

    assert.NoError(t, err)
    assert.NotNil(t, resp)
}
```

---

## 🎯 Quick Reference Checklist

When implementing a new feature, follow this checklist:

### ✅ For Commands (Write Operations)

1. [ ] Define command message in `commands.proto`
2. [ ] Add to `CommandService` in proto
3. [ ] Run `buf generate` to generate CQRS code
4. [ ] Implement handler method matching `CqrsXXXCommandServiceHandler` interface
5. [ ] Use `(ctx context.Context, cmd *Command) (*Response, error)` signature
6. [ ] Return `protocol.ErrXXX()` for validation errors
7. [ ] Load or create aggregate
8. [ ] Create and apply event using type-safe `Apply{Event}` method
9. [ ] Save aggregate via repository
10. [ ] Return response with version number

### ✅ For Queries (Read Operations)

1. [ ] Define query message in `queries.proto`
2. [ ] Add to `QueryService` in proto
3. [ ] Run `buf generate`
4. [ ] Implement handler matching `CqrsXXXQueryServiceHandler` interface
5. [ ] Load aggregate or query read model
6. [ ] Return view/DTO, never return aggregate directly
7. [ ] Use `protocol.ErrNotFound()` if resource doesn't exist

### ✅ For Events (Domain Events)

1. [ ] Define event message in proto with `(eventsourcing.event)` option
2. [ ] Run `buf generate` to generate event applier interface
3. [ ] Implement applier in domain layer (outside pb/)
4. [ ] Update aggregate state based on event data
5. [ ] Keep appliers pure and idempotent

### ✅ For Projections (Read Models)

1. [ ] Use generated projection builder
2. [ ] Implement `On{Event}` handlers for each event type
3. [ ] Implement `OnReset` for rebuilding projection
4. [ ] Subscribe to event store
5. [ ] Handle errors gracefully (projection runner should retry)

---

## 💡 LLM Tips for Code Generation

### When Asked to Generate a Handler

1. **Always check** if the proto service is defined and CQRS code is generated
2. **Import correctly**: Use generated package (e.g., `accountv1`), not proto package
3. **Use idiomatic Go**: `(result, error)` pattern, NOT `*eventsourcing.Response`
4. **Add interface checks**: `var _ accountv1.CqrsXXXHandler = (*MyHandler)(nil)`
5. **Use protocol errors**: `protocol.ErrInvalidArgument()` not custom AppError

### When Asked to Generate an Aggregate

1. **Events first**: Define events before commands
2. **Implement appliers**: Always in domain layer, outside pb/
3. **Factory function**: Create `NewXXX()` that injects appliers
4. **Type-safe apply**: Use generated `ApplyXXXEvent()` methods

### When Asked About Architecture

1. **CQRS = Transport**: Commands/queries arrive via CQRS, not eventsourcing
2. **EventSourcing = Domain**: Aggregates, events, no transport code
3. **Separation is strict**: No overlap between packages
4. **Wire protocol**: Headers indicate success/error, not wrapper types

---

## 📚 Additional Resources

- **Examples**: See `examples/cmd/cqrs-multitenancy/` for working multi-tenant example
- **Proto Options**: See `proto/eventsourcing/options.proto` and `proto/cqrs/options.proto`
- **Generated Code**: Check `examples/pb/account/v1/` to see generated patterns

---

**Last Updated**: 2025-01-24
**Maintainer**: Event Sourcing Framework Team
**Architecture Version**: 2.0 (Go-Idiomatic)

---

## 🚨 Error Handling: Application vs System Errors

### Overview

This framework uses **Go-idiomatic error handling** with a critical distinction between:

1. **Application Errors** (Expected) - Business logic, validation, resource not found
2. **System Errors** (Unexpected) - Infrastructure failures, bugs, corruption

### Package: `pkg/errorx`

The `pkg/errorx` package provides comprehensive error handling with sentinel errors and structured types.

### Application Errors (Expected)

These represent valid application states and should be returned to clients:

```go
import "github.com/plaenen/eventstore/pkg/errorx"

// Sentinel errors for application logic
var (
    ErrNotFound           = errorx.New("resource not found")
    ErrAlreadyExists      = errorx.New("resource already exists")
    ErrConflict           = errorx.New("version conflict")
    ErrInvalidArgument    = errorx.New("invalid argument")
    ErrPermissionDenied   = errorx.New("permission denied")
    ErrUnauthenticated    = errorx.New("unauthenticated")
    ErrPreconditionFailed = errorx.New("precondition failed")
    ErrResourceExhausted  = errorx.New("resource exhausted")
)

// Repository-specific application errors
var (
    ErrAggregateNotFound         = errorx.New("aggregate not found")
    ErrEventStreamNotFound       = errorx.New("event stream not found")
    ErrConcurrencyConflict       = errorx.New("concurrency conflict")
    ErrInvalidVersion            = errorx.New("invalid version")
    ErrSnapshotNotFound          = errorx.New("snapshot not found")
    ErrUniqueConstraintViolation = errorx.New("unique constraint violation")
)
```

### System Errors (Unexpected)

These represent infrastructure failures and should be logged and sanitized:

```go
var (
    ErrInternal        = errorx.New("internal system error")
    ErrTimeout         = errorx.New("operation timeout")
    ErrUnavailable     = errorx.New("service unavailable")
    ErrDataCorruption  = errorx.New("data corruption detected")
)
```

### Pattern 1: Repository Implementation

```go
func (r *AccountRepository) Load(aggregateID string) (*Account, error) {
    // ✅ CORRECT: Validate input (APPLICATION ERROR)
    if aggregateID == "" {
        return nil, fmt.Errorf("aggregate_id: %w", errorx.ErrInvalidArgument)
    }

    // Load from database
    events, err := r.store.LoadEvents(aggregateID)
    if err != nil {
        // ✅ CORRECT: Check for APPLICATION error first
        if errorx.Is(err, errorx.ErrNotFound) {
            return nil, errorx.NewNotFoundError("Aggregate", aggregateID)
        }
        // ✅ CORRECT: Database failure is SYSTEM error - wrap with context
        return nil, fmt.Errorf("failed to load events: %w", err)
    }

    // ✅ CORRECT: Empty stream is APPLICATION error (not found)
    if len(events) == 0 {
        return nil, errorx.NewNotFoundError("Aggregate", aggregateID)
    }

    return aggregate, nil
}

func (r *AccountRepository) Save(aggregate *Account) error {
    err := r.store.AppendEvents(aggregate)
    if err != nil {
        // ✅ CORRECT: Check for APPLICATION errors
        if errorx.Is(err, errorx.ErrConcurrencyConflict) {
            return errorx.NewConflictError(
                aggregate.ID(),
                aggregate.Version(),
                -1,
            )
        }

        // ✅ CORRECT: Database failure is SYSTEM error
        return fmt.Errorf("failed to append events: %w", err)
    }

    return nil
}
```

**❌ WRONG: Not distinguishing error types**
```go
func (r *AccountRepository) Load(aggregateID string) (*Account, error) {
    events, err := r.store.LoadEvents(aggregateID)
    if err != nil {
        // ❌ ALL errors treated the same (no distinction)
        return nil, err
    }
    return aggregate, nil
}
```

### Pattern 2: Command Handler with Error Classification

```go
func (h *AccountHandler) OpenAccount(
    ctx context.Context,
    cmd *OpenAccountCommand,
) (*OpenAccountResponse, error) {
    // ✅ CORRECT: Validation errors are APPLICATION errors
    if cmd.AccountId == "" {
        return nil, fmt.Errorf("account_id: %w", errorx.ErrInvalidArgument)
    }

    // Create and save aggregate
    agg := domain.NewAccount(cmd.AccountId)
    // ... apply events ...

    if err := h.repo.Save(agg); err != nil {
        // ✅ CORRECT: Check for specific APPLICATION errors
        if errorx.Is(err, errorx.ErrUniqueConstraintViolation) {
            return nil, fmt.Errorf("account %s: %w", cmd.AccountId, errorx.ErrAlreadyExists)
        }

        // ✅ CORRECT: System errors are logged and sanitized
        logger.Error("failed to save aggregate",
            "aggregate_id", cmd.AccountId,
            "error", err,
        )
        return nil, errorx.ErrInternal
    }

    return &OpenAccountResponse{
        AccountId: cmd.AccountId,
        Version:   agg.Version(),
    }, nil
}
```

**❌ WRONG: Leaking internal errors to clients**
```go
func (h *AccountHandler) OpenAccount(...) (*OpenAccountResponse, error) {
    if err := h.repo.Save(agg); err != nil {
        // ❌ Returning raw database error to client (security issue)
        return nil, err
    }
    return response, nil
}
```

### Pattern 3: Transport Layer Error Handling

```go
func (s *Server) HandleCommand(ctx context.Context, req *CommandRequest) error {
    response, err := s.handler.Execute(ctx, req)
    if err != nil {
        // ✅ CORRECT: Classify error type
        if errorx.IsApplicationError(err) {
            // Return to client with details
            return ConvertToProtocolError(err)
        }

        if errorx.IsSystemError(err) {
            // ✅ CORRECT: Log full error, return sanitized message
            logger.Error("system error", "error", err)
            return protocol.ErrInternal("an internal error occurred")
        }

        // ✅ CORRECT: Unknown errors treated as SYSTEM errors
        logger.Error("unexpected error", "error", err)
        return protocol.ErrInternal("an internal error occurred")
    }

    return nil
}

func ConvertToProtocolError(err error) error {
    switch {
    case errorx.Is(err, errorx.ErrNotFound):
        return protocol.ErrNotFound(err.Error())
    case errorx.Is(err, errorx.ErrConflict):
        return protocol.ErrConflict(err.Error())
    case errorx.Is(err, errorx.ErrInvalidArgument):
        return protocol.ErrInvalidArgument(err.Error())
    default:
        return protocol.ErrInternal("an error occurred")
    }
}
```

### Pattern 4: Retry Logic for Retryable Errors

```go
func (h *AccountHandler) Deposit(
    ctx context.Context,
    cmd *DepositCommand,
) (*DepositResponse, error) {
    const maxRetries = 3

    var lastErr error
    for attempt := 0; attempt < maxRetries; attempt++ {
        response, err := h.tryDeposit(ctx, cmd)
        if err == nil {
            return response, nil
        }

        lastErr = err

        // ✅ CORRECT: Only retry if error is retryable
        if !errorx.IsRetryable(err) {
            return nil, err
        }

        // ✅ CORRECT: Exponential backoff
        if attempt < maxRetries-1 {
            backoff := time.Duration(1<<attempt) * 100 * time.Millisecond
            time.Sleep(backoff)
        }
    }

    return nil, fmt.Errorf("operation failed after %d retries: %w", maxRetries, lastErr)
}
```

### Error Classification Helper Functions

```go
// Check if error is expected (business logic)
if errorx.IsApplicationError(err) {
    // Return to client with details
}

// Check if error is unexpected (infrastructure)
if errorx.IsSystemError(err) {
    // Log and sanitize
}

// Check if error might succeed on retry
if errorx.IsRetryable(err) {
    // Implement retry logic
}
```

### Structured Error Types

For richer error context, use structured error types:

```go
// Not found with details
return errorx.NewNotFoundError("Aggregate", aggregateID)

// Version conflict with versions
return errorx.NewConflictError(aggregateID, expected, actual)

// Unique constraint with details
return errorx.NewUniqueConstraintError("account_id", accountID, ownerID)

// Validation error with field
return errorx.NewValidationError("email", value, "must be valid email format")
```

### Error Decision Tree

```
When you encounter an error, ask:

1. Is it EXPECTED? (validation, not found, conflict)
   ├─ YES → Return APPLICATION error with details
   └─ NO  → Continue to 2

2. Is it RETRYABLE? (concurrency, timeout)
   ├─ YES → Implement retry logic
   └─ NO  → Continue to 3

3. Is it UNEXPECTED? (database, network, bug)
   └─ YES → Log full error, return SYSTEM error (sanitized)
```

### Error Handling Matrix

| Error Type          | Classification | Action              | Client Response |
|---------------------|----------------|---------------------|-----------------|
| Not Found           | APPLICATION    | Return with ID      | 404 + details   |
| Already Exists      | APPLICATION    | Return with ID      | 409 + details   |
| Invalid Input       | APPLICATION    | Return with field   | 400 + details   |
| Version Conflict    | APPLICATION    | Retry or return     | 409 + details   |
| Permission Denied   | APPLICATION    | Return              | 403 + message   |
| Database Failure    | SYSTEM         | Log + sanitize      | 500 + generic   |
| Network Timeout     | SYSTEM         | Log + retry/fail    | 503 + generic   |
| Data Corruption     | SYSTEM         | Log + alert         | 500 + generic   |

---

## ✅ Input Validation Package

### Package: `pkg/validation`

The validation package provides Go-idiomatic validators with sentinel errors.

### Validation Sentinel Errors

```go
import "github.com/plaenen/eventstore/pkg/validation"

var (
    ErrInvalidUUID       = errorx.New("invalid UUID format")
    ErrInvalidEmail      = errorx.New("invalid email format")
    ErrInvalidTenantID   = errorx.New("invalid tenant_id")
    ErrEmptyValue        = errorx.New("value cannot be empty")
    ErrTooShort          = errorx.New("value too short")
    ErrTooLong           = errorx.New("value too long")
    ErrTooLarge          = errorx.New("size too large")
    ErrInvalidVersion    = errorx.New("invalid version")
)
```

### ✅ CORRECT: Use Package Functions Directly

```go
import "github.com/plaenen/eventstore/pkg/validation"

func (h *Handler) OpenAccount(ctx context.Context, cmd *OpenAccountCommand) error {
    // ✅ CORRECT: Use package-level functions
    if err := validation.ValidateAggregateID(cmd.AccountId); err != nil {
        return fmt.Errorf("validation failed: %w", err)
    }

    if err := validation.ValidateEmail(cmd.OwnerEmail); err != nil {
        return fmt.Errorf("validation failed: %w", err)
    }

    // ✅ CORRECT: Check error type with errorx.Is()
    if err := validation.ValidateStringLength(cmd.OwnerName, "owner_name", 1, 100); err != nil {
        if errorx.Is(err, validation.ErrTooLong) {
            // Handle too long specifically
        }
        return err
    }

    return nil
}
```

### ❌ WRONG: Don't Use InputValidators Struct (Removed)

```go
// ❌ WRONG: This pattern was removed (not Go-idiomatic)
validators := validation.DefaultInputValidators()
err := validators.ValidateAggregateID(id)  // ❌ Don't do this
```

### Available Validators

```go
// Identity validation
validation.ValidateUUIDv4(uuid string) error
validation.ValidateAggregateID(aggregateID string) error
validation.ValidateCommandID(commandID string) error
validation.ValidateTenantID(tenantID string) error
validation.ValidatePrincipalID(principalID string) error

// Format validation
validation.ValidateEmail(email string) error
validation.ValidateEventType(eventType string) error
validation.ValidateAggregateType(aggregateType string) error

// Size validation
validation.ValidateStringLength(value, fieldName string, minLength, maxLength int) error
validation.ValidateStringNotEmpty(value, fieldName string) error
validation.ValidateArraySize(size int, fieldName string, maxSize int) error
validation.ValidateBinarySize(size int64, fieldName string, maxSize int64) error

// Version validation
validation.ValidateVersion(version int64) error
```

### Validation Pattern in Handlers

```go
func (h *Handler) CreateAccount(ctx context.Context, cmd *CreateAccountCommand) error {
    // ✅ CORRECT: Validate all inputs
    if err := validation.ValidateAggregateID(cmd.AccountId); err != nil {
        return err  // Already includes ErrInvalidArgument in chain
    }

    if err := validation.ValidateStringNotEmpty(cmd.OwnerName, "owner_name"); err != nil {
        return err
    }

    if err := validation.ValidateStringLength(cmd.OwnerName, "owner_name", 1, 256); err != nil {
        return err
    }

    if cmd.Email != "" {  // Optional field
        if err := validation.ValidateEmail(cmd.Email); err != nil {
            return err
        }
    }

    // Continue with business logic...
}
```

### Default Validation Limits

```go
const (
    DefaultMaxStringLength = 1000
    DefaultMaxTextLength   = 10000
    DefaultMaxNameLength   = 256
    DefaultMaxArraySize    = 100
    DefaultMaxBinarySize   = 10 * 1024 * 1024 // 10 MB
)
```

---

## 📝 Complete Error Handling Example

### Full Handler Implementation

```go
package handlers

import (
    "context"
    "fmt"

    "github.com/plaenen/eventstore/pkg/errorx"
    "github.com/plaenen/eventstore/pkg/validation"
    accountv1 "github.com/plaenen/eventstore/examples/pb/account/v1"
)

type AccountHandler struct {
    repo *accountv1.AccountRepository
}

func (h *AccountHandler) OpenAccount(
    ctx context.Context,
    cmd *accountv1.OpenAccountCommand,
) (*accountv1.OpenAccountResponse, error) {
    // ============================================================================
    // Step 1: Validate inputs (APPLICATION ERRORS)
    // ============================================================================

    if err := validation.ValidateAggregateID(cmd.AccountId); err != nil {
        // Validation error - APPLICATION ERROR
        return nil, err
    }

    if err := validation.ValidateStringNotEmpty(cmd.OwnerName, "owner_name"); err != nil {
        return nil, err
    }

    if err := validation.ValidateStringLength(cmd.OwnerName, "owner_name", 1, 256); err != nil {
        return nil, err
    }

    // ============================================================================
    // Step 2: Business logic validation (APPLICATION ERRORS)
    // ============================================================================

    balance, err := decimal.NewFromString(cmd.InitialBalance)
    if err != nil || balance.IsNegative() {
        return nil, errorx.NewValidationError(
            "initial_balance",
            cmd.InitialBalance,
            "must be a non-negative number",
        )
    }

    // ============================================================================
    // Step 3: Domain logic
    // ============================================================================

    agg := domain.NewAccount(cmd.AccountId)

    event := &accountv1.AccountOpenedEvent{
        AccountId:      cmd.AccountId,
        OwnerName:      cmd.OwnerName,
        InitialBalance: cmd.InitialBalance,
        Timestamp:      time.Now().Unix(),
    }

    if err := agg.ApplyAccountOpenedEvent(event); err != nil {
        // Event application failure on NEW aggregate = programming error (SYSTEM)
        return nil, fmt.Errorf("%w: failed to apply event: %v", errorx.ErrInternal, err)
    }

    // ============================================================================
    // Step 4: Persistence (handle both APPLICATION and SYSTEM errors)
    // ============================================================================

    if err := h.repo.Save(agg); err != nil {
        // Check for APPLICATION errors first
        if errorx.Is(err, errorx.ErrUniqueConstraintViolation) {
            return nil, fmt.Errorf("account %s: %w", cmd.AccountId, errorx.ErrAlreadyExists)
        }

        if errorx.Is(err, errorx.ErrConcurrencyConflict) {
            return nil, err  // Pass through (shouldn't happen on new aggregate)
        }

        // SYSTEM error - log and sanitize
        logger.Error("failed to save aggregate",
            "aggregate_id", cmd.AccountId,
            "error", err,
        )
        return nil, errorx.ErrInternal
    }

    return &accountv1.OpenAccountResponse{
        AccountId: cmd.AccountId,
        Version:   agg.Version(),
    }, nil
}

func (h *AccountHandler) Deposit(
    ctx context.Context,
    cmd *accountv1.DepositCommand,
) (*accountv1.DepositResponse, error) {
    // ============================================================================
    // Retry logic for concurrent modifications
    // ============================================================================

    const maxRetries = 3
    var lastErr error

    for attempt := 0; attempt < maxRetries; attempt++ {
        response, err := h.tryDeposit(ctx, cmd)
        if err == nil {
            return response, nil
        }

        lastErr = err

        // Only retry if error is retryable (concurrency conflict)
        if !errorx.IsRetryable(err) {
            return nil, err
        }

        // Exponential backoff
        if attempt < maxRetries-1 {
            backoff := time.Duration(1<<attempt) * 100 * time.Millisecond
            time.Sleep(backoff)
        }
    }

    return nil, fmt.Errorf("deposit failed after %d retries: %w", maxRetries, lastErr)
}

func (h *AccountHandler) tryDeposit(
    ctx context.Context,
    cmd *accountv1.DepositCommand,
) (*accountv1.DepositResponse, error) {
    // Validation
    if err := validation.ValidateAggregateID(cmd.AccountId); err != nil {
        return nil, err
    }

    amount, err := decimal.NewFromString(cmd.Amount)
    if err != nil || amount.LessThanOrEqual(decimal.Zero) {
        return nil, errorx.NewValidationError(
            "amount",
            cmd.Amount,
            "must be a positive number",
        )
    }

    // Load aggregate
    agg, err := h.repo.Load(cmd.AccountId)
    if err != nil {
        // Check for APPLICATION error (not found)
        if errorx.Is(err, errorx.ErrNotFound) || errorx.Is(err, errorx.ErrAggregateNotFound) {
            return nil, errorx.NewNotFoundError("Account", cmd.AccountId)
        }
        // SYSTEM error
        logger.Error("failed to load aggregate", "error", err)
        return nil, errorx.ErrInternal
    }

    // Business rule validation
    if agg.Status != accountv1.AccountStatus_ACCOUNT_STATUS_OPEN {
        return nil, fmt.Errorf("%w: account is closed", errorx.ErrPreconditionFailed)
    }

    // Apply event
    currentBalance, _ := decimal.NewFromString(agg.Balance)
    newBalance := currentBalance.Add(amount)

    event := &accountv1.MoneyDepositedEvent{
        AccountId:  cmd.AccountId,
        Amount:     cmd.Amount,
        NewBalance: newBalance.String(),
        Timestamp:  time.Now().Unix(),
    }

    if err := agg.ApplyMoneyDepositedEvent(event); err != nil {
        return nil, fmt.Errorf("%w: %v", errorx.ErrInternal, err)
    }

    // Save (might get concurrency conflict - that's why we retry)
    if err := h.repo.Save(agg); err != nil {
        return nil, err  // Propagate error (will be retried if retryable)
    }

    return &accountv1.DepositResponse{
        NewBalance: newBalance.String(),
        Version:    agg.Version(),
    }, nil
}
```

---

## 🧰 Error Handling Tooling (Optional Utilities)

The framework provides optional error handling utilities for common application patterns. These are **not required** but provide convenient helpers for HTTP APIs, retry logic, and logging.

### Package: `pkg/store/sqlite/errors.go`

**Purpose**: Translates SQLite/LibSQL errors to domain errors for the SQLite event store.

**Key Functions**:

```go
import "github.com/plaenen/eventstore/pkg/store/sqlite"

// translateError converts SQLite errors to pkg/errorx types
// Used internally by SQLite event store
func translateError(err error, resourceType, resourceID string) error

// Check if error should be retried (SQLITE_BUSY, SQLITE_LOCKED)
func isRetryable(err error) bool

// Check for constraint violations
func isConstraintViolation(err error) bool
func isUniqueViolation(err error) bool
```

**Error Mapping**:

```go
// SQLite Error → Domain Error
SQLITE_CONSTRAINT_PRIMARYKEY   → errorx.ErrConflict (version conflict)
SQLITE_CONSTRAINT_UNIQUE       → errorx.ErrUniqueConstraintViolation
SQLITE_BUSY / SQLITE_LOCKED    → errorx.ErrTimeout (retryable)
SQLITE_CONSTRAINT_FOREIGNKEY   → errorx.ErrInvalidArgument
SQLITE_CORRUPT                 → errorx.ErrDataCorruption
sql.ErrNoRows                  → errorx.ErrNotFound
```

**When to Use**: The SQLite event store automatically uses this translator. You don't need to call it directly unless building custom SQLite stores.

---

### Package: `pkg/http/errors.go`

**Purpose**: Converts domain errors to HTTP JSON responses for REST APIs.

**Key Types**:

```go
import httputil "github.com/plaenen/eventstore/pkg/http"

// ErrorResponse is the standard JSON error format
type ErrorResponse struct {
    Code    string `json:"code"`             // Machine-readable
    Message string `json:"message"`          // Human-readable
    Detail  string `json:"detail,omitempty"` // Specific details
    Hint    string `json:"hint,omitempty"`   // Actionable suggestion
}

// Handler wraps HTTP handlers that return errors
type Handler func(w http.ResponseWriter, r *http.Request) error

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    if err := h(w, r); err != nil {
        HandleError(w, r, err)
    }
}
```

**Pattern 1: Using Handler Wrapper**

```go
import (
    "net/http"
    httputil "github.com/plaenen/eventstore/pkg/http"
)

// ✅ CORRECT: Return errors directly from handler
func (h *MyHandler) GetAccount(w http.ResponseWriter, r *http.Request) error {
    id := r.PathValue("id")

    agg, err := h.repo.Load(id)
    if err != nil {
        return err  // Automatically converted to HTTP response
    }

    return json.NewEncoder(w).Encode(agg)
}

// Register with mux
mux.Handle("GET /accounts/{id}", httputil.Handler(handler.GetAccount))
```

**Pattern 2: Manual Error Handling**

```go
func (h *MyHandler) GetAccount(w http.ResponseWriter, r *http.Request) {
    agg, err := h.repo.Load(r.PathValue("id"))
    if err != nil {
        httputil.HandleError(w, r, err)
        return
    }

    json.NewEncoder(w).Encode(agg)
}
```

**Error to HTTP Status Mapping**:

```go
Domain Error                → HTTP Status
errorx.ErrNotFound          → 404 Not Found
errorx.ErrAlreadyExists     → 409 Conflict
errorx.ErrConflict          → 409 Conflict
errorx.ErrInvalidArgument   → 400 Bad Request
errorx.ErrPermissionDenied  → 403 Forbidden
errorx.ErrUnauthenticated   → 401 Unauthorized
errorx.ErrTimeout           → 503 Service Unavailable
errorx.ErrUnavailable       → 503 Service Unavailable
errorx.ErrDataCorruption    → 500 Internal Server Error
Unknown errors              → 500 Internal Server Error (sanitized)
```

**Example Error Response**:

```json
{
  "code": "NOT_FOUND",
  "message": "Aggregate not found",
  "detail": "No aggregate exists with ID 'acc-123'",
  "hint": "Verify the ID is correct, or check if the resource was deleted"
}
```

---

### Package: `pkg/retry/retry.go`

**Purpose**: Provides retry logic with exponential backoff for handling transient failures.

**Key Functions**:

```go
import "github.com/plaenen/eventstore/pkg/retry"

// Do executes an operation with retry logic (default config)
func Do(ctx context.Context, op Operation) error

// DoWithResult executes an operation returning a value
func DoWithResult[T any](ctx context.Context, op OperationWithResult[T]) (T, error)

// DoWithConfig allows custom retry configuration
func DoWithConfig(ctx context.Context, cfg Config, op Operation) error
```

**Pattern 1: Simple Retry**

```go
import (
    "context"
    "github.com/plaenen/eventstore/pkg/retry"
)

// ✅ CORRECT: Retry a save operation
err := retry.Do(ctx, func(ctx context.Context) error {
    return repository.Save(aggregate)
})
if err != nil {
    // Failed after retries
    return err
}
```

**Pattern 2: Retry with Result (Generic)**

```go
// ✅ CORRECT: Type-safe retry with result
aggregate, err := retry.DoWithResult(ctx, func(ctx context.Context) (*Account, error) {
    return repository.Load(aggregateID)
})
if err != nil {
    return nil, err
}
```

**Pattern 3: Custom Configuration**

```go
config := retry.NewConfig(
    retry.WithMaxAttempts(5),
    retry.WithInitialBackoff(50 * time.Millisecond),
    retry.WithMaxBackoff(5 * time.Second),
    retry.WithBackoffMultiplier(2.0),
    retry.WithJitter(true),
)

err := retry.DoWithConfig(ctx, config, func(ctx context.Context) error {
    return repository.Save(aggregate)
})
```

**Default Configuration**:

```go
MaxAttempts:       3
InitialBackoff:    100ms
MaxBackoff:        10s
BackoffMultiplier: 2.0
Jitter:            true
ShouldRetry:       errorx.IsRetryable() // Uses pkg/errorx classification
```

**Retry Logic**:

- Only retries errors where `errorx.IsRetryable(err)` returns `true`
- Uses exponential backoff: 100ms → 200ms → 400ms → ...
- Adds jitter (±25%) to avoid thundering herd
- Respects context cancellation
- Stops immediately for non-retryable errors

**Example: Retry on Concurrency Conflicts**

```go
err := retry.Do(ctx, func(ctx context.Context) error {
    agg, err := repo.Load(aggregateID)
    if err != nil {
        return err
    }

    // Apply changes
    event := &DepositEvent{Amount: "100.00"}
    if err := agg.ApplyDeposit(event); err != nil {
        return err
    }

    // Save (might fail with ErrConcurrencyConflict - will retry)
    return repo.Save(agg)
})
```

---

### Package: `pkg/logging/errors.go`

**Purpose**: Provides structured error logging with automatic classification.

**Key Types**:

```go
import "github.com/plaenen/eventstore/pkg/logging"

// ErrorLogger provides structured error logging
type ErrorLogger struct {
    logger *slog.Logger
}

func NewErrorLogger(logger *slog.Logger) *ErrorLogger
```

**Key Functions**:

```go
// LogError automatically classifies error and chooses appropriate level
func (l *ErrorLogger) LogError(ctx context.Context, msg string, args ...any)

// LogApplicationError uses Warn level (expected errors)
func (l *ErrorLogger) LogApplicationError(ctx context.Context, msg string, args ...any)

// LogSystemError uses Error level (unexpected errors)
func (l *ErrorLogger) LogSystemError(ctx context.Context, msg string, args ...any)

// LogWithDetails extracts details from structured error types
func (l *ErrorLogger) LogWithDetails(ctx context.Context, msg string, err error)
```

**Pattern 1: Automatic Classification**

```go
import (
    "log/slog"
    "github.com/plaenen/eventstore/pkg/logging"
)

logger := logging.NewErrorLogger(slog.Default())

// ✅ CORRECT: Automatically classifies and logs at appropriate level
if err := repo.Save(aggregate); err != nil {
    logger.LogError(ctx, "failed to save aggregate",
        "aggregate_id", aggregate.ID,
        "error", err,
    )
    return err
}

// APPLICATION errors → slog.Warn
// SYSTEM errors → slog.Error
```

**Pattern 2: Explicit Classification**

```go
// Force Warn level for APPLICATION errors
logger.LogApplicationError(ctx, "validation failed",
    "field", "account_id",
    "error", err,
)

// Force Error level for SYSTEM errors
logger.LogSystemError(ctx, "database connection failed",
    "error", err,
)
```

**Pattern 3: Logging with Extracted Details**

```go
// ✅ CORRECT: Automatically extracts context from structured errors
logger.LogWithDetails(ctx, "operation failed", err)

// For NotFoundError, logs:
//   resource_type, resource_id
// For ConflictError, logs:
//   aggregate_id, expected_version, actual_version
// For UniqueConstraintError, logs:
//   field, value
// For ValidationError, logs:
//   field, validation_message
```

**Default Package Functions**:

```go
// Use default logger (slog.Default())
logging.LogError(ctx, "operation failed", "error", err)
logging.LogApplicationError(ctx, "not found", "error", err)
logging.LogSystemError(ctx, "database error", "error", err)
logging.LogWithDetails(ctx, "failed", err)
```

**Error Metadata Added**:

The logger automatically adds:
- `error_type`: "APPLICATION" or "SYSTEM"
- `retryable`: `true` or `false`
- Structured details from error types (resource IDs, versions, etc.)

**Example Log Output**:

```json
{
  "time": "2025-01-24T10:30:00Z",
  "level": "WARN",
  "msg": "failed to save aggregate",
  "aggregate_id": "acc-123",
  "error": "Aggregate acc-123: version conflict (expected 5, got 6)",
  "error_type": "APPLICATION",
  "retryable": true,
  "expected_version": 5,
  "actual_version": 6
}
```

---

### When to Use These Tools

**SQLite Error Translator** (`pkg/store/sqlite/errors.go`):
- Automatically used by SQLite event store
- Use if building custom SQLite-based stores

**HTTP Error Handler** (`pkg/http/errors.go`):
- Building REST APIs
- Want consistent error responses across HTTP endpoints
- Need automatic domain error → HTTP status mapping

**Retry Helper** (`pkg/retry/retry.go`):
- Handling transient failures (database locks, network timeouts)
- Implementing retry logic for concurrency conflicts
- Need exponential backoff with jitter

**Error Logger** (`pkg/logging/errors.go`):
- Structured logging with slog
- Want automatic error classification (APPLICATION vs SYSTEM)
- Need rich error context in logs

**Note**: All these packages are **optional utilities**. The core framework (`pkg/errorx`) works independently and doesn't require them.

---

