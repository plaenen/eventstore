// Package eventbus provides a runner.Service adapter for NATS EventBus.
// This package bridges pkg/eventbus/nats with pkg/runner for lifecycle management.
package eventbus

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	natseventbus "github.com/plaenen/eventstore/pkg/messaging/nats"
	"github.com/plaenen/eventstore/pkg/infrastructure/nats"
	"github.com/plaenen/eventstore/pkg/observability"
	"github.com/plaenen/eventstore/pkg/runner"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// Service wraps both an embedded NATS server and EventBus as a runner.Service.
// This provides a complete event bus solution with automatic lifecycle management.
//
// This adapter provides:
// - Lifecycle management (Start/Stop with proper ordering)
// - Health checks
// - OpenTelemetry integration (optional)
// - Graceful shutdown with connection draining
//
// Example usage:
//
//	eventBusService := eventbus.New(
//	    eventbus.WithConfig(natseventbus.DefaultConfig()),
//	    eventbus.WithLogger(logger),
//	    eventbus.WithTracer(tracer),
//	)
//	projectionService := NewProjectionService(eventBusService.EventBus())
//
//	runner := runner.New([]runner.Service{
//	    eventBusService,
//	    projectionService,
//	})
//	runner.Run(ctx)
type Service struct {
	config natseventbus.Config
	server *nats.EmbeddedServer
	bus    *natseventbus.EventBus
	logger *slog.Logger
	tracer trace.Tracer
}

// Option configures the EventBus service.
type Option func(*Service)

// WithConfig sets the NATS configuration.
// The URL in the config is ignored and replaced with the embedded server URL.
func WithConfig(config natseventbus.Config) Option {
	return func(s *Service) {
		s.config = config
	}
}

// WithLogger sets the logger for the service.
func WithLogger(logger *slog.Logger) Option {
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

// New creates a new EventBus service for use with runner.
func New(opts ...Option) *Service {
	s := &Service{
		config: natseventbus.DefaultConfig(),
		logger: slog.Default(),
		tracer: noop.NewTracerProvider().Tracer("eventbus"),
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// Name returns the service name for logging.
func (s *Service) Name() string {
	return "eventbus"
}

// Start starts the embedded NATS server and creates the EventBus.
func (s *Service) Start(ctx context.Context) error {
	ctx, span := s.tracer.Start(ctx, "eventbus.Start")
	defer span.End()

	s.logger.Info("starting eventbus service")

	// Start embedded NATS server
	s.logger.Debug("starting embedded NATS server")
	srv, err := nats.StartEmbeddedServer()
	if err != nil {
		observability.SetSpanError(ctx, err)
		s.logger.Error("failed to start embedded NATS", "error", err)
		return fmt.Errorf("failed to start embedded NATS: %w", err)
	}
	s.server = srv

	// Update config to use embedded server URL
	s.config.URL = srv.URL()

	// Create EventBus connected to embedded server
	s.logger.Debug("creating event bus",
		"stream", s.config.StreamName,
		"subjects", s.config.StreamSubjects)

	bus, err := natseventbus.NewEventBus(s.config)
	if err != nil {
		srv.Shutdown()
		observability.SetSpanError(ctx, err)
		s.logger.Error("failed to create event bus", "error", err)
		return fmt.Errorf("failed to create event bus: %w", err)
	}
	s.bus = bus

	span.SetAttributes(
		attribute.String("nats.url", srv.URL()),
		attribute.String("stream.name", s.config.StreamName),
		attribute.Int64("stream.max_bytes", s.config.MaxBytes),
		attribute.String("stream.max_age", s.config.MaxAge.String()),
	)

	s.logger.Info("eventbus service started",
		"url", srv.URL(),
		"stream", s.config.StreamName)

	return nil
}

// Stop gracefully shuts down the EventBus and embedded NATS server.
// Uses the proper shutdown order: EventBus first, then server.
func (s *Service) Stop(ctx context.Context) error {
	ctx, span := s.tracer.Start(ctx, "eventbus.Stop")
	defer span.End()

	s.logger.Info("stopping eventbus service")

	if s.bus != nil {
		// Close EventBus first (closes all connections)
		s.logger.Debug("closing event bus")
		if err := s.bus.Close(); err != nil {
			s.logger.Warn("error closing event bus", "error", err)
			// Continue with shutdown even if close fails
		}

		// Give connections time to drain
		s.logger.Debug("draining connections")
		time.Sleep(100 * time.Millisecond)
	}

	if s.server != nil {
		// Then shutdown server
		s.logger.Debug("shutting down NATS server")
		s.server.Shutdown()
	}

	s.logger.Info("eventbus service stopped")
	return nil
}

// HealthCheck checks if both the NATS server and EventBus are healthy.
func (s *Service) HealthCheck(ctx context.Context) error {
	ctx, span := s.tracer.Start(ctx, "eventbus.HealthCheck")
	defer span.End()

	if s.server == nil {
		err := fmt.Errorf("nats server not started")
		observability.SetSpanError(ctx, err)
		return err
	}

	if s.bus == nil {
		err := fmt.Errorf("event bus not created")
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

// EventBus returns the EventBus instance.
// Only available after Start() succeeds.
func (s *Service) EventBus() *natseventbus.EventBus {
	return s.bus
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
