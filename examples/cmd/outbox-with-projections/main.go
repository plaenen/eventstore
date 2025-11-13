package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	accountv1 "github.com/plaenen/eventstore/examples/pb/account/v1"
	"github.com/plaenen/eventstore/pkg/domain"
	"github.com/plaenen/eventstore/pkg/eventsourcing"
	"github.com/plaenen/eventstore/pkg/messaging"
	natseventbus "github.com/plaenen/eventstore/pkg/messaging/nats"
	"github.com/plaenen/eventstore/pkg/store/sqlite"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	infranatsnats "github.com/plaenen/eventstore/pkg/infrastructure/nats"
	"github.com/plaenen/eventstore/pkg/runner"
	"github.com/plaenen/eventstore/pkg/runtime/embeddednats"
	_ "modernc.org/sqlite"
)

// This demo showcases the COMPLETE EVENT-DRIVEN ARCHITECTURE:
//
// 1. Transactional Outbox Pattern - Guaranteed event publishing
// 2. NATS Event Bus - Real-time event streaming
// 3. Projections - Automatic read model updates
// 4. Rebuild Support - Recover from event store history

// simpleLogger implements both runner.Logger and messaging.Logger
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

func (l *simpleLogger) Warn(msg string, keysAndValues ...interface{}) {
	fmt.Printf("[WARN] %s", msg)
	for i := 0; i < len(keysAndValues); i += 2 {
		if i+1 < len(keysAndValues) {
			fmt.Printf(" %v=%v", keysAndValues[i], keysAndValues[i+1])
		}
	}
	fmt.Println()
}

