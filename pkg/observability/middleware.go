package observability

import (
	"context"
	"time"

	"github.com/plaenen/eventsourcing/pkg/eventsourcing"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/protobuf/proto"
)

// HandlerMiddleware wraps a HandlerFunc with observability (tracing and metrics)
func HandlerMiddleware(tel *Telemetry, subject string) func(eventsourcing.HandlerFunc) eventsourcing.HandlerFunc {
	tracer := tel.Tracer("eventsourcing.handler")

	return func(next eventsourcing.HandlerFunc) eventsourcing.HandlerFunc {
		return func(ctx context.Context, request proto.Message) (*eventsourcing.Response, error) {
			// Extract message type for better observability
			messageType := string(request.ProtoReflect().Descriptor().FullName())

			// Start span
			ctx, span := tracer.Start(ctx, subject,
				trace.WithSpanKind(trace.SpanKindServer),
				trace.WithAttributes(
					attribute.String("messaging.system", "nats"),
					attribute.String("messaging.destination", subject),
					attribute.String("messaging.operation", "process"),
					attribute.String("message.type", messageType),
				),
			)
			defer span.End()

			// Record metrics
			start := time.Now()

			// Call the actual handler
			response, err := next(ctx, request)

			duration := time.Since(start)

			// Record command metrics (assuming this is a command handler)
			if tel.Metrics != nil {
				tel.Metrics.RecordCommand(ctx, messageType, duration, err)
			}

			// Update span based on result
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
				span.SetAttributes(attribute.Bool("success", false))
			} else if response != nil && response.Error != nil {
				// Application error (not transport error)
				span.SetAttributes(
					attribute.Bool("success", false),
					attribute.String("app.error.code", response.Error.Code),
					attribute.String("app.error.message", response.Error.Message),
				)
				span.SetStatus(codes.Error, response.Error.Message)
			} else {
				span.SetStatus(codes.Ok, "")
				span.SetAttributes(attribute.Bool("success", true))
			}

			// Record duration
			span.SetAttributes(attribute.Float64("duration_ms", float64(duration.Milliseconds())))

			return response, err
		}
	}
}

// RepositoryMiddleware provides observability for repository operations
type RepositoryMiddleware struct {
	tel *Telemetry
}

// NewRepositoryMiddleware creates a new repository middleware
func NewRepositoryMiddleware(tel *Telemetry) *RepositoryMiddleware {
	return &RepositoryMiddleware{tel: tel}
}

// WrapLoad wraps a repository Load operation with tracing and metrics
func (m *RepositoryMiddleware) WrapLoad(aggregateType, aggregateID string, snapshotUsed bool, operation func() error) error {
	tracer := m.tel.Tracer("eventsourcing.repository")
	ctx := context.Background()

	ctx, span := tracer.Start(ctx, "repository.load",
		trace.WithAttributes(
			AttrAggregateType.String(aggregateType),
			AttrAggregateID.String(aggregateID),
			AttrOperation.String("load"),
			AttrSnapshotHit.Bool(snapshotUsed),
		),
	)
	defer span.End()

	start := time.Now()
	err := operation()
	duration := time.Since(start)

	// Record metrics
	if m.tel.Metrics != nil {
		m.tel.Metrics.RecordRepositoryOperation(ctx, "load", aggregateType)
		m.tel.Metrics.RecordAggregateLoad(ctx, aggregateType, snapshotUsed)
	}

	// Update span
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	} else {
		span.SetStatus(codes.Ok, "")
	}

	span.SetAttributes(attribute.Float64("duration_ms", float64(duration.Milliseconds())))

	return err
}

// WrapSave wraps a repository Save operation with tracing and metrics
func (m *RepositoryMiddleware) WrapSave(aggregateType, aggregateID string, version int64, eventCount int, operation func() error) error {
	tracer := m.tel.Tracer("eventsourcing.repository")
	ctx := context.Background()

	ctx, span := tracer.Start(ctx, "repository.save",
		trace.WithAttributes(
			AttrAggregateType.String(aggregateType),
			AttrAggregateID.String(aggregateID),
			AttrVersion.Int64(version),
			AttrOperation.String("save"),
			AttrEventCount.Int(eventCount),
		),
	)
	defer span.End()

	start := time.Now()
	err := operation()
	duration := time.Since(start)

	// Record metrics
	if m.tel.Metrics != nil {
		m.tel.Metrics.RecordRepositoryOperation(ctx, "save", aggregateType)
		m.tel.Metrics.RecordEventStoreOperation(ctx, "append", duration, eventCount)
	}

	// Update span
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	} else {
		span.SetStatus(codes.Ok, "")
	}

	span.SetAttributes(attribute.Float64("duration_ms", float64(duration.Milliseconds())))

	return err
}

