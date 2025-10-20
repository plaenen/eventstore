package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"

	"github.com/plaenen/eventsourcing/pkg/eventsourcing"
)

// RecoveryMiddleware recovers from panics in command handlers.
func RecoveryMiddleware(logger *slog.Logger) eventsourcing.CommandMiddleware {
	if logger == nil {
		logger = slog.Default()
	}

	return func(next eventsourcing.CommandHandler) eventsourcing.CommandHandler {
		return eventsourcing.CommandHandlerFunc(func(ctx context.Context, cmd *eventsourcing.CommandEnvelope) (events []*eventsourcing.Event, err error) {
			defer func() {
				if r := recover(); r != nil {
					stack := string(debug.Stack())

					logger.ErrorContext(ctx, "Command handler panicked",
						slog.String("command_id", cmd.Metadata.CommandID),
						slog.String("command_type", cmd.Metadata.Custom["command_type"]),
						slog.Any("panic", r),
						slog.String("stack_trace", stack),
					)

					err = fmt.Errorf("command handler panicked: %v", r)
					events = nil
				}
			}()

			return next.Handle(ctx, cmd)
		})
	}
}
