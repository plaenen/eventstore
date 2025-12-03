package main

import (
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/plaenen/eventstore/pkg/eventsourcing/store/sqlite"

	domainv1 "github.com/plaenen/eventstore/examples/pb/account/domain/v1"
)

// ============================================================================
// Domain Layer (User Code)
// ============================================================================

// Account is the Domain Aggregate Root.
// The user OWNS this struct and can add methods.
type Account struct {
	*domainv1.AccountAggregateBase
}

// Ensure Account implements AccountAggregate interface
var _ domainv1.AccountAggregate = &Account{}

// AccountAppliers implements the event application logic
type AccountAppliers struct{}

func (ap *AccountAppliers) ApplyAccountOpenedEvent(agg *domainv1.AccountAggregateBase, e *domainv1.AccountOpenedEvent) error {
	agg.State.AccountId = e.AccountId
	agg.State.OwnerName = e.OwnerName
	agg.State.Balance = e.OpeningAmount
	agg.State.Status = domainv1.AccountStatus_ACCOUNT_STATUS_OPEN
	return nil
}

func (ap *AccountAppliers) ApplyMoneyDepositedEvent(agg *domainv1.AccountAggregateBase, e *domainv1.MoneyDepositedEvent) error {
	agg.State.Balance = e.NewBalance
	return nil
}

func (ap *AccountAppliers) ApplyMoneyWithdrawnEvent(agg *domainv1.AccountAggregateBase, e *domainv1.MoneyWithdrawnEvent) error {
	agg.State.Balance = e.NewBalance
	return nil
}

func (ap *AccountAppliers) ApplyAccountClosedEvent(agg *domainv1.AccountAggregateBase, e *domainv1.AccountClosedEvent) error {
	agg.State.Status = domainv1.AccountStatus_ACCOUNT_STATUS_CLOSED
	return nil
}

// Factory
func NewAccount(id string) *Account {
	applier := &AccountAppliers{}
	return &Account{
		AccountAggregateBase: domainv1.NewAccountAggregateBase(id, applier),
	}
}

// Domain Methods

func (a *Account) Open(owner string, amount string) error {
	if a.State.Status != domainv1.AccountStatus_ACCOUNT_STATUS_UNSPECIFIED {
		return errors.New("account already exists")
	}

	event := &domainv1.AccountOpenedEvent{
		AccountId:     a.ID(),
		OwnerName:     owner,
		OpeningAmount: amount,
		Timestamp:     time.Now().Unix(),
	}

	// Apply event (updates state immediately AND records for persistence)
	return a.ApplyAccountOpenedEvent(event)
}

func (a *Account) Deposit(amount string) error {
	if a.State.Status != domainv1.AccountStatus_ACCOUNT_STATUS_OPEN {
		return errors.New("account not open")
	}

	// Simplified balance calculation (string concat for demo)
	newBalance := a.State.Balance + "+" + amount

	event := &domainv1.MoneyDepositedEvent{
		AccountId:  a.ID(),
		Amount:     amount,
		NewBalance: newBalance,
		Timestamp:  time.Now().Unix(),
	}

	return a.ApplyMoneyDepositedEvent(event)
}

// ============================================================================
// Main
// ============================================================================

func main() {
	// 1. Setup Event Store (In-Memory for demo, using SQLite implementation)
	// Note: We use a temporary DB for this example
	eventStore, err := sqlite.NewEventStore(sqlite.WithDSN("file::memory:?cache=shared"))
	if err != nil {
		log.Fatalf("Failed to create event store: %v", err)
	}
	defer eventStore.Close()

	// 2. Create Repository
	repo := domainv1.NewAccountRepository[*Account](eventStore, NewAccount)

	// 3. Create and Modify Aggregate
	id := "acc-rich-1"
	account := NewAccount(id)

	fmt.Println("--- Creating Account ---")
	if err := account.Open("Alice", "100"); err != nil {
		log.Fatalf("Failed to open account: %v", err)
	}
	fmt.Printf("State after Open: ID=%s, Owner=%s, Balance=%s\n",
		account.State.AccountId, account.State.OwnerName, account.State.Balance)

	// Verify immediate state update
	if account.State.OwnerName != "Alice" {
		log.Fatalf("State not updated immediately! Expected Alice, got %s", account.State.OwnerName)
	}

	fmt.Println("--- Depositing Money ---")
	if err := account.Deposit("50"); err != nil {
		log.Fatalf("Failed to deposit: %v", err)
	}
	fmt.Printf("State after Deposit: Balance=%s\n", account.State.Balance)

	// 4. Save to Event Store
	fmt.Println("--- Saving to Event Store ---")
	if err := repo.Save(account); err != nil {
		log.Fatalf("Failed to save: %v", err)
	}

	// 5. Load from Event Store
	fmt.Println("--- Loading from Event Store ---")
	loadedAccount, err := repo.Load(id)
	if err != nil {
		log.Fatalf("Failed to load: %v", err)
	}

	fmt.Printf("Loaded State: ID=%s, Owner=%s, Balance=%s\n",
		loadedAccount.State.AccountId, loadedAccount.State.OwnerName, loadedAccount.State.Balance)

	if loadedAccount.State.Balance != "100+50" {
		log.Fatalf("Loaded state mismatch! Expected 100+50, got %s", loadedAccount.State.Balance)
	}

	fmt.Println("--- Success! Rich Domain Model Verified ---")
}
