# Error Package (errorx)

The `errorx` package provides Go-idiomatic error handling for event sourcing applications. It establishes clear patterns for classifying, handling, and propagating errors throughout your application.

## Philosophy

This package distinguishes between two fundamental error categories:

| Category | Description | Client Response | Logging |
|----------|-------------|-----------------|---------|
| **Application Errors** | Expected errors from business logic, validation, domain rules | Return with helpful details | Normal logging |
| **System Errors** | Unexpected infrastructure failures, bugs | Return generic message | Log full context + alert |

This distinction is critical for:
- **Security**: System errors may leak internal details if exposed
- **User Experience**: Application errors should provide actionable feedback
- **Debugging**: System errors need full context for troubleshooting
- **Monitoring**: Different alert thresholds for each category

## Quick Start

```go
import "github.com/plaenen/eventstore/pkg/errorx"

// Check error classification
if errorx.IsApplicationError(err) {
    // Safe to return details to client
    return protocol.ErrNotFound(err.Error())
}

if errorx.IsSystemError(err) {
    // Log full error, return generic message
    logger.Error("system failure", "error", err)
    return protocol.ErrInternal("an internal error occurred")
}

// Check if operation should be retried
if errorx.IsRetryable(err) {
    return retryWithBackoff(operation)
}
```

## Sentinel Errors

### Application Errors (Expected)

These represent valid application states and should be returned to clients with helpful messages.

```go
var (
    ErrNotFound           // Resource doesn't exist
    ErrAlreadyExists      // Resource already exists (duplicate)
    ErrConflict           // Optimistic concurrency conflict
    ErrInvalidArgument    // Invalid input/arguments
    ErrPermissionDenied   // Insufficient permissions
    ErrUnauthenticated    // Missing or invalid authentication
    ErrPreconditionFailed // System state doesn't allow operation
    ErrResourceExhausted  // Quota or limit exceeded
)
```

### System Errors (Unexpected)

These represent infrastructure failures and should be sanitized before returning to clients.

```go
var (
    ErrInternal       // Internal system error
    ErrTimeout        // Operation timed out
    ErrUnavailable    // Service temporarily unavailable
    ErrDataCorruption // Data integrity issues
)
```

### Repository-Specific Errors

Specialized errors for event sourcing repositories.

```go
var (
    ErrAggregateNotFound         // Aggregate doesn't exist
    ErrEventStreamNotFound       // Event stream doesn't exist
    ErrConcurrencyConflict       // Version mismatch during save
    ErrInvalidVersion            // Invalid aggregate version
    ErrSnapshotNotFound          // Snapshot doesn't exist
    ErrUniqueConstraintViolation // Unique constraint violated
)
```

## Error Classification Functions

### IsApplicationError

Checks if an error is an expected application error.

```go
func IsApplicationError(err error) bool

// Usage
if errorx.IsApplicationError(err) {
    // Return to client with error message
    return protocol.ErrNotFound(err.Error())
}
```

### IsSystemError

Checks if an error is an unexpected system error.

```go
func IsSystemError(err error) bool

// Usage
if errorx.IsSystemError(err) {
    // Log full error for debugging
    logger.Error("system error", "error", err, "context", ctx)
    // Return sanitized error to client
    return protocol.ErrInternal("an internal error occurred")
}
```

### IsRetryable

Checks if an error might succeed on retry.

```go
func IsRetryable(err error) bool

// Retryable errors:
// - ErrConflict (optimistic locking)
// - ErrConcurrencyConflict
// - ErrTimeout
// - ErrUnavailable
// - ErrResourceExhausted

// Usage
if errorx.IsRetryable(err) {
    return retryOperation(ctx, operation, maxRetries)
}
```

## Structured Error Types

For errors that need additional context, use structured error types.

### NotFoundError

```go
type NotFoundError struct {
    ResourceType string // e.g., "Account", "Order"
    ResourceID   string // e.g., "550e8400-..."
}

// Create
err := errorx.NewNotFoundError("Account", "acc-123")
// Output: "Account not found: acc-123"

// Check with errors.Is
errors.Is(err, errorx.ErrNotFound) // true
```

### ConflictError

```go
type ConflictError struct {
    AggregateID     string
    ExpectedVersion int64
    ActualVersion   int64
}

// Create
err := errorx.NewConflictError("acc-123", 5, 7)
// Output: "version conflict on aggregate acc-123: expected v5, got v7"

// Check with errors.Is
errors.Is(err, errorx.ErrConflict)           // true
errors.Is(err, errorx.ErrConcurrencyConflict) // true
```

