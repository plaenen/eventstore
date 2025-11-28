# Observability Package

The `pkg/observability` package provides a production-ready, backend-agnostic observability stack based on [OpenTelemetry](https://opentelemetry.io/). It supports distributed tracing, metrics, and structured logging.

## Features

- **OpenTelemetry Integration**: Standardized tracing and metrics API.
- **Pluggable Backends**: Support for any OpenTelemetry exporter (OTLP, Jaeger, Prometheus, etc.).
- **SQLite Backend**: Built-in support for storing traces and metrics in a local SQLite database (perfect for edge debugging and local development).
- **Graceful Shutdown**: Ensures all telemetry data is flushed before application exit.
- **W3C Trace Context**: Standard context propagation for distributed systems.

## Quick Start

Initialize the observability stack at the start of your application:

```go
import "github.com/plaenen/eventstore/pkg/observability"

func main() {
    ctx := context.Background()

    // 1. Configure
    config := observability.Config{
        ServiceName:    "my-service",
        ServiceVersion: "1.0.0",
        Environment:    "production",
        // Add exporters here (see below)
    }

    // 2. Initialize
    tel, err := observability.Init(ctx, config)
    if err != nil {
        log.Fatal(err)
    }
    defer tel.Shutdown(ctx)

    // 3. Use
    tracer := tel.Tracer("my-component")
    ctx, span := tracer.Start(ctx, "operation-name")
    defer span.End()
}
```

## SQLite Backend

This package includes a specialized SQLite exporter that stores telemetry data locally. This is ideal for:
- **Edge Deployments**: Where running a full collector stack (Jaeger/Prometheus) is too heavy.
- **Local Development**: Easy debugging without external dependencies.
- **Embedded Applications**: Self-contained observability.

### Configuration

```go
import (
    "database/sql"
    "github.com/plaenen/eventstore/pkg/observability"
    _ "modernc.org/sqlite"
)

func main() {
    // 1. Open Database
    db, _ := sql.Open("sqlite", "observability.db")

    // 2. Create Exporters
    exporterConfig := &observability.SQLiteExporterConfig{
        DB:            db,
        RetentionDays: 7, // Auto-delete data older than 7 days
    }

    traceExporter, _ := observability.NewSQLiteTraceExporter(exporterConfig)
    metricExporter, _ := observability.NewSQLiteMetricExporter(exporterConfig)

    // 3. Initialize with Exporters
    tel, _ := observability.Init(ctx, observability.Config{
        ServiceName:     "edge-service",
        TraceExporter:   traceExporter,
        // Note: For metrics, we use a PeriodicReader with the exporter
        MetricReader:    sdkmetric.NewPeriodicReader(metricExporter),
    })
}
```

### Querying Data

The package provides `SQLiteObservabilityQueries` to easily query the stored data. This is useful for building admin dashboards or debugging tools.

```go
queries := observability.NewSQLiteObservabilityQueries(db, nil)

// 1. Find Traces
traces, _ := queries.QueryTraces(
    time.Now().Add(-1 * time.Hour), // Since 1 hour ago
    time.Now(),                     // Until now
    10,                             // Limit
)

// 2. Get Full Trace Details
trace, _ := queries.GetTrace(traces[0].TraceID)
fmt.Printf("Trace Duration: %dms\n", trace.DurationMs)
for _, span := range trace.Spans {
    fmt.Printf("  Span: %s (%dms)\n", span.Name, span.DurationMs)
}

// 3. Query Metrics
metrics, _ := queries.QueryMetrics(observability.MetricQuery{
    Name:  "http_requests_total",
    Since: time.Now().Add(-1 * time.Hour),
})
```

## Database Schema

The SQLite backend creates the following tables automatically:

### `otel_traces`
Stores trace-level metadata.
- `trace_id` (PK)
- `trace_state`
- `resource_attributes` (JSON)
- `created_at`

### `otel_spans`
Stores individual spans.
- `span_id` (PK)
- `trace_id` (FK)
- `parent_span_id`
- `name`
- `kind`
- `start_time`, `end_time`
- `status_code`, `status_message`
- `attributes`, `events`, `links` (JSON)

### `otel_metrics`
Stores metric data points.
- `id` (PK)
- `name`, `description`, `unit`
- `type` (gauge, sum, histogram)
- `timestamp`
- `value` (for gauges/sums)
- `count`, `sum`, `min`, `max` (for histograms)
- `attributes` (JSON)
