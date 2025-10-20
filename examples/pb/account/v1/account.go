// Custom business logic for Account aggregate
// This file is NOT generated and contains your business rules
// Safe to edit - will never be overwritten by code generation

package accountv1

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/plaenen/eventsourcing/pkg/eventsourcing"
)

// OpenAccount handles opening a new bank account with business validation
func (a *AccountAggregate) OpenAccount(_ context.Context, cmd *OpenAccountCommand, metadata eventsourcing.EventMetadata) error {
	// Business validation
	if cmd.OwnerName == "" {
		return fmt.Errorf("owner name is required")
	}

	if cmd.AccountId == "" {
		return fmt.Errorf("account ID is required")
	}

	// Validate initial balance is non-negative
	balance := new(big.Float)
	if _, ok := balance.SetString(cmd.InitialBalance); !ok {
		return fmt.Errorf("invalid initial balance: %s", cmd.InitialBalance)
	}
	if balance.Cmp(big.NewFloat(0)) < 0 {
		return fmt.Errorf("initial balance cannot be negative")
	}

	// Create event
	event := &AccountOpenedEvent{
		AccountId:      cmd.AccountId,
		OwnerName:      cmd.OwnerName,
		InitialBalance: cmd.InitialBalance,
		Timestamp:      time.Now().Unix(),
	}

	// Use generated helper to emit event
	return a.EmitAccountOpenedEvent(event, metadata)
}

// Deposit handles depositing money into the account
func (a *AccountAggregate) Deposit(_ context.Context, cmd *DepositCommand, metadata eventsourcing.EventMetadata) error {
	// Business validation: account must be open
	if a.Status != AccountStatus_ACCOUNT_STATUS_OPEN {
		return fmt.Errorf("account is not open")
	}

	// Parse and validate deposit amount
	amount := new(big.Float)
	if _, ok := amount.SetString(cmd.Amount); !ok {
		return fmt.Errorf("invalid deposit amount: %s", cmd.Amount)
	}
	if amount.Cmp(big.NewFloat(0)) <= 0 {
		return fmt.Errorf("deposit amount must be positive")
	}

	// Calculate new balance
	currentBalance := new(big.Float)
	if _, ok := currentBalance.SetString(a.Balance); !ok {
		return fmt.Errorf("invalid current balance: %s", a.Balance)
	}
	newBalance := new(big.Float).Add(currentBalance, amount)

	// Create event
	event := &MoneyDepositedEvent{
		AccountId:  cmd.AccountId,
		Amount:     cmd.Amount,
		NewBalance: newBalance.String(),
		Timestamp:  time.Now().Unix(),
	}

	// Use generated helper to emit event
	return a.EmitMoneyDepositedEvent(event, metadata)
}

// Withdraw handles withdrawing money from the account
func (a *AccountAggregate) Withdraw(_ context.Context, cmd *WithdrawCommand, metadata eventsourcing.EventMetadata) error {
	// Business validation: account must be open
	if a.Status != AccountStatus_ACCOUNT_STATUS_OPEN {
		return fmt.Errorf("account is not open")
	}

	// Parse and validate withdrawal amount
	amount := new(big.Float)
	if _, ok := amount.SetString(cmd.Amount); !ok {
		return fmt.Errorf("invalid withdrawal amount: %s", cmd.Amount)
	}
	if amount.Cmp(big.NewFloat(0)) <= 0 {
		return fmt.Errorf("withdrawal amount must be positive")
	}

	// Check sufficient balance
	currentBalance := new(big.Float)
	if _, ok := currentBalance.SetString(a.Balance); !ok {
		return fmt.Errorf("invalid current balance: %s", a.Balance)
	}
	if currentBalance.Cmp(amount) < 0 {
		return fmt.Errorf("insufficient balance: have %s, need %s", a.Balance, cmd.Amount)
	}

	// Calculate new balance
	newBalance := new(big.Float).Sub(currentBalance, amount)

	// Create event
	event := &MoneyWithdrawnEvent{
		AccountId:  cmd.AccountId,
		Amount:     cmd.Amount,
		NewBalance: newBalance.String(),
		Timestamp:  time.Now().Unix(),
	}

	// Use generated helper to emit event
	return a.EmitMoneyWithdrawnEvent(event, metadata)
}

// CloseAccount handles closing an existing account
func (a *AccountAggregate) CloseAccount(_ context.Context, cmd *CloseAccountCommand, metadata eventsourcing.EventMetadata) error {
	// Business validation: account must be open
	if a.Status != AccountStatus_ACCOUNT_STATUS_OPEN {
		return fmt.Errorf("account is not open")
	}

	// Optional: require zero balance before closing
	balance := new(big.Float)
	if _, ok := balance.SetString(a.Balance); !ok {
		return fmt.Errorf("invalid balance: %s", a.Balance)
	}
	if balance.Cmp(big.NewFloat(0)) != 0 {
		return fmt.Errorf("cannot close account with non-zero balance: %s", a.Balance)
	}

	// Create event
	event := &AccountClosedEvent{
		AccountId:    cmd.AccountId,
		FinalBalance: a.Balance,
		Timestamp:    time.Now().Unix(),
	}

	// Use generated helper to emit event
	return a.EmitAccountClosedEvent(event, metadata)
}
