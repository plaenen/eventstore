package loadtest

import (
	"context"
	"fmt"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/plaenen/eventstore/examples/bankaccount/handlers"
	accountv1 "github.com/plaenen/eventstore/examples/pb/account/v1"
	"github.com/plaenen/eventstore/pkg/eventsourcing"
	natspkg "github.com/plaenen/eventstore/pkg/nats"
	"github.com/plaenen/eventstore/pkg/observability"
	"github.com/plaenen/eventstore/pkg/sqlite"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// TestDeps holds all dependencies for load tests
type TestDeps struct {
	EventStore   eventsourcing.EventStore
	Repo         *accountv1.AccountRepository
	Handler      *handlers.AccountCommandHandler
	QueryHandler *handlers.AccountQueryHandler
	NATSServer   *server.Server
	Server       *natspkg.Server
	Transport    eventsourcing.Transport
	Client       *accountv1.AccountClient
	Telemetry    *observability.Telemetry
	ctx          context.Context
	cancel       context.CancelFunc
	t            testing.TB
}

// SetupTestDeps creates all test dependencies
func SetupTestDeps(t testing.TB) *TestDeps {
	ctx, cancel := context.WithCancel(context.Background())

	// Create in-memory event store for fast tests
	eventStore, err := sqlite.NewEventStore(
		sqlite.WithMemoryDatabase(),
		sqlite.WithWALMode(true),
	)
	require.NoError(t, err)

	// Create repository and handlers
	repo := accountv1.NewAccountRepository(eventStore)
	commandHandler := handlers.NewAccountCommandHandler(repo)
	queryHandler := handlers.NewAccountQueryHandler(repo)

	deps := &TestDeps{
		EventStore:   eventStore,
		Repo:         repo,
		Handler:      commandHandler,
		QueryHandler: queryHandler,
		ctx:          ctx,
		cancel:       cancel,
		t:            t,
	}

	return deps
}

// SetupFullStack creates dependencies with NATS server
func SetupFullStack(t testing.TB) *TestDeps {
	deps := SetupTestDeps(t)

	// Start embedded NATS server
	opts := &server.Options{
		Host: "127.0.0.1",
		Port: -1, // Random port
	}
	ns, err := server.NewServer(opts)
	require.NoError(t, err)

	go ns.Start()

	if !ns.ReadyForConnections(4 * time.Second) {
		t.Fatal("NATS server not ready")
	}

	deps.NATSServer = ns

	// Create NATS server with handlers
	natsServer, err := natspkg.NewServer(&natspkg.ServerConfig{
		ServerConfig: &eventsourcing.ServerConfig{
			QueueGroup:     "loadtest-handlers",
			MaxConcurrent:  100,
			HandlerTimeout: 30 * time.Second,
		},
		URL:         ns.ClientURL(),
		Name:        "LoadTestService",
		Version:     "1.0.0",
		Description: "Load test service",
	})
	require.NoError(t, err)

	deps.Server = natsServer

	// Register handlers
	commandService := accountv1.NewAccountCommandServiceServer(natsServer, deps.Handler)
	queryService := accountv1.NewAccountQueryServiceServer(natsServer, deps.QueryHandler)

	err = commandService.Start(deps.ctx)
	require.NoError(t, err)

	err = queryService.Start(deps.ctx)
	require.NoError(t, err)

	// Create client transport
	transport, err := natspkg.NewTransport(&natspkg.TransportConfig{
		TransportConfig: eventsourcing.DefaultTransportConfig(),
		URL:             ns.ClientURL(),
		Name:            "loadtest-client",
	})
	require.NoError(t, err)

	deps.Transport = transport
	deps.Client = accountv1.NewAccountClient(transport)

	return deps
}

// Cleanup closes all resources
func (d *TestDeps) Cleanup() {
	if d.cancel != nil {
		d.cancel()
	}
	if d.Transport != nil {
		d.Transport.Close()
	}
	if d.Server != nil {
		d.Server.Close()
	}
	if d.NATSServer != nil {
		d.NATSServer.Shutdown()
	}
	if d.EventStore != nil {
		d.EventStore.Close()
	}
	if d.Telemetry != nil {
		d.Telemetry.Shutdown(context.Background())
	}
}

// CreateAccount creates a new account
func CreateAccount(t testing.TB, handler *handlers.AccountCommandHandler, accountID string, initialBalance string) {
	_, err := handler.OpenAccount(context.Background(), &accountv1.OpenAccountCommand{
		AccountId:      accountID,
		OwnerName:      "Load Test User",
		InitialBalance: initialBalance,
	})
	require.Nil(t, err, "Failed to create account %s", accountID)
}

// CreateAccounts creates multiple accounts
func CreateAccounts(t testing.TB, handler *handlers.AccountCommandHandler, count int, initialBalance string) []string {
	accountIDs := make([]string, count)
	for i := 0; i < count; i++ {
		accountIDs[i] = fmt.Sprintf("loadtest-account-%d", i)
		CreateAccount(t, handler, accountIDs[i], initialBalance)
	}
	return accountIDs
}

// VerifyAccountBalance verifies account balance matches expected
func VerifyAccountBalance(t testing.TB, repo *accountv1.AccountRepository, accountID string, expectedBalance string) {
	agg, err := repo.Load(accountID)
	require.NoError(t, err, "Failed to load account %s", accountID)

	expected, _ := decimal.NewFromString(expectedBalance)
	actual, _ := decimal.NewFromString(agg.Balance)

	require.True(t, expected.Equal(actual),
		"Balance mismatch for account %s: expected %s, got %s",
		accountID, expectedBalance, agg.Balance)
}

// CalculateBalanceFromEvents recalculates balance from event stream
func CalculateBalanceFromEvents(events []*eventsourcing.Event) decimal.Decimal {
	balance := decimal.Zero

	for _, event := range events {
		switch event.EventType {
		case "accountv1.AccountOpenedEvent":
			var e accountv1.AccountOpenedEvent
			proto.Unmarshal(event.Data, &e)
			balance, _ = decimal.NewFromString(e.InitialBalance)

		case "accountv1.MoneyDepositedEvent":
			var e accountv1.MoneyDepositedEvent
			proto.Unmarshal(event.Data, &e)
			balance, _ = decimal.NewFromString(e.NewBalance)

		case "accountv1.MoneyWithdrawnEvent":
			var e accountv1.MoneyWithdrawnEvent
			proto.Unmarshal(event.Data, &e)
			balance, _ = decimal.NewFromString(e.NewBalance)
		}
	}

	return balance
}

// LogResourceUsage logs current memory and GC stats
func LogResourceUsage(t testing.TB) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	t.Logf("Memory: Alloc=%v MB, TotalAlloc=%v MB, Sys=%v MB, NumGC=%v, Goroutines=%d",
		m.Alloc/1024/1024,
		m.TotalAlloc/1024/1024,
		m.Sys/1024/1024,
		m.NumGC,
		runtime.NumGoroutine())
}

