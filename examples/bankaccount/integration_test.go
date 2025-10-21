package bankaccount_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/plaenen/eventsourcing/pkg/eventsourcing"
	"github.com/plaenen/eventsourcing/pkg/sqlite"
	natspkg "github.com/plaenen/eventsourcing/pkg/nats"
	accountv1 "github.com/plaenen/eventsourcing/examples/pb/account/v1"
)

func TestFullStack_OpenAccount(t *testing.T) {
	// Setup
	ctx := context.Background()

	// 1. Event Store
	eventStore, err := sqlite.NewEventStore(
		sqlite.WithDSN(":memory:"),
		sqlite.WithWALMode(false),
	)
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	defer eventStore.Close()

	// 2. Event Bus (embedded NATS)
	eventBus, natsServer, err := natspkg.NewEmbeddedEventBus()
	if err != nil {
		t.Fatalf("failed to create event bus: %v", err)
	}
	defer natsServer.Shutdown()
	defer eventBus.Close()

	// 3. Repository
	repo := accountv1.NewAccountRepository(eventStore)

	// 4. Create and execute command
	account := accountv1.NewAccount("acc-123")
	commandID := eventsourcing.GenerateID()
	account.SetCommandID(commandID)

	cmd := &accountv1.OpenAccountCommand{
		AccountId:      "acc-123",
		OwnerName:      "John Doe",
		InitialBalance: "1000.00",
	}

	metadata := eventsourcing.EventMetadata{
		CausationID:   commandID,
		CorrelationID: eventsourcing.GenerateID(),
		PrincipalID:   "test-user",
	}

	err = account.OpenAccount(ctx, cmd, metadata)
	if err != nil {
		t.Fatalf("OpenAccount failed: %v", err)
	}

	// 5. Save with idempotency
	result, err := repo.SaveWithCommand(account, commandID)
	if err != nil {
		t.Fatalf("SaveWithCommand failed: %v", err)
	}

	if result.AlreadyProcessed {
		t.Error("first save should not be marked as already processed")
	}

	if len(result.Events) != 1 {
		t.Errorf("expected 1 event, got %d", len(result.Events))
	}

	// 6. Test idempotency - retry same command
	account2 := accountv1.NewAccount("acc-123")
	account2.SetCommandID(commandID)

	err = account2.OpenAccount(ctx, cmd, metadata)
	if err != nil {
		t.Fatalf("second OpenAccount failed: %v", err)
	}

	result2, err := repo.SaveWithCommand(account2, commandID)
	if err != nil {
		t.Fatalf("second SaveWithCommand failed: %v", err)
	}

	if !result2.AlreadyProcessed {
		t.Error("second save should be marked as already processed")
	}

	// 7. Load aggregate from event store
	loadedAccount, err := repo.Load("acc-123")
	if err != nil {
		t.Fatalf("failed to load account: %v", err)
	}

	if loadedAccount.ID() != "acc-123" {
		t.Errorf("expected ID 'acc-123', got '%s'", loadedAccount.ID())
	}

	if loadedAccount.OwnerName != "John Doe" {
		t.Errorf("expected owner 'John Doe', got '%s'", loadedAccount.OwnerName)
	}

	if loadedAccount.Balance != "1000.00" {
		t.Errorf("expected balance '1000.00', got '%s'", loadedAccount.Balance)
	}

	if loadedAccount.Version() != 1 {
		t.Errorf("expected version 1, got %d", loadedAccount.Version())
	}

	// 8. Publish events to event bus
	err = eventBus.Publish(result.Events)
	if err != nil {
		t.Fatalf("failed to publish events: %v", err)
	}
}

