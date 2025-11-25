package errorx

import (
	"errors"
	"fmt"
)

// Error Classification:
//
// Go-idiomatic error handling distinguishes between two main categories:
//
// 1. Application Errors (Expected):
//    - Business logic violations
//    - Validation failures
//    - Domain rule violations
//    - Resource not found
//    - Optimistic concurrency conflicts
//    - These are EXPECTED errors that represent valid application states
//    - Should be exposed to clients with helpful messages
//    - Use sentinel errors (errors.New) and errors.Is() for checking
//
// 2. System Errors (Unexpected):
//    - Database connection failures
//    - Network timeouts
//    - Infrastructure failures
//    - Programming bugs (nil pointers, etc.)
//    - These are UNEXPECTED errors that represent system failures
//    - Should be logged with full context
//    - Should return generic error messages to clients (security)
//    - Use error wrapping with %w for context

// ============================================================================
// Application Errors (Expected - Domain/Business Logic)
// ============================================================================

var (
	// ErrNotFound indicates a requested resource doesn't exist.
	// This is an APPLICATION error - the request was valid, but the resource doesn't exist.
	ErrNotFound = errors.New("resource not found")

	// ErrAlreadyExists indicates a resource already exists (duplicate).
	// This is an APPLICATION error - creation failed due to existing resource.
	ErrAlreadyExists = errors.New("resource already exists")

	// ErrConflict indicates an optimistic concurrency conflict.
	// This is an APPLICATION error - another operation modified the resource.
	ErrConflict = errors.New("version conflict")

	// ErrInvalidArgument indicates invalid input/arguments.
	// This is an APPLICATION error - the request is malformed or invalid.
	ErrInvalidArgument = errors.New("invalid argument")

	// ErrPermissionDenied indicates insufficient permissions.
	// This is an APPLICATION error - the user lacks required permissions.
	ErrPermissionDenied = errors.New("permission denied")

	// ErrUnauthenticated indicates missing or invalid authentication.
	// This is an APPLICATION error - authentication required or failed.
	ErrUnauthenticated = errors.New("unauthenticated")

	// ErrPreconditionFailed indicates a precondition wasn't met.
	// This is an APPLICATION error - the system state doesn't allow this operation.
	ErrPreconditionFailed = errors.New("precondition failed")

	// ErrResourceExhausted indicates a quota or limit was exceeded.
	// This is an APPLICATION error - rate limit or quota exceeded.
	ErrResourceExhausted = errors.New("resource exhausted")
)

// ============================================================================
// System Errors (Unexpected - Infrastructure/System Failures)
// ============================================================================

var (
	// ErrInternal indicates an internal system error.
	// This is a SYSTEM error - unexpected infrastructure failure.
	// When returning to clients, sanitize the message to avoid leaking internal details.
	ErrInternal = errors.New("internal system error")

	// ErrTimeout indicates an operation timed out.
	// This is a SYSTEM error - infrastructure/network timeout.
	ErrTimeout = errors.New("operation timeout")

	// ErrUnavailable indicates a service is temporarily unavailable.
	// This is a SYSTEM error - infrastructure temporarily down.
	ErrUnavailable = errors.New("service unavailable")

	// ErrDataCorruption indicates data integrity issues.
	// This is a SYSTEM error - data is corrupted or inconsistent.
	ErrDataCorruption = errors.New("data corruption detected")
)

// ============================================================================
// Repository-Specific Errors
// ============================================================================

var (
	// ErrAggregateNotFound indicates an aggregate doesn't exist in the repository.
	// This is an APPLICATION error.
	ErrAggregateNotFound = errors.New("aggregate not found")

	// ErrEventStreamNotFound indicates an event stream doesn't exist.
	// This is an APPLICATION error.
	ErrEventStreamNotFound = errors.New("event stream not found")

	// ErrConcurrencyConflict indicates an optimistic concurrency conflict during save.
	// This is an APPLICATION error - expected in concurrent systems.
	ErrConcurrencyConflict = errors.New("concurrency conflict: aggregate version mismatch")

	// ErrInvalidVersion indicates an invalid aggregate version.
	// This is an APPLICATION error - version validation failed.
	ErrInvalidVersion = errors.New("invalid version")

	// ErrSnapshotNotFound indicates a snapshot doesn't exist.
	// This is an APPLICATION error (not always critical - can rebuild from events).
	ErrSnapshotNotFound = errors.New("snapshot not found")

	// ErrUniqueConstraintViolation indicates a unique constraint violation.
	// This is an APPLICATION error - business rule violation.
	ErrUniqueConstraintViolation = errors.New("unique constraint violation")
)

// ============================================================================
// Error Classification Functions
// ============================================================================

// IsApplicationError checks if an error is an expected application error.
//
// Application errors represent valid application states and should be:
// - Returned to clients with helpful messages
// - Used for control flow
// - Not logged as system errors
//
// Example:
//
//	if errors.IsApplicationError(err) {
//	    // Return to client with error message
//	    return protocol.ErrNotFound(err.Error())
//	}
func IsApplicationError(err error) bool {
	return errors.Is(err, ErrNotFound) ||
		errors.Is(err, ErrAlreadyExists) ||
		errors.Is(err, ErrConflict) ||
		errors.Is(err, ErrInvalidArgument) ||
		errors.Is(err, ErrPermissionDenied) ||
		errors.Is(err, ErrUnauthenticated) ||
		errors.Is(err, ErrPreconditionFailed) ||
		errors.Is(err, ErrResourceExhausted) ||
		errors.Is(err, ErrAggregateNotFound) ||
		errors.Is(err, ErrEventStreamNotFound) ||
		errors.Is(err, ErrConcurrencyConflict) ||
		errors.Is(err, ErrInvalidVersion) ||
		errors.Is(err, ErrSnapshotNotFound) ||
		errors.Is(err, ErrUniqueConstraintViolation)
}