// Metrics tracks test metrics
type Metrics struct {
	TotalOperations int64
	SuccessfulOps   int64
	FailedOps       int64
	BusinessErrors  int64
	SystemErrors    int64
	TotalLatency    int64 // nanoseconds
	MinLatency      int64
	MaxLatency      int64
	StartTime       time.Time
}

// NewMetrics creates a new metrics tracker
func NewMetrics() *Metrics {
	return &Metrics{
		StartTime:  time.Now(),
		MinLatency: 1<<63 - 1, // Max int64
	}
}

// RecordOperation records an operation result
func (m *Metrics) RecordOperation(success bool, isBusinessError bool, latency time.Duration) {
	atomic.AddInt64(&m.TotalOperations, 1)

	if success {
		atomic.AddInt64(&m.SuccessfulOps, 1)
	} else {
		atomic.AddInt64(&m.FailedOps, 1)
		if isBusinessError {
			atomic.AddInt64(&m.BusinessErrors, 1)
		} else {
			atomic.AddInt64(&m.SystemErrors, 1)
		}
	}

	latencyNs := latency.Nanoseconds()
	atomic.AddInt64(&m.TotalLatency, latencyNs)

	// Update min/max
	for {
		old := atomic.LoadInt64(&m.MinLatency)
		if latencyNs >= old {
			break
		}
		if atomic.CompareAndSwapInt64(&m.MinLatency, old, latencyNs) {
			break
		}
	}

	for {
		old := atomic.LoadInt64(&m.MaxLatency)
		if latencyNs <= old {
			break
		}
		if atomic.CompareAndSwapInt64(&m.MaxLatency, old, latencyNs) {
			break
		}
	}
}

// Report prints a summary report
func (m *Metrics) Report(t testing.TB) {
	duration := time.Since(m.StartTime)
	total := atomic.LoadInt64(&m.TotalOperations)
	successful := atomic.LoadInt64(&m.SuccessfulOps)
	failed := atomic.LoadInt64(&m.FailedOps)
	businessErrors := atomic.LoadInt64(&m.BusinessErrors)
	systemErrors := atomic.LoadInt64(&m.SystemErrors)
	totalLatency := atomic.LoadInt64(&m.TotalLatency)
	minLatency := atomic.LoadInt64(&m.MinLatency)
	maxLatency := atomic.LoadInt64(&m.MaxLatency)

	throughput := float64(total) / duration.Seconds()
	avgLatency := time.Duration(0)
	if total > 0 {
		avgLatency = time.Duration(totalLatency / total)
	}

	t.Logf("\n=== Load Test Results ===")
	t.Logf("Duration:           %v", duration)
	t.Logf("Total Operations:   %d", total)
	t.Logf("Successful:         %d (%.2f%%)", successful, float64(successful)/float64(total)*100)
	t.Logf("Failed:             %d (%.2f%%)", failed, float64(failed)/float64(total)*100)
	t.Logf("  Business Errors:  %d", businessErrors)
	t.Logf("  System Errors:    %d", systemErrors)
	t.Logf("Throughput:         %.2f ops/sec", throughput)
	t.Logf("Latency:")
	t.Logf("  Min:              %v", time.Duration(minLatency))
	t.Logf("  Avg:              %v", avgLatency)
	t.Logf("  Max:              %v", time.Duration(maxLatency))

	LogResourceUsage(t)
}

// UnmarshalEvent is a helper to unmarshal events
func UnmarshalEvent(event *eventsourcing.Event, msg interface{}) error {
	// This would use proto.Unmarshal in real implementation
	// For now, we'll skip the implementation as it's just a helper
	return nil
}
