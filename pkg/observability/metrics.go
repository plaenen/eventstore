package observability

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Metrics holds all metric instruments for the event sourcing framework
type Metrics struct {
	// Command metrics
	CommandDuration metric.Float64Histogram
	CommandTotal    metric.Int64Counter
	CommandErrors   metric.Int64Counter

	// Event metrics
	EventsAppended    metric.Int64Counter
	EventsPublished   metric.Int64Counter
	EventStoreLatency metric.Float64Histogram

	// Aggregate metrics
	AggregateLoads metric.Int64Counter
	SnapshotHits   metric.Int64Counter
	SnapshotMisses metric.Int64Counter

	// Projection metrics
	ProjectionLag    metric.Float64Gauge
	ProjectionErrors metric.Int64Counter

	// Repository metrics
	RepositorySaves metric.Int64Counter
	RepositoryLoads metric.Int64Counter

	// NATS metrics
	NATSPublishLatency metric.Float64Histogram
	NATSMessages       metric.Int64Counter
}

// NewMetrics creates all metric instruments
func NewMetrics(meter metric.Meter) (*Metrics, error) {
	m := &Metrics{}
	var err error

	// Command metrics
	m.CommandDuration, err = meter.Float64Histogram(
		"eventsourcing.command.duration",
		metric.WithDescription("Command execution duration in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, fmt.Errorf("creating command.duration: %w", err)
	}

	m.CommandTotal, err = meter.Int64Counter(
		"eventsourcing.command.total",
		metric.WithDescription("Total commands executed"),
	)
	if err != nil {
		return nil, fmt.Errorf("creating command.total: %w", err)
	}

	m.CommandErrors, err = meter.Int64Counter(
		"eventsourcing.command.errors",
		metric.WithDescription("Total command errors"),
	)
	if err != nil {
		return nil, fmt.Errorf("creating command.errors: %w", err)
	}

	// Event metrics
	m.EventsAppended, err = meter.Int64Counter(
		"eventsourcing.events.appended",
		metric.WithDescription("Total events appended to event store"),
	)
	if err != nil {
		return nil, fmt.Errorf("creating events.appended: %w", err)
	}

	m.EventsPublished, err = meter.Int64Counter(
		"eventsourcing.events.published",
		metric.WithDescription("Total events published to event bus"),
	)
	if err != nil {
		return nil, fmt.Errorf("creating events.published: %w", err)
	}

	m.EventStoreLatency, err = meter.Float64Histogram(
		"eventsourcing.eventstore.latency",
		metric.WithDescription("Event store operation latency in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, fmt.Errorf("creating eventstore.latency: %w", err)
	}

	// Aggregate metrics
	m.AggregateLoads, err = meter.Int64Counter(
		"eventsourcing.aggregate.loads",
		metric.WithDescription("Total aggregate loads"),
	)
	if err != nil {
		return nil, fmt.Errorf("creating aggregate.loads: %w", err)
	}

	m.SnapshotHits, err = meter.Int64Counter(
		"eventsourcing.snapshot.hits",
		metric.WithDescription("Snapshot cache hits"),
	)
	if err != nil {
		return nil, fmt.Errorf("creating snapshot.hits: %w", err)
	}

	m.SnapshotMisses, err = meter.Int64Counter(
		"eventsourcing.snapshot.misses",
		metric.WithDescription("Snapshot cache misses"),
	)
	if err != nil {
		return nil, fmt.Errorf("creating snapshot.misses: %w", err)
	}

	// Projection metrics
	m.ProjectionLag, err = meter.Float64Gauge(
		"eventsourcing.projection.lag",
		metric.WithDescription("Projection lag in seconds behind event stream"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, fmt.Errorf("creating projection.lag: %w", err)
	}

	m.ProjectionErrors, err = meter.Int64Counter(
		"eventsourcing.projection.errors",
		metric.WithDescription("Projection processing errors"),
	)
	if err != nil {
		return nil, fmt.Errorf("creating projection.errors: %w", err)
	}

	// Repository metrics
	m.RepositorySaves, err = meter.Int64Counter(
		"eventsourcing.repository.saves",
		metric.WithDescription("Total repository save operations"),
	)
	if err != nil {
		return nil, fmt.Errorf("creating repository.saves: %w", err)
	}

	m.RepositoryLoads, err = meter.Int64Counter(
		"eventsourcing.repository.loads",
		metric.WithDescription("Total repository load operations"),
	)
	if err != nil {
		return nil, fmt.Errorf("creating repository.loads: %w", err)
	}

	// NATS metrics
	m.NATSPublishLatency, err = meter.Float64Histogram(
		"eventsourcing.nats.publish.latency",
		metric.WithDescription("NATS publish latency in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, fmt.Errorf("creating nats.publish.latency: %w", err)
	}

	m.NATSMessages, err = meter.Int64Counter(
		"eventsourcing.nats.messages",
		metric.WithDescription("Total NATS messages published/received"),
	)
	if err != nil {
		return nil, fmt.Errorf("creating nats.messages: %w", err)
	}

	return m, nil
}

// RecordCommand records command execution metrics
func (m *Metrics) RecordCommand(ctx context.Context, commandType string, duration time.Duration, err error) {
	attrs := []attribute.KeyValue{
		attribute.String("command_type", commandType),
	}

	m.CommandDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(attrs...))
	m.CommandTotal.Add(ctx, 1, metric.WithAttributes(attrs...))

	if err != nil {
		errorAttrs := append(attrs,
			attribute.String("error_type", fmt.Sprintf("%T", err)),
			attribute.Bool("success", false),
		)
		m.CommandErrors.Add(ctx, 1, metric.WithAttributes(errorAttrs...))
	} else {
		successAttrs := append(attrs, attribute.Bool("success", true))
		// Can track success rate by comparing total vs errors
		_ = successAttrs
	}
}

// RecordEventStoreOperation records event store operation metrics
func (m *Metrics) RecordEventStoreOperation(ctx context.Context, operation string, duration time.Duration, eventCount int) {
	attrs := []attribute.KeyValue{
		attribute.String("operation", operation),
	}

	m.EventStoreLatency.Record(ctx, duration.Seconds(), metric.WithAttributes(attrs...))

	if operation == "append" {
		m.EventsAppended.Add(ctx, int64(eventCount), metric.WithAttributes(attrs...))
	}
}

// RecordAggregateLoad records aggregate load metrics with snapshot usage
func (m *Metrics) RecordAggregateLoad(ctx context.Context, aggregateType string, snapshotUsed bool) {
	attrs := []attribute.KeyValue{
		attribute.String("aggregate_type", aggregateType),
	}

	m.AggregateLoads.Add(ctx, 1, metric.WithAttributes(attrs...))

	if snapshotUsed {
		m.SnapshotHits.Add(ctx, 1, metric.WithAttributes(attrs...))
	} else {
		m.SnapshotMisses.Add(ctx, 1, metric.WithAttributes(attrs...))
	}
}

// RecordRepositoryOperation records repository operations
func (m *Metrics) RecordRepositoryOperation(ctx context.Context, operation string, aggregateType string) {
	attrs := []attribute.KeyValue{
		attribute.String("aggregate_type", aggregateType),
	}

	switch operation {
	case "save":
		m.RepositorySaves.Add(ctx, 1, metric.WithAttributes(attrs...))
	case "load":
		m.RepositoryLoads.Add(ctx, 1, metric.WithAttributes(attrs...))
	}
}

// RecordProjectionLag records how far behind a projection is
func (m *Metrics) RecordProjectionLag(ctx context.Context, projectionName string, lagSeconds float64) {
	attrs := []attribute.KeyValue{
		attribute.String("projection", projectionName),
	}

	m.ProjectionLag.Record(ctx, lagSeconds, metric.WithAttributes(attrs...))
}

// RecordProjectionError records projection processing errors
func (m *Metrics) RecordProjectionError(ctx context.Context, projectionName string, errorType string) {
	attrs := []attribute.KeyValue{
		attribute.String("projection", projectionName),
		attribute.String("error_type", errorType),
	}

	m.ProjectionErrors.Add(ctx, 1, metric.WithAttributes(attrs...))
}

// RecordNATSPublish records NATS publish metrics
func (m *Metrics) RecordNATSPublish(ctx context.Context, subject string, duration time.Duration, messageCount int) {
	attrs := []attribute.KeyValue{
		attribute.String("subject", subject),
		attribute.String("direction", "publish"),
	}

	m.NATSPublishLatency.Record(ctx, duration.Seconds(), metric.WithAttributes(attrs...))
	m.NATSMessages.Add(ctx, int64(messageCount), metric.WithAttributes(attrs...))
}
