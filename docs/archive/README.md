# Historical Documentation Archive

This directory contains historical design documents, migration guides, and architectural decision records (ADRs).

## Purpose

These documents are kept for:
- Understanding past architectural decisions
- Learning from design evolution
- Migration reference for older projects
- Historical context for current structure

## Documents

### Architecture Evolution

**[PROPOSED_STRUCTURE.md](PROPOSED_STRUCTURE.md)** - Original proposal for clean architecture
- Proposed separation of domain, store, CQRS, and messaging concerns
- Rationale for package structure
- Migration path from original structure

**[REFACTORING.md](REFACTORING.md)** - EventBus package extraction
- Separation of event sourcing domain from NATS infrastructure
- Benefits of pkg/messaging structure
- Test results and migration guide

### Proto Options Evolution

**[MIGRATION_COMPLETE.md](MIGRATION_COMPLETE.md)** - Proto options simplification
- 90% reduction in proto boilerplate
- Aggregate-level upcasting support
- Before/after comparison
- Migration examples

### Original Documentation

**[README_v1.md](README_v1.md)** - Original comprehensive README
- Complete API reference (1842 lines)
- All features documented in single file
- Valuable reference for detailed examples

## Current Documentation

For current documentation, see:
- **[Main README](../../README.md)** - Streamlined overview
- **[Documentation Index](../README.md)** - Organized by topic
- **[Examples](../../examples/)** - Working examples
- **[Package READMEs](../../pkg/)** - Package-specific docs

## When to Reference These Documents

**Use archive docs when:**
- Migrating from older versions
- Understanding why certain design choices were made
- Learning about the evolution of the framework
- Looking for detailed API examples

**Use current docs for:**
- Getting started with the framework
- Learning current best practices
- Understanding current architecture
- Finding working examples

## Document Status

| Document | Date | Status | Superseded By |
|----------|------|--------|---------------|
| PROPOSED_STRUCTURE.md | 2025-10-23 | Historical | pkg/ structure |
| REFACTORING.md | 2025-10-23 | Historical | pkg/messaging/README.md |
| MIGRATION_COMPLETE.md | 2025-10-23 | Reference | Current proto examples |
| README_v1.md | 2025-10-23 | Reference | Current README.md + docs/ |

## Contributing

When adding new historical documents:
1. Add a summary in this README
2. Update the document status table
3. Link to current replacement documentation
4. Explain why the document was archived
