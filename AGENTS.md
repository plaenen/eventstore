# Event Sourcing Framework - Implementation Guide for AI Agents

**Version**: 0.0.11
**Last Updated**: 2025-10-26
**Target Audience**: AI Coding Agents (Claude Code, GitHub Copilot, Cursor, etc.)

---

## Overview

This is a comprehensive implementation guide for AI coding agents building applications with the Event Sourcing Framework. It covers everything from basic concepts to production deployment, including domain modeling, CQRS patterns, testing strategies, security best practices, and operational considerations.

**Module**: `github.com/plaenen/eventstore`

**Purpose**: Production-grade event sourcing framework with CQRS, projections, multi-tenancy, and comprehensive security.

**Status**: ✅ Production-ready (Phase 0 security complete)

**Approach**: **Protocol Buffers First** - This framework uses Protocol Buffers (proto3) to define aggregates, events, and commands. The protobuf compiler with the eventsourcing plugin generates all boilerplate code (aggregates, handlers, servers, clients). You only write:
- Proto definitions (schema)
- Event appliers (state mutations)
- Command handlers (business logic)

---

## Table of Contents

### Getting Started
1. [Quick Start](#quick-start)
2. [Core Concepts](#core-concepts)
3. [Architecture Overview](#architecture-overview)

### Building Applications
4. [Domain Modeling](#domain-modeling)
5. [Implementing Aggregates](#implementing-aggregates)
6. [Command Handlers](#command-handlers)
7. [Event Handlers](#event-handlers)
8. [Building Projections](#building-projections)
9. [Query Models](#query-models)

### Advanced Patterns
10. [Sagas and Process Managers](#sagas-and-process-managers)
11. [Event Versioning](#event-versioning)
12. [Snapshots](#snapshots)
13. [Multi-Tenancy](#multi-tenancy)

### Quality & Security
14. [Testing Strategies](#testing-strategies)
15. [Security Best Practices](#security-best-practices)
16. [Performance Optimization](#performance-optimization)

### Operations
17. [Configuration Guide](#configuration-guide)
18. [Monitoring & Observability](#monitoring--observability)
19. [Deployment](#deployment)
20. [Common Pitfalls](#common-pitfalls)

### Reference
21. [API Reference](#api-reference)
22. [Production Checklist](#production-checklist)
23. [Documentation Links](#documentation-links)

---

## Quick Start

### Installation

```bash
# Install the framework
go get github.com/plaenen/eventstore

# Install Protocol Buffers compiler (if not already installed)
# macOS:
brew install protobuf

# Linux:
apt-get install protobuf-compiler

# Install the eventsourcing protoc plugin
go install github.com/plaenen/eventstore/cmd/protoc-gen-eventsourcing@latest

# Install standard Go proto plugin
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
```

### Your First Event Sourced Application

This framework uses **Protocol Buffers** to define aggregates, events, and commands. The protobuf compiler with the eventsourcing plugin generates all the boilerplate code for you.

#### Step 1: Define Your Domain in Proto

Create `proto/account/v1/account.proto`:

```protobuf
syntax = "proto3";

package account.v1;

import "eventsourcing/options.proto";

// Define the command service (single source of truth)
service AccountCommandService {
  option (eventsourcing.service) = {
    aggregate_name: "Account"
    aggregate_root_message: "Account"
  };

  rpc OpenAccount(OpenAccountCommand) returns (OpenAccountResponse);
  rpc Deposit(DepositCommand) returns (DepositResponse);
}

// Commands (no options needed - inherited from service)
message OpenAccountCommand {
  string account_id = 1;
  string owner_name = 2;
  string initial_balance = 3;  // Decimal as string
}

message OpenAccountResponse {
  string account_id = 1;
  int64 version = 2;
}

message DepositCommand {
  string account_id = 1;
  string amount = 2;
}

message DepositResponse {
  string new_balance = 1;
  int64 version = 2;
}

// Aggregate root (minimal options)
message Account {
  option (eventsourcing.aggregate_root) = {
    id_field: "account_id"
  };

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

// Events (minimal options)
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

#### Step 2: Generate Code

```bash
# The protoc compiler generates all the boilerplate:
# - AccountAggregate (aggregate implementation)
# - AccountCommandHandlers (command handler scaffolding)
# - AccountEventApplier (interface for applying events)

protoc --go_out=. --eventsourcing_out=. proto/account/v1/account.proto
```

Generated files:
- `pb/account/v1/account.pb.go` - Proto message types
- `pb/account/v1/account_aggregate.es.pb.go` - Aggregate implementation
- `pb/account/v1/account_handler.es.pb.go` - Command handler scaffolding
- `pb/account/v1/account_server.es.pb.go` - gRPC server implementation

#### Step 3: Implement Business Logic

Create `domain/account_appliers.go` to implement event application:

```go
package domain

import (
    accountv1 "your-module/pb/account/v1"
)

type AccountAppliers struct{}

// Apply AccountOpenedEvent to aggregate state
func (ap *AccountAppliers) ApplyAccountOpenedEvent(
    agg *accountv1.AccountAggregate,
    e *accountv1.AccountOpenedEvent,
) error {
    agg.AccountId = e.AccountId
    agg.OwnerName = e.OwnerName
    agg.Balance = e.InitialBalance
    agg.Status = accountv1.AccountStatus_ACCOUNT_STATUS_OPEN
    return nil
}

// Apply MoneyDepositedEvent to aggregate state
func (ap *AccountAppliers) ApplyMoneyDepositedEvent(
    agg *accountv1.AccountAggregate,
    e *accountv1.MoneyDepositedEvent,
) error {
    agg.Balance = e.NewBalance
    return nil
}
```

Create `domain/account_handlers.go` to implement command handlers:

```go
package domain

import (
    "context"
    "time"

    accountv1 "your-module/pb/account/v1"
    "github.com/shopspring/decimal"
)

type AccountHandlers struct {
    appliers *AccountAppliers
}

func NewAccountHandlers() *AccountHandlers {
    return &AccountHandlers{
        appliers: &AccountAppliers{},
    }
}

// Handle OpenAccount command
func (h *AccountHandlers) HandleOpenAccount(
    ctx context.Context,
    agg *accountv1.AccountAggregate,
    cmd *accountv1.OpenAccountCommand,
) ([]proto.Message, error) {
    // Business validation
    balance, err := decimal.NewFromString(cmd.InitialBalance)
    if err != nil {
        return nil, fmt.Errorf("invalid balance: %w", err)
    }

    if balance.IsNegative() {
        return nil, errors.New("initial balance cannot be negative")
    }

    // Produce event
    return []proto.Message{
        &accountv1.AccountOpenedEvent{
            AccountId:      cmd.AccountId,
            OwnerName:      cmd.OwnerName,
            InitialBalance: cmd.InitialBalance,
            Timestamp:      time.Now().Unix(),
        },
    }, nil
}

// Handle Deposit command
func (h *AccountHandlers) HandleDeposit(
    ctx context.Context,
    agg *accountv1.AccountAggregate,
    cmd *accountv1.DepositCommand,
) ([]proto.Message, error) {
    // Business validation
    amount, _ := decimal.NewFromString(cmd.Amount)
    balance, _ := decimal.NewFromString(agg.Balance)

    if amount.LessThanOrEqual(decimal.Zero) {
        return nil, errors.New("deposit amount must be positive")
    }

    newBalance := balance.Add(amount)

    // Produce event
    return []proto.Message{
        &accountv1.MoneyDepositedEvent{
            AccountId:  agg.AccountId,
            Amount:     cmd.Amount,
            NewBalance: newBalance.String(),
            Timestamp:  time.Now().Unix(),
        },
    }, nil
}
```

#### Step 4: Set Up the Application

```go
package main

import (
    "context"
    "log"

    accountv1 "your-module/pb/account/v1"
    "your-module/domain"
    "github.com/plaenen/eventstore/pkg/store/sqlite"
)

func main() {
    // Create event store
    store, err := sqlite.NewEventStore(
        sqlite.WithFilename("bank.db"),
        sqlite.WithWALMode(true),
    )
    if err != nil {
        log.Fatal(err)
    }
    defer store.Close()

    // Create handlers and appliers
    handlers := domain.NewAccountHandlers()
    appliers := &domain.AccountAppliers{}

    // Create gRPC server (code-generated)
    server := accountv1.NewAccountCommandServer(store, handlers, appliers)

    // Use the server to execute commands
    ctx := context.Background()

    resp, err := server.OpenAccount(ctx, &accountv1.OpenAccountCommand{
        AccountId:      "acc-123",
        OwnerName:      "John Doe",
        InitialBalance: "100.00",
    })
    if err != nil {
        log.Fatal(err)
    }

    log.Printf("Account created! Version: %d", resp.Version)
}
```

That's it! The framework generates all the boilerplate, and you only write:
1. **Proto definitions** - Your domain model
2. **Event appliers** - How events modify state
3. **Command handlers** - Business logic and validation

Now let's explore the concepts in depth.

---

## Core Concepts

### Event Sourcing Fundamentals

**Event Sourcing** stores the state of a business entity as a sequence of state-changing events. Instead of storing the current state, you store the history of everything that happened.

**Traditional Approach**:
```
Current State: { balance: 100, owner: "John" }
```

**Event Sourcing Approach**:
```
Events:
1. AccountCreated { owner: "John", initial_balance: 0 }
2. MoneyDeposited { amount: 50 }
3. MoneyDeposited { amount: 50 }

Current State = Apply(Event1) → Apply(Event2) → Apply(Event3)
              = { balance: 100, owner: "John" }
```

### Key Benefits

1. **Complete Audit Trail** - Every state change is recorded
2. **Time Travel** - Reconstruct state at any point in time
3. **Event Replay** - Rebuild projections from events
4. **Scalability** - Append-only writes are fast
5. **Event-Driven** - Natural fit for event-driven architectures

### Core Components

```
┌─────────────────────────────────────────────────┐
│                  Application                    │
└─────────────────────────────────────────────────┘
                      │
                      ↓
┌─────────────────────────────────────────────────┐
│              Command Bus (CQRS)                 │
│  - Routes commands to handlers                  │
│  - Applies middleware (validation, auth, etc)   │
└─────────────────────────────────────────────────┘
                      │
                      ↓
┌─────────────────────────────────────────────────┐
│            Command Handler                       │
│  - Validates business rules                     │
│  - Produces events                              │
└─────────────────────────────────────────────────┘
                      │
                      ↓
┌─────────────────────────────────────────────────┐
│              Event Store                         │
│  - Persists events (append-only)                │
│  - Ensures consistency                          │
└─────────────────────────────────────────────────┘
                      │
                      ↓
┌─────────────────────────────────────────────────┐
│            Event Publisher                       │
│  - Publishes events to message bus (NATS)       │
└─────────────────────────────────────────────────┘
                      │
                      ↓
┌─────────────────────────────────────────────────┐
│            Event Handlers                        │
│  - Update projections (read models)             │
│  - Trigger side effects                         │
│  - Send notifications                           │
└─────────────────────────────────────────────────┘
```

### CQRS Pattern

**CQRS** (Command Query Responsibility Segregation) separates reads and writes:

**Commands** (Write Side):
- Modify state
- Go through command bus
- Produce events
- Return success/failure

**Queries** (Read Side):
- Read state
- Query projections
- Never modify state
- Optimized for reading

```go
// Command (Write) - Goes through event sourcing
result, err := commandBus.Dispatch(ctx, createAccountCommand)

// Query (Read) - Direct database query
account, err := db.QueryRow("SELECT * FROM accounts WHERE id = ?", accountID)
```

---

## Architecture Overview

### System Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                         Client Layer                          │
│  HTTP API / gRPC / GraphQL                                    │
└──────────────────────────────────────────────────────────────┘
                             │
                             ↓
┌──────────────────────────────────────────────────────────────┐
│                      Application Layer                        │
│  ┌────────────────┐  ┌────────────────┐  ┌────────────────┐ │
│  │  Command Bus   │  │   Query API    │  │   Event Bus    │ │
│  │    (CQRS)      │  │                │  │    (NATS)      │ │
│  └────────────────┘  └────────────────┘  └────────────────┘ │
└──────────────────────────────────────────────────────────────┘
         │                      │                      │
         ↓                      ↓                      ↓
┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐
│ Command Handler │  │  Read Models    │  │ Event Handlers  │
│  (Write Model)  │  │  (Projections)  │  │  (Subscribers)  │
└─────────────────┘  └─────────────────┘  └─────────────────┘
         │                      │                      │
         ↓                      ↓                      ↓
┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐
│  Event Store    │  │   Query DB      │  │  External APIs  │
│   (SQLite)      │  │   (SQLite)      │  │  Notifications  │
└─────────────────┘  └─────────────────┘  └─────────────────┘
```

### Middleware Stack

The command bus uses a middleware pipeline for cross-cutting concerns:

```go
commandBus := cqrs.NewCommandBus(
    // Error handling (catches and sanitizes errors)
    middleware.ProductionErrorSanitizationMiddleware(logger),
    middleware.RecoveryMiddleware(logger),

    // Validation (input validation)
    middleware.EnhancedValidationMiddleware(),

    // Security (authentication & authorization)
    middleware.AuthorizationMiddleware(authzService),
    middleware.TenantIsolationMiddleware(),

    // Observability (logging, metrics, tracing)
    middleware.LoggingMiddleware(logger),
    middleware.MetricsMiddleware(metrics),
    middleware.TracingMiddleware(tracer),
)
```

**Middleware Order Matters!** Place error handling first, then validation, then security.

---

## Domain Modeling

### Designing Aggregates with Protocol Buffers

An **aggregate** is a cluster of domain objects that can be treated as a single unit. Each aggregate has a root entity and a unique identifier.

**Good Aggregate Design**:
- Small and focused
- Protects business invariants
- Single responsibility
- Clear boundaries

**Example: E-Commerce Order in Proto**

Create `proto/order/v1/order.proto`:

```protobuf
syntax = "proto3";

package order.v1;

import "eventsourcing/options.proto";

// Define the command service
service OrderCommandService {
  option (eventsourcing.service) = {
    aggregate_name: "Order"
    aggregate_root_message: "Order"
  };

  rpc PlaceOrder(PlaceOrderCommand) returns (PlaceOrderResponse);
  rpc ShipOrder(ShipOrderCommand) returns (ShipOrderResponse);
  rpc CancelOrder(CancelOrderCommand) returns (CancelOrderResponse);
}

// Aggregate root
message Order {
  option (eventsourcing.aggregate_root) = {
    id_field: "order_id"
  };

  string order_id = 1;
  string customer_id = 2;
  repeated OrderItem items = 3;
  OrderStatus status = 4;
  string total = 5;  // Decimal as string
  int64 version = 6;
}

message OrderItem {
  string product_id = 1;
  int32 quantity = 2;
  string price = 3;  // Decimal as string
}

enum OrderStatus {
  ORDER_STATUS_UNSPECIFIED = 0;
  ORDER_STATUS_DRAFT = 1;
  ORDER_STATUS_PLACED = 2;
  ORDER_STATUS_SHIPPED = 3;
  ORDER_STATUS_DELIVERED = 4;
  ORDER_STATUS_CANCELLED = 5;
}
```

### Identifying Domain Events

Events represent things that have happened in your domain. Define them in your proto file:

```protobuf
// Good event names (past tense)
message OrderPlacedEvent {
  option (eventsourcing.event) = {
    aggregate_name: "Order"
  };

  string order_id = 1;
  string customer_id = 2;
  repeated OrderItem items = 3;
  string total = 4;
  int64 placed_at = 5;
}

message OrderShippedEvent {
  option (eventsourcing.event) = {
    aggregate_name: "Order"
  };

  string order_id = 1;
  string tracking_number = 2;
  int64 shipped_at = 3;
}

message OrderCancelledEvent {
  option (eventsourcing.event) = {
    aggregate_name: "Order"
  };

  string order_id = 1;
  string reason = 2;
  int64 cancelled_at = 3;
}
```

**Event Naming Conventions**:
- Use past tense (OrderPlaced, not PlaceOrder)
- Be specific (MoneyDeposited, not AccountUpdated)
- Include all data needed to understand what happened
- Include timestamp information
- Add `Event` suffix to the message name

### Defining Commands

Commands represent intentions to change state. Define them in your proto file:

```protobuf
// Commands use imperative form
message PlaceOrderCommand {
  string order_id = 1;
  string customer_id = 2;
  repeated OrderItem items = 3;
}

message PlaceOrderResponse {
  string order_id = 1;
  int64 version = 2;
}

message ShipOrderCommand {
  string order_id = 1;
  string tracking_number = 2;
}

message ShipOrderResponse {
  int64 version = 1;
}

message CancelOrderCommand {
  string order_id = 1;
  string reason = 2;
}

message CancelOrderResponse {
  int64 version = 1;
}
```

**Command Guidelines**:
- Use imperative form (PlaceOrder, not OrderPlaced)
- Include all required data
- Commands can fail (business rules)
- One command = one intention
- Define a response message for each command

---

## Implementing Aggregates

### Proto-Based Aggregate Implementation

With the proto-based approach, the aggregate structure is **generated automatically**. You only need to:
1. Define the aggregate in proto
2. Implement event appliers
3. Implement command handlers (business logic)

### Step 1: Proto Definition (Already Done)

From the previous section, you've defined `Order` in `proto/order/v1/order.proto`.

### Step 2: Generated Aggregate

After running `protoc`, the framework generates `pb/order/v1/order_aggregate.es.pb.go`:

```go
// Generated code (DO NOT EDIT)
type OrderAggregate struct {
    domain.AggregateRoot
    *Order  // Embeds the proto-defined Order state
    applier OrderEventApplier
}

func NewOrder(id string, applier OrderEventApplier) *OrderAggregate {
    return &OrderAggregate{
        AggregateRoot: domain.NewAggregateRoot(id, "Order"),
        Order:         &Order{},
        applier:       applier,
    }
}

// Interface you must implement
type OrderEventApplier interface {
    ApplyOrderPlacedEvent(agg *OrderAggregate, e *OrderPlacedEvent) error
    ApplyOrderShippedEvent(agg *OrderAggregate, e *OrderShippedEvent) error
    ApplyOrderCancelledEvent(agg *OrderAggregate, e *OrderCancelledEvent) error
}
```

### Step 3: Implement Event Appliers

Create `domain/order_appliers.go` to implement how events modify state:

```go
package domain

import (
    orderv1 "your-module/pb/order/v1"
    "github.com/shopspring/decimal"
)

type OrderAppliers struct{}

// Apply OrderPlacedEvent to aggregate state
func (ap *OrderAppliers) ApplyOrderPlacedEvent(
    agg *orderv1.OrderAggregate,
    e *orderv1.OrderPlacedEvent,
) error {
    agg.OrderId = e.OrderId
    agg.CustomerId = e.CustomerId
    agg.Items = e.Items
    agg.Total = e.Total
    agg.Status = orderv1.OrderStatus_ORDER_STATUS_PLACED
    return nil
}

// Apply OrderShippedEvent to aggregate state
func (ap *OrderAppliers) ApplyOrderShippedEvent(
    agg *orderv1.OrderAggregate,
    e *orderv1.OrderShippedEvent,
) error {
    agg.Status = orderv1.OrderStatus_ORDER_STATUS_SHIPPED
    return nil
}

// Apply OrderCancelledEvent to aggregate state
func (ap *OrderAppliers) ApplyOrderCancelledEvent(
    agg *orderv1.OrderAggregate,
    e *orderv1.OrderCancelledEvent,
) error {
    agg.Status = orderv1.OrderStatus_ORDER_STATUS_CANCELLED
    return nil
}
```

### Step 4: Implement Command Handlers (Business Logic)

Create `domain/order_handlers.go` to implement business rules:

```go
package domain

import (
    "context"
    "errors"
    "time"

    orderv1 "your-module/pb/order/v1"
    "github.com/shopspring/decimal"
    "google.golang.org/protobuf/proto"
)

type OrderHandlers struct{}

// Handle PlaceOrder command
func (h *OrderHandlers) HandlePlaceOrder(
    ctx context.Context,
    agg *orderv1.OrderAggregate,
    cmd *orderv1.PlaceOrderCommand,
) ([]proto.Message, error) {
    // Business validation
    if cmd.OrderId == "" {
        return nil, errors.New("order ID is required")
    }
    if cmd.CustomerId == "" {
        return nil, errors.New("customer ID is required")
    }
    if len(cmd.Items) == 0 {
        return nil, errors.New("order must have at least one item")
    }

    // Calculate total
    total := decimal.Zero
    for _, item := range cmd.Items {
        price, _ := decimal.NewFromString(item.Price)
        itemTotal := price.Mul(decimal.NewFromInt(int64(item.Quantity)))
        total = total.Add(itemTotal)
    }

    // Produce event
    return []proto.Message{
        &orderv1.OrderPlacedEvent{
            OrderId:    cmd.OrderId,
            CustomerId: cmd.CustomerId,
            Items:      cmd.Items,
            Total:      total.String(),
            PlacedAt:   time.Now().Unix(),
        },
    }, nil
}

// Handle ShipOrder command
func (h *OrderHandlers) HandleShipOrder(
    ctx context.Context,
    agg *orderv1.OrderAggregate,
    cmd *orderv1.ShipOrderCommand,
) ([]proto.Message, error) {
    // Business rules
    if agg.Status != orderv1.OrderStatus_ORDER_STATUS_PLACED {
        return nil, errors.New("only placed orders can be shipped")
    }

    // Produce event
    return []proto.Message{
        &orderv1.OrderShippedEvent{
            OrderId:        cmd.OrderId,
            TrackingNumber: cmd.TrackingNumber,
            ShippedAt:      time.Now().Unix(),
        },
    }, nil
}

// Handle CancelOrder command
func (h *OrderHandlers) HandleCancelOrder(
    ctx context.Context,
    agg *orderv1.OrderAggregate,
    cmd *orderv1.CancelOrderCommand,
) ([]proto.Message, error) {
    // Business rules
    if agg.Status == orderv1.OrderStatus_ORDER_STATUS_DELIVERED {
        return nil, errors.New("delivered orders cannot be cancelled")
    }
    if agg.Status == orderv1.OrderStatus_ORDER_STATUS_CANCELLED {
        return nil, errors.New("order is already cancelled")
    }

    // Produce event
    return []proto.Message{
        &orderv1.OrderCancelledEvent{
            OrderId:     cmd.OrderId,
            Reason:      cmd.Reason,
            CancelledAt: time.Now().Unix(),
        },
    }, nil
}
```

### Key Benefits of Proto-Based Approach

1. **Type Safety** - Proto definitions are strongly typed
2. **Code Generation** - Aggregate boilerplate is auto-generated
3. **Versioning** - Proto field evolution handles event upcasting
4. **Serialization** - Efficient binary serialization built-in
5. **Cross-Language** - Can consume events from other languages
6. **Clear Separation** - Proto (schema) vs. Go (business logic)

---

## Command Handlers

### Proto-Generated Command Server

The framework **generates a complete gRPC server** from your proto definitions. The generated server:
- Loads aggregates from the event store
- Routes commands to your handler methods
- Applies events via your appliers
- Persists events to the store
- Handles concurrency and versioning

### Generated Server Structure

After running `protoc`, the framework generates `pb/order/v1/order_server.es.pb.go`:

```go
// Generated code (DO NOT EDIT)
type OrderCommandServerImpl struct {
    store    store.AggregateStore
    handlers OrderCommandHandlers  // Interface you implement
    appliers OrderEventApplier     // Interface you implement
}

func NewOrderCommandServer(
    store store.AggregateStore,
    handlers OrderCommandHandlers,
    appliers OrderEventApplier,
) *OrderCommandServerImpl {
    return &OrderCommandServerImpl{
        store:    store,
        handlers: handlers,
        appliers: appliers,
    }
}

// Interface you must implement
type OrderCommandHandlers interface {
    HandlePlaceOrder(ctx context.Context, agg *OrderAggregate, cmd *PlaceOrderCommand) ([]proto.Message, error)
    HandleShipOrder(ctx context.Context, agg *OrderAggregate, cmd *ShipOrderCommand) ([]proto.Message, error)
    HandleCancelOrder(ctx context.Context, agg *OrderAggregate, cmd *CancelOrderCommand) ([]proto.Message, error)
}

// Generated gRPC method
func (s *OrderCommandServerImpl) PlaceOrder(
    ctx context.Context,
    cmd *PlaceOrderCommand,
) (*PlaceOrderResponse, error) {
    // 1. Create new aggregate (or load existing)
    agg := NewOrder(cmd.OrderId, s.appliers)

    // 2. Call your handler
    events, err := s.handlers.HandlePlaceOrder(ctx, agg, cmd)
    if err != nil {
        return nil, err
    }

    // 3. Save events to store
    if err := s.store.Save(ctx, agg, events); err != nil {
        return nil, err
    }

    // 4. Return response
    return &PlaceOrderResponse{
        OrderId: cmd.OrderId,
        Version: agg.GetVersion(),
    }, nil
}
```

### Your Handler Implementation

You only implement the business logic in `domain/order_handlers.go`:

```go
package domain

import (
    "context"
    "errors"

    orderv1 "your-module/pb/order/v1"
    "google.golang.org/protobuf/proto"
)

type OrderHandlers struct{}

// Handle PlaceOrder - creates new aggregate
func (h *OrderHandlers) HandlePlaceOrder(
    ctx context.Context,
    agg *orderv1.OrderAggregate,
    cmd *orderv1.PlaceOrderCommand,
) ([]proto.Message, error) {
    // 1. Validate command
    if err := h.validatePlaceOrder(cmd); err != nil {
        return nil, err
    }

    // 2. Business logic (already shown in previous section)
    // ...

    // 3. Return events
    return []proto.Message{
        &orderv1.OrderPlacedEvent{
            OrderId:    cmd.OrderId,
            CustomerId: cmd.CustomerId,
            // ... event fields
        },
    }, nil
}

// Handle ShipOrder - updates existing aggregate
func (h *OrderHandlers) HandleShipOrder(
    ctx context.Context,
    agg *orderv1.OrderAggregate,
    cmd *orderv1.ShipOrderCommand,
) ([]proto.Message, error) {
    // The aggregate is already loaded and hydrated by the server
    // You have access to the current state in agg.Status, agg.Items, etc.

    // 1. Check business rules using current state
    if agg.Status != orderv1.OrderStatus_ORDER_STATUS_PLACED {
        return nil, errors.New("only placed orders can be shipped")
    }

    // 2. Produce event
    return []proto.Message{
        &orderv1.OrderShippedEvent{
            OrderId:        cmd.OrderId,
            TrackingNumber: cmd.TrackingNumber,
            ShippedAt:      time.Now().Unix(),
        },
    }, nil
}

func (h *OrderHandlers) validatePlaceOrder(cmd *orderv1.PlaceOrderCommand) error {
    if cmd.OrderId == "" {
        return errors.New("order ID is required")
    }
    if cmd.CustomerId == "" {
        return errors.New("customer ID is required")
    }
    if len(cmd.Items) == 0 {
        return errors.New("order must have items")
    }
    return nil
}
```

### Wiring It All Together

In your `main.go`:

```go
package main

import (
    "context"
    "log"

    orderv1 "your-module/pb/order/v1"
    "your-module/domain"
    "github.com/plaenen/eventstore/pkg/store/sqlite"
    "google.golang.org/grpc"
)

func main() {
    // 1. Create event store
    store, err := sqlite.NewEventStore(
        sqlite.WithFilename("orders.db"),
        sqlite.WithWALMode(true),
    )
    if err != nil {
        log.Fatal(err)
    }
    defer store.Close()

    // 2. Create aggregate store wrapper
    aggregateStore := store.NewAggregateStore()

    // 3. Create your implementations
    handlers := &domain.OrderHandlers{}
    appliers := &domain.OrderAppliers{}

    // 4. Create the generated server
    orderServer := orderv1.NewOrderCommandServer(
        aggregateStore,
        handlers,
        appliers,
    )

    // 5. Use directly or expose via gRPC
    ctx := context.Background()

    // Direct usage:
    resp, err := orderServer.PlaceOrder(ctx, &orderv1.PlaceOrderCommand{
        OrderId:    "order-123",
        CustomerId: "customer-456",
        Items: []*orderv1.OrderItem{
            {ProductId: "prod-1", Quantity: 2, Price: "10.00"},
        },
    })

    // OR expose via gRPC:
    grpcServer := grpc.NewServer()
    orderv1.RegisterOrderCommandServiceServer(grpcServer, orderServer)
    // grpcServer.Serve(lis)
}
```

### Key Benefits

1. **No Boilerplate** - Server code is generated
2. **Type-Safe** - Proto messages ensure type safety
3. **Aggregate Loading** - Automatically handled by server
4. **Event Persistence** - Automatically handled by server
5. **Concurrency Control** - Optimistic locking built-in
6. **Focus on Business Logic** - You only write domain rules

---

## Event Handlers

### Projection Updates

Event handlers listen to events and update read models (projections):

```go
type OrderProjectionHandler struct {
    db *sql.DB
}

func (h *OrderProjectionHandler) HandleOrderPlaced(ctx context.Context, event *eventsourcing.Event) error {
    data := event.Data.(map[string]interface{})

    _, err := h.db.ExecContext(ctx, `
        INSERT INTO order_summary (
            order_id, customer_id, status, total, placed_at
        ) VALUES (?, ?, ?, ?, ?)
    `,
        event.AggregateID,
        data["customer_id"],
        "placed",
        data["total"],
        data["placed_at"],
    )

    return err
}

func (h *OrderProjectionHandler) HandleOrderShipped(ctx context.Context, event *eventsourcing.Event) error {
    data := event.Data.(map[string]interface{})

    _, err := h.db.ExecContext(ctx, `
        UPDATE order_summary
        SET status = 'shipped',
            tracking_number = ?,
            shipped_at = ?
        WHERE order_id = ?
    `,
        data["tracking_number"],
        data["shipped_at"],
        event.AggregateID,
    )

    return err
}
```

### Side Effects

Event handlers can also trigger side effects:

```go
type OrderNotificationHandler struct {
    emailService EmailService
}

func (h *OrderNotificationHandler) HandleOrderPlaced(ctx context.Context, event *eventsourcing.Event) error {
    data := event.Data.(map[string]interface{})

    return h.emailService.SendOrderConfirmation(
        data["customer_id"].(string),
        event.AggregateID,
        data["total"].(float64),
    )
}

func (h *OrderNotificationHandler) HandleOrderShipped(ctx context.Context, event *eventsourcing.Event) error {
    data := event.Data.(map[string]interface{})

    return h.emailService.SendShippingNotification(
        event.AggregateID,
        data["tracking_number"].(string),
    )
}
```

---

## Building Projections

### Creating a Projection

Projections are denormalized read models optimized for queries:

```go
// Create projection builder
projectionBuilder := sqlite.NewProjectionBuilder(db).
    WithName("order_summary").
    WithCreateTable(`
        CREATE TABLE IF NOT EXISTS order_summary (
            order_id TEXT PRIMARY KEY,
            customer_id TEXT NOT NULL,
            status TEXT NOT NULL,
            total REAL NOT NULL,
            placed_at TIMESTAMP NOT NULL,
            shipped_at TIMESTAMP,
            tracking_number TEXT,
            updated_at TIMESTAMP NOT NULL
        );

        CREATE INDEX IF NOT EXISTS idx_customer ON order_summary(customer_id);
        CREATE INDEX IF NOT EXISTS idx_status ON order_summary(status);
        CREATE INDEX IF NOT EXISTS idx_placed_at ON order_summary(placed_at DESC);
    `).
    OnEvent("OrderPlaced", func(ctx context.Context, event *eventsourcing.Event) error {
        data := event.Data.(map[string]interface{})
        return db.Exec(ctx, `
            INSERT INTO order_summary (order_id, customer_id, status, total, placed_at, updated_at)
            VALUES (?, ?, 'placed', ?, ?, ?)
        `, event.AggregateID, data["customer_id"], data["total"],
           data["placed_at"], time.Now())
    }).
    OnEvent("OrderShipped", func(ctx context.Context, event *eventsourcing.Event) error {
        data := event.Data.(map[string]interface{})
        return db.Exec(ctx, `
            UPDATE order_summary
            SET status = 'shipped',
                shipped_at = ?,
                tracking_number = ?,
                updated_at = ?
            WHERE order_id = ?
        `, data["shipped_at"], data["tracking_number"], time.Now(), event.AggregateID)
    }).
    OnEvent("OrderCancelled", func(ctx context.Context, event *eventsourcing.Event) error {
        return db.Exec(ctx, `
            UPDATE order_summary
            SET status = 'cancelled',
                updated_at = ?
            WHERE order_id = ?
        `, time.Now(), event.AggregateID)
    })

// Build projection (catches up from all historical events)
if err := projectionBuilder.Build(ctx); err != nil {
    return err
}
```

### Projection Strategies

**1. Single Table Per Aggregate**:
```sql
CREATE TABLE order_summary (...);  -- One row per order
```

**2. Denormalized Views**:
```sql
CREATE TABLE customer_orders (
    customer_id TEXT,
    order_count INT,
    total_spent REAL,
    last_order_date TIMESTAMP
);
```

**3. Specialized Queries**:
```sql
CREATE TABLE recent_orders (
    order_id TEXT,
    placed_at TIMESTAMP,
    -- Only orders from last 30 days
);
```

---

## Query Models

### Querying Projections

```go
type OrderQueryService struct {
    db *sql.DB
}

// Get single order
func (s *OrderQueryService) GetOrder(ctx context.Context, orderID string) (*OrderSummary, error) {
    var order OrderSummary
    err := s.db.QueryRowContext(ctx, `
        SELECT order_id, customer_id, status, total, placed_at, shipped_at
        FROM order_summary
        WHERE order_id = ?
    `, orderID).Scan(
        &order.OrderID,
        &order.CustomerID,
        &order.Status,
        &order.Total,
        &order.PlacedAt,
        &order.ShippedAt,
    )
    if err != nil {
        return nil, err
    }
    return &order, nil
}

// List orders with filters
func (s *OrderQueryService) ListOrders(ctx context.Context, filter OrderFilter) ([]*OrderSummary, error) {
    query := `
        SELECT order_id, customer_id, status, total, placed_at, shipped_at
        FROM order_summary
        WHERE 1=1
    `
    args := []interface{}{}

    if filter.CustomerID != "" {
        query += " AND customer_id = ?"
        args = append(args, filter.CustomerID)
    }

    if filter.Status != "" {
        query += " AND status = ?"
        args = append(args, filter.Status)
    }

    query += " ORDER BY placed_at DESC LIMIT ? OFFSET ?"
    args = append(args, filter.Limit, filter.Offset)

    rows, err := s.db.QueryContext(ctx, query, args...)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var orders []*OrderSummary
    for rows.Next() {
        var order OrderSummary
        if err := rows.Scan(
            &order.OrderID,
            &order.CustomerID,
            &order.Status,
            &order.Total,
            &order.PlacedAt,
            &order.ShippedAt,
        ); err != nil {
            return nil, err
        }
        orders = append(orders, &order)
    }

    return orders, nil
}

// Aggregate queries
func (s *OrderQueryService) GetCustomerStats(ctx context.Context, customerID string) (*CustomerStats, error) {
    var stats CustomerStats
    err := s.db.QueryRowContext(ctx, `
        SELECT
            COUNT(*) as order_count,
            SUM(total) as total_spent,
            MAX(placed_at) as last_order_date
        FROM order_summary
        WHERE customer_id = ?
    `, customerID).Scan(&stats.OrderCount, &stats.TotalSpent, &stats.LastOrderDate)

    return &stats, err
}
```

---

## Sagas and Process Managers

### Recommendation: Use Workflow Engines

For orchestrating long-running processes and sagas, **use dedicated workflow engines** instead of building your own:

- **[Inngest](https://www.inngest.com/)** - ⭐ **Recommended** - Simpler, event-driven, great developer experience
- **[Temporal](https://temporal.io/)** - More powerful, for complex workflows with high reliability requirements

**Why use a workflow engine?**
- ✅ Built-in state persistence and recovery
- ✅ Automatic retries and error handling
- ✅ Compensation/rollback logic
- ✅ Observability and debugging tools
- ✅ Durable execution across failures
- ✅ No need to manually manage saga state

### Approach 1: Inngest (Recommended - Simpler)

**Inngest** provides event-driven workflows with a simple, elegant API. It's perfect for most saga patterns.

#### Installation

```bash
go get github.com/inngest/inngestgo
```

#### Subscribe to Events

Set up Inngest to listen to your event store events:

```go
package workflows

import (
    "context"
    "fmt"

    "github.com/inngest/inngestgo"
    orderv1 "your-module/pb/order/v1"
)

// OrderFulfillmentWorkflow orchestrates order fulfillment
func OrderFulfillmentWorkflow(client *inngestgo.Client) *inngestgo.Function {
    return client.NewFunction(
        inngestgo.FunctionOpts{
            Name: "order-fulfillment",
        },
        inngestgo.EventTrigger("order/placed", nil),
        orderFulfillmentHandler,
    )
}

func orderFulfillmentHandler(ctx context.Context, input inngestgo.Input[OrderPlacedEvent]) (any, error) {
    order := input.Event.Data

    // Step 1: Process payment
    paymentResult, err := inngestgo.Step(ctx, "process-payment", func(ctx context.Context) (PaymentResult, error) {
        return processPayment(ctx, order.OrderId, order.Total)
    })
    if err != nil {
        return nil, fmt.Errorf("payment failed: %w", err)
    }

    // Step 2: Reserve inventory (automatic retry on failure)
    inventoryResult, err := inngestgo.Step(ctx, "reserve-inventory", func(ctx context.Context) (InventoryResult, error) {
        return reserveInventory(ctx, order.OrderId, order.Items)
    })
    if err != nil {
        // Automatic compensation: refund payment
        inngestgo.Step(ctx, "refund-payment", func(ctx context.Context) (any, error) {
            return refundPayment(ctx, paymentResult.PaymentID)
        })
        return nil, fmt.Errorf("inventory reservation failed: %w", err)
    }

    // Step 3: Ship order
    shipmentResult, err := inngestgo.Step(ctx, "ship-order", func(ctx context.Context) (ShipmentResult, error) {
        return shipOrder(ctx, order.OrderId, inventoryResult.ReservationID)
    })
    if err != nil {
        // Compensation: release inventory and refund payment
        inngestgo.Step(ctx, "release-inventory", func(ctx context.Context) (any, error) {
            return releaseInventory(ctx, inventoryResult.ReservationID)
        })
        inngestgo.Step(ctx, "refund-payment", func(ctx context.Context) (any, error) {
            return refundPayment(ctx, paymentResult.PaymentID)
        })
        return nil, fmt.Errorf("shipping failed: %w", err)
    }

    return map[string]any{
        "orderID":    order.OrderId,
        "paymentID":  paymentResult.PaymentID,
        "shipmentID": shipmentResult.TrackingNumber,
        "status":     "completed",
    }, nil
}

// Implementation of each step
func processPayment(ctx context.Context, orderID string, amount string) (PaymentResult, error) {
    // Call your payment aggregate via command
    // ...
    return PaymentResult{PaymentID: "pay-123"}, nil
}

func reserveInventory(ctx context.Context, orderID string, items []*orderv1.OrderItem) (InventoryResult, error) {
    // Call your inventory aggregate via command
    // ...
    return InventoryResult{ReservationID: "inv-123"}, nil
}

func shipOrder(ctx context.Context, orderID, reservationID string) (ShipmentResult, error) {
    // Call your shipping aggregate via command
    // ...
    return ShipmentResult{TrackingNumber: "TRACK-123"}, nil
}

func refundPayment(ctx context.Context, paymentID string) (any, error) {
    // Call refund command
    // ...
    return nil, nil
}

func releaseInventory(ctx context.Context, reservationID string) (any, error) {
    // Call inventory release command
    // ...
    return nil, nil
}
```

#### Integrate with Event Store

Send events from your event store to Inngest:

```go
package main

import (
    "context"

    "github.com/inngest/inngestgo"
    "github.com/plaenen/eventstore/pkg/domain"
)

func publishToInngest(client *inngestgo.Client, event *domain.Event) error {
    // Convert domain event to Inngest event
    _, err := client.Send(context.Background(), inngestgo.Event{
        Name: fmt.Sprintf("%s/%s",
            strings.ToLower(event.AggregateType),
            toKebabCase(event.EventType),
        ),
        Data: event.Data,
        ID:   event.EventID,
        Timestamp: event.Timestamp,
    })
    return err
}

// Set up event handler to forward events to Inngest
func setupInngestForwarding(eventBus *nats.EventBus, inngestClient *inngestgo.Client) {
    eventBus.Subscribe("*", func(ctx context.Context, event *domain.Event) error {
        return publishToInngest(inngestClient, event)
    })
}
```

#### Benefits of Inngest

- ✅ **Simple API** - Easy to read and write
- ✅ **Built-in retries** - Automatic exponential backoff
- ✅ **Step memoization** - Completed steps don't re-run
- ✅ **Visual debugging** - Web UI to see workflow execution
- ✅ **Event-driven** - Natural fit for event sourcing
- ✅ **Free tier** - Good for development and small projects

### Approach 2: Temporal (For Complex Cases)

**Temporal** is more powerful for complex, mission-critical workflows with advanced requirements.

#### Installation

```bash
go get go.temporal.io/sdk
```

#### Define Workflow

```go
package workflows

import (
    "time"

    "go.temporal.io/sdk/workflow"
)

type OrderFulfillmentInput struct {
    OrderID    string
    CustomerID string
    Items      []OrderItem
    Total      string
}

type OrderFulfillmentResult struct {
    OrderID     string
    PaymentID   string
    ShipmentID  string
    Status      string
}

// OrderFulfillmentWorkflow is a Temporal workflow
func OrderFulfillmentWorkflow(ctx workflow.Context, input OrderFulfillmentInput) (*OrderFulfillmentResult, error) {
    logger := workflow.GetLogger(ctx)

    // Configure activity options
    ao := workflow.ActivityOptions{
        StartToCloseTimeout: 10 * time.Minute,
        RetryPolicy: &temporal.RetryPolicy{
            InitialInterval:    time.Second,
            BackoffCoefficient: 2.0,
            MaximumInterval:    time.Minute,
            MaximumAttempts:    5,
        },
    }
    ctx = workflow.WithActivityOptions(ctx, ao)

    var result OrderFulfillmentResult
    result.OrderID = input.OrderID

    // Step 1: Process payment
    var paymentID string
    err := workflow.ExecuteActivity(ctx, ProcessPaymentActivity, input.OrderID, input.Total).Get(ctx, &paymentID)
    if err != nil {
        logger.Error("Payment failed", "error", err)
        return nil, err
    }
    result.PaymentID = paymentID

    // Step 2: Reserve inventory
    var reservationID string
    err = workflow.ExecuteActivity(ctx, ReserveInventoryActivity, input.OrderID, input.Items).Get(ctx, &reservationID)
    if err != nil {
        logger.Error("Inventory reservation failed", "error", err)
        // Compensate: refund payment
        workflow.ExecuteActivity(ctx, RefundPaymentActivity, paymentID).Get(ctx, nil)
        return nil, err
    }

    // Step 3: Ship order
    var trackingNumber string
    err = workflow.ExecuteActivity(ctx, ShipOrderActivity, input.OrderID, reservationID).Get(ctx, &trackingNumber)
    if err != nil {
        logger.Error("Shipping failed", "error", err)
        // Compensate: release inventory and refund payment
        workflow.ExecuteActivity(ctx, ReleaseInventoryActivity, reservationID).Get(ctx, nil)
        workflow.ExecuteActivity(ctx, RefundPaymentActivity, paymentID).Get(ctx, nil)
        return nil, err
    }

    result.ShipmentID = trackingNumber
    result.Status = "completed"

    return &result, nil
}

// Activities (the actual work)
func ProcessPaymentActivity(ctx context.Context, orderID, amount string) (string, error) {
    // Call your payment aggregate via command
    // ...
    return "payment-123", nil
}

func ReserveInventoryActivity(ctx context.Context, orderID string, items []OrderItem) (string, error) {
    // Call your inventory aggregate via command
    // ...
    return "reservation-123", nil
}

func ShipOrderActivity(ctx context.Context, orderID, reservationID string) (string, error) {
    // Call your shipping aggregate via command
    // ...
    return "TRACK-123", nil
}

func RefundPaymentActivity(ctx context.Context, paymentID string) error {
    // Call refund command
    // ...
    return nil
}

func ReleaseInventoryActivity(ctx context.Context, reservationID string) error {
    // Call inventory release command
    // ...
    return nil
}
```

#### Start Workflow from Event

```go
package main

import (
    "context"

    "go.temporal.io/sdk/client"
)

func handleOrderPlacedEvent(ctx context.Context, event *orderv1.OrderPlacedEvent, temporalClient client.Client) error {
    workflowOptions := client.StartWorkflowOptions{
        ID:        "order-fulfillment-" + event.OrderId,
        TaskQueue: "order-fulfillment",
    }

    input := OrderFulfillmentInput{
        OrderID:    event.OrderId,
        CustomerID: event.CustomerId,
        Items:      event.Items,
        Total:      event.Total,
    }

    _, err := temporalClient.ExecuteWorkflow(ctx, workflowOptions, OrderFulfillmentWorkflow, input)
    return err
}
```

#### Benefits of Temporal

- ✅ **Extremely reliable** - Guaranteed execution
- ✅ **Advanced features** - Signals, queries, child workflows, timers
- ✅ **Versioning** - Workflow code versioning
- ✅ **Visibility** - Powerful web UI
- ✅ **Polyglot** - Works across multiple languages
- ✅ **Self-hosted** - Full control over infrastructure

### Comparison: Inngest vs Temporal

| Feature | Inngest | Temporal |
|---------|---------|----------|
| **Ease of Use** | ⭐⭐⭐⭐⭐ Simple, minimal boilerplate | ⭐⭐⭐ More complex setup |
| **Event-Driven** | ⭐⭐⭐⭐⭐ Native event triggers | ⭐⭐⭐ Requires glue code |
| **Hosting** | ⭐⭐⭐⭐⭐ Managed (free tier) | ⭐⭐⭐ Self-hosted or Temporal Cloud |
| **Reliability** | ⭐⭐⭐⭐ Very good | ⭐⭐⭐⭐⭐ Exceptional |
| **Advanced Features** | ⭐⭐⭐ Good for most use cases | ⭐⭐⭐⭐⭐ Very advanced |
| **Developer Experience** | ⭐⭐⭐⭐⭐ Excellent | ⭐⭐⭐⭐ Good |
| **Free Tier** | ✅ Yes | ❌ Self-host only |
| **Best For** | Most sagas, simpler workflows | Mission-critical, complex workflows |

### When to Use What

**Use Inngest when:**
- You want simple, event-driven workflows
- Your sagas have < 10 steps
- You prefer managed services
- You want faster development velocity
- You're building a startup/MVP

**Use Temporal when:**
- You need guaranteed execution for critical workflows
- You have complex workflows with many branches
- You need advanced features (signals, queries, child workflows)
- You have strict compliance/audit requirements
- You're building enterprise systems

### Integration Pattern

Both tools integrate similarly with your event sourcing framework:

1. **Listen to Events** - Subscribe to domain events from your event store
2. **Start Workflow** - Trigger workflow when saga-initiating event occurs
3. **Execute Steps** - Each step executes commands on your aggregates
4. **Handle Failures** - Automatic retries and compensation
5. **Emit Events** - Workflows can emit events back to your event store

```go
// Common pattern for both Inngest and Temporal
func setupSagaOrchestration(eventBus *EventBus, orchestrator WorkflowOrchestrator) {
    // Subscribe to events that start sagas
    eventBus.Subscribe("order.OrderPlaced", func(ctx context.Context, event *Event) error {
        return orchestrator.StartWorkflow(ctx, "order-fulfillment", event.Data)
    })

    eventBus.Subscribe("payment.PaymentReceived", func(ctx context.Context, event *Event) error {
        return orchestrator.StartWorkflow(ctx, "payment-processing", event.Data)
    })
}
```

---

## Event Versioning

### Handling Schema Evolution with Proto

Events are immutable, but you need to evolve your schema. Protocol Buffers provide **built-in field evolution** that handles backward compatibility automatically.

**Proto Field Evolution Rules**:
1. You can add new fields (they default to zero/empty)
2. You can deprecate old fields
3. Never reuse field numbers
4. Never change field types

### Strategy 1: Proto Field Evolution (Recommended)

This is the simplest approach - use proto's built-in versioning:

```protobuf
message AccountOpenedEvent {
  option (eventsourcing.event) = {
    aggregate_name: "Account"
  };

  string account_id = 1;
  string owner_name = 2;

  // V1: Old field (deprecated but still readable)
  string initial_balance = 3 [deprecated = true];

  // V2: New fields added
  string opening_amount = 7;  // Replaces initial_balance
  string currency = 5;        // New in V2, defaults to ""
  int64 created_at = 6;       // New in V2, defaults to 0

  int64 timestamp = 4;
}
```

Your applier handles both old and new events:

```go
func (ap *AccountAppliers) ApplyAccountOpenedEvent(
    agg *accountv1.AccountAggregate,
    e *accountv1.AccountOpenedEvent,
) error {
    agg.AccountId = e.AccountId
    agg.OwnerName = e.OwnerName

    // Handle V1 events (initial_balance is set)
    if e.InitialBalance != "" {
        agg.Balance = e.InitialBalance
        agg.Currency = "USD"  // Default for old events
    } else {
        // Handle V2 events (opening_amount is set)
        agg.Balance = e.OpeningAmount
        agg.Currency = e.Currency
    }

    // New fields default to zero for old events
    if e.CreatedAt == 0 {
        agg.CreatedAt = e.Timestamp  // Fallback
    } else {
        agg.CreatedAt = e.CreatedAt
    }

    return nil
}
```

### Strategy 2: Custom Event Upcasting

For complex transformations, implement the `EventUpcaster` interface:

```go
// Implement on your aggregate
func (agg *AccountAggregate) UpcastEvent(event proto.Message) proto.Message {
    switch e := event.(type) {
    case *AccountOpenedEvent:
        // V1 event: has initial_balance but no opening_amount
        if e.InitialBalance != "" && e.OpeningAmount == "" {
            // Create V2 event
            return &AccountOpenedEvent{
                AccountId:     e.AccountId,
                OwnerName:     e.OwnerName,
                OpeningAmount: e.InitialBalance,  // Copy to new field
                Currency:      "USD",             // Default
                CreatedAt:     e.Timestamp,       // Use timestamp
                Timestamp:     e.Timestamp,
            }
        }
        return e

    case *OrderPlacedEventV1:
        // Transform old event version to new
        return &OrderPlacedEvent{
            OrderId:    e.OrderId,
            CustomerId: e.CustomerId,
            Subtotal:   calculateSubtotal(e.Amount),
            Tax:        calculateTax(e.Amount),
            Total:      e.Amount,
            PlacedAt:   e.Timestamp,
        }
    }

    return event
}
```

### Strategy 3: Separate Event Versions

For major breaking changes, define separate event messages:

```protobuf
// V1: Old event (deprecated)
message AccountOpenedEventV1 {
  option (eventsourcing.event) = {
    aggregate_name: "Account"
  };

  string account_id = 1;
  string owner_name = 2;
  string initial_balance = 3;
  int64 timestamp = 4;
}

// V2: Current event
message AccountOpenedEventV2 {
  option (eventsourcing.event) = {
    aggregate_name: "Account"
  };

  string account_id = 1;
  string owner_name = 2;
  string opening_amount = 3;
  string currency = 4;
  int64 created_at = 5;
  int64 timestamp = 6;
}
```

Handle both in your applier or use upcasting:

```go
func (agg *AccountAggregate) UpcastEvent(event proto.Message) proto.Message {
    switch e := event.(type) {
    case *AccountOpenedEventV1:
        // Upcast V1 to V2
        return &AccountOpenedEventV2{
            AccountId:     e.AccountId,
            OwnerName:     e.OwnerName,
            OpeningAmount: e.InitialBalance,
            Currency:      "USD",      // Default
            CreatedAt:     e.Timestamp,
            Timestamp:     e.Timestamp,
        }
    }
    return event
}

func (ap *AccountAppliers) ApplyAccountOpenedEventV2(
    agg *accountv1.AccountAggregate,
    e *accountv1.AccountOpenedEventV2,
) error {
    // Only handle V2 (V1 is upcasted)
    agg.AccountId = e.AccountId
    agg.OwnerName = e.OwnerName
    agg.Balance = e.OpeningAmount
    agg.Currency = e.Currency
    agg.CreatedAt = e.CreatedAt
    return nil
}
```

### Best Practices

1. **Prefer Field Evolution** - Use proto's built-in versioning when possible
2. **Add, Don't Remove** - Add new fields instead of removing old ones
3. **Deprecate Old Fields** - Mark old fields as deprecated
4. **Test Both Versions** - Ensure old events still work
5. **Document Changes** - Comment why fields were added/deprecated

---

## Snapshots

### Optimizing Large Aggregates

For aggregates with many events (>100), use snapshots:

```go
type SnapshotService struct {
    store         eventsourcing.EventStore
    snapshotStore eventsourcing.SnapshotStore
}

func (s *SnapshotService) LoadAggregate(ctx context.Context, aggregateID string) (*Order, error) {
    // Try to load snapshot
    snapshot, err := s.snapshotStore.LoadSnapshot(ctx, aggregateID)

    var order *Order
    var fromVersion int64

    if err == nil {
        // Load from snapshot
        order = &Order{}
        order.ID = snapshot.AggregateID
        // ... restore state from snapshot
        fromVersion = snapshot.Version
    } else {
        order = &Order{}
        fromVersion = 0
    }

    // Load events since snapshot
    events, err := s.store.LoadEvents(ctx, aggregateID, eventsourcing.LoadEventsOptions{
        FromVersion: fromVersion,
    })
    if err != nil {
        return nil, err
    }

    // Apply remaining events
    for _, event := range events {
        if err := order.Apply(event); err != nil {
            return nil, err
        }
    }

    // Save new snapshot if needed (every 50 events)
    if order.Version % 50 == 0 {
        s.saveSnapshot(ctx, order)
    }

    return order, nil
}

func (s *SnapshotService) saveSnapshot(ctx context.Context, order *Order) error {
    snapshot := &eventsourcing.Snapshot{
        AggregateID:   order.ID,
        AggregateType: "Order",
        Version:       order.Version,
        Data: map[string]interface{}{
            "id":          order.ID,
            "customer_id": order.CustomerID,
            "status":      order.Status,
            "total":       order.Total,
            // ... other fields
        },
        Timestamp: time.Now(),
    }

    return s.snapshotStore.SaveSnapshot(ctx, snapshot)
}
```

---

## Unique Constraints

### Overview

The framework provides built-in support for **unique constraints** to enforce business invariants like unique emails, account numbers, or usernames. Constraints are managed **transactionally** with events, ensuring consistency in distributed systems.

### How It Works

1. **Constraint Table** - A separate `unique_constraints` table tracks claimed values
2. **Atomic Validation** - Constraints are checked and claimed atomically when saving events
3. **Event-Attached** - Constraints are attached to events during emission
4. **Automatic Release** - Constraints are released when needed (e.g., account deletion)

### Defining Constraints

Constraints are specified when applying events in your command handlers:

```go
func (h *AccountHandlers) HandleOpenAccount(
    ctx context.Context,
    agg *accountv1.AccountAggregate,
    cmd *accountv1.OpenAccountCommand,
) ([]proto.Message, error) {
    // Business validation
    if cmd.AccountId == "" {
        return nil, errors.New("account ID is required")
    }

    // Emit event with unique constraint
    event := &accountv1.AccountOpenedEvent{
        AccountId:      cmd.AccountId,
        OwnerName:      cmd.OwnerName,
        InitialBalance: cmd.InitialBalance,
        Timestamp:      time.Now().Unix(),
    }

    // Apply event with unique constraint on account_id
    if err := agg.ApplyAccountOpenedEvent(event,
        accountv1.WithUniqueConstraints(domain.UniqueConstraint{
            IndexName: "account_id",         // Name of the constraint
            Value:     cmd.AccountId,         // Value to claim
            Operation: domain.ConstraintClaim, // Claim operation
        }),
    ); err != nil {
        return nil, err
    }

    return []proto.Message{event}, nil
}
```

### Constraint Operations

Two operations are supported:

```go
// ConstraintClaim - Claims a unique value
domain.UniqueConstraint{
    IndexName: "email",
    Value:     "user@example.com",
    Operation: domain.ConstraintClaim,
}

// ConstraintRelease - Releases a previously claimed value
domain.UniqueConstraint{
    IndexName: "email",
    Value:     "user@example.com",
    Operation: domain.ConstraintRelease,
}
```

### Multiple Constraints

You can claim multiple constraints in a single event:

```go
agg.ApplyAccountOpenedEvent(event,
    accountv1.WithUniqueConstraints(
        domain.UniqueConstraint{
            IndexName: "account_id",
            Value:     cmd.AccountId,
            Operation: domain.ConstraintClaim,
        },
        domain.UniqueConstraint{
            IndexName: "email",
            Value:     cmd.Email,
            Operation: domain.ConstraintClaim,
        },
    ),
)
```

### Error Handling

When a constraint is violated, the framework returns a detailed error:

```go
if err := h.repo.Save(agg); err != nil {
    var constraintErr *domain.UniqueConstraintError
    if errors.As(err, &constraintErr) {
        // Handle constraint violation
        // constraintErr.IndexName - which constraint was violated
        // constraintErr.Value - what value was already claimed
        // constraintErr.OwnerID - who owns it
        return nil, fmt.Errorf(
            "Email '%s' is already registered to account %s",
            constraintErr.Value,
            constraintErr.OwnerID,
        )
    }
    return nil, err
}
```

### Checking Before Claiming

You can check if a value is available before attempting to claim it:

```go
available, ownerID, err := store.CheckUniqueness("email", "user@example.com")
if err != nil {
    return err
}

if !available {
    return fmt.Errorf("Email is already registered to account %s", ownerID)
}
```

### Releasing Constraints

Release constraints when they're no longer needed:

```go
func (h *AccountHandlers) HandleCloseAccount(
    ctx context.Context,
    agg *accountv1.AccountAggregate,
    cmd *accountv1.CloseAccountCommand,
) ([]proto.Message, error) {
    // Emit event that releases constraints
    event := &accountv1.AccountClosedEvent{
        AccountId:    cmd.AccountId,
        FinalBalance: agg.Balance,
        Timestamp:    time.Now().Unix(),
    }

    // Release the account_id constraint
    if err := agg.ApplyAccountClosedEvent(event,
        accountv1.WithUniqueConstraints(domain.UniqueConstraint{
            IndexName: "account_id",
            Value:     cmd.AccountId,
            Operation: domain.ConstraintRelease, // Release the constraint
        }),
    ); err != nil {
        return nil, err
    }

    return []proto.Message{event}, nil
}
```

### Rebuilding Constraints

If the constraint index becomes corrupted, you can rebuild it from the event stream:

```go
if err := store.RebuildConstraints(); err != nil {
    log.Fatal("Failed to rebuild constraints:", err)
}
```

This replays all events and re-applies their constraints.

### Best Practices

1. **Claim Early** - Claim constraints in creation events (e.g., AccountOpened)
2. **Release on Delete** - Release constraints when entities are deleted or deactivated
3. **Descriptive Names** - Use clear index names (e.g., "email", "username", "account_number")
4. **Validate First** - Check availability before attempting operations if you want friendly errors
5. **Handle Errors** - Always check for `UniqueConstraintError` and provide helpful messages

### Common Patterns

**User Registration with Unique Email**:
```go
event := &UserRegisteredEvent{
    UserId: userId,
    Email:  email,
}

agg.ApplyUserRegisteredEvent(event,
    WithUniqueConstraints(domain.UniqueConstraint{
        IndexName: "user_email",
        Value:     email,
        Operation: domain.ConstraintClaim,
    }),
)
```

**Account Number Assignment**:
```go
event := &AccountCreatedEvent{
    AccountId:     accountId,
    AccountNumber: accountNumber,
}

agg.ApplyAccountCreatedEvent(event,
    WithUniqueConstraints(domain.UniqueConstraint{
        IndexName: "account_number",
        Value:     accountNumber,
        Operation: domain.ConstraintClaim,
    }),
)
```

**Updating Email (Release + Claim)**:
```go
// Release old email
oldEmailEvent := &EmailChangedEvent{...}
agg.ApplyEmailChangedEvent(oldEmailEvent,
    WithUniqueConstraints(domain.UniqueConstraint{
        IndexName: "user_email",
        Value:     oldEmail,
        Operation: domain.ConstraintRelease,
    }),
)

// Claim new email
newEmailEvent := &EmailChangedEvent{...}
agg.ApplyEmailChangedEvent(newEmailEvent,
    WithUniqueConstraints(domain.UniqueConstraint{
        IndexName: "user_email",
        Value:     newEmail,
        Operation: domain.ConstraintClaim,
    }),
)
```

---

## Multi-Tenancy

### Tenant Isolation

```go
// Add tenant isolation middleware
commandBus := cqrs.NewCommandBus(
    middleware.TenantIsolationMiddleware(),
    // ... other middleware
)

// Include tenant_id in all commands
cmd := &eventsourcing.CommandEnvelope{
    Metadata: eventsourcing.CommandMetadata{
        CommandID:   uuid.New().String(),
        PrincipalID: "user@example.com",
        Custom: map[string]string{
            "command_type": "PlaceOrder",
            "tenant_id":    "tenant-123",  // REQUIRED
        },
    },
    Command: placeOrderCommand,
}

// Query by tenant
events, err := store.LoadEventsByTenant(ctx, tenantID, aggregateID, options)
```

### Tenant-Specific Projections

```go
// Create tenant-specific tables
CREATE TABLE tenant_orders (
    tenant_id TEXT NOT NULL,
    order_id TEXT NOT NULL,
    customer_id TEXT NOT NULL,
    status TEXT NOT NULL,
    total REAL NOT NULL,
    PRIMARY KEY (tenant_id, order_id)
);

CREATE INDEX idx_tenant_customer ON tenant_orders(tenant_id, customer_id);
```

---

## Testing Strategies

### Unit Testing Aggregates

```go
func TestOrder_Ship(t *testing.T) {
    t.Run("can ship placed order", func(t *testing.T) {
        // Arrange
        order := &Order{
            ID:      "order-123",
            Status:  OrderStatusPlaced,
            Version: 1,
        }

        // Act
        err := order.Ship("TRACK-123")

        // Assert
        assert.NoError(t, err)
        changes := order.GetUncommittedChanges()
        assert.Len(t, changes, 1)
        assert.Equal(t, "OrderShipped", changes[0].EventType)
    })

    t.Run("cannot ship draft order", func(t *testing.T) {
        order := &Order{Status: OrderStatusDraft}
        err := order.Ship("TRACK-123")
        assert.Error(t, err)
    })
}
```

### Testing Command Handlers

```go
func TestPlaceOrderHandler(t *testing.T) {
    // Setup
    store, _ := sqlite.NewEventStore(sqlite.WithMemoryDatabase())
    defer store.Close()

    handler := &PlaceOrderHandler{
        store:  store,
        logger: slog.Default(),
    }

    t.Run("places valid order", func(t *testing.T) {
        cmd := &eventsourcing.CommandEnvelope{
            Command: &PlaceOrderCommand{
                OrderID:    uuid.New().String(),
                CustomerID: "customer-123",
                Items: []OrderItem{
                    {ProductID: "product-1", Quantity: 2, Price: 10.0},
                },
            },
        }

        events, err := handler.Handle(context.Background(), cmd)

        assert.NoError(t, err)
        assert.Len(t, events, 1)
        assert.Equal(t, "OrderPlaced", events[0].EventType)
    })
}
```

### Integration Testing

```go
func TestOrderWorkflow(t *testing.T) {
    // Setup full system
    store, _ := sqlite.NewEventStore(sqlite.WithMemoryDatabase())
    defer store.Close()

    commandBus := cqrs.NewCommandBus(
        middleware.LoggingMiddleware(slog.Default()),
    )

    placeHandler := &PlaceOrderHandler{store: store}
    shipHandler := &ShipOrderHandler{store: store}

    commandBus.Register("PlaceOrder", placeHandler)
    commandBus.Register("ShipOrder", shipHandler)

    ctx := context.Background()
    orderID := uuid.New().String()

    // Test: Place order
    placeCmd := &eventsourcing.CommandEnvelope{
        Metadata: eventsourcing.CommandMetadata{
            CommandID:   uuid.New().String(),
            PrincipalID: "user@example.com",
            Custom:      map[string]string{"command_type": "PlaceOrder"},
        },
        Command: &PlaceOrderCommand{
            OrderID:    orderID,
            CustomerID: "customer-123",
            Items: []OrderItem{
                {ProductID: "product-1", Quantity: 1, Price: 10.0},
            },
        },
    }

    result, err := commandBus.Dispatch(ctx, placeCmd)
    assert.NoError(t, err)
    assert.Len(t, result.Events, 1)

    // Test: Ship order
    shipCmd := &eventsourcing.CommandEnvelope{
        Metadata: eventsourcing.CommandMetadata{
            CommandID:   uuid.New().String(),
            PrincipalID: "user@example.com",
            Custom:      map[string]string{"command_type": "ShipOrder"},
        },
        Command: &ShipOrderCommand{
            OrderID:        orderID,
            TrackingNumber: "TRACK-123",
        },
    }

    result, err = commandBus.Dispatch(ctx, shipCmd)
    assert.NoError(t, err)
    assert.Len(t, result.Events, 1)
    assert.Equal(t, "OrderShipped", result.Events[0].EventType)
}
```

### Testing Projections

```go
func TestOrderProjection(t *testing.T) {
    db, _ := setupTestDB()
    defer db.Close()

    handler := &OrderProjectionHandler{db: db}

    // Test: OrderPlaced event
    event := &eventsourcing.Event{
        AggregateID:   "order-123",
        AggregateType: "Order",
        EventType:     "OrderPlaced",
        Data: map[string]interface{}{
            "customer_id": "customer-123",
            "total":       100.0,
            "placed_at":   time.Now(),
        },
    }

    err := handler.HandleOrderPlaced(context.Background(), event)
    assert.NoError(t, err)

    // Verify projection
    var count int
    db.QueryRow("SELECT COUNT(*) FROM order_summary WHERE order_id = ?", "order-123").Scan(&count)
    assert.Equal(t, 1, count)
}
```

---

## Credential Management

### Overview

The framework provides **enterprise-grade credential management** using the [Go Cloud Development Kit](https://gocloud.dev/howto/secrets/). Never hardcode credentials - always use credential providers.

**Key Features:**
- ✅ Vendor-agnostic (AWS, GCP, Azure, HashiCorp Vault)
- ✅ Automatic rotation
- ✅ Built-in caching with TTL
- ✅ Thread-safe
- ✅ Multiple authentication types (token, user/password, mTLS, NKey, JWT)

### Quick Start

```go
import (
    "github.com/plaenen/eventstore/pkg/security/credentials"
    _ "gocloud.dev/secrets/awskms" // AWS Secrets Manager
)

ctx := context.Background()

// Production: AWS Secrets Manager
provider, err := credentials.NewSecretProvider(ctx,
    "awskms://arn:aws:secretsmanager:us-east-1:123456:secret:nats-creds")
if err != nil {
    log.Fatal(err)
}
defer provider.Close()

// Use with NATS transport
transport, err := natstransport.NewTransport(&natstransport.TransportConfig{
    URL:                "nats://production:4222",
    CredentialProvider: provider, // ✅ Secure
})
```

### Provider Types

#### 1. SecretProvider (Production - Recommended)

For cloud-based secret management with automatic rotation:

```go
// AWS Secrets Manager
provider, err := credentials.NewSecretProvider(ctx,
    "awskms://arn:aws:secretsmanager:us-east-1:123456:secret:nats-creds")

// GCP Secret Manager
provider, err := credentials.NewSecretProvider(ctx,
    "gcpkms://projects/my-project/secrets/nats-creds/versions/latest")

// Azure Key Vault
provider, err := credentials.NewSecretProvider(ctx,
    "azurekeyvault://my-vault.vault.azure.net/secrets/nats-creds")

// HashiCorp Vault
provider, err := credentials.NewSecretProvider(ctx,
    "hashivault://my-vault.example.com:8200/secret/data/nats-creds")
```

**Benefits:**
- Automatic credential rotation
- Centralized secret management
- Audit logging
- Access control via IAM/RBAC

#### 2. EnvProvider (CI/CD & Kubernetes)

For environment variable-based credentials:

```go
// Token from environment
provider := credentials.NewEnvTokenProvider("NATS_TOKEN", 5*time.Minute)

// Username/Password from environment
provider := credentials.NewEnvUserPasswordProvider("NATS_USER", "NATS_PASS")
```

Use with Kubernetes secrets:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: nats-credentials
type: Opaque
stringData:
  NATS_TOKEN: "your-secret-token"
---
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
      - name: app
        env:
        - name: NATS_TOKEN
          valueFrom:
            secretKeyRef:
              name: nats-credentials
              key: NATS_TOKEN
```

#### 3. ChainProvider (Flexible Fallback)

Try multiple providers in order:

```go
provider := credentials.NewChainProvider(
    // Try cloud secret manager first
    credentials.NewSecretProvider(ctx, "awskms://..."),
    // Fall back to environment variables
    credentials.NewEnvTokenProvider("NATS_TOKEN", 5*time.Minute),
    // Last resort: local file (dev only)
    credentials.NewStaticTokenProvider("dev-token", 24*time.Hour),
)
defer provider.Close()
```

#### 4. StaticProvider (Development ONLY)

⚠️ **WARNING: Never use in production!**

```go
// Development only
provider := credentials.NewStaticTokenProvider("dev-token", 24*time.Hour)
```

### Credential Types

The framework supports multiple authentication mechanisms:

```go
// Token authentication
tokenCreds := &credentials.Credentials{
    Type:  credentials.CredentialTypeToken,
    Token: "your-secret-token",
}

// Username/Password
userPassCreds := &credentials.Credentials{
    Type:     credentials.CredentialTypeUserPassword,
    User:     "admin",
    Password: "secure-password",
}

// NKey (NATS)
nkeyCreds := &credentials.Credentials{
    Type:      credentials.CredentialTypeNKey,
    PublicKey: "UABC...",
    Seed:      "SUABC...",
}

// JWT
jwtCreds := &credentials.Credentials{
    Type:     credentials.CredentialTypeJWT,
    JWTToken: "eyJhbGciOiJIUzI1NiIs...",
}

// mTLS
mtlsCreds := &credentials.Credentials{
    Type:    credentials.CredentialTypeMTLS,
    CertPEM: "-----BEGIN CERTIFICATE-----\n...",
    KeyPEM:  "-----BEGIN PRIVATE KEY-----\n...",
}
```

### Automatic Rotation

Credentials automatically rotate based on TTL:

```go
config := credentials.ProviderConfig{
    URL:             "awskms://...",
    CacheTTL:        5 * time.Minute,     // Cache for 5 minutes
    AutoRefresh:     true,                // Enable auto-refresh
    RefreshInterval: 2.5 * time.Minute,   // Refresh at 50% of TTL
}

provider, err := credentials.NewSecretProviderWithConfig(ctx, config.URL, config)
defer provider.Close()

// Manual rotation (if needed)
if err := provider.Rotate(ctx); err != nil {
    log.Printf("Rotation failed: %v", err)
}
```

### Complete Example

```go
package main

import (
    "context"
    "log"
    "time"

    "github.com/plaenen/eventstore/pkg/cqrs/nats"
    "github.com/plaenen/eventstore/pkg/security/credentials"
    "github.com/plaenen/eventstore/pkg/security/tls"
    _ "gocloud.dev/secrets/awskms"
)

func main() {
    ctx := context.Background()

    // Load credentials from AWS Secrets Manager
    credProvider, err := credentials.NewSecretProvider(ctx,
        "awskms://arn:aws:secretsmanager:us-east-1:123456:secret:prod/nats-creds")
    if err != nil {
        log.Fatalf("Failed to create credential provider: %v", err)
    }
    defer credProvider.Close()

    // Configure TLS
    tlsConfig := tls.ProductionConfig(
        tls.WithCertFile("/etc/certs/cert.pem"),
        tls.WithKeyFile("/etc/certs/key.pem"),
        tls.WithCAFile("/etc/certs/ca.pem"),
    )

    // Create transport with secure credentials
    transport, err := nats.NewTransport(&nats.TransportConfig{
        URL:                "nats://production.example.com:4222",
        CredentialProvider: credProvider,
        TLSConfig:          tlsConfig,
        Name:               "my-service",
    })
    if err != nil {
        log.Fatalf("Failed to create transport: %v", err)
    }
    defer transport.Close()

    log.Printf("Connected to NATS: %s", transport.ConnectedURL())

    // Credentials will automatically rotate based on TTL
    select {}
}
```

### Best Practices

**✅ DO:**
1. Use cloud secret managers in production
2. Enable automatic rotation
3. Set appropriate cache TTLs (5 minutes recommended)
4. Always call `defer provider.Close()`
5. Validate credentials before use
6. Use different credentials per environment

**❌ DON'T:**
1. Never hardcode credentials in code
2. Never commit secrets to version control
3. Never use StaticProvider in production
4. Never log sensitive credentials
5. Never share credentials between environments
6. Never set long cache TTLs (> 10 minutes)

For complete documentation, see [pkg/security/README.md](pkg/security/README.md)

---

## Configuration Management

### Overview

The framework provides **dynamic configuration management** using the [Go Cloud Development Kit's runtimevar](https://gocloud.dev/howto/runtimevar/). Update configuration in real-time without application restarts.

**Key Features:**
- ✅ Vendor-agnostic (AWS, GCP, Azure, etcd)
- ✅ Real-time updates (hot-reload)
- ✅ Type-safe with generics
- ✅ Built-in validation
- ✅ Watch for changes
- ✅ Automatic decoding (JSON, YAML)

### Quick Start

```go
import (
    "github.com/plaenen/eventstore/pkg/config"
    _ "gocloud.dev/runtimevar/awsparamstore"
)

// Define your configuration type
type AppConfig struct {
    MaxConnections int           `json:"max_connections"`
    Timeout        time.Duration `json:"timeout"`
    EnableFeatureX bool          `json:"enable_feature_x"`
}

ctx := context.Background()

// Production: AWS Parameter Store
provider, err := config.NewProvider[AppConfig](ctx,
    "awsparamstore:///prod/myapp/config?decoder=json")
if err != nil {
    log.Fatal(err)
}
defer provider.Close()

// Get configuration
cfg, err := provider.Get(ctx)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Max connections: %d\n", cfg.MaxConnections)

// Watch for changes (hot-reload)
stop, err := provider.Watch(ctx, func(cfg AppConfig) {
    log.Printf("Configuration updated: %+v", cfg)
    // Update application settings here
})
defer stop()
```

### Provider Types

#### 1. RuntimeVarProvider (Production - Recommended)

For cloud-based configuration with real-time updates:

```go
// AWS Parameter Store
provider, err := config.NewProvider[AppConfig](ctx,
    "awsparamstore:///prod/myapp/config?decoder=json")

// GCP Runtime Configurator
provider, err := config.NewProvider[AppConfig](ctx,
    "gcpruntimeconfig://projects/my-project/configs/app/variables/config?decoder=json")

// Azure App Configuration
provider, err := config.NewProvider[AppConfig](ctx,
    "azureappconfig://connection-string?key=app-config&decoder=json")

// etcd
provider, err := config.NewProvider[AppConfig](ctx,
    "etcd://localhost:2379/app-config?decoder=json")

// Local file (development)
provider, err := config.NewProvider[AppConfig](ctx,
    "file:///etc/myapp/config.json?decoder=json")
```

#### 2. EnvProvider (Kubernetes & Containers)

For environment variable-based configuration:

```go
// JSON from environment variable
provider := config.NewEnvJSONProvider[AppConfig]("APP_CONFIG", 5*time.Minute)
```

#### 3. ChainProvider (Flexible Fallback)

Try multiple sources in order:

```go
provider := config.NewChainProvider(
    // Try cloud config first
    config.NewProvider[AppConfig](ctx, "awsparamstore://..."),
    // Fall back to environment variables
    config.NewEnvJSONProvider[AppConfig]("APP_CONFIG", 5*time.Minute),
    // Last resort: static defaults
    config.NewStaticProvider(defaultConfig),
)
defer provider.Close()
```

#### 4. StaticProvider (Development & Testing)

For static configuration:

```go
cfg := AppConfig{
    MaxConnections: 10,
    Timeout:        5 * time.Second,
}

provider := config.NewStaticProvider(cfg)
defer provider.Close()

// For testing: manually update
provider.Update(newConfig)
```

### Pre-Built Configuration Types

The package provides ready-to-use configuration types:

#### Feature Flags

```go
flags := config.FeatureFlags{
    EnableNewUI:       true,
    EnableDebugMode:   false,
    EnableMetrics:     true,
    Features: map[string]bool{
        "dark_mode":    true,
        "beta_feature": false,
    },
}

provider := config.NewProvider[config.FeatureFlags](ctx, configURL)

// Check feature
if flags.IsEnabled("dark_mode") {
    // Enable dark mode
}
```

#### Service Endpoints

```go
endpoints := config.ServiceEndpoints{
    NATSURL:     "nats://localhost:4222",
    DatabaseURL: "postgres://localhost:5432/db",
    CacheURL:    "redis://localhost:6379",
    Endpoints: map[string]string{
        "auth_service": "http://auth:8080",
    },
}

provider := config.NewProvider[config.ServiceEndpoints](ctx, configURL)
```

#### Runtime Tuning

```go
tuning := config.RuntimeTuning{
    MaxConcurrency:    10,
    EventBatchSize:    100,
    ProjectionWorkers: 5,
    CacheTTL:          5 * time.Minute,
    RequestTimeout:    10 * time.Second,
}

provider := config.NewProvider[config.RuntimeTuning](ctx, configURL)
```

#### Comprehensive App Configuration

```go
appConfig := config.AppConfig{
    Environment: "production",
    ServiceName: "my-service",
    Version:     "1.0.0",

    FeatureFlags: config.FeatureFlags{
        EnableMetrics: true,
    },

    Endpoints: config.ServiceEndpoints{
        NATSURL:     "nats://nats:4222",
        DatabaseURL: "postgres://db:5432/prod",
    },

    Tuning: config.RuntimeTuning{
        MaxConcurrency:    20,
        EventBatchSize:    200,
        ProjectionWorkers: 10,
    },

    Logging: config.LoggingConfig{
        Level:  "info",
        Format: "json",
    },
}

// Validate entire config
if err := appConfig.Validate(); err != nil {
    log.Fatal(err)
}
```

### Hot-Reload Example

```go
package main

import (
    "context"
    "log"

    "github.com/plaenen/eventstore/pkg/config"
)

type EventProcessor struct {
    batchSize int
    workers   int
}

func (ep *EventProcessor) UpdateConfig(tuning config.RuntimeTuning) {
    ep.batchSize = tuning.EventBatchSize
    ep.workers = tuning.ProjectionWorkers
    log.Printf("Updated: batch=%d, workers=%d", ep.batchSize, ep.workers)
}

func main() {
    ctx := context.Background()

    provider, _ := config.NewProvider[config.RuntimeTuning](ctx,
        "awsparamstore:///prod/tuning?decoder=json")
    defer provider.Close()

    processor := &EventProcessor{}

    // Load initial config
    tuning, _ := provider.Get(ctx)
    processor.UpdateConfig(tuning)

    // Watch for tuning updates (zero-downtime updates!)
    provider.Watch(ctx, func(tuning config.RuntimeTuning) {
        processor.UpdateConfig(tuning)
    })

    // Process events with current tuning
    processor.ProcessEvents()
}
```

### Integration with Runner

The runner package supports automatic configuration updates:

```go
// MyService implements runner.ConfigurableService
type MyService struct {
    config config.RuntimeTuning
}

func (s *MyService) UpdateConfig(ctx context.Context, cfg interface{}) error {
    tuning := cfg.(config.RuntimeTuning)
    log.Printf("Updating configuration: %+v", tuning)
    s.config = tuning
    return nil
}

func main() {
    ctx := context.Background()

    // Create config provider
    configProvider, _ := config.NewProvider[config.RuntimeTuning](ctx,
        "awsparamstore:///prod/tuning?decoder=json")
    defer configProvider.Close()

    // Create runner with config provider
    r := runner.New(services,
        runner.WithConfigProvider(configProvider), // ✅ Auto-update on changes
    )

    // When configuration changes, all ConfigurableService instances
    // automatically receive UpdateConfig callbacks
    r.Run(ctx)
}
```

### Best Practices

**✅ DO:**
1. Use cloud providers in production
2. Implement the `Validator` interface on config types
3. Watch for configuration changes
4. Use `Latest()` for frequently accessed config (cached)
5. Set appropriate poll intervals (30s-1m recommended)
6. Handle configuration errors gracefully

**❌ DON'T:**
1. Never store secrets in configuration (use credential store)
2. Never ignore validation errors
3. Never use `Get()` in hot paths (use `Latest()` instead)
4. Never create providers in loops
5. Never block on Watch callbacks
6. Never use long poll intervals (> 5 minutes)

For complete documentation, see [pkg/config/README.md](pkg/config/README.md)

---

## Security Best Practices

### 1. Always Use Security Middleware

```go
// Production configuration
commandBus := cqrs.NewCommandBus(
    // Error handling
    middleware.ProductionErrorSanitizationMiddleware(logger),
    middleware.RecoveryMiddleware(logger),

    // Input validation
    middleware.StrictValidationMiddleware(),

    // Authorization
    middleware.AuthorizationMiddleware(authzService),
    middleware.TenantIsolationMiddleware(),
)
```

### 2. Validate All Inputs

```go
import "github.com/plaenen/eventstore/pkg/validation"

func (h *Handler) Handle(ctx context.Context, cmd *CommandEnvelope) ([]*Event, error) {
    // Validate UUIDs
    if err := validation.ValidateUUIDv4(cmd.AggregateID); err != nil {
        return nil, fmt.Errorf("%w: invalid aggregate ID", eventsourcing.ErrInvalidCommand)
    }

    // Validate emails
    if err := validation.ValidateEmail(cmd.Email); err != nil {
        return nil, fmt.Errorf("%w: invalid email", eventsourcing.ErrInvalidCommand)
    }

    // Validate lengths
    if err := validation.ValidateStringLength(cmd.Name, "name", 1, 256); err != nil {
        return nil, fmt.Errorf("%w: %v", eventsourcing.ErrInvalidCommand, err)
    }

    // ... handler logic
}
```

### 3. Use Credential Providers

```go
// NEVER hardcode credentials
provider, err := credentials.NewEnvTokenProvider("NATS_TOKEN")

transport, err := natstransport.NewTransport(
    natstransport.WithCredentialProvider(provider),
    natstransport.WithTLS(tlsConfig),
)
```

### 4. Enable TLS

```go
tlsConfig := tls.ProductionConfig(
    tls.WithCertFile("/path/to/cert.pem"),
    tls.WithKeyFile("/path/to/key.pem"),
    tls.WithCAFile("/path/to/ca.pem"),
)
```

### 5. Sanitize Errors

```go
sanitizer := security.NewErrorSanitizer(security.ErrorModeProduction)

if err != nil {
    logger.Error("Operation failed", slog.Any("error", err))  // Log server-side
    return sanitizer.SanitizeError(err)                        // Return to client
}
```

**For detailed security documentation, see:**
- [SEC-001: Credentials Management](docs/security/CREDENTIALS.md)
- [SEC-002: TLS/Encryption](docs/security/TLS.md)
- [SEC-003: SQL Injection](docs/security/SQL_INJECTION.md)
- [SEC-004: Error Handling](docs/security/ERROR_HANDLING.md)
- [SEC-005: Input Validation](docs/security/INPUT_VALIDATION.md)

---

## Performance Optimization

### 1. Connection Pooling

```go
store, err := sqlite.NewEventStore(
    sqlite.WithFilename("events.db"),
    sqlite.WithMaxOpenConns(50),     // Increase for high load
    sqlite.WithMaxIdleConns(10),     // Keep connections ready
    sqlite.WithWALMode(true),        // Better concurrency
)
```

### 2. Use Snapshots

```go
// Save snapshot every 50 events
if aggregate.Version % 50 == 0 {
    snapshotStore.SaveSnapshot(ctx, aggregate.ToSnapshot())
}

// Load from snapshot + events
snapshot, _ := snapshotStore.LoadSnapshot(ctx, aggregateID)
events, _ := store.LoadEvents(ctx, aggregateID,
    eventsourcing.LoadEventsOptions{FromVersion: snapshot.Version})
```

### 3. Optimize Projections

```go
// Use indexes
CREATE INDEX idx_customer ON orders(customer_id);
CREATE INDEX idx_status ON orders(status);
CREATE INDEX idx_date ON orders(created_at DESC);

// Batch updates
projectionBuilder.WithBatchSize(100)
```

### 4. Cache Read Models

```go
type CachedQueryService struct {
    cache redis.Client
    db    *sql.DB
}

func (s *CachedQueryService) GetOrder(ctx context.Context, orderID string) (*Order, error) {
    // Check cache first
    if cached, err := s.cache.Get(ctx, "order:"+orderID); err == nil {
        return parseOrder(cached), nil
    }

    // Query database
    order, err := s.queryDatabase(ctx, orderID)
    if err != nil {
        return nil, err
    }

    // Cache result
    s.cache.Set(ctx, "order:"+orderID, order, 5*time.Minute)

    return order, nil
}
```

---

## Configuration Guide

### Development

```go
store, _ := sqlite.NewEventStore(
    sqlite.WithMemoryDatabase(),  // In-memory for tests
)

commandBus := cqrs.NewCommandBus(
    middleware.DevelopmentErrorSanitizationMiddleware(logger),
    middleware.DevModeRecoveryMiddleware(logger),
    middleware.DevModeValidationMiddleware(),
)
```

### Staging

```go
store, _ := sqlite.NewEventStore(
    sqlite.WithFilename("/var/lib/staging/events.db"),
    sqlite.WithWALMode(true),
    sqlite.WithMaxOpenConns(25),
)

commandBus := cqrs.NewCommandBus(
    middleware.ErrorSanitizationMiddleware(logger, security.ErrorModeProduction),
    middleware.RecoveryMiddleware(logger),
    middleware.EnhancedValidationMiddleware(),  // Standard validation
    middleware.LoggingMiddleware(logger),
)
```

### Production

```go
store, _ := sqlite.NewEventStore(
    sqlite.WithFilename("/var/lib/eventstore/events.db"),
    sqlite.WithWALMode(true),
    sqlite.WithMaxOpenConns(50),
    sqlite.WithMaxIdleConns(10),
)

commandBus := cqrs.NewCommandBus(
    middleware.ProductionErrorSanitizationMiddleware(logger),
    middleware.RecoveryMiddleware(logger),
    middleware.StrictValidationMiddleware(),  // Strict validation
    middleware.AuthorizationMiddleware(authzService),
    middleware.TenantIsolationMiddleware(),
    middleware.LoggingMiddleware(logger),
    middleware.MetricsMiddleware(metrics),
    middleware.TracingMiddleware(tracer),
)

tlsConfig := tls.ProductionConfig(
    tls.WithCertFile("/etc/certs/cert.pem"),
    tls.WithKeyFile("/etc/certs/key.pem"),
    tls.WithCAFile("/etc/certs/ca.pem"),
)

provider, _ := credentials.NewSecretProvider(
    credentials.WithURL("nats://nats.example.com:4222"),
    credentials.WithTokenAuth("my-service"),
    credentials.WithLocalStorage("/var/lib/secrets"),
)

transport, _ := natstransport.NewTransport(
    natstransport.WithCredentialProvider(provider),
    natstransport.WithTLS(tlsConfig),
)
```

---

## Monitoring & Observability

### Logging

```go
logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelInfo,
}))

commandBus := cqrs.NewCommandBus(
    middleware.LoggingMiddleware(logger),
)
```

### Metrics

```go
import "github.com/prometheus/client_golang/prometheus"

commandDuration := prometheus.NewHistogramVec(
    prometheus.HistogramOpts{
        Name: "command_duration_seconds",
        Help: "Command processing duration",
    },
    []string{"command_type", "status"},
)

eventCount := prometheus.NewCounterVec(
    prometheus.CounterOpts{
        Name: "events_total",
        Help: "Total number of events",
    },
    []string{"event_type", "aggregate_type"},
)
```

### Tracing

```go
import "go.opentelemetry.io/otel"

commandBus := cqrs.NewCommandBus(
    middleware.TracingMiddleware(tracer),
)
```

### Health Checks

```go
func healthCheck(w http.ResponseWriter, r *http.Request) {
    // Check event store
    if err := store.Ping(r.Context()); err != nil {
        w.WriteHeader(http.StatusServiceUnavailable)
        json.NewEncoder(w).Encode(map[string]string{
            "status": "unhealthy",
            "error":  "event store unavailable",
        })
        return
    }

    // Check NATS
    if !transport.IsConnected() {
        w.WriteHeader(http.StatusServiceUnavailable)
        json.NewEncoder(w).Encode(map[string]string{
            "status": "unhealthy",
            "error":  "message bus unavailable",
        })
        return
    }

    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]string{
        "status": "healthy",
    })
}
```

---

## Deployment

### Docker

```dockerfile
FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o /app/server ./cmd/server

FROM alpine:latest

RUN apk --no-cache add ca-certificates
WORKDIR /root/

COPY --from=builder /app/server .
COPY --from=builder /app/migrations ./migrations

EXPOSE 8080

CMD ["./server"]
```

---

## Common Pitfalls

### ❌ Pitfall 1: Not Using WAL Mode

```go
// WRONG: Poor concurrency
store, _ := sqlite.NewEventStore(
    sqlite.WithFilename("events.db"),
)
```

**Solution**:
```go
// CORRECT: Enable WAL mode
store, _ := sqlite.NewEventStore(
    sqlite.WithFilename("events.db"),
    sqlite.WithWALMode(true),  // ✅
)
```

### ❌ Pitfall 2: Large Aggregates Without Snapshots

```go
// WRONG: Loading 1000+ events every time
events, _ := store.LoadEvents(ctx, aggregateID, options)
```

**Solution**:
```go
// CORRECT: Use snapshots
snapshot, _ := snapshotStore.LoadSnapshot(ctx, aggregateID)
events, _ := store.LoadEvents(ctx, aggregateID,
    eventsourcing.LoadEventsOptions{FromVersion: snapshot.Version})
```

### ❌ Pitfall 3: Missing Security Middleware

```go
// WRONG: No security
commandBus := cqrs.NewCommandBus(
    middleware.LoggingMiddleware(logger),
)
```

**Solution**:
```go
// CORRECT: Full security stack
commandBus := cqrs.NewCommandBus(
    middleware.ProductionErrorSanitizationMiddleware(logger),
    middleware.RecoveryMiddleware(logger),
    middleware.StrictValidationMiddleware(),
    middleware.AuthorizationMiddleware(authz),
    middleware.LoggingMiddleware(logger),
)
```

### ❌ Pitfall 4: Mutating Event Data

```go
// WRONG: Events are immutable!
event.Data["amount"] = 200.0  // ❌ Don't modify
```

**Solution**:
```go
// CORRECT: Create new event
newEvent := &eventsourcing.Event{
    AggregateID:   event.AggregateID,
    EventType:     "AmountCorrected",
    Data: map[string]interface{}{
        "original_amount": event.Data["amount"],
        "corrected_amount": 200.0,
    },
}
```

### ❌ Pitfall 5: Querying Event Store for Reads

```go
// WRONG: Inefficient reads
events, _ := store.LoadEvents(ctx, orderID, options)
order, _ := LoadOrderFromHistory(events)
return order.Total  // Slow!
```

**Solution**:
```go
// CORRECT: Use projections
order, _ := queryService.GetOrder(ctx, orderID)  // Fast!
return order.Total
```

---

## API Reference

### Core Types

```go
// Event represents a domain event
type Event struct {
    AggregateID   string
    AggregateType string
    EventType     string
    Data          map[string]interface{}
    Version       int64
    Timestamp     time.Time
}

// CommandEnvelope wraps a command with metadata
type CommandEnvelope struct {
    Metadata CommandMetadata
    Command  interface{}
}

// CommandMetadata contains command context
type CommandMetadata struct {
    CommandID   string
    PrincipalID string
    Custom      map[string]string
}
```

### Event Store

```go
// EventStore interface
type EventStore interface {
    SaveEvents(ctx context.Context, aggregateID string, events []*Event, expectedVersion int64) error
    LoadEvents(ctx context.Context, aggregateID string, options LoadEventsOptions) ([]*Event, error)
    LoadEventsByTenant(ctx context.Context, tenantID, aggregateID string, options LoadEventsOptions) ([]*Event, error)
}

// Usage
store, err := sqlite.NewEventStore(
    sqlite.WithFilename("events.db"),
    sqlite.WithWALMode(true),
)
```

### Command Bus

```go
// CommandBus routes commands to handlers
type CommandBus struct {
    // ...
}

// Register a handler
commandBus.Register("CreateOrder", handler)

// Dispatch a command
result, err := commandBus.Dispatch(ctx, commandEnvelope)
```

### Middleware

```go
// CommandMiddleware is applied to all commands
type CommandMiddleware func(next CommandHandler) CommandHandler

// Common middleware
middleware.ProductionErrorSanitizationMiddleware(logger)
middleware.RecoveryMiddleware(logger)
middleware.StrictValidationMiddleware()
middleware.AuthorizationMiddleware(authz)
middleware.LoggingMiddleware(logger)
middleware.MetricsMiddleware(metrics)
middleware.TracingMiddleware(tracer)
```

---

## Production Checklist

### Application Setup

- [ ] Event store configured with WAL mode
- [ ] Connection pool sized appropriately
- [ ] Command bus configured with all middleware
- [ ] All command handlers registered
- [ ] Projections defined and built
- [ ] Query services implemented
- [ ] Health checks implemented

### Security

- [ ] Error sanitization middleware enabled
- [ ] Recovery middleware enabled
- [ ] Input validation middleware enabled (strict mode)
- [ ] Authorization middleware configured
- [ ] Tenant isolation enabled (if multi-tenant)
- [ ] TLS enabled for all connections
- [ ] Credentials managed via providers (no hardcoding)
- [ ] SQL injection prevention verified

### Performance

- [ ] Snapshots implemented for large aggregates
- [ ] Projections indexed appropriately
- [ ] Connection pooling configured
- [ ] Caching strategy implemented for read models
- [ ] Batch processing for high-throughput operations

### Observability

- [ ] Structured logging configured
- [ ] Metrics collection enabled
- [ ] Distributed tracing enabled
- [ ] Health endpoints exposed
- [ ] Alerting configured

### Testing

- [ ] Unit tests for aggregates (>80% coverage)
- [ ] Integration tests for workflows
- [ ] Load tests performed
- [ ] Security tests (input validation, SQL injection)
- [ ] Chaos testing for resilience

### Operations

- [ ] Backup strategy implemented
- [ ] Disaster recovery plan documented
- [ ] Deployment automation configured
- [ ] Monitoring dashboards created
- [ ] Runbooks for common operations
- [ ] Incident response procedures documented

---

## Documentation Links

### Framework Guides

- [Projections Guide](docs/guides/projections.md)
- [Event Upcasting](docs/guides/event-upcasting.md)
- [SDK Generation](docs/guides/sdk-generation.md)

### Security Documentation

- [SEC-001: Credentials Management (Completed)](docs/security/SEC-001-COMPLETED.md)
- [SQL Injection Prevention](docs/security/SQL_INJECTION_PREVENTION.md)
- [Error Handling & Information Disclosure](docs/security/ERROR_HANDLING.md)
- [Input Validation](docs/security/INPUT_VALIDATION.md)
- [Security Roadmap](docs/SECURITY_ROADMAP.md)

### Package Documentation

- [CQRS Package](pkg/cqrs/README.md)
- [Security Package](pkg/security/README.md)
- [TLS Configuration](pkg/security/tls/README.md)
- [Encryption](pkg/security/encryption/README.md)
- [Configuration](pkg/config/README.md)
- [NATS Infrastructure](pkg/infrastructure/nats/README.md)
- [Messaging](pkg/messaging/README.md)
- [Runtime](pkg/runtime/README.md)

### Release Notes

- [Release Notes Index](docs/releases/README.md)
- [Version 0.0.6](docs/releases/v0.0.6.md)

### External Resources

- [Martin Fowler - Event Sourcing](https://martinfowler.com/eaaDev/EventSourcing.html)
- [CQRS Journey (Microsoft)](https://docs.microsoft.com/en-us/previous-versions/msp-n-p/jj554200(v=pandp.10))
- [Event Sourcing Patterns](https://eventstore.com/event-sourcing)
- [Protocol Buffers - Field Evolution](https://protobuf.dev/programming-guides/proto3/#updating)

---

## Version History

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2025-10-26 | Initial comprehensive implementation guide |

---

## For AI Coding Agents

When implementing event-sourced applications with this framework:

### The Proto-First Approach

**This framework uses Protocol Buffers as the primary way to define domain models**:

1. **Define in Proto** - All aggregates, events, and commands are defined in `.proto` files
2. **Generate Code** - Run `protoc` with the eventsourcing plugin to generate boilerplate
3. **Implement Business Logic** - Write event appliers and command handlers in Go

**What Gets Generated**:
- `*_aggregate.es.pb.go` - Aggregate implementations with state management
- `*_handler.es.pb.go` - Command handler interfaces
- `*_server.es.pb.go` - Complete gRPC server implementations
- `*_client.es.pb.go` - Client SDKs
- `*_sdk.es.pb.go` - Projection SDKs

**What You Write**:
- Proto definitions (schema)
- Event appliers (how events modify state)
- Command handlers (business rules and validation)

### Implementation Steps

1. **Start Simple** - Begin with the Quick Start example using proto definitions
2. **Model Your Domain in Proto** - Define aggregates, events, and commands in `.proto` files
3. **Generate Code** - Run `protoc --go_out=. --eventsourcing_out=. proto/**/*.proto`
4. **Implement Appliers** - Write how events modify aggregate state
5. **Implement Handlers** - Write business logic and validation
6. **Configure Credentials** - Use credential providers (never hardcode secrets)
7. **Configure Settings** - Use config providers for dynamic configuration
8. **Build Projections** - Create read models optimized for queries
9. **Add Security** - Always include security middleware
10. **Test Thoroughly** - Unit, integration, and security tests
11. **Monitor** - Add logging, metrics, and tracing
12. **Optimize** - Use snapshots, caching, and indexing as needed

### Key Benefits of Proto-Based Approach

1. **Type Safety** - Strong typing with proto messages
2. **Code Generation** - Automatic boilerplate generation
3. **Versioning** - Built-in proto field evolution for event upcasting
4. **Cross-Language** - Can consume events from other languages
5. **Serialization** - Efficient binary serialization
6. **Documentation** - Proto files serve as living documentation
7. **Tooling** - Proto ecosystem (validation, linting, breaking change detection)

### Essential: Credential & Configuration Management

**ALWAYS use the framework's credential and config stores - never hardcode values!**

#### Credential Management (pkg/security/credentials)

For **all sensitive data** (passwords, tokens, API keys, certificates):

```go
import (
    "github.com/plaenen/eventstore/pkg/security/credentials"
    _ "gocloud.dev/secrets/awskms" // Cloud provider
)

// Production: Cloud secret manager
credProvider, _ := credentials.NewSecretProvider(ctx,
    "awskms://arn:aws:secretsmanager:us-east-1:123:secret:nats-creds")
defer credProvider.Close()

// Development: Environment variables
credProvider := credentials.NewEnvTokenProvider("NATS_TOKEN", 5*time.Minute)

// Use with transport
transport, _ := nats.NewTransport(&nats.TransportConfig{
    URL:                "nats://prod:4222",
    CredentialProvider: credProvider, // ✅ Secure
})
```

**Supported backends:**
- AWS Secrets Manager
- GCP Secret Manager
- Azure Key Vault
- HashiCorp Vault
- Environment variables (Kubernetes secrets)

**Key features:**
- Automatic credential rotation
- Built-in caching with TTL
- Multiple auth types (token, user/pass, mTLS, NKey, JWT)

#### Configuration Management (pkg/config)

For **all application settings** (feature flags, endpoints, tuning parameters):

```go
import (
    "github.com/plaenen/eventstore/pkg/config"
    _ "gocloud.dev/runtimevar/awsparamstore"
)

// Define your config type
type AppConfig struct {
    MaxConnections int           `json:"max_connections"`
    Timeout        time.Duration `json:"timeout"`
    EnableFeatureX bool          `json:"enable_feature_x"`
}

// Production: AWS Parameter Store
configProvider, _ := config.NewProvider[AppConfig](ctx,
    "awsparamstore:///prod/config?decoder=json")
defer configProvider.Close()

// Get configuration
cfg, _ := configProvider.Get(ctx)

// Watch for changes (hot-reload!)
configProvider.Watch(ctx, func(cfg AppConfig) {
    log.Printf("Config updated: %+v", cfg)
    updateApplicationSettings(cfg)
})
```

**Supported backends:**
- AWS Parameter Store
- GCP Runtime Configurator
- Azure App Configuration
- etcd
- Environment variables (Kubernetes ConfigMaps)
- Local files (development)

**Key features:**
- Real-time configuration updates (zero-downtime)
- Type-safe with generics
- Built-in validation
- Pre-built config types (FeatureFlags, ServiceEndpoints, RuntimeTuning)

**Pre-built configuration types:**

```go
// Feature flags
flags := config.FeatureFlags{
    EnableMetrics: true,
    Features: map[string]bool{"dark_mode": true},
}

// Service endpoints
endpoints := config.ServiceEndpoints{
    NATSURL:     "nats://nats:4222",
    DatabaseURL: "postgres://db:5432/prod",
}

// Runtime tuning
tuning := config.RuntimeTuning{
    MaxConcurrency:    20,
    EventBatchSize:    200,
    ProjectionWorkers: 10,
}

// Comprehensive app config
appConfig := config.AppConfig{
    Environment:  "production",
    FeatureFlags: flags,
    Endpoints:    endpoints,
    Tuning:       tuning,
}
```

#### When to Use What

| Use Case | Store | Example |
|----------|-------|---------|
| Database passwords | Credential Store | `credentials.NewSecretProvider(...)` |
| API keys | Credential Store | `credentials.NewEnvTokenProvider(...)` |
| TLS certificates | Credential Store | `credentials.CredentialTypeMTLS` |
| Feature flags | Config Store | `config.FeatureFlags` |
| Service URLs | Config Store | `config.ServiceEndpoints` |
| Performance tuning | Config Store | `config.RuntimeTuning` |
| Logging levels | Config Store | `config.LoggingConfig` |

**Critical Rules:**
- ✅ **DO** use credential providers for all secrets
- ✅ **DO** use config providers for all settings
- ✅ **DO** enable automatic rotation/hot-reload
- ✅ **DO** use cloud backends in production
- ❌ **DON'T** hardcode credentials or config
- ❌ **DON'T** commit secrets to version control
- ❌ **DON'T** store secrets in configuration (separate stores!)
- ❌ **DON'T** use static providers in production

For complete documentation:
- [Credential Management](#credential-management)
- [Configuration Management](#configuration-management)
- [pkg/security/README.md](pkg/security/README.md)
- [pkg/config/README.md](pkg/config/README.md)

---

**Remember**: Event sourcing is a powerful pattern but adds complexity. Use it when you need:
- Complete audit trails
- Time-travel capabilities
- Event-driven architectures
- Complex business workflows

For simple CRUD applications, traditional approaches may be more appropriate.

---

**For Questions or Issues**: Refer to the documentation links above or review the source code examples in the repository.
