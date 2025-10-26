package security

import (
	"errors"
	"fmt"

	"github.com/plaenen/eventstore/pkg/eventsourcing"
)

// ErrorMode determines how errors are sanitized.
type ErrorMode string

const (
	// ErrorModeProduction sanitizes all errors to prevent information disclosure
	ErrorModeProduction ErrorMode = "production"

	// ErrorModeDevelopment returns detailed errors for debugging
	ErrorModeDevelopment ErrorMode = "development"
)

// ErrorCode represents a safe error code that can be returned to clients.
type ErrorCode string

const (
	// Client errors (4xx equivalent)
	ErrorCodeNotFound          ErrorCode = "NOT_FOUND"
	ErrorCodeAlreadyExists     ErrorCode = "ALREADY_EXISTS"
	ErrorCodeInvalidInput      ErrorCode = "INVALID_INPUT"
	ErrorCodeConcurrencyConflict ErrorCode = "CONCURRENCY_CONFLICT"
	ErrorCodePermissionDenied  ErrorCode = "PERMISSION_DENIED"
	ErrorCodeUnauthenticated   ErrorCode = "UNAUTHENTICATED"
	ErrorCodeDuplicateCommand  ErrorCode = "DUPLICATE_COMMAND"

	// Server errors (5xx equivalent)
	ErrorCodeInternal         ErrorCode = "INTERNAL_ERROR"
	ErrorCodeUnavailable      ErrorCode = "SERVICE_UNAVAILABLE"
	ErrorCodeStorageError     ErrorCode = "STORAGE_ERROR"
	ErrorCodeTimeout          ErrorCode = "TIMEOUT"
)

// SafeError represents an error with a safe client-facing message and error code.
type SafeError struct {
	Code           ErrorCode
	Message        string
	InternalError  error
	InternalDetails map[string]interface{}
}

func (e *SafeError) Error() string {
	return e.Message
}

func (e *SafeError) Unwrap() error {
	return e.InternalError
}

// GetCode returns the error code for client responses.
func (e *SafeError) GetCode() ErrorCode {
	return e.Code
}

// GetInternalError returns the original error for server-side logging.
func (e *SafeError) GetInternalError() error {
	return e.InternalError
}

// GetInternalDetails returns additional context for server-side logging.
func (e *SafeError) GetInternalDetails() map[string]interface{} {
	return e.InternalDetails
}

// ErrorSanitizer sanitizes errors based on the current mode.
type ErrorSanitizer struct {
	mode ErrorMode
}

// NewErrorSanitizer creates a new error sanitizer with the specified mode.
func NewErrorSanitizer(mode ErrorMode) *ErrorSanitizer {
	return &ErrorSanitizer{mode: mode}
}

// SanitizeError converts an error into a safe error appropriate for the current mode.
//
// In production mode, it returns sanitized error messages with error codes.
// In development mode, it returns detailed error information for debugging.
//
// Example:
//   sanitizer := NewErrorSanitizer(ErrorModeProduction)
//   safeErr := sanitizer.SanitizeError(err)
//   // Returns: "An internal error occurred" with code INTERNAL_ERROR
func (s *ErrorSanitizer) SanitizeError(err error) error {
	if err == nil {
		return nil
	}

	// In development mode, return the original error
	if s.mode == ErrorModeDevelopment {
		return err
	}

	// In production mode, sanitize based on error type
	return s.sanitizeForProduction(err)
}

// sanitizeForProduction sanitizes errors for production use.
func (s *ErrorSanitizer) sanitizeForProduction(err error) error {
	// Check if it's already a SafeError
	var safeErr *SafeError
	if errors.As(err, &safeErr) {
		return safeErr
	}

	// Map known error types to safe errors
	switch {
	case errors.Is(err, eventsourcing.ErrAggregateNotFound):
		return &SafeError{
			Code:          ErrorCodeNotFound,
			Message:       "The requested resource was not found",
			InternalError: err,
		}

	case errors.Is(err, eventsourcing.ErrConcurrencyConflict):
		return &SafeError{
			Code:          ErrorCodeConcurrencyConflict,
			Message:       "The resource was modified by another operation. Please retry",
			InternalError: err,
		}

	case errors.Is(err, eventsourcing.ErrInvalidVersion):
		return &SafeError{
			Code:          ErrorCodeInvalidInput,
			Message:       "Invalid version specified",
			InternalError: err,
		}

	case errors.Is(err, eventsourcing.ErrUniqueConstraintViolation):
		return &SafeError{
			Code:          ErrorCodeAlreadyExists,
			Message:       "The resource already exists",
			InternalError: err,
		}

	case errors.Is(err, eventsourcing.ErrCommandAlreadyProcessed):
		return &SafeError{
			Code:          ErrorCodeDuplicateCommand,
			Message:       "This command has already been processed",
			InternalError: err,
		}

	case errors.Is(err, eventsourcing.ErrCommandNotFound):
		return &SafeError{
			Code:          ErrorCodeNotFound,
			Message:       "Command handler not found",
			InternalError: err,
		}

	case errors.Is(err, eventsourcing.ErrInvalidCommand):
		return &SafeError{
			Code:          ErrorCodeInvalidInput,
			Message:       "Invalid command",
			InternalError: err,
		}

	case errors.Is(err, eventsourcing.ErrSnapshotNotFound):
		return &SafeError{
			Code:          ErrorCodeNotFound,
			Message:       "Snapshot not found",
			InternalError: err,
		}

	default:
		// For any unrecognized error, return a generic internal error
		return &SafeError{
			Code:          ErrorCodeInternal,
			Message:       "An internal error occurred",
			InternalError: err,
		}
	}
}

