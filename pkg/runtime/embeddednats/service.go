// Package embeddednats provides a runner.Service adapter for embedded NATS server.
// This package bridges pkg/nats with pkg/runner for lifecycle management.
package embeddednats

import (
	"context"
	"fmt"

	"github.com/plaenen/eventstore/pkg/infrastructure/nats"
	"github.com/plaenen/eventstore/pkg/observability"
	"github.com/plaenen/eventstore/pkg/runner"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// Service wraps an embedded NATS server as a runner.Service.
// Use this when you only need the NATS server without EventBus integration.
//
// This adapter provides:
// - Lifecycle management (Start/Stop)
// - Health checks
// - OpenTelemetry integration (optional)
type Service struct {
	server      *nats.EmbeddedServer
	logger      runner.Logger
	tracer      trace.Tracer
	natsOptions []nats.Option
}

// Option configures the NATS service.
type Option func(*Service)

// WithLogger sets the logger for the service.
func WithLogger(logger runner.Logger) Option {
	return func(s *Service) {
		s.logger = logger
	}
}

// WithTracer sets the OpenTelemetry tracer for the service.
func WithTracer(tracer trace.Tracer) Option {
	return func(s *Service) {
		s.tracer = tracer
	}
}

// WithNATSOptions sets the NATS server configuration options.
// These options are passed to nats.StartEmbeddedServer().
//
// Example:
//
//	service := embeddednats.New(
//	    embeddednats.WithNATSOptions(
//	        nats.WithPort(4222),
//	        nats.WithStoreDir("/var/nats/data"),
//	        nats.WithDebug(true),
//	    ),
//	)
func WithNATSOptions(opts ...nats.Option) Option {
	return func(s *Service) {
		s.natsOptions = opts
	}
}

// New creates a new embedded NATS service for use with runner.
func New(opts ...Option) *Service {
	s := &Service{
		logger: runner.NewNoopLogger(),
		tracer: noop.NewTracerProvider().Tracer("embeddednats"),
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// Name returns the service name for logging.
func (s *Service) Name() string {
	return "embedded-nats"
}

// Start starts the embedded NATS server.
func (s *Service) Start(ctx context.Context) error {
	ctx, span := s.tracer.Start(ctx, "embeddednats.Start")
	defer span.End()

	s.logger.Info("starting embedded NATS server")

	// Start server with configured options
	srv, err := nats.StartEmbeddedServer(s.natsOptions...)
	if err != nil {
		observability.SetSpanError(ctx, err)
		s.logger.Error("failed to start embedded NATS", "error", err)
		return fmt.Errorf("failed to start embedded NATS: %w", err)
	}

	s.server = srv

	span.SetAttributes(
		attribute.String("nats.url", srv.URL()),
	)

	s.logger.Info("embedded NATS server started",
		"url", srv.URL())

	return nil
}

// Stop gracefully shuts down the embedded NATS server.
func (s *Service) Stop(ctx context.Context) error {
	ctx, span := s.tracer.Start(ctx, "embeddednats.Stop")
	defer span.End()

	s.logger.Info("stopping embedded NATS server")

	if s.server != nil {
		s.server.Shutdown()
		s.logger.Info("embedded NATS server stopped")
	}

	return nil
}

// HealthCheck checks if the NATS server is running.
func (s *Service) HealthCheck(ctx context.Context) error {
	ctx, span := s.tracer.Start(ctx, "embeddednats.HealthCheck")
	defer span.End()

	if s.server == nil {
		err := fmt.Errorf("nats server not started")
		observability.SetSpanError(ctx, err)
		return err
	}

	// Try to connect to verify server is responsive
	nc, err := nats.ConnectToEmbedded(s.server)
	if err != nil {
		observability.SetSpanError(ctx, err)
		return fmt.Errorf("nats server not responsive: %w", err)
	}
	nc.Close()

	span.SetAttributes(attribute.Bool("healthy", true))
	return nil
}

// URL returns the NATS server connection URL.
// Only available after Start() succeeds.
func (s *Service) URL() string {
	if s.server == nil {
		return ""
	}
	return s.server.URL()
}

// Server returns the underlying embedded server.
// Only available after Start() succeeds.
func (s *Service) Server() *nats.EmbeddedServer {
	return s.server
}

// Ensure Service implements runner.Service and runner.HealthChecker
var _ runner.Service = (*Service)(nil)
var _ runner.HealthChecker = (*Service)(nil)