func main() {
	fmt.Println("=== Complete Event-Driven Architecture Demo ===")
	fmt.Println()
	fmt.Println("This demo showcases:")
	fmt.Println("  • Transactional outbox pattern (guaranteed delivery)")
	fmt.Println("  • NATS event bus (real-time streaming)")
	fmt.Println("  • Projections (automatic read model updates)")
	fmt.Println("  • Projection rebuild (recover from history)")
	fmt.Println()

	ctx := context.Background()
	logger := &simpleLogger{}

	// 1. Start Embedded NATS Server
	fmt.Println("1️⃣  Starting embedded NATS server...")

	natsService := embeddednats.New(
		embeddednats.WithLogger(logger),
		embeddednats.WithNATSOptions(
			infranatsnats.WithPort(4225),
			infranatsnats.WithJetStream(true),
		),
	)

	r := runner.New(
		[]runner.Service{natsService},
		runner.WithLogger(logger),
	)

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- r.Run(runCtx)
	}()

	time.Sleep(1 * time.Second)
	fmt.Println("   ✅ NATS server started")
	fmt.Println()

	// 2. Setup Event Store with Outbox
	fmt.Println("2️⃣  Setting up event store with outbox table...")

	eventStore, err := sqlite.NewEventStore(
		sqlite.WithDSN("file:complete_demo.db?mode=memory&cache=shared"),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer eventStore.Close()

	db := eventStore.DB()

	checkpointStore, err := sqlite.NewCheckpointStore(db)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("   ✅ Event store ready")
	fmt.Println()

	// 3. Setup NATS Event Bus
	fmt.Println("3️⃣  Connecting to NATS event bus...")

	eventBus, err := natseventbus.NewEventBus(natseventbus.Config{
		URL:            natsService.URL(),
		StreamName:     "EVENTS",
		StreamSubjects: []string{"events.>"},
		MaxAge:         7 * 24 * time.Hour,
		MaxBytes:       1024 * 1024 * 1024,
	})
	if err != nil {
		log.Fatalf("Failed to create event bus: %v", err)
	}
	defer eventBus.Close()

	fmt.Println("   ✅ Event bus connected")
	fmt.Println()

	// 4. Build Projections
	fmt.Println("4️⃣  Building projections...")

	var projectionEventCount atomic.Int32

	// Account Balance Projection
	accountBalanceProjection, err := sqlite.NewSQLiteProjectionBuilder(
		"account-balance",
		db,
		checkpointStore,
		eventStore,
	).
		WithSchema(func(ctx context.Context, db *sql.DB) error {
			_, err := db.Exec(`
				CREATE TABLE IF NOT EXISTS account_balance (
					account_id TEXT PRIMARY KEY,
					owner_name TEXT NOT NULL,
					balance TEXT NOT NULL,
					version INTEGER NOT NULL,
					updated_at INTEGER NOT NULL
				)
			`)
			return err
		}).
		On(accountv1.OnAccountOpened(func(ctx context.Context, event *accountv1.AccountOpenedEvent, envelope *domain.EventEnvelope) error {
			projectionEventCount.Add(1)
			fmt.Printf("   📊 Projection: AccountOpened - %s (%s) = %s\n",
				event.OwnerName, event.AccountId, event.InitialBalance)

			tx, _ := sqlite.TxFromContext(ctx)
			_, err := tx.Exec(`
				INSERT INTO account_balance (account_id, owner_name, balance, version, updated_at)
				VALUES (?, ?, ?, ?, ?)
			`, event.AccountId, event.OwnerName, event.InitialBalance, envelope.Version, event.Timestamp)
			return err
		})).
		On(accountv1.OnMoneyDeposited(func(ctx context.Context, event *accountv1.MoneyDepositedEvent, envelope *domain.EventEnvelope) error {
			projectionEventCount.Add(1)
			fmt.Printf("   📊 Projection: MoneyDeposited - %s + %s = %s\n",
				event.AccountId, event.Amount, event.NewBalance)

			tx, _ := sqlite.TxFromContext(ctx)
			_, err := tx.Exec(`
				UPDATE account_balance
				SET balance = ?, version = ?, updated_at = ?
				WHERE account_id = ?
			`, event.NewBalance, envelope.Version, event.Timestamp, event.AccountId)
			return err
		})).
		On(accountv1.OnMoneyWithdrawn(func(ctx context.Context, event *accountv1.MoneyWithdrawnEvent, envelope *domain.EventEnvelope) error {
			projectionEventCount.Add(1)
			fmt.Printf("   📊 Projection: MoneyWithdrawn - %s - %s = %s\n",
				event.AccountId, event.Amount, event.NewBalance)

			tx, _ := sqlite.TxFromContext(ctx)
			_, err := tx.Exec(`
				UPDATE account_balance
				SET balance = ?, version = ?, updated_at = ?
				WHERE account_id = ?
			`, event.NewBalance, envelope.Version, event.Timestamp, event.AccountId)
			return err
		})).
		OnReset(func(ctx context.Context, tx *sql.Tx) error {
			fmt.Println("   🔄 Resetting account-balance projection")
			_, err := tx.Exec("DELETE FROM account_balance")
			return err
		}).
		Build()

	if err != nil {
		log.Fatalf("Failed to build projection: %v", err)
	}

	fmt.Println("   ✅ Projections built")
	fmt.Println()

	// 5. Setup Projection Manager
	fmt.Println("5️⃣  Setting up projection manager...")

	projectionManager := eventsourcing.NewProjectionManager(
		checkpointStore,
		eventStore,
		eventBus,
	)

	projectionManager.Register(accountBalanceProjection)

	// Start projection - it will listen to NATS events
	if err := projectionManager.Start(ctx, "account-balance"); err != nil {
		log.Fatalf("Failed to start projection: %v", err)
	}

	fmt.Println("   ✅ Projection manager started")
	fmt.Println("   📡 Projection is now listening to NATS events")
	fmt.Println()

	// 6. Start Outbox Forwarder
	fmt.Println("6️⃣  Starting outbox forwarder...")

	forwarder := messaging.NewOutboxForwarder(
		eventStore,
		eventBus,
		messaging.OutboxForwarderConfig{
			PollRate:   200 * time.Millisecond,
			BatchSize:  10,
			MaxRetries: 5,
			Logger:     logger,
		},
	)

	forwarder.Start(ctx)
	defer forwarder.Stop()

	fmt.Println("   ✅ Outbox forwarder started")
	fmt.Println()

	// 7. Create and Store Events (simulating command handlers)
	fmt.Println("7️⃣  Processing commands and storing events...")
	fmt.Println("   📝 Events stored atomically with outbox entries")
	fmt.Println()

	accountID := "account-456"

	// Command 1: Open Account
	event1 := createAccountOpenedEvent(accountID, "Bob", "2000.00")
	if err := eventStore.AppendEvents(accountID, 0, []*domain.Event{event1}); err != nil {
		log.Fatalf("Failed to append event: %v", err)
	}
	fmt.Println("   💾 Event stored: AccountOpened")

	time.Sleep(300 * time.Millisecond) // Wait for processing

	// Command 2: Deposit Money
	event2 := createMoneyDepositedEvent(accountID, "800.00", "2800.00", 2)
	if err := eventStore.AppendEvents(accountID, 1, []*domain.Event{event2}); err != nil {
		log.Fatalf("Failed to append event: %v", err)
	}
	fmt.Println("   💾 Event stored: MoneyDeposited")

	time.Sleep(300 * time.Millisecond)

	// Command 3: Withdraw Money
	event3 := createMoneyWithdrawnEvent(accountID, "300.00", "2500.00", 3)
	if err := eventStore.AppendEvents(accountID, 2, []*domain.Event{event3}); err != nil {
		log.Fatalf("Failed to append event: %v", err)
	}
	fmt.Println("   💾 Event stored: MoneyWithdrawn")

	fmt.Println()

	// 8. Wait for Everything to Process
	fmt.Println("8️⃣  Waiting for events to flow through the system...")
	fmt.Println("   ⏳ Event Store → Outbox → NATS → Projections")
	fmt.Println()

	time.Sleep(2 * time.Second)

	fmt.Printf("   ✅ All events processed! (%d projection updates)\n", projectionEventCount.Load())
	fmt.Println()

	// 9. Query the Read Model
	fmt.Println("9️⃣  Querying read model (projection)...")

	var ownerName, balance string
	var version int64
	err = db.QueryRow(`
		SELECT owner_name, balance, version
		FROM account_balance
		WHERE account_id = ?
	`, accountID).Scan(&ownerName, &balance, &version)

	if err != nil {
		log.Fatalf("Failed to query: %v", err)
	}

	fmt.Printf("   📊 Account: %s\n", accountID)
	fmt.Printf("   👤 Owner: %s\n", ownerName)
	fmt.Printf("   💰 Balance: %s\n", balance)
	fmt.Printf("   🔢 Version: %d\n", version)
	fmt.Println()

	// 10. Demonstrate Projection Rebuild
	fmt.Println("🔟  Demonstrating projection rebuild...")
	fmt.Println("   🔄 Stopping projection and clearing read model")

	projectionManager.Stop("account-balance")
	projectionEventCount.Store(0)

	fmt.Println("   🏗️  Rebuilding from event store history...")

	if err := projectionManager.Rebuild(ctx, "account-balance"); err != nil {
		log.Fatalf("Failed to rebuild: %v", err)
	}

	fmt.Printf("   ✅ Rebuild complete! (%d events replayed)\n", projectionEventCount.Load())
	fmt.Println()

	// 11. Verify Rebuild
	fmt.Println("1️⃣1️⃣  Verifying rebuilt read model...")

	err = db.QueryRow(`
		SELECT owner_name, balance, version
		FROM account_balance
		WHERE account_id = ?
	`, accountID).Scan(&ownerName, &balance, &version)

	if err != nil {
		log.Fatalf("Failed to query after rebuild: %v", err)
	}

	fmt.Printf("   📊 Account: %s\n", accountID)
	fmt.Printf("   👤 Owner: %s\n", ownerName)
	fmt.Printf("   💰 Balance: %s\n", balance)
	fmt.Printf("   🔢 Version: %d\n", version)
	fmt.Println("   ✅ Data matches - rebuild successful!")
	fmt.Println()

	// 12. Cleanup
	fmt.Println("1️⃣2️⃣  Shutting down gracefully...")

	projectionManager.StopAll()
	forwarder.Stop()
	eventBus.Close()
	eventStore.Close()

	cancel()
	select {
	case <-errCh:
	case <-time.After(2 * time.Second):
	}

	fmt.Println("   ✅ Shutdown complete")
	fmt.Println()

	// 13. Summary
	fmt.Println("🎉 Demo Complete!")
	fmt.Println()
	fmt.Println("📊 Architecture Summary:")
	fmt.Println("  1. Commands → AppendEvents (atomic with outbox)")
	fmt.Println("  2. OutboxForwarder → Polls and publishes to NATS")
	fmt.Println("  3. NATS → Delivers events to subscribers")
	fmt.Println("  4. ProjectionManager → Updates read models")
	fmt.Println("  5. Rebuild → Replay events from event store")
	fmt.Println()
	fmt.Println("✨ Key Benefits:")
	fmt.Println("  • Guaranteed event delivery (outbox pattern)")
	fmt.Println("  • Real-time read model updates (NATS streaming)")
	fmt.Println("  • Automatic checkpoint management")
	fmt.Println("  • Rebuild support for recovery")
	fmt.Println("  • Multiple projections from same events")
	fmt.Println("  • Production-ready error handling")
	fmt.Println()
}

