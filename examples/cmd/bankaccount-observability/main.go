package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/plaenen/eventstore/examples/bankaccount/domain"
	"github.com/plaenen/eventstore/examples/bankaccount/handlers"
	accountv1 "github.com/plaenen/eventstore/examples/pb/account/v1"
	"github.com/plaenen/eventstore/pkg/cqrs"
	cqrsnats "github.com/plaenen/eventstore/pkg/cqrs/nats"
	"github.com/plaenen/eventstore/pkg/observability"
	"github.com/plaenen/eventstore/pkg/store/sqlite"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	_ "modernc.org/sqlite" // SQLite driver
)

func main() {
	fmt.Println("=== Bank Account SQLite Observability Demo ===")
	fmt.Println("Single-binary application with file-based storage")
	fmt.Println()
	fmt.Println("This demo creates two SQLite database files:")
	fmt.Println("  • eventstore.db - Event sourcing data (aggregates, events)")
	fmt.Println("  • observability.db - Traces and metrics")
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

	// 2. Setup SQLite for Observability
	fmt.Println("2️⃣  Setting up SQLite for observability...")

	// Create separate database file for observability data
	observabilityDBPath := "./observability.db"
	// Configure SQLite for concurrent access with WAL mode
	observabilityDB, err := sql.Open("sqlite", observabilityDBPath+"?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL")
	if err != nil {
		log.Fatalf("Failed to open observability database: %v", err)
	}
	defer observabilityDB.Close()

	// Set connection pool settings for better concurrency
	observabilityDB.SetMaxOpenConns(1) // SQLite works best with single writer
	observabilityDB.SetMaxIdleConns(1)

	fmt.Printf("   📁 Observability database: %s\n", observabilityDBPath)

	// Create SQLite exporters
	exporterConfig := observability.DefaultSQLiteExporterConfig(observabilityDB)
	exporterConfig.RetentionDays = 7 // Keep 1 week

	traceExporter, err := observability.NewSQLiteTraceExporter(exporterConfig)
	if err != nil {
		log.Fatalf("Failed to create trace exporter: %v", err)
	}

	metricExporter, err := observability.NewSQLiteMetricExporter(exporterConfig)
	if err != nil {
		log.Fatalf("Failed to create metric exporter: %v", err)
	}

	// Create metric reader that exports every 5 seconds
	metricReader := sdkmetric.NewPeriodicReader(
		metricExporter,
		sdkmetric.WithInterval(5*time.Second),
		sdkmetric.WithTimeout(3*time.Second),
	)

	// Initialize telemetry with SQLite backends
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

	fmt.Println("   ✅ SQLite observability initialized")
	fmt.Println("      - Traces: SQLite database (queryable)")
	fmt.Println("      - Metrics: SQLite database (queryable)")
	fmt.Println("      - Retention: 7 days")
	fmt.Println()

	// 3. Setup SQLite Event Store
	fmt.Println("3️⃣  Setting up SQLite event store...")

	// Create separate database file for event store with WAL mode and pragmas for concurrency
	eventStoreDBPath := "./eventstore.db?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL"
	eventStore, err := sqlite.NewEventStore(
		sqlite.WithFilename(eventStoreDBPath),
		sqlite.WithWALMode(true),
	)
	if err != nil {
		log.Fatalf("Failed to create event store: %v", err)
	}
	defer eventStore.Close()

	fmt.Printf("   📁 Event store database: ./eventstore.db\n")
	fmt.Println("   ✅ Event store ready (WAL mode enabled)")
	fmt.Println()

	// 4. Create Repository and Handlers
	fmt.Println("4️⃣  Creating repository and handlers...")
	repo := accountv1.NewAccountRepository(eventStore, domain.NewAccount)
	commandHandler := handlers.NewAccountCommandHandler(repo)
	queryHandler := handlers.NewAccountQueryHandler(repo)
	fmt.Println("   ✅ Ready")
	fmt.Println()

	// 5. Create NATS Server with Observability
	fmt.Println("5️⃣  Starting NATS server with observability...")
	natsServer, err := cqrsnats.NewServer(&cqrsnats.ServerConfig{
		ServerConfig: &cqrs.ServerConfig{
			QueueGroup:     "bankaccount-handlers",
			MaxConcurrent:  10,
			HandlerTimeout: 5 * time.Second,
		},
		URL:         "nats://localhost:4222",
		Name:        "BankAccountService",
		Version:     "1.0.0",
		Description: "Bank account management service",
		Telemetry:   telemetry,
	})
	if err != nil {
		log.Fatalf("Failed to create NATS server: %v", err)
	}
	defer natsServer.Close()
	fmt.Println("   ✅ NATS server created")
	fmt.Println()

	// 6. Start Services
	fmt.Println("6️⃣  Starting services...")
	commandService := accountv1.NewAccountCommandServiceServer(natsServer, commandHandler)
	if err := commandService.Start(ctx); err != nil {
		log.Fatalf("Failed to start command service: %v", err)
	}

	queryService := accountv1.NewAccountQueryServiceServer(natsServer, queryHandler)
	if err := queryService.Start(ctx); err != nil {
		log.Fatalf("Failed to start query service: %v", err)
	}
	fmt.Println("   ✅ Services started")
	fmt.Println()

	time.Sleep(500 * time.Millisecond)

	// 7. Create Client with Observability
	fmt.Println("7️⃣  Creating client...")
	transport, err := cqrsnats.NewTransport(&cqrsnats.TransportConfig{
		TransportConfig: &cqrs.TransportConfig{
			Timeout:              5 * time.Second,
			MaxReconnectAttempts: 3,
			ReconnectWait:        1 * time.Second,
		},
		URL:       "nats://localhost:4222",
		Name:      "bankaccount-client",
		Telemetry: telemetry,
	})
	if err != nil {
		log.Fatalf("Failed to create transport: %v", err)
	}
	defer transport.Close()

	client := accountv1.NewAccountClient(transport)
	fmt.Println("   ✅ Client ready")
	fmt.Println()

	// 8. Execute Transactions
	fmt.Println("8️⃣  Executing transactions...")
	fmt.Println()

	accountID := "acc-sqlite-demo-789"

	// Open Account
	fmt.Println("   📝 Opening account...")
	openResp, appErr := client.OpenAccount(ctx, &accountv1.OpenAccountCommand{
		AccountId:      accountID,
		OwnerName:      "Charlie Brown",
		InitialBalance: "3000.00",
	})
	if appErr != nil {
		fmt.Printf("   ❌ Error: [%s] %s\n", appErr.Code, appErr.Message)
	} else {
		fmt.Printf("   ✅ Account opened: %s\n", openResp.AccountId)
	}

	// Multiple deposits (transport layer handles retries automatically)
	for i := 1; i <= 3; i++ {
		fmt.Printf("   💵 Deposit #%d ($%d00)...\n", i, i)
		resp, appErr := client.Deposit(ctx, &accountv1.DepositCommand{
			AccountId: accountID,
			Amount:    fmt.Sprintf("%d00.00", i),
		})
		if appErr != nil {
			fmt.Printf("   ❌ Error: [%s] %s\n", appErr.Code, appErr.Message)
		} else {
			fmt.Printf("   ✅ New balance: %s\n", resp.NewBalance)
		}
	}

	// Multiple withdrawals (transport layer handles retries automatically)
	for i := 1; i <= 2; i++ {
		fmt.Printf("   💸 Withdrawal #%d ($%d50)...\n", i, i)
		resp, appErr := client.Withdraw(ctx, &accountv1.WithdrawCommand{
			AccountId: accountID,
			Amount:    fmt.Sprintf("%d50.00", i),
		})
		if appErr != nil {
			fmt.Printf("   ❌ Error: [%s] %s\n", appErr.Code, appErr.Message)
		} else {
			fmt.Printf("   ✅ New balance: %s\n", resp.NewBalance)
		}
	}

	// Get balance
	balance, appErr := client.GetAccountBalance(ctx, &accountv1.GetAccountBalanceRequest{
		AccountId: accountID,
	})
	if appErr != nil {
		fmt.Printf("   ❌ Error: [%s] %s\n", appErr.Code, appErr.Message)
	} else {
		fmt.Printf("   ✅ Final balance: %s\n", balance.Balance)
	}

	fmt.Println()

	// 9. Wait for metrics to be exported
	fmt.Println("9️⃣  Waiting for metrics export...")
	time.Sleep(6 * time.Second)
	fmt.Println("   ✅ Metrics exported to SQLite")
	fmt.Println()

	// 10. Query Observability Data
	fmt.Println("🔟 Querying observability data from SQLite...")
	fmt.Println()

	queries := observability.NewSQLiteObservabilityQueries(observabilityDB, exporterConfig)

	// Query recent traces
	fmt.Println("   📊 Recent traces:")
	traces, err := queries.QueryTraces(time.Time{}, time.Now(), 10)
	if err != nil {
		log.Printf("Failed to query traces: %v", err)
	} else {
		fmt.Printf("      Found %d traces\n", len(traces))
		for _, trace := range traces {
			fmt.Printf("      - Trace: %s (created: %s)\n",
				trace.TraceID[:16]+"...", trace.CreatedAt.Format("15:04:05"))
		}
	}
	fmt.Println()

	// Query spans
	fmt.Println("   🔗 Spans for operations:")
	spans, err := queries.QuerySpans(observability.TraceQuery{
		Limit: 20,
	})
	if err != nil {
		log.Printf("Failed to query spans: %v", err)
	} else {
		fmt.Printf("      Found %d spans\n", len(spans))
		spansByName := make(map[string]int)
		var totalDuration int64
		for _, span := range spans {
			spansByName[span.Name]++
			totalDuration += span.DurationMs
		}
		for name, count := range spansByName {
			fmt.Printf("      - %s: %d spans\n", name, count)
		}
		if len(spans) > 0 {
			fmt.Printf("      Average duration: %dms\n", totalDuration/int64(len(spans)))
		}
	}
	fmt.Println()

	// Query metrics
	fmt.Println("   📈 Metric summary:")
	metricNames := []string{
		"eventsourcing.command.total",
		"eventsourcing.command.duration",
		"eventsourcing.events.appended",
	}

	for _, metricName := range metricNames {
		summary, err := queries.GetMetricSummary(metricName, time.Time{}, time.Now())
		if err != nil {
			log.Printf("Failed to query metric %s: %v", metricName, err)
			continue
		}
		if summary != nil {
			fmt.Printf("      - %s:\n", metricName)
			if count, ok := summary["count"].(int64); ok {
				fmt.Printf("        Count: %d\n", count)
			}
			if avgValue, ok := summary["avg_value"].(float64); ok {
				fmt.Printf("        Average: %.3f\n", avgValue)
			}
		}
	}
	fmt.Println()

	// Query detailed metrics
	fmt.Println("   🔍 Command metrics details:")
	cmdMetrics, err := queries.QueryMetrics(observability.MetricQuery{
		Name:  "eventsourcing.command.total",
		Limit: 10,
	})
	if err != nil {
		log.Printf("Failed to query command metrics: %v", err)
	} else {
		for _, m := range cmdMetrics {
			if cmdType, ok := m.Attributes["command_type"].(string); ok {
				if m.Value != nil {
					fmt.Printf("      - %s: count=%.0f\n", cmdType, *m.Value)
				} else if m.Count != nil {
					fmt.Printf("      - %s: count=%d\n", cmdType, *m.Count)
				}
			}
		}
	}
	fmt.Println()

	// Summary
	fmt.Println("🎉 Demo Complete!")
	fmt.Println()
	fmt.Println("📁 Database Files Created:")
	fmt.Println("  • eventstore.db - Contains all event sourcing data")
	fmt.Println("  • observability.db - Contains traces and metrics")
	fmt.Println()
	fmt.Println("Summary:")
	fmt.Println("  ✅ All data persisted to disk")
	fmt.Println("  ✅ Traces are queryable with full span details")
	fmt.Println("  ✅ Metrics are queryable with aggregations")
	fmt.Println("  ✅ Perfect for single-binary applications")
	fmt.Println()
	fmt.Println("Benefits:")
	fmt.Println("  • No external dependencies (Jaeger, Prometheus, etc.)")
	fmt.Println("  • All data in SQLite - easy to backup/restore")
	fmt.Println("  • Query with standard SQL tools")
	fmt.Println("  • Great for embedded systems, edge devices, CLIs")
	fmt.Println("  • Can migrate to external backends later")
	fmt.Println()
	fmt.Println("Event Store Tables (eventstore.db):")
	fmt.Println("  • events - All domain events")
	fmt.Println("  • snapshots - Aggregate snapshots")
	fmt.Println()
	fmt.Println("Observability Tables (observability.db):")
	fmt.Println("  • otel_traces - Trace metadata")
	fmt.Println("  • otel_spans - Individual span data with full context")
	fmt.Println("  • otel_metrics - Time-series metric data")
	fmt.Println()
	fmt.Println("Query Examples:")
	fmt.Println("  # View all events")
	fmt.Println("  sqlite3 eventstore.db \"SELECT aggregate_id, event_type, version FROM events\"")
	fmt.Println()
	fmt.Println("  # View traces")
	fmt.Println("  sqlite3 observability.db \"SELECT trace_id, COUNT(*) as span_count FROM otel_spans GROUP BY trace_id\"")
	fmt.Println()
	fmt.Println("  # View metrics")
	fmt.Println("  sqlite3 observability.db \"SELECT name, type, COUNT(*) FROM otel_metrics GROUP BY name, type\"")
	fmt.Println()
	fmt.Println("  # Slow operations")
	fmt.Println("  sqlite3 observability.db \"SELECT name, (end_time - start_time)/1000000 as duration_ms FROM otel_spans ORDER BY duration_ms DESC LIMIT 10\"")
	fmt.Println()
	fmt.Println("💡 Tip: Keep these database files for historical analysis!")
	fmt.Println("   You can query past performance, debug issues, or migrate to production backends.")
}
