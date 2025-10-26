package config_test

import (
	"context"
	"testing"
	"time"

	"github.com/plaenen/eventstore/pkg/config"
)

type TestConfig struct {
	Name    string `json:"name"`
	Timeout int    `json:"timeout"`
	Enabled bool   `json:"enabled"`
}

func TestStaticProvider(t *testing.T) {
	cfg := TestConfig{
		Name:    "test",
		Timeout: 30,
		Enabled: true,
	}

	provider := config.NewStaticProvider(cfg)
	defer provider.Close()

	t.Run("Get", func(t *testing.T) {
		ctx := context.Background()
		result, err := provider.Get(ctx)
		if err != nil {
			t.Fatalf("Failed to get config: %v", err)
		}

		if result.Name != "test" {
			t.Errorf("Expected name=test, got %s", result.Name)
		}
		if result.Timeout != 30 {
			t.Errorf("Expected timeout=30, got %d", result.Timeout)
		}
		if !result.Enabled {
			t.Error("Expected enabled=true")
		}
	})

	t.Run("Latest", func(t *testing.T) {
		result, err := provider.Latest()
		if err != nil {
			t.Fatalf("Failed to get latest: %v", err)
		}

		if result.Name != "test" {
			t.Errorf("Expected name=test, got %s", result.Name)
		}
	})

	t.Run("Watch", func(t *testing.T) {
		ctx := context.Background()
		called := make(chan TestConfig, 1)

		stop, err := provider.Watch(ctx, func(cfg TestConfig) {
			called <- cfg
		})
		if err != nil {
			t.Fatalf("Failed to watch: %v", err)
		}
		defer stop()

		// Should be called once with initial value
		select {
		case cfg := <-called:
			if cfg.Name != "test" {
				t.Errorf("Expected name=test, got %s", cfg.Name)
			}
		case <-time.After(1 * time.Second):
			t.Error("Watch callback not called")
		}
	})

	t.Run("Update", func(t *testing.T) {
		newCfg := TestConfig{
			Name:    "updated",
			Timeout: 60,
			Enabled: false,
		}

		provider.Update(newCfg)

		result, err := provider.Get(context.Background())
		if err != nil {
			t.Fatalf("Failed to get updated config: %v", err)
		}

		if result.Name != "updated" {
			t.Errorf("Expected name=updated, got %s", result.Name)
		}
		if result.Timeout != 60 {
			t.Errorf("Expected timeout=60, got %d", result.Timeout)
		}
		if result.Enabled {
			t.Error("Expected enabled=false")
		}
	})
}

func TestChainProvider(t *testing.T) {
	cfg1 := TestConfig{Name: "provider1"}
	cfg2 := TestConfig{Name: "provider2"}

	provider1 := config.NewStaticProvider(cfg1)
	provider2 := config.NewStaticProvider(cfg2)
	defer provider1.Close()
	defer provider2.Close()

	chain := config.NewChainProvider(provider1, provider2)
	defer chain.Close()

	t.Run("UsesFirstProvider", func(t *testing.T) {
		ctx := context.Background()
		result, err := chain.Get(ctx)
		if err != nil {
			t.Fatalf("Failed to get config: %v", err)
		}

		if result.Name != "provider1" {
			t.Errorf("Expected provider1, got %s", result.Name)
		}
	})

	t.Run("FallsBackToSecond", func(t *testing.T) {
		// Close first provider to force fallback
		provider1.Close()

		ctx := context.Background()
		result, err := chain.Get(ctx)
		if err != nil {
			t.Fatalf("Failed to get config: %v", err)
		}

		if result.Name != "provider2" {
			t.Errorf("Expected provider2, got %s", result.Name)
		}
	})
}