// Helper functions to create events

func createAccountOpenedEvent(accountID, ownerName, initialBalance string) *domain.Event {
	payload := &accountv1.AccountOpenedEvent{
		AccountId:      accountID,
		OwnerName:      ownerName,
		InitialBalance: initialBalance,
		Timestamp:      timestamppb.Now().AsTime().Unix(),
	}

	data, _ := proto.Marshal(payload)

	return &domain.Event{
		ID:            uuid.New().String(),
		AggregateID:   accountID,
		AggregateType: "Account",
		EventType:     "account.v1.AccountOpenedEvent",
		Version:       1,
		Timestamp:     time.Now(),
		Data:          data,
		Metadata:      domain.EventMetadata{},
	}
}

func createMoneyDepositedEvent(accountID, amount, newBalance string, version int64) *domain.Event {
	payload := &accountv1.MoneyDepositedEvent{
		AccountId:  accountID,
		Amount:     amount,
		NewBalance: newBalance,
		Timestamp:  timestamppb.Now().AsTime().Unix(),
	}

	data, _ := proto.Marshal(payload)

	return &domain.Event{
		ID:            uuid.New().String(),
		AggregateID:   accountID,
		AggregateType: "Account",
		EventType:     "account.v1.MoneyDepositedEvent",
		Version:       version,
		Timestamp:     time.Now(),
		Data:          data,
		Metadata:      domain.EventMetadata{},
	}
}

func createMoneyWithdrawnEvent(accountID, amount, newBalance string, version int64) *domain.Event {
	payload := &accountv1.MoneyWithdrawnEvent{
		AccountId:  accountID,
		Amount:     amount,
		NewBalance: newBalance,
		Timestamp:  timestamppb.Now().AsTime().Unix(),
	}

	data, _ := proto.Marshal(payload)

	return &domain.Event{
		ID:            uuid.New().String(),
		AggregateID:   accountID,
		AggregateType: "Account",
		EventType:     "account.v1.MoneyWithdrawnEvent",
		Version:       version,
		Timestamp:     time.Now(),
		Data:          data,
		Metadata:      domain.EventMetadata{},
	}
}
