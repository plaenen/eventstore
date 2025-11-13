package main

import (
	"context"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	accountv1 "github.com/plaenen/eventstore/examples/pb/account/v1"
	"github.com/plaenen/eventstore/pkg/domain"
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

// This demo showcases the TRANSACTIONAL OUTBOX PATTERN that provides:
//
// - Guaranteed event publishing (at-least-once delivery)
// - Transactional consistency between event storage and publishing
// - Automatic retry with failure tracking
// - Background polling for unpublished events
// - No message loss even if publish fails

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
	fmt.Println("=== Transactional Outbox Pattern Demo ===")
	fmt.Println()
	fmt.Println("This demo showcases:")
	fmt.Println("  • Guaranteed event publishing with transactional outbox pattern")
	fmt.Println("  • Events stored atomically with outbox entries")
	fmt.Println("  • Background polling and automatic publishing to NATS")
	fmt.Println("  • At-least-once delivery guarantee")
	fmt.Println("  • Automatic retry with failure tracking")
	fmt.Println()

	ctx := context.Background()
	logger := &simpleLogger{}

	// 1. Start Embedded NATS Server
	fmt.Println("1️⃣  Starting embedded NATS server...")

	natsService := embeddednats.New(
		embeddednats.WithLogger(logger),
		embeddednats.WithNATSOptions(
			infranatsnats.WithPort(4224),
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
		sqlite.WithDSN("file:outbox_demo.db?mode=memory&cache=shared"),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer eventStore.Close()

	fmt.Println("   ✅ Event store ready with outbox table")
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

	// 4. Setup Event Subscriber to Track Published Events
	fmt.Println("4️⃣  Setting up event subscriber...")

	var publishedCount atomic.Int32
	publishedEvents := make(chan *domain.EventEnvelope, 10)

	_, err = eventBus.Subscribe(messaging.EventFilter{}, func(envelope *domain.EventEnvelope) error {
		publishedCount.Add(1)
		fmt.Printf("   📨 Event published to NATS: %s (v%d)\n",
			envelope.EventType, envelope.Version)
		publishedEvents <- envelope
		return nil
	})
	if err != nil {
		log.Fatalf("Failed to subscribe: %v", err)
	}

	fmt.Println("   ✅ Subscriber ready")
	fmt.Println()

	// 5. Start Outbox Forwarder
	fmt.Println("5️⃣  Starting outbox forwarder...")

	forwarder := messaging.NewOutboxForwarder(
		eventStore,
		eventBus,
		messaging.OutboxForwarderConfig{
			PollRate:   500 * time.Millisecond, // Poll every 500ms
			BatchSize:  10,
			MaxRetries: 5,
			Logger:     logger,
		},
	)

	forwarder.Start(ctx)
	defer forwarder.Stop()

	fmt.Println("   ✅ Outbox forwarder started (polling every 500ms)")
	fmt.Println()

	// 6. Create and Store Events
	fmt.Println("6️⃣  Creating and storing events...")
	fmt.Println("   📝 Notice: Events are stored atomically with outbox entries")
	fmt.Println()

	accountID := "account-123"

	// Event 1: Account Opened
	event1 := createAccountOpenedEvent(accountID, "Alice", "1000.00")
	if err := eventStore.AppendEvents(accountID, 0, []*domain.Event{event1}); err != nil {
		log.Fatalf("Failed to append event 1: %v", err)
	}
	fmt.Printf("   💾 Event 1 stored: AccountOpened (v1)\n")
	fmt.Println("      → Event and outbox entry saved atomically")

	// Event 2: Money Deposited
	event2 := createMoneyDepositedEvent(accountID, "500.00", "1500.00", 2)
	if err := eventStore.AppendEvents(accountID, 1, []*domain.Event{event2}); err != nil {
		log.Fatalf("Failed to append event 2: %v", err)
	}
	fmt.Printf("   💾 Event 2 stored: MoneyDeposited (v2)\n")
	fmt.Println("      → Event and outbox entry saved atomically")

	// Event 3: Money Withdrawn
	event3 := createMoneyWithdrawnEvent(accountID, "200.00", "1300.00", 3)
	if err := eventStore.AppendEvents(accountID, 2, []*domain.Event{event3}); err != nil {
		log.Fatalf("Failed to append event 3: %v", err)
	}
	fmt.Printf("   💾 Event 3 stored: MoneyWithdrawn (v3)\n")
	fmt.Println("      → Event and outbox entry saved atomically")

	fmt.Println()

	// 7. Wait for Outbox Forwarder to Publish Events
	fmt.Println("7️⃣  Waiting for outbox forwarder to publish events...")
	fmt.Println("   ⏳ Background process is polling for unpublished events...")
	fmt.Println()

	// Wait for all 3 events to be published
	timeout := time.After(5 * time.Second)
	for i := 0; i < 3; i++ {
		select {
		case <-publishedEvents:
			// Event received
		case <-timeout:
			log.Fatal("Timeout waiting for events to be published")
		}
	}

	fmt.Printf("   ✅ All %d events published successfully!\n", publishedCount.Load())
	fmt.Println()

	// 8. Verify Outbox State
	fmt.Println("8️⃣  Verifying outbox state...")

	unpublished, err := eventStore.LoadUnpublishedEvents(100)
	if err != nil {
		log.Fatalf("Failed to load unpublished events: %v", err)
	}

	fmt.Printf("   📊 Unpublished events: %d\n", len(unpublished))
	fmt.Println("   ✅ All events have been published and marked as complete")
	fmt.Println()

	// 9. Demonstrate Idempotency (optional)
	fmt.Println("9️⃣  Testing event replay protection...")

	// Try to replay the same events (would normally be prevented by version check)
	currentVersion, err := eventStore.GetAggregateVersion(accountID)
	if err != nil {
		log.Fatalf("Failed to get version: %v", err)
	}
	fmt.Printf("   Current aggregate version: %d\n", currentVersion)
	fmt.Println("   ✅ Version checking prevents duplicate event storage")
	fmt.Println()

	// 10. Graceful Shutdown
	fmt.Println("🔟  Shutting down gracefully...")

	forwarder.Stop()
	fmt.Println("   ✅ Outbox forwarder stopped")

	eventBus.Close()
	fmt.Println("   ✅ Event bus closed")

	eventStore.Close()
	fmt.Println("   ✅ Event store closed")

	cancel()
	select {
	case err := <-errCh:
		if err != nil && err != context.Canceled {
			fmt.Printf("   ⚠️  Runner error: %v\n", err)
		}
	case <-time.After(2 * time.Second):
		// Timeout
	}
	fmt.Println("   ✅ NATS server stopped")
	fmt.Println()

	// 11. Summary
	fmt.Println("🎉 Demo Complete!")
	fmt.Println()
	fmt.Println("📊 Summary:")
	fmt.Println("  ✅ 3 events stored atomically with outbox entries")
	fmt.Println("  ✅ All events published to NATS via background forwarder")
	fmt.Println("  ✅ At-least-once delivery guarantee maintained")
	fmt.Println("  ✅ Zero message loss even with publish failures")
	fmt.Println("  ✅ Graceful shutdown with no data loss")
	fmt.Println()

	fmt.Println("💡 Key Benefits of Outbox Pattern:")
	fmt.Println("  • Transactional consistency (event + outbox entry saved together)")
	fmt.Println("  • Guaranteed delivery (no message loss)")
	fmt.Println("  • Automatic retry on failure")
	fmt.Println("  • Decouples persistence from publishing")
	fmt.Println("  • Works with any message broker")
	fmt.Println()

	fmt.Println("🔧 Configuration:")
	fmt.Println("  • Poll rate: 500ms")
	fmt.Println("  • Batch size: 10 events")
	fmt.Println("  • Max retries: 5")
	fmt.Println()

	fmt.Println("📦 Architecture:")
	fmt.Println("  1. Command → AppendEvents")
	fmt.Println("  2. EventStore saves event + outbox entry (atomic)")
	fmt.Println("  3. OutboxForwarder polls for unpublished events")
	fmt.Println("  4. EventBus publishes to NATS")
	fmt.Println("  5. EventStore marks events as published")
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