func TestFullStack_AccountLifecycle(t *testing.T) {
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

	repo := accountv1.NewAccountRepository(eventStore)

	accountID := "acc-lifecycle-test"

	// 1. Open account
	t.Run("OpenAccount", func(t *testing.T) {
		account := accountv1.NewAccount(accountID)
		commandID := eventsourcing.GenerateID()
		account.SetCommandID(commandID)

		cmd := &accountv1.OpenAccountCommand{
			AccountId:      accountID,
			OwnerName:      "Alice",
			InitialBalance: "1000.00",
		}

		metadata := eventsourcing.EventMetadata{
			CausationID:   commandID,
			CorrelationID: eventsourcing.GenerateID(),
			PrincipalID:   "alice",
		}

		err := account.OpenAccount(ctx, cmd, metadata)
		if err != nil {
			t.Fatalf("OpenAccount failed: %v", err)
		}

		_, err = repo.SaveWithCommand(account, commandID)
		if err != nil {
			t.Fatalf("Save failed: %v", err)
		}
	})

	// 2. Deposit money
	t.Run("Deposit", func(t *testing.T) {
		account, err := repo.Load(accountID)
		if err != nil {
			t.Fatalf("Load failed: %v", err)
		}

		commandID := eventsourcing.GenerateID()
		account.SetCommandID(commandID)

		cmd := &accountv1.DepositCommand{
			AccountId: accountID,
			Amount:    "500.00",
		}

		metadata := eventsourcing.EventMetadata{
			CausationID:   commandID,
			CorrelationID: eventsourcing.GenerateID(),
			PrincipalID:   "alice",
		}

		err = account.Deposit(ctx, cmd, metadata)
		if err != nil {
			t.Fatalf("Deposit failed: %v", err)
		}

		_, err = repo.SaveWithCommand(account, commandID)
		if err != nil {
			t.Fatalf("Save failed: %v", err)
		}

		// Verify new version
		if account.Version() != 2 {
			t.Errorf("expected version 2, got %d", account.Version())
		}
	})

	// 3. Withdraw money
	t.Run("Withdraw", func(t *testing.T) {
		account, err := repo.Load(accountID)
		if err != nil {
			t.Fatalf("Load failed: %v", err)
		}

		commandID := eventsourcing.GenerateID()
		account.SetCommandID(commandID)

		cmd := &accountv1.WithdrawCommand{
			AccountId: accountID,
			Amount:    "300.00",
		}

		metadata := eventsourcing.EventMetadata{
			CausationID:   commandID,
			CorrelationID: eventsourcing.GenerateID(),
			PrincipalID:   "alice",
		}

		err = account.Withdraw(ctx, cmd, metadata)
		if err != nil {
			t.Fatalf("Withdraw failed: %v", err)
		}

		_, err = repo.SaveWithCommand(account, commandID)
		if err != nil {
			t.Fatalf("Save failed: %v", err)
		}

		if account.Version() != 3 {
			t.Errorf("expected version 3, got %d", account.Version())
		}
	})

	// 4. Verify final state
	t.Run("VerifyFinalState", func(t *testing.T) {
		account, err := repo.Load(accountID)
		if err != nil {
			t.Fatalf("Load failed: %v", err)
		}

		// Balance should be: 1000 + 500 - 300 = 1200
		expectedBalance := "1200"
		if account.Balance != expectedBalance {
			t.Errorf("expected balance '%s', got '%s'", expectedBalance, account.Balance)
		}

		if account.Version() != 3 {
			t.Errorf("expected version 3, got %d", account.Version())
		}
	})
}

func TestFullStack_UniqueConstraints(t *testing.T) {
	ctx := context.Background()

	eventStore, _ := sqlite.NewEventStore(
		sqlite.WithDSN(":memory:"),
		sqlite.WithWALMode(false),
	)
	defer eventStore.Close()

	repo := accountv1.NewAccountRepository(eventStore)

	// 1. Create first account
	account1 := accountv1.NewAccount("acc-unique-1")
	commandID1 := eventsourcing.GenerateID()
	account1.SetCommandID(commandID1)

	cmd1 := &accountv1.OpenAccountCommand{
		AccountId:      "acc-unique-1",
		OwnerName:      "User 1",
		InitialBalance: "100.00",
	}

	metadata1 := eventsourcing.EventMetadata{
		CausationID:   commandID1,
		CorrelationID: eventsourcing.GenerateID(),
		PrincipalID:   "user1",
	}

	err := account1.OpenAccount(ctx, cmd1, metadata1)
	if err != nil {
		t.Fatalf("first OpenAccount failed: %v", err)
	}

	_, err = repo.SaveWithCommand(account1, commandID1)
	if err != nil {
		t.Fatalf("first save failed: %v", err)
	}

	// 2. Try to create account with same ID (should fail)
	account2 := accountv1.NewAccount("acc-unique-1")
	commandID2 := eventsourcing.GenerateID()
	account2.SetCommandID(commandID2)

	cmd2 := &accountv1.OpenAccountCommand{
		AccountId:      "acc-unique-1", // Same ID!
		OwnerName:      "User 2",
		InitialBalance: "200.00",
	}

	metadata2 := eventsourcing.EventMetadata{
		CausationID:   commandID2,
		CorrelationID: eventsourcing.GenerateID(),
		PrincipalID:   "user2",
	}

	err = account2.OpenAccount(ctx, cmd2, metadata2)
	if err != nil {
		t.Fatalf("second OpenAccount failed: %v", err)
	}

	_, err = repo.SaveWithCommand(account2, commandID2)
	if err == nil {
		t.Fatal("expected error (concurrency conflict or unique constraint violation), got nil")
	}

	// When trying to create two aggregates with the same ID concurrently,
	// we expect a concurrency conflict (version mismatch) because the first
	// save succeeds and increments the version, so the second save fails
	// the optimistic concurrency check before unique constraints are validated.
	// This is expected behavior - the concurrency check protects against
	// duplicate aggregate creation.
	if !errors.Is(err, eventsourcing.ErrConcurrencyConflict) {
		// If it's not a concurrency conflict, it might be a unique constraint error
		var constraintErr *eventsourcing.UniqueConstraintError
		if !errors.As(err, &constraintErr) {
			// Also accept database-level version conflicts which manifest as constraint errors
			if err == nil || !strings.Contains(err.Error(), "UNIQUE constraint failed: events.aggregate_id, events.version") {
				t.Errorf("expected ErrConcurrencyConflict or UniqueConstraintError, got: %v", err)
			}
		}
	}
}

