package bankaccount_test

import (
	"context"
	"testing"

	"github.com/plaenen/eventstore/examples/bankaccount"
	accountv1 "github.com/plaenen/eventstore/examples/pb/account/v1"
	"github.com/plaenen/eventstore/pkg/eventsourcing"
	"github.com/plaenen/eventstore/pkg/sqlite"
)

func TestSnapshots_BasicFlow(t *testing.T) {
	ctx := context.Background()

	// Setup event store
	eventStore, err := sqlite.NewEventStore(
		sqlite.WithDSN(":memory:"),
		sqlite.WithWALMode(false),
	)
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	defer eventStore.Close()

	// Setup snapshot store
	snapshotStore := sqlite.NewSnapshotStore(eventStore.DB())

	// Create repository with snapshot support (snapshot every 3 events)
	strategy := eventsourcing.NewIntervalSnapshotStrategy(3)
	repo := bankaccount.NewAccountRepositoryWithSnapshots(
		eventStore,
		snapshotStore,
		strategy,
	)

	accountID := "snap-test-001"

	// 1. Open account (event 1)
	account := accountv1.NewAccount(accountID)
	account.SetCommandID(eventsourcing.GenerateID())

	openCmd := &accountv1.OpenAccountCommand{
		AccountId:      accountID,
		OwnerName:      "Bob Smith",
		InitialBalance: "1000.00",
	}

	metadata := eventsourcing.EventMetadata{
		CausationID:   eventsourcing.GenerateID(),
		CorrelationID: eventsourcing.GenerateID(),
		PrincipalID:   "test-user",
	}

	if err := account.OpenAccount(ctx, openCmd, metadata); err != nil {
		t.Fatalf("OpenAccount failed: %v", err)
	}

	if _, err := repo.SaveWithCommand(account, eventsourcing.GenerateID()); err != nil {
		t.Fatalf("SaveWithCommand failed: %v", err)
	}

	// 2. Deposit (event 2)
	account, err = repo.Load(accountID)
	if err != nil {
		t.Fatalf("failed to load account: %v", err)
	}

	depositCmd := &accountv1.DepositCommand{
		AccountId: accountID,
		Amount:    "500.00",
	}

	if err := account.Deposit(ctx, depositCmd, metadata); err != nil {
		t.Fatalf("Deposit failed: %v", err)
	}

	if _, err := repo.SaveWithCommand(account, eventsourcing.GenerateID()); err != nil {
		t.Fatalf("SaveWithCommand failed: %v", err)
	}

	// 3. Withdraw (event 3) - should trigger snapshot creation
	account, err = repo.Load(accountID)
	if err != nil {
		t.Fatalf("failed to load account: %v", err)
	}

	withdrawCmd := &accountv1.WithdrawCommand{
		AccountId: accountID,
		Amount:    "200.00",
	}

	if err := account.Withdraw(ctx, withdrawCmd, metadata); err != nil {
		t.Fatalf("Withdraw failed: %v", err)
	}

	if _, err := repo.SaveWithCommand(account, eventsourcing.GenerateID()); err != nil {
		t.Fatalf("SaveWithCommand failed: %v", err)
	}

	// 4. Verify snapshot was created
	snapshot, err := snapshotStore.GetLatestSnapshot(accountID)
	if err != nil {
		t.Fatalf("expected snapshot to exist, got error: %v", err)
	}

	if snapshot.Version != 3 {
		t.Errorf("expected snapshot version 3, got %d", snapshot.Version)
	}

	if snapshot.AggregateID != accountID {
		t.Errorf("expected aggregate ID %s, got %s", accountID, snapshot.AggregateID)
	}

	if snapshot.AggregateType != "Account" {
		t.Errorf("expected aggregate type 'Account', got %s", snapshot.AggregateType)
	}

	// 5. Verify snapshot metadata
	if snapshot.Metadata == nil {
		t.Fatal("expected snapshot metadata, got nil")
	}

	if snapshot.Metadata.EventCount != 3 {
		t.Errorf("expected event count 3, got %d", snapshot.Metadata.EventCount)
	}

	if snapshot.Metadata.SnapshotType != "protobuf" {
		t.Errorf("expected snapshot type 'protobuf', got %s", snapshot.Metadata.SnapshotType)
	}

	// 6. Add more events (4, 5, 6) to trigger another snapshot
	for i := 0; i < 3; i++ {
		account, err = repo.Load(accountID)
		if err != nil {
			t.Fatalf("failed to load account: %v", err)
		}

		depositCmd := &accountv1.DepositCommand{
			AccountId: accountID,
			Amount:    "100.00",
		}

		if err := account.Deposit(ctx, depositCmd, metadata); err != nil {
			t.Fatalf("Deposit failed: %v", err)
		}

		if _, err := repo.SaveWithCommand(account, eventsourcing.GenerateID()); err != nil {
			t.Fatalf("SaveWithCommand failed: %v", err)
		}
	}

	// 7. Verify new snapshot was created at version 6
	snapshot, err = snapshotStore.GetLatestSnapshot(accountID)
	if err != nil {
		t.Fatalf("expected snapshot to exist, got error: %v", err)
	}

	if snapshot.Version != 6 {
		t.Errorf("expected snapshot version 6, got %d", snapshot.Version)
	}

	// 8. Load account and verify it uses snapshot
	// The account should have correct state without replaying all 6 events
	account, err = repo.Load(accountID)
	if err != nil {
		t.Fatalf("failed to load account: %v", err)
	}

	if account.Version() != 6 {
		t.Errorf("expected account version 6, got %d", account.Version())
	}

	// Balance might be "1600" or "1600.00" depending on big.Float formatting
	if account.Balance != "1600" && account.Balance != "1600.00" {
		t.Errorf("expected balance 1600 or 1600.00, got %s", account.Balance)
	}

	if account.OwnerName != "Bob Smith" {
		t.Errorf("expected owner Bob Smith, got %s", account.OwnerName)
	}

	if account.Status != accountv1.AccountStatus_ACCOUNT_STATUS_OPEN {
		t.Errorf("expected status OPEN, got %s", account.Status)
	}
}

