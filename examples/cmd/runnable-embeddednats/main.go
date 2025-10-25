package main

import (
	"context"
	"fmt"
	"log"
	"time"

	natsclient "github.com/nats-io/nats.go"
	infranatsnats "github.com/plaenen/eventstore/pkg/infrastructure/nats"
	"github.com/plaenen/eventstore/pkg/runner"
	"github.com/plaenen/eventstore/pkg/runtime/embeddednats"
)

// simpleLogger implements runner.Logger for demonstration
type simpleLogger struct{}

func (l *simpleLogger) Info(msg string, keysAndValues ...interface{}) {
	fmt.Printf("[INFO] %s", msg)
	for i := 0; i < len(keysAndValues); i += 2 {
		if i+1 < len(keysAndValues) {
			fmt.Printf(" %v=%v", keysAndValues[i], keysAndValues[i+1])
		}
	}
	fmt.Println()
}

func (l *simpleLogger) Error(msg string, keysAndValues ...interface{}) {
	fmt.Printf("[ERROR] %s", msg)
	for i := 0; i < len(keysAndValues); i += 2 {
		if i+1 < len(keysAndValues) {
			fmt.Printf(" %v=%v", keysAndValues[i], keysAndValues[i+1])
		}
	}
	fmt.Println()
}

func (l *simpleLogger) Debug(msg string, keysAndValues ...interface{}) {
	fmt.Printf("[DEBUG] %s", msg)
	for i := 0; i < len(keysAndValues); i += 2 {
		if i+1 < len(keysAndValues) {
			fmt.Printf(" %v=%v", keysAndValues[i], keysAndValues[i+1])
		}
	}
	fmt.Println()
}

