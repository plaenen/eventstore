package datastarx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/plaenen/eventstore/pkg/errorx"
	"github.com/starfederation/datastar-go/datastar"
)

// Notification levels for user feedback
type NotificationLevel string

const (
	NotificationLevelSuccess NotificationLevel = "success"
	NotificationLevelInfo    NotificationLevel = "info"
	NotificationLevelWarning NotificationLevel = "warning"
	NotificationLevelError   NotificationLevel = "error"
)

// Notification represents a user notification/toast message.
type Notification struct {
	Level   NotificationLevel `json:"level"`
	Message string            `json:"message"`
	Title   string            `json:"title,omitempty"`
	Detail  string            `json:"detail,omitempty"`
}

// ValidationError represents a field-level validation error for forms.
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ErrorFeedback represents UI feedback for an error.
type ErrorFeedback struct {
	Type         string            `json:"type"` // "application" or "system"
	Message      string            `json:"message"`
	Detail       string            `json:"detail,omitempty"`
	Hint         string            `json:"hint,omitempty"`
	Validations  []ValidationError `json:"validations,omitempty"`
	Notification *Notification     `json:"notification,omitempty"`
}

// FeedbackWriter wraps the official Datastar SDK to provide high-level
// error handling and user feedback helpers.
//
// This is a thin wrapper around datastar.ServerSentEventGenerator that adds:
// - Automatic error classification (APPLICATION vs SYSTEM)
// - Validation error handling
// - Notification system
// - Integration with pkg/errorx
type FeedbackWriter struct {
	sse *datastar.ServerSentEventGenerator
}

// NewFeedbackWriter creates a new feedback writer that wraps the official Datastar SDK.
//
// Example:
//
//	http.HandleFunc("/api/users", func(w http.ResponseWriter, r *http.Request) {
//	    sse := datastar.NewSSE(w, r)
//	    feedback := datastarx.NewFeedbackWriter(sse)
//
//	    user, err := service.LoadUser(id)
//	    if err != nil {
//	        feedback.Error(r.Context(), err)
//	        return
//	    }
//
//	    feedback.Success("User Loaded", "User data loaded successfully")
//	})
func NewFeedbackWriter(sse *datastar.ServerSentEventGenerator) *FeedbackWriter {
	return &FeedbackWriter{sse: sse}
}

// SSE returns the underlying Datastar SSE generator for direct access.
//
// Use this when you need to use Datastar features directly:
//
//	feedback.SSE().PatchElements("<div>content</div>", datastar.WithSelector("#target"))
//	feedback.SSE().ExecuteScript("console.log('hello')")
func (w *FeedbackWriter) SSE() *datastar.ServerSentEventGenerator {
	return w.sse
}

// Notification sends a notification to be displayed to the user.
//
// This updates the client's notification store via a signal patch.
//
// Example:
//
//	feedback.Notification(Notification{
//	    Level:   NotificationLevelSuccess,
//	    Title:   "Account Created",
//	    Message: "Your account has been created successfully",
//	})
func (w *FeedbackWriter) Notification(notification Notification) error {
	return w.patchSignal("notification", notification)
}

// ValidationErrors sends field-level validation errors to the client.
//
// This updates the client's validation store via a signal patch.
//
// Example:
//
//	feedback.ValidationErrors([]ValidationError{
//	    {Field: "email", Message: "Invalid email format"},
//	    {Field: "password", Message: "Password must be at least 8 characters"},
//	})
func (w *FeedbackWriter) ValidationErrors(errs []ValidationError) error {
	// Convert to map for easier consumption on the client
	errorsMap := make(map[string]string)
	for _, e := range errs {
		errorsMap[e.Field] = e.Message
	}

	return w.patchSignal("validationErrors", errorsMap)
}

// ClearValidationErrors clears all validation errors from the client store.
func (w *FeedbackWriter) ClearValidationErrors() error {
	return w.patchSignal("validationErrors", map[string]string{})
}

// Error sends error feedback to the client based on the error type.
//
// This automatically classifies the error as APPLICATION or SYSTEM using
// pkg/errorx and provides appropriate user feedback:
//
// - APPLICATION errors: Full details with helpful hints
// - SYSTEM errors: Sanitized message (no internal details exposed)
//
// Example:
//
//	if err := service.CreateAccount(ctx, cmd); err != nil {
//	    feedback.Error(ctx, err)
//	    return
//	}
func (w *FeedbackWriter) Error(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}

	feedback := classifyError(err)

	// Send notification for user awareness
	if feedback.Notification != nil {
		if err := w.Notification(*feedback.Notification); err != nil {
			return fmt.Errorf("send error notification: %w", err)
		}
	}

	// Send detailed feedback to error store
	return w.patchSignal("error", feedback)
}

// Success sends a success notification to the client.
//
// Example:
//
//	feedback.Success("Account Created", "Your account has been created successfully")
func (w *FeedbackWriter) Success(title, message string) error {
	return w.Notification(Notification{
		Level:   NotificationLevelSuccess,
		Title:   title,
		Message: message,
	})
}

// Info sends an info notification to the client.
func (w *FeedbackWriter) Info(title, message string) error {
	return w.Notification(Notification{
		Level:   NotificationLevelInfo,
		Title:   title,
		Message: message,
	})
}

// Warning sends a warning notification to the client.
func (w *FeedbackWriter) Warning(title, message string) error {
	return w.Notification(Notification{
		Level:   NotificationLevelWarning,
		Title:   title,
		Message: message,
	})
}

