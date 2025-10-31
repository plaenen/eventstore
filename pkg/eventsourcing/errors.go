package eventsourcing

import (
	"errors"
	"fmt"
)

var (
	// ErrAggregateNotFound is returned when an aggregate doesn't exist.
	ErrAggregateNotFound = errors.New("aggregate not found")

	// ErrConcurrencyConflict is returned when there's an optimistic concurrency conflict.
	ErrConcurrencyConflict = errors.New("concurrency conflict: aggregate version mismatch")

	// ErrInvalidVersion is returned when an invalid version is provided.
	ErrInvalidVersion = errors.New("invalid version")

	// ErrUniqueConstraintViolation is returned when a unique constraint would be violated.
	ErrUniqueConstraintViolation = errors.New("unique constraint violation")

	// ErrInvalidConstraintOperation is returned when an invalid constraint operation is provided.
	ErrInvalidConstraintOperation = errors.New("invalid constraint operation")

	// ErrCommandAlreadyProcessed is returned when a command has already been processed (idempotent).
	ErrCommandAlreadyProcessed = errors.New("command already processed")

	// ErrCommandNotFound is returned when a command handler is not registered.
	ErrCommandNotFound = errors.New("command handler not found")

	// ErrInvalidCommand is returned when a command is invalid.
	ErrInvalidCommand = errors.New("invalid command")

	// ErrSnapshotNotFound is returned when a snapshot cannot be found.
	ErrSnapshotNotFound = errors.New("snapshot not found")
)

// UniqueConstraintError provides detailed information about a constraint violation.
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

// NewUniqueConstraintError creates a new unique constraint error.
func NewUniqueConstraintError(indexName, value, ownerID string) error {
	return &UniqueConstraintError{
		IndexName: indexName,
		Value:     value,
		OwnerID:   ownerID,
	}
}

// NotImplementedError is returned by unimplemented aggregate handler methods
type NotImplementedError struct {
	Method string
}

func (e *NotImplementedError) Error() string {
	return fmt.Sprintf("method %s is not implemented", e.Method)
}

// ErrorToAppError converts a standard error to AppError for wire protocol
// This is used internally by generated server code
func ErrorToAppError(err error) *AppError {
	if err == nil {
		return nil
	}

	// Handle specific error types
	switch e := err.(type) {
	case *NotImplementedError:
		return &AppError{
			Code:    "UNIMPLEMENTED",
			Message: e.Error(),
		}
	case *UniqueConstraintError:
		return &AppError{
			Code:    "UNIQUE_CONSTRAINT_VIOLATION",
			Message: e.Error(),
		}
	default:
		// Check for standard errors
		if errors.Is(err, ErrAggregateNotFound) {
			return &AppError{
				Code:    "NOT_FOUND",
				Message: err.Error(),
			}
		}
		if errors.Is(err, ErrConcurrencyConflict) {
			return &AppError{
				Code:    "CONFLICT",
				Message: err.Error(),
			}
		}
		if errors.Is(err, ErrUniqueConstraintViolation) {
			return &AppError{
				Code:    "UNIQUE_CONSTRAINT_VIOLATION",
				Message: err.Error(),
			}
		}

		// Default to internal error
		return &AppError{
			Code:    "INTERNAL",
			Message: err.Error(),
		}
	}
}
