package security

import (
	"errors"
	"fmt"
	"testing"

	"github.com/plaenen/eventstore/pkg/eventsourcing"
)

func TestSafeError(t *testing.T) {
	t.Run("Error method returns message", func(t *testing.T) {
		err := &SafeError{
			Code:    ErrorCodeInternal,
			Message: "test message",
		}
		if err.Error() != "test message" {
			t.Errorf("Error() = %q, want %q", err.Error(), "test message")
		}
	})

	t.Run("GetCode returns error code", func(t *testing.T) {
		err := &SafeError{
			Code:    ErrorCodeNotFound,
			Message: "not found",
		}
		if err.GetCode() != ErrorCodeNotFound {
			t.Errorf("GetCode() = %q, want %q", err.GetCode(), ErrorCodeNotFound)
		}
	})

	t.Run("Unwrap returns internal error", func(t *testing.T) {
		internalErr := errors.New("internal")
		err := &SafeError{
			Code:          ErrorCodeInternal,
			Message:       "safe message",
			InternalError: internalErr,
		}
		if errors.Unwrap(err) != internalErr {
			t.Error("Unwrap() should return internal error")
		}
	})

	t.Run("GetInternalDetails returns details", func(t *testing.T) {
		details := map[string]interface{}{
			"key": "value",
		}
		err := &SafeError{
			Code:            ErrorCodeInternal,
			Message:         "message",
			InternalDetails: details,
		}
		if err.GetInternalDetails()["key"] != "value" {
			t.Error("GetInternalDetails() should return correct details")
		}
	})
}

func TestNewSafeError(t *testing.T) {
	internalErr := errors.New("internal error")
	details := map[string]interface{}{"key": "value"}

	safeErr := NewSafeError(
		ErrorCodeInternal,
		"client message",
		internalErr,
		details,
	)

	if safeErr.Code != ErrorCodeInternal {
		t.Errorf("Code = %q, want %q", safeErr.Code, ErrorCodeInternal)
	}
	if safeErr.Message != "client message" {
		t.Errorf("Message = %q, want %q", safeErr.Message, "client message")
	}
	if safeErr.InternalError != internalErr {
		t.Error("InternalError should match")
	}
	if safeErr.InternalDetails["key"] != "value" {
		t.Error("InternalDetails should match")
	}
}

func TestErrorSanitizerDevelopmentMode(t *testing.T) {
	sanitizer := NewErrorSanitizer(ErrorModeDevelopment)

	t.Run("passes through all errors", func(t *testing.T) {
		originalErr := errors.New("detailed error with sensitive info")
		sanitizedErr := sanitizer.SanitizeError(originalErr)

		if sanitizedErr != originalErr {
			t.Error("Development mode should pass through original error")
		}
	})

	t.Run("returns nil for nil error", func(t *testing.T) {
		if sanitizer.SanitizeError(nil) != nil {
			t.Error("Should return nil for nil error")
		}
	})
}