// ErrorWithValidations sends both error feedback and validation errors.
//
// This is useful when you have both general error information and
// specific field-level validation errors.
//
// Example:
//
//	validationErrs := []ValidationError{
//	    {Field: "email", Message: "Invalid email format"},
//	    {Field: "password", Message: "Too short"},
//	}
//	feedback.ErrorWithValidations(ctx, err, validationErrs)
func (w *FeedbackWriter) ErrorWithValidations(ctx context.Context, err error, validations []ValidationError) error {
	// Send validation errors first
	if len(validations) > 0 {
		if err := w.ValidationErrors(validations); err != nil {
			return fmt.Errorf("send validation errors: %w", err)
		}
	}

	// Send general error feedback
	feedback := classifyError(err)
	feedback.Validations = validations

	// Send notification
	if feedback.Notification != nil {
		if err := w.Notification(*feedback.Notification); err != nil {
			return fmt.Errorf("send error notification: %w", err)
		}
	}

	return w.patchSignal("error", feedback)
}

// ClearError clears the error state on the client.
func (w *FeedbackWriter) ClearError() error {
	return w.patchSignal("error", nil)
}

// ClearAll clears all feedback (errors, validations, notifications) from the client.
func (w *FeedbackWriter) ClearAll() error {
	signals := map[string]interface{}{
		"error":            nil,
		"validationErrors": map[string]string{},
		"notification":     nil,
	}

	data, err := json.Marshal(signals)
	if err != nil {
		return fmt.Errorf("marshal signals: %w", err)
	}

	return w.sse.PatchSignals(data)
}

// patchSignal is a helper to patch a single signal using the official SDK.
func (w *FeedbackWriter) patchSignal(key string, value interface{}) error {
	signal := map[string]interface{}{key: value}

	data, err := json.Marshal(signal)
	if err != nil {
		return fmt.Errorf("marshal signal: %w", err)
	}

	return w.sse.PatchSignals(data)
}

// classifyError converts an error into ErrorFeedback based on its type.
func classifyError(err error) ErrorFeedback {
	// Check for specific error types

	// Not Found Error
	var notFoundErr *errorx.NotFoundError
	if errors.As(err, &notFoundErr) {
		return ErrorFeedback{
			Type:    "application",
			Message: notFoundErr.Error(),
			Hint:    "Please verify the ID is correct and try again",
			Notification: &Notification{
				Level:   NotificationLevelWarning,
				Title:   "Not Found",
				Message: fmt.Sprintf("%s not found", notFoundErr.ResourceType),
			},
		}
	}

	// Conflict Error (Optimistic Locking)
	var conflictErr *errorx.ConflictError
	if errors.As(err, &conflictErr) {
		return ErrorFeedback{
			Type:    "application",
			Message: conflictErr.Error(),
			Hint:    "The resource was modified by another user. Please refresh and try again",
			Notification: &Notification{
				Level:   NotificationLevelWarning,
				Title:   "Conflict",
				Message: "This resource was modified by another user",
			},
		}
	}

	// Unique Constraint Error
	var uniqueErr *errorx.UniqueConstraintError
	if errors.As(err, &uniqueErr) {
		return ErrorFeedback{
			Type:    "application",
			Message: uniqueErr.Error(),
			Detail:  fmt.Sprintf("A record with this %s already exists", uniqueErr.IndexName),
			Hint:    "Please use a different value",
			Notification: &Notification{
				Level:   NotificationLevelError,
				Title:   "Duplicate Entry",
				Message: fmt.Sprintf("This %s is already in use", uniqueErr.IndexName),
			},
		}
	}

	// Already Exists Error
	if errors.Is(err, errorx.ErrAlreadyExists) {
		return ErrorFeedback{
			Type:    "application",
			Message: err.Error(),
			Hint:    "Please use a different identifier",
			Notification: &Notification{
				Level:   NotificationLevelWarning,
				Title:   "Already Exists",
				Message: "This resource already exists",
			},
		}
	}

	// Permission Denied Error
	if errors.Is(err, errorx.ErrPermissionDenied) {
		return ErrorFeedback{
			Type:    "application",
			Message: "You don't have permission to perform this action",
			Hint:    "Contact your administrator if you believe this is incorrect",
			Notification: &Notification{
				Level:   NotificationLevelError,
				Title:   "Permission Denied",
				Message: "You don't have permission to perform this action",
			},
		}
	}

	// Invalid Argument Error
	if errors.Is(err, errorx.ErrInvalidArgument) {
		return ErrorFeedback{
			Type:    "application",
			Message: err.Error(),
			Hint:    "Please check your input and try again",
			Notification: &Notification{
				Level:   NotificationLevelError,
				Title:   "Invalid Input",
				Message: "Please check your input and try again",
			},
		}
	}

	// Application Error (Expected)
	if errorx.IsApplicationError(err) {
		return ErrorFeedback{
			Type:    "application",
			Message: err.Error(),
			Notification: &Notification{
				Level:   NotificationLevelWarning,
				Title:   "Operation Failed",
				Message: err.Error(),
			},
		}
	}

	// System Error (Unexpected) - Sanitize details
	return ErrorFeedback{
		Type:    "system",
		Message: "An unexpected error occurred. Please try again later",
		Detail:  "Our team has been notified and is working on a fix",
		Hint:    "If the problem persists, please contact support",
		Notification: &Notification{
			Level:   NotificationLevelError,
			Title:   "System Error",
			Message: "An unexpected error occurred. Please try again later",
		},
	}
}
