# Generate Unified SDK

A code generator that creates a unified SDK combining all service SDKs into a single client with one transport.

## Overview

The `generate-unified-sdk` tool scans your protobuf-generated service SDKs and creates a unified SDK that:
- Combines all services into one client
- Requires only a single transport instance
- Provides type-safe access to all service methods
- Works with any Go module (not tied to this repository)

## Installation

### From Source

```bash
go install github.com/plaenen/eventstore/cmd/generate-unified-sdk@latest
```

### From Repository

```bash
task build:sdk-generator
# Binary will be at ./bin/generate-unified-sdk
```

## Usage

### Basic Usage (Auto-detect Module)

```bash
generate-unified-sdk -pb-dir ./pb -output ./sdk/client.go
```

The tool will automatically detect your Go module path from `go.mod`.

### Full Usage with All Options

```bash
generate-unified-sdk \
  -pb-dir ./pb \
  -output ./sdk/client.go \
  -module github.com/mycompany/myapp \
  -package client \
  -eventsourcing github.com/plaenen/eventstore/pkg/eventsourcing
```

## Flags

### Required Flags

- `-pb-dir` - Directory containing protobuf-generated files with service SDKs
- `-output` - Output file path for the generated unified SDK

### Optional Flags

- `-module` - Go module path (auto-detected from go.mod if not provided)
- `-package` - Package name for generated SDK (default: "sdk")
- `-eventsourcing` - Import path for eventsourcing package (default: "github.com/plaenen/eventstore/pkg/eventsourcing")

## How It Works

1. **Discovery**: Scans the specified directory for `*_sdk.pb.go` files
2. **Parsing**: Extracts SDK types (structs ending with "SDK") using Go AST
3. **Import Path Resolution**: Builds correct import paths using:
   - Auto-detected or provided module path
   - Relative path from module root to pb directory
   - Relative path within pb directory structure
4. **Code Generation**: Creates a unified SDK with all services

## Examples

### Example 1: Using in This Repository

```bash
generate-unified-sdk \
  -pb-dir ./examples/pb \
  -output ./examples/sdk/unified.go
```

Output:
```go
package sdk

import (
    accountv1 "github.com/plaenen/eventstore/examples/pb/account/v1"
    "github.com/plaenen/eventstore/pkg/eventsourcing"
)

type SDK struct {
    Account *accountv1.AccountSDK
    transport eventsourcing.Transport
}
```

### Example 2: Using in External Project

```bash
# In your project: github.com/mycompany/myapp
generate-unified-sdk \
  -pb-dir ./gen/pb \
  -output ./client/unified.go \
  -package client
```

Output:
```go
package client

import (
    accountv1 "github.com/mycompany/myapp/gen/pb/account/v1"
    orderv1 "github.com/mycompany/myapp/gen/pb/order/v1"
    "github.com/plaenen/eventstore/pkg/eventsourcing"
)

type SDK struct {
    Account *accountv1.AccountSDK
    Order *orderv1.OrderSDK
    transport eventsourcing.Transport
}
```

### Example 3: Custom Everything

```bash
generate-unified-sdk \
  -pb-dir ./internal/proto/gen \
  -output ./pkg/apiclient/client.go \
  -module github.com/acme/platform \
  -package apiclient \
  -eventsourcing github.com/plaenen/eventstore/pkg/eventsourcing
```

## Integration with Taskfile

Add to your `Taskfile.yml`:

```yaml
tasks:
  generate:sdk:
    desc: Generate unified SDK
    cmds:
      - generate-unified-sdk -pb-dir ./pb -output ./sdk/unified.go
    sources:
      - pb/**/*_sdk.pb.go
    generates:
      - sdk/unified.go
```

## Generated SDK Usage

```go
package main

import (
    "context"
    "github.com/mycompany/myapp/sdk"
    "github.com/plaenen/eventstore/pkg/nats"
)

func main() {
    // Create transport (only one needed!)
    transport, _ := nats.NewTransport(&nats.TransportConfig{
        URL: "nats://localhost:4222",
    })

    // Create unified SDK
    client := sdk.NewSDK(transport)
    defer client.Close()

    // Use any service
    ctx := context.Background()

    // Account service
    accountResp, _ := client.Account.OpenAccount(ctx, &OpenAccountCommand{
        AccountId: "acc-123",
        OwnerName: "John Doe",
    })

    // Order service (if you have multiple services)
    orderResp, _ := client.Order.CreateOrder(ctx, &CreateOrderCommand{
        OrderId: "ord-456",
        Items: []string{"item1", "item2"},
    })
}
```

## Requirements

- Generated service SDKs must follow the naming convention: `*SDK` (e.g., `AccountSDK`, `OrderSDK`)
- SDK files must be named: `*_sdk.es.pb.go` (eventsourcing-generated files use `.es.pb.go` extension)
- Go module must be initialized (`go.mod` must exist for auto-detection)

## Troubleshooting

### "Failed to auto-detect module path"

**Solution**: Either run from within a Go module directory, or explicitly provide `-module`:

```bash
generate-unified-sdk -pb-dir ./pb -output ./sdk/unified.go -module github.com/mycompany/myapp
```

### "No services found"

**Problem**: The tool looks for files matching `*_sdk.es.pb.go` with types ending in `SDK`.

**Solution**: Ensure your protobuf generation includes the eventsourcing SDK generation:
```yaml
# buf.gen.yaml
plugins:
  - plugin: eventsourcing
    out: pb
    opt: gen_sdk=true
```

### Import paths are wrong

**Solution**: The tool constructs import paths as: `{module}/{pb-dir-from-root}/{file-rel-path}`

Verify:
1. Your module path is correct (check `go.mod` or provide `-module`)
2. The `-pb-dir` path is relative to your module root
3. Run the generator from your module root directory

## Development

### Building

```bash
go build -o generate-unified-sdk ./cmd/generate-unified-sdk
```

### Testing

```bash
# Test with this repository's examples
go run ./cmd/generate-unified-sdk -pb-dir ./examples/pb -output /tmp/test.go

# Test with custom module
go run ./cmd/generate-unified-sdk \
  -pb-dir ./examples/pb \
  -output /tmp/test.go \
  -module github.com/example/test \
  -package testclient
```