func TestErrorSanitizerProductionMode(t *testing.T) {
	sanitizer := NewErrorSanitizer(ErrorModeProduction)

	tests := []struct {
		name           string
		err            error
		wantCode       ErrorCode
		wantMessage    string
		wantClientSafe bool
	}{
		{
			name:           "aggregate not found",
			err:            eventsourcing.ErrAggregateNotFound,
			wantCode:       ErrorCodeNotFound,
			wantMessage:    "The requested resource was not found",
			wantClientSafe: true,
		},
		{
			name:           "concurrency conflict",
			err:            eventsourcing.ErrConcurrencyConflict,
			wantCode:       ErrorCodeConcurrencyConflict,
			wantMessage:    "The resource was modified by another operation. Please retry",
			wantClientSafe: true,
		},
		{
			name:           "invalid version",
			err:            eventsourcing.ErrInvalidVersion,
			wantCode:       ErrorCodeInvalidInput,
			wantMessage:    "Invalid version specified",
			wantClientSafe: true,
		},
		{
			name:           "unique constraint violation",
			err:            eventsourcing.ErrUniqueConstraintViolation,
			wantCode:       ErrorCodeAlreadyExists,
			wantMessage:    "The resource already exists",
			wantClientSafe: true,
		},
		{
			name:           "command already processed",
			err:            eventsourcing.ErrCommandAlreadyProcessed,
			wantCode:       ErrorCodeDuplicateCommand,
			wantMessage:    "This command has already been processed",
			wantClientSafe: true,
		},
		{
			name:           "command not found",
			err:            eventsourcing.ErrCommandNotFound,
			wantCode:       ErrorCodeNotFound,
			wantMessage:    "Command handler not found",
			wantClientSafe: true,
		},
		{
			name:           "invalid command",
			err:            eventsourcing.ErrInvalidCommand,
			wantCode:       ErrorCodeInvalidInput,
			wantMessage:    "Invalid command",
			wantClientSafe: true,
		},
		{
			name:           "snapshot not found",
			err:            eventsourcing.ErrSnapshotNotFound,
			wantCode:       ErrorCodeNotFound,
			wantMessage:    "Snapshot not found",
			wantClientSafe: true,
		},
		{
			name:           "unknown error",
			err:            errors.New("database connection failed: cannot connect to /var/lib/sqlite/events.db"),
			wantCode:       ErrorCodeInternal,
			wantMessage:    "An internal error occurred",
			wantClientSafe: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sanitized := sanitizer.SanitizeError(tt.err)

			var safeErr *SafeError
			if !errors.As(sanitized, &safeErr) {
				t.Fatal("Sanitized error should be a SafeError")
			}

			if safeErr.Code != tt.wantCode {
				t.Errorf("Code = %q, want %q", safeErr.Code, tt.wantCode)
			}

			if safeErr.Message != tt.wantMessage {
				t.Errorf("Message = %q, want %q", safeErr.Message, tt.wantMessage)
			}

			// Check that sensitive info is not in client message
			if tt.wantClientSafe {
				// Original error should not contain "database", "sqlite", file paths, etc.
				if contains(safeErr.Message, []string{"database", "sqlite", "/var/", ".db", "SQL"}) {
					t.Errorf("Client message contains sensitive info: %q", safeErr.Message)
				}
			}

			// Check that internal error is preserved
			if safeErr.InternalError == nil {
				t.Error("InternalError should be preserved")
			}
		})
	}
}

func TestSanitizeUniqueConstraintError(t *testing.T) {
	t.Run("development mode", func(t *testing.T) {
		sanitizer := NewErrorSanitizer(ErrorModeDevelopment)
		err := sanitizer.SanitizeUniqueConstraintError(
			"email_index",
			"user@example.com",
			"aggregate-123",
		)

		// Should return the detailed UniqueConstraintError
		var ucErr *eventsourcing.UniqueConstraintError
		if !errors.As(err, &ucErr) {
			t.Fatal("Should return UniqueConstraintError in development mode")
		}

		errMsg := err.Error()
		if !contains(errMsg, []string{"email_index", "user@example.com", "aggregate-123"}) {
			t.Errorf("Development error should contain all details: %q", errMsg)
		}
	})

	t.Run("production mode", func(t *testing.T) {
		sanitizer := NewErrorSanitizer(ErrorModeProduction)
		err := sanitizer.SanitizeUniqueConstraintError(
			"email_index",
			"user@example.com",
			"aggregate-123",
		)

		var safeErr *SafeError
		if !errors.As(err, &safeErr) {
			t.Fatal("Should return SafeError in production mode")
		}

		// Client message should be generic
		if safeErr.Message != "The resource already exists" {
			t.Errorf("Message = %q, want generic message", safeErr.Message)
		}

		// Sensitive details should not be in client message
		if contains(safeErr.Message, []string{"email_index", "user@example.com", "aggregate-123"}) {
			t.Errorf("Client message contains sensitive info: %q", safeErr.Message)
		}

		// Internal details should be preserved for logging
		if safeErr.InternalDetails["index_name"] != "email_index" {
			t.Error("Internal details should preserve index name")
		}
		if safeErr.InternalDetails["value"] != "user@example.com" {
			t.Error("Internal details should preserve value")
		}
		if safeErr.InternalDetails["owner_id"] != "aggregate-123" {
			t.Error("Internal details should preserve owner ID")
		}
	})
}

