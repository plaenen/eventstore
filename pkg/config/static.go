package config

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// StaticProvider provides configuration from a static value
// USE ONLY FOR DEVELOPMENT AND TESTING
type StaticProvider[T any] struct {
	mu       sync.RWMutex
	snapshot Snapshot[T]
	closed   bool
}

// NewStaticProvider creates a provider with a static configuration value
// USE ONLY FOR DEVELOPMENT AND TESTING
func NewStaticProvider[T any](value T) *StaticProvider[T] {
	return &StaticProvider[T]{
		snapshot: Snapshot[T]{
			Value:      value,
			UpdateTime: time.Now(),
			Metadata: map[string]string{
				"provider": "static",
				"warning":  "USE ONLY FOR DEVELOPMENT",
			},
		},
	}
}

// Get returns the static configuration
func (p *StaticProvider[T]) Get(ctx context.Context) (T, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.closed {
		var zero T
		return zero, ErrProviderClosed
	}

	return p.snapshot.Value, nil
}

// Latest returns the static configuration
func (p *StaticProvider[T]) Latest() (T, error) {
	return p.Get(context.Background())
}

// Watch is not supported for static providers (configuration never changes)
func (p *StaticProvider[T]) Watch(ctx context.Context, handler func(T)) (func(), error) {
	// Call handler once with current value
	p.mu.RLock()
	value := p.snapshot.Value
	p.mu.RUnlock()

	go handler(value)

	// Return no-op stop function
	return func() {}, nil
}

// Update allows changing the static value (useful for testing)
func (p *StaticProvider[T]) Update(value T) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.snapshot = Snapshot[T]{
		Value:      value,
		UpdateTime: time.Now(),
		Metadata:   p.snapshot.Metadata,
	}
}

// Close releases resources (no-op for static)
func (p *StaticProvider[T]) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.closed = true
	return nil
}

// EnvProvider provides configuration from environment variables
// More dynamic than static since env vars can be injected at runtime
type EnvProvider[T any] struct {
	varName   string
	decoder   func(string) (T, error)
	mu        sync.RWMutex
	cached    *Snapshot[T]
	cacheTime time.Time
	cacheTTL  time.Duration
}

// NewEnvProvider creates a provider that reads configuration from environment variable
// The decoder function should parse the environment variable value into T
func NewEnvProvider[T any](varName string, decoder func(string) (T, error), cacheTTL time.Duration) *EnvProvider[T] {
	return &EnvProvider[T]{
		varName:  varName,
		decoder:  decoder,
		cacheTTL: cacheTTL,
	}
}

// NewEnvJSONProvider creates a provider that reads JSON from an environment variable
func NewEnvJSONProvider[T any](varName string, cacheTTL time.Duration) *EnvProvider[T] {
	decoder := func(s string) (T, error) {
		var value T
		if err := json.Unmarshal([]byte(s), &value); err != nil {
			return value, fmt.Errorf("failed to decode JSON: %w", err)
		}
		return value, nil
	}

	return NewEnvProvider(varName, decoder, cacheTTL)
}

// Get reads configuration from environment variable
func (p *EnvProvider[T]) Get(ctx context.Context) (T, error) {
	p.mu.RLock()
	if p.cached != nil && time.Since(p.cacheTime) < p.cacheTTL {
		value := p.cached.Value
		p.mu.RUnlock()
		return value, nil
	}
	p.mu.RUnlock()

	// Read from environment
	envValue := os.Getenv(p.varName)
	if envValue == "" {
		var zero T
		return zero, &ConfigError{
			Op:  "get",
			Err: fmt.Errorf("environment variable %s not set", p.varName),
		}
	}

	// Decode
	value, err := p.decoder(envValue)
	if err != nil {
		var zero T
		return zero, &ConfigError{
			Op:  "decode",
			Err: fmt.Errorf("failed to decode %s: %w", p.varName, err),
		}
	}

	// Validate if type implements Validator
	if validator, ok := any(value).(Validator); ok {
		if err := validator.Validate(); err != nil {
			var zero T
			return zero, &ConfigError{
				Op:  "validate",
				Err: err,
			}
		}
	}

	// Update cache
	p.mu.Lock()
	p.cached = &Snapshot[T]{
		Value:      value,
		UpdateTime: time.Now(),
		Metadata: map[string]string{
			"provider": "environment",
			"var_name": p.varName,
		},
	}
	p.cacheTime = time.Now()
	p.mu.Unlock()

	return value, nil
}

