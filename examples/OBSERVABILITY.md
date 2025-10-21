# SQLite Observability Demo

This demo shows how to build a complete event sourcing application with built-in observability, all stored in SQLite database files. Perfect for single-binary applications!

## What It Creates

The demo creates two SQLite database files:

### `eventstore.db` - Event Sourcing Data
Contains all your domain events and aggregate state:
- **events** table - All domain events with full event sourcing support
- **snapshots** table - Aggregate snapshots for performance optimization

### `observability.db` - Traces & Metrics
Contains all observability data using OpenTelemetry format:
- **otel_traces** table - Trace metadata (trace ID, resource attributes)
- **otel_spans** table - Individual span data with timing, attributes, events
- **otel_metrics** table - Time-series metrics data (counters, histograms, gauges)

## Running the Demo

```bash
# Build the demo
go build -o bankaccount_sqlite_obs_demo ./bankaccount_sqlite_observability_demo.go

# Run it
./bankaccount_sqlite_obs_demo
```

The demo will:
1. Start an embedded NATS server
2. Create two SQLite database files (eventstore.db, observability.db)
3. Set up event sourcing with full observability
4. Execute several banking transactions
5. Query and display observability data

## Querying the Data

After running the demo, you can query the databases directly with SQLite CLI or any SQLite tool.

### Event Store Queries

```bash
# View all events
sqlite3 eventstore.db "SELECT aggregate_id, event_type, version, timestamp FROM events ORDER BY timestamp"

# View events for a specific account
sqlite3 eventstore.db "SELECT event_type, version FROM events WHERE aggregate_id = 'acc-sqlite-demo-789'"

# Count events by type
sqlite3 eventstore.db "SELECT event_type, COUNT(*) FROM events GROUP BY event_type"

# View snapshots
sqlite3 eventstore.db "SELECT aggregate_id, version, timestamp FROM snapshots"
```

### Observability Queries

```bash
# View all traces
sqlite3 observability.db "SELECT trace_id, created_at FROM otel_traces LIMIT 10"

# View spans with duration
sqlite3 observability.db "
  SELECT
    name,
    (end_time - start_time)/1000000 as duration_ms,
    status_code
  FROM otel_spans
  ORDER BY duration_ms DESC
  LIMIT 10
"

# Count spans by operation
sqlite3 observability.db "
  SELECT name, COUNT(*) as count
  FROM otel_spans
  GROUP BY name
  ORDER BY count DESC
"

# View trace with all spans
sqlite3 observability.db "
  SELECT
    trace_id,
    span_id,
    parent_span_id,
    name,
    (end_time - start_time)/1000000 as duration_ms
  FROM otel_spans
  WHERE trace_id = '<YOUR_TRACE_ID>'
  ORDER BY start_time
"

# View metrics summary
sqlite3 observability.db "
  SELECT
    name,
    type,
    COUNT(*) as data_points,
    AVG(value) as avg_value
  FROM otel_metrics
  GROUP BY name, type
"

# View command metrics
sqlite3 observability.db "
  SELECT
    name,
    json_extract(attributes, '$.command_type') as command,
    value,
    timestamp
  FROM otel_metrics
  WHERE name = 'eventsourcing.command.total'
  ORDER BY timestamp DESC
"

# Find slow operations
sqlite3 observability.db "
  SELECT
    name,
    (end_time - start_time)/1000000 as duration_ms,
    json_extract(attributes, '$.aggregate.type') as aggregate_type,
    datetime(start_time/1000000000, 'unixepoch') as occurred_at
  FROM otel_spans
  WHERE duration_ms > 10
  ORDER BY duration_ms DESC
"
```

### Pretty Output with SQLite CLI

```bash
# Enable headers and column mode
sqlite3 observability.db << EOF
.headers on
.mode column
SELECT name, COUNT(*) as span_count
FROM otel_spans
GROUP BY name;
EOF
```

## Programmatic Queries

You can also query the data programmatically using the provided query helpers:

```go
import "github.com/plaenen/eventsourcing/pkg/observability"

// Open the database
db, _ := sql.Open("sqlite", "./observability.db")
queries := observability.NewSQLiteObservabilityQueries(db, nil)

// Query recent traces
traces, _ := queries.QueryTraces(since, until, limit)

// Get a specific trace with all spans
trace, _ := queries.GetTrace(traceID)

// Query spans by criteria
spans, _ := queries.QuerySpans(observability.TraceQuery{
    Name:        "account.v1.%",  // Pattern matching
    MinDuration: 100,              // Slower than 100ms
    Since:       time.Now().Add(-1 * time.Hour),
    Limit:       50,
})

// Query metrics
metrics, _ := queries.QueryMetrics(observability.MetricQuery{
    Name:  "eventsourcing.command.total",
    Type:  "sum",
    Since: time.Now().Add(-24 * time.Hour),
})

// Get metric summary with aggregations
summary, _ := queries.GetMetricSummary("eventsourcing.command.duration", since, until)
fmt.Printf("Average: %v, Min: %v, Max: %v\n",
    summary["avg_value"], summary["min_value"], summary["max_value"])
```