### UniqueConstraintError

```go
type UniqueConstraintError struct {
    IndexName string
    Value     string
    OwnerID   string
}

// Create
err := errorx.NewUniqueConstraintError("email", "user@example.com", "user-456")
// Output: "unique constraint violation: email='user@example.com' is already claimed by aggregate user-456"

// Check with errors.Is
errors.Is(err, errorx.ErrUniqueConstraintViolation) // true
```

### ValidationError

```go
type ValidationError struct {
    Field   string
    Value   string
    Message string
}

// Create
err := errorx.NewValidationError("email", "invalid", "must be a valid email address")
// Output: "validation failed for email='invalid': must be a valid email address"

// Create without value
err := errorx.NewValidationError("password", "", "is required")
// Output: "validation failed for password: is required"

// Check with errors.Is
errors.Is(err, errorx.ErrInvalidArgument) // true
```

## Helper Functions

### Wrap

Wraps an error with context while preserving error type.

```go
err := repo.Load(id)
if err != nil {
    return errorx.Wrap(err, "failed to load aggregate")
}
// Output: "failed to load aggregate: <original error>"
```

### Wrapf

Wraps an error with formatted context.

```go
err := repo.Load(id)
if err != nil {
    return errorx.Wrapf(err, "failed to load aggregate %s", id)
}
// Output: "failed to load aggregate acc-123: <original error>"
```

## Usage Patterns

### Repository Implementation

```go
func (r *AccountRepository) Load(aggregateID string) (*Account, error) {
    // Validate input (APPLICATION ERROR)
    if aggregateID == "" {
        return nil, fmt.Errorf("aggregate_id: %w", errorx.ErrInvalidArgument)
    }

    // Query database
    events, err := r.store.LoadEvents(aggregateID)
    if err != nil {
        // Check if it's "not found" (APPLICATION ERROR)
        if errors.Is(err, errorx.ErrNotFound) {
            return nil, errorx.NewNotFoundError("Account", aggregateID)
        }
        // Otherwise it's a SYSTEM ERROR
        return nil, fmt.Errorf("failed to load events: %w", err)
    }

    // Empty stream means aggregate doesn't exist
    if len(events) == 0 {
        return nil, errorx.NewNotFoundError("Account", aggregateID)
    }

    // Rebuild aggregate
    aggregate := NewAccount(aggregateID)
    for _, event := range events {
        if err := aggregate.Apply(event); err != nil {
            // Event application failure is a SYSTEM ERROR
            return nil, fmt.Errorf("%w: %v", errorx.ErrDataCorruption, err)
        }
    }

    return aggregate, nil
}
```

### Command Handler

```go
func (h *AccountHandler) OpenAccount(ctx context.Context, cmd *OpenAccountCommand) (*Response, error) {
    // Validate command (APPLICATION ERROR)
    if cmd.AccountId == "" {
        return nil, fmt.Errorf("account_id: %w", errorx.ErrInvalidArgument)
    }

    // Business validation
    if balance.IsNegative() {
        return nil, errorx.NewValidationError(
            "initial_balance",
            cmd.InitialBalance,
            "must be non-negative",
        )
    }

    // Create and save aggregate
    agg := NewAccount(cmd.AccountId)
    if err := agg.Open(cmd); err != nil {
        return nil, err
    }

    if err := h.repo.Save(agg); err != nil {
        // Check for APPLICATION errors
        if errors.Is(err, errorx.ErrUniqueConstraintViolation) {
            return nil, fmt.Errorf("account %s: %w", cmd.AccountId, errorx.ErrAlreadyExists)
        }
        // SYSTEM error - log and sanitize
        logger.Error("save failed", "error", err)
        return nil, errorx.ErrInternal
    }

    return &Response{AccountId: cmd.AccountId}, nil
}
```

### API/Transport Layer

