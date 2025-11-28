# Building Applications with CQRS and Event Sourcing

This guide provides a step-by-step walkthrough for building scalable, multi-tenant applications using the Event Sourcing and CQRS framework provided by this project.

## Overview

The architecture separates **Writes (Commands)** from **Reads (Queries)**:

1.  **Command Side**: Handles business logic, validates commands, and emits events. State is derived purely from events.
2.  **Event Store**: The source of truth. Stores all events immutably.
3.  **Read Side**: Listens to events and updates optimized read models (Projections).
4.  **Query Side**: Serves data from read models.

## Prerequisites

*   Go 1.21+
*   `buf` (for Protobuf generation)
*   `protoc-gen-go`, `protoc-gen-connect-go`
*   NATS server (for messaging)

## Step 1: Define Service & Domain (Proto)

Define your service, commands, events, and aggregate state in a `.proto` file. Use the custom options to configure code generation.

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
import "account/domain/v1/account.proto"; // Import domain

// Command Service Definition
service AccountCommandService {
  option (eventsourcing.service) = {
    aggregate_name: "Account"
    aggregate_root_message: "account.domain.v1.Account" // Reference domain aggregate
    aggregate_handler: true
  };
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

## Step 2: Generate Code

Configure `buf.gen.yaml` to use the custom plugins:

```yaml
version: v2
plugins:
  - remote: buf.build/protocolbuffers/go
    out: pkg
    opt: paths=source_relative
  - remote: buf.build/connectrpc/go
    out: pkg
    opt: paths=source_relative
  - local: ["go", "run", "github.com/plaenen/eventstore/cmd/protoc-gen-eventsourcing"]
    out: pkg
    opt: paths=source_relative
  - local: ["go", "run", "github.com/plaenen/eventstore/cmd/protoc-gen-cqrs"]
    out: pkg
    opt: paths=source_relative
```

Run generation:
```bash
buf generate
```

This generates:
*   `AccountAggregate`: The domain object with `Apply*` methods.
*   `AccountRepository`: For loading/saving aggregates.
*   `AccountEventApplier`: Interface for domain logic.
*   `AccountCommandServiceHandler`: Interface for your service implementation.

## Step 3: Implement Domain Logic (Appliers)

Implement the `AccountEventApplier` interface to define how events mutate state. This is your **pure domain logic**.

```go
package domain

import (
    accountdomainv1 "your/project/pkg/account/domain/v1"
)

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

## Step 4: Implement Command Handler

Implement the `AccountCommandServiceHandler` interface. This is where you coordinate loading, validating, and saving.

```go
package handlers

import (
    "context"
    accountdomainv1 "your/project/pkg/account/domain/v1"
    accountservicev1 "your/project/pkg/account/service/v1"
    "your/project/pkg/domain"
)

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

        // Calculate new state
        // ... (decimal math) ...

        // Emit Event
        event := &accountdomainv1.MoneyDepositedEvent{
            AccountId: cmd.AccountId,
            Amount: cmd.Amount,
            NewBalance: newBalance,
        }
        
        // Apply Event (updates in-memory state)
        if err := agg.ApplyMoneyDepositedEvent(event); err != nil { return err }
        
        return nil // Save happens automatically if this returns nil
    })

    return &accountservicev1.DepositResponse{...}, err
}
```

## Step 5: Create Projections (Read Models)

Projections listen to the event stream and update a read-optimized database.

```go
type AccountProjection struct {
    db *sql.DB
}

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

## Step 6: Implement Query Service

Implement a standard RPC service that reads from the projection database.

```go
func (q *QueryService) GetAccount(ctx context.Context, req *GetAccountRequest) (*AccountView, error) {
    row := q.db.QueryRow("SELECT id, balance FROM accounts WHERE id = ?", req.AccountId)
    // ... scan and return
}
```

## Step 7: Wiring It All Together

In your `main.go`:

1.  Initialize **NATS Connection**.
2.  Initialize **EventStore** (e.g., SQLite/Postgres).
3.  Initialize **Repository**: `repo := accountservicev1.NewAccountRepository(eventStore, domain.NewAccountAppliers())`.
4.  Initialize **Command Handler**: `handler := handlers.NewAccountHandler(repo)`.
5.  Start **NATS Server**:
    ```go
    server, _ := nats.NewServer(&nats.ServerConfig{...})
    server.RegisterHandler("commands.account.deposit", handler.Deposit)
    server.Start(ctx)
    ```
6.  Start **Projections** (subscribe to `events.>`).

## Best Practices

*   **Tenant Isolation**: Always use `multitenancy.GetTenantID(ctx)` in your handlers if you need tenant-specific logic (though the framework handles data isolation automatically).
*   **Idempotency**: The `RetryOnConflict` helper handles optimistic locking. Ensure your logic inside the closure is side-effect free (except for applying events).
*   **Evolution**: Use Protobuf best practices for schema evolution (don't rename fields, use `deprecated` option).
