# Runner Package

`pkg/runner` provides a robust service lifecycle manager for Go applications. It handles the orchestration of multiple services, ensuring they start up sequentially and shut down gracefully.

## Features

*   **Sequential Startup**: Services are started in the order they are registered, allowing for dependency management (e.g., start Database before API).
*   **Graceful Shutdown**: Handles OS signals (`SIGINT`, `SIGTERM`) and Context cancellation to shut down services cleanly.
*   **Concurrent Shutdown**: Services are stopped concurrently to minimize shutdown time (Note: ensure your services can handle this if they have strict shutdown dependencies).
*   **Timeouts**: Configurable timeouts for both startup and shutdown operations.
*   **Health Checks**: Integrated support for service health checks.
*   **Dynamic Configuration**: Support for runtime configuration updates via `ConfigurableService` interface.
*   **Error Management**: Aggregates errors from services and ensures proper cleanup even during partial failures.

## Usage

### Basic Example

```go
package main

import (
    "context"
    "log"
    "github.com/plaenen/eventstore/pkg/runner"
)

func main() {
    // Create your services (must implement runner.Service)
    dbService := NewDatabaseService()
    apiService := NewAPIService()

    // Create the runner
    r := runner.New(
        []runner.Service{dbService, apiService},
        runner.WithStartupTimeout(30 * time.Second),
        runner.WithShutdownTimeout(10 * time.Second),
    )

    // Run (blocks until shutdown)
    if err := r.Run(context.Background()); err != nil {
        log.Fatalf("Runner failed: %v", err)
    }
}
```

### Implementing a Service

Implement the `runner.Service` interface:

```go
type Service interface {
    Name() string
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
}
```

### Optional Interfaces

*   **`HealthChecker`**: Implement `HealthCheck(ctx context.Context) error` to participate in health checks.
*   **`ConfigurableService`**: Implement `UpdateConfig(ctx context.Context, config interface{}) error` to receive dynamic config updates.
*   **`ConfigValidator`**: Implement `ValidateConfig(config interface{}) error` to validate config before application.

## Configuration

The runner can be configured using functional options:

*   `WithLogger(logger Logger)`: Set a custom logger.
*   `WithStartupTimeout(d time.Duration)`: Set max time for services to start.
*   `WithShutdownTimeout(d time.Duration)`: Set max time for services to stop.
*   `WithConfigProvider(provider interface{})`: Enable dynamic configuration watching.

## Best Practices

1.  **Order Matters**: Register services in dependency order (independent services first).
2.  **Context Propagation**: Always respect the `ctx` passed to `Start` and `Stop`.
3.  **Idempotency**: `Stop` should be safe to call multiple times or on an unstarted service.
4.  **Fast Shutdown**: Keep `Stop` implementations fast to ensure clean termination within the timeout.
