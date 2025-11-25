package datastarx

import (
	"bytes"
	"context"
	stderrors "errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/plaenen/eventstore/pkg/errorx"
	"github.com/starfederation/datastar-go/datastar"
)

func TestNewFeedbackWriter(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	sse := datastar.NewSSE(w, r)

	feedback := NewFeedbackWriter(sse)

	if feedback == nil {
		t.Fatal("NewFeedbackWriter() returned nil")
	}
	if feedback.sse != sse {
		t.Error("FeedbackWriter.sse not set correctly")
	}
}

func TestSSE(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	sse := datastar.NewSSE(w, r)

	feedback := NewFeedbackWriter(sse)

	if feedback.SSE() != sse {
		t.Error("SSE() did not return the underlying SSE generator")
	}
}

func TestNotification(t *testing.T) {
	tests := []struct {
		name         string
		notification Notification
	}{
		{
			name: "success notification",
			notification: Notification{
				Level:   NotificationLevelSuccess,
				Title:   "Account Created",
				Message: "Your account has been created successfully",
			},
		},
		{
			name: "error notification",
			notification: Notification{
				Level:   NotificationLevelError,
				Title:   "Operation Failed",
				Message: "Could not process your request",
				Detail:  "Invalid input provided",
			},
		},
		{
			name: "info notification",
			notification: Notification{
				Level:   NotificationLevelInfo,
				Message: "New features available",
			},
		},
		{
			name: "warning notification",
			notification: Notification{
				Level:   NotificationLevelWarning,
				Title:   "Warning",
				Message: "Your session will expire soon",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest("GET", "/", nil)
			sse := datastar.NewSSE(w, r)
			feedback := NewFeedbackWriter(sse)

			err := feedback.Notification(tt.notification)
			if err != nil {
				t.Fatalf("Notification() error = %v", err)
			}

			output := w.Body.String()
			if !strings.Contains(output, "datastar-patch-signals") {
				t.Error("Output missing signal patch event type")
			}
			if !strings.Contains(output, "notification") {
				t.Error("Output missing notification key")
			}
		})
	}
}

func TestValidationErrors(t *testing.T) {
	tests := []struct {
		name   string
		errors []ValidationError
	}{
		{
			name: "single validation error",
			errors: []ValidationError{
				{Field: "email", Message: "Invalid email format"},
			},
		},
		{
			name: "multiple validation errors",
			errors: []ValidationError{
				{Field: "email", Message: "Invalid email format"},
				{Field: "password", Message: "Password must be at least 8 characters"},
				{Field: "username", Message: "Username is required"},
			},
		},
		{
			name:   "empty validation errors",
			errors: []ValidationError{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest("GET", "/", nil)
			sse := datastar.NewSSE(w, r)
			feedback := NewFeedbackWriter(sse)

			err := feedback.ValidationErrors(tt.errors)
			if err != nil {
				t.Fatalf("ValidationErrors() error = %v", err)
			}

			output := w.Body.String()
			if !strings.Contains(output, "datastar-patch-signals") {
				t.Error("Output missing signal patch event type")
			}
			if !strings.Contains(output, "validationErrors") {
				t.Error("Output missing validationErrors key")
			}
		})
	}
}

func TestClearValidationErrors(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	sse := datastar.NewSSE(w, r)
	feedback := NewFeedbackWriter(sse)

	err := feedback.ClearValidationErrors()
	if err != nil {
		t.Fatalf("ClearValidationErrors() error = %v", err)
	}

	output := w.Body.String()
	if !strings.Contains(output, "datastar-patch-signals") {
		t.Error("Output missing signal patch event type")
	}
	if !strings.Contains(output, "validationErrors") {
		t.Error("Output missing validationErrors key")
	}
}

func TestError_NotFound(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	sse := datastar.NewSSE(w, r)
	feedback := NewFeedbackWriter(sse)

	err := errorx.NewNotFoundError("User", "123")
	if err := feedback.Error(context.Background(), err); err != nil {
		t.Fatalf("Error() error = %v", err)
	}

	output := w.Body.String()

	// Should have both notification and error signals
	eventCount := strings.Count(output, "event:")
	if eventCount < 2 {
		t.Errorf("Expected at least 2 SSE events (notification + error), got %d", eventCount)
	}

	// Verify signals were patched
	if !strings.Contains(output, "notification") {
		t.Error("Output missing notification")
	}

	if !strings.Contains(output, "error") {
		t.Error("Output missing error signal")
	}

	if !strings.Contains(output, "application") {
		t.Error("Output should classify as application error")
	}
}

func TestError_Conflict(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	sse := datastar.NewSSE(w, r)
	feedback := NewFeedbackWriter(sse)

	err := errorx.NewConflictError("agg-123", 5, 7)
	if err := feedback.Error(context.Background(), err); err != nil {
		t.Fatalf("Error() error = %v", err)
	}

	output := w.Body.String()

	if !strings.Contains(output, "application") {
		t.Error("Conflict should be classified as application error")
	}

	if !strings.Contains(output, "Conflict") {
		t.Error("Output should include conflict notification")
	}
}

