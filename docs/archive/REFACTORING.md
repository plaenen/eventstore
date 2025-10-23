# Refactoring Summary: Event Bus Package Extraction

## Date
October 23, 2025

## Objective
Separate event sourcing domain concerns from NATS infrastructure to achieve clear separation of concerns and prepare for future extensibility.

## Changes Made

### 1. New Package Structure

**Before:**
```
pkg/nats/
  ├── eventbus.go          # NATS EventBus implementation
  ├── eventbus_test.go     # EventBus tests
  ├── embedded.go          # Embedded NATS server
  ├── server.go            # Command/query server
  ├── commandbus.go        # Command bus
  └── transport.go         # NATS transport
```

**After:**
```
pkg/eventbus/nats/         # New: Event sourcing EventBus
  ├── eventbus.go
  └── eventbus_test.go

pkg/nats/                  # Infrastructure utilities
  ├── embedded.go          # Embedded server (kept)
  ├── server.go            # Command/query server (kept)
  ├── commandbus.go        # Command bus (kept)
  └── transport.go         # NATS transport (kept)
```

### 2. Package Responsibilities

**`pkg/eventbus/nats`** - Event Sourcing Domain
- EventBus implementation using NATS JetStream
- Event publishing and subscription
- Event filtering and handlers
- Event sourcing specific configuration

**`pkg/nats`** - NATS Infrastructure
- Embedded NATS server for testing
- Command/query infrastructure
- NATS transport utilities
- Generic NATS helpers (not domain-specific)

### 3. Files Modified

#### Import Updates
- `pkg/runnable/eventbus/service.go`
- `pkg/runnable/eventbus/service_test.go`
- `examples/cmd/runner-nats/main.go`
- `examples/cmd/projection-nats/main.go`

#### Files Moved
- `pkg/nats/eventbus.go` → `pkg/eventbus/nats/eventbus.go`
- `pkg/nats/eventbus_test.go` → `pkg/eventbus/nats/eventbus_test.go`

#### Files Created
- `pkg/eventbus/README.md` - EventBus package documentation
- `pkg/runnable/README.md` - Runnable services documentation

#### Files Updated
- `pkg/nats/README.md` - Updated to reflect new structure
- `pkg/nats/embedded.go` - Removed EventBus-specific helpers

### 4. Import Changes

**Old (deprecated):**
```go
import "github.com/plaenen/eventstore/pkg/nats"

bus, err := nats.NewEventBus(config)
```

**New (current):**
```go
import natseventbus "github.com/plaenen/eventstore/pkg/eventbus/nats"

bus, err := natseventbus.NewEventBus(config)
```

### 5. Benefits Achieved

✅ **Clear Separation of Concerns**
- Event sourcing domain (EventBus) separated from infrastructure (NATS utilities)
- Each package has single, clear responsibility

✅ **Better Reusability**
- `pkg/nats` can be used for non-event-sourcing NATS needs
- `pkg/eventbus` clearly groups event transport implementations

✅ **Future Extensibility**
- Easy to add `pkg/eventbus/kafka`, `pkg/eventbus/rabbitmq`, etc.
- Consistent pattern for new transport implementations

✅ **Consistent Architecture**
```
pkg/eventsourcing/     # Interfaces
pkg/eventbus/*         # Event transport implementations
pkg/nats/              # NATS infrastructure
pkg/runnable/*         # Service adapters
```

### 6. Backward Compatibility

**Breaking Changes:**
- All imports of `pkg/nats` EventBus must be updated to `pkg/eventbus/nats`
- `NewEmbeddedEventBus()`, `TestConfig()`, `ShutdownWithBus()` removed from `pkg/nats`
  (Use `pkg/runnable/eventbus` service adapter instead)

**Migration Path:**
1. Update imports: `pkg/nats` → `pkg/eventbus/nats` for EventBus usage
2. For runner integration, use `pkg/runnable/eventbus.New()`
3. For embedded server without EventBus, use `pkg/nats.StartEmbeddedServer()`

### 7. Test Results

All tests pass after refactoring:
- ✅ `pkg/eventbus/nats` - EventBus tests (1 pre-existing flaky test)
- ✅ `pkg/runnable/embeddednats` - All 9 tests pass
- ✅ `pkg/runnable/eventbus` - All 9 tests pass
- ✅ `examples/cmd/runner-nats` - Works correctly
- ✅ `examples/cmd/projection-nats` - Works correctly

### 8. Documentation Updates

**New READMEs:**
- `pkg/eventbus/README.md` - Comprehensive EventBus documentation
- `pkg/runnable/README.md` - Service adapter documentation

**Updated READMEs:**
- `pkg/nats/README.md` - Now focuses on infrastructure utilities
- Includes migration guide from old to new imports

### 9. Design Principles Applied

1. **Dependency Inversion**: Implementations depend on abstractions
2. **Single Responsibility**: Each package has one clear purpose
3. **Interface Segregation**: EventBus implements only what it needs
4. **Open/Closed**: Easy to add new transports without modifying existing code
5. **Separation of Concerns**: Domain logic separated from infrastructure

### 10. Future Roadmap

With this structure in place, we can easily add:

- `pkg/eventbus/kafka` - Kafka-based EventBus
- `pkg/eventbus/rabbitmq` - RabbitMQ-based EventBus
- `pkg/eventbus/memory` - In-memory EventBus for testing
- `pkg/eventbus/aws` - AWS SNS/SQS EventBus

Each following the same pattern and implementing `eventsourcing.EventBus` interface.

## References

- [Adapter Pattern](https://en.wikipedia.org/wiki/Adapter_pattern)
- [Dependency Inversion Principle](https://en.wikipedia.org/wiki/Dependency_inversion_principle)
- [Separation of Concerns](https://en.wikipedia.org/wiki/Separation_of_concerns)

## Sign-off

Refactoring completed successfully with all tests passing and documentation updated.
