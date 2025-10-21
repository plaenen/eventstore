package handlers

import (
	"context"
	"fmt"
	"time"

	accountv1 "github.com/plaenen/eventsourcing/examples/pb/account/v1"
	"github.com/plaenen/eventsourcing/pkg/eventsourcing"
	"github.com/shopspring/decimal"
)

// AccountCommandHandler implements the AccountCommandServiceHandler interface
type AccountCommandHandler struct {
	repo *accountv1.AccountRepository
}

// NewAccountCommandHandler creates a new command handler
func NewAccountCommandHandler(repo *accountv1.AccountRepository) *AccountCommandHandler {
	return &AccountCommandHandler{
		repo: repo,
	}
}

// OpenAccount handles the OpenAccount command
func (h *AccountCommandHandler) OpenAccount(ctx context.Context, cmd *accountv1.OpenAccountCommand) (*accountv1.OpenAccountResponse, *eventsourcing.AppError) {
	// Validate command
	if cmd.AccountId == "" {
		return nil, &eventsourcing.AppError{
			Code:     "INVALID_ACCOUNT_ID",
			Message:  "Account ID is required",
			Solution: "Provide a valid account ID",
		}
	}

	if cmd.OwnerName == "" {
		return nil, &eventsourcing.AppError{
			Code:     "INVALID_OWNER_NAME",
			Message:  "Owner name is required",
			Solution: "Provide a valid owner name",
		}
	}

	// Parse initial balance
	balance, err := decimal.NewFromString(cmd.InitialBalance)
	if err != nil || balance.IsNegative() {
		return nil, &eventsourcing.AppError{
			Code:     "INVALID_BALANCE",
			Message:  "Initial balance must be a non-negative number",
			Solution: "Provide a valid balance (e.g., '100.00')",
		}
	}

	// Create new aggregate
	agg := accountv1.NewAccount(cmd.AccountId)

	// Create and emit event
	event := &accountv1.AccountOpenedEvent{
		AccountId:      cmd.AccountId,
		OwnerName:      cmd.OwnerName,
		InitialBalance: cmd.InitialBalance,
		Timestamp:      time.Now().Unix(),
	}

	if err := agg.EmitAccountOpenedEvent(event, eventsourcing.EventMetadata{}); err != nil {
		return nil, &eventsourcing.AppError{
			Code:    "EVENT_EMIT_FAILED",
			Message: fmt.Sprintf("Failed to emit event: %v", err),
		}
	}

	// Save aggregate
	if err := h.repo.Save(agg); err != nil {
		return nil, &eventsourcing.AppError{
			Code:    "SAVE_FAILED",
			Message: fmt.Sprintf("Failed to save account: %v", err),
		}
	}

	return &accountv1.OpenAccountResponse{
		AccountId: cmd.AccountId,
		Version:   agg.Version(),
	}, nil
}

// Deposit handles the Deposit command
func (h *AccountCommandHandler) Deposit(ctx context.Context, cmd *accountv1.DepositCommand) (*accountv1.DepositResponse, *eventsourcing.AppError) {
	// Validate command
	if cmd.AccountId == "" {
		return nil, &eventsourcing.AppError{
			Code:     "INVALID_ACCOUNT_ID",
			Message:  "Account ID is required",
			Solution: "Provide a valid account ID",
		}
	}

	amount, err := decimal.NewFromString(cmd.Amount)
	if err != nil || amount.LessThanOrEqual(decimal.Zero) {
		return nil, &eventsourcing.AppError{
			Code:     "INVALID_AMOUNT",
			Message:  "Amount must be a positive number",
			Solution: "Provide a valid amount (e.g., '50.00')",
		}
	}

	// Load aggregate
	agg, err := h.repo.Load(cmd.AccountId)
	if err != nil {
		return nil, &eventsourcing.AppError{
			Code:    "ACCOUNT_NOT_FOUND",
			Message: fmt.Sprintf("Account not found: %v", err),
		}
	}

	// Check account is open
	if agg.Status != accountv1.AccountStatus_ACCOUNT_STATUS_OPEN {
		return nil, &eventsourcing.AppError{
			Code:     "ACCOUNT_CLOSED",
			Message:  "Cannot deposit to a closed account",
			Solution: "Open a new account",
		}
	}

	// Calculate new balance
	currentBalance, _ := decimal.NewFromString(agg.Balance)
	newBalance := currentBalance.Add(amount)

	// Create and emit event
	event := &accountv1.MoneyDepositedEvent{
		AccountId:  cmd.AccountId,
		Amount:     cmd.Amount,
		NewBalance: newBalance.String(),
		Timestamp:  time.Now().Unix(),
	}

	if err := agg.EmitMoneyDepositedEvent(event, eventsourcing.EventMetadata{}); err != nil {
		return nil, &eventsourcing.AppError{
			Code:    "EVENT_EMIT_FAILED",
			Message: fmt.Sprintf("Failed to emit event: %v", err),
		}
	}

	// Save aggregate
	if err := h.repo.Save(agg); err != nil {
		return nil, &eventsourcing.AppError{
			Code:    "SAVE_FAILED",
			Message: fmt.Sprintf("Failed to save account: %v", err),
		}
	}

	return &accountv1.DepositResponse{
		NewBalance: newBalance.String(),
		Version:    agg.Version(),
	}, nil
}