func TestError_UniqueConstraint(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	sse := datastar.NewSSE(w, r)
	feedback := NewFeedbackWriter(sse)

	err := errorx.NewUniqueConstraintError("email", "john@example.com", "user-456")
	if err := feedback.Error(context.Background(), err); err != nil {
		t.Fatalf("Error() error = %v", err)
	}

	output := w.Body.String()

	if !strings.Contains(output, "application") {
		t.Error("Unique constraint should be classified as application error")
	}

	if !strings.Contains(output, "Duplicate Entry") {
		t.Error("Output should include duplicate entry notification")
	}
}

func TestError_PermissionDenied(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	sse := datastar.NewSSE(w, r)
	feedback := NewFeedbackWriter(sse)

	err := errorx.ErrPermissionDenied
	if err := feedback.Error(context.Background(), err); err != nil {
		t.Fatalf("Error() error = %v", err)
	}

	output := w.Body.String()

	if !strings.Contains(output, "application") {
		t.Error("Permission denied should be classified as application error")
	}

	if !strings.Contains(output, "Permission Denied") {
		t.Error("Output should include permission denied notification")
	}
}

func TestError_InvalidArgument(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	sse := datastar.NewSSE(w, r)
	feedback := NewFeedbackWriter(sse)

	err := errorx.ErrInvalidArgument
	if err := feedback.Error(context.Background(), err); err != nil {
		t.Fatalf("Error() error = %v", err)
	}

	output := w.Body.String()

	if !strings.Contains(output, "application") {
		t.Error("Invalid argument should be classified as application error")
	}

	if !strings.Contains(output, "Invalid Input") {
		t.Error("Output should include invalid input notification")
	}
}

func TestError_AlreadyExists(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	sse := datastar.NewSSE(w, r)
	feedback := NewFeedbackWriter(sse)

	err := errorx.ErrAlreadyExists
	if err := feedback.Error(context.Background(), err); err != nil {
		t.Fatalf("Error() error = %v", err)
	}

	output := w.Body.String()

	if !strings.Contains(output, "application") {
		t.Error("Already exists should be classified as application error")
	}

	if !strings.Contains(output, "Already Exists") {
		t.Error("Output should include already exists notification")
	}
}

func TestError_SystemError(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	sse := datastar.NewSSE(w, r)
	feedback := NewFeedbackWriter(sse)

	// System error (unexpected)
	err := errorx.ErrInternal
	if err := feedback.Error(context.Background(), err); err != nil {
		t.Fatalf("Error() error = %v", err)
	}

	output := w.Body.String()

	// Should be classified as system error
	if !strings.Contains(output, "system") {
		t.Error("Internal error should be classified as system error")
	}

	// Should have sanitized message (not expose internals)
	if !strings.Contains(output, "unexpected error occurred") {
		t.Error("System error should have sanitized message")
	}

	if !strings.Contains(output, "System Error") {
		t.Error("Output should include system error notification")
	}
}

func TestError_UnknownError(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	sse := datastar.NewSSE(w, r)
	feedback := NewFeedbackWriter(sse)

	// Unknown error (not from errorx package)
	err := stderrors.New("some random error")
	if err := feedback.Error(context.Background(), err); err != nil {
		t.Fatalf("Error() error = %v", err)
	}

	output := w.Body.String()

	// Unknown errors should be treated as system errors (sanitized)
	if !strings.Contains(output, "system") {
		t.Error("Unknown error should be classified as system error")
	}

	if !strings.Contains(output, "unexpected error occurred") {
		t.Error("Unknown error should have sanitized message")
	}
}

func TestError_NilError(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	sse := datastar.NewSSE(w, r)
	feedback := NewFeedbackWriter(sse)

	err := feedback.Error(context.Background(), nil)
	if err != nil {
		t.Errorf("Error(nil) error = %v, want nil", err)
	}

	// Should not write anything for nil error
	output := w.Body.String()
	if len(output) > 0 {
		t.Error("Error(nil) should not write anything")
	}
}

func TestSuccess(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	sse := datastar.NewSSE(w, r)
	feedback := NewFeedbackWriter(sse)

	err := feedback.Success("Operation Complete", "The operation completed successfully")
	if err != nil {
		t.Fatalf("Success() error = %v", err)
	}

	output := w.Body.String()

	if !strings.Contains(output, string(NotificationLevelSuccess)) {
		t.Error("Output should contain success level")
	}

	if !strings.Contains(output, "Operation Complete") {
		t.Error("Output missing title")
	}
}

func TestInfo(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	sse := datastar.NewSSE(w, r)
	feedback := NewFeedbackWriter(sse)

	err := feedback.Info("Information", "Here is some info")
	if err != nil {
		t.Fatalf("Info() error = %v", err)
	}

	output := w.Body.String()

	if !strings.Contains(output, string(NotificationLevelInfo)) {
		t.Error("Output should contain info level")
	}
}

