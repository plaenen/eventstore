# datastarx - Error Handling & User Feedback for Datastar

A thin wrapper around the official [Datastar Go SDK](https://github.com/starfederation/datastar-go) that provides high-level error handling and user feedback helpers.

## Why datastarx?

While the official Datastar SDK provides excellent low-level SSE functionality, `datastarx` adds a layer specifically designed for user feedback:

- **Automatic error classification** (APPLICATION vs SYSTEM)
- **Integration with pkg/errorx** for intelligent error handling
- **Validation error helpers** for form feedback
- **Notification system** (success, info, warning, error)
- **Security-conscious** - sanitizes system errors automatically
- **UI-independent** - sends signals only, your frontend decides how to render

## Installation

```bash
go get github.com/plaenen/eventstore/pkg/datastarx
go get github.com/starfederation/datastar-go
```

## Quick Start

```go
import (
    "github.com/plaenen/eventstore/pkg/datastarx"
    "github.com/plaenen/eventstore/pkg/errorx"
    "github.com/starfederation/datastar-go/datastar"
)

http.HandleFunc("/api/users/:id", func(w http.ResponseWriter, r *http.Request) {
    // Create Datastar SSE writer (official SDK)
    sse := datastar.NewSSE(w, r)

    // Wrap with datastarx feedback helpers
    feedback := datastarx.NewFeedbackWriter(sse)

    user, err := userService.Load(id)
    if err != nil {
        // Automatically classifies and sends appropriate feedback:
        // - NotFoundError → "User not found" with helpful hint
        // - ConflictError → "Version conflict, please refresh"
        // - System errors → Sanitized "An error occurred"
        feedback.Error(r.Context(), err)
        return
    }

    feedback.Success("User Loaded", "User data loaded successfully")
})
```

## Key Features

### 1. Automatic Error Classification

Errors are automatically classified using `pkg/errorx`:

**APPLICATION Errors** (Expected - show full details):
```go
feedback.Error(ctx, errorx.NewNotFoundError("User", "123"))
// Sends: {type: "application", message: "User not found: 123", hint: "Please verify the ID..."}
```

**SYSTEM Errors** (Unexpected - sanitize details):
```go
feedback.Error(ctx, errorx.ErrInternal)
// Sends: {type: "system", message: "An unexpected error occurred. Please try again later"}
// ✅ No internal details exposed to the user!
```

### 2. Validation Error Handling

```go
validations := []datastarx.ValidationError{
    {Field: "email", Message: "Invalid email format"},
    {Field: "password", Message: "Password must be at least 8 characters"},
}

feedback.ValidationErrors(validations)
// Sends signal patch: {validationErrors: {email: "Invalid...", password: "Password..."}}
```

### 3. Notification System

```go
// Success
feedback.Success("Account Created", "Your account is ready")

// Info
feedback.Info("New Features", "Check out our new dashboard")

// Warning
feedback.Warning("Session Expiring", "Your session will expire in 5 minutes")

// Custom notification
feedback.Notification(datastarx.Notification{
    Level:   datastarx.NotificationLevelError,
    Title:   "Operation Failed",
    Message: "Could not process your request",
    Detail:  "Please try again later",
})
```

### 4. Direct SDK Access

Access the underlying Datastar SDK when you need it:

```go
// Use feedback helpers
feedback.Success("Account Created", "Success!")

// Use official SDK directly for DOM manipulation
feedback.SSE().PatchElements(
    "<div>Updated content</div>",
    datastar.WithSelector("#target"),
    datastar.WithMode(datastar.ElementPatchModeInner),
)

// Execute scripts
feedback.SSE().ExecuteScript("console.log('Server update')")

// Navigate
feedback.SSE().Redirect("/dashboard")
```

## API Reference

### FeedbackWriter

```go
type FeedbackWriter struct {
    // internal fields
}

// Create a new feedback writer wrapping the official Datastar SDK
func NewFeedbackWriter(sse *datastar.ServerSentEventGenerator) *FeedbackWriter

// Get the underlying Datastar SDK for direct access
func (w *FeedbackWriter) SSE() *datastar.ServerSentEventGenerator
```

#### User Feedback Methods

```go
// Send notification
func (w *FeedbackWriter) Notification(notification Notification) error

// Send field-level validation errors
func (w *FeedbackWriter) ValidationErrors(errs []ValidationError) error

// Clear validation errors
func (w *FeedbackWriter) ClearValidationErrors() error

// Automatic error classification and feedback
func (w *FeedbackWriter) Error(ctx context.Context, err error) error

// Error with validation details
func (w *FeedbackWriter) ErrorWithValidations(ctx context.Context, err error, validations []ValidationError) error

// Clear error state
func (w *FeedbackWriter) ClearError() error

// Clear all feedback (errors, validations, notifications)
func (w *FeedbackWriter) ClearAll() error

// Convenience methods
func (w *FeedbackWriter) Success(title, message string) error
func (w *FeedbackWriter) Info(title, message string) error
func (w *FeedbackWriter) Warning(title, message string) error
```

### Types

```go
type NotificationLevel string

const (
    NotificationLevelSuccess NotificationLevel = "success"
    NotificationLevelInfo    NotificationLevel = "info"
    NotificationLevelWarning NotificationLevel = "warning"
    NotificationLevelError   NotificationLevel = "error"
)

type Notification struct {
    Level   NotificationLevel `json:"level"`
    Message string            `json:"message"`
    Title   string            `json:"title,omitempty"`
    Detail  string            `json:"detail,omitempty"`
}

type ValidationError struct {
    Field   string `json:"field"`
    Message string `json:"message"`
}

type ErrorFeedback struct {
    Type         string            `json:"type"` // "application" or "system"
    Message      string            `json:"message"`
    Detail       string            `json:"detail,omitempty"`
    Hint         string            `json:"hint,omitempty"`
    Validations  []ValidationError `json:"validations,omitempty"`
    Notification *Notification     `json:"notification,omitempty"`
}
```

## Integration with pkg/errorx

The `datastarx` package is designed to work seamlessly with `pkg/errorx`:

```go
// In your command handler
func (h *Handler) CreateAccount(ctx context.Context, cmd CreateAccountCommand) error {
    // Domain validation
    if err := cmd.Validate(); err != nil {
        return errorx.NewValidationError("email", cmd.Email, "invalid format")
    }

    // Business logic
    if exists, err := h.repo.Exists(cmd.AccountID); err != nil {
        return err // System error - will be sanitized
    } else if exists {
        return errorx.ErrAlreadyExists // Application error - user-friendly
    }

    return h.repo.Save(aggregate)
}

// In your HTTP handler
http.HandleFunc("/api/accounts", func(w http.ResponseWriter, r *http.Request) {
    sse := datastar.NewSSE(w, r)
    feedback := datastarx.NewFeedbackWriter(sse)

    err := handler.CreateAccount(r.Context(), cmd)
    if err != nil {
        // Automatically handles all error types correctly
        feedback.Error(r.Context(), err)
        return
    }

    feedback.Success("Account Created", "Your account is ready")
})
```

## UI Independence

The `datastarx` package is **UI-independent**. It only sends signals via SSE - your frontend decides how to render them:

```html
<!-- Example Alpine.js/Datastar integration -->
<div x-data="{
  notification: null,
  error: null,
  validationErrors: {}
}">
  <!-- Your custom toast component -->
  <div x-show="notification" class="my-toast-component">
    <h3 x-text="notification?.title"></h3>
    <p x-text="notification?.message"></p>
  </div>

  <!-- Form with inline errors -->
  <form>
    <input name="email" />
    <span x-show="validationErrors.email"
          x-text="validationErrors.email"
          class="error-message"></span>
  </form>

  <!-- Error banner -->
  <div x-show="error?.type === 'system'" class="error-banner">
    <p x-text="error?.message"></p>
    <p x-text="error?.hint" class="hint"></p>
  </div>
</div>
```

## Best Practices

### 1. Always Clear Feedback on Success

```go
if err != nil {
    feedback.Error(ctx, err)
    return
}

// Clear previous errors on success
feedback.ClearAll()
feedback.Success("Operation Complete", "Success!")
```

### 2. Use Specific Error Types

```go
// Good - provides helpful feedback
return errorx.NewNotFoundError("User", userID)

// Less helpful
return fmt.Errorf("user not found")
```

### 3. Combine Validations with Errors

```go
validations := validateInput(input)
if len(validations) > 0 {
    feedback.ErrorWithValidations(ctx, errorx.ErrInvalidArgument, validations)
    return
}
```

### 4. Leverage the Official SDK

```go
// Use feedback for user messaging
feedback.Success("Updated", "Changes saved")

// Use official SDK for DOM updates
feedback.SSE().PatchElements(
    "<div>New content</div>",
    datastar.WithSelector("#content"),
)
```

## Error Classification Matrix

| Error Type | Classification | Message Style | Example |
|------------|---------------|---------------|---------|
| `NotFoundError` | APPLICATION | Full details + hint | "User not found: 123. Please verify the ID is correct" |
| `ConflictError` | APPLICATION | Full details + hint | "Version conflict on aggregate abc. Please refresh and try again" |
| `UniqueConstraintError` | APPLICATION | Full details + hint | "Email already in use. Please use a different value" |
| `ErrInvalidArgument` | APPLICATION | Full details + hint | "Invalid input. Please check your data" |
| `ErrPermissionDenied` | APPLICATION | Full details + hint | "You don't have permission. Contact your administrator" |
| `ErrInternal` | SYSTEM | **Sanitized** | "An unexpected error occurred. Please try again later" |
| `ErrTimeout` | SYSTEM | **Sanitized** | "An unexpected error occurred. Please try again later" |
| Unknown errors | SYSTEM | **Sanitized** | "An unexpected error occurred. Please try again later" |

## Comparison: Official SDK vs datastarx

**Official Datastar SDK** provides:
- Low-level SSE event streaming
- DOM manipulation (PatchElements, RemoveElement)
- Signal patching (PatchSignals)
- Script execution (ExecuteScript, ConsoleLog, etc.)
- Navigation (Redirect, ReplaceURL)
- Custom events (DispatchCustomEvent)

**datastarx** adds:
- ✅ Automatic error classification (APPLICATION vs SYSTEM)
- ✅ Security-conscious error sanitization
- ✅ Validation error helpers
- ✅ Notification system
- ✅ Integration with pkg/errorx
- ✅ Still provides full access to the official SDK

## Related Packages

- [Official Datastar Go SDK](https://github.com/starfederation/datastar-go) - Low-level SSE functionality
- [Datastar Documentation](https://data-star.dev/) - Frontend framework documentation
- `pkg/errorx` - Error classification and handling

## License

Same as parent repository.
