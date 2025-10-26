package config

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"gocloud.dev/runtimevar"
	// Cloud provider imports are opt-in - import in your application code:
	// _ "gocloud.dev/runtimevar/awsparamstore"    // AWS Parameter Store
	// _ "gocloud.dev/runtimevar/gcpruntimeconfig" // GCP Runtime Configurator
	// _ "gocloud.dev/runtimevar/azureappconfig"   // Azure App Configuration
	// _ "gocloud.dev/runtimevar/etcdvar"          // etcd
	// _ "gocloud.dev/runtimevar/filevar"          // Local file
	// _ "gocloud.dev/runtimevar/constantvar"      // Constant (testing)
)

// RuntimeVarProvider implements Provider using Go Cloud Development Kit
type RuntimeVarProvider[T any] struct {
	variable   *runtimevar.Variable
	config     ProviderConfig

	// Cache
	mu         sync.RWMutex
	latest     *Snapshot[T]

	// Lifecycle
	closed     bool
	closeOnce  sync.Once
}

// NewProvider creates a new configuration provider using Go Cloud runtimevar
//
// URL formats:
//   - AWS Parameter Store: "awsparamstore:///path/to/config?decoder=json"
//   - GCP Runtime Configurator: "gcpruntimeconfig://projects/PROJECT/configs/CONFIG/variables/VAR?decoder=json"
//   - Azure App Configuration: "azureappconfig://connection-string?key=KEY&decoder=json"
//   - etcd: "etcd://localhost:2379/config?decoder=json"
//   - Local file: "file:///path/to/config.json?decoder=json"
//   - Constant (testing): "constant://?val={"key":"value"}&decoder=json"
//
// Decoders:
//   - json: JSON decoder (default)
//   - jsonmap: JSON decoder that decodes into map[string]interface{}
//   - string: Raw string decoder
//   - bytes: Raw bytes decoder
//
// Example:
//
//	// Production: AWS Parameter Store
//	provider, err := NewProvider[AppConfig](ctx, "awsparamstore:///prod/myapp/config?decoder=json")
//
//	// Development: Local file with auto-reload
//	provider, err := NewProvider[AppConfig](ctx, "file:///etc/myapp/config.json?decoder=json")
func NewProvider[T any](ctx context.Context, url string) (*RuntimeVarProvider[T], error) {
	config := DefaultConfig()
	config.URL = url
	return NewProviderWithConfig[T](ctx, config)
}

// NewProviderWithConfig creates a provider with custom configuration
func NewProviderWithConfig[T any](ctx context.Context, config ProviderConfig) (*RuntimeVarProvider[T], error) {
	if config.URL == "" {
		return nil, &ConfigError{
			Op:  "open",
			Err: fmt.Errorf("configuration URL is required"),
		}
	}

	// Open the variable using Go Cloud
	variable, err := runtimevar.OpenVariable(ctx, config.URL)
	if err != nil {
		return nil, &ConfigError{
			Op:  "open",
			URL: config.URL,
			Err: err,
		}
	}

	provider := &RuntimeVarProvider[T]{
		variable: variable,
		config:   config,
	}

	// Load initial configuration
	if err := provider.loadConfig(ctx); err != nil {
		variable.Close()
		return nil, &ConfigError{
			Op:  "load",
			URL: config.URL,
			Err: err,
		}
	}

	return provider, nil
}

// Get retrieves the current configuration
func (p *RuntimeVarProvider[T]) Get(ctx context.Context) (T, error) {
	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		var zero T
		return zero, ErrProviderClosed
	}
	p.mu.RUnlock()

	if err := p.loadConfig(ctx); err != nil {
		var zero T
		return zero, err
	}

	return p.Latest()
}

// Latest returns the most recently retrieved configuration without fetching
func (p *RuntimeVarProvider[T]) Latest() (T, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.closed {
		var zero T
		return zero, ErrProviderClosed
	}

	if p.latest == nil {
		var zero T
		return zero, &ConfigError{
			Op:  "latest",
			Err: fmt.Errorf("no configuration loaded"),
		}
	}

	return p.latest.Value, nil
}

