package errorx

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestSentinelErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		msg  string
	}{
		{"ErrNotFound", ErrNotFound, "resource not found"},
		{"ErrAlreadyExists", ErrAlreadyExists, "resource already exists"},
		{"ErrConflict", ErrConflict, "version conflict"},
		{"ErrInvalidArgument", ErrInvalidArgument, "invalid argument"},
		{"ErrPermissionDenied", ErrPermissionDenied, "permission denied"},
		{"ErrUnauthenticated", ErrUnauthenticated, "unauthenticated"},
		{"ErrPreconditionFailed", ErrPreconditionFailed, "precondition failed"},
		{"ErrResourceExhausted", ErrResourceExhausted, "resource exhausted"},
		{"ErrInternal", ErrInternal, "internal system error"},
		{"ErrTimeout", ErrTimeout, "operation timeout"},
		{"ErrUnavailable", ErrUnavailable, "service unavailable"},
		{"ErrDataCorruption", ErrDataCorruption, "data corruption detected"},
		{"ErrAggregateNotFound", ErrAggregateNotFound, "aggregate not found"},
		{"ErrEventStreamNotFound", ErrEventStreamNotFound, "event stream not found"},
		{"ErrConcurrencyConflict", ErrConcurrencyConflict, "concurrency conflict: aggregate version mismatch"},
		{"ErrInvalidVersion", ErrInvalidVersion, "invalid version"},
		{"ErrSnapshotNotFound", ErrSnapshotNotFound, "snapshot not found"},
		{"ErrUniqueConstraintViolation", ErrUniqueConstraintViolation, "unique constraint violation"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Error() != tt.msg {
				t.Errorf("expected error message %q, got %q", tt.msg, tt.err.Error())
			}
		})
	}
}

