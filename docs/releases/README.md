# Release Notes

Release notes for the Event Sourcing Framework.

## Latest Release

**[v0.0.6](v0.0.6.md)** - October 23, 2025
- Major architectural refactoring
- Clean architecture with clear separation of concerns
- 90% reduction in proto boilerplate
- Event upcasting support
- Comprehensive documentation

## All Releases

| Version | Date | Highlights |
|---------|------|------------|
| [v0.0.6](v0.0.6.md) | Oct 23, 2025 | Clean architecture, proto simplification, event upcasting |

## Versioning

This project follows [Semantic Versioning](https://semver.org/):
- **MAJOR** version for incompatible API changes
- **MINOR** version for backwards-compatible functionality
- **PATCH** version for backwards-compatible bug fixes

### Pre-1.0 Releases

**Current Status:** Development (v0.0.x)

⚠️ **Note:** Prior to 1.0, APIs may change between minor versions. Breaking changes are documented in each release.

**Stability:**
- Core event sourcing patterns are stable
- APIs are evolving based on feedback
- Production use is possible with careful version pinning

**Path to 1.0:**
- API stability validation
- Production hardening
- Comprehensive testing
- Performance benchmarking

## Release Process

### Changelog Format

Each release note includes:
- **What's New** - Key highlights and features
- **Breaking Changes** - Incompatible changes with migration guide
- **New Features** - Added functionality
- **Improvements** - Enhancements to existing features
- **Bug Fixes** - Resolved issues
- **Documentation** - Doc updates
- **Upgrade Guide** - Step-by-step migration instructions

### Upgrade Recommendations

**Before upgrading:**
1. Read the release notes thoroughly
2. Review breaking changes section
3. Test in a development environment
4. Follow the upgrade guide
5. Run full test suite

**For production:**
- Pin versions in `go.mod`
- Test upgrades in staging first
- Have rollback plan ready
- Monitor after deployment

## Notification

Stay updated on new releases:
- **GitHub Releases** - Watch this repository
- **GitHub Discussions** - Follow announcements
- **Changelog** - Check this file regularly

## Contributing

Found an issue in a release? Please [open an issue](https://github.com/plaenen/eventstore/issues).

Suggestions for future releases? Start a [discussion](https://github.com/plaenen/eventstore/discussions).
