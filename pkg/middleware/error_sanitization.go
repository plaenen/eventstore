package middleware

import (
	"context"
	"log/slog"

	"github.com/plaenen/eventstore/pkg/eventsourcing"
	"github.com/plaenen/eventstore/pkg/security"
)

// ErrorSanitizationMiddleware sanitizes errors returned from command handlers.
//
// SEC-004: This middleware prevents information disclosure by sanitizing errors
// before they are returned to clients. In production mode, it replaces detailed
// error messages with safe, generic messages while preserving full details for
// server-side logging.
//
// Example:
//   bus := cqrs.NewCommandBus(
//       middleware.ErrorSanitizationMiddleware(logger, security.ErrorModeProduction),
//       middleware.RecoveryMiddleware(logger),
//       // ... other middleware
//   )
func ErrorSanitizationMiddleware(logger *slog.Logger, mode security.ErrorMode) eventsourcing.CommandMiddleware {
	if logger == nil {
		logger = slog.Default()
	}

	sanitizer := security.NewErrorSanitizer(mode)

	return func(next eventsourcing.CommandHandler) eventsourcing.CommandHandler {
		return eventsourcing.CommandHandlerFunc(func(ctx context.Context, cmd *eventsourcing.CommandEnvelope) ([]*eventsourcing.Event, error) {
			// Execute the handler
			events, err := next.Handle(ctx, cmd)

			if err == nil {
				return events, nil
			}

			// SEC-004: Log the original error server-side
			logger.ErrorContext(ctx, "Command handler error",
				slog.String("command_id", cmd.Metadata.CommandID),
				slog.String("command_type", cmd.Metadata.Custom["command_type"]),
				slog.Any("error", err),
			)

			// SEC-004: Sanitize the error before returning
			sanitizedErr := sanitizer.SanitizeError(err)

			// Log sanitization if error was changed
			if sanitizedErr != err {
				if safeErr, ok := sanitizedErr.(*security.SafeError); ok {
					logger.DebugContext(ctx, "Error sanitized",
						slog.String("command_id", cmd.Metadata.CommandID),
						slog.String("error_code", string(safeErr.GetCode())),
						slog.String("client_message", safeErr.Message),
					)
				}
			}

			return events, sanitizedErr
		})
	}
}

// ProductionErrorSanitizationMiddleware creates error sanitization middleware for production use.
//
// This is a convenience function that sets the error mode to production.
func ProductionErrorSanitizationMiddleware(logger *slog.Logger) eventsourcing.CommandMiddleware {
	return ErrorSanitizationMiddleware(logger, security.ErrorModeProduction)
}

// DevelopmentErrorSanitizationMiddleware creates error sanitization middleware for development use.
//
// This passes through all error details for debugging.
// WARNING: Never use this in production!
func DevelopmentErrorSanitizationMiddleware(logger *slog.Logger) eventsourcing.CommandMiddleware {
	return ErrorSanitizationMiddleware(logger, security.ErrorModeDevelopment)
}
