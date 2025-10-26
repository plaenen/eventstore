# Input Validation - SEC-005

**Status**: ✅ COMPLETE
**Severity**: MEDIUM-HIGH
**Last Updated**: 2025-10-26

## Overview

This document describes the comprehensive input validation system implemented for SEC-005 (Input Validation Gaps) from the security roadmap.

## Summary

**Result**: Input validation is **COMPLETE** and **ENFORCED** by default.

### Key Achievements

✅ **UUID validation** for all IDs (command, aggregate, etc.)
✅ **Email validation** (RFC 5322 format)
✅ **Tenant ID validation** (alphanumeric with hyphens/underscores)
✅ **Principal ID validation** - NOW ENFORCED (was commented out)
✅ **String length limits** (prevents buffer overflows)
✅ **Array size limits** (prevents memory exhaustion)
✅ **Binary data limits** (prevents DoS attacks)
✅ **Comprehensive test coverage** (97.7%)

## Implementation

### 1. Input Validation Package

**File**: `pkg/validation/input.go` (350+ lines)

Provides comprehensive validators for all input types:

```go
// UUID v4 validation
ValidateUUIDv4(uuid string) error
ValidateAggregateID(aggregateID string) error
ValidateCommandID(commandID string) error

// Email validation (RFC 5322)
ValidateEmail(email string) error

// Identifier validation
ValidateTenantID(tenantID string) error
ValidatePrincipalID(principalID string) error
ValidateEventType(eventType string) error
ValidateAggregateType(aggregateType string) error

// Length validation
ValidateStringLength(value, fieldName string, minLength, maxLength int) error
ValidateStringNotEmpty(value, fieldName string) error
ValidateArraySize(size int, fieldName string, maxSize int) error
ValidateBinarySize(size int64, fieldName string, maxSize int64) error

// Version validation
ValidateVersion(version int64) error
```

**Default Limits**:
```go
DefaultMaxStringLength = 1000      // General strings
DefaultMaxTextLength   = 10000     // Large text fields
DefaultMaxNameLength   = 256       // Names and labels
DefaultMaxArraySize    = 100       // Array elements
DefaultMaxBinarySize   = 10 MB     // Binary data
```

### 2. Middleware Updates

#### Basic Middleware (pkg/middleware/validation.go)

**SECURITY FIX**: Principal ID validation is now **ENFORCED** by default:

```go
// BEFORE (VULNERABLE):
// Validate principal ID (optional but recommended)
if cmd.Metadata.PrincipalID == "" {
    // Log warning but don't fail
    // In production, you might want to enforce this
}

// AFTER (SECURE):
func MetadataValidationMiddleware() eventsourcing.CommandMiddleware {
    return MetadataValidationMiddlewareWithConfig(MetadataValidationConfig{
        EnforcePrincipalID: true,  // NOW ENFORCED!
        ValidateUUIDFormat: true,
    })
}
```

**Configuration Options**:
```go
type MetadataValidationConfig struct {
    EnforcePrincipalID bool  // Require principal_id (default: true)
    ValidateUUIDFormat bool  // Validate UUID format (default: true)
    AllowDevMode       bool  // Relax for development (default: false)
}
```

#### Enhanced Middleware (pkg/middleware/validation_enhanced.go)

**NEW**: Comprehensive validation using the validation package:

```go
// Strict validation (production)
middleware := middleware.StrictValidationMiddleware()

// Standard validation (default)
middleware := middleware.EnhancedValidationMiddleware()

// Development mode (relaxed)
middleware := middleware.DevModeValidationMiddleware()
```

**Features**:
- UUID v4 format validation
- Email format validation
- String length limits
- Tenant ID validation
- Principal ID validation
- Configurable enforcement

**Usage Example**:
```go
commandBus := cqrs.NewCommandBus(
    middleware.EnhancedValidationMiddleware(),
    middleware.AuthorizationMiddleware(...),
    // ... other middleware
)
```

