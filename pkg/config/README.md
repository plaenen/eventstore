# Configuration Package

**Dynamic configuration management for the Event Sourcing Framework**

[![Go Reference](https://pkg.go.dev/badge/github.com/plaenen/eventstore/pkg/config.svg)](https://pkg.go.dev/github.com/plaenen/eventstore/pkg/config)

This package provides enterprise-grade dynamic configuration management using [Go Cloud Development Kit's runtimevar](https://gocloud.dev/howto/runtimevar/), enabling real-time configuration updates across multiple cloud providers and local environments without application restarts.

## Table of Contents

- [Quick Start](#quick-start)
- [Provider Types](#provider-types)
- [Common Configuration Types](#common-configuration-types)
- [Usage Examples](#usage-examples)
- [Production Deployment](#production-deployment)
- [Integration with Runner](#integration-with-runner)
- [Best Practices](#best-practices)
- [Migration Guide](#migration-guide)
- [API Reference](#api-reference)
- [Troubleshooting](#troubleshooting)

## Quick Start

### Installation

The config package is part of the eventsourcing framework. Import the providers you need:

```go
import (
    "github.com/plaenen/eventstore/pkg/config"

    // Cloud provider imports (choose what you need):
    _ "gocloud.dev/runtimevar/awsparamstore"    // AWS Parameter Store
    _ "gocloud.dev/runtimevar/gcpruntimeconfig" // GCP Runtime Configurator
    _ "gocloud.dev/runtimevar/azureappconfig"   // Azure App Configuration
    _ "gocloud.dev/runtimevar/etcdvar"          // etcd
    _ "gocloud.dev/runtimevar/filevar"          // Local file
)
```

### Basic Usage

```go
ctx := context.Background()

// Define your configuration type
type AppConfig struct {
    MaxConnections int           `json:"max_connections"`
    Timeout        time.Duration `json:"timeout"`
    EnableFeatureX bool          `json:"enable_feature_x"`
}

// Development: Static configuration
provider := config.NewStaticProvider(AppConfig{
    MaxConnections: 10,
    Timeout:        5 * time.Second,
    EnableFeatureX: true,
})
defer provider.Close()

// Get configuration
cfg, err := provider.Get(ctx)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Max connections: %d\n", cfg.MaxConnections)
```

### Watch for Changes

```go
// Watch for configuration updates (hot-reload)
stop, err := provider.Watch(ctx, func(cfg AppConfig) {
    log.Printf("Configuration updated: %+v", cfg)
    // Update application settings here
})
defer stop()
```

## Provider Types

### Overview

| Provider | Use Case | Update Mechanism | Watch Support |
|----------|----------|------------------|---------------|
| `RuntimeVarProvider` | Production (Cloud) | ⭐⭐⭐⭐⭐ Real-time | ✅ Native |
| `EnvProvider` | CI/CD, Containers | ⭐⭐⭐ Polling | ✅ Polling |
| `ChainProvider` | Fallback scenarios | ⭐⭐⭐⭐ Depends | ✅ First provider |
| `StaticProvider` | Development/Testing | ⭐ Manual only | ✅ Initial only |

### 1. RuntimeVarProvider (Recommended for Production)

Uses Go Cloud Development Kit for vendor-agnostic configuration management.

**Supported Backends:**
- AWS Parameter Store
- GCP Runtime Configurator
- Azure App Configuration
- etcd
- Local file (development)

**Features:**
- Real-time updates
- Automatic decoding (JSON, YAML)
- Built-in caching
- Thread-safe
- Health checks

**Example:**

```go
// AWS Parameter Store
provider, err := config.NewProvider[AppConfig](ctx,
    "awsparamstore:///prod/myapp/config?decoder=json")

// GCP Runtime Configurator
provider, err := config.NewProvider[AppConfig](ctx,
    "gcpruntimeconfig://projects/my-project/configs/app/variables/config?decoder=json")

// Azure App Configuration
provider, err := config.NewProvider[AppConfig](ctx,
    "azureappconfig://connection-string?key=app-config&decoder=json")

// etcd
provider, err := config.NewProvider[AppConfig](ctx,
    "etcd://localhost:2379/app-config?decoder=json")

// Local file (development)
provider, err := config.NewProvider[AppConfig](ctx,
    "file:///etc/myapp/config.json?decoder=json")
```

### 2. EnvProvider (Recommended for CI/CD)

Reads configuration from environment variables.

**Best for:**
- Kubernetes ConfigMaps
- Docker containers
- CI/CD pipelines
- 12-factor apps

**Example:**

```go
// JSON from environment variable
provider := config.NewEnvJSONProvider[AppConfig]("APP_CONFIG", 5*time.Minute)

// Custom decoder
decoder := func(s string) (AppConfig, error) {
    // Parse environment variable value
    return parseConfig(s)
}
provider := config.NewEnvProvider("APP_CONFIG", decoder, 5*time.Minute)
```

### 3. ChainProvider (Recommended for Flexibility)

Tries multiple providers in order, falling back on failure.

**Example:**

```go
provider := config.NewChainProvider(
    // Try cloud config first
    config.NewProvider[AppConfig](ctx, "awsparamstore://..."),
    // Fall back to environment variables
    config.NewEnvJSONProvider[AppConfig]("APP_CONFIG", 5*time.Minute),
    // Last resort: static defaults
    config.NewStaticProvider(defaultConfig),
)
defer provider.Close()
```

### 4. StaticProvider (Development Only)

⚠️ **WARNING: Use only for development and testing!**

Provides configuration from static values.

**Example:**

```go
cfg := AppConfig{
    MaxConnections: 10,
    Timeout:        5 * time.Second,
}

provider := config.NewStaticProvider(cfg)
defer provider.Close()

// For testing: manually update configuration
provider.Update(newConfig)
```

## Common Configuration Types

The package provides pre-built configuration types for common use cases:

### Feature Flags

```go
flags := config.FeatureFlags{
    EnableNewUI:                true,
    EnableExperimentalFeatures: false,
    EnableDebugMode:            true,
    EnableMetrics:              true,
    Features: map[string]bool{
        "dark_mode":     true,
        "beta_feature":  false,
    },
}

provider := config.NewStaticProvider(flags)
defer provider.Close()

// Check feature
if flags.IsEnabled("dark_mode") {
    // Enable dark mode
}
```

### Service Endpoints

```go
endpoints := config.ServiceEndpoints{
    NATSURL:     "nats://localhost:4222",
    DatabaseURL: "postgres://localhost:5432/db",
    CacheURL:    "redis://localhost:6379",
    Endpoints: map[string]string{
        "auth_service": "http://auth:8080",
    },
}

// Validate
if err := endpoints.Validate(); err != nil {
    log.Fatal(err)
}

provider := config.NewStaticProvider(endpoints)
```

### Runtime Tuning

```go
tuning := config.RuntimeTuning{
    MaxConcurrency:    10,
    EventBatchSize:    100,
    ProjectionWorkers: 5,
    CacheTTL:          5 * time.Minute,
    RequestTimeout:    10 * time.Second,
    Parameters: map[string]interface{}{
        "custom_param": 42,
    },
}

// Validate
if err := tuning.Validate(); err != nil {
    log.Fatal(err)
}

provider := config.NewStaticProvider(tuning)
```

### Comprehensive App Configuration

```go
appConfig := config.AppConfig{
    Environment: "production",
    ServiceName: "my-service",
    Version:     "1.0.0",

    FeatureFlags: config.FeatureFlags{
        EnableMetrics: true,
    },

    Endpoints: config.ServiceEndpoints{
        NATSURL:     "nats://nats:4222",
        DatabaseURL: "postgres://db:5432/prod",
    },

    Tuning: config.RuntimeTuning{
        MaxConcurrency:    20,
        EventBatchSize:    200,
        ProjectionWorkers: 10,
    },

    Logging: config.LoggingConfig{
        Level:  "info",
        Format: "json",
        Output: "stdout",
    },

    Database: config.DatabaseConfig{
        MaxConnections:     25,
        MaxIdleConnections: 10,
    },
}

// Validate entire config
if err := appConfig.Validate(); err != nil {
    log.Fatal(err)
}
```

## Usage Examples

### Example 1: Feature Flags with Hot-Reload

```go
package main

import (
    "context"
    "log"

    "github.com/plaenen/eventstore/pkg/config"
)

type FeatureManager struct {
    provider config.Provider[config.FeatureFlags]
}

func NewFeatureManager(provider config.Provider[config.FeatureFlags]) *FeatureManager {
    return &FeatureManager{provider: provider}
}

func (fm *FeatureManager) IsEnabled(feature string) bool {
    flags, err := fm.provider.Latest()
    if err != nil {
        log.Printf("Failed to get flags: %v", err)
        return false
    }
    return flags.IsEnabled(feature)
}

func main() {
    ctx := context.Background()

    // Production: AWS Parameter Store
    provider, err := config.NewProvider[config.FeatureFlags](ctx,
        "awsparamstore:///prod/feature-flags?decoder=json")
    if err != nil {
        log.Fatal(err)
    }
    defer provider.Close()

    fm := NewFeatureManager(provider)

    // Watch for changes
    stop, _ := provider.Watch(ctx, func(flags config.FeatureFlags) {
        log.Printf("Feature flags updated: %+v", flags)
    })
    defer stop()

    // Use feature flags
    if fm.IsEnabled("new_ui") {
        // Serve new UI
    }
}
```

### Example 2: Service Discovery with Failover

```go
type ServiceClient struct {
    endpointProvider config.Provider[config.ServiceEndpoints]
}

func (sc *ServiceClient) Connect() error {
    endpoints, err := sc.endpointProvider.Get(context.Background())
    if err != nil {
        return err
    }

    // Connect to services
    log.Printf("Connecting to NATS: %s", endpoints.NATSURL)
    log.Printf("Connecting to Database: %s", endpoints.DatabaseURL)

    return nil
}

func main() {
    ctx := context.Background()

    provider, _ := config.NewProvider[config.ServiceEndpoints](ctx,
        "awsparamstore:///prod/endpoints?decoder=json")
    defer provider.Close()

    client := &ServiceClient{endpointProvider: provider}

    // Watch for endpoint changes (e.g., failover)
    provider.Watch(ctx, func(endpoints config.ServiceEndpoints) {
        log.Println("Endpoints updated - reconnecting...")
        client.Connect()
    })

    client.Connect()
}
```

### Example 3: Dynamic Performance Tuning

```go
type EventProcessor struct {
    tuningProvider config.Provider[config.RuntimeTuning]
    batchSize      int
    workers        int
}

func (ep *EventProcessor) UpdateConfig(tuning config.RuntimeTuning) {
    ep.batchSize = tuning.EventBatchSize
    ep.workers = tuning.ProjectionWorkers

    log.Printf("Updated: batch=%d, workers=%d",
        ep.batchSize, ep.workers)
}

func main() {
    ctx := context.Background()

    provider, _ := config.NewProvider[config.RuntimeTuning](ctx,
        "awsparamstore:///prod/tuning?decoder=json")
    defer provider.Close()

    processor := &EventProcessor{tuningProvider: provider}

    // Load initial config
    tuning, _ := provider.Get(ctx)
    processor.UpdateConfig(tuning)

    // Watch for tuning updates
    provider.Watch(ctx, func(tuning config.RuntimeTuning) {
        processor.UpdateConfig(tuning)
    })

    // Process events with current tuning
    processor.ProcessEvents()
}
```

### Example 4: Custom Configuration Type

```go
type MyAppConfig struct {
    AppName     string        `json:"app_name"`
    Port        int           `json:"port"`
    Timeout     time.Duration `json:"timeout"`
    EnableSSL   bool          `json:"enable_ssl"`
    AllowedIPs  []string      `json:"allowed_ips"`
}

// Implement Validator interface
func (c *MyAppConfig) Validate() error {
    if c.Port < 1 || c.Port > 65535 {
        return fmt.Errorf("invalid port: %d", c.Port)
    }
    if c.Timeout < time.Second {
        return fmt.Errorf("timeout too short: %s", c.Timeout)
    }
    return nil
}

func main() {
    ctx := context.Background()

    // Use your custom config type
    provider, err := config.NewProvider[MyAppConfig](ctx,
        "file:///etc/myapp/config.json?decoder=json")
    if err != nil {
        log.Fatal(err)
    }
    defer provider.Close()

    cfg, _ := provider.Get(ctx)
    // Validation happens automatically

    log.Printf("Starting %s on port %d", cfg.AppName, cfg.Port)
}
```

## Production Deployment

### AWS Parameter Store

```go
package main

import (
    "context"
    "log"

    "github.com/plaenen/eventstore/pkg/config"
    _ "gocloud.dev/runtimevar/awsparamstore"
)

func main() {
    ctx := context.Background()

    // Parameter name: /prod/myapp/config
    provider, err := config.NewProvider[config.AppConfig](ctx,
        "awsparamstore:///prod/myapp/config?decoder=json")
    if err != nil {
        log.Fatal(err)
    }
    defer provider.Close()

    cfg, _ := provider.Get(ctx)
    log.Printf("Loaded config: %+v", cfg)
}
```

**Setup:**

1. **Create parameter in AWS:**
```bash
aws ssm put-parameter \
    --name /prod/myapp/config \
    --type String \
    --value file://config.json \
    --description "Production app configuration"
```

2. **Grant IAM permissions:**
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "ssm:GetParameter",
        "ssm:GetParameters"
      ],
      "Resource": "arn:aws:ssm:*:*:parameter/prod/myapp/*"
    }
  ]
}
```

3. **Configuration format (config.json):**
```json
{
  "environment": "production",
  "service_name": "my-service",
  "version": "1.0.0",
  "endpoints": {
    "nats_url": "nats://nats-prod:4222",
    "database_url": "postgres://db-prod:5432/app"
  },
  "tuning": {
    "max_concurrency": 20,
    "event_batch_size": 200,
    "projection_workers": 10,
    "cache_ttl": "5m"
  }
}
```

### GCP Runtime Configurator

```go
provider, err := config.NewProvider[config.AppConfig](ctx,
    "gcpruntimeconfig://projects/my-project/configs/app-config/variables/production?decoder=json")
```

**Setup:**

```bash
# Create config
gcloud beta runtime-config configs create app-config

# Set variable
gcloud beta runtime-config configs variables set \
    production \
    "$(cat config.json)" \
    --config-name app-config
```

### Azure App Configuration

```go
provider, err := config.NewProvider[config.AppConfig](ctx,
    "azureappconfig://Endpoint=https://myapp.azconfig.io;Key=...?key=app-config&decoder=json")
```

**Setup:**

```bash
# Create configuration
az appconfig kv set \
    --name myapp \
    --key app-config \
    --value @config.json
```

### etcd

```go
provider, err := config.NewProvider[config.AppConfig](ctx,
    "etcd://etcd-cluster:2379/app-config?decoder=json")
```

**Setup:**

```bash
# Store configuration
etcdctl put /app-config "$(cat config.json)"
```

### Kubernetes ConfigMap

Use environment variables with Kubernetes:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: app-config
data:
  APP_CONFIG: |
    {
      "environment": "production",
      "service_name": "my-service",
      "endpoints": {
        "nats_url": "nats://nats:4222",
        "database_url": "postgres://postgres:5432/app"
      }
    }
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-service
spec:
  template:
    spec:
      containers:
      - name: app
        env:
        - name: APP_CONFIG
          valueFrom:
            configMapKeyRef:
              name: app-config
              key: APP_CONFIG
```

**Application code:**

```go
provider := config.NewEnvJSONProvider[config.AppConfig]("APP_CONFIG", 1*time.Minute)
defer provider.Close()

// Watch for ConfigMap updates
provider.Watch(ctx, func(cfg config.AppConfig) {
    log.Printf("Config updated: %+v", cfg)
})
```

## Integration with Runner

The runner package supports automatic configuration updates for services:

```go
package main

import (
    "context"
    "log"
    "time"

    "github.com/plaenen/eventstore/pkg/config"
    "github.com/plaenen/eventstore/pkg/runner"
)

// MyService implements runner.ConfigurableService
type MyService struct {
    name   string
    config config.RuntimeTuning
}

func (s *MyService) Name() string {
    return s.name
}

func (s *MyService) Start(ctx context.Context) error {
    log.Printf("Starting %s with config: %+v", s.name, s.config)
    return nil
}

func (s *MyService) Stop(ctx context.Context) error {
    log.Printf("Stopping %s", s.name)
    return nil
}

// UpdateConfig is called when configuration changes
func (s *MyService) UpdateConfig(ctx context.Context, cfg interface{}) error {
    tuning := cfg.(config.RuntimeTuning)
    log.Printf("Updating %s configuration: %+v", s.name, tuning)
    s.config = tuning
    return nil
}

// ValidateConfig validates configuration before applying
func (s *MyService) ValidateConfig(cfg interface{}) error {
    tuning := cfg.(config.RuntimeTuning)
    return tuning.Validate()
}

func main() {
    ctx := context.Background()

    // Create config provider
    configProvider, err := config.NewProvider[config.RuntimeTuning](ctx,
        "awsparamstore:///prod/tuning?decoder=json")
    if err != nil {
        log.Fatal(err)
    }
    defer configProvider.Close()

    // Load initial config
    initialConfig, _ := configProvider.Get(ctx)

    // Create services
    services := []runner.Service{
        &MyService{
            name:   "service-1",
            config: initialConfig,
        },
        &MyService{
            name:   "service-2",
            config: initialConfig,
        },
    }

    // Create runner with config provider
    r := runner.New(services,
        runner.WithLogger(runner.NewStdLogger()),
        runner.WithConfigProvider(configProvider), // ✅ Auto-update on config changes
    )

    // Run services
    // When configuration changes, all ConfigurableService instances
    // will automatically receive UpdateConfig callbacks
    if err := r.Run(ctx); err != nil {
        log.Fatal(err)
    }
}
```

**Benefits:**
- ✅ Zero-downtime configuration updates
- ✅ Automatic validation before applying
- ✅ Consistent configuration across all services
- ✅ Centralized configuration management

## Best Practices

### ✅ DO

1. **Use cloud providers in production**
   ```go
   provider, _ := config.NewProvider[T](ctx, "awsparamstore://...")
   ```

2. **Implement the Validator interface**
   ```go
   func (c *MyConfig) Validate() error {
       if c.Port < 1 || c.Port > 65535 {
           return fmt.Errorf("invalid port")
       }
       return nil
   }
   ```

3. **Watch for configuration changes**
   ```go
   stop, _ := provider.Watch(ctx, func(cfg MyConfig) {
       log.Printf("Config updated: %+v", cfg)
       applyNewConfig(cfg)
   })
   defer stop()
   ```

4. **Use Latest() for frequently accessed config**
   ```go
   // Cached, no network call
   cfg, _ := provider.Latest()
   ```

5. **Set appropriate poll intervals**
   ```go
   config := config.ProviderConfig{
       URL:           "awsparamstore://...",
       WatchInterval: 30 * time.Second, // Balance freshness vs load
   }
   ```

6. **Handle configuration errors gracefully**
   ```go
   cfg, err := provider.Get(ctx)
   if err != nil {
       // Use cached/default config
       cfg = fallbackConfig
   }
   ```

### ❌ DON'T

1. **Never store secrets in configuration**
   ```go
   // ❌ BAD
   type Config struct {
       DatabasePassword string // NO!
   }

   // ✅ GOOD
   type Config struct {
       DatabaseURLRef string // Reference to secret
   }
   ```

2. **Never ignore validation errors**
   ```go
   // ❌ BAD
   cfg.Validate() // Ignored!

   // ✅ GOOD
   if err := cfg.Validate(); err != nil {
       log.Fatal(err)
   }
   ```

3. **Never use Get() in hot paths**
   ```go
   // ❌ BAD - calls backend every time
   for _, item := range items {
       cfg, _ := provider.Get(ctx)
       process(item, cfg)
   }

   // ✅ GOOD - use cached Latest()
   cfg, _ := provider.Latest()
   for _, item := range items {
       process(item, cfg)
   }
   ```

4. **Never create providers in loops**
   ```go
   // ❌ BAD
   for i := 0; i < 100; i++ {
       p, _ := config.NewProvider[T](ctx, url)
       defer p.Close()
   }

   // ✅ GOOD
   provider, _ := config.NewProvider[T](ctx, url)
   defer provider.Close()
   for i := 0; i < 100; i++ {
       cfg, _ := provider.Latest()
   }
   ```

5. **Never block on Watch callbacks**
   ```go
   // ❌ BAD
   provider.Watch(ctx, func(cfg Config) {
       time.Sleep(10 * time.Second) // Blocks watcher!
   })

   // ✅ GOOD
   provider.Watch(ctx, func(cfg Config) {
       go applyConfigAsync(cfg) // Non-blocking
   })
   ```

## Migration Guide

### From Hardcoded Config to Dynamic Config

#### Before (Static)

```go
const (
    MaxConnections = 10
    BatchSize      = 100
    CacheTTL       = 5 * time.Minute
)

func main() {
    processor := NewProcessor(MaxConnections, BatchSize)
    cache := NewCache(CacheTTL)
    // ...
}
```

#### After (Dynamic)

```go
type AppConfig struct {
    MaxConnections int           `json:"max_connections"`
    BatchSize      int           `json:"batch_size"`
    CacheTTL       time.Duration `json:"cache_ttl"`
}

func main() {
    ctx := context.Background()

    provider, _ := config.NewProvider[AppConfig](ctx,
        "awsparamstore:///prod/config?decoder=json")
    defer provider.Close()

    cfg, _ := provider.Get(ctx)

    processor := NewProcessor(cfg.MaxConnections, cfg.BatchSize)
    cache := NewCache(cfg.CacheTTL)

    // Watch for updates
    provider.Watch(ctx, func(cfg AppConfig) {
        processor.UpdateConfig(cfg.MaxConnections, cfg.BatchSize)
        cache.UpdateTTL(cfg.CacheTTL)
    })
}
```

### From Environment Variables to Config Provider

#### Before

```go
maxConn, _ := strconv.Atoi(os.Getenv("MAX_CONNECTIONS"))
batchSize, _ := strconv.Atoi(os.Getenv("BATCH_SIZE"))
```

#### After

```go
provider := config.NewEnvJSONProvider[AppConfig]("APP_CONFIG", 1*time.Minute)
cfg, _ := provider.Get(ctx)
```

## API Reference

### Provider Interface

```go
type Provider[T any] interface {
    Get(ctx context.Context) (T, error)
    Watch(ctx context.Context, handler func(T)) (stop func(), err error)
    Latest() (T, error)
    Close() error
}
```

### Configuration Types

- `FeatureFlags` - Feature toggle management
- `ServiceEndpoints` - Service discovery
- `RuntimeTuning` - Performance tuning
- `RateLimits` - Rate limiting configuration
- `SecurityConfig` - Security settings
- `LoggingConfig` - Logging configuration
- `DatabaseConfig` - Database settings
- `AppConfig` - Comprehensive application config

### Validator Interface

```go
type Validator interface {
    Validate() error
}
```

Implement this interface on your configuration types for automatic validation.

## Troubleshooting

### Common Issues

#### 1. "Provider is closed"

**Problem:** Attempting to use a provider after calling `Close()`.

**Solution:**
```go
defer provider.Close() // Ensure Close() is only called once

cfg, err := provider.Get(ctx)
if errors.Is(err, config.ErrProviderClosed) {
    // Recreate provider
}
```

#### 2. "Failed to decode configuration"

**Problem:** Invalid JSON or wrong decoder.

**Solution:**
```go
// Ensure URL includes correct decoder
"awsparamstore:///config?decoder=json"  // For JSON
"awsparamstore:///config?decoder=string" // For plain text
```

#### 3. "Validation failed"

**Problem:** Configuration doesn't meet validation requirements.

**Solution:**
```go
func (c *MyConfig) Validate() error {
    if c.Port < 1 {
        return fmt.Errorf("invalid port: %d", c.Port)
    }
    return nil
}
```

#### 4. Watch not triggering

**Problem:** Configuration changes but Watch callback not called.

**Solution:**
```go
// Check watch interval
config := config.ProviderConfig{
    WatchInterval: 10 * time.Second, // Adjust polling frequency
}

// Ensure stop function is not called prematurely
stop, _ := provider.Watch(ctx, handler)
defer stop() // Not stop() immediately!
```

### Debug Logging

```go
provider.Watch(ctx, func(cfg MyConfig) {
    log.Printf("Configuration updated: %+v", cfg)
    log.Printf("Applying changes...")
})
```

### Testing

#### Unit Tests

```go
func TestMyService(t *testing.T) {
    cfg := MyConfig{
        MaxConnections: 10,
    }

    provider := config.NewStaticProvider(cfg)
    defer provider.Close()

    service := NewService(provider)
    // Test service...
}
```

#### Integration Tests

```go
func TestIntegration(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test")
    }

    ctx := context.Background()
    provider, err := config.NewProvider[MyConfig](ctx,
        os.Getenv("TEST_CONFIG_URL"))
    if err != nil {
        t.Fatal(err)
    }
    defer provider.Close()

    // Run integration test...
}
```

## Additional Resources

- [Go Cloud Runtime Variables Documentation](https://gocloud.dev/howto/runtimevar/)
- [Example Applications](../../examples/cmd/config-examples/)
- [Unit Tests](./provider_test.go)
- [Credentials Package](../security/credentials/) - For secret management

## Support

For issues or questions:

1. Check [Troubleshooting](#troubleshooting) section
2. Review [examples](../../examples/cmd/config-examples/)
3. Open an issue on GitHub

## License

See the main project LICENSE file.
