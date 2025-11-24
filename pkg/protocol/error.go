package protocol

import (
	"encoding/json"
	"fmt"
)

// AppError provides structured error information for RPC operations.
// It implements the error interface, making it fully Go-idiomatic.
type AppError struct {
	// Code is a machine-readable error code (e.g., "INSUFFICIENT_BALANCE")
	Code string `json:"code"`

	// Message is a human-readable error message
	Message string `json:"message"`

	// Solution suggests how to fix the error
	Solution string `json:"solution,omitempty"`

	// Details provides additional context (field names, values, etc.)
	Details map[string]string `json:"details,omitempty"`
}

// Error implements the error interface
func (e *AppError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// WithDetails adds additional context to the error
func (e *AppError) WithDetails(key, value string) *AppError {
	if e.Details == nil {
		e.Details = make(map[string]string)
	}
	e.Details[key] = value
	return e
}

// WithSolution adds a solution suggestion to the error
func (e *AppError) WithSolution(solution string) *AppError {
	e.Solution = solution
	return e
}

// NewAppError creates a new AppError with the given code and message
func NewAppError(code, message string) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
	}
}

// ErrorToAppError converts a standard error to AppError.
// If the error is already an AppError, it returns it as-is.
// Otherwise, it creates a new AppError with a generic code.
func ErrorToAppError(err error) *AppError {
	if err == nil {
		return nil
	}

	// If already an AppError, return as-is
	if appErr, ok := err.(*AppError); ok {
		return appErr
	}

	// Create generic AppError from standard error
	return &AppError{
		Code:    "INTERNAL_ERROR",
		Message: err.Error(),
	}
}

// MarshalJSON implements json.Marshaler
func (e *AppError) MarshalJSON() ([]byte, error) {
	type Alias AppError
	return json.Marshal(&struct {
		*Alias
	}{
		Alias: (*Alias)(e),
	})
}

// UnmarshalJSON implements json.Unmarshaler
func (e *AppError) UnmarshalJSON(data []byte) error {
	type Alias AppError
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(e),
	}
	return json.Unmarshal(data, &aux)
}

// Common error codes
const (
	ErrCodeInternal          = "INTERNAL_ERROR"
	ErrCodeInvalidArgument   = "INVALID_ARGUMENT"
	ErrCodeNotFound          = "NOT_FOUND"
	ErrCodeAlreadyExists     = "ALREADY_EXISTS"
	ErrCodePermissionDenied  = "PERMISSION_DENIED"
	ErrCodeUnauthenticated   = "UNAUTHENTICATED"
	ErrCodeResourceExhausted = "RESOURCE_EXHAUSTED"
	ErrCodeTimeout           = "TIMEOUT"
	ErrCodeConflict          = "CONFLICT"
)

// Common error constructors
func ErrInvalidArgument(message string) *AppError {
	return NewAppError(ErrCodeInvalidArgument, message)
}

func ErrNotFound(message string) *AppError {
	return NewAppError(ErrCodeNotFound, message)
}

func ErrAlreadyExists(message string) *AppError {
	return NewAppError(ErrCodeAlreadyExists, message)
}

func ErrPermissionDenied(message string) *AppError {
	return NewAppError(ErrCodePermissionDenied, message)
}

func ErrUnauthenticated(message string) *AppError {
	return NewAppError(ErrCodeUnauthenticated, message)
}

func ErrResourceExhausted(message string) *AppError {
	return NewAppError(ErrCodeResourceExhausted, message)
}

func ErrTimeout(message string) *AppError {
	return NewAppError(ErrCodeTimeout, message)
}

func ErrConflict(message string) *AppError {
	return NewAppError(ErrCodeConflict, message)
}

func ErrInternal(message string) *AppError {
	return NewAppError(ErrCodeInternal, message)
}

// IsAppError checks if an error is an AppError
func IsAppError(err error) bool {
	_, ok := err.(*AppError)
	return ok
}

// GetErrorCode returns the error code if the error is an AppError,
// otherwise returns ErrCodeInternal
func GetErrorCode(err error) string {
	if appErr, ok := err.(*AppError); ok {
		return appErr.Code
	}
	return ErrCodeInternal
}

// GetErrorMessage returns the error message
func GetErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// FormatError formats an error with additional context
func FormatError(err error, context string) error {
	if err == nil {
		return nil
	}

	if appErr, ok := err.(*AppError); ok {
		return &AppError{
			Code:     appErr.Code,
			Message:  fmt.Sprintf("%s: %s", context, appErr.Message),
			Solution: appErr.Solution,
			Details:  appErr.Details,
		}
	}

	return fmt.Errorf("%s: %w", context, err)
}
