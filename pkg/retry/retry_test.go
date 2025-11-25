package retry

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/plaenen/eventstore/pkg/errorx"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.MaxAttempts != 3 {
		t.Errorf("MaxAttempts = %d, want 3", cfg.MaxAttempts)
	}
	if cfg.InitialBackoff != 100*time.Millisecond {
		t.Errorf("InitialBackoff = %v, want 100ms", cfg.InitialBackoff)
	}
	if cfg.MaxBackoff != 10*time.Second {
		t.Errorf("MaxBackoff = %v, want 10s", cfg.MaxBackoff)
	}
	if cfg.BackoffMultiplier != 2.0 {
		t.Errorf("BackoffMultiplier = %f, want 2.0", cfg.BackoffMultiplier)
	}
	if !cfg.Jitter {
		t.Error("Jitter = false, want true")
	}
	if cfg.ShouldRetry != nil {
		t.Error("ShouldRetry should be nil by default")
	}
}

func TestNewConfig(t *testing.T) {
	cfg := NewConfig(
		WithMaxAttempts(5),
		WithInitialBackoff(50*time.Millisecond),
		WithMaxBackoff(5*time.Second),
		WithBackoffMultiplier(3.0),
		WithJitter(false),
	)

	if cfg.MaxAttempts != 5 {
		t.Errorf("MaxAttempts = %d, want 5", cfg.MaxAttempts)
	}
	if cfg.InitialBackoff != 50*time.Millisecond {
		t.Errorf("InitialBackoff = %v, want 50ms", cfg.InitialBackoff)
	}
	if cfg.MaxBackoff != 5*time.Second {
		t.Errorf("MaxBackoff = %v, want 5s", cfg.MaxBackoff)
	}
	if cfg.BackoffMultiplier != 3.0 {
		t.Errorf("BackoffMultiplier = %f, want 3.0", cfg.BackoffMultiplier)
	}
	if cfg.Jitter {
		t.Error("Jitter = true, want false")
	}
}

func TestDo_Success(t *testing.T) {
	ctx := context.Background()
	attempts := 0

	err := Do(ctx, func(ctx context.Context) error {
		attempts++
		return nil
	})

	if err != nil {
		t.Errorf("Do() error = %v, want nil", err)
	}
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1", attempts)
	}
}

func TestDo_SuccessAfterRetries(t *testing.T) {
	ctx := context.Background()
	attempts := 0

	err := Do(ctx, func(ctx context.Context) error {
		attempts++
		if attempts < 3 {
			return errorx.ErrTimeout // Retryable error
		}
		return nil
	})

	if err != nil {
		t.Errorf("Do() error = %v, want nil", err)
	}
	if attempts != 3 {
		t.Errorf("attempts = %d, want 3", attempts)
	}
}

func TestDo_NonRetryableError(t *testing.T) {
	ctx := context.Background()
	attempts := 0

	err := Do(ctx, func(ctx context.Context) error {
		attempts++
		return errorx.ErrNotFound // Non-retryable error
	})

	if !errors.Is(err, errorx.ErrNotFound) {
		t.Errorf("Do() error = %v, want ErrNotFound", err)
	}
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1 (should not retry)", attempts)
	}
}

func TestDo_AllAttemptsExhausted(t *testing.T) {
	ctx := context.Background()
	attempts := 0

	cfg := NewConfig(WithMaxAttempts(3))
	err := DoWithConfig(ctx, cfg, func(ctx context.Context) error {
		attempts++
		return errorx.ErrTimeout // Always fails with retryable error
	})

	if err == nil {
		t.Error("Do() error = nil, want error")
	}
	if attempts != 3 {
		t.Errorf("attempts = %d, want 3", attempts)
	}
	if !errors.Is(err, errorx.ErrTimeout) {
		t.Errorf("error should wrap ErrTimeout, got %v", err)
	}
}

func TestDo_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := Do(ctx, func(ctx context.Context) error {
		return errorx.ErrTimeout
	})

	if err == nil {
		t.Error("Do() error = nil, want error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error should wrap context.Canceled, got %v", err)
	}
}

func TestDo_ContextTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	cfg := NewConfig(
		WithInitialBackoff(100*time.Millisecond), // Longer than timeout
		WithMaxAttempts(3),
	)

	err := DoWithConfig(ctx, cfg, func(ctx context.Context) error {
		return errorx.ErrTimeout
	})

	if err == nil {
		t.Error("Do() error = nil, want error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error should wrap context.DeadlineExceeded, got %v", err)
	}
}

