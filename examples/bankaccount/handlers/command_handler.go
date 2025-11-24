package handlers

import (
	"context"
	"fmt"
	"time"

	exampledomain "github.com/plaenen/eventstore/examples/bankaccount/domain"
	accountv1 "github.com/plaenen/eventstore/examples/pb/account/v1"
	"github.com/plaenen/eventstore/pkg/domain"
	"github.com/plaenen/eventstore/pkg/protocol"
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
func (h *AccountCommandHandler) OpenAccount(ctx context.Context, cmd *accountv1.OpenAccountCommand) (*accountv1.OpenAccountResponse, error) {
	// Validate command
	if cmd.AccountId == "" {
		return nil, protocol.ErrInvalidArgument("Account ID is required")
	}

	if cmd.OwnerName == "" {
		return nil, protocol.ErrInvalidArgument("Owner name is required")
	}

	// Parse initial balance
	balance, err := decimal.NewFromString(cmd.InitialBalance)
	if err != nil || balance.IsNegative() {
		return nil, protocol.ErrInvalidArgument("Initial balance must be a non-negative number")
	}

	// Create new aggregate (appliers are injected by domain factory)
	agg := exampledomain.NewAccount(cmd.AccountId)

	// Create and emit event using type-safe helper
	event := &accountv1.AccountOpenedEvent{
		AccountId:      cmd.AccountId,
		OwnerName:      cmd.OwnerName,
		InitialBalance: cmd.InitialBalance,
		Timestamp:      time.Now().Unix(),
	}

	// Use generated type-safe Apply method with unique constraint
	if err := agg.ApplyAccountOpenedEvent(event,
		accountv1.WithUniqueConstraints(domain.UniqueConstraint{
			IndexName: "account_id",
			Value:     cmd.AccountId,
			Operation: domain.ConstraintClaim,
		}),
	); err != nil {
		return nil, fmt.Errorf("failed to emit event: %w", err)
	}

	// Save aggregate
	if err := h.repo.Save(agg); err != nil {
		return nil, fmt.Errorf("failed to save account: %w", err)
	}

	return &accountv1.OpenAccountResponse{
		AccountId: cmd.AccountId,
		Version:   agg.Version(),
	}, nil
}

// Deposit handles the Deposit command
func (h *AccountCommandHandler) Deposit(ctx context.Context, cmd *accountv1.DepositCommand) (*accountv1.DepositResponse, error) {
	// Validate command
	if cmd.AccountId == "" {
		return nil, protocol.ErrInvalidArgument("Account ID is required")
	}

	amount, err := decimal.NewFromString(cmd.Amount)
	if err != nil || amount.LessThanOrEqual(decimal.Zero) {
		return nil, protocol.ErrInvalidArgument("Amount must be a positive number")
	}

	var response *accountv1.DepositResponse

	// Retry on concurrency conflicts
	err = h.repo.RetryOnConflict(cmd.AccountId, 3, func(agg *accountv1.AccountAggregate) error {
		// Check account is open
		if agg.Status != accountv1.AccountStatus_ACCOUNT_STATUS_OPEN {
			return fmt.Errorf("ACCOUNT_CLOSED: Cannot deposit to a closed account")
		}

		// Calculate new balance
		currentBalance, _ := decimal.NewFromString(agg.Balance)
		newBalance := currentBalance.Add(amount)

		// Create and emit event using type-safe helper
		event := &accountv1.MoneyDepositedEvent{
			AccountId:  cmd.AccountId,
			Amount:     cmd.Amount,
			NewBalance: newBalance.String(),
			Timestamp:  time.Now().Unix(),
		}

		// Use generated type-safe Apply method (no constraints needed)
		if err := agg.ApplyMoneyDepositedEvent(event); err != nil {
			return fmt.Errorf("EVENT_EMIT_FAILED: Failed to emit event: %v", err)
		}

		// Save aggregate
		if err := h.repo.Save(agg); err != nil {
			return err // Return as-is for retry detection
		}

		// Store response for return
		response = &accountv1.DepositResponse{
			NewBalance: newBalance.String(),
			Version:    agg.Version(),
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("operation failed: %w", err)
	}

	return response, nil
}

// Withdraw handles the Withdraw command
func (h *AccountCommandHandler) Withdraw(ctx context.Context, cmd *accountv1.WithdrawCommand) (*accountv1.WithdrawResponse, error) {
	// Validate command
	if cmd.AccountId == "" {
		return nil, protocol.ErrInvalidArgument("Account ID is required")
	}

	amount, err := decimal.NewFromString(cmd.Amount)
	if err != nil || amount.LessThanOrEqual(decimal.Zero) {
		return nil, protocol.ErrInvalidArgument("Amount must be a positive number")
	}

	var response *accountv1.WithdrawResponse

	// Retry on concurrency conflicts
	err = h.repo.RetryOnConflict(cmd.AccountId, 3, func(agg *accountv1.AccountAggregate) error {
		// Check account is open
		if agg.Status != accountv1.AccountStatus_ACCOUNT_STATUS_OPEN {
			return fmt.Errorf("ACCOUNT_CLOSED: Cannot withdraw from a closed account")
		}

		// Calculate new balance
		currentBalance, _ := decimal.NewFromString(agg.Balance)
		newBalance := currentBalance.Sub(amount)

		// Check for sufficient funds
		if newBalance.IsNegative() {
			return fmt.Errorf("INSUFFICIENT_FUNDS: Insufficient funds. Current balance: %s, Withdrawal amount: %s", currentBalance, amount)
		}

		// Create and emit event using type-safe helper
		event := &accountv1.MoneyWithdrawnEvent{
			AccountId:  cmd.AccountId,
			Amount:     cmd.Amount,
			NewBalance: newBalance.String(),
			Timestamp:  time.Now().Unix(),
		}

		// Use generated type-safe Apply method (no constraints needed)
		if err := agg.ApplyMoneyWithdrawnEvent(event); err != nil {
			return fmt.Errorf("EVENT_EMIT_FAILED: Failed to emit event: %v", err)
		}

		// Save aggregate
		if err := h.repo.Save(agg); err != nil {
			return err // Return as-is for retry detection
		}

		// Store response for return
		response = &accountv1.WithdrawResponse{
			NewBalance: newBalance.String(),
			Version:    agg.Version(),
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("operation failed: %w", err)
	}

	return response, nil
}

// CloseAccount handles the CloseAccount command
func (h *AccountCommandHandler) CloseAccount(ctx context.Context, cmd *accountv1.CloseAccountCommand) (*accountv1.CloseAccountResponse, error) {
	// Validate command
	if cmd.AccountId == "" {
		return nil, protocol.ErrInvalidArgument("Account ID is required")
	}

	// Load aggregate
	agg, err := h.repo.Load(cmd.AccountId)
	if err != nil {
		return nil, protocol.ErrNotFound(fmt.Sprintf("Account not found: %v", err))
	}

	// Check account is not already closed
	if agg.Status == accountv1.AccountStatus_ACCOUNT_STATUS_CLOSED {
		return nil, protocol.ErrConflict("Account is already closed")
	}

	// Create and emit event using type-safe helper
	event := &accountv1.AccountClosedEvent{
		AccountId:    cmd.AccountId,
		FinalBalance: agg.Balance,
		Timestamp:    time.Now().Unix(),
	}

	// Use generated type-safe Apply method with constraint release
	if err := agg.ApplyAccountClosedEvent(event,
		accountv1.WithUniqueConstraints(domain.UniqueConstraint{
			IndexName: "account_id",
			Value:     cmd.AccountId,
			Operation: domain.ConstraintRelease,
		}),
	); err != nil {
		return nil, fmt.Errorf("failed to emit event: %w", err)
	}

	// Save aggregate
	if err := h.repo.Save(agg); err != nil {
		return nil, fmt.Errorf("failed to save account: %w", err)
	}

	return &accountv1.CloseAccountResponse{
		FinalBalance: agg.Balance,
		Version:      agg.Version(),
	}, nil
}