func TestConfigWithValidator(t *testing.T) {
	type ValidatedConfig struct {
		MaxConnections int `json:"max_connections"`
	}

	// Make it implement Validator
	type ConfigWithValidation struct {
		ValidatedConfig
	}

	testValidator := func(cfg ConfigWithValidation) error {
		if cfg.MaxConnections < 1 {
			return config.ErrInvalidConfig
		}
		return nil
	}

	t.Run("ValidConfig", func(t *testing.T) {
		cfg := ConfigWithValidation{
			ValidatedConfig: ValidatedConfig{MaxConnections: 10},
		}

		if err := testValidator(cfg); err != nil {
			t.Errorf("Valid config failed validation: %v", err)
		}
	})

	t.Run("InvalidConfig", func(t *testing.T) {
		cfg := ConfigWithValidation{
			ValidatedConfig: ValidatedConfig{MaxConnections: 0},
		}

		if err := testValidator(cfg); err == nil {
			t.Error("Expected validation error for invalid config")
		}
	})
}

func TestFeatureFlags(t *testing.T) {
	flags := config.FeatureFlags{
		EnableNewUI:    true,
		EnableDebugMode: false,
		Features: map[string]bool{
			"experimental_api": true,
			"beta_feature":     false,
		},
	}

	t.Run("CheckStandardFlags", func(t *testing.T) {
		if !flags.EnableNewUI {
			t.Error("Expected EnableNewUI to be true")
		}
		if flags.EnableDebugMode {
			t.Error("Expected EnableDebugMode to be false")
		}
	})

	t.Run("IsEnabled", func(t *testing.T) {
		if !flags.IsEnabled("experimental_api") {
			t.Error("Expected experimental_api to be enabled")
		}
		if flags.IsEnabled("beta_feature") {
			t.Error("Expected beta_feature to be disabled")
		}
		if flags.IsEnabled("nonexistent") {
			t.Error("Expected nonexistent feature to be disabled")
		}
	})
}

func TestServiceEndpoints(t *testing.T) {
	t.Run("ValidEndpoints", func(t *testing.T) {
		endpoints := config.ServiceEndpoints{
			NATSURL:     "nats://localhost:4222",
			DatabaseURL: "postgres://localhost:5432/db",
			Endpoints: map[string]string{
				"custom": "http://custom:8080",
			},
		}

		if err := endpoints.Validate(); err != nil {
			t.Errorf("Valid endpoints failed validation: %v", err)
		}

		url, ok := endpoints.GetEndpoint("custom")
		if !ok {
			t.Error("Expected custom endpoint to exist")
		}
		if url != "http://custom:8080" {
			t.Errorf("Expected custom endpoint URL, got %s", url)
		}
	})

	t.Run("MissingRequired", func(t *testing.T) {
		endpoints := config.ServiceEndpoints{
			NATSURL: "nats://localhost:4222",
			// Missing DatabaseURL
		}

		if err := endpoints.Validate(); err == nil {
			t.Error("Expected validation error for missing database URL")
		}
	})
}

func TestRuntimeTuning(t *testing.T) {
	t.Run("ValidTuning", func(t *testing.T) {
		tuning := config.RuntimeTuning{
			MaxConcurrency:    10,
			EventBatchSize:    100,
			ProjectionWorkers: 5,
			CacheTTL:          5 * time.Minute,
			Parameters: map[string]interface{}{
				"custom_param": 42,
			},
		}

		if err := tuning.Validate(); err != nil {
			t.Errorf("Valid tuning failed validation: %v", err)
		}

		val, ok := tuning.GetParameter("custom_param")
		if !ok {
			t.Error("Expected custom parameter to exist")
		}
		if val != 42 {
			t.Errorf("Expected custom_param=42, got %v", val)
		}
	})

	t.Run("InvalidValues", func(t *testing.T) {
		tuning := config.RuntimeTuning{
			MaxConcurrency:    0, // Invalid
			EventBatchSize:    100,
			ProjectionWorkers: 5,
		}

		if err := tuning.Validate(); err == nil {
			t.Error("Expected validation error for invalid max_concurrency")
		}
	})
}

