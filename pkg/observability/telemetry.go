// Package observability provides OpenTelemetry-based tracing and metrics
// with backend-agnostic configuration for production observability.
package observability

import (
	"context"
	"fmt"
	"log/slog"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// Config configures the observability stack
type Config struct {
	// Service metadata
	ServiceName    string
	ServiceVersion string
	Environment    string // dev, staging, prod

	// Tracing
	TraceExporter  sdktrace.SpanExporter // Pluggable exporter (OTLP, Jaeger, stdout, etc)
	TraceSampleRate float64               // 0.0 to 1.0 (1.0 = trace everything)

	// Metrics
	MetricReader   sdkmetric.Reader      // Pluggable reader (Prometheus, OTLP, stdout, etc)

	// Logging
	Logger *slog.Logger
}

// Telemetry manages the observability stack
type Telemetry struct {
	TracerProvider trace.TracerProvider
	MeterProvider  metric.MeterProvider
	Metrics        *Metrics
	Logger         *slog.Logger

	shutdown func(context.Context) error
}

// Init initializes OpenTelemetry with graceful degradation.
// If exporters/readers are nil, telemetry is disabled but calls are no-ops.
func Init(ctx context.Context, cfg Config) (*Telemetry, error) {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	// Create resource describing this service
	res, err := resource.New(ctx,
		resource.WithAttributes(
			attribute.String("service.name", cfg.ServiceName),
			attribute.String("service.version", cfg.ServiceVersion),
			attribute.String("deployment.environment", cfg.Environment),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	tel := &Telemetry{
		Logger: cfg.Logger,
	}

	var shutdownFuncs []func(context.Context) error

	// Setup Tracing (if exporter provided)
	if cfg.TraceExporter != nil {
		tp, shutdown, err := setupTracing(res, cfg)
		if err != nil {
			cfg.Logger.Warn("tracing setup failed, continuing without tracing", "error", err)
		} else {
			tel.TracerProvider = tp
			shutdownFuncs = append(shutdownFuncs, shutdown)
			otel.SetTracerProvider(tp)
			cfg.Logger.Info("tracing initialized", "service", cfg.ServiceName)
		}
	} else {
		// Use no-op tracer provider
		tel.TracerProvider = trace.NewNoopTracerProvider()
		cfg.Logger.Info("tracing disabled (no exporter configured)")
	}

	// Setup Metrics (if reader provided)
	if cfg.MetricReader != nil {
		mp, metrics, shutdown, err := setupMetrics(res, cfg)
		if err != nil {
			cfg.Logger.Warn("metrics setup failed, continuing without metrics", "error", err)
		} else {
			tel.MeterProvider = mp
			tel.Metrics = metrics
			shutdownFuncs = append(shutdownFuncs, shutdown)
			otel.SetMeterProvider(mp)
			cfg.Logger.Info("metrics initialized", "service", cfg.ServiceName)
		}
	} else {
		// Create no-op meter provider (empty provider acts as no-op)
		tel.MeterProvider = sdkmetric.NewMeterProvider()
		cfg.Logger.Info("metrics disabled (no reader configured)")
	}

	// Setup Context Propagation (W3C Trace Context standard)
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)

	// Combine shutdown functions
	tel.shutdown = func(ctx context.Context) error {
		var errs []error
		for _, shutdown := range shutdownFuncs {
			if err := shutdown(ctx); err != nil {
				errs = append(errs, err)
			}
		}
		if len(errs) > 0 {
			return fmt.Errorf("shutdown errors: %v", errs)
		}
		return nil
	}

	return tel, nil
}

// setupTracing creates a TracerProvider with the configured exporter
func setupTracing(res *resource.Resource, cfg Config) (trace.TracerProvider, func(context.Context) error, error) {
	// Configure sampling strategy
	var sampler sdktrace.Sampler
	if cfg.TraceSampleRate <= 0 {
		sampler = sdktrace.NeverSample()
	} else if cfg.TraceSampleRate >= 1.0 {
		sampler = sdktrace.AlwaysSample()
	} else {
		sampler = sdktrace.TraceIDRatioBased(cfg.TraceSampleRate)
	}

	// Create trace provider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(cfg.TraceExporter), // Batches spans for efficiency
		sdktrace.WithSampler(sampler),
	)

	return tp, tp.Shutdown, nil
}

// setupMetrics creates a MeterProvider with the configured reader
func setupMetrics(res *resource.Resource, cfg Config) (metric.MeterProvider, *Metrics, func(context.Context) error, error) {
	// Create meter provider
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(cfg.MetricReader),
	)

	// Create metric instruments
	meter := mp.Meter("eventsourcing")
	metrics, err := NewMetrics(meter)
	if err != nil {
		return nil, nil, nil, err
	}

	return mp, metrics, mp.Shutdown, nil
}

// Shutdown gracefully shuts down the telemetry stack
func (t *Telemetry) Shutdown(ctx context.Context) error {
	if t.shutdown != nil {
		t.Logger.Info("shutting down observability")
		return t.shutdown(ctx)
	}
	return nil
}

// Tracer returns a tracer for the given name
func (t *Telemetry) Tracer(name string) trace.Tracer {
	return t.TracerProvider.Tracer(name)
}

// Meter returns a meter for the given name
func (t *Telemetry) Meter(name string) metric.Meter {
	return t.MeterProvider.Meter(name)
}