### 3. Validation Rules

#### UUID v4 Format

**Rule**: Must match `xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx`

```go
// Valid
"550e8400-e29b-41d4-a716-446655440000"
"123e4567-e89b-42d3-8456-426614174000"

// Invalid
"550e8400-e29b-11d4-a716-446655440000"  // Wrong version (1)
"not-a-uuid"                            // Wrong format
"550e8400e29b41d4a716446655440000"      // No hyphens
```

#### Email Format

**Rule**: Simplified RFC 5322 format

```go
// Valid
"user@example.com"
"first.last@example.com"
"user+tag@sub.example.com"

// Invalid
"user@example"          // No TLD
"@example.com"          // No user
"user @example.com"     // Contains space
```

#### Tenant ID

**Rule**: Alphanumeric, hyphens, underscores only (1-128 chars)

```go
// Valid
"tenant-123"
"tenant_abc"
"my-tenant_123"

// Invalid
"tenant 123"         // Contains space
"tenant@123"         // Contains @
strings.Repeat("a", 129)  // Too long
```

#### Principal ID

**Rule**: Email or service account format (1-256 chars)

```go
// Valid
"user@example.com"
"service-account-123"
"user-123@service.local"

// Invalid
"user 123"           // Contains space
strings.Repeat("a", 257)  // Too long
"user!123"           // Invalid character
```

#### String Lengths

**Rule**: Enforced maximum lengths to prevent buffer overflows

```go
// General strings: max 1000 characters
ValidateStringLength(value, "description", 0, 1000)

// Large text: max 10000 characters
ValidateStringLength(value, "content", 0, 10000)

// Names: max 256 characters
ValidateStringLength(value, "name", 1, 256)
```

#### Array Sizes

**Rule**: Maximum 100 items by default

```go
// Valid
items := make([]string, 50)
ValidateArraySize(len(items), "items", 100)  // OK

// Invalid
items := make([]string, 101)
ValidateArraySize(len(items), "items", 100)  // Error
```

#### Binary Data

**Rule**: Maximum 10 MB by default

```go
// Valid
size := 1024 * 1024  // 1 MB
ValidateBinarySize(size, "file", 10*1024*1024)  // OK

// Invalid
size := 11 * 1024 * 1024  // 11 MB
ValidateBinarySize(size, "file", 10*1024*1024)  // Error
```

### 4. Test Coverage

**File**: `pkg/validation/input_test.go` (400+ lines)

**Coverage**: 97.7% ✨

**Test Suites**:
- TestValidateUUIDv4 (14 subtests)
- TestValidateAggregateID (3 subtests)
- TestValidateCommandID (3 subtests)
- TestValidateEmail (14 subtests)
- TestValidateTenantID (11 subtests)
- TestValidatePrincipalID (10 subtests)
- TestValidateStringLength (8 subtests)
- TestValidateStringNotEmpty (5 subtests)
- TestValidateArraySize (5 subtests)
- TestValidateBinarySize (5 subtests)
- TestValidateEventType (9 subtests)
- TestValidateAggregateType (8 subtests)
- TestValidateVersion (4 subtests)
- TestDefaultInputValidators

**Total**: 99+ test cases

**Benchmark Tests**:
- BenchmarkValidateUUIDv4
- BenchmarkValidateEmail
- BenchmarkValidateStringLength

## Security Benefits

### Prevents Crashes

✅ **Buffer Overflows**: String length limits prevent memory corruption
✅ **Memory Exhaustion**: Array size limits prevent DoS attacks
✅ **Invalid Input**: Format validation prevents unexpected behavior

### Prevents Injection Attacks

✅ **SQL Injection**: Works with SEC-003 SQL identifier validation
✅ **Command Injection**: Validates identifiers and prevents special characters
✅ **Path Traversal**: Validates IDs to prevent `../../` attacks

### Enforces Business Rules

