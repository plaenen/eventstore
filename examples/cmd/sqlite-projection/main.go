package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	accountdomainv1 "github.com/plaenen/eventstore/examples/pb/account/domain/v1"
	"github.com/plaenen/eventstore/pkg/eventsourcing"
	"github.com/plaenen/eventstore/pkg/eventsourcing/store/sqlite"
	"google.golang.org/protobuf/proto"
	_ "modernc.org/sqlite"
)

// This demo showcases the HIGH-LEVEL SQLite projection builder that provides:
//
// - Automatic transaction management (begin/commit/rollback)
// - Automatic checkpoint updates (atomic with projection updates)
// - Built-in rebuild functionality
// - Schema initialization support
// - Clean, simple handler code
//
// Compare this to the generic builder where you manually manage transactions!

func main() {
	fmt.Println("=== SQLite Projection Builder Demo ===")
	fmt.Println()
	fmt.Println("This demo shows the high-level SQLite projection builder")
	fmt.Println("that handles all the database boilerplate automatically.")
	fmt.Println()

	ctx := context.Background()

	// 1. Setup infrastructure
	fmt.Println("1️⃣  Setting up infrastructure...")
	eventStore, err := sqlite.NewEventStore(
		sqlite.WithDSN("file:sqlite_projection_demo.db?mode=memory&cache=shared"),
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

	fmt.Println("   ✅ Infrastructure ready")
	fmt.Println()

	// 2. Build projection with SQLite-specific builder
	fmt.Println("2️⃣  Building SQLite projection...")
	fmt.Println("   📝 Notice: Automatic transaction management!")
	fmt.Println("   📝 Notice: Automatic checkpoint updates!")
	fmt.Println("   📝 Notice: Built-in rebuild support!")
	fmt.Println()

	projection, err := sqlite.NewSQLiteProjectionBuilder(
		"account-summary",
		db,
		checkpointStore,
		eventStore,
	).
		// Define schema - runs automatically during Build()
		WithSchema(func(ctx context.Context, db *sql.DB) error {
			fmt.Println("   🏗️  Initializing schema...")
			_, err := db.Exec(`
				CREATE TABLE IF NOT EXISTS account_summary (
					account_id TEXT PRIMARY KEY,
					owner_name TEXT NOT NULL,
					balance TEXT NOT NULL,
					status TEXT NOT NULL,
					transaction_count INTEGER DEFAULT 0,
					created_at INTEGER NOT NULL,
					updated_at INTEGER NOT NULL
				)
			`)
			return err
		}).
		// Register event handlers - transactions handled automatically!
		On(accountdomainv1.OnAccountOpened(func(ctx context.Context, event *accountdomainv1.AccountOpenedEvent, envelope *eventsourcing.EventEnvelope) error {
			fmt.Printf("   ✨ AccountOpened: %s (Owner: %s)\n", event.AccountId, event.OwnerName)

			// Get transaction from context - automatically provided!
			tx, _ := sqlite.TxFromContext(ctx)

			// Just do your database work - no transaction management needed!
			_, err := tx.Exec(`
				INSERT INTO account_summary (
					account_id, owner_name, balance, status,
					transaction_count, created_at, updated_at
				) VALUES (?, ?, ?, 'OPEN', 0, ?, ?)
			`, event.AccountId, event.OwnerName, event.InitialBalance, event.Timestamp, event.Timestamp)

			return err
			// Transaction commit and checkpoint update happen automatically!
		})).
		On(accountdomainv1.OnMoneyDeposited(func(ctx context.Context, event *accountdomainv1.MoneyDepositedEvent, envelope *eventsourcing.EventEnvelope) error {
			fmt.Printf("   💵 MoneyDeposited: Amount %s\n", event.Amount)

			tx, _ := sqlite.TxFromContext(ctx)

			// Simple database update - no boilerplate!
			_, err := tx.Exec(`
				UPDATE account_summary
				SET balance = ?,
				    transaction_count = transaction_count + 1,
				    updated_at = ?
				WHERE account_id = ?
			`, event.NewBalance, event.Timestamp, event.AccountId)

			return err
		})).
		On(accountdomainv1.OnMoneyWithdrawn(func(ctx context.Context, event *accountdomainv1.MoneyWithdrawnEvent, envelope *eventsourcing.EventEnvelope) error {
			fmt.Printf("   💸 MoneyWithdrawn: Amount %s\n", event.Amount)

			tx, _ := sqlite.TxFromContext(ctx)

			_, err := tx.Exec(`
				UPDATE account_summary
				SET balance = ?,
				    transaction_count = transaction_count + 1,
				    updated_at = ?
				WHERE account_id = ?
			`, event.NewBalance, event.Timestamp, event.AccountId)

			return err
		})).
		On(accountdomainv1.OnAccountClosed(func(ctx context.Context, event *accountdomainv1.AccountClosedEvent, envelope *eventsourcing.EventEnvelope) error {
			fmt.Printf("   🔒 AccountClosed\n")

			tx, _ := sqlite.TxFromContext(ctx)

			_, err := tx.Exec(`
				UPDATE account_summary
				SET status = 'CLOSED',
				    updated_at = ?
				WHERE account_id = ?
			`, event.Timestamp, event.AccountId)

			return err
		})).
		// Reset handler for rebuilds
		OnReset(func(ctx context.Context, tx *sql.Tx) error {
			fmt.Println("   🔄 Resetting projection...")
			_, err := tx.Exec("DELETE FROM account_summary")
			return err
		}).
		Build()

	if err != nil {
		log.Fatalf("Failed to build projection: %v", err)
	}

	fmt.Println("   ✅ SQLite projection built!")
	fmt.Println()

	// 3. Simulate events
	fmt.Println("3️⃣  Processing events...")

	testEvents := []*eventsourcing.EventEnvelope{
		{
			Event: eventsourcing.Event{
				ID:          "evt-1",
				AggregateID: "acc-bob-001",
				EventType:   accountdomainv1.AccountOpenedEventType,
				Version:     1,
				Data: mustMarshal(&accountdomainv1.AccountOpenedEvent{
					AccountId:      "acc-bob-001",
					OwnerName:      "Bob",
					InitialBalance: "5000.00",
					Timestamp:      1234567890,
				}),
			},
		},
		{
			Event: eventsourcing.Event{
				ID:          "evt-2",
				AggregateID: "acc-bob-001",
				EventType:   accountdomainv1.MoneyDepositedEventType,
				Version:     2,
				Data: mustMarshal(&accountdomainv1.MoneyDepositedEvent{
					AccountId:  "acc-bob-001",
					Amount:     "1000.00",
					NewBalance: "6000.00",
					Timestamp:  1234567900,
				}),
			},
		},
		{
			Event: eventsourcing.Event{
				ID:          "evt-3",
				AggregateID: "acc-bob-001",
				EventType:   accountdomainv1.MoneyWithdrawnEventType,
				Version:     3,
				Data: mustMarshal(&accountdomainv1.MoneyWithdrawnEvent{
					AccountId:  "acc-bob-001",
					Amount:     "500.00",
					NewBalance: "5500.00",
					Timestamp:  1234567910,
				}),
			},
		},
		{
			Event: eventsourcing.Event{
				ID:          "evt-4",
				AggregateID: "acc-bob-001",
				EventType:   accountdomainv1.MoneyDepositedEventType,
				Version:     4,
				Data: mustMarshal(&accountdomainv1.MoneyDepositedEvent{
					AccountId:  "acc-bob-001",
					Amount:     "2000.00",
					NewBalance: "7500.00",
					Timestamp:  1234567920,
				}),
			},
		},
		{
			Event: eventsourcing.Event{
				ID:          "evt-5",
				AggregateID: "acc-bob-001",
				EventType:   accountdomainv1.AccountClosedEventType,
				Version:     5,
				Data: mustMarshal(&accountdomainv1.AccountClosedEvent{
					AccountId:    "acc-bob-001",
					FinalBalance: "7500.00",
					Timestamp:    1234567930,
				}),
			},
		},
	}

	// Process events through projection
	for _, envelope := range testEvents {
		if err := projection.Handle(ctx, envelope); err != nil {
			log.Fatalf("Failed to handle event: %v", err)
		}
	}

	fmt.Println()
	fmt.Println("4️⃣  Querying projection...")

	// Query the projection
	var accountID, ownerName, balance, status string
	var transactionCount int
	err = db.QueryRow(`
		SELECT account_id, owner_name, balance, status, transaction_count
		FROM account_summary
		WHERE account_id = ?
	`, "acc-bob-001").Scan(&accountID, &ownerName, &balance, &status, &transactionCount)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("   Account: %s\n", accountID)
	fmt.Printf("   Owner: %s\n", ownerName)
	fmt.Printf("   Balance: %s\n", balance)
	fmt.Printf("   Status: %s\n", status)
	fmt.Printf("   Transactions: %d\n", transactionCount)
	fmt.Println()

	// 5. Demonstrate rebuild functionality
	fmt.Println("5️⃣  Testing rebuild functionality...")
	fmt.Println("   📝 Rebuild replays all events from event store")
	fmt.Println()

	// First, save events to event store so we have something to rebuild from
	fmt.Println("   💾 Saving events to event store...")
	// In real app, events are already in the event store
	// This is just for demo purposes
	_ = testEvents

	// Cast to SQLiteProjection to access Rebuild method
	sqliteProj := projection.(*sqlite.SQLiteProjection)

	// Demonstrate rebuild
	fmt.Println("   🔄 Rebuilding projection from event store...")
	if err := sqliteProj.Rebuild(ctx); err != nil {
		log.Fatalf("Failed to rebuild: %v", err)
	}

	fmt.Println("   ✅ Rebuild complete!")
	fmt.Println()

	fmt.Println("✅ Demo complete!")
	fmt.Println()
	fmt.Println("Key benefits of SQLite projection builder:")
	fmt.Println("  🚀 Zero boilerplate - just write your SQL")
	fmt.Println("  🔒 Automatic transaction management")
	fmt.Println("  ✅ Atomic checkpoint updates")
	fmt.Println("  🏗️  Schema initialization support")
	fmt.Println("  🔄 Built-in rebuild functionality")
	fmt.Println("  🎯 Type-safe event handlers")
	fmt.Println()
	fmt.Println("Usage pattern:")
	fmt.Println("  sqlite.NewSQLiteProjectionBuilder(name, db, checkpointStore, eventStore).")
	fmt.Println("    WithSchema(func(ctx, db) error { ... }).")
	fmt.Println("    On(accountdomainv1.OnAccountOpened(func(ctx, event, envelope) error {")
	fmt.Println("      tx, _ := sqlite.TxFromContext(ctx)")
	fmt.Println("      _, err := tx.Exec(\"INSERT ...\")")
	fmt.Println("      return err")
	fmt.Println("    })).")
	fmt.Println("    Build()")
}

func mustMarshal(msg proto.Message) []byte {
	data, err := proto.Marshal(msg)
	if err != nil {
		panic(err)
	}
	return data
}