func TestSnapshots_LoadFromSnapshot(t *testing.T) {
	ctx := context.Background()

	// Setup
	eventStore, _ := sqlite.NewEventStore(
		sqlite.WithDSN(":memory:"),
		sqlite.WithWALMode(false),
	)
	defer eventStore.Close()

	snapshotStore := sqlite.NewSnapshotStore(eventStore.DB())
	strategy := eventsourcing.NewIntervalSnapshotStrategy(5)
	repo := bankaccount.NewAccountRepositoryWithSnapshots(
		eventStore,
		snapshotStore,
		strategy,
	)

	accountID := "snap-test-002"

	// Create account with 10 events (will create 2 snapshots at events 5 and 10)
	account := accountv1.NewAccount(accountID)
	account.SetCommandID(eventsourcing.GenerateID())

	openCmd := &accountv1.OpenAccountCommand{
		AccountId:      accountID,
		OwnerName:      "Alice",
		InitialBalance: "0.00",
	}

	metadata := eventsourcing.EventMetadata{
		CausationID:   eventsourcing.GenerateID(),
		CorrelationID: eventsourcing.GenerateID(),
		PrincipalID:   "test-user",
	}

	if err := account.OpenAccount(ctx, openCmd, metadata); err != nil {
		t.Fatalf("OpenAccount failed: %v", err)
	}

	if _, err := repo.SaveWithCommand(account, eventsourcing.GenerateID()); err != nil {
		t.Fatalf("SaveWithCommand failed: %v", err)
	}

	// Add 9 more events
	for i := 0; i < 9; i++ {
		account, err := repo.Load(accountID)
		if err != nil {
			t.Fatalf("failed to load account on iteration %d: %v", i, err)
		}

		depositCmd := &accountv1.DepositCommand{
			AccountId: accountID,
			Amount:    "100.00",
		}
		if err := account.Deposit(ctx, depositCmd, metadata); err != nil {
			t.Fatalf("Deposit failed on iteration %d: %v", i, err)
		}

		if _, err := repo.SaveWithCommand(account, eventsourcing.GenerateID()); err != nil {
			t.Fatalf("SaveWithCommand failed on iteration %d: %v", i, err)
		}
	}

	// Verify snapshot at version 10 exists
	snapshot, err := snapshotStore.GetLatestSnapshot(accountID)
	if err != nil {
		t.Fatalf("expected snapshot, got error: %v", err)
	}

	if snapshot.Version != 10 {
		t.Errorf("expected snapshot version 10, got %d", snapshot.Version)
	}

	// Load account - should use snapshot and only replay events after version 10
	loadedAccount, err := repo.Load(accountID)
	if err != nil {
		t.Fatalf("failed to load account: %v", err)
	}

	if loadedAccount.Version() != 10 {
		t.Errorf("expected version 10, got %d", loadedAccount.Version())
	}

	if loadedAccount.Balance != "900" && loadedAccount.Balance != "900.00" {
		t.Errorf("expected balance 900, got %s", loadedAccount.Balance)
	}

	// Add one more event after the snapshot
	depositCmd := &accountv1.DepositCommand{
		AccountId: accountID,
		Amount:    "50.00",
	}
	loadedAccount.Deposit(ctx, depositCmd, metadata)
	repo.SaveWithCommand(loadedAccount, eventsourcing.GenerateID())

	// Load again - should use snapshot at version 10 and replay event 11
	finalAccount, err := repo.Load(accountID)
	if err != nil {
		t.Fatalf("failed to load account: %v", err)
	}

	if finalAccount.Version() != 11 {
		t.Errorf("expected version 11, got %d", finalAccount.Version())
	}

	if finalAccount.Balance != "950" && finalAccount.Balance != "950.00" {
		t.Errorf("expected balance 950, got %s", finalAccount.Balance)
	}
}

