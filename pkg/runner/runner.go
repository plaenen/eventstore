package runner

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"time"
)

// Runner manages the lifecycle of multiple services.
// It handles concurrent startup, graceful shutdown, error aggregation, and configuration updates.
type Runner struct {
	services        []Service
	logger          Logger
	shutdownTimeout time.Duration
	startupTimeout  time.Duration
	configProvider  interface{} // Generic config provider
	configWatchStop func()      // Function to stop config watching
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

// WithConfigProvider sets a configuration provider that watches for config changes.
// When configuration changes, all ConfigurableService instances will be notified.
// The provider must have a Watch(ctx, handler) method.
func WithConfigProvider(provider interface{}) Option {
	return func(r *Runner) {
		r.configProvider = provider
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

// startConfigWatch starts watching configuration changes using reflection
// to call the Watch method on the provider
func (r *Runner) startConfigWatch(ctx context.Context, services []Service) error {
	// Use reflection to call Watch method
	// This allows us to support any config.Provider[T] without type parameters
	providerValue := reflect.ValueOf(r.configProvider)
	watchMethod := providerValue.MethodByName("Watch")

	if !watchMethod.IsValid() {
		return fmt.Errorf("config provider does not have Watch method")
	}

	// Create a handler function that will be called on config updates
	handlerFunc := func(configValue reflect.Value) {
		r.logger.Info("configuration updated, notifying services")

		// Extract the actual config value
		config := configValue.Interface()

		// Notify all ConfigurableService instances
		for _, service := range services {
			if cs, ok := service.(ConfigurableService); ok {
				r.logger.Info("updating service configuration",
					"service", service.Name())

				// Validate first if service implements ConfigValidator
				if cv, ok := service.(ConfigValidator); ok {
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
	}

	// Create the handler function value that matches Watch's expected signature
	handlerType := watchMethod.Type().In(1)
	handler := reflect.MakeFunc(handlerType, func(args []reflect.Value) []reflect.Value {
		if len(args) > 0 {
			handlerFunc(args[0])
		}
		return nil
	})

	// Call Watch(ctx, handler)
	results := watchMethod.Call([]reflect.Value{
		reflect.ValueOf(ctx),
		handler,
	})

	// Check for error return
	if len(results) != 2 {
		return fmt.Errorf("Watch method has unexpected signature")
	}

	// Get stop function
	stopFunc := results[0]
	if stopFunc.IsNil() {
		return fmt.Errorf("Watch returned nil stop function")
	}

	// Store stop function
	r.configWatchStop = func() {
		stopFunc.Call(nil)
	}

	// Check for error
	errValue := results[1]
	if !errValue.IsNil() {
		return errValue.Interface().(error)
	}

	r.logger.Info("configuration watching started")
	return nil
}
