package config

import (
	"fmt"
	"time"
)

// Common configuration types that applications can use or embed

// FeatureFlags represents feature toggle configuration
type FeatureFlags struct {
	// EnableNewUI enables the new user interface
	EnableNewUI bool `json:"enable_new_ui"`

	// EnableExperimentalFeatures enables experimental features
	EnableExperimentalFeatures bool `json:"enable_experimental_features"`

	// EnableDebugMode enables debug logging and endpoints
	EnableDebugMode bool `json:"enable_debug_mode"`

	// EnableMetrics enables metrics collection
	EnableMetrics bool `json:"enable_metrics"`

	// EnableTracing enables distributed tracing
	EnableTracing bool `json:"enable_tracing"`

	// Features is a map of arbitrary feature flags
	Features map[string]bool `json:"features,omitempty"`
}

// IsEnabled checks if a specific feature is enabled
func (f *FeatureFlags) IsEnabled(feature string) bool {
	if f.Features == nil {
		return false
	}
	return f.Features[feature]
}

// ServiceEndpoints represents service discovery configuration
type ServiceEndpoints struct {
	// NATSURL is the NATS server URL
	NATSURL string `json:"nats_url"`

	// DatabaseURL is the database connection string
	DatabaseURL string `json:"database_url"`

	// CacheURL is the cache server URL (Redis, Memcached, etc.)
	CacheURL string `json:"cache_url,omitempty"`

	// APIURL is the API server base URL
	APIURL string `json:"api_url,omitempty"`

	// MetricsURL is the metrics collection endpoint
	MetricsURL string `json:"metrics_url,omitempty"`

	// TracingURL is the tracing collector endpoint
	TracingURL string `json:"tracing_url,omitempty"`

	// Custom endpoints
	Endpoints map[string]string `json:"endpoints,omitempty"`
}

// Validate ensures required endpoints are configured
func (s *ServiceEndpoints) Validate() error {
	if s.NATSURL == "" {
		return fmt.Errorf("%w: nats_url is required", ErrInvalidConfig)
	}
	if s.DatabaseURL == "" {
		return fmt.Errorf("%w: database_url is required", ErrInvalidConfig)
	}
	return nil
}

// GetEndpoint retrieves a custom endpoint by name
func (s *ServiceEndpoints) GetEndpoint(name string) (string, bool) {
	if s.Endpoints == nil {
		return "", false
	}
	url, ok := s.Endpoints[name]
	return url, ok
}

// RuntimeTuning represents runtime performance tuning parameters
type RuntimeTuning struct {
	// MaxConcurrency is the maximum number of concurrent operations
	MaxConcurrency int `json:"max_concurrency"`

	// EventBatchSize is the number of events to process in a batch
	EventBatchSize int `json:"event_batch_size"`

	// ProjectionWorkers is the number of concurrent projection workers
	ProjectionWorkers int `json:"projection_workers"`

	// CacheTTL is the cache time-to-live
	CacheTTL time.Duration `json:"cache_ttl"`

	// RequestTimeout is the default request timeout
	RequestTimeout time.Duration `json:"request_timeout"`

	// IdleTimeout is the idle connection timeout
	IdleTimeout time.Duration `json:"idle_timeout"`

	// BufferSize is the size of various buffers
	BufferSize int `json:"buffer_size"`

	// Custom tuning parameters
	Parameters map[string]interface{} `json:"parameters,omitempty"`
}

// Validate ensures tuning parameters are reasonable
func (r *RuntimeTuning) Validate() error {
	if r.MaxConcurrency < 1 {
		return fmt.Errorf("%w: max_concurrency must be >= 1", ErrInvalidConfig)
	}
	if r.EventBatchSize < 1 {
		return fmt.Errorf("%w: event_batch_size must be >= 1", ErrInvalidConfig)
	}
	if r.ProjectionWorkers < 1 {
		return fmt.Errorf("%w: projection_workers must be >= 1", ErrInvalidConfig)
	}
	if r.CacheTTL < 0 {
		return fmt.Errorf("%w: cache_ttl cannot be negative", ErrInvalidConfig)
	}
	return nil
}

// GetParameter retrieves a custom parameter
func (r *RuntimeTuning) GetParameter(name string) (interface{}, bool) {
	if r.Parameters == nil {
		return nil, false
	}
	val, ok := r.Parameters[name]
	return val, ok
}

// RateLimits represents rate limiting configuration
type RateLimits struct {
	// RequestsPerSecond is the global rate limit
	RequestsPerSecond int `json:"requests_per_second"`

	// BurstSize is the maximum burst size
	BurstSize int `json:"burst_size"`

	// PerUserLimit is the rate limit per user
	PerUserLimit int `json:"per_user_limit,omitempty"`

	// PerIPLimit is the rate limit per IP address
	PerIPLimit int `json:"per_ip_limit,omitempty"`

	// Enabled indicates if rate limiting is enabled
	Enabled bool `json:"enabled"`
}

// Validate ensures rate limits are valid
func (r *RateLimits) Validate() error {
	if r.Enabled {
		if r.RequestsPerSecond < 1 {
			return fmt.Errorf("%w: requests_per_second must be >= 1 when enabled", ErrInvalidConfig)
		}
		if r.BurstSize < 1 {
			return fmt.Errorf("%w: burst_size must be >= 1 when enabled", ErrInvalidConfig)
		}
	}
	return nil
}

