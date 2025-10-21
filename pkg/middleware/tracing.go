package middleware

import (
	"context"
	"fmt"

	"github.com/plaenen/eventstore/pkg/eventsourcing"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// OpenTelemetryMiddleware adds OpenTelemetry distributed tracing to command execution.
// Uses the global tracer provider by default, or a custom tracer can be provided.
func OpenTelemetryMiddleware(tracerName string) eventsourcing.CommandMiddleware {
	if tracerName == "" {
		tracerName = "github.com/plaenen/eventstore"
	}

	tracer := otel.Tracer(tracerName)

	return func(next eventsourcing.CommandHandler) eventsourcing.CommandHandler {
		return eventsourcing.CommandHandlerFunc(func(ctx context.Context, cmd *eventsourcing.CommandEnvelope) ([]*eventsourcing.Event, error) {
			commandType := cmd.Metadata.Custom["command_type"]
			if commandType == "" {
				commandType = "unknown"
			}

			// Start OpenTelemetry span
			spanCtx, span := tracer.Start(ctx, fmt.Sprintf("command.%s", commandType),
				trace.WithSpanKind(trace.SpanKindInternal),
				trace.WithAttributes(
					attribute.String("command.id", cmd.Metadata.CommandID),
					attribute.String("command.type", commandType),
					attribute.String("command.principal_id", cmd.Metadata.PrincipalID),
					attribute.String("command.correlation_id", cmd.Metadata.CorrelationID),
				),
			)
			defer span.End()

			// Execute command
			events, err := next.Handle(spanCtx, cmd)

			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
				return nil, err
			}

			// Add event information to span
			span.SetAttributes(
				attribute.Int("events.count", len(events)),
			)

			if len(events) > 0 {
				eventTypes := make([]string, len(events))
				for i, evt := range events {
					eventTypes[i] = evt.EventType
				}
				span.SetAttributes(
					attribute.StringSlice("events.types", eventTypes),
				)
			}

			span.SetStatus(codes.Ok, "command executed successfully")

			return events, nil
		})
	}
}

// OpenTelemetryMiddlewareWithTracer creates middleware with a specific tracer.
func OpenTelemetryMiddlewareWithTracer(tracer trace.Tracer) eventsourcing.CommandMiddleware {
	return func(next eventsourcing.CommandHandler) eventsourcing.CommandHandler {
		return eventsourcing.CommandHandlerFunc(func(ctx context.Context, cmd *eventsourcing.CommandEnvelope) ([]*eventsourcing.Event, error) {
			commandType := cmd.Metadata.Custom["command_type"]
			if commandType == "" {
				commandType = "unknown"
			}

			spanCtx, span := tracer.Start(ctx, fmt.Sprintf("command.%s", commandType),
				trace.WithSpanKind(trace.SpanKindInternal),
				trace.WithAttributes(
					attribute.String("command.id", cmd.Metadata.CommandID),
					attribute.String("command.type", commandType),
					attribute.String("command.principal_id", cmd.Metadata.PrincipalID),
					attribute.String("command.correlation_id", cmd.Metadata.CorrelationID),
				),
			)
			defer span.End()

			events, err := next.Handle(spanCtx, cmd)

			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
				return nil, err
			}

			span.SetAttributes(attribute.Int("events.count", len(events)))
			if len(events) > 0 {
				eventTypes := make([]string, len(events))
				for i, evt := range events {
					eventTypes[i] = evt.EventType
				}
				span.SetAttributes(attribute.StringSlice("events.types", eventTypes))
			}

			span.SetStatus(codes.Ok, "command executed successfully")
			return events, nil
		})
	}
}
