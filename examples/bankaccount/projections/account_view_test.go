package projections_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/plaenen/eventsourcing/pkg/eventsourcing"
	"github.com/plaenen/eventsourcing/pkg/sqlite"
	natspkg "github.com/plaenen/eventsourcing/pkg/nats"
	accountv1 "github.com/plaenen/eventsourcing/examples/pb/account/v1"
	"github.com/plaenen/eventsourcing/examples/bankaccount/projections"
	"google.golang.org/protobuf/proto"
)

func TestAccountViewProjection_Handle(t *testing.T) {
	// Setup in-memory database
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Create table
	_, err = db.Exec(`
		CREATE TABLE account_view (
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
		t.Fatalf("failed to create table: %v", err)
	}

	projection := projections.NewAccountViewProjection(db)

	t.Run("HandleAccountOpened", func(t *testing.T) {
		event := &accountv1.AccountOpenedEvent{
			AccountId:      "acc-view-1",
			OwnerName:      "John Doe",
			InitialBalance: "1000.00",
			Timestamp:      time.Now().Unix(),
		}

		data, _ := proto.Marshal(event)

		envelope := &eventsourcing.EventEnvelope{
			Event: eventsourcing.Event{
				ID:            "evt-1",
				AggregateID:   "acc-view-1",
				AggregateType: "Account",
				EventType:     "accountv1.AccountOpenedEvent",
				Version:       1,
				Timestamp:     time.Now(),
				Data:          data,
			},
		}

		err := projection.Handle(context.Background(), envelope)
		if err != nil {
			t.Fatalf("Handle failed: %v", err)
		}

		// Verify inserted
		var ownerName, balance, status string
		err = db.QueryRow(`
			SELECT owner_name, balance, status
			FROM account_view
			WHERE account_id = ?
		`, "acc-view-1").Scan(&ownerName, &balance, &status)

		if err != nil {
			t.Fatalf("query failed: %v", err)
		}

		if ownerName != "John Doe" {
			t.Errorf("expected owner 'John Doe', got '%s'", ownerName)
		}

		if balance != "1000.00" {
			t.Errorf("expected balance '1000.00', got '%s'", balance)
		}

		if status != "OPEN" {
			t.Errorf("expected status 'OPEN', got '%s'", status)
		}
	})

	t.Run("HandleMoneyDeposited", func(t *testing.T) {
		event := &accountv1.MoneyDepositedEvent{
			AccountId:  "acc-view-1",
			Amount:     "500.00",
			NewBalance: "1500.00",
			Timestamp:  time.Now().Unix(),
		}

		data, _ := proto.Marshal(event)

		envelope := &eventsourcing.EventEnvelope{
			Event: eventsourcing.Event{
				ID:            "evt-2",
				AggregateID:   "acc-view-1",
				AggregateType: "Account",
				EventType:     "accountv1.MoneyDepositedEvent",
				Version:       2,
				Timestamp:     time.Now(),
				Data:          data,
			},
		}

		err := projection.Handle(context.Background(), envelope)
		if err != nil {
			t.Fatalf("Handle failed: %v", err)
		}

		// Verify updated
		var balance string
		err = db.QueryRow(`
			SELECT balance FROM account_view WHERE account_id = ?
		`, "acc-view-1").Scan(&balance)

		if err != nil {
			t.Fatalf("query failed: %v", err)
		}

		if balance != "1500.00" {
			t.Errorf("expected balance '1500.00', got '%s'", balance)
		}
	})

	t.Run("Reset", func(t *testing.T) {
		err := projection.Reset(context.Background())
		if err != nil {
			t.Fatalf("Reset failed: %v", err)
		}

		// Verify all data deleted
		var count int
		err = db.QueryRow(`SELECT COUNT(*) FROM account_view`).Scan(&count)
		if err != nil {
			t.Fatalf("query failed: %v", err)
		}

		if count != 0 {
			t.Errorf("expected 0 rows after reset, got %d", count)
		}
	})
}

func TestProjectionManager_Integration(t *testing.T) {
	ctx := context.Background()

	// Setup event store
	eventStore, _ := sqlite.NewEventStore(
		sqlite.WithDSN(":memory:"),
		sqlite.WithWALMode(false),
	)
	defer eventStore.Close()

	// Setup event bus
	eventBus, srv, _ := natspkg.NewEmbeddedEventBus()
	defer srv.Shutdown()
	defer eventBus.Close()

	// Setup projection manager
	checkpointStore, _ := sqlite.NewCheckpointStore(eventStore.DB())
	projectionMgr := eventsourcing.NewProjectionManager(checkpointStore, eventStore, eventBus)

	// Create projection
	projection := projections.NewAccountViewProjection(eventStore.DB())

	// Setup table
	eventStore.DB().Exec(`
		CREATE TABLE account_view (
			account_id TEXT PRIMARY KEY,
			owner_name TEXT NOT NULL,
			balance TEXT NOT NULL,
			status TEXT NOT NULL
		)
	`)

	// Register and start projection
	projectionMgr.Register(projection)
	err := projectionMgr.Start(ctx, "account_view")
	if err != nil {
		t.Fatalf("failed to start projection: %v", err)
	}
	defer projectionMgr.Stop("account_view")

	time.Sleep(100 * time.Millisecond) // Wait for subscription

	// Create and publish event
	event := &accountv1.AccountOpenedEvent{
		AccountId:      "acc-projection-test",
		OwnerName:      "Test User",
		InitialBalance: "750.00",
		Timestamp:      time.Now().Unix(),
	}

	data, _ := proto.Marshal(event)

	evt := &eventsourcing.Event{
		ID:            eventsourcing.GenerateID(),
		AggregateID:   "acc-projection-test",
		AggregateType: "Account",
		EventType:     "accountv1.AccountOpenedEvent",
		Version:       1,
		Timestamp:     time.Now(),
		Data:          data,
	}

	err = eventBus.Publish([]*eventsourcing.Event{evt})
	if err != nil {
		t.Fatalf("failed to publish event: %v", err)
	}

	// Wait for projection to process
	time.Sleep(500 * time.Millisecond)

	// Verify projection was updated
	var ownerName, balance string
	err = eventStore.DB().QueryRow(`
		SELECT owner_name, balance
		FROM account_view
		WHERE account_id = ?
	`, "acc-projection-test").Scan(&ownerName, &balance)

	if err != nil {
		t.Fatalf("query failed: %v", err)
	}

	if ownerName != "Test User" {
		t.Errorf("expected owner 'Test User', got '%s'", ownerName)
	}

	if balance != "750.00" {
		t.Errorf("expected balance '750.00', got '%s'", balance)
	}

	// Test checkpoint
	checkpoint, err := projectionMgr.GetCheckpoint("account_view")
	if err != nil {
		t.Fatalf("failed to get checkpoint: %v", err)
	}

	if checkpoint.Position != 1 {
		t.Errorf("expected position 1, got %d", checkpoint.Position)
	}
}

func TestProjectionManager_Rebuild(t *testing.T) {
	ctx := context.Background()

	// Setup
	eventStore, _ := sqlite.NewEventStore(
		sqlite.WithDSN(":memory:"),
		sqlite.WithWALMode(false),
	)
	defer eventStore.Close()

	eventBus, srv, _ := natspkg.NewEmbeddedEventBus()
	defer srv.Shutdown()
	defer eventBus.Close()

	// Populate event store with historical events
	repo := accountv1.NewAccountRepository(eventStore)

	for i := 1; i <= 5; i++ {
		account := accountv1.NewAccount(fmt.Sprintf("acc-%d", i))
		commandID := eventsourcing.GenerateID()
		account.SetCommandID(commandID)

		cmd := &accountv1.OpenAccountCommand{
			AccountId:      fmt.Sprintf("acc-%d", i),
			OwnerName:      fmt.Sprintf("User %d", i),
			InitialBalance: fmt.Sprintf("%d00.00", i*10),
		}

		metadata := eventsourcing.EventMetadata{
			CausationID: commandID,
			PrincipalID: fmt.Sprintf("user%d", i),
		}

		account.OpenAccount(ctx, cmd, metadata)
		repo.SaveWithCommand(account, commandID)
	}

	// Setup projection
	checkpointStore, _ := sqlite.NewCheckpointStore(eventStore.DB())
	projectionMgr := eventsourcing.NewProjectionManager(checkpointStore, eventStore, eventBus)

	projection := projections.NewAccountViewProjection(eventStore.DB())

	eventStore.DB().Exec(`
		CREATE TABLE account_view (
			account_id TEXT PRIMARY KEY,
			owner_name TEXT NOT NULL,
			balance TEXT NOT NULL,
			status TEXT NOT NULL
		)
	`)

	projectionMgr.Register(projection)

	// Rebuild from history
	err := projectionMgr.Rebuild(ctx, "account_view")
	if err != nil {
		t.Fatalf("Rebuild failed: %v", err)
	}

	// Verify all accounts in projection
	var count int
	err = eventStore.DB().QueryRow(`SELECT COUNT(*) FROM account_view`).Scan(&count)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}

	if count != 5 {
		t.Errorf("expected 5 accounts in projection, got %d", count)
	}

	// Verify checkpoint
	checkpoint, _ := projectionMgr.GetCheckpoint("account_view")
	if checkpoint.Position != 5 {
		t.Errorf("expected position 5, got %d", checkpoint.Position)
	}
}