// Latest returns the cached value if available, otherwise reads from environment
func (p *EnvProvider[T]) Latest() (T, error) {
	p.mu.RLock()
	if p.cached != nil {
		value := p.cached.Value
		p.mu.RUnlock()
		return value, nil
	}
	p.mu.RUnlock()

	return p.Get(context.Background())
}

// Watch monitors environment variable changes (by polling)
func (p *EnvProvider[T]) Watch(ctx context.Context, handler func(T)) (func(), error) {
	watchCtx, cancel := context.WithCancel(ctx)

	go func() {
		ticker := time.NewTicker(p.cacheTTL)
		defer ticker.Stop()

		var lastValue T
		initialized := false

		for {
			select {
			case <-watchCtx.Done():
				return
			case <-ticker.C:
				value, err := p.Get(watchCtx)
				if err != nil {
					continue
				}

				// Check if value changed
				valueJSON, _ := json.Marshal(value)
				lastValueJSON, _ := json.Marshal(lastValue)

				if !initialized || string(valueJSON) != string(lastValueJSON) {
					lastValue = value
					initialized = true
					handler(value)
				}
			}
		}
	}()

	return cancel, nil
}

// Close releases resources
func (p *EnvProvider[T]) Close() error {
	return nil
}

// ChainProvider tries multiple providers in order until one succeeds
// Useful for fallback scenarios (e.g., try runtime config, fall back to env)
type ChainProvider[T any] struct {
	providers []Provider[T]
}

// NewChainProvider creates a provider that chains multiple providers
func NewChainProvider[T any](providers ...Provider[T]) *ChainProvider[T] {
	return &ChainProvider[T]{
		providers: providers,
	}
}

// Get tries each provider in order
func (p *ChainProvider[T]) Get(ctx context.Context) (T, error) {
	var lastErr error

	for i, provider := range p.providers {
		value, err := provider.Get(ctx)
		if err == nil {
			return value, nil
		}
		lastErr = fmt.Errorf("provider %d failed: %w", i, err)
	}

	if lastErr != nil {
		var zero T
		return zero, &ConfigError{
			Op:  "chain",
			Err: fmt.Errorf("all providers failed: %w", lastErr),
		}
	}

	var zero T
	return zero, &ConfigError{
		Op:  "chain",
		Err: fmt.Errorf("no providers configured"),
	}
}

// Latest tries each provider in order
func (p *ChainProvider[T]) Latest() (T, error) {
	var lastErr error

	for i, provider := range p.providers {
		value, err := provider.Latest()
		if err == nil {
			return value, nil
		}
		lastErr = fmt.Errorf("provider %d failed: %w", i, err)
	}

	if lastErr != nil {
		var zero T
		return zero, fmt.Errorf("all providers failed: %w", lastErr)
	}

	var zero T
	return zero, fmt.Errorf("no providers configured")
}

// Watch watches the first successful provider
func (p *ChainProvider[T]) Watch(ctx context.Context, handler func(T)) (func(), error) {
	for _, provider := range p.providers {
		stop, err := provider.Watch(ctx, handler)
		if err == nil {
			return stop, nil
		}
	}

	return nil, &ConfigError{
		Op:  "watch",
		Err: fmt.Errorf("no provider supports watching"),
	}
}

// Close closes all providers
func (p *ChainProvider[T]) Close() error {
	var errs []error

	for _, provider := range p.providers {
		if err := provider.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return &ConfigError{
			Op:  "close",
			Err: fmt.Errorf("failed to close %d provider(s): %v", len(errs), errs),
		}
	}

	return nil
}