// Withdraw handles the Withdraw command
func (h *AccountCommandHandler) Withdraw(ctx context.Context, cmd *accountv1.WithdrawCommand) (*accountv1.WithdrawResponse, *eventsourcing.AppError) {
	// Validate command
	if cmd.AccountId == "" {
		return nil, &eventsourcing.AppError{
			Code:     "INVALID_ACCOUNT_ID",
			Message:  "Account ID is required",
			Solution: "Provide a valid account ID",
		}
	}

	amount, err := decimal.NewFromString(cmd.Amount)
	if err != nil || amount.LessThanOrEqual(decimal.Zero) {
		return nil, &eventsourcing.AppError{
			Code:     "INVALID_AMOUNT",
			Message:  "Amount must be a positive number",
			Solution: "Provide a valid amount (e.g., '50.00')",
		}
	}

	// Load aggregate
	agg, err := h.repo.Load(cmd.AccountId)
	if err != nil {
		return nil, &eventsourcing.AppError{
			Code:    "ACCOUNT_NOT_FOUND",
			Message: fmt.Sprintf("Account not found: %v", err),
		}
	}

	// Check account is open
	if agg.Status != accountv1.AccountStatus_ACCOUNT_STATUS_OPEN {
		return nil, &eventsourcing.AppError{
			Code:     "ACCOUNT_CLOSED",
			Message:  "Cannot withdraw from a closed account",
			Solution: "Open a new account",
		}
	}

	// Calculate new balance
	currentBalance, _ := decimal.NewFromString(agg.Balance)
	newBalance := currentBalance.Sub(amount)

	// Check for sufficient funds
	if newBalance.IsNegative() {
		return nil, &eventsourcing.AppError{
			Code:     "INSUFFICIENT_FUNDS",
			Message:  fmt.Sprintf("Insufficient funds. Current balance: %s, Withdrawal amount: %s", currentBalance, amount),
			Solution: "Reduce withdrawal amount or deposit more funds",
		}
	}

	// Create and emit event
	event := &accountv1.MoneyWithdrawnEvent{
		AccountId:  cmd.AccountId,
		Amount:     cmd.Amount,
		NewBalance: newBalance.String(),
		Timestamp:  time.Now().Unix(),
	}

	if err := agg.EmitMoneyWithdrawnEvent(event, eventsourcing.EventMetadata{}); err != nil {
		return nil, &eventsourcing.AppError{
			Code:    "EVENT_EMIT_FAILED",
			Message: fmt.Sprintf("Failed to emit event: %v", err),
		}
	}

	// Save aggregate
	if err := h.repo.Save(agg); err != nil {
		return nil, &eventsourcing.AppError{
			Code:    "SAVE_FAILED",
			Message: fmt.Sprintf("Failed to save account: %v", err),
		}
	}

	return &accountv1.WithdrawResponse{
		NewBalance: newBalance.String(),
		Version:    agg.Version(),
	}, nil
}

// CloseAccount handles the CloseAccount command
func (h *AccountCommandHandler) CloseAccount(ctx context.Context, cmd *accountv1.CloseAccountCommand) (*accountv1.CloseAccountResponse, *eventsourcing.AppError) {
	// Validate command
	if cmd.AccountId == "" {
		return nil, &eventsourcing.AppError{
			Code:     "INVALID_ACCOUNT_ID",
			Message:  "Account ID is required",
			Solution: "Provide a valid account ID",
		}
	}

	// Load aggregate
	agg, err := h.repo.Load(cmd.AccountId)
	if err != nil {
		return nil, &eventsourcing.AppError{
			Code:    "ACCOUNT_NOT_FOUND",
			Message: fmt.Sprintf("Account not found: %v", err),
		}
	}

	// Check account is not already closed
	if agg.Status == accountv1.AccountStatus_ACCOUNT_STATUS_CLOSED {
		return nil, &eventsourcing.AppError{
			Code:     "ACCOUNT_ALREADY_CLOSED",
			Message:  "Account is already closed",
			Solution: "No action needed",
		}
	}

	// Create and emit event
	event := &accountv1.AccountClosedEvent{
		AccountId:    cmd.AccountId,
		FinalBalance: agg.Balance,
		Timestamp:    time.Now().Unix(),
	}

	if err := agg.EmitAccountClosedEvent(event, eventsourcing.EventMetadata{}); err != nil {
		return nil, &eventsourcing.AppError{
			Code:    "EVENT_EMIT_FAILED",
			Message: fmt.Sprintf("Failed to emit event: %v", err),
		}
	}

	// Save aggregate
	if err := h.repo.Save(agg); err != nil {
		return nil, &eventsourcing.AppError{
			Code:    "SAVE_FAILED",
			Message: fmt.Sprintf("Failed to save account: %v", err),
		}
	}

	return &accountv1.CloseAccountResponse{
		FinalBalance: agg.Balance,
		Version:      agg.Version(),
	}, nil
}
