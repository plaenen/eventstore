package middleware

import (
	"context"
	"log/slog"
	"time"

	"github.com/plaenen/eventsourcing/pkg/eventsourcing"
)

// LoggingMiddleware logs command execution with timing information using slog.
func LoggingMiddleware(logger *slog.Logger) eventsourcing.CommandMiddleware {
	if logger == nil {
		logger = slog.Default()
	}

	return func(next eventsourcing.CommandHandler) eventsourcing.CommandHandler {
		return eventsourcing.CommandHandlerFunc(func(ctx context.Context, cmd *eventsourcing.CommandEnvelope) ([]*eventsourcing.Event, error) {
			start := time.Now()

			commandType := cmd.Metadata.Custom["command_type"]
			commandID := cmd.Metadata.CommandID
			principalID := cmd.Metadata.PrincipalID

			logger.InfoContext(ctx, "Executing command",
				slog.String("command_type", commandType),
				slog.String("command_id", commandID),
				slog.String("principal_id", principalID),
				slog.String("correlation_id", cmd.Metadata.CorrelationID),
			)

			events, err := next.Handle(ctx, cmd)

			duration := time.Since(start)

			if err != nil {
				logger.ErrorContext(ctx, "Command execution failed",
					slog.String("command_type", commandType),
					slog.String("command_id", commandID),
					slog.Int64("duration_ms", duration.Milliseconds()),
					slog.String("error", err.Error()),
				)
				return nil, err
			}

			logger.InfoContext(ctx, "Command executed successfully",
				slog.String("command_type", commandType),
				slog.String("command_id", commandID),
				slog.Int("events_count", len(events)),
				slog.Int64("duration_ms", duration.Milliseconds()),
			)

			return events, nil
		})
	}
}
