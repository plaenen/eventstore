# protoc-gen-eventsourcing

Protocol Buffers plugin for generating event sourcing boilerplate.

## Installation

```bash
go install github.com/plaenen/eventstore/cmd/protoc-gen-eventsourcing@latest
```

## Generation Modes

The plugin supports selective code generation using the `generate` parameter:

### 1. Generate All Files (Default)

Generates all files: aggregates, clients, SDKs, servers, and handlers.

```bash
protoc --go_out=. --eventsourcing_out=. proto/**/*.proto
```

**Generated files:**
- `*_aggregate.es.pb.go` - Aggregates, event appliers, repository, projection SDK
- `*_client.es.pb.go` - Low-level client implementations
- `*_sdk.es.pb.go` - Type-safe SDK clients
- `*_handler.es.pb.go` - Handler interfaces
- `*_server.es.pb.go` - Server-side routing

### 2. Generate Only Aggregates

Generates only the aggregate file with domain logic.

```bash
protoc --go_out=. --eventsourcing_out=. \
  --eventsourcing_opt=generate=aggregate \
  proto/**/*.proto
```

**Generated files:**
- `*_aggregate.es.pb.go` - Includes:
  - Aggregate root structs
  - Event applier interfaces
  - Repository implementation
  - **Projection SDK builders** ✅

**Use case:** When you only need domain logic and projections, without client/server code.

### 3. Generate Only Client Files

Generates only client-side code for consuming services.

```bash
protoc --go_out=. --eventsourcing_out=. \
  --eventsourcing_opt=generate=client \
  proto/**/*.proto
```

**Generated files:**
- `*_client.es.pb.go` - Low-level transport clients
- `*_sdk.es.pb.go` - Type-safe SDK clients

**Use case:** When building a client application that consumes the event sourcing service.

### 4. Generate Only Server Files

Generates only server-side code for implementing services.

```bash
protoc --go_out=. --eventsourcing_out=. \
  --eventsourcing_opt=generate=server \
  proto/**/*.proto
```

**Generated files:**
- `*_handler.es.pb.go` - Handler interfaces to implement
- `*_server.es.pb.go` - Server routing and request handling

**Use case:** When building the server implementation without including client code.

## Examples

### Example 1: Monorepo - Generate Everything

```bash
# In a monorepo where you have both client and server
protoc --go_out=. --eventsourcing_out=. proto/**/*.proto
```

### Example 2: Separate Domain Layer

```bash
# Generate only aggregates for your domain layer
protoc --go_out=. --eventsourcing_out=. \
  --eventsourcing_opt=generate=aggregate \
  proto/account/v1/*.proto
```

### Example 3: Client-Only Package

```bash
# Generate only client code for a client library package
protoc --go_out=. --eventsourcing_out=. \
  --eventsourcing_opt=generate=client \
  proto/**/*.proto
```

### Example 4: Server-Only Package

```bash
# Generate only server code for a server implementation
protoc --go_out=. --eventsourcing_out=. \
  --eventsourcing_opt=generate=server \
  proto/**/*.proto
```

## What's Included in Each Mode

| Component | all | aggregate | client | server |
|-----------|-----|-----------|--------|--------|
| Aggregate structs | ✅ | ✅ | ❌ | ❌ |
| Event appliers | ✅ | ✅ | ❌ | ❌ |
| Repository | ✅ | ✅ | ❌ | ❌ |
| **Projection SDK** | ✅ | ✅ | ❌ | ❌ |
| Client implementation | ✅ | ❌ | ✅ | ❌ |
| SDK wrapper | ✅ | ❌ | ✅ | ❌ |
| Handler interfaces | ✅ | ❌ | ❌ | ✅ |
| Server routing | ✅ | ❌ | ❌ | ✅ |

## Projection SDK

The **Projection SDK** is included in the `aggregate` mode. This means when you use:

```bash
--eventsourcing_opt=generate=aggregate
```

You get:
- Aggregate root implementation
- Event applier interfaces
- Repository
- **Type-safe projection builders** for creating read models

Example projection usage:

```go
// Generated projection builder (included in aggregate mode)
projection := accountv1.NewAccountProjectionBuilder("account-summary").
    OnAccountOpened(func(ctx context.Context, e *accountv1.AccountOpenedEvent, envelope *domain.EventEnvelope) error {
        // Handle account opened
        return insertAccountSummary(ctx, e.AccountId, e.OwnerName, e.InitialBalance)
    }).
    OnMoneyDeposited(func(ctx context.Context, e *accountv1.MoneyDepositedEvent, envelope *domain.EventEnvelope) error {
        // Handle deposit
        return updateAccountBalance(ctx, e.AccountId, e.NewBalance)
    }).
    Build()

// Use with projection manager
manager.Register(projection)
```

## Build the Plugin

```bash
cd cmd/protoc-gen-eventsourcing
go build -o $GOPATH/bin/protoc-gen-eventsourcing
```

## Version

Current version: 0.0.11

## License

See main project LICENSE file.
