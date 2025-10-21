package sqlite_test

import (
	"fmt"
	"log"
	"time"

	"github.com/plaenen/eventstore/pkg/eventsourcing"
	"github.com/plaenen/eventstore/pkg/sqlite"
)

// ExampleCheckpointStore_SaveInTx demonstrates how to use transactional
// checkpoint updates to avoid dual-write issues when updating projections.
func ExampleCheckpointStore_SaveInTx() {
	// Create event store
	store, err := sqlite.NewEventStore(
		sqlite.WithDSN(":memory:"),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()

	// Create checkpoint store
	checkpointStore, err := sqlite.NewCheckpointStore(store.DB())
	if err != nil {
		log.Fatal(err)
	}

	// Create a projection table (e.g., user summary)
	_, err = store.DB().Exec(`
		CREATE TABLE user_summary (
			user_id TEXT PRIMARY KEY,
			total_orders INTEGER,
			last_updated INTEGER
		)
	`)
	if err != nil {
		log.Fatal(err)
	}

	// Simulate processing an event and updating the projection
	// This demonstrates the CORRECT way to avoid dual-write issues

	// Begin transaction
	tx, err := checkpointStore.DB().Begin()
	if err != nil {
		log.Fatal(err)
	}
	defer tx.Rollback() // Rollback if we don't commit

	// Update projection within the transaction
	_, err = tx.Exec(`
		INSERT INTO user_summary (user_id, total_orders, last_updated)
		VALUES (?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET
			total_orders = total_orders + 1,
			last_updated = ?
	`, "user-123", 1, time.Now().Unix(), time.Now().Unix())
	if err != nil {
		log.Fatal(err)
	}

	// Save checkpoint in the SAME transaction
	// This ensures atomicity - either both succeed or both fail
	checkpoint := &eventsourcing.ProjectionCheckpoint{
		ProjectionName: "user-summary",
		Position:       100,
		LastEventID:    "event-100",
		UpdatedAt:      time.Now(),
	}
	err = checkpointStore.SaveInTx(tx, checkpoint)
	if err != nil {
		log.Fatal(err)
	}

	// Commit the transaction
	// Both the projection update and checkpoint save are atomic
	err = tx.Commit()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Projection and checkpoint updated atomically")
	// Output: Projection and checkpoint updated atomically
}

// Example_projectionLoop demonstrates a complete projection update loop
// using transactional checkpoint updates for consistency.
func Example_projectionLoop() {
	// Setup (omitted for brevity - see ExampleCheckpointStore_SaveInTx)
	store, _ := sqlite.NewEventStore(
		sqlite.WithDSN(":memory:"),
	)
	defer store.Close()

	checkpointStore, _ := sqlite.NewCheckpointStore(store.DB())

	// Create projection table
	store.DB().Exec(`CREATE TABLE accounts (account_id TEXT PRIMARY KEY, balance INTEGER)`)

	projectionName := "account-balance"

	// Load last checkpoint
	checkpoint, err := checkpointStore.Load(projectionName)
	if err != nil {
		// First run - start from beginning
		checkpoint = &eventsourcing.ProjectionCheckpoint{
			ProjectionName: projectionName,
			Position:       0,
		}
	}

	// Load events since last checkpoint
	events, _ := store.LoadAllEvents(checkpoint.Position, 100)

	// Process events in a transaction
	for _, event := range events {
		tx, err := checkpointStore.DB().Begin()
		if err != nil {
			log.Fatal(err)
		}

		// Update projection based on event
		switch event.EventType {
		case "account.Created":
			tx.Exec("INSERT INTO accounts (account_id, balance) VALUES (?, ?)",
				event.AggregateID, 0)
		case "account.Deposited":
			// Parse amount from event.Data and update balance
			tx.Exec("UPDATE accounts SET balance = balance + ? WHERE account_id = ?",
				100, event.AggregateID)
		}

		// Update checkpoint in same transaction
		checkpoint.Position = event.Version
		checkpoint.LastEventID = event.ID
		checkpoint.UpdatedAt = time.Now()

		err = checkpointStore.SaveInTx(tx, checkpoint)
		if err != nil {
			tx.Rollback()
			log.Fatal(err)
		}

		// Commit atomically
		err = tx.Commit()
		if err != nil {
			log.Fatal(err)
		}
	}

	fmt.Printf("Processed %d events\n", len(events))
	// Output: Processed 0 events
}