func TestSnapshots_MultipleVersionsRetention(t *testing.T) {
	ctx := context.Background()

	// Setup
	eventStore, _ := sqlite.NewEventStore(
		sqlite.WithDSN(":memory:"),
		sqlite.WithWALMode(false),
	)
	defer eventStore.Close()

	snapshotStore := sqlite.NewSnapshotStore(eventStore.DB())
	strategy := eventsourcing.NewIntervalSnapshotStrategy(2)
	repo := bankaccount.NewAccountRepositoryWithSnapshots(
		eventStore,
		snapshotStore,
		strategy,
	)

	accountID := "snap-test-003"

	// Create account
	account := accountv1.NewAccount(accountID)
	account.SetCommandID(eventsourcing.GenerateID())

	openCmd := &accountv1.OpenAccountCommand{
		AccountId:      accountID,
		OwnerName:      "Charlie",
		InitialBalance: "1000.00",
	}

	metadata := eventsourcing.EventMetadata{
		CausationID:   eventsourcing.GenerateID(),
		CorrelationID: eventsourcing.GenerateID(),
		PrincipalID:   "test-user",
	}

	account.OpenAccount(ctx, openCmd, metadata)
	repo.SaveWithCommand(account, eventsourcing.GenerateID())

	// Add events to create multiple snapshots (at versions 2, 4, 6, 8, 10)
	for i := 0; i < 9; i++ {
		account, _ = repo.Load(accountID)
		depositCmd := &accountv1.DepositCommand{
			AccountId: accountID,
			Amount:    "100.00",
		}
		account.Deposit(ctx, depositCmd, metadata)
		repo.SaveWithCommand(account, eventsourcing.GenerateID())
	}

	// Check how many snapshots exist for this aggregate
	// The retention policy should keep last 3 versions
	// With interval=2 and retention=3*2=6, after version 10:
	// - Should delete snapshots before version 10-6=4
	// - Should keep versions 4, 6, 8, 10 (but version 2 should be deleted)

	allSnapshots, err := eventStore.DB().Query(
		"SELECT version FROM snapshots WHERE aggregate_id = ? ORDER BY version",
		accountID,
	)
	if err != nil {
		t.Fatalf("failed to query snapshots: %v", err)
	}
	defer allSnapshots.Close()

	var versions []int64
	for allSnapshots.Next() {
		var version int64
		if err := allSnapshots.Scan(&version); err != nil {
			t.Fatalf("failed to scan version: %v", err)
		}
		versions = append(versions, version)
	}

	// With cleanup, we should have at most 4 snapshots remaining
	if len(versions) > 4 {
		t.Errorf("expected at most 4 snapshots, got %d: %v", len(versions), versions)
	}

	// Latest snapshot should be at version 10
	latestSnapshot, _ := snapshotStore.GetLatestSnapshot(accountID)
	if latestSnapshot.Version != 10 {
		t.Errorf("expected latest snapshot at version 10, got %d", latestSnapshot.Version)
	}
}

func TestSnapshots_NoSnapshotFallback(t *testing.T) {
	ctx := context.Background()

	// Setup
	eventStore, _ := sqlite.NewEventStore(
		sqlite.WithDSN(":memory:"),
		sqlite.WithWALMode(false),
	)
	defer eventStore.Close()

	snapshotStore := sqlite.NewSnapshotStore(eventStore.DB())

	// High interval means no snapshots will be created for this test
	strategy := eventsourcing.NewIntervalSnapshotStrategy(100)
	repo := bankaccount.NewAccountRepositoryWithSnapshots(
		eventStore,
		snapshotStore,
		strategy,
	)

	accountID := "snap-test-004"

	// Create account without triggering snapshot
	account := accountv1.NewAccount(accountID)
	account.SetCommandID(eventsourcing.GenerateID())

	openCmd := &accountv1.OpenAccountCommand{
		AccountId:      accountID,
		OwnerName:      "David",
		InitialBalance: "500.00",
	}

	metadata := eventsourcing.EventMetadata{
		CausationID:   eventsourcing.GenerateID(),
		CorrelationID: eventsourcing.GenerateID(),
		PrincipalID:   "test-user",
	}

	account.OpenAccount(ctx, openCmd, metadata)
	repo.SaveWithCommand(account, eventsourcing.GenerateID())

	// Verify no snapshot exists
	_, err := snapshotStore.GetLatestSnapshot(accountID)
	if err != eventsourcing.ErrSnapshotNotFound {
		t.Errorf("expected ErrSnapshotNotFound, got %v", err)
	}

	// Load should still work (falls back to event replay)
	loadedAccount, err := repo.Load(accountID)
	if err != nil {
		t.Fatalf("failed to load account without snapshot: %v", err)
	}

	if loadedAccount.Balance != "500.00" {
		t.Errorf("expected balance 500.00, got %s", loadedAccount.Balance)
	}

	if loadedAccount.Version() != 1 {
		t.Errorf("expected version 1, got %d", loadedAccount.Version())
	}
}