// SanitizeUniqueConstraintError creates a safe error from a UniqueConstraintError.
//
// SEC-004: UniqueConstraintError exposes sensitive information (index names, values, owner IDs).
// This function sanitizes it to prevent information disclosure.
func (s *ErrorSanitizer) SanitizeUniqueConstraintError(indexName, value, ownerID string) error {
	internalErr := eventsourcing.NewUniqueConstraintError(indexName, value, ownerID)

	if s.mode == ErrorModeDevelopment {
		return internalErr
	}

	return &SafeError{
		Code:          ErrorCodeAlreadyExists,
		Message:       "The resource already exists",
		InternalError: internalErr,
		InternalDetails: map[string]interface{}{
			"index_name": indexName,
			"value":      value,
			"owner_id":   ownerID,
		},
	}
}

// SanitizeDatabaseError sanitizes database errors to prevent information disclosure.
//
// SEC-004: Database errors can expose:
// - File paths
// - SQL queries
// - Database schema
// - Internal state
func (s *ErrorSanitizer) SanitizeDatabaseError(operation string, err error) error {
	if err == nil {
		return nil
	}

	if s.mode == ErrorModeDevelopment {
		return fmt.Errorf("%s: %w", operation, err)
	}

	// Check for known database error patterns
	if errors.Is(err, eventsourcing.ErrAggregateNotFound) {
		return &SafeError{
			Code:          ErrorCodeNotFound,
			Message:       "The requested resource was not found",
			InternalError: err,
		}
	}

	// Generic storage error for unrecognized database errors
	return &SafeError{
		Code:          ErrorCodeStorageError,
		Message:       "A storage error occurred",
		InternalError: fmt.Errorf("%s: %w", operation, err),
		InternalDetails: map[string]interface{}{
			"operation": operation,
		},
	}
}

// SanitizePanicError sanitizes panic errors to prevent information disclosure.
//
// SEC-004: Panic values can contain sensitive information from the call stack.
func (s *ErrorSanitizer) SanitizePanicError(panicValue interface{}) error {
	internalErr := fmt.Errorf("panic: %v", panicValue)

	if s.mode == ErrorModeDevelopment {
		return internalErr
	}

	return &SafeError{
		Code:          ErrorCodeInternal,
		Message:       "An internal error occurred",
		InternalError: internalErr,
		InternalDetails: map[string]interface{}{
			"panic": true,
		},
	}
}

// NewSafeError creates a new SafeError with the specified code and messages.
//
// The clientMessage is returned to clients.
// The internalMessage and error are logged server-side only.
func NewSafeError(code ErrorCode, clientMessage string, internalError error, details map[string]interface{}) *SafeError {
	return &SafeError{
		Code:            code,
		Message:         clientMessage,
		InternalError:   internalError,
		InternalDetails: details,
	}
}

// IsClientError returns true if the error code represents a client error (4xx equivalent).
func IsClientError(code ErrorCode) bool {
	switch code {
	case ErrorCodeNotFound,
		ErrorCodeAlreadyExists,
		ErrorCodeInvalidInput,
		ErrorCodeConcurrencyConflict,
		ErrorCodePermissionDenied,
		ErrorCodeUnauthenticated,
		ErrorCodeDuplicateCommand:
		return true
	default:
		return false
	}
}

// IsServerError returns true if the error code represents a server error (5xx equivalent).
func IsServerError(code ErrorCode) bool {
	switch code {
	case ErrorCodeInternal,
		ErrorCodeUnavailable,
		ErrorCodeStorageError,
		ErrorCodeTimeout:
		return true
	default:
		return false
	}
}

// GetHTTPStatus returns the HTTP status code equivalent for an error code.
func GetHTTPStatus(code ErrorCode) int {
	switch code {
	case ErrorCodeNotFound:
		return 404
	case ErrorCodeAlreadyExists:
		return 409
	case ErrorCodeInvalidInput:
		return 400
	case ErrorCodeConcurrencyConflict:
		return 409
	case ErrorCodePermissionDenied:
		return 403
	case ErrorCodeUnauthenticated:
		return 401
	case ErrorCodeDuplicateCommand:
		return 409
	case ErrorCodeInternal:
		return 500
	case ErrorCodeUnavailable:
		return 503
	case ErrorCodeStorageError:
		return 500
	case ErrorCodeTimeout:
		return 504
	default:
		return 500
	}
}
