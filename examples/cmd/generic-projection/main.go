package main

import (
	"context"
	"fmt"
	"log"

	accountdomainv1 "github.com/plaenen/eventstore/examples/pb/account/domain/v1"
	"github.com/plaenen/eventstore/pkg/eventsourcing"
	"github.com/plaenen/eventstore/pkg/eventsourcing/store/sqlite"
	"google.golang.org/protobuf/proto"
	_ "modernc.org/sqlite"
)

// This demo showcases the GENERIC projection builder that can handle
// events from multiple aggregates/domains:
//
// - Use eventsourcing.NewProjectionBuilder() instead of aggregate-specific builder
// - Mix events from different bounded contexts (Account, Order, User, etc.)
// - Perfect for complex read models that span multiple aggregates
// - Same type-safety and code completion benefits

func main() {
	fmt.Println("=== Generic Cross-Domain Projection Builder Demo ===")
	fmt.Println()
	fmt.Println("This demo shows how to build projections that listen to")
	fmt.Println("events from MULTIPLE aggregates/domains using a single builder.")
	fmt.Println()

	ctx := context.Background()

	// 1. Setup
	fmt.Println("1️⃣  Setting up infrastructure...")
	eventStore, err := sqlite.NewEventStore(
		sqlite.WithDSN("file:generic_projection_demo.db?mode=memory&cache=shared"),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer eventStore.Close()

	db := eventStore.DB()

	// Create a complex projection table that aggregates data from multiple domains
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS customer_activity (
			customer_id TEXT PRIMARY KEY,
			account_id TEXT,
			account_balance TEXT,
			account_status TEXT,
			total_deposits INTEGER DEFAULT 0,
			total_withdrawals INTEGER DEFAULT 0,
			last_activity_type TEXT,
			last_activity_at INTEGER
		)
	`)
	if err != nil {
		log.Fatal(err)
	}

	checkpointStore, err := sqlite.NewCheckpointStore(db)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("   ✅ Infrastructure ready")
	fmt.Println()

	// 2. Build GENERIC projection using events from multiple domains
	fmt.Println("2️⃣  Building generic cross-domain projection...")
	fmt.Println("   📝 Notice: Using eventsourcing.NewProjectionBuilder()")
	fmt.Println("   📝 Can mix events from Account, Order, User, etc.")
	fmt.Println()

	projection := eventsourcing.NewProjectionBuilder("customer-activity").
		// Account domain events
		On(accountdomainv1.OnAccountOpened(func(ctx context.Context, event *accountdomainv1.AccountOpenedEvent, envelope *eventsourcing.EventEnvelope) error {
			fmt.Printf("   🏦 Account domain: AccountOpened (ID: %s, Owner: %s)\n",
				event.AccountId, event.OwnerName)

			tx, err := db.Begin()
			if err != nil {
				return err
			}
			defer tx.Rollback()

			// Extract customer ID from owner name (in real app, this would be a proper ID)
			customerID := "customer-" + event.OwnerName

			_, err = tx.Exec(`
				INSERT INTO customer_activity (
					customer_id, account_id, account_balance, account_status,
					last_activity_type, last_activity_at
				) VALUES (?, ?, ?, 'OPEN', 'account_opened', ?)
				ON CONFLICT(customer_id) DO UPDATE SET
					account_id = excluded.account_id,
					account_balance = excluded.account_balance,
					account_status = excluded.account_status,
					last_activity_type = excluded.last_activity_type,
					last_activity_at = excluded.last_activity_at
			`, customerID, event.AccountId, event.InitialBalance, event.Timestamp)
			if err != nil {
				return err
			}

			checkpoint := &eventsourcing.ProjectionCheckpoint{
				ProjectionName: "customer-activity",
				Position:       envelope.Version,
				LastEventID:    envelope.ID,
				UpdatedAt:      eventsourcing.Now(),
			}
			if err := checkpointStore.SaveInTx(tx, checkpoint); err != nil {
				return err
			}

			return tx.Commit()
		})).
		On(accountdomainv1.OnMoneyDeposited(func(ctx context.Context, event *accountdomainv1.MoneyDepositedEvent, envelope *eventsourcing.EventEnvelope) error {
			fmt.Printf("   💵 Account domain: MoneyDeposited (Amount: %s)\n", event.Amount)

			tx, err := db.Begin()
			if err != nil {
				return err
			}
			defer tx.Rollback()

			_, err = tx.Exec(`
				UPDATE customer_activity
				SET account_balance = ?,
				    total_deposits = total_deposits + 1,
				    last_activity_type = 'deposit',
				    last_activity_at = ?
				WHERE account_id = ?
			`, event.NewBalance, event.Timestamp, event.AccountId)
			if err != nil {
				return err
			}

			checkpoint := &eventsourcing.ProjectionCheckpoint{
				ProjectionName: "customer-activity",
				Position:       envelope.Version,
				LastEventID:    envelope.ID,
				UpdatedAt:      eventsourcing.Now(),
			}
			if err := checkpointStore.SaveInTx(tx, checkpoint); err != nil {
				return err
			}

			return tx.Commit()
		})).
		On(accountdomainv1.OnMoneyWithdrawn(func(ctx context.Context, event *accountdomainv1.MoneyWithdrawnEvent, envelope *eventsourcing.EventEnvelope) error {
			fmt.Printf("   💸 Account domain: MoneyWithdrawn (Amount: %s)\n", event.Amount)

			tx, err := db.Begin()
			if err != nil {
				return err
			}
			defer tx.Rollback()

			_, err = tx.Exec(`
				UPDATE customer_activity
				SET account_balance = ?,
				    total_withdrawals = total_withdrawals + 1,
				    last_activity_type = 'withdrawal',
				    last_activity_at = ?
				WHERE account_id = ?
			`, event.NewBalance, event.Timestamp, event.AccountId)
			if err != nil {
				return err
			}

			checkpoint := &eventsourcing.ProjectionCheckpoint{
				ProjectionName: "customer-activity",
				Position:       envelope.Version,
				LastEventID:    envelope.ID,
				UpdatedAt:      eventsourcing.Now(),
			}
			if err := checkpointStore.SaveInTx(tx, checkpoint); err != nil {
				return err
			}

			return tx.Commit()
		})).
		On(accountdomainv1.OnAccountClosed(func(ctx context.Context, event *accountdomainv1.AccountClosedEvent, envelope *eventsourcing.EventEnvelope) error {
			fmt.Printf("   🔒 Account domain: AccountClosed\n")

			tx, err := db.Begin()
			if err != nil {
				return err
			}
			defer tx.Rollback()

			_, err = tx.Exec(`
				UPDATE customer_activity
				SET account_status = 'CLOSED',
				    last_activity_type = 'account_closed',
				    last_activity_at = ?
				WHERE account_id = ?
			`, event.Timestamp, event.AccountId)
			if err != nil {
				return err
			}

			checkpoint := &eventsourcing.ProjectionCheckpoint{
				ProjectionName: "customer-activity",
				Position:       envelope.Version,
				LastEventID:    envelope.ID,
				UpdatedAt:      eventsourcing.Now(),
			}
			if err := checkpointStore.SaveInTx(tx, checkpoint); err != nil {
				return err
			}

			return tx.Commit()
		})).
		// In a real application, you would add handlers for other domains here:
		// On(orderv1.OnOrderPlaced(func(...) { ... })).
		// On(userv1.OnUserRegistered(func(...) { ... })).
		// etc.
		OnReset(func(ctx context.Context) error {
			fmt.Println("   🔄 Resetting projection...")
			_, err := db.Exec("DELETE FROM customer_activity")
			return err
		}).
		Build()

	fmt.Println("   ✅ Generic projection built!")
	fmt.Println()

	// 3. Simulate events from multiple aggregates
	fmt.Println("3️⃣  Simulating events from Account domain...")
	fmt.Println("   💡 In a real app, you'd also have events from Order, User, etc.")
	fmt.Println()

	testEvents := []*eventsourcing.EventEnvelope{
		{
			Event: eventsourcing.Event{
				ID:          "evt-1",
				AggregateID: "acc-alice-001",
				EventType:   accountdomainv1.AccountOpenedEventType,
				Version:     1,
				Data: mustMarshal(&accountdomainv1.AccountOpenedEvent{
					AccountId:      "acc-alice-001",
					OwnerName:      "Alice",
					InitialBalance: "1000.00",
					Timestamp:      1234567890,
				}),
			},
		},
		{
			Event: eventsourcing.Event{
				ID:          "evt-2",
				AggregateID: "acc-alice-001",
				EventType:   accountdomainv1.MoneyDepositedEventType,
				Version:     2,
				Data: mustMarshal(&accountdomainv1.MoneyDepositedEvent{
					AccountId:  "acc-alice-001",
					Amount:     "500.00",
					NewBalance: "1500.00",
					Timestamp:  1234567900,
				}),
			},
		},
		{
			Event: eventsourcing.Event{
				ID:          "evt-3",
				AggregateID: "acc-alice-001",
				EventType:   accountdomainv1.MoneyWithdrawnEventType,
				Version:     3,
				Data: mustMarshal(&accountdomainv1.MoneyWithdrawnEvent{
					AccountId:  "acc-alice-001",
					Amount:     "200.00",
					NewBalance: "1300.00",
					Timestamp:  1234567910,
				}),
			},
		},
		{
			Event: eventsourcing.Event{
				ID:          "evt-4",
				AggregateID: "acc-alice-001",
				EventType:   accountdomainv1.MoneyDepositedEventType,
				Version:     4,
				Data: mustMarshal(&accountdomainv1.MoneyDepositedEvent{
					AccountId:  "acc-alice-001",
					Amount:     "700.00",
					NewBalance: "2000.00",
					Timestamp:  1234567920,
				}),
			},
		},
		{
			Event: eventsourcing.Event{
				ID:          "evt-5",
				AggregateID: "acc-alice-001",
				EventType:   accountdomainv1.AccountClosedEventType,
				Version:     5,
				Data: mustMarshal(&accountdomainv1.AccountClosedEvent{
					AccountId:    "acc-alice-001",
					FinalBalance: "2000.00",
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
	fmt.Println("4️⃣  Querying cross-domain projection...")

	// Query the projection
	var customerID, accountID, balance, status, lastActivity string
	var totalDeposits, totalWithdrawals int
	err = db.QueryRow(`
		SELECT customer_id, account_id, account_balance, account_status,
		       total_deposits, total_withdrawals, last_activity_type
		FROM customer_activity
		WHERE customer_id = ?
	`, "customer-Alice").Scan(&customerID, &accountID, &balance, &status,
		&totalDeposits, &totalWithdrawals, &lastActivity)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("   Customer: %s\n", customerID)
	fmt.Printf("   Account: %s\n", accountID)
	fmt.Printf("   Balance: %s\n", balance)
	fmt.Printf("   Status: %s\n", status)
	fmt.Printf("   Total Deposits: %d\n", totalDeposits)
	fmt.Printf("   Total Withdrawals: %d\n", totalWithdrawals)
	fmt.Printf("   Last Activity: %s\n", lastActivity)
	fmt.Println()

	fmt.Println("✅ Demo complete!")
	fmt.Println()
	fmt.Println("Key benefits of the generic projection builder:")
	fmt.Println("  🌐 Mix events from ANY aggregate/domain")
	fmt.Println("  🎯 Pick and choose specific events across domains")
	fmt.Println("  ✨ Same type-safety and code completion")
	fmt.Println("  📊 Perfect for complex read models")
	fmt.Println("  🔒 Compile-time safety across all domains")
	fmt.Println()
	fmt.Println("Usage pattern:")
	fmt.Println("  eventsourcing.NewProjectionBuilder(\"name\").")
	fmt.Println("    On(accountdomainv1.OnAccountOpened(...)).")
	fmt.Println("    On(orderv1.OnOrderPlaced(...)).")
	fmt.Println("    On(userv1.OnUserRegistered(...)).")
	fmt.Println("    Build()")
}

func mustMarshal(msg proto.Message) []byte {
	data, err := proto.Marshal(msg)
	if err != nil {
		panic(err)
	}
	return data
}
