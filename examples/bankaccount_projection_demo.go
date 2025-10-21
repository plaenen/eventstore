package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/plaenen/eventstore/examples/bankaccount/projections"
	accountv1 "github.com/plaenen/eventstore/examples/pb/account/v1"
	"github.com/plaenen/eventstore/pkg/eventsourcing"
	natspkg "github.com/plaenen/eventstore/pkg/nats"
	"github.com/plaenen/eventstore/pkg/sqlite"
)

func main() {
	fmt.Println("=== Bank Account Projection Demo ===\n")

	ctx := context.Background()

	// 1. Setup Event Store
	fmt.Println("1. Setting up Event Store...")
	eventStore, err := sqlite.NewEventStore(
		sqlite.WithFilename("./projection_demo.db"),
		sqlite.WithWALMode(true),
	)
	if err != nil {
		log.Fatalf("Failed to create event store: %v", err)
	}
	defer eventStore.Close()
	fmt.Println("✓ Event store ready\n")

	// 2. Setup Event Bus (NATS)
	fmt.Println("2. Setting up Event Bus...")
	eventBus, srv, err := natspkg.NewEmbeddedEventBus()
	if err != nil {
		log.Fatalf("Failed to create event bus: %v", err)
	}
	defer srv.Shutdown()
	defer eventBus.Close()
	fmt.Println("✓ Event bus ready\n")

	// 3. Create Projection Table
	fmt.Println("3. Creating projection table...")
	_, err = eventStore.DB().Exec(`
		CREATE TABLE IF NOT EXISTS account_view (
			account_id TEXT PRIMARY KEY,
			owner_name TEXT NOT NULL,
			balance TEXT NOT NULL,
			status TEXT NOT NULL,
			version INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER,
			updated_at INTEGER
		)
	`)
	if err != nil {
		log.Fatalf("Failed to create table: %v", err)
	}
	fmt.Println("✓ Table created\n")

	// 4. Setup Projection Manager
	fmt.Println("4. Setting up Projection Manager...")
	checkpointStore, err := sqlite.NewCheckpointStore(eventStore.DB())
	if err != nil {
		log.Fatalf("Failed to create checkpoint store: %v", err)
	}

	projectionMgr := eventsourcing.NewProjectionManager(checkpointStore, eventStore, eventBus)

	// Register the account view projection
	accountViewProjection := projections.NewAccountViewProjection(eventStore.DB())
	projectionMgr.Register(accountViewProjection)

	// Start the projection
	err = projectionMgr.Start(ctx, "account_view")
	if err != nil {
		log.Fatalf("Failed to start projection: %v", err)
	}
	defer projectionMgr.Stop("account_view")

	fmt.Println("✓ Projection manager started\n")
	time.Sleep(200 * time.Millisecond) // Wait for subscription

	// 5. Create Repository
	fmt.Println("5. Setting up repository...")
	repo := accountv1.NewAccountRepository(eventStore)
	fmt.Println("✓ Repository ready\n")

	// 6. Execute Commands and Watch Projection Update
	fmt.Println("=== Executing Commands & Watching Projections ===\n")

	// Command 1: Open Account
	fmt.Println("📝 Command: Open Account")
	account1 := accountv1.NewAccount("acc-projection-1")
	commandID1 := eventsourcing.GenerateID()
	account1.SetCommandID(commandID1)

	cmd1 := &accountv1.OpenAccountCommand{
		AccountId:      "acc-projection-1",
		OwnerName:      "Alice Smith",
		InitialBalance: "1000.00",
	}

	metadata1 := eventsourcing.EventMetadata{
		CausationID: commandID1,
		PrincipalID: "admin",
	}

	account1.OpenAccount(ctx, cmd1, metadata1)

	// Get events before saving (Save() will mark them as committed)
	events := account1.UncommittedEvents()

	if _, err := repo.SaveWithCommand(account1, commandID1); err != nil {
		log.Fatalf("Failed to save account: %v", err)
	}

	// Publish events to event bus for real-time projections
	if err := eventBus.Publish(events); err != nil {
		log.Fatalf("Failed to publish events: %v", err)
	}

	fmt.Println("  ✓ Command executed")

	// Give projection time to process
	time.Sleep(300 * time.Millisecond)

	// Query projection
	showAccountView(eventStore.DB(), "acc-projection-1")

	// Command 2: Deposit Money
	fmt.Println("\n📝 Command: Deposit $500")
	account1Reload, err := repo.Load("acc-projection-1")
	if err != nil {
		log.Fatalf("Failed to load account: %v", err)
	}

	commandID2 := eventsourcing.GenerateID()
	account1Reload.SetCommandID(commandID2)

	cmd2 := &accountv1.DepositCommand{
		AccountId: "acc-projection-1",
		Amount:    "500.00",
	}

	metadata2 := eventsourcing.EventMetadata{
		CausationID: commandID2,
		PrincipalID: "admin",
	}

	account1Reload.Deposit(ctx, cmd2, metadata2)

	events = account1Reload.UncommittedEvents()

	if _, err = repo.SaveWithCommand(account1Reload, commandID2); err != nil {
		log.Fatalf("Failed to save account: %v", err)
	}

	if err = eventBus.Publish(events); err != nil {
		log.Fatalf("Failed to publish events: %v", err)
	}

	fmt.Println("  ✓ Command executed")

	time.Sleep(300 * time.Millisecond)
	showAccountView(eventStore.DB(), "acc-projection-1")

	// Command 3: Withdraw Money
	fmt.Println("\n📝 Command: Withdraw $200")
	account1Reload2, err := repo.Load("acc-projection-1")
	if err != nil {
		log.Fatalf("Failed to load account: %v", err)
	}

	commandID3 := eventsourcing.GenerateID()
	account1Reload2.SetCommandID(commandID3)

	cmd3 := &accountv1.WithdrawCommand{
		AccountId: "acc-projection-1",
		Amount:    "200.00",
	}

	metadata3 := eventsourcing.EventMetadata{
		CausationID: commandID3,
		PrincipalID: "admin",
	}

	account1Reload2.Withdraw(ctx, cmd3, metadata3)

	events = account1Reload2.UncommittedEvents()

	if _, err = repo.SaveWithCommand(account1Reload2, commandID3); err != nil {
		log.Fatalf("Failed to save account: %v", err)
	}

	if err = eventBus.Publish(events); err != nil {
		log.Fatalf("Failed to publish events: %v", err)
	}

	fmt.Println("  ✓ Command executed")

	time.Sleep(300 * time.Millisecond)
	showAccountView(eventStore.DB(), "acc-projection-1")

	// Command 4: Open Another Account
	fmt.Println("\n📝 Command: Open Second Account")
	account2 := accountv1.NewAccount("acc-projection-2")
	commandID4 := eventsourcing.GenerateID()
	account2.SetCommandID(commandID4)

	cmd4 := &accountv1.OpenAccountCommand{
		AccountId:      "acc-projection-2",
		OwnerName:      "Bob Johnson",
		InitialBalance: "2500.00",
	}

	metadata4 := eventsourcing.EventMetadata{
		CausationID: commandID4,
		PrincipalID: "admin",
	}

	account2.OpenAccount(ctx, cmd4, metadata4)

	events = account2.UncommittedEvents()

	if _, err = repo.SaveWithCommand(account2, commandID4); err != nil {
		log.Fatalf("Failed to save account: %v", err)
	}

	if err = eventBus.Publish(events); err != nil {
		log.Fatalf("Failed to publish events: %v", err)
	}

	fmt.Println("  ✓ Command executed")

	time.Sleep(300 * time.Millisecond)
	showAccountView(eventStore.DB(), "acc-projection-2")

	// Command 5: Close Account
	fmt.Println("\n📝 Command: Close First Account")
	account1Reload3, err := repo.Load("acc-projection-1")
	if err != nil {
		log.Fatalf("Failed to load account: %v", err)
	}

	commandID5 := eventsourcing.GenerateID()
	account1Reload3.SetCommandID(commandID5)

	cmd5 := &accountv1.CloseAccountCommand{
		AccountId: "acc-projection-1",
	}

	metadata5 := eventsourcing.EventMetadata{
		CausationID: commandID5,
		PrincipalID: "admin",
	}

	account1Reload3.CloseAccount(ctx, cmd5, metadata5)

	events = account1Reload3.UncommittedEvents()

	if _, err = repo.SaveWithCommand(account1Reload3, commandID5); err != nil {
		log.Fatalf("Failed to save account: %v", err)
	}

	if err = eventBus.Publish(events); err != nil {
		log.Fatalf("Failed to publish events: %v", err)
	}

	fmt.Println("  ✓ Command executed")

	time.Sleep(300 * time.Millisecond)
	showAccountView(eventStore.DB(), "acc-projection-1")

	// 7. Show All Accounts
	fmt.Println("\n=== All Accounts in Projection ===")
	showAllAccounts(eventStore.DB())

	// 8. Show Checkpoint
	fmt.Println("\n=== Projection Checkpoint ===")
	checkpoint, err := projectionMgr.GetCheckpoint("account_view")
	if err != nil {
		fmt.Printf("  (No checkpoint yet - projection may not have processed events)\n")
	} else {
		fmt.Printf("  Position: %d\n", checkpoint.Position)
		fmt.Printf("  Updated At: %s\n", checkpoint.UpdatedAt.Format(time.RFC3339))
	}

	// 9. Demonstrate Rebuild
	fmt.Println("\n=== Testing Projection Rebuild ===")
	fmt.Println("Clearing projection data...")
	err = accountViewProjection.Reset(ctx)
	if err != nil {
		log.Fatalf("Failed to reset projection: %v", err)
	}

	showAllAccounts(eventStore.DB())

	fmt.Println("\nRebuilding projection from event history...")
	projectionMgr.Stop("account_view")
	time.Sleep(500 * time.Millisecond) // Wait for projection to fully stop
	err = projectionMgr.Rebuild(ctx, "account_view")
	if err != nil {
		log.Fatalf("Failed to rebuild projection: %v", err)
	}

	err = projectionMgr.Start(ctx, "account_view")
	if err != nil {
		log.Fatalf("Failed to restart projection: %v", err)
	}
	defer projectionMgr.Stop("account_view")

	time.Sleep(300 * time.Millisecond)

	fmt.Println("\nAfter rebuild:")
	showAllAccounts(eventStore.DB())

	// 10. Compare Event Store vs Projection
	fmt.Println("\n=== Event Store vs Projection ===")
	loadedEvents, err := eventStore.LoadEvents("acc-projection-1", 0)
	if err != nil {
		log.Fatalf("Failed to load events: %v", err)
	}
	fmt.Printf("Event Store: %d events for acc-projection-1\n", len(loadedEvents))

	var viewExists bool
	err = eventStore.DB().QueryRow(`
		SELECT EXISTS(SELECT 1 FROM account_view WHERE account_id = ?)
	`, "acc-projection-1").Scan(&viewExists)
	if err != nil {
		log.Fatalf("Failed to query view: %v", err)
	}
	fmt.Printf("Projection:  Account exists in view: %v\n", viewExists)

	fmt.Println("\n=== Demo Complete ===")
	fmt.Println("\nKey Concepts Demonstrated:")
	fmt.Println("  1. ✅ Projections update automatically when events are published")
	fmt.Println("  2. ✅ Projections maintain denormalized read models")
	fmt.Println("  3. ✅ Projections track checkpoints for resumption")
	fmt.Println("  4. ✅ Projections can be rebuilt from event history")
	fmt.Println("  5. ✅ Event sourcing separates write (events) from read (projections)")
	fmt.Println("\nDatabase file: projection_demo.db")
	fmt.Println("Explore with:")
	fmt.Println("  sqlite3 projection_demo.db \"SELECT * FROM events;\"")
	fmt.Println("  sqlite3 projection_demo.db \"SELECT * FROM account_view;\"")
	fmt.Println("  sqlite3 projection_demo.db \"SELECT * FROM projection_checkpoints;\"")
}