func TestSanitizeDatabaseError(t *testing.T) {
	t.Run("development mode", func(t *testing.T) {
		sanitizer := NewErrorSanitizer(ErrorModeDevelopment)
		dbErr := errors.New("database error: UNIQUE constraint failed: events.aggregate_id")
		err := sanitizer.SanitizeDatabaseError("insert event", dbErr)

		errMsg := err.Error()
		if !contains(errMsg, []string{"insert event", "database error", "UNIQUE"}) {
			t.Errorf("Development error should contain all details: %q", errMsg)
		}
	})

	t.Run("production mode - unknown error", func(t *testing.T) {
		sanitizer := NewErrorSanitizer(ErrorModeProduction)
		dbErr := errors.New("database error: UNIQUE constraint failed: events.aggregate_id")
		err := sanitizer.SanitizeDatabaseError("insert event", dbErr)

		var safeErr *SafeError
		if !errors.As(err, &safeErr) {
			t.Fatal("Should return SafeError in production mode")
		}

		if safeErr.Code != ErrorCodeStorageError {
			t.Errorf("Code = %q, want %q", safeErr.Code, ErrorCodeStorageError)
		}

		if safeErr.Message != "A storage error occurred" {
			t.Errorf("Message = %q, want generic storage error", safeErr.Message)
		}

		// Sensitive details should not be in client message
		if contains(safeErr.Message, []string{"database", "UNIQUE", "aggregate_id", "events"}) {
			t.Errorf("Client message contains sensitive info: %q", safeErr.Message)
		}
	})

	t.Run("production mode - known error", func(t *testing.T) {
		sanitizer := NewErrorSanitizer(ErrorModeProduction)
		err := sanitizer.SanitizeDatabaseError("load aggregate", eventsourcing.ErrAggregateNotFound)

		var safeErr *SafeError
		if !errors.As(err, &safeErr) {
			t.Fatal("Should return SafeError")
		}

		if safeErr.Code != ErrorCodeNotFound {
			t.Errorf("Code = %q, want %q", safeErr.Code, ErrorCodeNotFound)
		}
	})
}

func TestSanitizePanicError(t *testing.T) {
	t.Run("development mode", func(t *testing.T) {
		sanitizer := NewErrorSanitizer(ErrorModeDevelopment)
		err := sanitizer.SanitizePanicError("null pointer dereference at /app/handler.go:123")

		errMsg := err.Error()
		if !contains(errMsg, []string{"panic", "null pointer", "/app/handler.go"}) {
			t.Errorf("Development error should contain panic details: %q", errMsg)
		}
	})

	t.Run("production mode", func(t *testing.T) {
		sanitizer := NewErrorSanitizer(ErrorModeProduction)
		err := sanitizer.SanitizePanicError("null pointer dereference at /app/handler.go:123")

		var safeErr *SafeError
		if !errors.As(err, &safeErr) {
			t.Fatal("Should return SafeError in production mode")
		}

		if safeErr.Code != ErrorCodeInternal {
			t.Errorf("Code = %q, want %q", safeErr.Code, ErrorCodeInternal)
		}

		if safeErr.Message != "An internal error occurred" {
			t.Errorf("Message = %q, want generic message", safeErr.Message)
		}

		// Panic details should not be in client message
		if contains(safeErr.Message, []string{"panic", "null pointer", "/app/", ".go"}) {
			t.Errorf("Client message contains sensitive info: %q", safeErr.Message)
		}

		// Panic flag should be in internal details
		if !safeErr.InternalDetails["panic"].(bool) {
			t.Error("Internal details should flag panic")
		}
	})
}

