package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	accountv1 "github.com/plaenen/eventstore/examples/pb/account/v1"
	"github.com/plaenen/eventstore/pkg/eventsourcing"
	natspkg "github.com/plaenen/eventstore/pkg/nats"
	"github.com/plaenen/eventstore/pkg/sqlite"
	_ "modernc.org/sqlite"
)

// This demo shows how to register projections with the ProjectionManager
// to listen to NATS events in real-time.
//
// The ProjectionManager provides:
// - Real-time event processing via NATS EventBus
// - Automatic checkpoint management
// - Rebuild support from EventStore
// - Multiple projection coordination

func main() {
	fmt.Println("=== Projection with NATS Event Bus Demo ===")
	fmt.Println()
	fmt.Println("This demo shows how projections listen to real-time events")
	fmt.Println("from NATS and automatically update read models.")
	fmt.Println()

	ctx := context.Background()

	// 1. Setup infrastructure
	fmt.Println("1️⃣  Setting up infrastructure...")

	// Event store
	eventStore, err := sqlite.NewEventStore(
		sqlite.WithDSN("file:projection_nats_demo.db?mode=memory&cache=shared"),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer eventStore.Close()

	db := eventStore.DB()

	// Checkpoint store
	checkpointStore, err := sqlite.NewCheckpointStore(db)
	if err != nil {
		log.Fatal(err)
	}

	// NATS server (embedded)
	natsServer, err := natspkg.NewServer(&natspkg.ServerConfig{
		Port: -1, // Random port
	})
	if err != nil {
		log.Fatal(err)
	}
	defer natsServer.Shutdown()

	// NATS EventBus
	eventBus, err := natspkg.NewEventBus(natspkg.EventBusConfig{
		URL:    natsServer.ClientURL(),
		Stream: "events",
	})
	if err != nil {
		log.Fatal(err)
	}
	defer eventBus.Close()

	fmt.Println("   ✅ Infrastructure ready")
	fmt.Println()

	// 2. Build projections using the high-level SQLite builder
	fmt.Println("2️⃣  Building projections...")

	// Projection 1: Account balance (simple projection)
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
					balance TEXT NOT NULL,
					updated_at INTEGER NOT NULL
				)
			`)
			return err
		}).
		On(accountv1.OnAccountOpened(func(ctx context.Context, event *accountv1.AccountOpenedEvent, envelope *eventsourcing.EventEnvelope) error {
			tx, _ := sqlite.TxFromContext(ctx)
			_, err := tx.Exec(`
				INSERT INTO account_balance (account_id, balance, updated_at)
				VALUES (?, ?, ?)
			`, event.AccountId, event.InitialBalance, event.Timestamp)
			return err
		})).
		On(accountv1.OnMoneyDeposited(func(ctx context.Context, event *accountv1.MoneyDepositedEvent, envelope *eventsourcing.EventEnvelope) error {
			tx, _ := sqlite.TxFromContext(ctx)
			_, err := tx.Exec(`
				UPDATE account_balance
				SET balance = ?, updated_at = ?
				WHERE account_id = ?
			`, event.NewBalance, event.Timestamp, event.AccountId)
			return err
		})).
		On(accountv1.OnMoneyWithdrawn(func(ctx context.Context, event *accountv1.MoneyWithdrawnEvent, envelope *eventsourcing.EventEnvelope) error {
			tx, _ := sqlite.TxFromContext(ctx)
			_, err := tx.Exec(`
				UPDATE account_balance
				SET balance = ?, updated_at = ?
				WHERE account_id = ?
			`, event.NewBalance, event.Timestamp, event.AccountId)
			return err
		})).
		OnReset(func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.Exec("DELETE FROM account_balance")
			return err
		}).
		Build()

	if err != nil {
		log.Fatalf("Failed to build account balance projection: %v", err)
	}

	// Projection 2: Account activity log (complex projection)
	activityLogProjection, err := sqlite.NewSQLiteProjectionBuilder(
		"account-activity-log",
		db,
		checkpointStore,
		eventStore,
	).
		WithSchema(func(ctx context.Context, db *sql.DB) error {
			_, err := db.Exec(`
				CREATE TABLE IF NOT EXISTS account_activity_log (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					account_id TEXT NOT NULL,
					activity_type TEXT NOT NULL,
					amount TEXT,
					timestamp INTEGER NOT NULL
				)
			`)
			return err
		}).
		On(accountv1.OnAccountOpened(func(ctx context.Context, event *accountv1.AccountOpenedEvent, envelope *eventsourcing.EventEnvelope) error {
			tx, _ := sqlite.TxFromContext(ctx)
			_, err := tx.Exec(`
				INSERT INTO account_activity_log (account_id, activity_type, amount, timestamp)
				VALUES (?, 'OPENED', ?, ?)
			`, event.AccountId, event.InitialBalance, event.Timestamp)
			return err
		})).
		On(accountv1.OnMoneyDeposited(func(ctx context.Context, event *accountv1.MoneyDepositedEvent, envelope *eventsourcing.EventEnvelope) error {
			tx, _ := sqlite.TxFromContext(ctx)
			_, err := tx.Exec(`
				INSERT INTO account_activity_log (account_id, activity_type, amount, timestamp)
				VALUES (?, 'DEPOSIT', ?, ?)
			`, event.AccountId, event.Amount, event.Timestamp)
			return err
		})).
		On(accountv1.OnMoneyWithdrawn(func(ctx context.Context, event *accountv1.MoneyWithdrawnEvent, envelope *eventsourcing.EventEnvelope) error {
			tx, _ := sqlite.TxFromContext(ctx)
			_, err := tx.Exec(`
				INSERT INTO account_activity_log (account_id, activity_type, amount, timestamp)
				VALUES (?, 'WITHDRAWAL', ?, ?)
			`, event.AccountId, event.Amount, event.Timestamp)
			return err
		})).
		On(accountv1.OnAccountClosed(func(ctx context.Context, event *accountv1.AccountClosedEvent, envelope *eventsourcing.EventEnvelope) error {
			tx, _ := sqlite.TxFromContext(ctx)
			_, err := tx.Exec(`
				INSERT INTO account_activity_log (account_id, activity_type, amount, timestamp)
				VALUES (?, 'CLOSED', ?, ?)
			`, event.AccountId, event.FinalBalance, event.Timestamp)
			return err
		})).
		OnReset(func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.Exec("DELETE FROM account_activity_log")
			return err
		}).
		Build()

	if err != nil {
		log.Fatalf("Failed to build activity log projection: %v", err)
	}

	fmt.Println("   ✅ Projections built")
	fmt.Println()

	// 3. Register projections with ProjectionManager
	fmt.Println("3️⃣  Registering projections with ProjectionManager...")

	// Create projection manager
	projectionManager := eventsourcing.NewProjectionManager(
		checkpointStore,
		eventStore,
		eventBus,
	)

	// Register projections
	projectionManager.Register(accountBalanceProjection)
	projectionManager.Register(activityLogProjection)

	fmt.Println("   ✅ Projections registered")
	fmt.Println()

	// 4. Start projections (listens to NATS events)
	fmt.Println("4️⃣  Starting projections...")
	fmt.Println("   📡 Projections are now listening to NATS events")
	fmt.Println()

	// Start both projections
	if err := projectionManager.Start(ctx, "account-balance"); err != nil {
		log.Fatalf("Failed to start account-balance projection: %v", err)
	}

	if err := projectionManager.Start(ctx, "account-activity-log"); err != nil {
		log.Fatalf("Failed to start activity-log projection: %v", err)
	}

	fmt.Println("   ✅ Projections started")
	fmt.Println()

	// 5. Simulate events being published to NATS
	fmt.Println("5️⃣  Publishing events to NATS...")
	fmt.Println("   📝 Projections will automatically process these events")
	fmt.Println()

	// Simulate command handler publishing events
	events := []*eventsourcing.Event{
		{
			ID:            "evt-1",
			AggregateID:   "acc-carol-001",
			AggregateType: "Account",
			EventType:     accountv1.AccountOpenedEventType,
			Version:       1,
			Timestamp:     time.Now(),
			Data:          mustMarshalProto(&accountv1.AccountOpenedEvent{
				AccountId:      "acc-carol-001",
				OwnerName:      "Carol",
				InitialBalance: "10000.00",
				Timestamp:      time.Now().Unix(),
			}),
		},
		{
			ID:            "evt-2",
			AggregateID:   "acc-carol-001",
			AggregateType: "Account",
			EventType:     accountv1.MoneyDepositedEventType,
			Version:       2,
			Timestamp:     time.Now(),
			Data:          mustMarshalProto(&accountv1.MoneyDepositedEvent{
				AccountId:  "acc-carol-001",
				Amount:     "5000.00",
				NewBalance: "15000.00",
				Timestamp:  time.Now().Unix(),
			}),
		},
		{
			ID:            "evt-3",
			AggregateID:   "acc-carol-001",
			AggregateType: "Account",
			EventType:     accountv1.MoneyWithdrawnEventType,
			Version:       3,
			Timestamp:     time.Now(),
			Data:          mustMarshalProto(&accountv1.MoneyWithdrawnEvent{
				AccountId:  "acc-carol-001",
				Amount:     "2000.00",
				NewBalance: "13000.00",
				Timestamp:  time.Now().Unix(),
			}),
		},
	}

	// Publish events to NATS
	if err := eventBus.Publish(events); err != nil {
		log.Fatalf("Failed to publish events: %v", err)
	}

	fmt.Println("   ✅ Events published to NATS")
	fmt.Println()

	// Give projections time to process events
	fmt.Println("6️⃣  Waiting for projections to process events...")
	time.Sleep(2 * time.Second)
	fmt.Println("   ✅ Processing complete")
	fmt.Println()

	// 7. Query projections
	fmt.Println("7️⃣  Querying projections...")
	fmt.Println()

	// Query account balance projection
	var balance string
	var updatedAt int64
	err = db.QueryRow(`
		SELECT balance, updated_at
		FROM account_balance
		WHERE account_id = ?
	`, "acc-carol-001").Scan(&balance, &updatedAt)
	if err != nil {
		log.Printf("   ⚠️  Account balance not found (events may still be processing): %v", err)
	} else {
		fmt.Printf("   💰 Account Balance Projection:\n")
		fmt.Printf("      Balance: %s\n", balance)
		fmt.Println()
	}

	// Query activity log projection
	rows, err := db.Query(`
		SELECT activity_type, amount, timestamp
		FROM account_activity_log
		WHERE account_id = ?
		ORDER BY timestamp ASC
	`, "acc-carol-001")
	if err != nil {
		log.Printf("   ⚠️  Activity log query failed: %v", err)
	} else {
		defer rows.Close()

		fmt.Printf("   📊 Activity Log Projection:\n")
		for rows.Next() {
			var activityType, amount string
			var timestamp int64
			if err := rows.Scan(&activityType, &amount, &timestamp); err != nil {
				log.Printf("   ⚠️  Failed to scan row: %v", err)
				continue
			}
			fmt.Printf("      %s: %s\n", activityType, amount)
		}
		fmt.Println()
	}

	// 8. Check checkpoints
	fmt.Println("8️⃣  Checking projection checkpoints...")

	checkpoint1, err := projectionManager.GetCheckpoint("account-balance")
	if err != nil {
		log.Printf("   ⚠️  Account balance checkpoint: %v", err)
	} else {
		fmt.Printf("   account-balance: position=%d, last_event=%s\n", checkpoint1.Position, checkpoint1.LastEventID)
	}

	checkpoint2, err := projectionManager.GetCheckpoint("account-activity-log")
	if err != nil {
		log.Printf("   ⚠️  Activity log checkpoint: %v", err)
	} else {
		fmt.Printf("   account-activity-log: position=%d, last_event=%s\n", checkpoint2.Position, checkpoint2.LastEventID)
	}

	fmt.Println()

	// 9. Cleanup
	fmt.Println("9️⃣  Stopping projections...")
	projectionManager.StopAll()
	fmt.Println("   ✅ Projections stopped")
	fmt.Println()

	fmt.Println("✅ Demo complete!")
	fmt.Println()
	fmt.Println("Summary - How projections work with NATS:")
	fmt.Println()
	fmt.Println("1. Build projection using SQLite builder")
	fmt.Println("   projection, _ := sqlite.NewSQLiteProjectionBuilder(...).Build()")
	fmt.Println()
	fmt.Println("2. Create ProjectionManager with EventBus")
	fmt.Println("   manager := eventsourcing.NewProjectionManager(checkpointStore, eventStore, eventBus)")
	fmt.Println()
	fmt.Println("3. Register projections")
	fmt.Println("   manager.Register(projection)")
	fmt.Println()
	fmt.Println("4. Start projections (begins listening to NATS)")
	fmt.Println("   manager.Start(ctx, \"projection-name\")")
	fmt.Println()
	fmt.Println("5. Events published to NATS are automatically processed")
	fmt.Println("   - EventBus delivers events to all registered projections")
	fmt.Println("   - Projections update read models transactionally")
	fmt.Println("   - Checkpoints are saved atomically")
	fmt.Println()
	fmt.Println("6. Query read models for fast reads!")
	fmt.Println("   SELECT * FROM account_balance WHERE account_id = ?")
}

func mustMarshalProto(msg eventsourcing.ProtoMessage) []byte {
	data, err := eventsourcing.MarshalEvent(msg)
	if err != nil {
		panic(err)
	}
	return data
}