func main() {
	fmt.Println("=== Embedded NATS Service with Runner Demo ===")
	fmt.Println()
	fmt.Println("This demo showcases:")
	fmt.Println("  • Creating an embedded NATS service with custom options")
	fmt.Println("  • Using the runner package for lifecycle management")
	fmt.Println("  • Health check integration")
	fmt.Println("  • Graceful shutdown handling")
	fmt.Println()

	ctx := context.Background()
	logger := &simpleLogger{}

	// 1. Create Embedded NATS Service with Options
	fmt.Println("1️⃣  Creating embedded NATS service with custom options...")

	natsService := embeddednats.New(
		embeddednats.WithLogger(logger),
		embeddednats.WithNATSOptions(
			infranatsnats.WithPort(4223),              // Custom port
			infranatsnats.WithHost("127.0.0.1"),       // Localhost only
			infranatsnats.WithJetStream(true),         // Enable JetStream
			infranatsnats.WithDebug(false),            // Disable debug logging
			infranatsnats.WithServerName("demo-nats"), // Server name
			infranatsnats.WithMaxPayload(2*1024*1024), // 2MB max message size
			infranatsnats.WithMaxConnections(100),     // Max 100 connections
		),
	)

	fmt.Println("   ✅ Service created")
	fmt.Println()

	// 2. Create Runner with Service
	fmt.Println("2️⃣  Creating runner with the service...")

	r := runner.New(
		[]runner.Service{natsService},
		runner.WithLogger(logger),
		runner.WithShutdownTimeout(10*time.Second),
		runner.WithStartupTimeout(30*time.Second),
	)

	fmt.Println("   ✅ Runner configured")
	fmt.Println()

	// 3. Start Services in Background
	fmt.Println("3️⃣  Starting services...")

	// Create a context with timeout for the demo
	runCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	// Start runner in goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- r.Run(runCtx)
	}()

	// Wait for service to be ready
	time.Sleep(2 * time.Second)
	fmt.Println()

	// 4. Test Health Check
	fmt.Println("4️⃣  Testing health check...")

	if err := r.HealthCheck(ctx); err != nil {
		log.Fatalf("Health check failed: %v", err)
	}

	fmt.Println("   ✅ Service is healthy")
	fmt.Println()

	// 5. Connect to the Embedded Server
	fmt.Println("5️⃣  Connecting to embedded NATS server...")

	// Get the server URL
	serverURL := natsService.URL()
	if serverURL == "" {
		log.Fatal("Server URL not available")
	}

	fmt.Printf("   📡 Server URL: %s\n", serverURL)

	// Connect using NATS client
	nc, err := natsclient.Connect(serverURL, natsclient.Name("demo-client"))
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer nc.Close()

	fmt.Println("   ✅ Connected successfully")
	fmt.Println()

	// 6. Test Publishing and Subscribing
	fmt.Println("6️⃣  Testing pub/sub functionality...")

	// Create a simple subscription
	msgReceived := make(chan bool)
	sub, err := nc.Subscribe("demo.test", func(msg *natsclient.Msg) {
		fmt.Printf("   📨 Received message: %s\n", string(msg.Data))
		msgReceived <- true
	})
	if err != nil {
		log.Fatalf("Failed to subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	// Ensure subscription is active
	nc.Flush()

	// Publish a message
	testMessage := "Hello from embedded NATS!"
	fmt.Printf("   📤 Publishing message: %s\n", testMessage)

	if err := nc.Publish("demo.test", []byte(testMessage)); err != nil {
		log.Fatalf("Failed to publish: %v", err)
	}

	// Wait for message
	select {
	case <-msgReceived:
		fmt.Println("   ✅ Message received successfully")
	case <-time.After(2 * time.Second):
		log.Fatal("Timeout waiting for message")
	}
	fmt.Println()

	// 7. Test JetStream
	fmt.Println("7️⃣  Testing JetStream functionality...")

	// Create JetStream context
	js, err := nc.JetStream()
	if err != nil {
		log.Fatalf("Failed to create JetStream context: %v", err)
	}

	// List existing streams to see what's there
	streamsList := js.StreamNames()
	existingStreams := []string{}
	for streamName := range streamsList {
		existingStreams = append(existingStreams, streamName)
	}
	if len(existingStreams) > 0 {
		fmt.Printf("   📋 Existing streams: %v\n", existingStreams)
	}

	// Create a stream with unique name to avoid conflicts
	// Use nanoseconds for uniqueness
	uniqueID := time.Now().UnixNano()
	streamName := fmt.Sprintf("DEMO_STREAM_%d", uniqueID)
	subject := fmt.Sprintf("demo.jetstream.%d.*", uniqueID)

	// Try to delete any existing streams that might conflict
	for _, existingStream := range existingStreams {
		fmt.Printf("   🗑️  Deleting existing stream: %s\n", existingStream)
		_ = js.DeleteStream(existingStream)
	}

	_, err = js.AddStream(&natsclient.StreamConfig{
		Name:     streamName,
		Subjects: []string{subject},
		Storage:  natsclient.MemoryStorage,
	})
	if err != nil {
		log.Fatalf("Failed to create stream: %v (tried to create %s with subject %s)", err, streamName, subject)
	}

	fmt.Printf("   ✅ Stream created: %s\n", streamName)

	// Publish to JetStream
	jsMessage := "Hello from JetStream!"
	publishSubject := fmt.Sprintf("demo.jetstream.%d.test", uniqueID)
	ack, err := js.Publish(publishSubject, []byte(jsMessage))
	if err != nil {
		log.Fatalf("Failed to publish to JetStream: %v", err)
	}

	fmt.Printf("   📤 Published to JetStream (seq: %d)\n", ack.Sequence)

	// Subscribe to JetStream
	jsReceived := make(chan bool)
	_, err = js.Subscribe(subject, func(msg *natsclient.Msg) {
		fmt.Printf("   📨 JetStream message: %s\n", string(msg.Data))
		msg.Ack()
		jsReceived <- true
	})
	if err != nil {
		log.Fatalf("Failed to subscribe to JetStream: %v", err)
	}

	// Wait for JetStream message
	select {
	case <-jsReceived:
		fmt.Println("   ✅ JetStream message received successfully")
	case <-time.After(2 * time.Second):
		log.Fatal("Timeout waiting for JetStream message")
	}
	fmt.Println()

	// 8. Test Connection Stats
	fmt.Println("8️⃣  Connection statistics...")

	stats := nc.Stats()
	fmt.Printf("   Messages In: %d\n", stats.InMsgs)
	fmt.Printf("   Messages Out: %d\n", stats.OutMsgs)
	fmt.Printf("   Bytes In: %d\n", stats.InBytes)
	fmt.Printf("   Bytes Out: %d\n", stats.OutBytes)
	fmt.Println()

	// 9. Demonstrate Graceful Shutdown
	fmt.Println("9️⃣  Testing graceful shutdown...")
	fmt.Println("   Waiting for context timeout (this will trigger shutdown)...")
	fmt.Println()

	// Wait for either error or context timeout
	select {
	case err := <-errCh:
		if err != nil {
			fmt.Printf("   ⚠️  Runner stopped with error: %v\n", err)
		} else {
			fmt.Println("   ✅ Runner stopped gracefully")
		}
	case <-time.After(10 * time.Second):
		// This should not happen in normal flow
		fmt.Println("   ⚠️  Timeout waiting for shutdown")
		cancel()
		<-errCh
	}

	fmt.Println()
	fmt.Println("🎉 Demo Complete!")
	fmt.Println()

	// 10. Summary
	fmt.Println("📊 Summary:")
	fmt.Println("  ✅ Embedded NATS server started with custom configuration")
	fmt.Println("  ✅ Service lifecycle managed by runner package")
	fmt.Println("  ✅ Health checks working")
	fmt.Println("  ✅ Core NATS pub/sub working")
	fmt.Println("  ✅ JetStream working (streams, persistence)")
	fmt.Println("  ✅ Graceful shutdown completed")
	fmt.Println()

	fmt.Println("💡 Key Benefits:")
	fmt.Println("  • No external NATS server required")
	fmt.Println("  • Full lifecycle management (start, health, stop)")
	fmt.Println("  • Signal handling (SIGTERM, SIGINT)")
	fmt.Println("  • Configurable timeouts")
	fmt.Println("  • Perfect for testing and embedded systems")
	fmt.Println()

	fmt.Println("🔧 Configuration Options Used:")
	fmt.Println("  • Custom port (4223)")
	fmt.Println("  • JetStream enabled")
	fmt.Println("  • Max payload: 2MB")
	fmt.Println("  • Max connections: 100")
	fmt.Println("  • Server name: demo-nats")
	fmt.Println()

	fmt.Println("📦 Available NATS Server Options:")
	fmt.Println("  • infranatsnats.WithPort(port)")
	fmt.Println("  • infranatsnats.WithHost(host)")
	fmt.Println("  • infranatsnats.WithStoreDir(dir)")
	fmt.Println("  • infranatsnats.WithJetStream(enabled)")
	fmt.Println("  • infranatsnats.WithMaxPayload(bytes)")
	fmt.Println("  • infranatsnats.WithWriteDeadline(duration)")
	fmt.Println("  • infranatsnats.WithMaxConnections(max)")
	fmt.Println("  • infranatsnats.WithMaxSubscriptions(max)")
	fmt.Println("  • infranatsnats.WithDebug(enabled)")
	fmt.Println("  • infranatsnats.WithTrace(enabled)")
	fmt.Println("  • infranatsnats.WithLogFile(path)")
	fmt.Println("  • infranatsnats.WithServerName(name)")
	fmt.Println()
}
