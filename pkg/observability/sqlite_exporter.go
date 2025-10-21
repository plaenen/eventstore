package observability

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// SQLiteExporterConfig configures the SQLite exporter
type SQLiteExporterConfig struct {
	// DB is the SQLite database connection
	DB *sql.DB

	// TracesTable is the table name for traces (default: "otel_traces")
	TracesTable string

	// SpansTable is the table name for spans (default: "otel_spans")
	SpansTable string

	// MetricsTable is the table name for metrics (default: "otel_metrics")
	MetricsTable string

	// MaxBatchSize limits the number of items in a single batch
	MaxBatchSize int

	// RetentionDays removes data older than this (0 = keep forever)
	RetentionDays int
}

// DefaultSQLiteExporterConfig returns sensible defaults
func DefaultSQLiteExporterConfig(db *sql.DB) *SQLiteExporterConfig {
	return &SQLiteExporterConfig{
		DB:            db,
		TracesTable:   "otel_traces",
		SpansTable:    "otel_spans",
		MetricsTable:  "otel_metrics",
		MaxBatchSize:  100,
		RetentionDays: 7, // Keep 1 week by default
	}
}

// SQLiteTraceExporter exports traces to SQLite
type SQLiteTraceExporter struct {
	config *SQLiteExporterConfig
	mu     sync.Mutex
}

// NewSQLiteTraceExporter creates a new SQLite trace exporter
func NewSQLiteTraceExporter(config *SQLiteExporterConfig) (*SQLiteTraceExporter, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}
	if config.DB == nil {
		return nil, fmt.Errorf("database connection is required")
	}

	exporter := &SQLiteTraceExporter{
		config: config,
	}

	// Create tables
	if err := exporter.createTables(); err != nil {
		return nil, fmt.Errorf("failed to create tables: %w", err)
	}

	return exporter, nil
}

// createTables creates the necessary tables for trace storage
func (e *SQLiteTraceExporter) createTables() error {
	// Traces table stores trace-level metadata
	tracesSQL := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			trace_id TEXT PRIMARY KEY,
			trace_state TEXT,
			resource_attributes TEXT,
			created_at INTEGER NOT NULL
		)
	`, e.config.TracesTable)

	// Spans table stores individual span data
	spansSQL := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			span_id TEXT PRIMARY KEY,
			trace_id TEXT NOT NULL,
			parent_span_id TEXT,
			name TEXT NOT NULL,
			kind INTEGER NOT NULL,
			start_time INTEGER NOT NULL,
			end_time INTEGER NOT NULL,
			status_code INTEGER NOT NULL,
			status_message TEXT,
			attributes TEXT,
			events TEXT,
			links TEXT,
			FOREIGN KEY (trace_id) REFERENCES %s(trace_id)
		)
	`, e.config.SpansTable, e.config.TracesTable)

	// Create indexes for efficient queries
	indexSQL := fmt.Sprintf(`
		CREATE INDEX IF NOT EXISTS idx_spans_trace_id ON %s(trace_id);
		CREATE INDEX IF NOT EXISTS idx_spans_start_time ON %s(start_time);
		CREATE INDEX IF NOT EXISTS idx_spans_name ON %s(name);
		CREATE INDEX IF NOT EXISTS idx_traces_created_at ON %s(created_at);
	`, e.config.SpansTable, e.config.SpansTable, e.config.SpansTable, e.config.TracesTable)

	e.mu.Lock()
	defer e.mu.Unlock()

	if _, err := e.config.DB.Exec(tracesSQL); err != nil {
		return fmt.Errorf("creating traces table: %w", err)
	}
	if _, err := e.config.DB.Exec(spansSQL); err != nil {
		return fmt.Errorf("creating spans table: %w", err)
	}
	if _, err := e.config.DB.Exec(indexSQL); err != nil {
		return fmt.Errorf("creating indexes: %w", err)
	}

	return nil
}

