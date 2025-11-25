package logging

import (
	"context"
	stderrors "errors"
	"log/slog"

	"github.com/plaenen/eventstore/pkg/errorx"
	"github.com/plaenen/eventstore/pkg/protocol"
)

// ErrorLogger provides structured error logging with automatic classification.
//
// This utility integrates with pkg/errorx to provide appropriate log levels
// and context based on error type:
//
// - APPLICATION errors (ErrNotFound, ErrConflict, etc.) → slog.Warn
// - SYSTEM errors (ErrInternal, ErrTimeout, etc.) → slog.Error
// - Unknown errors → slog.Error
//
// Usage:
//
//	logger := logging.NewErrorLogger(slog.Default())
//	if err := repository.Save(aggregate); err != nil {
//	    logger.LogError(ctx, "failed to save aggregate",
//	        "aggregate_id", aggregate.ID,
//	        "error", err,
//	    )
//	}
type ErrorLogger struct {
	logger *slog.Logger
}

// NewErrorLogger creates a new ErrorLogger with the given slog.Logger.
func NewErrorLogger(logger *slog.Logger) *ErrorLogger {
	return &ErrorLogger{logger: logger}
}

// LogError logs an error with automatic classification and appropriate level.
//
// Log Levels:
// - APPLICATION errors → Warn (expected business logic failures)
// - SYSTEM errors → Error (unexpected infrastructure failures)
// - Unknown errors → Error (assume worst case)
//
// Example:
//
//	logger.LogError(ctx, "failed to process command",
//	    "command_id", cmd.ID,
//	    "aggregate_id", aggregate.ID,
//	    "error", err,
//	)
func (l *ErrorLogger) LogError(ctx context.Context, msg string, args ...any) {
	// Extract error from args
	err := extractError(args)
	if err == nil {
		l.logger.WarnContext(ctx, msg, args...)
		return
	}

	// Add error classification metadata
	args = append(args,
		"error_type", classifyError(err),
		"retryable", errorx.IsRetryable(err),
	)

	// Choose log level based on error type
	if errorx.IsApplicationError(err) {
		l.logger.WarnContext(ctx, msg, args...)
	} else {
		l.logger.ErrorContext(ctx, msg, args...)
	}
}

// LogApplicationError logs an expected application error (business logic failure).
//
// Uses Warn level since these are expected failures that don't indicate system issues.
//
// Example:
//
//	logger.LogApplicationError(ctx, "aggregate not found",
//	    "aggregate_id", id,
//	    "error", err,
//	)
func (l *ErrorLogger) LogApplicationError(ctx context.Context, msg string, args ...any) {
	l.logger.WarnContext(ctx, msg, args...)
}

// LogSystemError logs an unexpected system error (infrastructure failure).
//
// Uses Error level since these indicate infrastructure issues requiring attention.
//
// Example:
//
//	logger.LogSystemError(ctx, "database connection failed",
//	    "error", err,
//	)
func (l *ErrorLogger) LogSystemError(ctx context.Context, msg string, args ...any) {
	l.logger.ErrorContext(ctx, msg, args...)
}

// LogWithDetails logs an error with extracted details from structured error types.
//
// This extracts additional context from error types like:
// - NotFoundError: resource type, ID
// - ConflictError: expected/actual versions
// - UniqueConstraintError: field, value
// - ValidationError: field, message
//
// Example:
//
//	logger.LogWithDetails(ctx, "operation failed", err)
func (l *ErrorLogger) LogWithDetails(ctx context.Context, msg string, err error) {
	if err == nil {
		return
	}

	args := []any{"error", err}

	// Extract details from structured error types
	var notFoundErr *errorx.NotFoundError
	var conflictErr *errorx.ConflictError
	var uniqueErr *errorx.UniqueConstraintError
	var validationErr *errorx.ValidationError

	switch {
	case stderrors.As(err, &notFoundErr):
		args = append(args,
			"resource_type", notFoundErr.ResourceType,
			"resource_id", notFoundErr.ResourceID,
		)

	case stderrors.As(err, &conflictErr):
		args = append(args,
			"aggregate_id", conflictErr.AggregateID,
			"expected_version", conflictErr.ExpectedVersion,
			"actual_version", conflictErr.ActualVersion,
		)

	case stderrors.As(err, &uniqueErr):
		args = append(args,
			"index_name", uniqueErr.IndexName,
			"value", uniqueErr.Value,
		)

	case stderrors.As(err, &validationErr):
		args = append(args,
			"field", validationErr.Field,
			"validation_message", validationErr.Message,
		)
	}

	// Check for protocol.AppError
	var appErr *protocol.AppError
	if stderrors.As(err, &appErr) {
		args = append(args,
			"error_code", appErr.Code,
			"error_details", appErr.Details,
		)
	}

	l.LogError(ctx, msg, args...)
}

// extractError finds the first error in args slice
func extractError(args []any) error {
	for i := 0; i < len(args); i++ {
		if err, ok := args[i].(error); ok {
			return err
		}
	}
	return nil
}

// classifyError returns a string classification of the error
func classifyError(err error) string {
	if errorx.IsApplicationError(err) {
		return "APPLICATION"
	}
	if errorx.IsSystemError(err) {
		return "SYSTEM"
	}
	return "UNKNOWN"
}

// DefaultLogger is a convenience instance using slog.Default().
var DefaultLogger = NewErrorLogger(slog.Default())

// LogError logs an error using the default logger.
func LogError(ctx context.Context, msg string, args ...any) {
	DefaultLogger.LogError(ctx, msg, args...)
}

// LogApplicationError logs an application error using the default logger.
func LogApplicationError(ctx context.Context, msg string, args ...any) {
	DefaultLogger.LogApplicationError(ctx, msg, args...)
}

// LogSystemError logs a system error using the default logger.
func LogSystemError(ctx context.Context, msg string, args ...any) {
	DefaultLogger.LogSystemError(ctx, msg, args...)
}

// LogWithDetails logs an error with details using the default logger.
func LogWithDetails(ctx context.Context, msg string, err error) {
	DefaultLogger.LogWithDetails(ctx, msg, err)
}
