package config_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/plaenen/eventstore/pkg/config"
)

func TestEnvProvider(t *testing.T) {
	t.Run("Get", func(t *testing.T) {
		key := "TEST_ENV_VAR_GET"
		val := "test_value"
		os.Setenv(key, val)
		defer os.Unsetenv(key)

		provider := config.NewEnvProvider(key, func(s string) (string, error) {
			return s, nil
		}, time.Minute)
		defer provider.Close()

		ctx := context.Background()
		res, err := provider.Get(ctx)
		if err != nil {
			t.Fatalf("Failed to get value: %v", err)
		}
		if res != val {
			t.Errorf("Expected %s, got %s", val, res)
		}
	})

	t.Run("GetJSON", func(t *testing.T) {
		key := "TEST_ENV_VAR_JSON"
		cfg := TestConfig{
			Name:    "env_test",
			Timeout: 45,
			Enabled: true,
		}
		bytes, _ := json.Marshal(cfg)
		os.Setenv(key, string(bytes))
		defer os.Unsetenv(key)

		provider := config.NewEnvJSONProvider[TestConfig](key, time.Minute)
		defer provider.Close()

		ctx := context.Background()
		res, err := provider.Get(ctx)
		if err != nil {
			t.Fatalf("Failed to get value: %v", err)
		}

		if res.Name != cfg.Name {
			t.Errorf("Expected name=%s, got %s", cfg.Name, res.Name)
		}
		if res.Timeout != cfg.Timeout {
			t.Errorf("Expected timeout=%d, got %d", cfg.Timeout, res.Timeout)
		}
	})

	t.Run("MissingVar", func(t *testing.T) {
		key := "TEST_ENV_VAR_MISSING"
		os.Unsetenv(key)

		provider := config.NewEnvJSONProvider[TestConfig](key, time.Minute)
		defer provider.Close()

		_, err := provider.Get(context.Background())
		if err == nil {
			t.Error("Expected error for missing environment variable")
		}
	})

	t.Run("InvalidJSON", func(t *testing.T) {
		key := "TEST_ENV_VAR_INVALID"
		os.Setenv(key, "{invalid_json")
		defer os.Unsetenv(key)

		provider := config.NewEnvJSONProvider[TestConfig](key, time.Minute)
		defer provider.Close()

		_, err := provider.Get(context.Background())
		if err == nil {
			t.Error("Expected error for invalid JSON")
		}
	})

	t.Run("Watch", func(t *testing.T) {
		key := "TEST_ENV_VAR_WATCH"
		val1 := "value1"
		val2 := "value2"

		os.Setenv(key, val1)
		defer os.Unsetenv(key)

		// Short TTL for faster testing
		ttl := 10 * time.Millisecond
		provider := config.NewEnvProvider(key, func(s string) (string, error) {
			return s, nil
		}, ttl)
		defer provider.Close()

		// Initial get to populate cache
		_, err := provider.Get(context.Background())
		if err != nil {
			t.Fatalf("Failed to get initial value: %v", err)
		}

		updates := make(chan string, 1)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		stop, err := provider.Watch(ctx, func(val string) {
			updates <- val
		})
		if err != nil {
			t.Fatalf("Failed to watch: %v", err)
		}
		defer stop()

		// Update env var
		os.Setenv(key, val2)

		// Wait for update
		select {
		case val := <-updates:
			if val != val2 {
				t.Errorf("Expected %s, got %s", val2, val)
			}
		case <-time.After(200 * time.Millisecond):
			t.Error("Timeout waiting for watch update")
		}
	})
}
