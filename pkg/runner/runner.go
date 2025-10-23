package runner

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Logger is a simple logging interface that the runner uses.
// Implementations can wrap any logging library (zap, logrus, slog, etc).
type Logger interface {
	Info(msg string, keysAndValues ...interface{})
	Error(msg string, keysAndValues ...interface{})
	Debug(msg string, keysAndValues ...interface{})
}

// Runner manages the lifecycle of multiple services.
// It handles concurrent startup, graceful shutdown, and error aggregation.
type Runner struct {
	services        []Service
	logger          Logger
	shutdownTimeout time.Duration
	startupTimeout  time.Duration
}

// Option configures a Runner.
type Option func(*Runner)

// WithLogger sets the logger for the runner.
func WithLogger(logger Logger) Option {
	return func(r *Runner) {
		r.logger = logger
	}
}

// WithShutdownTimeout sets the timeout for graceful shutdown.
// Default is 30 seconds.
func WithShutdownTimeout(timeout time.Duration) Option {
	return func(r *Runner) {
		r.shutdownTimeout = timeout
	}
}

// WithStartupTimeout sets the timeout for service startup.
// Default is 1 minute.
func WithStartupTimeout(timeout time.Duration) Option {
	return func(r *Runner) {
		r.startupTimeout = timeout
	}
}

// New creates a new Runner with the given services and options.
func New(services []Service, opts ...Option) *Runner {
	r := &Runner{
		services:        services,
		logger:          &noopLogger{},
		shutdownTimeout: 30 * time.Second,
		startupTimeout:  1 * time.Minute,
	}

	for _, opt := range opts {
		opt(r)
	}

	return r
}

// Run starts all services and blocks until the context is cancelled
// or a service fails. It handles graceful shutdown on context cancellation.
//
// Services are started sequentially in the order they were registered.
// On shutdown, services are stopped in reverse order.
func (r *Runner) Run(ctx context.Context) error {
	// Setup signal handling
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Channel to receive shutdown signals
	shutdownCh := make(chan struct{})
	go func() {
		WaitForShutdownSignal()
		r.logger.Info("shutdown signal received")
		cancel()
		close(shutdownCh)
	}()

	// Start all services
	r.logger.Info("starting services", "count", len(r.services))
	started := []Service{}

	for _, service := range r.services {
		r.logger.Info("starting service", "service", service.Name())

		startCtx, startCancel := context.WithTimeout(ctx, r.startupTimeout)
		err := service.Start(startCtx)
		startCancel()

		if err != nil {
			r.logger.Error("failed to start service",
				"service", service.Name(),
				"error", err)

			// Stop already started services
			r.stopServices(started)
			return fmt.Errorf("start service %s: %w", service.Name(), err)
		}

		started = append(started, service)
		r.logger.Info("service started", "service", service.Name())
	}

	r.logger.Info("all services started successfully")

	// Wait for shutdown signal or context cancellation
	<-ctx.Done()

	// Graceful shutdown
	r.logger.Info("shutting down services gracefully",
		"timeout", r.shutdownTimeout)

	return r.stopServices(started)
}

// stopServices stops all services in reverse order with timeout.
func (r *Runner) stopServices(services []Service) error {
	if len(services) == 0 {
		return nil
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), r.shutdownTimeout)
	defer cancel()

	// Stop in reverse order
	var wg sync.WaitGroup
	errCh := make(chan error, len(services))

	for i := len(services) - 1; i >= 0; i-- {
		service := services[i]

		wg.Add(1)
		go func(svc Service) {
			defer wg.Done()

			r.logger.Info("stopping service", "service", svc.Name())

			if err := svc.Stop(shutdownCtx); err != nil {
				r.logger.Error("error stopping service",
					"service", svc.Name(),
					"error", err)
				errCh <- fmt.Errorf("stop %s: %w", svc.Name(), err)
				return
			}

			r.logger.Info("service stopped", "service", svc.Name())
		}(service)
	}

	// Wait for all services to stop or timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		close(errCh)
		// Collect any errors
		var errs []error
		for err := range errCh {
			errs = append(errs, err)
		}
		if len(errs) > 0 {
			return fmt.Errorf("shutdown errors: %v", errs)
		}
		r.logger.Info("all services stopped successfully")
		return nil

	case <-shutdownCtx.Done():
		r.logger.Error("shutdown timeout exceeded",
			"timeout", r.shutdownTimeout)
		return fmt.Errorf("shutdown timeout exceeded")
	}
}

// HealthCheck checks the health of all services that implement HealthChecker.
func (r *Runner) HealthCheck(ctx context.Context) error {
	for _, service := range r.services {
		if hc, ok := service.(HealthChecker); ok {
			if err := hc.HealthCheck(ctx); err != nil {
				return fmt.Errorf("service %s unhealthy: %w", service.Name(), err)
			}
		}
	}
	return nil
}

// noopLogger is a no-op logger implementation.
type noopLogger struct{}

func (noopLogger) Info(msg string, keysAndValues ...interface{})  {}
func (noopLogger) Error(msg string, keysAndValues ...interface{}) {}
func (noopLogger) Debug(msg string, keysAndValues ...interface{}) {}
