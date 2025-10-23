package main

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log"

	accountv1 "github.com/plaenen/eventstore/examples/pb/account/v1"
	"github.com/plaenen/eventstore/pkg/domain"
	"github.com/plaenen/eventstore/pkg/store/sqlite"
	"google.golang.org/protobuf/proto"
	_ "modernc.org/sqlite"
)

// Embed the migrations directory
//
//go:embed projections
var migrationsFS embed.FS

// This demo showcases projection migrations using embedded file systems.
//
// Benefits of WithMigrations over WithSchema:
// - Version-controlled schema evolution
// - Automatic migration tracking
// - Rollback support
// - Team collaboration friendly
// - Production-ready migration strategy

func main() {
	fmt.Println("=== Projection with Embedded Migrations Demo ===")
	fmt.Println()
	fmt.Println("This demo shows how to use embedded migrations for projections.")
	fmt.Println("Migrations are compiled into the binary for easy deployment!")
	fmt.Println()

	ctx := context.Background()

	// 1. Setup infrastructure
	fmt.Println("1️⃣  Setting up infrastructure...")
	eventStore, err := sqlite.NewEventStore(
		sqlite.WithDSN("file:projection_migrations_demo.db?mode=memory&cache=shared"),
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

	// 2. Build projection with embedded migrations
	fmt.Println("2️⃣  Building projection with embedded migrations...")
	fmt.Println("   📁 Migrations embedded from: examples/projections/account_balance/migrations/")
	fmt.Println()

	projection, err := sqlite.NewSQLiteProjectionBuilder(
		"account-balance",
		db,
		checkpointStore,
		eventStore,
	).
		// Use WithMigrations instead of WithSchema!
		// Migrations are automatically run during Build()
		WithMigrations(migrationsFS, "projections/account_balance/migrations").
		// Register event handlers
		On(accountv1.OnAccountOpened(func(ctx context.Context, event *accountv1.AccountOpenedEvent, envelope *domain.EventEnvelope) error {
			fmt.Printf("   ✨ AccountOpened: %s (Owner: %s)\n", event.AccountId, event.OwnerName)

			tx, _ := sqlite.TxFromContext(ctx)

			// Schema evolved through migrations!
			// - Migration 1: Added account_balance table
			// - Migration 2: Added owner_name column
			// - Migration 3: Added index on updated_at
			_, err := tx.Exec(`
				INSERT INTO account_balance (account_id, owner_name, balance, updated_at)
				VALUES (?, ?, ?, ?)
			`, event.AccountId, event.OwnerName, event.InitialBalance, event.Timestamp)

			return err
		})).
		On(accountv1.OnMoneyDeposited(func(ctx context.Context, event *accountv1.MoneyDepositedEvent, envelope *domain.EventEnvelope) error {
			fmt.Printf("   💵 MoneyDeposited: Amount %s\n", event.Amount)

			tx, _ := sqlite.TxFromContext(ctx)
			_, err := tx.Exec(`
				UPDATE account_balance
				SET balance = ?, updated_at = ?
				WHERE account_id = ?
			`, event.NewBalance, event.Timestamp, event.AccountId)

			return err
		})).
		On(accountv1.OnMoneyWithdrawn(func(ctx context.Context, event *accountv1.MoneyWithdrawnEvent, envelope *domain.EventEnvelope) error {
			fmt.Printf("   💸 MoneyWithdrawn: Amount %s\n", event.Amount)

			tx, _ := sqlite.TxFromContext(ctx)
			_, err := tx.Exec(`
				UPDATE account_balance
				SET balance = ?, updated_at = ?
				WHERE account_id = ?
			`, event.NewBalance, event.Timestamp, event.AccountId)

			return err
		})).
		OnReset(func(ctx context.Context, tx *sql.Tx) error {
			fmt.Println("   🔄 Resetting projection...")
			_, err := tx.Exec("DELETE FROM account_balance")
			return err
		}).
		Build()

	if err != nil {
		log.Fatalf("Failed to build projection: %v", err)
	}

	fmt.Println("   ✅ Projection built!")
	fmt.Println("   ✅ Migrations applied automatically!")
	fmt.Println()

	// 3. Check what migrations were applied
	fmt.Println("3️⃣  Checking applied migrations...")

	rows, err := db.Query(`
		SELECT version, name, applied_at
		FROM projection_account_balance_schema_migrations
		ORDER BY version
	`)
	if err != nil {
		log.Printf("   ⚠️  Could not query migrations: %v", err)
	} else {
		defer rows.Close()

		fmt.Println("   Applied migrations:")
		for rows.Next() {
			var version int
			var name string
			var appliedAt int64
			if err := rows.Scan(&version, &name, &appliedAt); err != nil {
				continue
			}
			fmt.Printf("   ✓ Version %d: %s\n", version, name)
		}
		fmt.Println()
	}

	// 4. Process events
	fmt.Println("4️⃣  Processing events...")

	testEvents := []*domain.EventEnvelope{
		{
			Event: domain.Event{
				ID:          "evt-1",
				AggregateID: "acc-diana-001",
				EventType:   accountv1.AccountOpenedEventType,
				Version:     1,
				Data:        mustMarshal(&accountv1.AccountOpenedEvent{
					AccountId:      "acc-diana-001",
					OwnerName:      "Diana",
					InitialBalance: "8000.00",
					Timestamp:      1234567890,
				}),
			},
		},
		{
			Event: domain.Event{
				ID:          "evt-2",
				AggregateID: "acc-diana-001",
				EventType:   accountv1.MoneyDepositedEventType,
				Version:     2,
				Data:        mustMarshal(&accountv1.MoneyDepositedEvent{
					AccountId:  "acc-diana-001",
					Amount:     "3000.00",
					NewBalance: "11000.00",
					Timestamp:  1234567900,
				}),
			},
		},
		{
			Event: domain.Event{
				ID:          "evt-3",
				AggregateID: "acc-diana-001",
				EventType:   accountv1.MoneyWithdrawnEventType,
				Version:     3,
				Data:        mustMarshal(&accountv1.MoneyWithdrawnEvent{
					AccountId:  "acc-diana-001",
					Amount:     "1000.00",
					NewBalance: "10000.00",
					Timestamp:  1234567910,
				}),
			},
		},
	}

	for _, envelope := range testEvents {
		if err := projection.Handle(ctx, envelope); err != nil {
			log.Fatalf("Failed to handle event: %v", err)
		}
	}

	fmt.Println()
	fmt.Println("5️⃣  Querying projection...")

	// Query the projection with all evolved fields
	var accountID, ownerName, balance string
	var updatedAt int64
	err = db.QueryRow(`
		SELECT account_id, owner_name, balance, updated_at
		FROM account_balance
		WHERE account_id = ?
	`, "acc-diana-001").Scan(&accountID, &ownerName, &balance, &updatedAt)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("   Account: %s\n", accountID)
	fmt.Printf("   Owner: %s (from migration 2)\n", ownerName)
	fmt.Printf("   Balance: %s\n", balance)
	fmt.Printf("   Updated: %d\n", updatedAt)
	fmt.Println()

	// 6. Check that index exists (from migration 3)
	fmt.Println("6️⃣  Verifying schema evolution...")

	var indexExists int
	err = db.QueryRow(`
		SELECT COUNT(*)
		FROM sqlite_master
		WHERE type = 'index'
		AND name = 'idx_account_balance_updated_at'
	`).Scan(&indexExists)
	if err != nil {
		log.Printf("   ⚠️  Could not check index: %v", err)
	} else if indexExists > 0 {
		fmt.Println("   ✅ Index on updated_at exists (from migration 3)")
	}

	fmt.Println()
	fmt.Println("✅ Demo complete!")
	fmt.Println()
	fmt.Println("Key benefits of embedded migrations:")
	fmt.Println("  📦 Migrations compiled into binary")
	fmt.Println("  🔄 Version-controlled schema evolution")
	fmt.Println("  🚀 No external migration files needed")
	fmt.Println("  ✅ Automatic migration tracking")
	fmt.Println("  👥 Team-friendly (migrations in git)")
	fmt.Println("  🎯 Production-ready deployment")
	fmt.Println()
	fmt.Println("Migration workflow:")
	fmt.Println("  1. Add migration file: projections/account_balance/migrations/000004_new_feature.up.sql")
	fmt.Println("  2. Binary automatically embeds it via //go:embed")
	fmt.Println("  3. On startup, Build() runs pending migrations")
	fmt.Println("  4. Migration tracking prevents re-running")
}

func mustMarshal(msg proto.Message) []byte {
	data, err := proto.Marshal(msg)
	if err != nil {
		panic(err)
	}
	return data
}
