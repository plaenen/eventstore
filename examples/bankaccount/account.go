package bankaccount

import (
	"context"
	"fmt"
	"math/big"
	"time"

	accountv1 "github.com/plaenen/eventsourcing/examples/pb/account/v1"
	"github.com/plaenen/eventsourcing/pkg/eventsourcing"
)

type Account struct {
	accountv1.Account
}

// OpenAccount handles opening a new bank account with business validation
func (a *Account) OpenAccount(ctx context.Context, cmd *accountv1.OpenAccountCommand, metadata eventsourcing.EventMetadata) error {
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
	event := &accountv1.AccountOpenedEvent{
		AccountId:      cmd.AccountId,
		OwnerName:      cmd.OwnerName,
		InitialBalance: cmd.InitialBalance,
		Timestamp:      time.Now().Unix(),
	}

	// Use generated helper to emit event
	return a.EmitAccountOpenedEvent(event, metadata)
}

// Deposit handles depositing money into the account
func (a *Account) Deposit(ctx context.Context, cmd *accountv1.DepositCommand, metadata eventsourcing.EventMetadata) error {
	// Business validation: account must be open
	if a.Status != accountv1.AccountStatus_ACCOUNT_STATUS_OPEN {
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
	currentBalance.SetString(a.Balance)
	newBalance := new(big.Float).Add(currentBalance, amount)

	// Create event
	event := &accountv1.MoneyDepositedEvent{
		AccountId:  cmd.AccountId,
		Amount:     cmd.Amount,
		NewBalance: newBalance.String(),
		Timestamp:  time.Now().Unix(),
	}

	// Use generated helper to emit event
	return a.EmitMoneyDepositedEvent(event, metadata)
}

// Withdraw handles withdrawing money from the account
func (a *Account) Withdraw(ctx context.Context, cmd *accountv1.WithdrawCommand, metadata eventsourcing.EventMetadata) error {
	// Business validation: account must be open
	if a.Status != accountv1.AccountStatus_ACCOUNT_STATUS_OPEN {
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
	currentBalance.SetString(a.Balance)
	if currentBalance.Cmp(amount) < 0 {
		return fmt.Errorf("insufficient balance: have %s, need %s", a.Balance, cmd.Amount)
	}

	// Calculate new balance
	newBalance := new(big.Float).Sub(currentBalance, amount)

	// Create event
	event := &accountv1.MoneyWithdrawnEvent{
		AccountId:  cmd.AccountId,
		Amount:     cmd.Amount,
		NewBalance: newBalance.String(),
		Timestamp:  time.Now().Unix(),
	}

	// Use generated helper to emit event
	return a.EmitMoneyWithdrawnEvent(event, metadata)
}

// CloseAccount handles closing an existing account
func (a *Account) CloseAccount(ctx context.Context, cmd *accountv1.CloseAccountCommand, metadata eventsourcing.EventMetadata) error {
	// Business validation: account must be open
	if a.Status != accountv1.AccountStatus_ACCOUNT_STATUS_OPEN {
		return fmt.Errorf("account is not open")
	}

	// Optional: require zero balance before closing
	balance := new(big.Float)
	balance.SetString(a.Balance)
	if balance.Cmp(big.NewFloat(0)) != 0 {
		return fmt.Errorf("cannot close account with non-zero balance: %s", a.Balance)
	}

	// Create event
	event := &accountv1.AccountClosedEvent{
		AccountId:    cmd.AccountId,
		FinalBalance: a.Balance,
		Timestamp:    time.Now().Unix(),
	}

	// Use generated helper to emit event
	return a.EmitAccountClosedEvent(event, metadata)
}
