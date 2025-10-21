# Contributing to Event Sourcing Framework

Thank you for your interest in contributing to this Event Sourcing and CQRS framework for Go! This guide will help you get started with development and understand our contribution process.

## Table of Contents

- [Getting Started](#getting-started)
- [Development Setup](#development-setup)
- [Project Structure](#project-structure)
- [Development Workflow](#development-workflow)
- [Code Style and Conventions](#code-style-and-conventions)
- [Testing](#testing)
- [Pull Request Process](#pull-request-process)
- [Code Review Guidelines](#code-review-guidelines)
- [Adding New Features](#adding-new-features)
- [Documentation](#documentation)
- [Community](#community)

## Getting Started

### Prerequisites

Before you begin, ensure you have the following installed:

- **Go 1.25.0 or later** - [Download](https://golang.org/dl/)
- **Task** - Task runner for build automation
  ```bash
  go install github.com/go-task/task/v3/cmd/task@latest
  ```
- **Buf** - Protocol Buffers tooling
  ```bash
  go install github.com/bufbuild/buf/cmd/buf@latest
  ```
- **protoc-gen-go** - Protocol Buffers Go plugin
  ```bash
  go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
  ```
- **golangci-lint** - Go linter (optional but recommended)
  ```bash
  go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
  ```

### Fork and Clone

1. Fork the repository on GitHub
2. Clone your fork locally:
   ```bash
   git clone https://github.com/YOUR_USERNAME/eventstore.git
   cd eventstore
   ```
3. Add the upstream repository:
   ```bash
   git remote add upstream https://github.com/plaenen/eventstore.git
   ```

### Initial Build

Run the full build to ensure everything works:

```bash
# Generate all code (protobuf + SQLC)
task generate

# Run all tests
task test

# Run linter
task lint
```

If all commands succeed, you're ready to contribute!

## Development Setup

### IDE Configuration

**VS Code** (recommended):
- Install the Go extension
- Install the Protobuf extension
- Configure your workspace settings:
  ```json
  {
    "go.useLanguageServer": true,
    "go.lintTool": "golangci-lint",
    "go.lintOnSave": "workspace",
    "protoc": {
      "path": "/usr/local/bin/protoc"
    }
  }
  ```

**GoLand/IntelliJ IDEA**:
- Enable Go modules integration
- Install Protobuf support plugin
- Configure File Watchers for proto files (optional)

### Environment Variables

No special environment variables are required for development. All configuration uses sensible defaults.

## Project Structure

```
eventsourcing/
├── cmd/
│   └── protoc-gen-eventsourcing/  # Code generator plugin
├── examples/
│   └── bankaccount/               # Complete example application
│       ├── domain/                # Business logic (hand-written)
│       └── README.md              # Example documentation
├── gen/                           # Generated code (DO NOT EDIT)
├── pkg/
│   ├── eventsourcing/             # Core framework
│   ├── middleware/                # Built-in middleware
│   ├── nats/                      # NATS JetStream implementation
│   └── sqlite/                    # SQLite event/snapshot store
├── proto/
│   ├── account/v1/                # Example domain protos
│   └── eventsourcing/             # Framework proto options
├── Taskfile.yml                   # Build automation
├── buf.work.yaml                  # Buf workspace config
└── sqlc.yaml                      # SQLC configuration
```

### Key Directories

- **`pkg/eventsourcing/`** - Core interfaces and base implementations. Changes here affect the entire framework.
- **`pkg/middleware/`** - Reusable middleware. Add new middleware here.
- **`cmd/protoc-gen-eventsourcing/`** - Code generator. Modify when adding new proto options or generation features.
- **`examples/bankaccount/`** - Reference implementation. Update when adding framework features.
- **`proto/eventsourcing/`** - Proto options definitions. Changes require regeneration.

## Development Workflow

### Common Tasks

View all available tasks:
```bash
task --list
```

**Development cycle:**
```bash
# 1. Make changes to proto files
vim proto/account/v1/account.proto

# 2. Regenerate code
task generate

# 3. Implement business logic in generated files
vim examples/pb/account/v1/account.go

# 4. Run tests
task test

# 5. Run full check (format, lint, test)
task dev:check
```

**Key tasks:**
- `task generate` - Regenerate all code (protobuf + SQLC)
- `task generate:proto` - Generate only protobuf code
- `task generate:sqlc` - Generate only SQLC code
- `task test` - Run all tests
- `task test:unit` - Run unit tests only
- `task test:integration` - Run integration tests only
- `task lint` - Run golangci-lint
- `task fmt` - Format all Go code
- `task dev:check` - Run format, lint, and test

### Working with Proto Files

**Adding a new command:**

1. Define the command in your proto file:
   ```protobuf
   message CreateOrderCommand {
     option (eventsourcing.aggregate_options) = {
       aggregate: "Order"
       produces_events: "OrderCreatedEvent"
     };
     string order_id = 1;
     string customer_id = 2;
   }
   ```

2. Define the corresponding event:
   ```protobuf
   message OrderCreatedEvent {
     option (eventsourcing.event_options) = {
       aggregate: "Order"
       applies_to_state: ["order_id", "customer_id", "status"]
     };
     string order_id = 1;
     string customer_id = 2;
     string status = 3;
   }
   ```

3. Regenerate code:
   ```bash
   task generate:proto
   ```

4. Implement business logic in the generated aggregate file (e.g., `gen/pb/order/v1/order.go`)

**Important**: Never edit files in `gen/` directly - they will be overwritten. The generator creates safe-to-edit files in your domain package.

### Working with Database Migrations

**Creating a new migration:**

1. Add SQL to appropriate file in `pkg/sqlite/`:
   - `schema.sql` - Core event store schema
   - `snapshots.sql` - Snapshot store schema

2. Regenerate SQLC code:
   ```bash
   task generate:sqlc
   ```

3. Update migration version in code if needed

**Migration guidelines:**
- Always use `IF NOT EXISTS` for schema creation
- Maintain backward compatibility when possible
- Test migrations with existing data
- Document breaking changes

## Code Style and Conventions

### Go Code Style

We follow standard Go conventions with some additions:

**Formatting:**
- Use `gofmt` (or `task fmt`) before committing
- Line length: aim for 100 characters, hard limit 120
- Use tabs for indentation

**Naming:**
- Aggregates: PascalCase singular (e.g., `Account`, `Order`)
- Commands: PascalCase with `Command` suffix (e.g., `OpenAccountCommand`)
- Events: PascalCase with `Event` suffix, past tense (e.g., `AccountOpenedEvent`)
- Interfaces: PascalCase with `-er` suffix (e.g., `EventStore`, `CommandHandler`)

**Error Handling:**
- Always check errors explicitly
- Use `fmt.Errorf` with `%w` for error wrapping
- Return errors early (guard clauses)
- Use custom error types for domain errors

**Example:**
```go
func (a *Account) Withdraw(ctx context.Context, cmd *WithdrawCommand, metadata eventsourcing.EventMetadata) error {
    // Business validation
    if a.Status != AccountStatus_ACCOUNT_STATUS_OPEN {
        return fmt.Errorf("account is not open")
    }

    // Parse amount
    amount := new(big.Float)
    if _, ok := amount.SetString(cmd.Amount); !ok {
        return fmt.Errorf("invalid withdrawal amount: %s", cmd.Amount)
    }

    // Check sufficient balance
    currentBalance := new(big.Float)
    currentBalance.SetString(a.Balance)
    if currentBalance.Cmp(amount) < 0 {
        return fmt.Errorf("insufficient balance: have %s, need %s", a.Balance, cmd.Amount)
    }

    // Emit event
    event := &MoneyWithdrawnEvent{
        AccountId:  cmd.AccountId,
        Amount:     cmd.Amount,
        NewBalance: new(big.Float).Sub(currentBalance, amount).String(),
        Timestamp:  time.Now().Unix(),
    }

    return a.EmitMoneyWithdrawnEvent(event, metadata)
}
```

### Proto File Style

**File organization:**
```protobuf
syntax = "proto3";

package account.v1;

import "eventsourcing/options.proto";

option go_package = "github.com/plaenen/eventstore/gen/pb/account/v1;accountv1";

// 1. Enums
enum AccountStatus { ... }

// 2. Commands
message OpenAccountCommand { ... }

// 3. Events
message AccountOpenedEvent { ... }

// 4. State (aggregate)
message Account { ... }

// 5. Queries (if any)
message GetAccountQuery { ... }
```

**Naming conventions:**
- Commands: imperative mood (e.g., `OpenAccount`, `Withdraw`)
- Events: past tense (e.g., `AccountOpened`, `MoneyWithdrawn`)
- Fields: snake_case (e.g., `account_id`, `owner_name`)
- Enums: SCREAMING_SNAKE_CASE with type prefix (e.g., `ACCOUNT_STATUS_OPEN`)

**Field numbering:**
- 1-15: Most frequently set fields (1 byte to encode)
- 16+: Less frequent fields
- Never reuse field numbers

### Event Sourcing Patterns

**Event design:**
- Events must be immutable and represent facts
- Include all data needed to rebuild state
- Use past tense naming
- Include timestamp and correlation IDs

**Command validation:**
- Validate in command handler BEFORE emitting events
- Check business rules (e.g., sufficient balance)
- Validate data types and formats
- Check aggregate state (e.g., account is open)

**Aggregate patterns:**
- Aggregates should be transaction boundaries
- Keep aggregates small and focused
- Apply events immediately after emission
- Never load other aggregates in command handlers

## Testing

### Test Organization

- **Unit tests**: Test business logic in isolation
  - File: `*_test.go` alongside source files
  - Focus: Domain logic, validation, event emission

- **Integration tests**: Test framework integration
  - File: `*_integration_test.go`
  - Focus: Database, event store, projections

- **Example tests**: End-to-end tests in examples
  - File: `examples/bankaccount/*_test.go`
  - Focus: Full workflow validation

### Writing Tests

**Unit test example:**
```go
func TestAccount_Withdraw(t *testing.T) {
    tests := []struct {
        name        string
        balance     string
        amount      string
        expectError bool
    }{
        {"sufficient balance", "1000.00", "100.00", false},
        {"insufficient balance", "50.00", "100.00", true},
        {"zero amount", "1000.00", "0.00", true},
        {"negative amount", "1000.00", "-10.00", true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            account := NewAccount("test-account")
            account.Balance = tt.balance
            account.Status = AccountStatus_ACCOUNT_STATUS_OPEN

            cmd := &WithdrawCommand{
                AccountId: "test-account",
                Amount:    tt.amount,
            }

            err := account.Withdraw(context.Background(), cmd, eventsourcing.EventMetadata{})

            if tt.expectError && err == nil {
                t.Errorf("expected error, got nil")
            }
            if !tt.expectError && err != nil {
                t.Errorf("unexpected error: %v", err)
            }
        })
    }
}
```

**Integration test example:**
```go
func TestEventStore_SaveAndLoad(t *testing.T) {
    store, err := sqlite.NewEventStore(sqlite.WithDSN(":memory:"))
    if err != nil {
        t.Fatalf("failed to create event store: %v", err)
    }
    defer store.Close()

    // Test event persistence
    events := []*eventsourcing.Event{
        {
            AggregateID:   "test-123",
            AggregateType: "Account",
            EventType:     "AccountOpenedEvent",
            Version:       1,
            Data:          []byte(`{}`),
        },
    }

    err = store.SaveEvents(events)
    if err != nil {
        t.Fatalf("failed to save events: %v", err)
    }

    loaded, err := store.LoadEvents("test-123", 0)
    if err != nil {
        t.Fatalf("failed to load events: %v", err)
    }

    if len(loaded) != 1 {
        t.Errorf("expected 1 event, got %d", len(loaded))
    }
}
```

### Running Tests

```bash
# All tests
task test

# Specific package
go test -v ./pkg/eventsourcing/...

# Specific test
go test -v ./pkg/eventsourcing -run TestEventStore

# With coverage
task test:coverage

# Integration tests only
task test:integration

# Race detector
go test -race ./...
```

### Test Coverage

We aim for:
- Core framework (`pkg/eventsourcing`): **90%+ coverage**
- Middleware: **80%+ coverage**
- Examples: **70%+ coverage**

View coverage report:
```bash
task test:coverage
go tool cover -html=coverage.out
```

## Pull Request Process

### Before Submitting

1. **Create a feature branch:**
   ```bash
   git checkout -b feature/my-new-feature
   ```

2. **Make your changes** following the guidelines above

3. **Run the full check:**
   ```bash
   task dev:check
   ```
   This runs formatting, linting, and all tests.

4. **Update documentation** if needed:
   - Update README.md for new features
   - Add/update code comments
   - Update AGENTS.md if build process changes

5. **Commit with clear messages:**
   ```bash
   git commit -m "feat: add snapshot compression support

   - Add gzip compression for snapshot data
   - Update SnapshotMetadata with compression info
   - Add benchmarks showing 60% size reduction"
   ```

   **Commit message format:**
   - `feat:` - New feature
   - `fix:` - Bug fix
   - `docs:` - Documentation changes
   - `test:` - Test additions/changes
   - `refactor:` - Code refactoring
   - `perf:` - Performance improvement
   - `chore:` - Build process, dependencies

### Submitting the PR

1. **Push to your fork:**
   ```bash
   git push origin feature/my-new-feature
   ```

2. **Create Pull Request** on GitHub with:
   - **Clear title**: Summarize the change in one line
   - **Description**: Explain what, why, and how
   - **Related issues**: Link to any related issues
   - **Testing**: Describe how you tested the changes
   - **Breaking changes**: Highlight any breaking changes

**PR Template:**
```markdown
## Description
Brief description of what this PR does.

## Motivation
Why is this change needed? What problem does it solve?

## Changes
- Change 1
- Change 2
- Change 3

## Testing
How did you test this? Include:
- New test cases added
- Manual testing performed
- Edge cases considered

## Breaking Changes
List any breaking changes and migration steps.

## Checklist
- [ ] Tests pass locally (`task test`)
- [ ] Code is formatted (`task fmt`)
- [ ] Linter passes (`task lint`)
- [ ] Documentation updated
- [ ] Examples updated (if applicable)
```

### Review Process

1. **Automated checks** must pass:
   - All tests
   - Linter
   - Code formatting

2. **Code review** by maintainers:
   - At least one approval required
   - Address all feedback
   - Keep discussions focused and respectful

3. **Iterations**:
   - Make requested changes
   - Push additional commits (don't force push)
   - Re-request review when ready

4. **Merge**:
   - Maintainers will merge using squash or merge commit
   - Delete your feature branch after merge

## Code Review Guidelines

### As a Reviewer

**What to look for:**
- Correctness: Does the code work as intended?
- Tests: Are there adequate tests?
- Design: Does it fit the framework architecture?
- Performance: Any performance concerns?
- Security: Any security implications?
- Documentation: Is the code well-documented?

**How to review:**
- Be constructive and specific
- Suggest alternatives, don't just criticize
- Approve when ready, even if minor nits remain
- Use "Request Changes" only for blocking issues

**Review checklist:**
- [ ] Code follows project conventions
- [ ] Tests are comprehensive
- [ ] Documentation is updated
- [ ] No breaking changes (or clearly documented)
- [ ] Performance impact considered
- [ ] Error handling is appropriate
- [ ] Generated code is up to date

### As an Author

**Responding to feedback:**
- Address all comments, even if just acknowledging
- Ask for clarification if feedback is unclear
- Explain your reasoning when disagreeing
- Be open to suggestions and alternatives
- Thank reviewers for their time

**Making changes:**
- Add commits for requested changes (don't amend)
- Mark conversations as resolved when addressed
- Re-request review when all feedback is addressed

## Adding New Features

### Framework Features

For changes to core framework (`pkg/eventsourcing`):

1. **Discuss first**: Open an issue to discuss the feature
2. **Design**: Document the API and behavior
3. **Implement**: Add the feature with tests
4. **Document**: Update README and add code examples
5. **Example**: Add usage to `examples/bankaccount`

### Middleware

For new middleware (`pkg/middleware`):

1. **Implement interface**:
   ```go
   type MyMiddleware struct {
       // configuration
   }

   func (m *MyMiddleware) Handle(ctx context.Context, cmd interface{}, next eventsourcing.CommandHandlerFunc) (*eventsourcing.CommandResult, error) {
       // before command
       result, err := next(ctx, cmd)
       // after command
       return result, err
   }
   ```

2. **Add tests**: Unit tests with mock handlers
3. **Document**: Add section to README
4. **Example**: Show usage in examples

### Event Store Implementations

For new event store backends:

1. **Implement interfaces**:
   - `EventStore`
   - `SnapshotStore` (optional)
   - `CheckpointStore` (for projections)

2. **Add tests**: Use standard test suite
3. **Document**: Add setup guide
4. **Example**: Create example directory

## Documentation

### Code Documentation

**Package documentation:**
```go
// Package eventsourcing provides core abstractions for event sourcing and CQRS.
//
// This package defines the fundamental interfaces and types used throughout
// the framework, including aggregates, events, commands, and stores.
package eventsourcing
```

**Function documentation:**
```go
// SaveEvents persists a batch of events to the event store atomically.
// All events must belong to the same aggregate and have sequential versions.
//
// Returns an error if:
//   - Events belong to different aggregates
//   - Versions are not sequential
//   - Optimistic concurrency check fails
//   - Database error occurs
func (s *EventStore) SaveEvents(events []*Event) error {
    // implementation
}
```

**Complex logic:**
```go
// Calculate retention version (keep last 3 snapshots)
// Example: at version 10 with interval 2:
//   10 - (3 * 2) = 4
//   Keep snapshots at versions 4, 6, 8, 10
//   Delete snapshots before version 4
retentionVersion := currentVersion - (3 * strategy.Interval)
```

### README Updates

Update README.md when:
- Adding new features
- Changing API interfaces
- Adding middleware
- Changing build process
- Adding dependencies

### AGENTS.md Updates

Update AGENTS.md when:
- Adding build steps
- Changing task commands
- Adding dependencies
- Changing project structure
- Adding coding conventions

## Community

### Getting Help

- **GitHub Issues**: For bugs and feature requests
- **GitHub Discussions**: For questions and general discussion
- **Code Review**: Ask questions in PR comments

### Contributing to Discussions

- Be respectful and inclusive
- Stay on topic
- Help others when you can
- Share your experiences using the framework

### Reporting Bugs

Include in bug reports:
- Go version (`go version`)
- Operating system and architecture
- Minimal reproduction code
- Expected vs actual behavior
- Relevant logs/errors

**Bug report template:**
```markdown
## Bug Description
Clear description of the bug.

## Reproduction
Steps to reproduce:
1. Step 1
2. Step 2
3. Step 3

## Expected Behavior
What should happen.

## Actual Behavior
What actually happens.

## Environment
- Go version:
- OS/Arch:
- Framework version:

## Logs/Errors
```
paste error logs here
```
```

### Suggesting Features

Include in feature requests:
- Use case and motivation
- Proposed API/interface
- Alternatives considered
- Willingness to contribute

---

## Thank You!

Your contributions make this project better for everyone. We appreciate your time and effort!

If you have any questions about contributing, please open a GitHub issue or discussion.