func TestFullStack_ConcurrencyControl(t *testing.T) {
	ctx := context.Background()

	eventStore, _ := sqlite.NewEventStore(
		sqlite.WithDSN(":memory:"),
		sqlite.WithWALMode(false),
	)
	defer eventStore.Close()

	repo := accountv1.NewAccountRepository(eventStore)

	// 1. Create account
	account := accountv1.NewAccount("acc-concurrency")
	commandID := eventsourcing.GenerateID()
	account.SetCommandID(commandID)

	cmd := &accountv1.OpenAccountCommand{
		AccountId:      "acc-concurrency",
		OwnerName:      "Test User",
		InitialBalance: "1000.00",
	}

	metadata := eventsourcing.EventMetadata{
		CausationID:   commandID,
		CorrelationID: eventsourcing.GenerateID(),
		PrincipalID:   "test",
	}

	err := account.OpenAccount(ctx, cmd, metadata)
	if err != nil {
		t.Fatalf("OpenAccount failed: %v", err)
	}

	_, err = repo.SaveWithCommand(account, commandID)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// 2. Load aggregate in two "processes"
	account1, _ := repo.Load("acc-concurrency")
	account2, _ := repo.Load("acc-concurrency")

	// 3. Both try to deposit (concurrent modification)
	commandID1 := eventsourcing.GenerateID()
	account1.SetCommandID(commandID1)
	deposit1 := &accountv1.DepositCommand{AccountId: "acc-concurrency", Amount: "100.00"}
	account1.Deposit(ctx, deposit1, eventsourcing.EventMetadata{
		CausationID:   commandID1,
		CorrelationID: eventsourcing.GenerateID(),
		PrincipalID:   "user1",
	})

	commandID2 := eventsourcing.GenerateID()
	account2.SetCommandID(commandID2)
	deposit2 := &accountv1.DepositCommand{AccountId: "acc-concurrency", Amount: "200.00"}
	account2.Deposit(ctx, deposit2, eventsourcing.EventMetadata{
		CausationID:   commandID2,
		CorrelationID: eventsourcing.GenerateID(),
		PrincipalID:   "user2",
	})

	// 4. First save should succeed
	_, err1 := repo.SaveWithCommand(account1, commandID1)
	if err1 != nil {
		t.Fatalf("first concurrent save failed: %v", err1)
	}

	// 5. Second save should fail with concurrency conflict
	_, err2 := repo.SaveWithCommand(account2, commandID2)
	if err2 == nil {
		t.Fatal("expected concurrency conflict, got nil")
	}

	if !errors.Is(err2, eventsourcing.ErrConcurrencyConflict) {
		t.Errorf("expected ErrConcurrencyConflict, got %v", err2)
	}
}

func TestFullStack_EventBusIntegration(t *testing.T) {
	ctx := context.Background()

	eventStore, _ := sqlite.NewEventStore(
		sqlite.WithDSN(":memory:"),
		sqlite.WithWALMode(false),
	)
	defer eventStore.Close()

	eventBus, srv, _ := natspkg.NewEmbeddedEventBus()
	defer srv.Shutdown()
	defer eventBus.Close()

	repo := accountv1.NewAccountRepository(eventStore)

	// Subscribe to events
	eventsReceived := make(chan *eventsourcing.Event, 10)

	_, err := eventBus.Subscribe(eventsourcing.EventFilter{
		AggregateTypes: []string{"Account"},
	}, func(envelope *eventsourcing.EventEnvelope) error {
		eventsReceived <- &envelope.Event
		return nil
	})
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	// Give subscription time to be ready
	time.Sleep(100 * time.Millisecond)

	// Create account
	account := accountv1.NewAccount("acc-eventbus")
	commandID := eventsourcing.GenerateID()
	account.SetCommandID(commandID)

	cmd := &accountv1.OpenAccountCommand{
		AccountId:      "acc-eventbus",
		OwnerName:      "Event Bus Test",
		InitialBalance: "500.00",
	}

	metadata := eventsourcing.EventMetadata{
		CausationID:   commandID,
		CorrelationID: eventsourcing.GenerateID(),
		PrincipalID:   "test",
	}

	account.OpenAccount(ctx, cmd, metadata)
	result, _ := repo.SaveWithCommand(account, commandID)

	// Publish events
	err = eventBus.Publish(result.Events)
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	// Wait for event
	select {
	case evt := <-eventsReceived:
		if evt.AggregateID != "acc-eventbus" {
			t.Errorf("expected aggregate ID 'acc-eventbus', got '%s'", evt.AggregateID)
		}
		if evt.EventType != "accountv1.AccountOpenedEvent" {
			t.Errorf("expected AccountOpenedEvent, got '%s'", evt.EventType)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for event from event bus")
	}
}