// EventStoreMiddleware provides observability for event store operations
type EventStoreMiddleware struct {
	tel *Telemetry
}

// NewEventStoreMiddleware creates a new event store middleware
func NewEventStoreMiddleware(tel *Telemetry) *EventStoreMiddleware {
	return &EventStoreMiddleware{tel: tel}
}

// WrapAppendEvents wraps an AppendEvents operation with tracing and metrics
func (m *EventStoreMiddleware) WrapAppendEvents(ctx context.Context, aggregateType, aggregateID string, eventCount int, operation func(context.Context) error) error {
	tracer := m.tel.Tracer("eventsourcing.eventstore")

	ctx, span := tracer.Start(ctx, "eventstore.append",
		trace.WithAttributes(
			AttrAggregateType.String(aggregateType),
			AttrAggregateID.String(aggregateID),
			AttrEventCount.Int(eventCount),
		),
	)
	defer span.End()

	start := time.Now()
	err := operation(ctx)
	duration := time.Since(start)

	// Record metrics
	if m.tel.Metrics != nil {
		m.tel.Metrics.RecordEventStoreOperation(ctx, "append", duration, eventCount)
	}

	// Update span
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	} else {
		span.SetStatus(codes.Ok, "")
		if m.tel.Metrics != nil {
			m.tel.Metrics.EventsAppended.Add(ctx, int64(eventCount))
		}
	}

	span.SetAttributes(attribute.Float64("duration_ms", float64(duration.Milliseconds())))

	return err
}

// WrapLoadEvents wraps a LoadEvents operation with tracing and metrics
func (m *EventStoreMiddleware) WrapLoadEvents(ctx context.Context, aggregateType, aggregateID string, operation func(context.Context) (int, error)) (int, error) {
	tracer := m.tel.Tracer("eventsourcing.eventstore")

	ctx, span := tracer.Start(ctx, "eventstore.load",
		trace.WithAttributes(
			AttrAggregateType.String(aggregateType),
			AttrAggregateID.String(aggregateID),
		),
	)
	defer span.End()

	start := time.Now()
	eventCount, err := operation(ctx)
	duration := time.Since(start)

	// Record metrics
	if m.tel.Metrics != nil {
		m.tel.Metrics.RecordEventStoreOperation(ctx, "load", duration, eventCount)
	}

	// Update span
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	} else {
		span.SetStatus(codes.Ok, "")
		span.SetAttributes(AttrEventCount.Int(eventCount))
	}

	span.SetAttributes(attribute.Float64("duration_ms", float64(duration.Milliseconds())))

	return eventCount, err
}

// TransportMiddleware provides observability for transport operations
type TransportMiddleware struct {
	tel *Telemetry
}

// NewTransportMiddleware creates a new transport middleware
func NewTransportMiddleware(tel *Telemetry) *TransportMiddleware {
	return &TransportMiddleware{tel: tel}
}

// WrapRequest wraps a transport Request operation with tracing and metrics
func (m *TransportMiddleware) WrapRequest(ctx context.Context, subject string, request proto.Message, operation func(context.Context) (*eventsourcing.Response, error)) (*eventsourcing.Response, error) {
	tracer := m.tel.Tracer("eventsourcing.transport")
	messageType := string(request.ProtoReflect().Descriptor().FullName())

	ctx, span := tracer.Start(ctx, subject,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("messaging.system", "nats"),
			attribute.String("messaging.destination", subject),
			attribute.String("messaging.operation", "send"),
			attribute.String("message.type", messageType),
		),
	)
	defer span.End()

	start := time.Now()
	response, err := operation(ctx)
	duration := time.Since(start)

	// Update span
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	} else if response != nil && response.Error != nil {
		span.SetAttributes(
			attribute.String("app.error.code", response.Error.Code),
			attribute.String("app.error.message", response.Error.Message),
		)
		span.SetStatus(codes.Error, response.Error.Message)
	} else {
		span.SetStatus(codes.Ok, "")
	}

	span.SetAttributes(attribute.Float64("duration_ms", float64(duration.Milliseconds())))

	return response, err
}
