# Event Store Protocol Buffer Definitions

This directory contains the protocol buffer definitions for the Event Store plugins.

## Published Module

**Module Name**: `buf.build/plaenen/eventstore`

This module provides proto options for code generation with:
- **protoc-gen-eventsourcing** - Event sourcing plugin
- **protoc-gen-cqrs** - CQRS infrastructure plugin

## Files

- `eventsourcing/options.proto` - Options for event sourcing plugin
- `eventsourcing/response.proto` - Shared response types
- `cqrs/options.proto` - Options for CQRS plugin

## Publishing to Buf Schema Registry

### Prerequisites

1. **Install buf CLI**:
   ```bash
   go install github.com/bufbuild/buf/cmd/buf@latest
   ```

2. **Authenticate with BSR**:
   ```bash
   buf registry login
   ```
   This will prompt you to create an API token at https://buf.build/settings/user

### Publish the Module

From the repository root:

```bash
cd proto
buf push
```

This will:
- Validate all proto files
- Run linting checks
- Check for breaking changes
- Push to `buf.build/plaenen/eventstore`

### Versioning

Buf automatically versions pushes using git commits. To create a labeled version:

```bash
buf push --tag v1.0.0
```

## Using the Published Options

### In Your Proto Files

Add the dependency to your `buf.yaml`:

```yaml
version: v2
deps:
  - buf.build/plaenen/eventstore
```

Then run:
```bash
buf dep update
```

### Import in Proto Files

#### Event Sourcing Options

```protobuf
syntax = "proto3";

package myapp.v1;

import "eventsourcing/options.proto";

// Define an aggregate
message Account {
  option (eventsourcing.aggregate_root) = {
    id_field: "account_id"
  };

  string account_id = 1;
  string balance = 2;
}

// Define an event
message AccountOpenedEvent {
  option (eventsourcing.event) = {
    aggregate_name: "Account"
  };

  string account_id = 1;
  string owner_name = 2;
}

// Define a service with aggregate handler
service AccountService {
  option (eventsourcing.service) = {
    aggregate_name: "Account"
    aggregate_root_message: "Account"
    aggregate_handler: true
  };

  rpc OpenAccount(OpenAccountCommand) returns (OpenAccountResponse);
}
```

#### CQRS Options

```protobuf
syntax = "proto3";

package myapp.v1;

import "cqrs/options.proto";

// Define a command service
service AccountCommandService {
  option (cqrs.service) = {
    service_type: SERVICE_TYPE_COMMAND
    generate_client: true
    timeout_ms: 5000
    max_retries: 3
    queue_group: "account-handlers"
  };

  rpc OpenAccount(OpenAccountCommand) returns (OpenAccountResponse);
  rpc Deposit(DepositCommand) returns (DepositResponse);
}

// Define a query service
service AccountQueryService {
  option (cqrs.service) = {
    service_type: SERVICE_TYPE_QUERY
    generate_client: true
  };

  rpc GetAccount(GetAccountRequest) returns (AccountView);
  rpc ListAccounts(ListAccountsRequest) returns (ListAccountsResponse);
}
```

### Code Generation

Configure your `buf.gen.yaml`:

```yaml
version: v2
plugins:
  # Standard protobuf
  - remote: buf.build/protocolbuffers/go
    out: gen
    opt:
      - paths=source_relative

  # Event sourcing plugin
  - local: protoc-gen-eventsourcing
    out: gen
    opt:
      - paths=source_relative

  # CQRS plugin
  - local: protoc-gen-cqrs
    out: gen
    opt:
      - paths=source_relative
```

Then generate:
```bash
buf generate
```

## Design Philosophy

### Event Sourcing Options
- **Aggregate-centric**: Options focus on domain model (aggregates, events)
- **Generates**: Domain types, repositories, projection builders, handlers
- **Use case**: Building event-sourced domain models

### CQRS Options
- **Infrastructure-focused**: Options for transport and routing
- **Generates**: Clients, servers, handlers (no domain logic)
- **Independence**: No dependency on event sourcing - pure CQRS
- **Use case**: Building command/query services over any message bus

### Separation of Concerns

The plugins are **independent**:
- **Event Sourcing**: Domain modeling and persistence
- **CQRS**: Infrastructure and communication

You can use:
- ✅ Event sourcing alone (without CQRS)
- ✅ CQRS alone (without event sourcing)
- ✅ Both together (recommended for full DDD/CQRS/ES)

## Examples

See the [examples directory](../examples/) for complete working examples:
- `examples/proto/account/v1/account.proto` - Full example using both plugins
- `examples/cmd/bankaccount-observability/` - Complete application

## Support

- **Issues**: https://github.com/plaenen/eventstore/issues
- **Documentation**: https://github.com/plaenen/eventstore/blob/main/README.md
- **Buf Schema Registry**: https://buf.build/plaenen/eventstore