func TestIsClientError(t *testing.T) {
	clientErrors := []ErrorCode{
		ErrorCodeNotFound,
		ErrorCodeAlreadyExists,
		ErrorCodeInvalidInput,
		ErrorCodeConcurrencyConflict,
		ErrorCodePermissionDenied,
		ErrorCodeUnauthenticated,
		ErrorCodeDuplicateCommand,
	}

	for _, code := range clientErrors {
		t.Run(string(code), func(t *testing.T) {
			if !IsClientError(code) {
				t.Errorf("%s should be a client error", code)
			}
			if IsServerError(code) {
				t.Errorf("%s should not be a server error", code)
			}
		})
	}
}

func TestIsServerError(t *testing.T) {
	serverErrors := []ErrorCode{
		ErrorCodeInternal,
		ErrorCodeUnavailable,
		ErrorCodeStorageError,
		ErrorCodeTimeout,
	}

	for _, code := range serverErrors {
		t.Run(string(code), func(t *testing.T) {
			if !IsServerError(code) {
				t.Errorf("%s should be a server error", code)
			}
			if IsClientError(code) {
				t.Errorf("%s should not be a client error", code)
			}
		})
	}
}

func TestGetHTTPStatus(t *testing.T) {
	tests := []struct {
		code       ErrorCode
		wantStatus int
	}{
		{ErrorCodeNotFound, 404},
		{ErrorCodeAlreadyExists, 409},
		{ErrorCodeInvalidInput, 400},
		{ErrorCodeConcurrencyConflict, 409},
		{ErrorCodePermissionDenied, 403},
		{ErrorCodeUnauthenticated, 401},
		{ErrorCodeDuplicateCommand, 409},
		{ErrorCodeInternal, 500},
		{ErrorCodeUnavailable, 503},
		{ErrorCodeStorageError, 500},
		{ErrorCodeTimeout, 504},
		{ErrorCode("UNKNOWN"), 500}, // Unknown codes default to 500
	}

	for _, tt := range tests {
		t.Run(string(tt.code), func(t *testing.T) {
			status := GetHTTPStatus(tt.code)
			if status != tt.wantStatus {
				t.Errorf("GetHTTPStatus(%q) = %d, want %d", tt.code, status, tt.wantStatus)
			}
		})
	}
}

func TestWrappedErrors(t *testing.T) {
	t.Run("wrapped known error", func(t *testing.T) {
		sanitizer := NewErrorSanitizer(ErrorModeProduction)

		// Simulate wrapped error like in the codebase
		wrappedErr := fmt.Errorf("failed to save aggregate: %w", eventsourcing.ErrConcurrencyConflict)
		sanitized := sanitizer.SanitizeError(wrappedErr)

		var safeErr *SafeError
		if !errors.As(sanitized, &safeErr) {
			t.Fatal("Should return SafeError")
		}

		if safeErr.Code != ErrorCodeConcurrencyConflict {
			t.Errorf("Should detect wrapped error, got code %q", safeErr.Code)
		}
	})
}

func TestAlreadySafeError(t *testing.T) {
	t.Run("already safe error passes through", func(t *testing.T) {
		sanitizer := NewErrorSanitizer(ErrorModeProduction)

		originalSafe := &SafeError{
			Code:    ErrorCodeNotFound,
			Message: "Resource not found",
		}

		sanitized := sanitizer.SanitizeError(originalSafe)

		if sanitized != originalSafe {
			t.Error("Already safe errors should pass through unchanged")
		}
	})
}

// Helper function to check if a string contains any of the substrings
func contains(s string, substrings []string) bool {
	for _, sub := range substrings {
		if len(s) >= len(sub) {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}