// ExportSpans implements sdktrace.SpanExporter
func (e *SQLiteTraceExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	if len(spans) == 0 {
		return nil
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	tx, err := e.config.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	traceStmt, err := tx.PrepareContext(ctx, fmt.Sprintf(`
		INSERT OR IGNORE INTO %s (trace_id, trace_state, resource_attributes, created_at)
		VALUES (?, ?, ?, ?)
	`, e.config.TracesTable))
	if err != nil {
		return fmt.Errorf("prepare trace statement: %w", err)
	}
	defer traceStmt.Close()

	spanStmt, err := tx.PrepareContext(ctx, fmt.Sprintf(`
		INSERT OR REPLACE INTO %s (
			span_id, trace_id, parent_span_id, name, kind,
			start_time, end_time, status_code, status_message,
			attributes, events, links
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, e.config.SpansTable))
	if err != nil {
		return fmt.Errorf("prepare span statement: %w", err)
	}
	defer spanStmt.Close()

	now := time.Now().Unix()

	for _, span := range spans {
		spanCtx := span.SpanContext()
		traceID := spanCtx.TraceID().String()

		// Insert trace record
		resourceAttrs, _ := json.Marshal(attributesToMap(span.Resource().Attributes()))
		if _, err := traceStmt.ExecContext(ctx,
			traceID,
			spanCtx.TraceState().String(),
			string(resourceAttrs),
			now,
		); err != nil {
			return fmt.Errorf("insert trace: %w", err)
		}

		// Insert span record
		var parentSpanID *string
		if span.Parent().SpanID().IsValid() {
			sid := span.Parent().SpanID().String()
			parentSpanID = &sid
		}

		attrs, _ := json.Marshal(attributesToMap(span.Attributes()))
		events, _ := json.Marshal(eventsToSlice(span.Events()))
		links, _ := json.Marshal(linksToSlice(span.Links()))

		if _, err := spanStmt.ExecContext(ctx,
			spanCtx.SpanID().String(),
			traceID,
			parentSpanID,
			span.Name(),
			int(span.SpanKind()),
			span.StartTime().UnixNano(),
			span.EndTime().UnixNano(),
			int(span.Status().Code),
			span.Status().Description,
			string(attrs),
			string(events),
			string(links),
		); err != nil {
			return fmt.Errorf("insert span: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	// Cleanup old data if retention is configured
	if e.config.RetentionDays > 0 {
		go e.cleanup()
	}

	return nil
}

// Shutdown implements sdktrace.SpanExporter
func (e *SQLiteTraceExporter) Shutdown(ctx context.Context) error {
	// SQLite connection is managed externally, nothing to do
	return nil
}

// cleanup removes old traces based on retention policy
func (e *SQLiteTraceExporter) cleanup() {
	if e.config.RetentionDays <= 0 {
		return
	}

	cutoff := time.Now().Add(-time.Duration(e.config.RetentionDays) * 24 * time.Hour).Unix()

	e.mu.Lock()
	defer e.mu.Unlock()

	// Delete old spans first (due to foreign key)
	_, _ = e.config.DB.Exec(fmt.Sprintf(`
		DELETE FROM %s WHERE trace_id IN (
			SELECT trace_id FROM %s WHERE created_at < ?
		)
	`, e.config.SpansTable, e.config.TracesTable), cutoff)

	// Then delete old traces
	_, _ = e.config.DB.Exec(fmt.Sprintf(`
		DELETE FROM %s WHERE created_at < ?
	`, e.config.TracesTable), cutoff)
}

// SQLiteMetricExporter exports metrics to SQLite
type SQLiteMetricExporter struct {
	config *SQLiteExporterConfig
	mu     sync.Mutex
}

// NewSQLiteMetricExporter creates a new SQLite metric exporter
func NewSQLiteMetricExporter(config *SQLiteExporterConfig) (*SQLiteMetricExporter, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}
	if config.DB == nil {
		return nil, fmt.Errorf("database connection is required")
	}

	exporter := &SQLiteMetricExporter{
		config: config,
	}

	// Create tables
	if err := exporter.createTables(); err != nil {
		return nil, fmt.Errorf("failed to create tables: %w", err)
	}

	return exporter, nil
}

// createTables creates the necessary tables for metric storage
func (e *SQLiteMetricExporter) createTables() error {
	metricsSQL := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			description TEXT,
			unit TEXT,
			type TEXT NOT NULL,
			timestamp INTEGER NOT NULL,
			value REAL,
			count INTEGER,
			sum REAL,
			min REAL,
			max REAL,
			attributes TEXT,
			resource_attributes TEXT
		)
	`, e.config.MetricsTable)

	indexSQL := fmt.Sprintf(`
		CREATE INDEX IF NOT EXISTS idx_metrics_name ON %s(name);
		CREATE INDEX IF NOT EXISTS idx_metrics_timestamp ON %s(timestamp);
		CREATE INDEX IF NOT EXISTS idx_metrics_type ON %s(type);
	`, e.config.MetricsTable, e.config.MetricsTable, e.config.MetricsTable)

	e.mu.Lock()
	defer e.mu.Unlock()

	if _, err := e.config.DB.Exec(metricsSQL); err != nil {
		return fmt.Errorf("creating metrics table: %w", err)
	}
	if _, err := e.config.DB.Exec(indexSQL); err != nil {
		return fmt.Errorf("creating indexes: %w", err)
	}

	return nil
}

// Export implements sdkmetric.Exporter
func (e *SQLiteMetricExporter) Export(ctx context.Context, rm *metricdata.ResourceMetrics) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	tx, err := e.config.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, fmt.Sprintf(`
		INSERT INTO %s (
			name, description, unit, type, timestamp,
			value, count, sum, min, max, attributes, resource_attributes
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, e.config.MetricsTable))
	if err != nil {
		return fmt.Errorf("prepare statement: %w", err)
	}
	defer stmt.Close()

	resourceAttrs, _ := json.Marshal(attributesToMap(rm.Resource.Attributes()))
	timestamp := time.Now().Unix()

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if err := e.exportMetric(ctx, stmt, m, string(resourceAttrs), timestamp); err != nil {
				return fmt.Errorf("export metric %s: %w", m.Name, err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	// Cleanup old data if retention is configured
	if e.config.RetentionDays > 0 {
		go e.cleanupMetrics()
	}

	return nil
}

// exportMetric exports a single metric
func (e *SQLiteMetricExporter) exportMetric(ctx context.Context, stmt *sql.Stmt, m metricdata.Metrics, resourceAttrs string, timestamp int64) error {
	switch data := m.Data.(type) {
	case metricdata.Gauge[int64]:
		for _, dp := range data.DataPoints {
			attrs, _ := json.Marshal(attributeSetToMap(dp.Attributes))
			if _, err := stmt.ExecContext(ctx,
				m.Name, m.Description, m.Unit, "gauge", timestamp,
				float64(dp.Value), nil, nil, nil, nil, string(attrs), resourceAttrs,
			); err != nil {
				return err
			}
		}
	case metricdata.Gauge[float64]:
		for _, dp := range data.DataPoints {
			attrs, _ := json.Marshal(attributeSetToMap(dp.Attributes))
			if _, err := stmt.ExecContext(ctx,
				m.Name, m.Description, m.Unit, "gauge", timestamp,
				dp.Value, nil, nil, nil, nil, string(attrs), resourceAttrs,
			); err != nil {
				return err
			}
		}
	case metricdata.Sum[int64]:
		for _, dp := range data.DataPoints {
			attrs, _ := json.Marshal(attributeSetToMap(dp.Attributes))
			if _, err := stmt.ExecContext(ctx,
				m.Name, m.Description, m.Unit, "sum", timestamp,
				float64(dp.Value), nil, nil, nil, nil, string(attrs), resourceAttrs,
			); err != nil {
				return err
			}
		}
	case metricdata.Sum[float64]:
		for _, dp := range data.DataPoints {
			attrs, _ := json.Marshal(attributeSetToMap(dp.Attributes))
			if _, err := stmt.ExecContext(ctx,
				m.Name, m.Description, m.Unit, "sum", timestamp,
				dp.Value, nil, nil, nil, nil, string(attrs), resourceAttrs,
			); err != nil {
				return err
			}
		}
	case metricdata.Histogram[int64]:
		for _, dp := range data.DataPoints {
			attrs, _ := json.Marshal(attributeSetToMap(dp.Attributes))
			var minVal, maxVal *float64
			if minV, ok := dp.Min.Value(); ok {
				v := float64(minV)
				minVal = &v
			}
			if maxV, ok := dp.Max.Value(); ok {
				v := float64(maxV)
				maxVal = &v
			}
			if _, err := stmt.ExecContext(ctx,
				m.Name, m.Description, m.Unit, "histogram", timestamp,
				nil, dp.Count, float64(dp.Sum), minVal, maxVal, string(attrs), resourceAttrs,
			); err != nil {
				return err
			}
		}
	case metricdata.Histogram[float64]:
		for _, dp := range data.DataPoints {
			attrs, _ := json.Marshal(attributeSetToMap(dp.Attributes))
			var minVal, maxVal *float64
			if minV, ok := dp.Min.Value(); ok {
				minVal = &minV
			}
			if maxV, ok := dp.Max.Value(); ok {
				maxVal = &maxV
			}
			if _, err := stmt.ExecContext(ctx,
				m.Name, m.Description, m.Unit, "histogram", timestamp,
				nil, dp.Count, dp.Sum, minVal, maxVal, string(attrs), resourceAttrs,
			); err != nil {
				return err
			}
		}
	}
	return nil
}

// Temporality implements sdkmetric.Exporter
func (e *SQLiteMetricExporter) Temporality(kind sdkmetric.InstrumentKind) metricdata.Temporality {
	return metricdata.CumulativeTemporality
}

// Aggregation implements sdkmetric.Exporter
func (e *SQLiteMetricExporter) Aggregation(kind sdkmetric.InstrumentKind) sdkmetric.Aggregation {
	return sdkmetric.DefaultAggregationSelector(kind)
}

// ForceFlush implements sdkmetric.Exporter
func (e *SQLiteMetricExporter) ForceFlush(ctx context.Context) error {
	return nil
}

// Shutdown implements sdkmetric.Exporter
func (e *SQLiteMetricExporter) Shutdown(ctx context.Context) error {
	return nil
}

// cleanupMetrics removes old metrics based on retention policy
func (e *SQLiteMetricExporter) cleanupMetrics() {
	if e.config.RetentionDays <= 0 {
		return
	}

	cutoff := time.Now().Add(-time.Duration(e.config.RetentionDays) * 24 * time.Hour).Unix()

	e.mu.Lock()
	defer e.mu.Unlock()

	_, _ = e.config.DB.Exec(fmt.Sprintf(`
		DELETE FROM %s WHERE timestamp < ?
	`, e.config.MetricsTable), cutoff)
}

// Helper functions to convert OpenTelemetry types to JSON-serializable maps/slices

func attributesToMap(attrs []attribute.KeyValue) map[string]interface{} {
	m := make(map[string]interface{}, len(attrs))
	for _, attr := range attrs {
		m[string(attr.Key)] = attr.Value.AsInterface()
	}
	return m
}

func attributeSetToMap(attrs attribute.Set) map[string]interface{} {
	m := make(map[string]interface{})
	iter := attrs.Iter()
	for iter.Next() {
		kv := iter.Attribute()
		m[string(kv.Key)] = kv.Value.AsInterface()
	}
	return m
}

func eventsToSlice(events []sdktrace.Event) []map[string]interface{} {
	result := make([]map[string]interface{}, len(events))
	for i, event := range events {
		result[i] = map[string]interface{}{
			"name":       event.Name,
			"timestamp":  event.Time.UnixNano(),
			"attributes": attributesToMap(event.Attributes),
		}
	}
	return result
}

func linksToSlice(links []sdktrace.Link) []map[string]interface{} {
	result := make([]map[string]interface{}, len(links))
	for i, link := range links {
		result[i] = map[string]interface{}{
			"trace_id":   link.SpanContext.TraceID().String(),
			"span_id":    link.SpanContext.SpanID().String(),
			"attributes": attributesToMap(link.Attributes),
		}
	}
	return result
}

// statusCodeToString converts OTel status code to string
func statusCodeToString(code codes.Code) string {
	switch code {
	case codes.Ok:
		return "OK"
	case codes.Error:
		return "ERROR"
	default:
		return "UNSET"
	}
}

// spanKindToString converts OTel span kind to string
func spanKindToString(kind trace.SpanKind) string {
	switch kind {
	case trace.SpanKindInternal:
		return "INTERNAL"
	case trace.SpanKindServer:
		return "SERVER"
	case trace.SpanKindClient:
		return "CLIENT"
	case trace.SpanKindProducer:
		return "PRODUCER"
	case trace.SpanKindConsumer:
		return "CONSUMER"
	default:
		return "UNSPECIFIED"
	}
}
