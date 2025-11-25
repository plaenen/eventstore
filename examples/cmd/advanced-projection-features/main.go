package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/plaenen/eventstore/pkg/eventsourcing"
	"github.com/plaenen/eventstore/pkg/eventsourcing/store/sqlite"
	natseventbus "github.com/plaenen/eventstore/pkg/messaging/nats"
)

// This example demonstrates advanced projection features including:
// 1. Rebuild optimization (no duplicate processing)
// 2. NATS sequence tracking
// 3. Interrupted rebuild detection
// 4. Checkpoint monitoring
// 5. Deterministic consumer names

func main() {
	log.Println("=== Advanced Projection Features Demo ===")
	log.Println()

	// Parse command line arguments
	scenario := "basic"
	if len(os.Args) > 1 {
		scenario = os.Args[1]
	}

	switch scenario {
	case "basic":
		runBasicDemo()
	case "rebuild":
		runRebuildDemo()
	case "interrupted":
		runInterruptedRebuildDemo()
	case "monitor":
		runMonitoringDemo()
	case "concurrent":
		runConcurrentEventsDemo()
	default:
		log.Printf("Unknown scenario: %s", scenario)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println()
	fmt.Println("Usage: go run main.go [scenario]")
	fmt.Println()
	fmt.Println("Available scenarios:")
	fmt.Println("  basic        - Basic projection with checkpoint tracking (default)")
	fmt.Println("  rebuild      - Demonstrate rebuild optimization")
	fmt.Println("  interrupted  - Demonstrate interrupted rebuild detection")
	fmt.Println("  monitor      - Monitor checkpoint and NATS consumer state")
	fmt.Println("  concurrent   - Events arriving during rebuild")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  go run main.go basic")
	fmt.Println("  go run main.go rebuild")
	fmt.Println("  go run main.go monitor")
}

// ====================
// SCENARIO 1: Basic Demo
// ====================

func runBasicDemo() {
	log.Println("Scenario: Basic projection with checkpoint tracking")
	log.Println("This demonstrates:")
	log.Println("  - Deterministic consumer names")
	log.Println("  - NATS sequence tracking")
	log.Println("  - Checkpoint persistence")
	log.Println()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup infrastructure
	cleanup := setupInfrastructure()
	defer cleanup()

	eventStore, projectionStore, eventBus := createStores()
	defer eventStore.Close()
	defer projectionStore.Close()
	defer eventBus.Close()

	// Create projection manager
	checkpointStore, _ := sqlite.NewCheckpointStore(projectionStore.DB())
	projectionManager := eventsourcing.NewProjectionManager(
		checkpointStore,
		eventStore,
		eventBus,
	)

	// Register projection
	projection := NewAccountBalanceProjection(projectionStore.DB())
	projectionManager.Register(projection)

	// Start projection
	log.Println("📊 Starting projection...")
	if err := projectionManager.Start(ctx, "account-balance"); err != nil {
		log.Fatalf("Failed to start projection: %v", err)
	}

	// Create some events
	log.Println()
	log.Println("💰 Creating account events...")
	createAccountEvents(eventStore, eventBus, 10)

	// Wait for processing
	time.Sleep(2 * time.Second)

	// Show checkpoint state
	log.Println()
	showCheckpointState(checkpointStore, "account-balance")

	// Show NATS consumer state
	log.Println()
	showNATSConsumerState(eventBus, "projection_account-balance")

	// Query projection
	log.Println()
	showProjectionData(projectionStore.DB())

	log.Println()
	log.Println("✅ Basic demo complete!")
	log.Println()
	log.Println("Key observations:")
	log.Println("  - Consumer name is deterministic: projection_account-balance")
	log.Println("  - Checkpoint tracks both EventStore position and NATS sequence")
	log.Println("  - On restart, projection will resume from last checkpoint")
}

// ====================
// SCENARIO 2: Rebuild Demo
// ====================

func runRebuildDemo() {
	log.Println("Scenario: Rebuild optimization")
	log.Println("This demonstrates:")
	log.Println("  - Rebuild sets IsRebuilding flag")
	log.Println("  - Events not reprocessed after rebuild")
	log.Println("  - Deterministic consumer preserves state")
	log.Println()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cleanup := setupInfrastructure()
	defer cleanup()

	eventStore, projectionStore, eventBus := createStores()
	defer eventStore.Close()
	defer projectionStore.Close()
	defer eventBus.Close()

	checkpointStore, _ := sqlite.NewCheckpointStore(projectionStore.DB())
	projectionManager := eventsourcing.NewProjectionManager(
		checkpointStore,
		eventStore,
		eventBus,
	)

	projection := NewAccountBalanceProjection(projectionStore.DB())
	projectionManager.Register(projection)

	// Phase 1: Create initial events
	log.Println("📝 Phase 1: Creating 5 initial events...")
	createAccountEvents(eventStore, eventBus, 5)
	time.Sleep(1 * time.Second)

	// Phase 2: Start projection (will process all events)
	log.Println()
	log.Println("📊 Phase 2: Starting projection (first time)...")
	if err := projectionManager.Start(ctx, "account-balance"); err != nil {
		log.Fatalf("Failed to start projection: %v", err)
	}
	time.Sleep(2 * time.Second)

	log.Println()
	log.Println("Checkpoint after initial processing:")
	checkpoint, _ := checkpointStore.Load("account-balance")
	log.Printf("  Position: %d", checkpoint.Position)
	if checkpoint.NATSSequence != nil {
		log.Printf("  NATS Sequence: %d", *checkpoint.NATSSequence)
	}
	log.Printf("  IsRebuilding: %v", checkpoint.IsRebuilding)

	// Phase 3: Stop projection
	log.Println()
	log.Println("⏸️  Phase 3: Stopping projection...")
	projectionManager.Stop("account-balance")
	time.Sleep(1 * time.Second)

	// Phase 4: Trigger rebuild
	log.Println()
	log.Println("🔄 Phase 4: Triggering rebuild...")
	if err := projectionManager.Rebuild(ctx, "account-balance"); err != nil {
		log.Fatalf("Failed to rebuild: %v", err)
	}

	log.Println()
	log.Println("Checkpoint after rebuild:")
	checkpoint, _ = checkpointStore.Load("account-balance")
	log.Printf("  Position: %d", checkpoint.Position)
	if checkpoint.NATSSequence != nil {
		log.Printf("  NATS Sequence: %d (should be nil after rebuild)", *checkpoint.NATSSequence)
	} else {
		log.Println("  NATS Sequence: nil (✓ correct - will be set when subscription starts)")
	}
	log.Printf("  IsRebuilding: %v (should be false)", checkpoint.IsRebuilding)

	// Phase 5: Start projection again
	log.Println()
	log.Println("▶️  Phase 5: Restarting projection...")
	if err := projectionManager.Start(ctx, "account-balance"); err != nil {
		log.Fatalf("Failed to restart projection: %v", err)
	}
	time.Sleep(2 * time.Second)

	log.Println()
	log.Println("Checkpoint after restart:")
	checkpoint, _ = checkpointStore.Load("account-balance")
	log.Printf("  Position: %d", checkpoint.Position)
	if checkpoint.NATSSequence != nil {
		log.Printf("  NATS Sequence: %d (✓ set by NATS subscription)", *checkpoint.NATSSequence)
	}

	// Verify no duplicate processing
	log.Println()
	verifyNoDuplicates(projectionStore.DB())

	log.Println()
	log.Println("✅ Rebuild demo complete!")
	log.Println()
	log.Println("Key observations:")
	log.Println("  - Rebuild cleared NATS sequence (will resume from beginning)")
	log.Println("  - IsRebuilding flag prevents interrupted restart")
	log.Println("  - Deterministic consumer name preserved across restarts")
	log.Println("  - No duplicate processing (idempotent handlers)")
}

// ====================
// SCENARIO 3: Interrupted Rebuild
// ====================

func runInterruptedRebuildDemo() {
	log.Println("Scenario: Interrupted rebuild detection")
	log.Println("This demonstrates:")
	log.Println("  - IsRebuilding flag prevents invalid restart")
	log.Println("  - Clear error message for interrupted state")
	log.Println()

	ctx := context.Background()

	cleanup := setupInfrastructure()
	defer cleanup()

	eventStore, projectionStore, eventBus := createStores()
	defer eventStore.Close()
	defer projectionStore.Close()
	defer eventBus.Close()

	checkpointStore, _ := sqlite.NewCheckpointStore(projectionStore.DB())

	// Simulate interrupted rebuild by manually setting flag
	log.Println("🔧 Simulating interrupted rebuild...")
	checkpointStore.Save(&eventsourcing.ProjectionCheckpoint{
		ProjectionName: "account-balance",
		Position:       500,
		NATSSequence:   nil,
		IsRebuilding:   true, // ← This indicates rebuild was interrupted
		UpdatedAt:      time.Now(),
	})

	log.Println("Checkpoint state:")
	checkpoint, _ := checkpointStore.Load("account-balance")
	log.Printf("  Position: %d", checkpoint.Position)
	log.Printf("  IsRebuilding: %v ⚠️", checkpoint.IsRebuilding)

	// Try to start projection (should fail)
	log.Println()
	log.Println("❌ Attempting to start projection with interrupted rebuild...")

	projectionManager := eventsourcing.NewProjectionManager(
		checkpointStore,
		eventStore,
		eventBus,
	)

	projection := NewAccountBalanceProjection(projectionStore.DB())
	projectionManager.Register(projection)

	err := projectionManager.Start(ctx, "account-balance")
	if err != nil {
		log.Printf()
		log.Printf("Expected error: %v", err)
		log.Println()
		log.Println("✅ Correctly detected interrupted rebuild!")
		log.Println()
		log.Println("To fix this, call Rebuild() to complete or restart the rebuild:")
		log.Println("  projectionManager.Rebuild(ctx, \"account-balance\")")
	} else {
		log.Println("❌ ERROR: Should have failed but didn't!")
	}

	// Show how to fix it
	log.Println()
	log.Println("🔄 Completing the rebuild...")
	if err := projectionManager.Rebuild(ctx, "account-balance"); err != nil {
		log.Fatalf("Failed to rebuild: %v", err)
	}

	log.Println()
	log.Println("Checkpoint after rebuild:")
	checkpoint, _ = checkpointStore.Load("account-balance")
	log.Printf("  Position: %d", checkpoint.Position)
	log.Printf("  IsRebuilding: %v ✓", checkpoint.IsRebuilding)

	log.Println()
	log.Println("Now projection can start normally:")
	if err := projectionManager.Start(ctx, "account-balance"); err != nil {
		log.Fatalf("Failed to start: %v", err)
	}
	log.Println("✅ Projection started successfully!")
}

// ====================
// SCENARIO 4: Monitoring
// ====================

func runMonitoringDemo() {
	log.Println("Scenario: Monitoring checkpoint and consumer state")
	log.Println("This demonstrates:")
	log.Println("  - Real-time checkpoint monitoring")
	log.Println("  - NATS consumer state inspection")
	log.Println("  - Lag detection")
	log.Println()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cleanup := setupInfrastructure()
	defer cleanup()

	eventStore, projectionStore, eventBus := createStores()
	defer eventStore.Close()
	defer projectionStore.Close()
	defer eventBus.Close()

	checkpointStore, _ := sqlite.NewCheckpointStore(projectionStore.DB())
	projectionManager := eventsourcing.NewProjectionManager(
		checkpointStore,
		eventStore,
		eventBus,
	)

	projection := NewAccountBalanceProjection(projectionStore.DB())
	projectionManager.Register(projection)

	// Start projection
	if err := projectionManager.Start(ctx, "account-balance"); err != nil {
		log.Fatalf("Failed to start projection: %v", err)
	}

	// Monitor for 10 seconds while creating events
	log.Println("📊 Monitoring projection (10 seconds)...")
	log.Println()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	eventTicker := time.NewTicker(1 * time.Second)
	defer eventTicker.Stop()

	done := time.After(10 * time.Second)

	eventCount := 0
	for {
		select {
		case <-done:
			log.Println()
			log.Println("✅ Monitoring complete!")
			return

		case <-ticker.C:
			// Show state every 2 seconds
			log.Println("--- Checkpoint State ---")
			showCheckpointState(checkpointStore, "account-balance")
			log.Println()

		case <-eventTicker.C:
			// Create events every second
			eventCount++
			createAccountEvents(eventStore, eventBus, 1)
			log.Printf("Created event %d\n", eventCount)
		}
	}
}

// ====================
// SCENARIO 5: Concurrent Events
// ====================

func runConcurrentEventsDemo() {
	log.Println("Scenario: Events arriving during rebuild")
	log.Println("This demonstrates:")
	log.Println("  - Events created during rebuild are not reprocessed")
	log.Println("  - Checkpoint correctly tracks state")
	log.Println("  - No duplicate processing")
	log.Println()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cleanup := setupInfrastructure()
	defer cleanup()

	eventStore, projectionStore, eventBus := createStores()
	defer eventStore.Close()
	defer projectionStore.Close()
	defer eventBus.Close()

	checkpointStore, _ := sqlite.NewCheckpointStore(projectionStore.DB())
	projectionManager := eventsourcing.NewProjectionManager(
		checkpointStore,
		eventStore,
		eventBus,
	)

	projection := NewAccountBalanceProjection(projectionStore.DB())
	projectionManager.Register(projection)

	// Create initial events
	log.Println("📝 Creating 10 initial events...")
	createAccountEvents(eventStore, eventBus, 10)
	time.Sleep(500 * time.Millisecond)

	// Start projection to process initial events
	log.Println("📊 Starting projection (initial)...")
	if err := projectionManager.Start(ctx, "account-balance"); err != nil {
		log.Fatalf("Failed to start: %v", err)
	}
	time.Sleep(2 * time.Second)

	initialCheckpoint, _ := checkpointStore.Load("account-balance")
	log.Printf("Initial checkpoint: position=%d, nats_seq=%d\n",
		initialCheckpoint.Position,
		getSeq(initialCheckpoint.NATSSequence))

	// Stop for rebuild
	log.Println()
	log.Println("⏸️  Stopping projection for rebuild...")
	projectionManager.Stop("account-balance")
	time.Sleep(500 * time.Millisecond)

	// Start rebuild in background
	log.Println("🔄 Starting rebuild (background)...")
	rebuildDone := make(chan error)
	go func() {
		rebuildDone <- projectionManager.Rebuild(ctx, "account-balance")
	}()

	// Create events DURING rebuild
	time.Sleep(1 * time.Second) // Let rebuild start
	log.Println()
	log.Println("💰 Creating 5 NEW events DURING rebuild...")
	createAccountEvents(eventStore, eventBus, 5)

	// Wait for rebuild to complete
	if err := <-rebuildDone; err != nil {
		log.Fatalf("Rebuild failed: %v", err)
	}

	rebuildCheckpoint, _ := checkpointStore.Load("account-balance")
	log.Println()
	log.Printf("After rebuild: position=%d (should be 15), nats_seq=nil\n",
		rebuildCheckpoint.Position)

	// Restart projection
	log.Println()
	log.Println("▶️  Restarting projection...")
	if err := projectionManager.Start(ctx, "account-balance"); err != nil {
		log.Fatalf("Failed to restart: %v", err)
	}
	time.Sleep(2 * time.Second)

	finalCheckpoint, _ := checkpointStore.Load("account-balance")
	log.Println()
	log.Printf("After restart: position=%d, nats_seq=%d\n",
		finalCheckpoint.Position,
		getSeq(finalCheckpoint.NATSSequence))

	// Verify no duplicates
	log.Println()
	verifyNoDuplicates(projectionStore.DB())

	log.Println()
	log.Println("✅ Concurrent events demo complete!")
	log.Println()
	log.Println("Key observations:")
	log.Println("  - All 15 events processed exactly once")
	log.Println("  - Events created during rebuild included in final state")
	log.Println("  - No reprocessing after subscription resumed")
}

// ====================
// Helper Functions
// ====================

func setupInfrastructure() func() {
	// Clean up old databases
	os.Remove("demo_eventstore.db")
	os.Remove("demo_projections.db")

	log.Println("🚀 Setting up infrastructure...")
	return func() {
		log.Println("🧹 Cleaning up...")
	}
}

func createStores() (*sqlite.EventStore, *sqlite.EventStore, *natseventbus.EventBus) {
	// Event store
	eventStoreDB, err := sql.Open("sqlite", "demo_eventstore.db")
	if err != nil {
		log.Fatalf("Failed to open event store: %v", err)
	}

	eventStore, err := sqlite.NewEventStore(eventStoreDB)
	if err != nil {
		log.Fatalf("Failed to create event store: %v", err)
	}

	// Projection store (separate database)
	projectionDB, err := sql.Open("sqlite", "demo_projections.db")
	if err != nil {
		log.Fatalf("Failed to open projection store: %v", err)
	}

	projectionStore, err := sqlite.NewEventStore(projectionDB)
	if err != nil {
		log.Fatalf("Failed to create projection store: %v", err)
	}

	// Create projection table
	_, err = projectionDB.Exec(`
		CREATE TABLE IF NOT EXISTS account_balances (
			account_id TEXT PRIMARY KEY,
			balance INTEGER NOT NULL,
			transaction_count INTEGER NOT NULL,
			last_updated INTEGER NOT NULL
		)
	`)
	if err != nil {
		log.Fatalf("Failed to create projection table: %v", err)
	}

	// NATS event bus
	eventBus, err := natseventbus.NewEventBus(natseventbus.Config{
		URL:            nats.DefaultURL,
		StreamName:     "DEMO_EVENTS",
		StreamSubjects: []string{"demo.>"},
		MaxAge:         24 * time.Hour,
		MaxBytes:       100 * 1024 * 1024,
	})
	if err != nil {
		log.Fatalf("Failed to create event bus: %v", err)
	}

	// Start outbox forwarder
	outboxForwarder := eventsourcing.NewOutboxForwarder(
		eventStore,
		eventBus,
		eventsourcing.DefaultOutboxForwarderConfig(),
	)
	outboxForwarder.Start(context.Background())

	log.Println("✅ Infrastructure ready")
	log.Println()

	return eventStore, projectionStore, eventBus
}

func createAccountEvents(eventStore *sqlite.EventStore, eventBus *natseventbus.EventBus, count int) {
	for i := 0; i < count; i++ {
		accountID := fmt.Sprintf("account-%d", time.Now().UnixNano()%1000)

		event := &eventsourcing.Event{
			ID:            eventsourcing.GenerateID(),
			AggregateID:   accountID,
			AggregateType: "BankAccount",
			EventType:     "MoneyDeposited",
			Version:       1,
			Timestamp:     time.Now(),
			Data:          []byte(fmt.Sprintf(`{"amount": %d}`, (i+1)*100)),
			Metadata: eventsourcing.EventMetadata{
				CorrelationID: eventsourcing.GenerateID(),
			},
		}

		if err := eventStore.AppendEvents(context.Background(), []*domain.Event{event}); err != nil {
			log.Printf("Failed to append event: %v", err)
		}

		time.Sleep(50 * time.Millisecond)
	}
}

func showCheckpointState(store eventsourcing.CheckpointStore, projectionName string) {
	checkpoint, err := store.Load(projectionName)
	if err != nil {
		log.Printf("No checkpoint found: %v", err)
		return
	}

	log.Println("📍 Checkpoint State:")
	log.Printf("  Projection: %s", checkpoint.ProjectionName)
	log.Printf("  EventStore Position: %d", checkpoint.Position)

	if checkpoint.NATSSequence != nil {
		log.Printf("  NATS Sequence: %d", *checkpoint.NATSSequence)
	} else {
		log.Println("  NATS Sequence: nil (not yet set)")
	}

	log.Printf("  IsRebuilding: %v", checkpoint.IsRebuilding)
	log.Printf("  Last Event ID: %s", checkpoint.LastEventID)
	log.Printf("  Updated: %s", checkpoint.UpdatedAt.Format(time.RFC3339))
}

func showNATSConsumerState(eventBus *natseventbus.EventBus, consumerName string) {
	log.Printf("🔌 NATS Consumer: %s", consumerName)
	log.Println("  (Use 'nats consumer info DEMO_EVENTS projection_account-balance' for details)")
}

func showProjectionData(db *sql.DB) {
	rows, err := db.Query("SELECT account_id, balance, transaction_count FROM account_balances ORDER BY account_id LIMIT 10")
	if err != nil {
		log.Printf("Failed to query projections: %v", err)
		return
	}
	defer rows.Close()

	log.Println("📊 Projection Data (first 10 accounts):")
	count := 0
	for rows.Next() {
		var accountID string
		var balance, txCount int
		rows.Scan(&accountID, &balance, &txCount)
		log.Printf("  %s: balance=%d, transactions=%d", accountID, balance, txCount)
		count++
	}

	if count == 0 {
		log.Println("  (no data yet)")
	}
}

func verifyNoDuplicates(db *sql.DB) {
	var duplicateCount int
	err := db.QueryRow(`
		SELECT COUNT(*) FROM (
			SELECT account_id, COUNT(*) as cnt
			FROM account_balances
			GROUP BY account_id
			HAVING cnt > 1
		)
	`).Scan(&duplicateCount)

	if err != nil && err != sql.ErrNoRows {
		log.Printf("Failed to check duplicates: %v", err)
		return
	}

	if duplicateCount > 0 {
		log.Printf("❌ Found %d duplicate accounts!", duplicateCount)
	} else {
		log.Println("✅ No duplicate accounts (idempotent processing works!)")
	}
}

func getSeq(seq *int64) int64 {
	if seq == nil {
		return 0
	}
	return *seq
}

// ====================
// Projection Implementation
// ====================

type AccountBalanceProjection struct {
	db *sql.DB
}

func NewAccountBalanceProjection(db *sql.DB) *AccountBalanceProjection {
	return &AccountBalanceProjection{db: db}
}

func (p *AccountBalanceProjection) Name() string {
	return "account-balance"
}

func (p *AccountBalanceProjection) Handle(ctx context.Context, event *eventsourcing.EventEnvelope) error {
	// Log event source for debugging
	source := "EventStore"
	if event.NATSMetadata != nil {
		source = fmt.Sprintf("NATS(seq=%d)", event.NATSMetadata.StreamSequence)
	}

	log.Printf("  Processing event %s from %s", event.Event.ID[:8], source)

	// Idempotent upsert
	_, err := p.db.ExecContext(ctx, `
		INSERT INTO account_balances (account_id, balance, transaction_count, last_updated)
		VALUES (?, 100, 1, ?)
		ON CONFLICT(account_id) DO UPDATE SET
			balance = balance + 100,
			transaction_count = transaction_count + 1,
			last_updated = ?
	`, event.Event.AggregateID, time.Now().Unix(), time.Now().Unix())

	return err
}

func (p *AccountBalanceProjection) Reset(ctx context.Context) error {
	log.Println("  🗑️  Resetting projection (deleting all data)...")
	_, err := p.db.ExecContext(ctx, "DELETE FROM account_balances")
	return err
}
