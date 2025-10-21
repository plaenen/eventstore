package observability

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// TraceQuery represents a query for trace data
type TraceQuery struct {
	// TraceID filters by specific trace ID
	TraceID string

	// MinDuration filters spans longer than this (in milliseconds)
	MinDuration int64

	// MaxDuration filters spans shorter than this (in milliseconds)
	MaxDuration int64

	// Name filters spans by name (exact match or LIKE pattern)
	Name string

	// Since filters spans that started after this time
	Since time.Time

	// Until filters spans that started before this time
	Until time.Time

	// Limit limits the number of results
	Limit int

	// Offset for pagination
	Offset int
}

// Span represents a trace span from the database
type Span struct {
	SpanID         string
	TraceID        string
	ParentSpanID   *string
	Name           string
	Kind           int
	StartTime      time.Time
	EndTime        time.Time
	DurationMs     int64
	StatusCode     int
	StatusMessage  string
	Attributes     map[string]interface{}
	Events         []map[string]interface{}
	Links          []map[string]interface{}
}

// Trace represents a complete trace with all its spans
type Trace struct {
	TraceID            string
	TraceState         string
	ResourceAttributes map[string]interface{}
	CreatedAt          time.Time
	Spans              []Span
	RootSpan           *Span
	DurationMs         int64
}

// MetricQuery represents a query for metric data
type MetricQuery struct {
	// Name filters by metric name (exact match or LIKE pattern)
	Name string

	// Type filters by metric type (gauge, sum, histogram)
	Type string

	// Since filters metrics recorded after this time
	Since time.Time

	// Until filters metrics recorded before this time
	Until time.Time

	// Limit limits the number of results
	Limit int

	// Offset for pagination
	Offset int

	// GroupByName aggregates metrics by name
	GroupByName bool
}

// MetricDataPoint represents a single metric data point
type MetricDataPoint struct {
	ID                 int64
	Name               string
	Description        string
	Unit               string
	Type               string
	Timestamp          time.Time
	Value              *float64
	Count              *int64
	Sum                *float64
	Min                *float64
	Max                *float64
	Attributes         map[string]interface{}
	ResourceAttributes map[string]interface{}
}

// SQLiteObservabilityQueries provides helper methods for querying observability data
type SQLiteObservabilityQueries struct {
	db           *sql.DB
	tracesTable  string
	spansTable   string
	metricsTable string
}

// NewSQLiteObservabilityQueries creates a new query helper
func NewSQLiteObservabilityQueries(db *sql.DB, config *SQLiteExporterConfig) *SQLiteObservabilityQueries {
	if config == nil {
		config = DefaultSQLiteExporterConfig(db)
	}
	return &SQLiteObservabilityQueries{
		db:           db,
		tracesTable:  config.TracesTable,
		spansTable:   config.SpansTable,
		metricsTable: config.MetricsTable,
	}
}

// QuerySpans queries spans based on the provided criteria
func (q *SQLiteObservabilityQueries) QuerySpans(query TraceQuery) ([]Span, error) {
	sql := fmt.Sprintf(`
		SELECT
			span_id, trace_id, parent_span_id, name, kind,
			start_time, end_time, status_code, status_message,
			attributes, events, links
		FROM %s
		WHERE 1=1
	`, q.spansTable)

	args := []interface{}{}

	if query.TraceID != "" {
		sql += " AND trace_id = ?"
		args = append(args, query.TraceID)
	}

	if query.Name != "" {
		if containsWildcard(query.Name) {
			sql += " AND name LIKE ?"
		} else {
			sql += " AND name = ?"
		}
		args = append(args, query.Name)
	}

	if !query.Since.IsZero() {
		sql += " AND start_time >= ?"
		args = append(args, query.Since.UnixNano())
	}

	if !query.Until.IsZero() {
		sql += " AND start_time <= ?"
		args = append(args, query.Until.UnixNano())
	}

	if query.MinDuration > 0 {
		sql += " AND (end_time - start_time) >= ?"
		args = append(args, query.MinDuration*1_000_000) // Convert ms to ns
	}

	if query.MaxDuration > 0 {
		sql += " AND (end_time - start_time) <= ?"
		args = append(args, query.MaxDuration*1_000_000) // Convert ms to ns
	}

	sql += " ORDER BY start_time DESC"

	if query.Limit > 0 {
		sql += " LIMIT ?"
		args = append(args, query.Limit)
	}

	if query.Offset > 0 {
		sql += " OFFSET ?"
		args = append(args, query.Offset)
	}

	rows, err := q.db.Query(sql, args...)
	if err != nil {
		return nil, fmt.Errorf("query spans: %w", err)
	}
	defer rows.Close()

	spans := []Span{}
	for rows.Next() {
		span, err := q.scanSpan(rows)
		if err != nil {
			return nil, err
		}
		spans = append(spans, span)
	}

	return spans, rows.Err()
}