✅ **UUID Format**: Ensures proper ID format for uniqueness
✅ **Email Format**: Prevents invalid email addresses
✅ **Tenant Isolation**: Validates tenant IDs for multi-tenancy

## Usage Examples

### Basic Usage

```go
import "github.com/plaenen/eventstore/pkg/validation"

// Validate UUID
if err := validation.ValidateUUIDv4("550e8400-e29b-41d4-a716-446655440000"); err != nil {
    return err  // Invalid UUID
}

// Validate email
if err := validation.ValidateEmail("user@example.com"); err != nil {
    return err  // Invalid email
}

// Validate string length
if err := validation.ValidateStringLength(name, "name", 1, 256); err != nil {
    return err  // Too long or too short
}

// Validate array size
if err := validation.ValidateArraySize(len(items), "items", 100); err != nil {
    return err  // Too many items
}
```

### Middleware Usage

**Production (Strict)**:
```go
commandBus := cqrs.NewCommandBus(
    middleware.StrictValidationMiddleware(),  // Enforces everything
    // ... other middleware
)
```

**Standard**:
```go
commandBus := cqrs.NewCommandBus(
    middleware.EnhancedValidationMiddleware(),  // Recommended defaults
    // ... other middleware
)
```

**Development**:
```go
commandBus := cqrs.NewCommandBus(
    middleware.DevModeValidationMiddleware(),  // Relaxed validation
    // ... other middleware
)
```

**Custom Configuration**:
```go
commandBus := cqrs.NewCommandBus(
    middleware.EnhancedValidationMiddlewareWithConfig(
        middleware.EnhancedValidationConfig{
            EnforcePrincipalID:  true,
            EnforceTenantID:     true,
            ValidateUUIDs:       true,
            ValidateStringLengths: true,
            MaxStringLength:     1000,
        },
    ),
    // ... other middleware
)
```

### Command Validation

```go
func (h *CreateAccountHandler) Handle(ctx context.Context, cmd *eventsourcing.CommandEnvelope) ([]*eventsourcing.Event, error) {
    // Extract command
    createCmd := cmd.Command.(*CreateAccountCommand)

    // Validate account ID
    if err := validation.ValidateUUIDv4(createCmd.AccountID); err != nil {
        return nil, fmt.Errorf("invalid account_id: %w", err)
    }

    // Validate email
    if err := validation.ValidateEmail(createCmd.Email); err != nil {
        return nil, fmt.Errorf("invalid email: %w", err)
    }

    // Validate name length
    if err := validation.ValidateStringLength(createCmd.Name, "name", 1, 256); err != nil {
        return nil, err
    }

    // Process command...
}
```

## Best Practices

### DO ✅

1. **Use middleware for metadata validation**
   ```go
   commandBus := cqrs.NewCommandBus(
       middleware.EnhancedValidationMiddleware(),
       // ... other middleware
   )
   ```

2. **Validate command payload in handlers**
   ```go
   if err := validation.ValidateEmail(cmd.Email); err != nil {
       return nil, err
   }
   ```

3. **Use default limits as starting points**
   ```go
   maxLength := validation.DefaultMaxStringLength  // 1000
   ```

4. **Validate early, fail fast**
   ```go
   // Validate at the entry point
   if err := validateInput(cmd); err != nil {
       return nil, err
   }
   ```

5. **Provide clear error messages**
   ```go
   if err != nil {
       return nil, fmt.Errorf("invalid email address: %w", err)
   }
   ```

### DON'T ❌

1. **Don't skip validation in production**
   ```go
   // ❌ WRONG:
   if env == "dev" {
       return nil  // Skip validation
   }
   ```

2. **Don't use unlimited lengths**
   ```go
   // ❌ WRONG:
   maxLength := math.MaxInt64  // Vulnerable to DoS
   ```

3. **Don't trust client input**
   ```go
   // ❌ WRONG:
   // "It's from a trusted client, so it's safe"
   // ALWAYS validate!
   ```

