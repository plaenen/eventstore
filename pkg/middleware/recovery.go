package middleware

import (
	"context"
	"log/slog"
	"runtime/debug"

	"github.com/plaenen/eventstore/pkg/eventsourcing"
	"github.com/plaenen/eventstore/pkg/security"
)

// RecoveryMiddleware recovers from panics in command handlers.
//
// SEC-004: Panic errors are sanitized to prevent information disclosure.
// In production mode, only safe error messages are returned to clients.
// Full panic details are logged server-side for debugging.
func RecoveryMiddleware(logger *slog.Logger) eventsourcing.CommandMiddleware {
	return RecoveryMiddlewareWithMode(logger, security.ErrorModeProduction)
}

// RecoveryMiddlewareWithMode recovers from panics with configurable error mode.
func RecoveryMiddlewareWithMode(logger *slog.Logger, mode security.ErrorMode) eventsourcing.CommandMiddleware {
	if logger == nil {
		logger = slog.Default()
	}

	sanitizer := security.NewErrorSanitizer(mode)

	return func(next eventsourcing.CommandHandler) eventsourcing.CommandHandler {
		return eventsourcing.CommandHandlerFunc(func(ctx context.Context, cmd *eventsourcing.CommandEnvelope) (events []*eventsourcing.Event, err error) {
			defer func() {
				if r := recover(); r != nil {
					stack := string(debug.Stack())

					// SEC-004: Log full panic details server-side only
					logger.ErrorContext(ctx, "Command handler panicked",
						slog.String("command_id", cmd.Metadata.CommandID),
						slog.String("command_type", cmd.Metadata.Custom["command_type"]),
						slog.Any("panic", r),
						slog.String("stack_trace", stack),
					)

					// SEC-004: Return sanitized error to client (no panic details)
					err = sanitizer.SanitizePanicError(r)
					events = nil
				}
			}()

			return next.Handle(ctx, cmd)
		})
	}
}

// DevModeRecoveryMiddleware is a convenience function for development mode.
// WARNING: Never use this in production!
func DevModeRecoveryMiddleware(logger *slog.Logger) eventsourcing.CommandMiddleware {
	return RecoveryMiddlewareWithMode(logger, security.ErrorModeDevelopment)
}