## Use Cases

This approach is perfect for:

- **Single-Binary Applications** - No external dependencies required
- **CLI Tools** - Track performance and usage patterns over time
- **Edge Devices** - Full observability without network connectivity
- **Desktop Applications** - Built-in diagnostics for support
- **Development/Testing** - Local observability without infrastructure
- **Embedded Systems** - Resource-constrained environments
- **Offline-First Apps** - Works without internet connection

## Benefits

1. **Zero Infrastructure** - No Jaeger, Prometheus, or Grafana needed
2. **Portable** - Just copy the .db files to backup or move
3. **Queryable** - Use any SQLite tool or library
4. **Version Control** - Can commit reference databases for regression testing
5. **Cost-Effective** - No cloud services required
6. **Developer-Friendly** - Easy to debug and inspect
7. **Future-Proof** - Can migrate to external backends when ready

## Database Schema

### Traces Table Structure
```sql
CREATE TABLE otel_traces (
    trace_id TEXT PRIMARY KEY,
    trace_state TEXT,
    resource_attributes TEXT,  -- JSON
    created_at INTEGER NOT NULL
);
```

### Spans Table Structure
```sql
CREATE TABLE otel_spans (
    span_id TEXT PRIMARY KEY,
    trace_id TEXT NOT NULL,
    parent_span_id TEXT,
    name TEXT NOT NULL,
    kind INTEGER NOT NULL,
    start_time INTEGER NOT NULL,
    end_time INTEGER NOT NULL,
    status_code INTEGER NOT NULL,
    status_message TEXT,
    attributes TEXT,  -- JSON
    events TEXT,      -- JSON
    links TEXT,       -- JSON
    FOREIGN KEY (trace_id) REFERENCES otel_traces(trace_id)
);
```

### Metrics Table Structure
```sql
CREATE TABLE otel_metrics (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    description TEXT,
    unit TEXT,
    type TEXT NOT NULL,  -- gauge, sum, histogram
    timestamp INTEGER NOT NULL,
    value REAL,          -- For gauges/sums
    count INTEGER,       -- For histograms
    sum REAL,           -- For histograms
    min REAL,           -- For histograms
    max REAL,           -- For histograms
    attributes TEXT,              -- JSON
    resource_attributes TEXT      -- JSON
);
```

## Data Retention

By default, the observability database keeps data for 7 days. This is configurable:

```go
config := observability.DefaultSQLiteExporterConfig(db)
config.RetentionDays = 30  // Keep 30 days
config.RetentionDays = 0   // Keep forever
```

Cleanup runs automatically when new data is exported.

## Performance Tips

1. **WAL Mode** - SQLite uses WAL mode by default for better concurrency
2. **Indexes** - All tables have indexes on commonly queried fields
3. **Batch Writes** - Metrics and traces are batched for efficiency
4. **Separate Databases** - Event store and observability use separate files to avoid contention
5. **Periodic Export** - Metrics export every 5 seconds to reduce overhead

## Production Considerations

While SQLite observability is great for single-binary apps, consider these for production:

- **Disk Space** - Monitor database file sizes, especially with high throughput
- **Backup Strategy** - Regular backups of both database files
- **Query Performance** - Add custom indexes if you have specific query patterns
- **Rotation** - Consider rotating old database files (e.g., monthly archives)
- **Migration Path** - Can easily migrate to Jaeger/Prometheus later with zero code changes

## Migration to External Backends

When you're ready to use external observability backends, just swap the exporters:

```go
// Instead of SQLite exporters:
traceExporter, _ := observability.NewSQLiteTraceExporter(config)
metricExporter, _ := observability.NewSQLiteMetricExporter(config)

// Use OTLP exporters (sends to Jaeger, Tempo, etc.):
traceExporter, _ := otlptracegrpc.New(ctx)
metricExporter, _ := otlpmetricgrpc.New(ctx)

// Everything else stays the same!
```

No changes needed to instrumentation code - it all uses OpenTelemetry standard interfaces.
