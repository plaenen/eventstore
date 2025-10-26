// Package config provides dynamic configuration management using Go Cloud Development Kit.
//
// This package wraps gocloud.dev/runtimevar to provide a vendor-agnostic configuration
// solution that works across AWS Parameter Store, GCP Runtime Configurator, Azure App
// Configuration, etcd, and local development.
//
// Example usage:
//
//	// Production: AWS Parameter Store
//	provider, err := config.NewProvider[MyConfig](ctx, "awsparamstore:///prod/app-config")
//
//	// Development: Local file
//	provider, err := config.NewProvider[MyConfig](ctx, "file:///etc/config.json?decoder=json")
//
//	// Get configuration
//	cfg, err := provider.Get(ctx)
//
//	// Watch for changes
//	provider.Watch(ctx, func(cfg MyConfig) {
//		log.Printf("Config updated: %+v", cfg)
//	})
package config

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var (
	// ErrProviderClosed is returned when attempting to use a closed provider
	ErrProviderClosed = errors.New("provider is closed")

	// ErrInvalidConfig is returned when configuration is malformed
	ErrInvalidConfig = errors.New("invalid configuration")

	// ErrWatchStopped is returned when the watch has been stopped
	ErrWatchStopped = errors.New("watch stopped")
)

// Provider defines the interface for configuration providers
type Provider[T any] interface {
	// Get retrieves the current configuration
	Get(ctx context.Context) (T, error)

	// Watch monitors configuration changes and calls the handler on updates
	// Returns a function to stop watching
	Watch(ctx context.Context, handler func(T)) (stop func(), err error)

	// Latest returns the most recently retrieved configuration without fetching
	Latest() (T, error)

	// Close releases any resources held by the provider
	Close() error
}

// Validator is an optional interface that configuration types can implement
// to validate themselves after deserialization
type Validator interface {
	Validate() error
}

// ProviderConfig configures configuration providers
type ProviderConfig struct {
	// URL is the configuration backend URL (e.g., "awsparamstore://...", "file://...")
	URL string

	// WatchInterval is how often to poll for changes (default: 30 seconds)
	// Only used for providers that don't support native watching
	WatchInterval time.Duration

	// Decoder specifies how to decode the configuration (json, yaml, etc.)
	// Usually inferred from URL query parameters
	Decoder string
}

// DefaultConfig returns a default provider configuration
func DefaultConfig() ProviderConfig {
	return ProviderConfig{
		WatchInterval: 30 * time.Second,
		Decoder:       "json",
	}
}

// Snapshot represents a point-in-time configuration value
type Snapshot[T any] struct {
	// Value is the configuration value
	Value T

	// UpdateTime is when this configuration was last updated
	UpdateTime time.Time

	// Version is an opaque version identifier (if supported by backend)
	Version string

	// Metadata contains additional information about the configuration
	Metadata map[string]string
}

// WatchFunc is called when configuration changes
type WatchFunc[T any] func(snapshot Snapshot[T])

// Updater is an optional interface for providers that support updating configuration
type Updater[T any] interface {
	Provider[T]

	// Update stores a new configuration value
	Update(ctx context.Context, value T) error
}

// HealthChecker is an optional interface for providers that support health checks
type HealthChecker interface {
	// HealthCheck returns an error if the provider is unhealthy
	HealthCheck(ctx context.Context) error
}

// ConfigError wraps configuration errors with context
type ConfigError struct {
	Op  string // Operation that failed
	URL string // Configuration URL
	Err error  // Underlying error
}

func (e *ConfigError) Error() string {
	if e.URL != "" {
		return fmt.Sprintf("config %s [%s]: %v", e.Op, e.URL, e.Err)
	}
	return fmt.Sprintf("config %s: %v", e.Op, e.Err)
}

func (e *ConfigError) Unwrap() error {
	return e.Err
}