// SecurityConfig represents security-related configuration
type SecurityConfig struct {
	// AllowedOrigins for CORS
	AllowedOrigins []string `json:"allowed_origins"`

	// AllowedMethods for CORS
	AllowedMethods []string `json:"allowed_methods,omitempty"`

	// AllowedHeaders for CORS
	AllowedHeaders []string `json:"allowed_headers,omitempty"`

	// JWTSecret for JWT validation (reference to secret, not actual secret!)
	JWTSecretRef string `json:"jwt_secret_ref,omitempty"`

	// TLSEnabled indicates if TLS is enabled
	TLSEnabled bool `json:"tls_enabled"`

	// TLSCertRef references the TLS certificate (in secret manager)
	TLSCertRef string `json:"tls_cert_ref,omitempty"`

	// TLSKeyRef references the TLS key (in secret manager)
	TLSKeyRef string `json:"tls_key_ref,omitempty"`

	// RequireAuth indicates if authentication is required
	RequireAuth bool `json:"require_auth"`
}

// LoggingConfig represents logging configuration
type LoggingConfig struct {
	// Level is the log level (debug, info, warn, error)
	Level string `json:"level"`

	// Format is the log format (json, text)
	Format string `json:"format"`

	// Output is where to write logs (stdout, stderr, file path)
	Output string `json:"output"`

	// EnableStackTrace enables stack traces in error logs
	EnableStackTrace bool `json:"enable_stack_trace"`

	// SamplingRate for high-volume logs (0.0 to 1.0)
	SamplingRate float64 `json:"sampling_rate,omitempty"`
}

// Validate ensures logging config is valid
func (l *LoggingConfig) Validate() error {
	validLevels := map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
	}

	if !validLevels[l.Level] {
		return fmt.Errorf("%w: invalid log level %s", ErrInvalidConfig, l.Level)
	}

	if l.SamplingRate < 0 || l.SamplingRate > 1.0 {
		return fmt.Errorf("%w: sampling_rate must be between 0.0 and 1.0", ErrInvalidConfig)
	}

	return nil
}

// DatabaseConfig represents database configuration
type DatabaseConfig struct {
	// URL is the database connection string (reference to secret!)
	URLRef string `json:"url_ref"`

	// MaxConnections is the maximum number of connections
	MaxConnections int `json:"max_connections"`

	// MaxIdleConnections is the maximum number of idle connections
	MaxIdleConnections int `json:"max_idle_connections"`

	// ConnectionTimeout is the connection timeout
	ConnectionTimeout time.Duration `json:"connection_timeout"`

	// QueryTimeout is the default query timeout
	QueryTimeout time.Duration `json:"query_timeout"`

	// EnableSSL indicates if SSL is required
	EnableSSL bool `json:"enable_ssl"`

	// MigrationsEnabled indicates if migrations should run on startup
	MigrationsEnabled bool `json:"migrations_enabled"`
}

// Validate ensures database config is valid
func (d *DatabaseConfig) Validate() error {
	if d.MaxConnections < 1 {
		return fmt.Errorf("%w: max_connections must be >= 1", ErrInvalidConfig)
	}
	if d.MaxIdleConnections < 0 {
		return fmt.Errorf("%w: max_idle_connections cannot be negative", ErrInvalidConfig)
	}
	if d.MaxIdleConnections > d.MaxConnections {
		return fmt.Errorf("%w: max_idle_connections cannot exceed max_connections", ErrInvalidConfig)
	}
	return nil
}

// AppConfig is a comprehensive application configuration
// Applications can embed this or use it as a reference
type AppConfig struct {
	// Environment (dev, staging, prod)
	Environment string `json:"environment"`

	// ServiceName is the name of this service
	ServiceName string `json:"service_name"`

	// Version is the application version
	Version string `json:"version"`

	// FeatureFlags control feature toggles
	FeatureFlags FeatureFlags `json:"feature_flags"`

	// Endpoints for service discovery
	Endpoints ServiceEndpoints `json:"endpoints"`

	// Tuning for runtime performance
	Tuning RuntimeTuning `json:"tuning"`

	// RateLimits for API throttling
	RateLimits RateLimits `json:"rate_limits"`

	// Security configuration
	Security SecurityConfig `json:"security"`

	// Logging configuration
	Logging LoggingConfig `json:"logging"`

	// Database configuration
	Database DatabaseConfig `json:"database"`

	// Custom application-specific config
	Custom map[string]interface{} `json:"custom,omitempty"`
}

// Validate validates the entire application configuration
func (a *AppConfig) Validate() error {
	if a.Environment == "" {
		return fmt.Errorf("%w: environment is required", ErrInvalidConfig)
	}

	if a.ServiceName == "" {
		return fmt.Errorf("%w: service_name is required", ErrInvalidConfig)
	}

	// Validate nested configs
	if err := a.Endpoints.Validate(); err != nil {
		return err
	}

	if err := a.Tuning.Validate(); err != nil {
		return err
	}

	if err := a.RateLimits.Validate(); err != nil {
		return err
	}

	if err := a.Logging.Validate(); err != nil {
		return err
	}

	if err := a.Database.Validate(); err != nil {
		return err
	}

	return nil
}
