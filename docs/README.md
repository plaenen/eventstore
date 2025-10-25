# Documentation Index

Complete documentation for the Event Sourcing Framework.

## Getting Started

Start here if you're new to the framework:

1. **[Quick Start](../README.md#quick-start)** - Get up and running in 5 minutes
2. **[Examples](../examples/README.md)** - Learn from working examples
3. **[Projection Patterns](guides/projections.md)** - Building read models

### Release Information

- **[Latest Release](releases/)** - What's new in v0.0.6
- **[All Releases](releases/README.md)** - Complete release history
- **[Upgrade Guide](releases/v0.0.6.md#upgrade-guide)** - Migration instructions

## Core Concepts

### Architecture

- **[Package Structure](../README.md#package-structure)** - Understanding the codebase organization
- **[Domain Layer](../pkg/domain/)** - Pure domain types (Event, Command, Aggregate)
- **[Clean Architecture](../README.md#architecture)** - Layer separation and dependencies

### Event Sourcing

- **[Event Store](../pkg/store/)** - Persisting events (EventStore, Repository)
- **[Aggregates](../pkg/eventsourcing/)** - Domain aggregates and state management
- **[Snapshots](../pkg/store/sqlite/)** - Performance optimization
- **[Projections](../examples/PROJECTIONS.md)** - Building read models from events

### CQRS

- **[Command/Query Separation](../pkg/cqrs/)** - Request/reply pattern
- **[Handlers](../pkg/cqrs/nats/)** - Processing commands and queries
- **[Transport](../pkg/cqrs/nats/)** - NATS-based communication

### Messaging

- **[Event Bus](../pkg/messaging/)** - Publish/subscribe pattern
- **[NATS JetStream](../pkg/messaging/nats/)** - Event streaming
- **[Subscriptions](../pkg/messaging/nats/)** - Event handlers

## Implementation Guides

### Building Applications

- **[Code Generation](../README.md#code-generation)** - Generating from protobuf
- **[Projection Patterns](guides/projections.md)** - Three projection approaches
- **[Service Management](../pkg/runtime/)** - Running services in production
- **[Multi-tenancy](../pkg/multitenancy/)** - Multi-tenant support

### Advanced Topics

- **[Event Upcasting](guides/event-upcasting.md)** - Schema evolution and backward compatibility
- **[SDK Generation](guides/sdk-generation.md)** - Generating unified SDKs
- **[Observability](../pkg/observability/)** - OpenTelemetry integration
- **[Testing](../CONTRIBUTING.md#testing)** - Writing tests

### Security & Production

- **[Security Review Summary](REVIEW_SUMMARY.md)** - ⚠️ Current security posture and recommendations
- **[Security Roadmap](SECURITY_ROADMAP.md)** - Comprehensive security and architecture roadmap
- **[Immediate Actions](security/IMMEDIATE_ACTIONS.md)** - Critical security fixes (P0 - Do Now)
- **[Multi-tenancy Security](../pkg/multitenancy/)** - Tenant isolation and security

## Package Documentation

Each package has its own README with detailed documentation:

### Core Packages

- **[pkg/domain](../pkg/domain/)** - Domain types
- **[pkg/store](../pkg/store/)** - Event storage
- **[pkg/eventsourcing](../pkg/eventsourcing/)** - Core interfaces

### Infrastructure

- **[pkg/cqrs](../pkg/cqrs/)** - Command/Query transport
- **[pkg/messaging](../pkg/messaging/)** - Event pub/sub
- **[pkg/infrastructure/nats](../pkg/infrastructure/nats/)** - NATS utilities
- **[pkg/runtime](../pkg/runtime/)** - Service lifecycle

### Storage Implementations

- **[pkg/store/sqlite](../pkg/store/sqlite/)** - SQLite event store
  - Includes projection builder
  - Migration support
  - Checkpoint management

### Utilities

- **[pkg/observability](../pkg/observability/)** - OpenTelemetry
- **[pkg/multitenancy](../pkg/multitenancy/)** - Multi-tenant patterns

## Examples

### Complete Examples

- **[bankaccount-observability](../examples/cmd/bankaccount-observability/)** - Full CQRS application
- **[generic-projection](../examples/cmd/generic-projection/)** - Cross-domain projections
- **[projection-migrations](../examples/cmd/projection-migrations/)** - Schema migrations
- **[sqlite-projection](../examples/cmd/sqlite-projection/)** - Basic projections
- **[projection-nats](../examples/cmd/projection-nats/)** - Real-time processing

### Example Documentation

- **[Examples Overview](../examples/README.md)** - Understanding example structure
- **[Projection Patterns](../examples/PROJECTIONS.md)** - Detailed projection guide

## Contributing

- **[Contributing Guide](../CONTRIBUTING.md)** - How to contribute
- **[Development Setup](../CONTRIBUTING.md#development-setup)** - IDE and tools
- **[Code Style](../CONTRIBUTING.md#code-style-and-conventions)** - Standards
- **[Testing Guide](../CONTRIBUTING.md#testing)** - Writing tests

## Reference

### Protocol Buffers

- **[Proto Options](../proto/eventsourcing/options.proto)** - Custom options
- **[Example Protos](../examples/proto/account/v1/)** - Working examples

### Historical Documents

See the `archive/` directory for historical design decisions and migration guides:

- **[Proto Simplification](archive/MIGRATION_COMPLETE.md)** - Proto options evolution
- **[Package Refactoring](archive/PROPOSED_STRUCTURE.md)** - Architecture decisions
- **[EventBus Extraction](archive/REFACTORING.md)** - Messaging separation

## Getting Help

- **GitHub Issues** - Bug reports and feature requests
- **GitHub Discussions** - Questions and community support
- **Code Examples** - See `examples/` directory

## Quick Links

- [Main README](../README.md)
- [Contributing Guide](../CONTRIBUTING.md)
- [Examples](../examples/)
- [Package Documentation](../pkg/)