// IsSystemError checks if an error is an unexpected system error.
//
// System errors represent infrastructure failures and should be:
// - Logged with full context for debugging
// - Sanitized before returning to clients (security)
// - Monitored and alerted on
//
// Example:
//
//	if errors.IsSystemError(err) {
//	    // Log full error for debugging
//	    logger.Error("system error", "error", err, "context", ctx)
//	    // Return sanitized error to client
//	    return protocol.ErrInternal("an internal error occurred")
//	}
func IsSystemError(err error) bool {
	return errors.Is(err, ErrInternal) ||
		errors.Is(err, ErrTimeout) ||
		errors.Is(err, ErrUnavailable) ||
		errors.Is(err, ErrDataCorruption)
}

// IsRetryable checks if an error might succeed on retry.
//
// Retryable errors are typically:
// - Concurrency conflicts (optimistic locking)
// - Temporary network issues
// - Service temporarily unavailable
//
// Example:
//
//	if errors.IsRetryable(err) {
//	    return retryOperation(ctx, operation, maxRetries)
//	}
func IsRetryable(err error) bool {
	return errors.Is(err, ErrConflict) ||
		errors.Is(err, ErrConcurrencyConflict) ||
		errors.Is(err, ErrTimeout) ||
		errors.Is(err, ErrUnavailable) ||
		errors.Is(err, ErrResourceExhausted)
}

// ============================================================================
// Structured Error Types (for additional context)
// ============================================================================

// NotFoundError provides details about a missing resource.
type NotFoundError struct {
	ResourceType string // e.g., "Aggregate", "User", "Order"
	ResourceID   string // e.g., "550e8400-e29b-41d4-a716-446655440000"
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("%s not found: %s", e.ResourceType, e.ResourceID)
}

func (e *NotFoundError) Is(target error) bool {
	return target == ErrNotFound
}

// NewNotFoundError creates a NotFoundError.
func NewNotFoundError(resourceType, resourceID string) error {
	return &NotFoundError{
		ResourceType: resourceType,
		ResourceID:   resourceID,
	}
}

// ConflictError provides details about a version conflict.
type ConflictError struct {
	AggregateID     string
	ExpectedVersion int64
	ActualVersion   int64
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("version conflict on aggregate %s: expected v%d, got v%d",
		e.AggregateID, e.ExpectedVersion, e.ActualVersion)
}

func (e *ConflictError) Is(target error) bool {
	return target == ErrConflict || target == ErrConcurrencyConflict
}

// NewConflictError creates a ConflictError.
func NewConflictError(aggregateID string, expected, actual int64) error {
	return &ConflictError{
		AggregateID:     aggregateID,
		ExpectedVersion: expected,
		ActualVersion:   actual,
	}
}

// UniqueConstraintError provides details about a constraint violation.
type UniqueConstraintError struct {
	IndexName string
	Value     string
	OwnerID   string
}

func (e *UniqueConstraintError) Error() string {
	return fmt.Sprintf("unique constraint violation: %s='%s' is already claimed by aggregate %s",
		e.IndexName, e.Value, e.OwnerID)
}

func (e *UniqueConstraintError) Is(target error) bool {
	return target == ErrUniqueConstraintViolation
}

// NewUniqueConstraintError creates a UniqueConstraintError.
func NewUniqueConstraintError(indexName, value, ownerID string) error {
	return &UniqueConstraintError{
		IndexName: indexName,
		Value:     value,
		OwnerID:   ownerID,
	}
}

// ValidationError provides details about validation failures.
type ValidationError struct {
	Field   string
	Value   string
	Message string
}

func (e *ValidationError) Error() string {
	if e.Value != "" {
		return fmt.Sprintf("validation failed for %s='%s': %s", e.Field, e.Value, e.Message)
	}
	return fmt.Sprintf("validation failed for %s: %s", e.Field, e.Message)
}

func (e *ValidationError) Is(target error) bool {
	return target == ErrInvalidArgument
}

// NewValidationError creates a ValidationError.
func NewValidationError(field, value, message string) error {
	return &ValidationError{
		Field:   field,
		Value:   value,
		Message: message,
	}
}

// ============================================================================
// Helper Functions
// ============================================================================

// Wrap wraps an error with context while preserving error type.
//
// Example:
//
//	err := repo.Load(id)
//	if err != nil {
//	    return errors.Wrap(err, "failed to load aggregate")
//	}
func Wrap(err error, message string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", message, err)
}

// Wrapf wraps an error with formatted context.
//
// Example:
//
//	err := repo.Load(id)
//	if err != nil {
//	    return errors.Wrapf(err, "failed to load aggregate %s", id)
//	}
func Wrapf(err error, format string, args ...interface{}) error {
	if err == nil {
		return nil
	}
	message := fmt.Sprintf(format, args...)
	return fmt.Errorf("%s: %w", message, err)
}