func TestRateLimits(t *testing.T) {
	t.Run("ValidLimits", func(t *testing.T) {
		limits := config.RateLimits{
			RequestsPerSecond: 100,
			BurstSize:         10,
			Enabled:           true,
		}

		if err := limits.Validate(); err != nil {
			t.Errorf("Valid rate limits failed validation: %v", err)
		}
	})

	t.Run("DisabledLimits", func(t *testing.T) {
		limits := config.RateLimits{
			Enabled: false,
			// Invalid values but doesn't matter when disabled
			RequestsPerSecond: 0,
			BurstSize:         0,
		}

		if err := limits.Validate(); err != nil {
			t.Errorf("Disabled rate limits should not fail validation: %v", err)
		}
	})

	t.Run("InvalidWhenEnabled", func(t *testing.T) {
		limits := config.RateLimits{
			RequestsPerSecond: 0,
			BurstSize:         10,
			Enabled:           true,
		}

		if err := limits.Validate(); err == nil {
			t.Error("Expected validation error for invalid rate limits")
		}
	})
}

func TestLoggingConfig(t *testing.T) {
	t.Run("ValidConfig", func(t *testing.T) {
		logging := config.LoggingConfig{
			Level:            "info",
			Format:           "json",
			Output:           "stdout",
			EnableStackTrace: true,
			SamplingRate:     0.5,
		}

		if err := logging.Validate(); err != nil {
			t.Errorf("Valid logging config failed validation: %v", err)
		}
	})

	t.Run("InvalidLevel", func(t *testing.T) {
		logging := config.LoggingConfig{
			Level:  "invalid",
			Format: "json",
			Output: "stdout",
		}

		if err := logging.Validate(); err == nil {
			t.Error("Expected validation error for invalid log level")
		}
	})

	t.Run("InvalidSamplingRate", func(t *testing.T) {
		logging := config.LoggingConfig{
			Level:        "info",
			SamplingRate: 1.5, // > 1.0
		}

		if err := logging.Validate(); err == nil {
			t.Error("Expected validation error for invalid sampling rate")
		}
	})
}

func TestDatabaseConfig(t *testing.T) {
	t.Run("ValidConfig", func(t *testing.T) {
		db := config.DatabaseConfig{
			URLRef:             "secret://db-url",
			MaxConnections:     10,
			MaxIdleConnections: 5,
			ConnectionTimeout:  10 * time.Second,
			QueryTimeout:       5 * time.Second,
			EnableSSL:          true,
		}

		if err := db.Validate(); err != nil {
			t.Errorf("Valid database config failed validation: %v", err)
		}
	})

	t.Run("InvalidMaxConnections", func(t *testing.T) {
		db := config.DatabaseConfig{
			MaxConnections:     0,
			MaxIdleConnections: 5,
		}

		if err := db.Validate(); err == nil {
			t.Error("Expected validation error for invalid max_connections")
		}
	})

	t.Run("IdleExceedsMax", func(t *testing.T) {
		db := config.DatabaseConfig{
			MaxConnections:     5,
			MaxIdleConnections: 10, // > MaxConnections
		}

		if err := db.Validate(); err == nil {
			t.Error("Expected validation error when idle > max")
		}
	})
}

func TestAppConfig(t *testing.T) {
	t.Run("ValidConfig", func(t *testing.T) {
		appCfg := config.AppConfig{
			Environment: "production",
			ServiceName: "my-service",
			Version:     "1.0.0",
			Endpoints: config.ServiceEndpoints{
				NATSURL:     "nats://localhost:4222",
				DatabaseURL: "postgres://localhost:5432/db",
			},
			Tuning: config.RuntimeTuning{
				MaxConcurrency:    10,
				EventBatchSize:    100,
				ProjectionWorkers: 5,
				CacheTTL:          5 * time.Minute,
			},
			RateLimits: config.RateLimits{
				RequestsPerSecond: 100,
				BurstSize:         10,
				Enabled:           false,
			},
			Logging: config.LoggingConfig{
				Level:  "info",
				Format: "json",
				Output: "stdout",
			},
			Database: config.DatabaseConfig{
				MaxConnections:     10,
				MaxIdleConnections: 5,
			},
		}

		if err := appCfg.Validate(); err != nil {
			t.Errorf("Valid app config failed validation: %v", err)
		}
	})

	t.Run("MissingRequired", func(t *testing.T) {
		appCfg := config.AppConfig{
			// Missing Environment and ServiceName
		}

		if err := appCfg.Validate(); err == nil {
			t.Error("Expected validation error for missing required fields")
		}
	})
}
