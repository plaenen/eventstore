package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/plaenen/eventstore/examples/bankaccount/handlers"
	accountv1 "github.com/plaenen/eventstore/examples/pb/account/v1"
	"github.com/plaenen/eventstore/pkg/eventsourcing"
	natspkg "github.com/plaenen/eventstore/pkg/nats"
	"github.com/plaenen/eventstore/pkg/observability"
	"github.com/plaenen/eventstore/pkg/sqlite"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

func main() {
	fmt.Println("=== Bank Account Observability Demo ===")
	fmt.Println("This demo shows end-to-end tracing and metrics")
	fmt.Println()

	ctx := context.Background()

	// 1. Start Embedded NATS Server
	fmt.Println("1️⃣  Starting embedded NATS server...")
	opts := &server.Options{
		Host: "127.0.0.1",
		Port: 4222,
	}
	ns, err := server.NewServer(opts)
	if err != nil {
		log.Fatalf("Failed to create NATS server: %v", err)
	}

	go ns.Start()

	if !ns.ReadyForConnections(4 * time.Second) {
		log.Fatal("NATS server not ready")
	}
	defer ns.Shutdown()
	fmt.Println("   ✅ Embedded NATS server ready")
	fmt.Println()

	// 2. Setup Observability
	fmt.Println("2️⃣  Setting up observability with OpenTelemetry...")

	// Create trace exporter (stdout for demo)
	traceExporter, err := stdouttrace.New(
		stdouttrace.WithPrettyPrint(),
	)
	if err != nil {
		log.Fatalf("Failed to create trace exporter: %v", err)
	}

	// Create metric reader (stdout for demo, periodic 10s)
	metricExporter, err := stdoutmetric.New(
		stdoutmetric.WithPrettyPrint(),
	)
	if err != nil {
		log.Fatalf("Failed to create metric exporter: %v", err)
	}

	metricReader := sdkmetric.NewPeriodicReader(
		metricExporter,
		sdkmetric.WithInterval(10*time.Second),
		sdkmetric.WithTimeout(5*time.Second),
	)

	// Initialize telemetry
	telemetry, err := observability.Init(ctx, observability.Config{
		ServiceName:     "BankAccountService",
		ServiceVersion:  "1.0.0",
		Environment:     "demo",
		TraceExporter:   traceExporter,
		TraceSampleRate: 1.0, // Sample everything for demo
		MetricReader:    metricReader,
	})
	if err != nil {
		log.Fatalf("Failed to initialize observability: %v", err)
	}
	defer telemetry.Shutdown(ctx)

	fmt.Println("   ✅ Observability initialized")
	fmt.Println("      - Traces: stdout (pretty printed)")
	fmt.Println("      - Metrics: stdout (every 10 seconds)")
	fmt.Println()

	// 3. Setup SQLite Event Store
	fmt.Println("3️⃣  Setting up SQLite event store...")
	eventStore, err := sqlite.NewEventStore(sqlite.WithMemoryDatabase())
	if err != nil {
		log.Fatalf("Failed to create event store: %v", err)
	}
	defer eventStore.Close()
	fmt.Println("   ✅ Event store ready")
	fmt.Println()

	// 4. Create Repository
	fmt.Println("4️⃣  Creating account repository...")
	repo := accountv1.NewAccountRepository(eventStore)
	fmt.Println("   ✅ Repository created")
	fmt.Println()

	// 5. Create Handlers
	fmt.Println("5️⃣  Creating command and query handlers...")
	commandHandler := handlers.NewAccountCommandHandler(repo)
	queryHandler := handlers.NewAccountQueryHandler(repo)
	fmt.Println("   ✅ Handlers created")
	fmt.Println()

	// 6. Create NATS Server with Observability
	fmt.Println("6️⃣  Starting NATS server with observability...")
	natsServer, err := natspkg.NewServer(&natspkg.ServerConfig{
		ServerConfig: &eventsourcing.ServerConfig{
			QueueGroup:     "bankaccount-handlers",
			MaxConcurrent:  10,
			HandlerTimeout: 5 * time.Second,
		},
		URL:         "nats://localhost:4222",
		Name:        "BankAccountService",
		Version:     "1.0.0",
		Description: "Bank account management service",
		Telemetry:   telemetry, // 🎯 Enable observability
	})
	if err != nil {
		log.Fatalf("Failed to create NATS server: %v", err)
	}
	defer natsServer.Close()
	fmt.Println("   ✅ NATS server created with observability enabled")
	fmt.Println()

	// 7. Start Services
	fmt.Println("7️⃣  Starting server services...")

	commandService := accountv1.NewAccountCommandServiceServer(natsServer, commandHandler)
	if err := commandService.Start(ctx); err != nil {
		log.Fatalf("Failed to start command service: %v", err)
	}
	fmt.Println("   ✅ Command service started (with tracing middleware)")

	queryService := accountv1.NewAccountQueryServiceServer(natsServer, queryHandler)
	if err := queryService.Start(ctx); err != nil {
		log.Fatalf("Failed to start query service: %v", err)
	}
	fmt.Println("   ✅ Query service started (with tracing middleware)")
	fmt.Println()

	time.Sleep(500 * time.Millisecond)

	// 8. Create Client Transport with Observability
	fmt.Println("8️⃣  Creating client transport with observability...")
	transport, err := natspkg.NewTransport(&natspkg.TransportConfig{
		TransportConfig: &eventsourcing.TransportConfig{
			Timeout:              5 * time.Second,
			MaxReconnectAttempts: 3,
			ReconnectWait:        1 * time.Second,
		},
		URL:       "nats://localhost:4222",
		Name:      "bankaccount-client",
		Telemetry: telemetry, // 🎯 Enable observability
	})
	if err != nil {
		log.Fatalf("Failed to create transport: %v", err)
	}
	defer transport.Close()
	fmt.Println("   ✅ Client transport ready with observability enabled")
	fmt.Println()

	// 9. Create SDK Client
	fmt.Println("9️⃣  Creating SDK client...")
	client := accountv1.NewAccountClient(transport)
	fmt.Println("   ✅ SDK client created")
	fmt.Println()

	// 10. Test Complete Flow with Distributed Tracing
	fmt.Println("🔟 Testing flow with distributed tracing...")
	fmt.Println()
	fmt.Println("=" + string(make([]byte, 60)) + "=")
	fmt.Println("All operations below will be traced and metrics collected")
	fmt.Println("=" + string(make([]byte, 60)) + "=")
	fmt.Println()

	accountID := "acc-obs-demo-456"

	// Open Account
	fmt.Println("   📝 Opening account...")
	openResp, appErr := client.OpenAccount(ctx, &accountv1.OpenAccountCommand{
		AccountId:      accountID,
		OwnerName:      "Bob Smith",
		InitialBalance: "2000.00",
	})
	if appErr != nil {
		fmt.Printf("   ❌ Error: [%s] %s\n", appErr.Code, appErr.Message)
	} else {
		fmt.Printf("   ✅ Account opened: %s\n", openResp.AccountId)
	}
	fmt.Println()

	// Deposit
	fmt.Println("   💵 Depositing $1000...")
	depositResp, appErr := client.Deposit(ctx, &accountv1.DepositCommand{
		AccountId: accountID,
		Amount:    "1000.00",
	})
	if appErr != nil {
		fmt.Printf("   ❌ Error: [%s] %s\n", appErr.Code, appErr.Message)
	} else {
		fmt.Printf("   ✅ New balance: %s\n", depositResp.NewBalance)
	}
	fmt.Println()

	// Get Balance
	fmt.Println("   🔍 Checking balance...")
	balance, appErr := client.GetAccountBalance(ctx, &accountv1.GetAccountBalanceRequest{
		AccountId: accountID,
	})
	if appErr != nil {
		fmt.Printf("   ❌ Error: [%s] %s\n", appErr.Code, appErr.Message)
	} else {
		fmt.Printf("   ✅ Current balance: %s\n", balance.Balance)
	}
	fmt.Println()

	// Withdraw
	fmt.Println("   💸 Withdrawing $500...")
	withdrawResp, appErr := client.Withdraw(ctx, &accountv1.WithdrawCommand{
		AccountId: accountID,
		Amount:    "500.00",
	})
	if appErr != nil {
		fmt.Printf("   ❌ Error: [%s] %s\n", appErr.Code, appErr.Message)
	} else {
		fmt.Printf("   ✅ New balance: %s\n", withdrawResp.NewBalance)
	}
	fmt.Println()

	fmt.Println("🎉 Demo Complete!")
	fmt.Println()
	fmt.Println("Summary:")
	fmt.Println("  • All operations were traced with OpenTelemetry")
	fmt.Println("  • Distributed traces show Client → NATS → Server → Handler → Aggregate")
	fmt.Println("  • Metrics collected for commands, events, and repository operations")
	fmt.Println("  • Context propagation maintains trace correlation across services")
	fmt.Println()
	fmt.Println("ℹ️  Check the output above for:")
	fmt.Println("  - Trace spans showing operation flow")
	fmt.Println("  - Timing information for each operation")
	fmt.Println("  - Metrics will be printed every 10 seconds")
	fmt.Println()

	// Wait for metrics to be exported once
	fmt.Println("⏳ Waiting 12 seconds for metrics export...")
	time.Sleep(12 * time.Second)

	fmt.Println()
	fmt.Println("✅ You can now see metrics above!")
	fmt.Println()
	fmt.Println("In production, you would:")
	fmt.Println("  - Replace stdout exporters with OTLP/Prometheus/Jaeger")
	fmt.Println("  - Configure sampling rate based on traffic")
	fmt.Println("  - Add custom spans for business operations")
	fmt.Println("  - Set up alerting based on metrics")
}