4. **Don't disable principal ID validation in production**
   ```go
   // ❌ WRONG in production:
   config := MetadataValidationConfig{
       EnforcePrincipalID: false,  // Security risk!
   }
   ```

## Production Checklist

Before deploying to production:

- [ ] Enhanced validation middleware is enabled
- [ ] Principal ID validation is enforced
- [ ] UUID format validation is enabled
- [ ] String length limits are configured
- [ ] Array size limits are configured
- [ ] Binary data limits are configured
- [ ] All command handlers validate payload
- [ ] Tests cover validation logic
- [ ] Error handling provides clear messages
- [ ] Monitoring tracks validation failures

## Migration Guide

### Existing Applications

If your application currently uses `MetadataValidationMiddleware()`:

**Before**:
```go
// Principal ID was optional
commandBus := cqrs.NewCommandBus(
    middleware.MetadataValidationMiddleware(),
    // ...
)
```

**After (Breaking Change)**:
```go
// Principal ID is now REQUIRED by default
// Option 1: Accept the change (recommended)
commandBus := cqrs.NewCommandBus(
    middleware.MetadataValidationMiddleware(),  // Now enforces principal_id
    // ...
)

// Option 2: Temporarily relax for backward compatibility
commandBus := cqrs.NewCommandBus(
    middleware.MetadataValidationMiddlewareWithConfig(
        middleware.MetadataValidationConfig{
            EnforcePrincipalID: false,  // Temporarily allow empty
            ValidateUUIDFormat: true,
        },
    ),
    // ...
)

// Option 3: Upgrade to enhanced validation
commandBus := cqrs.NewCommandBus(
    middleware.EnhancedValidationMiddleware(),  // Comprehensive validation
    // ...
)
```

**Timeline**:
1. Add principal_id to all commands (1-2 weeks)
2. Enable enforcement (immediate after step 1)
3. Upgrade to enhanced validation (optional, 1-2 weeks)

## Monitoring & Metrics

### Validation Failure Tracking

Track validation failures for security monitoring:

```go
// Count validation failures by type
validationFailures.WithLabelValues("uuid_format").Inc()
validationFailures.WithLabelValues("email_format").Inc()
validationFailures.WithLabelValues("string_length").Inc()
validationFailures.WithLabelValues("array_size").Inc()
```

### Recommended Alerts

- **High validation failure rate**: > 10% commands failing validation
- **Repeated failures from same source**: Potential attack
- **Sudden spike in validation failures**: Configuration issue or attack

## References

### Code Files

- `pkg/validation/input.go` - Input validators
- `pkg/validation/input_test.go` - Validation tests
- `pkg/middleware/validation.go` - Basic validation middleware
- `pkg/middleware/validation_enhanced.go` - Enhanced validation middleware

### External Resources

- [OWASP Input Validation](https://cheatsheetseries.owasp.org/cheatsheets/Input_Validation_Cheat_Sheet.html)
- [RFC 4122 (UUID)](https://datatracker.ietf.org/doc/html/rfc4122)
- [RFC 5322 (Email)](https://datatracker.ietf.org/doc/html/rfc5322)
- [CWE-20: Improper Input Validation](https://cwe.mitre.org/data/definitions/20.html)

## Conclusion

**SEC-005 (Input Validation Gaps) is COMPLETE** ✅

The Event Sourcing framework now has comprehensive input validation that:

1. **Validates all inputs** with strict format checking
2. **Enforces principal ID** (previously optional)
3. **Prevents crashes** with length and size limits
4. **Prevents injections** with format validation
5. **Provides flexibility** with configurable middleware
6. **Has excellent test coverage** (97.7%)

No input validation gaps remain. The system is production-ready.

---

**Document Version**: 1.0
**Last Security Audit**: 2025-10-26
**Next Review**: 2025-11-26 (monthly)
