package retry

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/plaenen/eventstore/pkg/errorx"
)

// Config holds retry configuration.
type Config struct {
	// MaxAttempts is the maximum number of retry attempts (including the first attempt).
	// Default: 3
	MaxAttempts int

	// InitialBackoff is the initial backoff duration.
	// Default: 100ms
	InitialBackoff time.Duration

	// MaxBackoff is the maximum backoff duration.
	// Default: 10s
	MaxBackoff time.Duration

	// BackoffMultiplier is the multiplier for exponential backoff.
	// Default: 2.0
	BackoffMultiplier float64

	// Jitter adds randomness to backoff to avoid thundering herd.
	// Default: true
	Jitter bool

	// ShouldRetry is a custom function to determine if an error should be retried.
	// If nil, uses errors.IsRetryable() by default.
	ShouldRetry func(error) bool
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		MaxAttempts:       3,
		InitialBackoff:    100 * time.Millisecond,
		MaxBackoff:        10 * time.Second,
		BackoffMultiplier: 2.0,
		Jitter:            true,
		ShouldRetry:       nil, // Will use errorx.IsRetryable()
	}
}

// Operation is a function that may fail and needs retry logic.
type Operation func(ctx context.Context) error

// Do executes an operation with retry logic using the default configuration.
//
// It retries operations that return retryable errors (determined by errorx.IsRetryable()).
// Uses exponential backoff with jitter to avoid thundering herd.
//
// Example:
//
//	err := retry.Do(ctx, func(ctx context.Context) error {
//	    return repository.Save(aggregate)
//	})
func Do(ctx context.Context, op Operation) error {
	return DoWithConfig(ctx, DefaultConfig(), op)
}

// DoWithConfig executes an operation with retry logic using custom configuration.
//
// Example:
//
//	config := retry.Config{
//	    MaxAttempts:    5,
//	    InitialBackoff: 50 * time.Millisecond,
//	    MaxBackoff:     5 * time.Second,
//	}
//	err := retry.DoWithConfig(ctx, config, func(ctx context.Context) error {
//	    return repository.Save(aggregate)
//	})
func DoWithConfig(ctx context.Context, cfg Config, op Operation) error {
	// Set default shouldRetry function if not provided
	shouldRetry := cfg.ShouldRetry
	if shouldRetry == nil {
		shouldRetry = errorx.IsRetryable
	}

	var lastErr error
	backoff := cfg.InitialBackoff

	for attempt := 0; attempt < cfg.MaxAttempts; attempt++ {
		// Check context before each attempt
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("retry aborted: %w", err)
		}

		// Execute the operation
		lastErr = op(ctx)
		if lastErr == nil {
			return nil // Success!
		}

		// Check if we should retry this error
		if !shouldRetry(lastErr) {
			return lastErr // Not retryable - fail immediately
		}

		// Don't sleep after the last attempt
		if attempt < cfg.MaxAttempts-1 {
			sleepDuration := calculateBackoff(backoff, cfg.MaxBackoff, cfg.Jitter)

			select {
			case <-time.After(sleepDuration):
				// Continue to next attempt
			case <-ctx.Done():
				return fmt.Errorf("retry aborted after attempt %d: %w", attempt+1, ctx.Err())
			}

			// Increase backoff for next iteration
			backoff = time.Duration(float64(backoff) * cfg.BackoffMultiplier)
		}
	}

	// All attempts exhausted
	return fmt.Errorf("operation failed after %d attempts: %w", cfg.MaxAttempts, lastErr)
}

// OperationWithResult is a function that returns a result and may need retry logic.
type OperationWithResult[T any] func(ctx context.Context) (T, error)

// DoWithResult executes an operation that returns a result, with retry logic.
//
// Example:
//
//	aggregate, err := retry.DoWithResult(ctx, func(ctx context.Context) (*Aggregate, error) {
//	    return repository.Load(aggregateID)
//	})
func DoWithResult[T any](ctx context.Context, op OperationWithResult[T]) (T, error) {
	return DoWithResultAndConfig(ctx, DefaultConfig(), op)
}