```go
func (s *Server) HandleCommand(ctx context.Context, req *Request) (*Response, error) {
    response, err := s.handler.Execute(ctx, req)
    if err != nil {
        if errorx.IsApplicationError(err) {
            // Safe to return details
            return nil, convertToProtocolError(err)
        }

        // System error - log and sanitize
        logger.Error("command failed", "command", req.Type, "error", err)
        return nil, protocol.ErrInternal("an internal error occurred")
    }

    return response, nil
}

func convertToProtocolError(err error) error {
    switch {
    case errors.Is(err, errorx.ErrNotFound):
        return protocol.ErrNotFound(err.Error())
    case errors.Is(err, errorx.ErrAlreadyExists):
        return protocol.ErrAlreadyExists(err.Error())
    case errors.Is(err, errorx.ErrConflict):
        return protocol.ErrConflict(err.Error())
    case errors.Is(err, errorx.ErrInvalidArgument):
        return protocol.ErrInvalidArgument(err.Error())
    case errors.Is(err, errorx.ErrPermissionDenied):
        return protocol.ErrPermissionDenied(err.Error())
    case errors.Is(err, errorx.ErrUnauthenticated):
        return protocol.ErrUnauthenticated(err.Error())
    default:
        return protocol.ErrInternal("an error occurred")
    }
}
```

### Retry Logic

```go
func (h *AccountHandler) Deposit(ctx context.Context, cmd *DepositCommand) (*Response, error) {
    const maxRetries = 3

    var lastErr error
    for attempt := 0; attempt < maxRetries; attempt++ {
        response, err := h.tryDeposit(ctx, cmd)
        if err == nil {
            return response, nil
        }

        lastErr = err

        // Only retry retryable errors
        if !errorx.IsRetryable(err) {
            return nil, err
        }

        // Exponential backoff
        if attempt < maxRetries-1 {
            backoff := time.Duration(1<<attempt) * 100 * time.Millisecond
            time.Sleep(backoff)
        }
    }

    return nil, fmt.Errorf("operation failed after %d retries: %w", maxRetries, lastErr)
}
```

## Decision Tree

When handling an error, follow this decision tree:

```
Error encountered
      │
      ▼
┌─────────────────────────────────────┐
│ Is it an EXPECTED error?            │
│ (validation, not found, business)   │
└─────────────────┬───────────────────┘
                  │
      ┌───────────┴───────────┐
      │                       │
     YES                      NO
      │                       │
      ▼                       ▼
┌─────────────────┐   ┌─────────────────┐
│ APPLICATION     │   │ Is it RETRYABLE?│
│ Return to client│   │ (timeout,       │
│ with details    │   │  conflict)      │
└─────────────────┘   └────────┬────────┘
                               │
                   ┌───────────┴───────────┐
                   │                       │
                  YES                      NO
                   │                       │
                   ▼                       ▼
           ┌─────────────────┐   ┌─────────────────┐
           │ Retry with      │   │ SYSTEM ERROR    │
           │ backoff         │   │ Log full error  │
           └─────────────────┘   │ Return generic  │
                                 └─────────────────┘
```

## Error Response Matrix

| Error Type | Classification | Action | HTTP Status | Client Response |
|------------|---------------|--------|-------------|-----------------|
| Not Found | APPLICATION | Return with ID | 404 | Full details |
| Already Exists | APPLICATION | Return with ID | 409 | Full details |
| Invalid Input | APPLICATION | Return with field | 400 | Full details |
| Version Conflict | APPLICATION | Retry or return | 409 | Full details |
| Permission Denied | APPLICATION | Return | 403 | Message |
| Unauthenticated | APPLICATION | Return | 401 | Message |
| Database Failure | SYSTEM | Log + sanitize | 500 | Generic |
| Network Timeout | SYSTEM | Log + retry/fail | 503 | Generic |
| Data Corruption | SYSTEM | Log + alert | 500 | Generic |

## Best Practices

1. **Use sentinel errors for common cases**
   ```go
   if errors.Is(err, errorx.ErrNotFound) {
       // Handle not found
   }
   ```

2. **Use structured errors when context is needed**
   ```go
   return errorx.NewNotFoundError("Account", accountID)
   ```

3. **Always wrap errors with context**
   ```go
   return errorx.Wrapf(err, "failed to process order %s", orderID)
   ```

4. **Never expose system errors to clients**
   ```go
   if errorx.IsSystemError(err) {
       logger.Error("internal failure", "error", err)
       return protocol.ErrInternal("an error occurred")
   }
   ```

5. **Implement retry for retryable errors**
   ```go
   if errorx.IsRetryable(err) {
       // Retry with exponential backoff
   }
   ```

6. **Use errors.Is() for checking, not type assertions**
   ```go
   // Good
   if errors.Is(err, errorx.ErrNotFound) { }

   // Avoid
   if err == errorx.ErrNotFound { }
   ```