func TestDoWithResult_Success(t *testing.T) {
	ctx := context.Background()
	attempts := 0

	result, err := DoWithResult(ctx, func(ctx context.Context) (string, error) {
		attempts++
		return "success", nil
	})

	if err != nil {
		t.Errorf("DoWithResult() error = %v, want nil", err)
	}
	if result != "success" {
		t.Errorf("result = %v, want 'success'", result)
	}
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1", attempts)
	}
}

func TestDoWithResult_SuccessAfterRetries(t *testing.T) {
	ctx := context.Background()
	attempts := 0

	result, err := DoWithResult(ctx, func(ctx context.Context) (int, error) {
		attempts++
		if attempts < 3 {
			return 0, errorx.ErrTimeout
		}
		return 42, nil
	})

	if err != nil {
		t.Errorf("DoWithResult() error = %v, want nil", err)
	}
	if result != 42 {
		t.Errorf("result = %d, want 42", result)
	}
	if attempts != 3 {
		t.Errorf("attempts = %d, want 3", attempts)
	}
}

func TestDoWithResult_NonRetryableError(t *testing.T) {
	ctx := context.Background()
	attempts := 0

	result, err := DoWithResult(ctx, func(ctx context.Context) (string, error) {
		attempts++
		return "", errorx.ErrNotFound
	})

	if !errors.Is(err, errorx.ErrNotFound) {
		t.Errorf("DoWithResult() error = %v, want ErrNotFound", err)
	}
	if result != "" {
		t.Errorf("result = %v, want empty string", result)
	}
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1", attempts)
	}
}

func TestDoWithResult_AllAttemptsExhausted(t *testing.T) {
	ctx := context.Background()
	attempts := 0

	cfg := NewConfig(WithMaxAttempts(3))
	result, err := DoWithResultAndConfig(ctx, cfg, func(ctx context.Context) (bool, error) {
		attempts++
		return false, errorx.ErrTimeout
	})

	if err == nil {
		t.Error("DoWithResult() error = nil, want error")
	}
	if result != false {
		t.Errorf("result = %v, want false (zero value)", result)
	}
	if attempts != 3 {
		t.Errorf("attempts = %d, want 3", attempts)
	}
}

func TestCustomShouldRetry(t *testing.T) {
	ctx := context.Background()
	attempts := 0

	// Custom predicate: only retry on specific error
	customErr := errors.New("custom retryable error")
	cfg := NewConfig(
		WithMaxAttempts(3),
		WithShouldRetry(func(err error) bool {
			return errors.Is(err, customErr)
		}),
	)

	err := DoWithConfig(ctx, cfg, func(ctx context.Context) error {
		attempts++
		return customErr
	})

	if err == nil {
		t.Error("Do() error = nil, want error")
	}
	if attempts != 3 {
		t.Errorf("attempts = %d, want 3", attempts)
	}

	// Test non-retryable with custom predicate
	attempts = 0
	err = DoWithConfig(ctx, cfg, func(ctx context.Context) error {
		attempts++
		return errorx.ErrNotFound // Not matched by custom predicate
	})

	if !errors.Is(err, errorx.ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1 (should not retry)", attempts)
	}
}

func TestCalculateBackoff_NoJitter(t *testing.T) {
	cfg := NewConfig(
		WithJitter(false),
		WithMaxBackoff(10*time.Second),
	)

	backoff := calculateBackoff(100*time.Millisecond, cfg.MaxBackoff, cfg.Jitter)
	if backoff != 100*time.Millisecond {
		t.Errorf("backoff = %v, want 100ms", backoff)
	}

	backoff = calculateBackoff(20*time.Second, cfg.MaxBackoff, cfg.Jitter)
	if backoff != 10*time.Second {
		t.Errorf("backoff = %v, want 10s (capped at MaxBackoff)", backoff)
	}
}

func TestCalculateBackoff_WithJitter(t *testing.T) {
	cfg := NewConfig(
		WithJitter(true),
		WithMaxBackoff(10*time.Second),
	)

	// Run multiple times to verify jitter adds randomness
	backoff := 100 * time.Millisecond
	results := make(map[time.Duration]bool)

	for i := 0; i < 20; i++ {
		result := calculateBackoff(backoff, cfg.MaxBackoff, cfg.Jitter)
		results[result] = true

		// Jitter should be within ±25%
		min := time.Duration(float64(backoff) * 0.75)
		max := time.Duration(float64(backoff) * 1.25)

		if result < min || result > max {
			t.Errorf("backoff %v out of range [%v, %v]", result, min, max)
		}
	}

	// Should have some variation due to jitter
	if len(results) < 5 {
		t.Error("jitter not producing enough variation")
	}
}

