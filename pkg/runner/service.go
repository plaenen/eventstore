package runner

import "context"

// Service represents a service that can be started and stopped.
// Services are managed by the Runner and should implement graceful
// startup and shutdown semantics.
type Service interface {
	// Name returns a unique identifier for this service.
	// Used for logging and error messages.
	Name() string

	// Start initializes and starts the service.
	// Should block until the service is ready to accept requests.
	// Must respect context cancellation.
	Start(ctx context.Context) error

	// Stop gracefully shuts down the service.
	// Should complete within the context timeout.
	// Must respect context cancellation.
	Stop(ctx context.Context) error
}

// HealthChecker is an optional interface that services can implement
// to provide health check capabilities.
type HealthChecker interface {
	Service

	// HealthCheck returns an error if the service is unhealthy.
	HealthCheck(ctx context.Context) error
}

// ConfigurableService is an optional interface that services can implement
// to receive configuration updates at runtime.
type ConfigurableService[T any] interface {
	Service

	// UpdateConfig is called when configuration changes.
	// The service should apply the new configuration without restarting.
	// Return an error if the configuration cannot be applied.
	UpdateConfig(ctx context.Context, config T) error
}

// ConfigValidator is an optional interface that services can implement
// to validate configuration before it's applied.
type ConfigValidator[T any] interface {
	ConfigurableService[T]

	// ValidateConfig validates configuration without applying it.
	// Return an error if the configuration is invalid.
	ValidateConfig(config T) error
}
