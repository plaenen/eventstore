package runner

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/plaenen/eventstore/pkg/config"
)

// Runner manages the lifecycle of multiple services.
// It handles concurrent startup, graceful shutdown, error aggregation, and configuration updates.
type Runner[T any] struct {
	services        []Service
	logger          Logger
	shutdownTimeout time.Duration
	startupTimeout  time.Duration
	configProvider  config.Provider[T]
	configWatchStop func()
}

// Option configures a Runner.
type Option[T any] func(*Runner[T])

// WithLogger sets the logger for the runner.
func WithLogger[T any](logger Logger) Option[T] {
	return func(r *Runner[T]) {
		r.logger = logger
	}
}

// WithShutdownTimeout sets the timeout for graceful shutdown.
// Default is 30 seconds.
func WithShutdownTimeout[T any](timeout time.Duration) Option[T] {
	return func(r *Runner[T]) {
		r.shutdownTimeout = timeout
	}
}

// WithStartupTimeout sets the timeout for service startup.
// Default is 1 minute.
func WithStartupTimeout[T any](timeout time.Duration) Option[T] {
	return func(r *Runner[T]) {
		r.startupTimeout = timeout
	}
}

// WithConfigProvider sets a configuration provider that watches for config changes.
// When configuration changes, all ConfigurableService instances will be notified.
func WithConfigProvider[T any](provider config.Provider[T]) Option[T] {
	return func(r *Runner[T]) {
		r.configProvider = provider
	}
}

// New creates a new Runner with the given services and options.
func New[T any](services []Service, opts ...Option[T]) *Runner[T] {
	r := &Runner[T]{
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
func (r *Runner[T]) Run(ctx context.Context) error {
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

	// Start config watching if provider is set
	if r.configProvider != nil {
		if err := r.startConfigWatch(ctx, started); err != nil {
			r.logger.Error("failed to start config watching",
				"error", err)
			// Continue anyway - config updates are optional
		}
	}

	// Wait for shutdown signal or context cancellation
	<-ctx.Done()

	// Stop config watching
	if r.configWatchStop != nil {
		r.configWatchStop()
	}

	// Graceful shutdown
	r.logger.Info("shutting down services gracefully",
		"timeout", r.shutdownTimeout)

	return r.stopServices(started)
}

// stopServices stops all services in reverse order with timeout.
// Services are stopped sequentially to respect dependencies.
func (r *Runner[T]) stopServices(services []Service) error {
	if len(services) == 0 {
		return nil
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), r.shutdownTimeout)
	defer cancel()

	var shutdownErr error
	var mu sync.Mutex

	// Stop in reverse order (LIFO)
	for i := len(services) - 1; i >= 0; i-- {
		service := services[i]
		r.logger.Info("stopping service", "service", service.Name())

		// Stop sequentially
		if err := service.Stop(shutdownCtx); err != nil {
			r.logger.Error("error stopping service",
				"service", service.Name(),
				"error", err)

			mu.Lock()
			if shutdownErr == nil {
				shutdownErr = fmt.Errorf("stop %s: %w", service.Name(), err)
			}
			mu.Unlock()
		} else {
			r.logger.Info("service stopped", "service", service.Name())
		}
	}

	if shutdownCtx.Err() != nil {
		r.logger.Error("shutdown timeout exceeded",
			"timeout", r.shutdownTimeout)
		return fmt.Errorf("shutdown timeout exceeded")
	}

	if shutdownErr != nil {
		return shutdownErr
	}

	r.logger.Info("all services stopped successfully")
	return nil
}

// HealthCheck checks the health of all services that implement HealthChecker.
func (r *Runner[T]) HealthCheck(ctx context.Context) error {
	for _, service := range r.services {
		if hc, ok := service.(HealthChecker); ok {
			if err := hc.HealthCheck(ctx); err != nil {
				return fmt.Errorf("service %s unhealthy: %w", service.Name(), err)
			}
		}
	}
	return nil
}

// startConfigWatch starts watching configuration changes
func (r *Runner[T]) startConfigWatch(ctx context.Context, services []Service) error {
	stop, err := r.configProvider.Watch(ctx, func(config T) {
		r.logger.Info("configuration updated, notifying services")

		// Notify all ConfigurableService instances
		for _, service := range services {
			if cs, ok := service.(ConfigurableService[T]); ok {
				r.logger.Info("updating service configuration",
					"service", service.Name())

				// Validate first if service implements ConfigValidator
				if cv, ok := service.(ConfigValidator[T]); ok {
					if err := cv.ValidateConfig(config); err != nil {
						r.logger.Error("config validation failed",
							"service", service.Name(),
							"error", err)
						continue
					}
				}

				// Apply configuration
				updateCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				if err := cs.UpdateConfig(updateCtx, config); err != nil {
					r.logger.Error("failed to update service configuration",
						"service", service.Name(),
						"error", err)
				} else {
					r.logger.Info("service configuration updated",
						"service", service.Name())
				}
				cancel()
			}
		}
	})

	if err != nil {
		return err
	}

	r.configWatchStop = stop
	r.logger.Info("configuration watching started")
	return nil
}