func TestExponentialBackoff(t *testing.T) {
	ctx := context.Background()
	attempts := 0
	startTime := time.Now()

	cfg := NewConfig(
		WithMaxAttempts(4),
		WithInitialBackoff(10*time.Millisecond),
		WithBackoffMultiplier(2.0),
		WithJitter(false), // Disable jitter for predictable timing
	)

	_ = DoWithConfig(ctx, cfg, func(ctx context.Context) error {
		attempts++
		return errorx.ErrTimeout
	})

	elapsed := time.Since(startTime)

	// Expected delays: 10ms, 20ms, 40ms = 70ms total
	// Allow some tolerance for test execution overhead
	expectedMin := 70 * time.Millisecond
	expectedMax := 200 * time.Millisecond

	if elapsed < expectedMin || elapsed > expectedMax {
		t.Errorf("elapsed time %v out of expected range [%v, %v]", elapsed, expectedMin, expectedMax)
	}

	if attempts != 4 {
		t.Errorf("attempts = %d, want 4", attempts)
	}
}

func TestRetryableErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		shouldRetry bool
	}{
		{"ErrTimeout", errorx.ErrTimeout, true},
		{"ErrUnavailable", errorx.ErrUnavailable, true},
		{"ErrConcurrencyConflict", errorx.ErrConcurrencyConflict, true},
		{"ErrNotFound", errorx.ErrNotFound, false},
		{"ErrAlreadyExists", errorx.ErrAlreadyExists, false},
		{"ErrInvalidArgument", errorx.ErrInvalidArgument, false},
		{"ErrPermissionDenied", errorx.ErrPermissionDenied, false},
		{"custom error", errors.New("custom"), false},
	}

	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attempts := 0
			cfg := NewConfig(WithMaxAttempts(3))

			_ = DoWithConfig(ctx, cfg, func(ctx context.Context) error {
				attempts++
				return tt.err
			})

			expectedAttempts := 1
			if tt.shouldRetry {
				expectedAttempts = 3
			}

			if attempts != expectedAttempts {
				t.Errorf("attempts = %d, want %d", attempts, expectedAttempts)
			}
		})
	}
}

func TestZeroMaxAttempts(t *testing.T) {
	ctx := context.Background()
	attempts := 0

	cfg := Config{
		MaxAttempts: 0,
	}

	err := DoWithConfig(ctx, cfg, func(ctx context.Context) error {
		attempts++
		return errorx.ErrTimeout
	})

	// Should not execute at all
	if err == nil {
		t.Error("expected error, got nil")
	}
	if attempts != 0 {
		t.Errorf("attempts = %d, want 0", attempts)
	}
}

func TestSingleAttempt(t *testing.T) {
	ctx := context.Background()
	attempts := 0

	cfg := NewConfig(WithMaxAttempts(1))

	err := DoWithConfig(ctx, cfg, func(ctx context.Context) error {
		attempts++
		return errorx.ErrTimeout
	})

	if err == nil {
		t.Error("expected error, got nil")
	}
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1", attempts)
	}
}

// Benchmark tests
func BenchmarkDo_Success(b *testing.B) {
	ctx := context.Background()
	for i := 0; i < b.N; i++ {
		_ = Do(ctx, func(ctx context.Context) error {
			return nil
		})
	}
}

func BenchmarkDo_WithRetries(b *testing.B) {
	ctx := context.Background()
	cfg := NewConfig(
		WithMaxAttempts(3),
		WithInitialBackoff(1*time.Millisecond),
	)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		attempts := 0
		_ = DoWithConfig(ctx, cfg, func(ctx context.Context) error {
			attempts++
			if attempts < 3 {
				return errorx.ErrTimeout
			}
			return nil
		})
	}
}

func BenchmarkDoWithResult_Success(b *testing.B) {
	ctx := context.Background()
	for i := 0; i < b.N; i++ {
		_, _ = DoWithResult(ctx, func(ctx context.Context) (int, error) {
			return 42, nil
		})
	}
}

func BenchmarkCalculateBackoff_WithJitter(b *testing.B) {
	for i := 0; i < b.N; i++ {
		calculateBackoff(100*time.Millisecond, 10*time.Second, true)
	}
}

func BenchmarkCalculateBackoff_NoJitter(b *testing.B) {
	for i := 0; i < b.N; i++ {
		calculateBackoff(100*time.Millisecond, 10*time.Second, false)
	}
}