// Watch monitors configuration changes and calls the handler on updates
func (p *RuntimeVarProvider[T]) Watch(ctx context.Context, handler func(T)) (func(), error) {
	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		return nil, ErrProviderClosed
	}
	p.mu.RUnlock()

	// Create a context for the watch goroutine
	watchCtx, cancel := context.WithCancel(ctx)

	// Start watching
	errCh := make(chan error, 1)
	go func() {
		ticker := time.NewTicker(p.config.WatchInterval)
		defer ticker.Stop()

		for {
			select {
			case <-watchCtx.Done():
				errCh <- watchCtx.Err()
				return

			case <-ticker.C:
				// Check for updates
				snapshot, err := p.variable.Latest(watchCtx)
				if err != nil {
					// Log error but continue watching
					continue
				}

				p.mu.Lock()
				// Check if value changed
				needsUpdate := p.latest == nil || snapshot.UpdateTime.After(p.latest.UpdateTime)
				p.mu.Unlock()

				if needsUpdate {
					if err := p.loadConfig(watchCtx); err != nil {
						// Log error but continue watching
						continue
					}

					p.mu.RLock()
					value := p.latest.Value
					p.mu.RUnlock()

					// Call handler
					handler(value)
				}
			}
		}
	}()

	// Return stop function
	stop := func() {
		cancel()
	}

	return stop, nil
}

// loadConfig loads and decodes configuration from the backend
func (p *RuntimeVarProvider[T]) loadConfig(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return ErrProviderClosed
	}

	// Get latest snapshot from runtimevar
	snapshot, err := p.variable.Latest(ctx)
	if err != nil {
		return &ConfigError{
			Op:  "fetch",
			URL: p.config.URL,
			Err: err,
		}
	}

	// Decode value - runtimevar returns the raw value (usually []byte)
	var value T
	rawValue := snapshot.Value

	// Try to decode based on type
	switch v := rawValue.(type) {
	case []byte:
		// Decode JSON bytes
		if err := json.Unmarshal(v, &value); err != nil {
			return &ConfigError{
				Op:  "decode",
				URL: p.config.URL,
				Err: err,
			}
		}
	case string:
		// Decode JSON string
		if err := json.Unmarshal([]byte(v), &value); err != nil {
			return &ConfigError{
				Op:  "decode",
				URL: p.config.URL,
				Err: err,
			}
		}
	default:
		// Try type assertion
		if typedValue, ok := rawValue.(T); ok {
			value = typedValue
		} else {
			return &ConfigError{
				Op:  "decode",
				URL: p.config.URL,
				Err: fmt.Errorf("unsupported value type: %T", rawValue),
			}
		}
	}

	// Validate if the type implements Validator
	if validator, ok := any(value).(Validator); ok {
		if err := validator.Validate(); err != nil {
			return &ConfigError{
				Op:  "validate",
				URL: p.config.URL,
				Err: err,
			}
		}
	}

	// Update cache
	p.latest = &Snapshot[T]{
		Value:      value,
		UpdateTime: snapshot.UpdateTime,
		Metadata:   make(map[string]string),
	}

	return nil
}

// Close releases resources
func (p *RuntimeVarProvider[T]) Close() error {
	var err error
	p.closeOnce.Do(func() {
		p.mu.Lock()
		p.closed = true
		p.mu.Unlock()

		if p.variable != nil {
			err = p.variable.Close()
		}
	})
	return err
}

// HealthCheck checks if the provider can access configuration
func (p *RuntimeVarProvider[T]) HealthCheck(ctx context.Context) error {
	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		return ErrProviderClosed
	}
	p.mu.RUnlock()

	// Try to get latest value
	_, err := p.variable.Latest(ctx)
	if err != nil {
		return &ConfigError{
			Op:  "health_check",
			URL: p.config.URL,
			Err: err,
		}
	}

	return nil
}