func TestIsApplicationError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"ErrNotFound", ErrNotFound, true},
		{"ErrAlreadyExists", ErrAlreadyExists, true},
		{"ErrConflict", ErrConflict, true},
		{"ErrInvalidArgument", ErrInvalidArgument, true},
		{"ErrPermissionDenied", ErrPermissionDenied, true},
		{"ErrUnauthenticated", ErrUnauthenticated, true},
		{"ErrPreconditionFailed", ErrPreconditionFailed, true},
		{"ErrResourceExhausted", ErrResourceExhausted, true},
		{"ErrAggregateNotFound", ErrAggregateNotFound, true},
		{"ErrEventStreamNotFound", ErrEventStreamNotFound, true},
		{"ErrConcurrencyConflict", ErrConcurrencyConflict, true},
		{"ErrInvalidVersion", ErrInvalidVersion, true},
		{"ErrSnapshotNotFound", ErrSnapshotNotFound, true},
		{"ErrUniqueConstraintViolation", ErrUniqueConstraintViolation, true},
		{"ErrInternal", ErrInternal, false},
		{"ErrTimeout", ErrTimeout, false},
		{"ErrUnavailable", ErrUnavailable, false},
		{"ErrDataCorruption", ErrDataCorruption, false},
		{"Generic Error", errors.New("generic error"), false},
		{"Nil Error", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsApplicationError(tt.err); got != tt.expected {
				t.Errorf("IsApplicationError() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestIsSystemError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"ErrInternal", ErrInternal, true},
		{"ErrTimeout", ErrTimeout, true},
		{"ErrUnavailable", ErrUnavailable, true},
		{"ErrDataCorruption", ErrDataCorruption, true},
		{"ErrNotFound", ErrNotFound, false},
		{"ErrAlreadyExists", ErrAlreadyExists, false},
		{"Generic Error", errors.New("generic error"), false},
		{"Nil Error", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsSystemError(tt.err); got != tt.expected {
				t.Errorf("IsSystemError() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"ErrConflict", ErrConflict, true},
		{"ErrConcurrencyConflict", ErrConcurrencyConflict, true},
		{"ErrTimeout", ErrTimeout, true},
		{"ErrUnavailable", ErrUnavailable, true},
		{"ErrResourceExhausted", ErrResourceExhausted, true},
		{"ErrInternal", ErrInternal, false},
		{"ErrNotFound", ErrNotFound, false},
		{"Generic Error", errors.New("generic error"), false},
		{"Nil Error", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsRetryable(tt.err); got != tt.expected {
				t.Errorf("IsRetryable() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestNotFoundError(t *testing.T) {
	err := NewNotFoundError("User", "123")

	if err.Error() != "User not found: 123" {
		t.Errorf("unexpected error message: %s", err.Error())
	}

	if !errors.Is(err, ErrNotFound) {
		t.Error("expected error to be ErrNotFound")
	}

	var notFoundErr *NotFoundError
	if !errors.As(err, &notFoundErr) {
		t.Error("expected error to be assignable to NotFoundError")
	}

	if notFoundErr.ResourceType != "User" || notFoundErr.ResourceID != "123" {
		t.Errorf("unexpected fields: %+v", notFoundErr)
	}
}

func TestConflictError(t *testing.T) {
	err := NewConflictError("agg-1", 1, 2)

	if err.Error() != "version conflict on aggregate agg-1: expected v1, got v2" {
		t.Errorf("unexpected error message: %s", err.Error())
	}

	if !errors.Is(err, ErrConflict) {
		t.Error("expected error to be ErrConflict")
	}
	if !errors.Is(err, ErrConcurrencyConflict) {
		t.Error("expected error to be ErrConcurrencyConflict")
	}

	var conflictErr *ConflictError
	if !errors.As(err, &conflictErr) {
		t.Error("expected error to be assignable to ConflictError")
	}
}

func TestUniqueConstraintError(t *testing.T) {
	err := NewUniqueConstraintError("email", "test@example.com", "user-1")

	if err.Error() != "unique constraint violation: email='test@example.com' is already claimed by aggregate user-1" {
		t.Errorf("unexpected error message: %s", err.Error())
	}

	if !errors.Is(err, ErrUniqueConstraintViolation) {
		t.Error("expected error to be ErrUniqueConstraintViolation")
	}

	var uniqueErr *UniqueConstraintError
	if !errors.As(err, &uniqueErr) {
		t.Error("expected error to be assignable to UniqueConstraintError")
	}
}

func TestValidationError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		msg  string
	}{
		{
			name: "with value",
			err:  NewValidationError("email", "invalid", "bad format"),
			msg:  "validation failed for email='invalid': bad format",
		},
		{
			name: "without value",
			err:  NewValidationError("password", "", "required"),
			msg:  "validation failed for password: required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Error() != tt.msg {
				t.Errorf("expected error message %q, got %q", tt.msg, tt.err.Error())
			}

			if !errors.Is(tt.err, ErrInvalidArgument) {
				t.Error("expected error to be ErrInvalidArgument")
			}
		})
	}
}

func TestWrap(t *testing.T) {
	original := errors.New("original error")
	wrapped := Wrap(original, "context")

	if wrapped.Error() != "context: original error" {
		t.Errorf("unexpected error message: %s", wrapped.Error())
	}

	if !errors.Is(wrapped, original) {
		t.Error("expected wrapped error to be original error")
	}

	if Wrap(nil, "context") != nil {
		t.Error("expected nil when wrapping nil error")
	}
}

func TestWrapf(t *testing.T) {
	original := errors.New("original error")
	wrapped := Wrapf(original, "context %d", 123)

	if wrapped.Error() != "context 123: original error" {
		t.Errorf("unexpected error message: %s", wrapped.Error())
	}

	if !errors.Is(wrapped, original) {
		t.Error("expected wrapped error to be original error")
	}

	if Wrapf(nil, "context %d", 123) != nil {
		t.Error("expected nil when wrapping nil error")
	}
}

func TestJoin(t *testing.T) {
	err1 := errors.New("error 1")
	err2 := errors.New("error 2")

	err := Join(err1, nil, err2)

	if err == nil {
		t.Fatal("expected joined error, got nil")
	}

	msg := err.Error()
	if !strings.Contains(msg, "error 1") || !strings.Contains(msg, "error 2") {
		t.Errorf("unexpected error message: %s", msg)
	}

	if !errors.Is(err, err1) {
		t.Error("expected joined error to contain err1")
	}
	if !errors.Is(err, err2) {
		t.Error("expected joined error to contain err2")
	}

	if Join(nil, nil) != nil {
		t.Error("expected nil when joining nil errors")
	}
}

func TestWithStack(t *testing.T) {
	original := errors.New("original error")
	err := WithStack(original)

	if err == nil {
		t.Fatal("expected error with stack, got nil")
	}

	if !errors.Is(err, original) {
		t.Error("expected wrapped error to be original error")
	}

	if WithStack(nil) != nil {
		t.Error("expected nil when wrapping nil error")
	}

	// Check stack trace formatting
	trace := fmt.Sprintf("%+v", err)
	if !strings.Contains(trace, "TestWithStack") {
		t.Error("stack trace should contain function name")
	}
	if !strings.Contains(trace, "errors_test.go") {
		t.Error("stack trace should contain file name")
	}

	// Check other formats
	if fmt.Sprintf("%s", err) != original.Error() {
		t.Errorf("expected %%s to format as original error")
	}
	if fmt.Sprintf("%v", err) != original.Error() {
		t.Errorf("expected %%v to format as original error")
	}
	if fmt.Sprintf("%q", err) != fmt.Sprintf("%q", original.Error()) {
		t.Errorf("expected %%q to format as quoted original error")
	}
}

func TestStackTrace(t *testing.T) {
	original := errors.New("original error")
	err := WithStack(original)

	trace := StackTrace(err)
	if trace == "" {
		t.Error("expected stack trace, got empty string")
	}

	if !strings.Contains(trace, "TestStackTrace") {
		t.Error("stack trace should contain function name")
	}

	// Test with wrapped error
	wrapped := Wrap(err, "wrapped")
	traceWrapped := StackTrace(wrapped)
	if traceWrapped == "" {
		t.Error("expected stack trace from wrapped error")
	}

	// Test with error without stack
	if StackTrace(original) != "" {
		t.Error("expected empty stack trace for error without stack")
	}
}