// GetTrace retrieves a complete trace with all its spans
func (q *SQLiteObservabilityQueries) GetTrace(traceID string) (*Trace, error) {
	// Get trace metadata
	var trace Trace
	var resourceAttrsJSON string
	var createdAt int64

	err := q.db.QueryRow(fmt.Sprintf(`
		SELECT trace_id, trace_state, resource_attributes, created_at
		FROM %s WHERE trace_id = ?
	`, q.tracesTable), traceID).Scan(
		&trace.TraceID,
		&trace.TraceState,
		&resourceAttrsJSON,
		&createdAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get trace: %w", err)
	}

	trace.CreatedAt = time.Unix(createdAt, 0)
	if err := json.Unmarshal([]byte(resourceAttrsJSON), &trace.ResourceAttributes); err != nil {
		return nil, fmt.Errorf("unmarshal resource attributes: %w", err)
	}

	// Get all spans for this trace
	spans, err := q.QuerySpans(TraceQuery{TraceID: traceID})
	if err != nil {
		return nil, fmt.Errorf("query spans: %w", err)
	}
	trace.Spans = spans

	// Find root span and calculate total duration
	var minStart, maxEnd int64
	for i := range spans {
		if spans[i].ParentSpanID == nil {
			trace.RootSpan = &spans[i]
		}
		startNano := spans[i].StartTime.UnixNano()
		endNano := spans[i].EndTime.UnixNano()
		if minStart == 0 || startNano < minStart {
			minStart = startNano
		}
		if endNano > maxEnd {
			maxEnd = endNano
		}
	}

	if minStart > 0 && maxEnd > 0 {
		trace.DurationMs = (maxEnd - minStart) / 1_000_000
	}

	return &trace, nil
}