func showAccountView(db *sql.DB, accountID string) {
	var ownerName, balance, status string
	err := db.QueryRow(`
		SELECT owner_name, balance, status
		FROM account_view
		WHERE account_id = ?
	`, accountID).Scan(&ownerName, &balance, &status)

	if err == sql.ErrNoRows {
		fmt.Printf("  📊 Projection: Account not found in view\n")
		return
	}
	if err != nil {
		log.Printf("  ⚠️  Query error: %v", err)
		return
	}

	fmt.Printf("  📊 Projection Updated:\n")
	fmt.Printf("     Owner:   %s\n", ownerName)
	fmt.Printf("     Balance: $%s\n", balance)
	fmt.Printf("     Status:  %s\n", status)
}

func showAllAccounts(db *sql.DB) {
	rows, err := db.Query(`
		SELECT account_id, owner_name, balance, status
		FROM account_view
		ORDER BY account_id
	`)
	if err != nil {
		log.Fatalf("Failed to query accounts: %v", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var accountID, ownerName, balance, status string
		err := rows.Scan(&accountID, &ownerName, &balance, &status)
		if err != nil {
			log.Fatalf("Failed to scan row: %v", err)
		}

		fmt.Printf("  %s | %-15s | $%-10s | %s\n",
			accountID, ownerName, balance, status)
		count++
	}

	if count == 0 {
		fmt.Println("  (No accounts in projection)")
	} else {
		fmt.Printf("  Total: %d accounts\n", count)
	}
}
