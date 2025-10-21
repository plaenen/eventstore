package observability

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// SpanOption configures a span
type SpanOption func(trace.Span)

// WithAttributes adds attributes to a span
func WithAttributes(attrs ...attribute.KeyValue) SpanOption {
	return func(span trace.Span) {
		span.SetAttributes(attrs...)
	}
}

// WithError marks a span as errored
func WithError(err error) SpanOption {
	return func(span trace.Span) {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
}

// StartSpan starts a new span with the given name and options
// Returns the span and a context containing the span
func StartSpan(ctx context.Context, tracer trace.Tracer, name string, opts ...SpanOption) (context.Context, trace.Span) {
	ctx, span := tracer.Start(ctx, name)

	for _, opt := range opts {
		opt(span)
	}

	return ctx, span
}

// EndSpan ends a span, optionally recording an error
func EndSpan(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	} else {
		span.SetStatus(codes.Ok, "")
	}
	span.End()
}

// TraceID extracts the trace ID from context as a string
func TraceID(ctx context.Context) string {
	spanCtx := trace.SpanFromContext(ctx).SpanContext()
	if spanCtx.IsValid() {
		return spanCtx.TraceID().String()
	}
	return ""
}

// SpanID extracts the span ID from context as a string
func SpanID(ctx context.Context) string {
	spanCtx := trace.SpanFromContext(ctx).SpanContext()
	if spanCtx.IsValid() {
		return spanCtx.SpanID().String()
	}
	return ""
}

// SetSpanAttributes adds attributes to the current span in the context
func SetSpanAttributes(ctx context.Context, attrs ...attribute.KeyValue) {
	trace.SpanFromContext(ctx).SetAttributes(attrs...)
}

// SetSpanError records an error on the current span in the context
func SetSpanError(ctx context.Context, err error) {
	span := trace.SpanFromContext(ctx)
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}

// AddSpanEvent adds an event to the current span in the context
func AddSpanEvent(ctx context.Context, name string, attrs ...attribute.KeyValue) {
	trace.SpanFromContext(ctx).AddEvent(name, trace.WithAttributes(attrs...))
}

// Common attribute keys for event sourcing
var (
	// Aggregate attributes
	AttrAggregateID   = attribute.Key("aggregate.id")
	AttrAggregateType = attribute.Key("aggregate.type")
	AttrVersion       = attribute.Key("aggregate.version")

	// Command attributes
	AttrCommandType = attribute.Key("command.type")
	AttrCommandID   = attribute.Key("command.id")

	// Event attributes
	AttrEventType  = attribute.Key("event.type")
	AttrEventID    = attribute.Key("event.id")
	AttrEventCount = attribute.Key("event.count")

	// Repository attributes
	AttrOperation = attribute.Key("repository.operation")

	// Snapshot attributes
	AttrSnapshotHit = attribute.Key("snapshot.hit")

	// Error attributes
	AttrErrorType = attribute.Key("error.type")
	AttrErrorCode = attribute.Key("error.code")

	// Tenant attributes
	AttrTenantID = attribute.Key("tenant.id")
)

// Helper functions for common attributes

// AggregateAttrs returns common aggregate attributes
func AggregateAttrs(id, aggregateType string, version int64) []attribute.KeyValue {
	return []attribute.KeyValue{
		AttrAggregateID.String(id),
		AttrAggregateType.String(aggregateType),
		AttrVersion.Int64(version),
	}
}

// CommandAttrs returns common command attributes
func CommandAttrs(commandType, commandID string) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		AttrCommandType.String(commandType),
	}
	if commandID != "" {
		attrs = append(attrs, AttrCommandID.String(commandID))
	}
	return attrs
}

// EventAttrs returns common event attributes
func EventAttrs(eventType, eventID string) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		AttrEventType.String(eventType),
	}
	if eventID != "" {
		attrs = append(attrs, AttrEventID.String(eventID))
	}
	return attrs
}

// ErrorAttrs returns common error attributes
func ErrorAttrs(err error, code string) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		AttrErrorType.String(fmt.Sprintf("%T", err)),
	}
	if code != "" {
		attrs = append(attrs, AttrErrorCode.String(code))
	}
	return attrs
}

// TenantAttrs returns tenant attribute
func TenantAttrs(tenantID string) []attribute.KeyValue {
	return []attribute.KeyValue{
		AttrTenantID.String(tenantID),
	}
}