func TestWarning(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	sse := datastar.NewSSE(w, r)
	feedback := NewFeedbackWriter(sse)

	err := feedback.Warning("Warning", "Please be careful")
	if err != nil {
		t.Fatalf("Warning() error = %v", err)
	}

	output := w.Body.String()

	if !strings.Contains(output, string(NotificationLevelWarning)) {
		t.Error("Output should contain warning level")
	}
}

func TestErrorWithValidations(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	sse := datastar.NewSSE(w, r)
	feedback := NewFeedbackWriter(sse)

	validations := []ValidationError{
		{Field: "email", Message: "Invalid format"},
		{Field: "password", Message: "Too short"},
	}

	err := feedback.ErrorWithValidations(
		context.Background(),
		errorx.ErrInvalidArgument,
		validations,
	)
	if err != nil {
		t.Fatalf("ErrorWithValidations() error = %v", err)
	}

	output := w.Body.String()

	// Should include validation errors
	if !strings.Contains(output, "validationErrors") {
		t.Error("Output missing validation errors")
	}

	// Should include error feedback
	if !strings.Contains(output, "application") {
		t.Error("Output missing error feedback")
	}

	// Should include notification
	if !strings.Contains(output, "notification") {
		t.Error("Output missing notification")
	}
}

func TestClearError(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	sse := datastar.NewSSE(w, r)
	feedback := NewFeedbackWriter(sse)

	err := feedback.ClearError()
	if err != nil {
		t.Fatalf("ClearError() error = %v", err)
	}

	output := w.Body.String()

	if !strings.Contains(output, "error") {
		t.Error("Output should reference error key")
	}
}

func TestClearAll(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	sse := datastar.NewSSE(w, r)
	feedback := NewFeedbackWriter(sse)

	err := feedback.ClearAll()
	if err != nil {
		t.Fatalf("ClearAll() error = %v", err)
	}

	output := w.Body.String()

	// Should clear all feedback signals
	if !strings.Contains(output, "error") {
		t.Error("Output should clear error")
	}
	if !strings.Contains(output, "validationErrors") {
		t.Error("Output should clear validationErrors")
	}
	if !strings.Contains(output, "notification") {
		t.Error("Output should clear notification")
	}
}

func TestClassifyError_Coverage(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		expectedType string
	}{
		{
			name:         "not found",
			err:          errorx.NewNotFoundError("User", "123"),
			expectedType: "application",
		},
		{
			name:         "conflict",
			err:          errorx.NewConflictError("agg-123", 5, 7),
			expectedType: "application",
		},
		{
			name:         "unique constraint",
			err:          errorx.NewUniqueConstraintError("email", "test@example.com", "user-1"),
			expectedType: "application",
		},
		{
			name:         "already exists",
			err:          errorx.ErrAlreadyExists,
			expectedType: "application",
		},
		{
			name:         "permission denied",
			err:          errorx.ErrPermissionDenied,
			expectedType: "application",
		},
		{
			name:         "invalid argument",
			err:          errorx.ErrInvalidArgument,
			expectedType: "application",
		},
		{
			name:         "aggregate not found",
			err:          errorx.ErrAggregateNotFound,
			expectedType: "application",
		},
		{
			name:         "internal error",
			err:          errorx.ErrInternal,
			expectedType: "system",
		},
		{
			name:         "timeout",
			err:          errorx.ErrTimeout,
			expectedType: "system",
		},
		{
			name:         "unknown error",
			err:          stderrors.New("random error"),
			expectedType: "system",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			feedback := classifyError(tt.err)

			if feedback.Type != tt.expectedType {
				t.Errorf("classifyError() type = %v, want %v", feedback.Type, tt.expectedType)
			}

			if feedback.Message == "" {
				t.Error("classifyError() should always return a message")
			}

			if feedback.Notification == nil {
				t.Error("classifyError() should always return a notification")
			}
		})
	}
}

// Benchmark tests
func BenchmarkNotification(b *testing.B) {
	w := &bytes.Buffer{}
	r := httptest.NewRequest("GET", "/", nil)
	sse := datastar.NewSSE(httptest.NewRecorder(), r)
	feedback := NewFeedbackWriter(sse)

	notification := Notification{
		Level:   NotificationLevelSuccess,
		Title:   "Success",
		Message: "Operation completed",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.Reset()
		_ = feedback.Notification(notification)
	}
}

func BenchmarkError(b *testing.B) {
	w := &bytes.Buffer{}
	r := httptest.NewRequest("GET", "/", nil)
	sse := datastar.NewSSE(httptest.NewRecorder(), r)
	feedback := NewFeedbackWriter(sse)
	ctx := context.Background()

	err := errorx.NewNotFoundError("User", "123")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.Reset()
		_ = feedback.Error(ctx, err)
	}
}

func BenchmarkClassifyError(b *testing.B) {
	err := errorx.NewConflictError("agg-123", 5, 7)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = classifyError(err)
	}
}
