# Embedded NATS with Runner Example

This example demonstrates how to use the embedded NATS service with the runner package for complete lifecycle management.

## What This Example Shows

1. **Creating an Embedded NATS Service** - Configure and create an embedded NATS server with custom options
2. **Runner Integration** - Use the runner package for service lifecycle management
3. **Health Checks** - Demonstrate health check integration
4. **Core NATS Pub/Sub** - Basic NATS publish/subscribe functionality
5. **JetStream** - Stream creation and message persistence
6. **Graceful Shutdown** - Proper service shutdown handling

## Running the Example

```bash
go run ./examples/cmd/runnable-embeddednats/main.go
```

## What Happens

The example performs the following steps:

### 1. Service Creation
Creates an embedded NATS service with custom configuration:
- Custom port (4223)
- JetStream enabled
- 2MB max message payload
- Max 100 connections
- Custom server name

### 2. Runner Setup
Configures the runner with:
- Custom logger
- 10-second shutdown timeout
- 30-second startup timeout

### 3. Service Startup
Starts the NATS server through the runner, which handles:
- Sequential service startup
- Startup timeout management
- Error handling

### 4. Health Check
Verifies the service is healthy and ready to accept connections.

### 5. Connection Test
Connects to the embedded server using the NATS client and verifies connectivity.

### 6. Pub/Sub Test
Tests basic NATS functionality:
- Creates a subscription
- Publishes a message
- Receives the message

### 7. JetStream Test
Tests JetStream functionality:
- Lists and cleans up existing streams
- Creates a new stream
- Publishes a message to the stream
- Subscribes and receives the message
- Acknowledges the message

### 8. Connection Stats
Displays connection statistics including message and byte counts.

### 9. Graceful Shutdown
Demonstrates graceful shutdown:
- Context timeout triggers shutdown
- Runner stops services in reverse order
- Proper cleanup

## Configuration Options

The example showcases these NATS server options:

```go
infranatsnats.WithPort(4223)                    // Custom port
infranatsnats.WithHost("127.0.0.1")            // Localhost only
infranatsnats.WithJetStream(true)              // Enable JetStream
infranatsnats.WithDebug(false)                 // Disable debug logging
infranatsnats.WithServerName("demo-nats")      // Server name
infranatsnats.WithMaxPayload(2 * 1024 * 1024)  // 2MB max message
infranatsnats.WithMaxConnections(100)          // Max 100 connections
```

### Available Options

All available NATS server configuration options:

- `WithPort(port)` - Set server port (-1 for random)
- `WithHost(host)` - Set host address (default: "127.0.0.1")
- `WithStoreDir(dir)` - Set JetStream storage directory
- `WithJetStream(enabled)` - Enable/disable JetStream
- `WithMaxPayload(bytes)` - Set max message payload size
- `WithWriteDeadline(duration)` - Set write deadline for connections
- `WithMaxConnections(max)` - Set max client connections
- `WithMaxSubscriptions(max)` - Set max subscriptions per connection
- `WithDebug(enabled)` - Enable debug logging
- `WithTrace(enabled)` - Enable trace logging
- `WithLogFile(path)` - Set log file path
- `WithServerName(name)` - Set server name

## Runner Options

The runner can be configured with:

```go
runner.WithLogger(logger)                      // Custom logger
runner.WithShutdownTimeout(10 * time.Second)   // Shutdown timeout
runner.WithStartupTimeout(30 * time.Second)    // Startup timeout
```

## Key Benefits

- **No External Dependencies** - Completely self-contained
- **Full Lifecycle Management** - Start, health check, stop
- **Signal Handling** - Automatic SIGTERM/SIGINT handling
- **Configurable Timeouts** - Control startup and shutdown timing
- **Perfect for Testing** - No need to manage external NATS servers
- **Production Ready** - Can be used in embedded systems and production

## Use Cases

This pattern is ideal for:

1. **Integration Testing** - No external NATS server required
2. **Development** - Quick local development setup
3. **Embedded Systems** - Self-contained service deployments
4. **CLI Tools** - Applications that need embedded messaging
5. **Edge Computing** - Lightweight deployments at the edge
6. **Prototyping** - Rapid application development

## Output

The example produces detailed output showing:

- Service creation and configuration
- Startup progress with logging
- Health check results
- Pub/sub message flow
- JetStream operations
- Connection statistics
- Shutdown sequence

## Notes

- The example uses a 15-second context timeout to trigger shutdown automatically
- JetStream streams are cleaned up from previous runs to avoid conflicts
- Memory storage is used for JetStream (data not persisted across runs)
- The runner handles signal interrupts (Ctrl+C) gracefully

## Next Steps

To use this pattern in your application:

1. Create your embedded NATS service with desired options
2. Add it to the runner along with other services
3. Implement proper health checks
4. Configure appropriate timeouts for your use case
5. Deploy and monitor

See the [runtime/embeddednats documentation](../../../pkg/runtime/embeddednats/) for more details on the service implementation.
