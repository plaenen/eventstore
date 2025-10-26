# Code Generation Examples

This directory contains examples of using different generation modes with Buf.

## Quick Reference

```bash
# Generate everything (default)
buf generate

# Generate ONLY aggregates (domain layer + projections)
buf generate --template buf.gen.aggregate.yaml
```

## What's the Difference?

### Default: `buf generate`

Generates **all** files:

```
pb/account/v1/
├── account.pb.go                    # Proto messages
├── account_aggregate.es.pb.go       # ✅ Aggregates + projections
├── account_client.es.pb.go          # ✅ Client implementations
├── account_sdk.es.pb.go             # ✅ SDK wrappers
├── account_handler.es.pb.go         # ✅ Handler interfaces
└── account_server.es.pb.go          # ✅ Server routing
```

### Aggregate Only: `buf generate --template buf.gen.aggregate.yaml`

Generates **only** domain layer:

```
pb/account/v1/
├── account.pb.go                    # Proto messages
└── account_aggregate.es.pb.go       # ✅ ONLY THIS FILE
                                     #    - Aggregates
                                     #    - Event appliers
                                     #    - Repository
                                     #    - Projection SDK
```

## When to Use Aggregate-Only Mode

Use `buf.gen.aggregate.yaml` when:

✅ You're building **projections** (read models)
✅ You only need **domain logic**, no transport layer
✅ You want **faster builds** (skip client/server generation)
✅ You're implementing **CQRS read side**
✅ Your proto file has **no service definition**

## Proto Requirements

### For Aggregate-Only Generation

You **only** need:
- Aggregate root with `aggregate_root` option
- Events with `event` option
- **NO service definition needed!**

Example:

```protobuf
syntax = "proto3";
package account.v1;
import "eventsourcing/options.proto";

// Aggregate root
message Account {
  option (eventsourcing.aggregate_root) = {
    id_field: "account_id"
  };
  string account_id = 1;
  string balance = 2;
}

// Events
message AccountOpenedEvent {
  option (eventsourcing.event) = {
    aggregate_name: "Account"
  };
  string account_id = 1;
  string initial_balance = 2;
}

// That's it! No service needed for aggregate-only mode.
```

### For Full Generation

You need:
- Aggregate root
- Events
- **Service definition** (for client/server code)

Example:

```protobuf
// (Same as above, plus...)

service AccountCommandService {
  option (eventsourcing.service) = {
    aggregate_name: "Account"
    aggregate_root_message: "Account"
  };

  rpc OpenAccount(OpenAccountCommand) returns (OpenAccountResponse);
}
```

## Example Workflow

### Scenario: Building a Projection

You want to build a read model that subscribes to events:

```bash
# Step 1: Generate only the aggregate file
cd examples
buf generate --template buf.gen.aggregate.yaml

# Step 2: Implement your projection
```

```go
package main

import (
    accountv1 "github.com/plaenen/eventstore/examples/pb/account/v1"
    "github.com/plaenen/eventstore/pkg/domain"
)

func main() {
    // Use the generated projection builder
    projection := accountv1.NewAccountProjectionBuilder("account-balance").
        OnAccountOpened(func(ctx context.Context, e *accountv1.AccountOpenedEvent, envelope *domain.EventEnvelope) error {
            // Insert into read model
            return insertAccount(ctx, e.AccountId, e.InitialBalance)
        }).
        OnMoneyDeposited(func(ctx context.Context, e *accountv1.MoneyDepositedEvent, envelope *domain.EventEnvelope) error {
            // Update read model
            return updateBalance(ctx, e.AccountId, e.NewBalance)
        }).
        Build()

    // Register with projection manager
    manager.Register(projection)
}
```

## Troubleshooting

### "No files generated"

Make sure your proto has the required options:
```protobuf
message Account {
  option (eventsourcing.aggregate_root) = {
    id_field: "account_id"
  };
  // ...
}

message AccountOpenedEvent {
  option (eventsourcing.event) = {
    aggregate_name: "Account"
  };
  // ...
}
```

### "Plugin not found"

Build the plugin first:
```bash
cd ..
go build -o .task/bin/protoc-gen-eventsourcing ./cmd/protoc-gen-eventsourcing
```

### Old files still exist

The aggregate-only mode **doesn't delete** old client/server files. It just doesn't regenerate them.

To clean up:
```bash
rm pb/account/v1/*_{client,sdk,handler,server}.es.pb.go
```

## See Also

- [../BUF_GENERATION.md](../BUF_GENERATION.md) - Complete Buf generation guide
- [../cmd/protoc-gen-eventsourcing/README.md](../cmd/protoc-gen-eventsourcing/README.md) - Plugin documentation
- [../AGENTS.md](../AGENTS.md) - Full framework documentation