// QueryTraces queries traces (without loading all spans)
func (q *SQLiteObservabilityQueries) QueryTraces(since, until time.Time, limit int) ([]Trace, error) {
	sql := fmt.Sprintf(`
		SELECT trace_id, trace_state, resource_attributes, created_at
		FROM %s
		WHERE 1=1
	`, q.tracesTable)

	args := []interface{}{}

	if !since.IsZero() {
		sql += " AND created_at >= ?"
		args = append(args, since.Unix())
	}

	if !until.IsZero() {
		sql += " AND created_at <= ?"
		args = append(args, until.Unix())
	}

	sql += " ORDER BY created_at DESC"

	if limit > 0 {
		sql += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := q.db.Query(sql, args...)
	if err != nil {
		return nil, fmt.Errorf("query traces: %w", err)
	}
	defer rows.Close()

	traces := []Trace{}
	for rows.Next() {
		var trace Trace
		var resourceAttrsJSON string
		var createdAt int64

		if err := rows.Scan(
			&trace.TraceID,
			&trace.TraceState,
			&resourceAttrsJSON,
			&createdAt,
		); err != nil {
			return nil, err
		}

		trace.CreatedAt = time.Unix(createdAt, 0)
		if err := json.Unmarshal([]byte(resourceAttrsJSON), &trace.ResourceAttributes); err != nil {
			return nil, fmt.Errorf("unmarshal resource attributes: %w", err)
		}

		traces = append(traces, trace)
	}

	return traces, rows.Err()
}

// QueryMetrics queries metrics based on the provided criteria
func (q *SQLiteObservabilityQueries) QueryMetrics(query MetricQuery) ([]MetricDataPoint, error) {
	sql := fmt.Sprintf(`
		SELECT
			id, name, description, unit, type, timestamp,
			value, count, sum, min, max, attributes, resource_attributes
		FROM %s
		WHERE 1=1
	`, q.metricsTable)

	args := []interface{}{}

	if query.Name != "" {
		if containsWildcard(query.Name) {
			sql += " AND name LIKE ?"
		} else {
			sql += " AND name = ?"
		}
		args = append(args, query.Name)
	}

	if query.Type != "" {
		sql += " AND type = ?"
		args = append(args, query.Type)
	}

	if !query.Since.IsZero() {
		sql += " AND timestamp >= ?"
		args = append(args, query.Since.Unix())
	}

	if !query.Until.IsZero() {
		sql += " AND timestamp <= ?"
		args = append(args, query.Until.Unix())
	}

	sql += " ORDER BY timestamp DESC"

	if query.Limit > 0 {
		sql += " LIMIT ?"
		args = append(args, query.Limit)
	}

	if query.Offset > 0 {
		sql += " OFFSET ?"
		args = append(args, query.Offset)
	}

	rows, err := q.db.Query(sql, args...)
	if err != nil {
		return nil, fmt.Errorf("query metrics: %w", err)
	}
	defer rows.Close()

	metrics := []MetricDataPoint{}
	for rows.Next() {
		metric, err := q.scanMetric(rows)
		if err != nil {
			return nil, err
		}
		metrics = append(metrics, metric)
	}

	return metrics, rows.Err()
}

// GetMetricSummary returns aggregated metric data over a time range
func (q *SQLiteObservabilityQueries) GetMetricSummary(name string, since, until time.Time) (map[string]interface{}, error) {
	query := fmt.Sprintf(`
		SELECT
			type,
			COUNT(*) as count,
			AVG(value) as avg_value,
			MIN(value) as min_value,
			MAX(value) as max_value,
			SUM(sum) as total_sum,
			SUM(count) as total_count
		FROM %s
		WHERE name = ?
	`, q.metricsTable)

	args := []interface{}{name}

	if !since.IsZero() {
		query += " AND timestamp >= ?"
		args = append(args, since.Unix())
	}

	if !until.IsZero() {
		query += " AND timestamp <= ?"
		args = append(args, until.Unix())
	}

	query += " GROUP BY type"

	var metricType string
	var count int64
	var avgValue, minValue, maxValue, totalSum *float64
	var totalCount *int64

	var avgValueFloat, minValueFloat, maxValueFloat, totalSumFloat sql.NullFloat64
	var totalCountInt sql.NullInt64

	err := q.db.QueryRow(query, args...).Scan(
		&metricType,
		&count,
		&avgValueFloat,
		&minValueFloat,
		&maxValueFloat,
		&totalSumFloat,
		&totalCountInt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get metric summary: %w", err)
	}

	if avgValueFloat.Valid {
		avgValue = &avgValueFloat.Float64
	}
	if minValueFloat.Valid {
		minValue = &minValueFloat.Float64
	}
	if maxValueFloat.Valid {
		maxValue = &maxValueFloat.Float64
	}
	if totalSumFloat.Valid {
		totalSum = &totalSumFloat.Float64
	}
	if totalCountInt.Valid {
		totalCount = &totalCountInt.Int64
	}

	summary := map[string]interface{}{
		"name":  name,
		"type":  metricType,
		"count": count,
	}

	if avgValue != nil {
		summary["avg_value"] = *avgValue
	}
	if minValue != nil {
		summary["min_value"] = *minValue
	}
	if maxValue != nil {
		summary["max_value"] = *maxValue
	}
	if totalSum != nil {
		summary["total_sum"] = *totalSum
	}
	if totalCount != nil {
		summary["total_count"] = *totalCount
	}

	return summary, nil
}

// scanSpan scans a span from a database row
func (q *SQLiteObservabilityQueries) scanSpan(rows *sql.Rows) (Span, error) {
	var span Span
	var startTimeNano, endTimeNano int64
	var attrsJSON, eventsJSON, linksJSON string

	err := rows.Scan(
		&span.SpanID,
		&span.TraceID,
		&span.ParentSpanID,
		&span.Name,
		&span.Kind,
		&startTimeNano,
		&endTimeNano,
		&span.StatusCode,
		&span.StatusMessage,
		&attrsJSON,
		&eventsJSON,
		&linksJSON,
	)
	if err != nil {
		return span, err
	}

	span.StartTime = time.Unix(0, startTimeNano)
	span.EndTime = time.Unix(0, endTimeNano)
	span.DurationMs = (endTimeNano - startTimeNano) / 1_000_000

	if err := json.Unmarshal([]byte(attrsJSON), &span.Attributes); err != nil {
		return span, fmt.Errorf("unmarshal attributes: %w", err)
	}
	if err := json.Unmarshal([]byte(eventsJSON), &span.Events); err != nil {
		return span, fmt.Errorf("unmarshal events: %w", err)
	}
	if err := json.Unmarshal([]byte(linksJSON), &span.Links); err != nil {
		return span, fmt.Errorf("unmarshal links: %w", err)
	}

	return span, nil
}

// scanMetric scans a metric from a database row
func (q *SQLiteObservabilityQueries) scanMetric(rows *sql.Rows) (MetricDataPoint, error) {
	var metric MetricDataPoint
	var timestamp int64
	var attrsJSON, resourceAttrsJSON string

	err := rows.Scan(
		&metric.ID,
		&metric.Name,
		&metric.Description,
		&metric.Unit,
		&metric.Type,
		&timestamp,
		&metric.Value,
		&metric.Count,
		&metric.Sum,
		&metric.Min,
		&metric.Max,
		&attrsJSON,
		&resourceAttrsJSON,
	)
	if err != nil {
		return metric, err
	}

	metric.Timestamp = time.Unix(timestamp, 0)

	if err := json.Unmarshal([]byte(attrsJSON), &metric.Attributes); err != nil {
		return metric, fmt.Errorf("unmarshal attributes: %w", err)
	}
	if err := json.Unmarshal([]byte(resourceAttrsJSON), &metric.ResourceAttributes); err != nil {
		return metric, fmt.Errorf("unmarshal resource attributes: %w", err)
	}

	return metric, nil
}

// containsWildcard checks if a string contains SQL wildcard characters
func containsWildcard(s string) bool {
	return len(s) > 0 && (s[0] == '%' || s[len(s)-1] == '%' || s[0] == '_')
}
