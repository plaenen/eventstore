# SDK Generation Guide

The event sourcing framework uses a **2-phase approach** to generate a unified SDK that combines all services:

## Overview

```
┌─────────────────┐
│  Proto Files    │
│  (account.proto,│
│   user.proto,   │
│   etc.)         │
└────────┬────────┘
         │
         │ buf generate
         ▼
┌─────────────────┐
│  Phase 1:       │
│  Per-Service    │
│  SDKs Generated │
│                 │
│ - AccountSDK    │
│ - UserSDK       │
│ - DocumentSDK   │
└────────┬────────┘
         │
         │ generate-unified-sdk
         ▼
┌─────────────────┐
│  Phase 2:       │
│  Unified SDK    │
│                 │
│  sdk.NewSDK()   │
│  ├─ Account     │
│  ├─ User        │
│  └─ Document    │
└─────────────────┘
```

## Phase 1: Per-Service SDK Generation

The `protoc-gen-eventsourcing` plugin generates SDKs for each service during `buf generate`:

### Input
```protobuf
// proto/account/v1/account.proto
service AccountCommandService {
  rpc OpenAccount(OpenAccountCommand) returns (OpenAccountResponse);
  rpc Deposit(DepositCommand) returns (DepositResponse);
}

service AccountQueryService {
  rpc GetAccount(GetAccountRequest) returns (AccountView);
}
```

### Output
Generated files in `pb/account/v1/`:
- `account.pb.go` - Protobuf messages
- `account_aggregate.pb.go` - Aggregate root
- `account_client.pb.go` - Low-level client
- **`account_sdk.pb.go`** - Service-specific SDK ✨
- `account_server.pb.go` - Server handlers
- `account_handler.pb.go` - Handler interfaces

### Generated SDK Structure

```go
// account_sdk.pb.go
type AccountSDK struct {
    client *AccountClient
}

func NewAccountSDK(transport eventsourcing.Transport) *AccountSDK

// Commands
func (s *AccountSDK) OpenAccount(ctx, cmd) (*OpenAccountResponse, *AppError)
func (s *AccountSDK) Deposit(ctx, cmd) (*DepositResponse, *AppError)

// Queries
func (s *AccountSDK) GetAccount(ctx, query) (*AccountView, *AppError)
```

## Phase 2: Unified SDK Generation

The `generate-unified-sdk` tool scans all generated `*_sdk.pb.go` files and creates a top-level SDK:

### How It Works

1. **Discovery**: Scans `examples/pb/` for all `*_sdk.pb.go` files
2. **Parsing**: Extracts SDK types and package information using Go AST
3. **Generation**: Creates a unified SDK that wraps all service SDKs

### Generated Unified SDK

```go
// sdk/unified.go
package sdk

import (
    accountv1 "github.com/plaenen/eventstore/examples/pb/account/v1"
    userv1 "github.com/plaenen/eventstore/examples/pb/user/v1"
    documentv1 "github.com/plaenen/eventstore/examples/pb/document/v1"
    "github.com/plaenen/eventstore/pkg/eventsourcing"
)

type SDK struct {
    Account  *accountv1.AccountSDK
    User     *userv1.UserSDK
    Document *documentv1.DocumentSDK

    transport eventsourcing.Transport
}

func NewSDK(transport eventsourcing.Transport) *SDK {
    return &SDK{
        Account:  accountv1.NewAccountSDK(transport),
        User:     userv1.NewUserSDK(transport),
        Document: documentv1.NewDocumentSDK(transport),
        transport: transport,
    }
}

func (s *SDK) Close() error {
    return s.transport.Close()
}
```

## Usage

### Automated via Taskfile

```bash
# Generate everything (proto + unified SDK)
task generate:proto

# Or just regenerate the unified SDK
task generate:sdk:unified
```

### Manual Steps

```bash
# Step 1: Generate proto files and per-service SDKs
cd examples
buf generate

# Step 2: Generate unified SDK
./bin/generate-unified-sdk ./examples/pb ./examples/sdk/unified.go
```

### Using the Generated SDK

```go
package main

import (
    "github.com/plaenen/eventstore/examples/sdk"
    "github.com/plaenen/eventstore/pkg/nats"
)

func main() {
    // Create transport once
    transport, _ := nats.NewTransport(&nats.TransportConfig{
        URL: "nats://localhost:4222",
    })
    defer transport.Close()

    // Create unified SDK - all services in one!
    s := sdk.NewSDK(transport)
    defer s.Close()

    // Use any service through the unified SDK
    s.Account.OpenAccount(ctx, accountCmd)
    s.User.CreateUser(ctx, userCmd)
    s.Document.Upload(ctx, docCmd)
}
```

## Adding New Services

When you add a new service:

1. **Create the proto file**
   ```bash
   # proto/order/v1/order.proto
   ```

2. **Run code generation**
   ```bash
   task generate:proto
   ```

3. **The unified SDK is automatically updated!**
   ```go
   sdk.Order.PlaceOrder(ctx, cmd)  // ✨ Automatically available
   ```

## Directory Structure

```
eventsourcing/
├── cmd/
│   ├── protoc-gen-eventsourcing/    # Phase 1: Per-service SDK generator
│   │   └── main.go
│   └── generate-unified-sdk/         # Phase 2: Unified SDK generator
│       └── main.go
├── examples/
│   ├── proto/                        # Proto definitions
│   │   ├── account/v1/
│   │   ├── user/v1/
│   │   └── document/v1/
│   ├── pb/                           # Generated code (Phase 1)
│   │   ├── account/v1/
│   │   │   ├── account_sdk.pb.go    # Per-service SDK
│   │   │   └── ...
│   │   ├── user/v1/
│   │   │   ├── user_sdk.pb.go       # Per-service SDK
│   │   │   └── ...
│   │   └── document/v1/
│   │       ├── document_sdk.pb.go   # Per-service SDK
│   │       └── ...
│   └── sdk/                          # Generated code (Phase 2)
│       └── unified.go                # Unified SDK combining all services
└── bin/
    ├── protoc-gen-eventsourcing      # Built plugin
    └── generate-unified-sdk          # Built tool
```

## Why 2-Phase?

### Problem
Protoc processes files in batches. It can't see all services at once to generate a single unified SDK.

### Solution
- **Phase 1 (buf generate)**: Generate individual service SDKs
- **Phase 2 (generate-unified-sdk)**: Combine them into one unified SDK

### Benefits
1. ✅ Each service can be developed independently
2. ✅ Services can be in different packages/versions
3. ✅ Unified SDK automatically discovers all services
4. ✅ No manual registration or configuration needed
5. ✅ Single command regenerates everything

## Troubleshooting

### "No services found"
The `generate-unified-sdk` tool looks for `*_sdk.pb.go` files. Make sure you ran `buf generate` first.

```bash
task generate:proto:buf
```

### "Package not found"
Make sure the generated code is in your module path:

```bash
go mod tidy
```

### Rebuilding Generators

```bash
# Rebuild both generators
task build

# Or individually
task build:plugin
task build:sdk-generator
```

## Advanced: Custom SDK Generation

You can customize the unified SDK template in `cmd/generate-unified-sdk/main.go`:

```go
const unifiedSDKTemplate = `// Your custom template here
package {{.PackageName}}
// ...
`
```

Then rebuild:
```bash
task build:sdk-generator
task generate:sdk:unified
```

## CI/CD Integration

Add to your CI pipeline:

```yaml
- name: Generate Code
  run: |
    task build
    task generate:proto
```

The unified SDK will be automatically generated and can be committed to version control.
