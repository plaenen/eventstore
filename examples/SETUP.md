# Examples Setup Guide

## Quick Start with Task

This project uses [Task](https://taskfile.dev) for build automation.

### Install Task

```bash
# macOS
brew install go-task

# Linux/WSL
sh -c "$(curl --location https://taskfile.dev/install.sh)" -- -d -b /usr/local/bin

# Windows (with Scoop)
scoop install task

# Or use Go
go install github.com/go-task/task/v3/cmd/task@latest
```

### Common Commands

```bash
# Show all available tasks
task

# Complete setup for new developers (install deps, generate code, run tests)
task dev:setup

# Generate proto code
task generate

# Run tests
task test

# Run all checks before committing (format, lint, test)
task dev:check

# Clean and rebuild everything
task clean && task generate
```

## Directory Structure

```
examples/
├── proto/                    # Proto definitions for all examples
│   └── account.proto
├── pb/                       # Generated protobuf code (auto-generated)
│   ├── account.pb.go
│   ├── accountconnect/
│   └── account_aggregate.pb.go
├── bankaccount/              # Bank account example
│   ├── domain/
│   ├── projections/
│   ├── handlers/
│   └── integration_test.go
├── buf.yaml                  # Buf configuration
└── buf.gen.yaml              # Code generation configuration
```

## Manual Setup (without Task)

### Prerequisites

1. Install dependencies:
```bash
# Install buf CLI
go install github.com/bufbuild/buf/cmd/buf@latest

# Install golangci-lint
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Install protoc-gen-eventsourcing plugin
go install ./cmd/protoc-gen-eventsourcing
```

### Generate Code

From the `examples/` directory:

```bash
cd examples
buf generate
```

This will:
1. Generate standard protobuf Go code → `pb/account.pb.go`
2. Generate Connect RPC services → `pb/accountconnect/account.connect.go`
3. Generate event sourcing boilerplate → `pb/account_aggregate.pb.go`

## Running Tests

### With Task (Recommended)

```bash
# Run all tests
task test

# Run unit tests only
task test:unit

# Run integration tests
task test:integration

# Run with coverage report
task test:coverage
```

### Manual

```bash
# All tests
go test -v -race ./...

# Example tests only
cd examples/bankaccount
go test ./...

# Integration tests
go test -v ./integration_test.go

# Projection tests
go test -v ./projections/...

# Domain tests
go test -v ./domain/...
```

## Code Quality

### With Task

```bash
# Format code
task fmt

# Lint code
task lint

# Run all checks (format + lint + test)
task dev:check
```

### Manual

```bash
# Format Go code
gofmt -w -s .

# Format proto files
cd examples && buf format -w

# Lint Go code
golangci-lint run ./...

# Lint proto files
cd examples && buf lint
```

## Clean Up

### With Task

```bash
# Clean all generated files
task clean

# Clean old example files in wrong locations
task example:clean
```

### Manual

```bash
# Remove old proto directory in bankaccount
rm -rf examples/bankaccount/proto/

# Remove wrongly placed generated files
rm -f examples/bankaccount/account.pb.go
rm -f examples/bankaccount/*.connect.go
```

## Key Configuration Files

### buf.yaml

Defines the proto module structure:
- Module path: `proto/`
- Linting: STANDARD rules
- Breaking change detection: FILE rules

### buf.gen.yaml

Defines code generation:
- **protoc-gen-go**: Standard protobuf messages → `pb/`
- **protoc-gen-connect-go**: Connect RPC services → `pb/`
- **protoc-gen-eventsourcing**: Event sourcing boilerplate → `pb/`
  - Uses `go run` to avoid binary installation issues
  - Generates aggregates, repositories, and event handlers

## Import Pattern

All examples import the shared `pb` package:

```go
import (
    pb "github.com/plaenen/eventsourcing/examples/pb"
)
```

## Adding New Proto Definitions

1. Add `.proto` file to `examples/proto/`
2. Update `option go_package` to `"github.com/plaenen/eventsourcing/examples/pb"`
3. Run `buf generate` from `examples/` directory
4. Import generated code with `pb "github.com/plaenen/eventsourcing/examples/pb"`

## Troubleshooting

### "no required module provides package"

Run from the repository root:
```bash
go mod tidy
```

### "protoc-gen-eventsourcing not found"

The buf.gen.yaml uses `go run` which doesn't require installation. Ensure:
1. You're in the `examples/` directory when running `buf generate`
2. The main module's `go.mod` is accessible

### Generated files have wrong package

Check that `option go_package` in `.proto` files points to:
```protobuf
option go_package = "github.com/plaenen/eventsourcing/examples/pb";
```

## Next Steps

After generating proto code:

1. Implement business logic in domain layer
2. Create projection handlers for read models
3. Implement command handlers
4. Write integration tests
5. Create service implementations for Connect RPC