// DoWithResultAndConfig executes an operation with result using custom configuration.
//
// Example:
//
//	config := retry.Config{MaxAttempts: 5}
//	aggregate, err := retry.DoWithResultAndConfig(ctx, config, func(ctx context.Context) (*Aggregate, error) {
//	    return repository.Load(aggregateID)
//	})
func DoWithResultAndConfig[T any](ctx context.Context, cfg Config, op OperationWithResult[T]) (T, error) {
	// Set default shouldRetry function if not provided
	shouldRetry := cfg.ShouldRetry
	if shouldRetry == nil {
		shouldRetry = errorx.IsRetryable
	}

	var result T
	var lastErr error
	backoff := cfg.InitialBackoff

	for attempt := 0; attempt < cfg.MaxAttempts; attempt++ {
		// Check context before each attempt
		if err := ctx.Err(); err != nil {
			return result, fmt.Errorf("retry aborted: %w", err)
		}

		// Execute the operation
		result, lastErr = op(ctx)
		if lastErr == nil {
			return result, nil // Success!
		}

		// Check if we should retry this error
		if !shouldRetry(lastErr) {
			return result, lastErr // Not retryable - fail immediately
		}

		// Don't sleep after the last attempt
		if attempt < cfg.MaxAttempts-1 {
			sleepDuration := calculateBackoff(backoff, cfg.MaxBackoff, cfg.Jitter)

			select {
			case <-time.After(sleepDuration):
				// Continue to next attempt
			case <-ctx.Done():
				return result, fmt.Errorf("retry aborted after attempt %d: %w", attempt+1, ctx.Err())
			}

			// Increase backoff for next iteration
			backoff = time.Duration(float64(backoff) * cfg.BackoffMultiplier)
		}
	}

	// All attempts exhausted
	return result, fmt.Errorf("operation failed after %d attempts: %w", cfg.MaxAttempts, lastErr)
}

// calculateBackoff calculates the next backoff duration with optional jitter
func calculateBackoff(backoff, maxBackoff time.Duration, jitter bool) time.Duration {
	if backoff > maxBackoff {
		backoff = maxBackoff
	}

	if jitter {
		// Add random jitter (±25%)
		jitterAmount := float64(backoff) * 0.25
		jitterOffset := (rand.Float64() * jitterAmount * 2) - jitterAmount
		backoff = time.Duration(float64(backoff) + jitterOffset)
	}

	return backoff
}

// WithMaxAttempts returns a config with custom max attempts.
func WithMaxAttempts(attempts int) func(*Config) {
	return func(c *Config) {
		c.MaxAttempts = attempts
	}
}

// WithInitialBackoff returns a config with custom initial backoff.
func WithInitialBackoff(backoff time.Duration) func(*Config) {
	return func(c *Config) {
		c.InitialBackoff = backoff
	}
}

// WithMaxBackoff returns a config with custom max backoff.
func WithMaxBackoff(backoff time.Duration) func(*Config) {
	return func(c *Config) {
		c.MaxBackoff = backoff
	}
}

// WithBackoffMultiplier returns a config with custom backoff multiplier.
func WithBackoffMultiplier(multiplier float64) func(*Config) {
	return func(c *Config) {
		c.BackoffMultiplier = multiplier
	}
}

// WithJitter returns a config with jitter enabled/disabled.
func WithJitter(enabled bool) func(*Config) {
	return func(c *Config) {
		c.Jitter = enabled
	}
}

// WithShouldRetry returns a config with custom retry predicate.
func WithShouldRetry(fn func(error) bool) func(*Config) {
	return func(c *Config) {
		c.ShouldRetry = fn
	}
}

// NewConfig creates a Config with custom options.
//
// Example:
//
//	config := retry.NewConfig(
//	    retry.WithMaxAttempts(5),
//	    retry.WithInitialBackoff(50 * time.Millisecond),
//	)
func NewConfig(opts ...func(*Config)) Config {
	cfg := DefaultConfig()
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg
}
