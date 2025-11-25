package handlers

import (
	"context"
	"fmt"
	"time"

	exampledomain "github.com/plaenen/eventstore/examples/bankaccount/domain"
	accountv1 "github.com/plaenen/eventstore/examples/pb/account/v1"
	"github.com/plaenen/eventstore/pkg/eventsourcing"
	"github.com/shopspring/decimal"
)

// AccountHandler implements CQRS handler interfaces for commands and queries
type AccountHandler struct {
	repo *accountv1.AccountRepository
}

// NewAccountHandler creates a new unified account handler
func NewAccountHandler(repo *accountv1.AccountRepository) *AccountHandler {
	return &AccountHandler{
		repo: repo,
	}
}

// Ensure AccountHandler implements the CQRS interfaces
var _ accountv1.CqrsAccountCommandServiceHandler = (*AccountHandler)(nil)
var _ accountv1.CqrsAccountQueryServiceHandler = (*AccountHandler)(nil)

// ============================================================================
// Commands
// ============================================================================

// OpenAccount handles the OpenAccount command
func (h *AccountHandler) OpenAccount(ctx context.Context, cmd *accountv1.OpenAccountCommand) (*accountv1.OpenAccountResponse, error) {
	// Validate command
	if cmd.AccountId == "" {
		return nil, fmt.Errorf("account ID is required")
	}

	if cmd.OwnerName == "" {
		return nil, fmt.Errorf("owner name is required")
	}

	// Parse initial balance
	balance, err := decimal.NewFromString(cmd.InitialBalance)
	if err != nil || balance.IsNegative() {
		return nil, fmt.Errorf("initial balance must be a non-negative number")
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
	// Note: Metadata can be extracted from context if needed using standard Go context patterns
	if err := agg.ApplyAccountOpenedEvent(event,
		accountv1.WithUniqueConstraints(eventsourcing.UniqueConstraint{
			IndexName: "account_id",
			Value:     cmd.AccountId,
			Operation: eventsourcing.ConstraintClaim,
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
func (h *AccountHandler) Deposit(ctx context.Context, cmd *accountv1.DepositCommand) (*accountv1.DepositResponse, error) {
	// Validate command
	if cmd.AccountId == "" {
		return nil, fmt.Errorf("account ID is required")
	}

	amount, err := decimal.NewFromString(cmd.Amount)
	if err != nil || amount.LessThanOrEqual(decimal.Zero) {
		return nil, fmt.Errorf("amount must be a positive number")
	}

	var response *accountv1.DepositResponse

	// Retry on concurrency conflicts
	err = h.repo.RetryOnConflict(cmd.AccountId, 3, func(agg *accountv1.AccountAggregate) error {
		// Check account is open
		if agg.Status != accountv1.AccountStatus_ACCOUNT_STATUS_OPEN {
			return fmt.Errorf("cannot deposit to a closed account")
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
			return fmt.Errorf("failed to emit event: %w", err)
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
func (h *AccountHandler) Withdraw(ctx context.Context, cmd *accountv1.WithdrawCommand) (*accountv1.WithdrawResponse, error) {
	// Validate command
	if cmd.AccountId == "" {
		return nil, fmt.Errorf("account ID is required")
	}

	amount, err := decimal.NewFromString(cmd.Amount)
	if err != nil || amount.LessThanOrEqual(decimal.Zero) {
		return nil, fmt.Errorf("amount must be a positive number")
	}

	var response *accountv1.WithdrawResponse

	// Retry on concurrency conflicts
	err = h.repo.RetryOnConflict(cmd.AccountId, 3, func(agg *accountv1.AccountAggregate) error {
		// Check account is open
		if agg.Status != accountv1.AccountStatus_ACCOUNT_STATUS_OPEN {
			return fmt.Errorf("cannot withdraw from a closed account")
		}

		// Calculate new balance
		currentBalance, _ := decimal.NewFromString(agg.Balance)
		newBalance := currentBalance.Sub(amount)

		// Check for sufficient funds
		if newBalance.IsNegative() {
			return fmt.Errorf("insufficient funds: current balance %s, withdrawal amount %s", currentBalance, amount)
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
			return fmt.Errorf("failed to emit event: %w", err)
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
func (h *AccountHandler) CloseAccount(ctx context.Context, cmd *accountv1.CloseAccountCommand) (*accountv1.CloseAccountResponse, error) {
	// Validate command
	if cmd.AccountId == "" {
		return nil, fmt.Errorf("account ID is required")
	}

	// Load aggregate
	agg, err := h.repo.Load(cmd.AccountId)
	if err != nil {
		return nil, fmt.Errorf("account not found: %w", err)
	}

	// Check account is not already closed
	if agg.Status == accountv1.AccountStatus_ACCOUNT_STATUS_CLOSED {
		return nil, fmt.Errorf("account is already closed")
	}

	// Create and emit event using type-safe helper
	event := &accountv1.AccountClosedEvent{
		AccountId:    cmd.AccountId,
		FinalBalance: agg.Balance,
		Timestamp:    time.Now().Unix(),
	}

	// Use generated type-safe Apply method with constraint release
	if err := agg.ApplyAccountClosedEvent(event,
		accountv1.WithUniqueConstraints(eventsourcing.UniqueConstraint{
			IndexName: "account_id",
			Value:     cmd.AccountId,
			Operation: eventsourcing.ConstraintRelease,
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

// ============================================================================
// Queries
// ============================================================================

// GetAccount handles the GetAccount query
func (h *AccountHandler) GetAccount(ctx context.Context, query *accountv1.GetAccountRequest) (*accountv1.AccountView, error) {
	if query.AccountId == "" {
		return nil, fmt.Errorf("account ID is required")
	}

	// Load aggregate
	agg, err := h.repo.Load(query.AccountId)
	if err != nil {
		return nil, fmt.Errorf("account not found: %w", err)
	}

	// Convert to view
	return &accountv1.AccountView{
		AccountId: agg.AccountId,
		OwnerName: agg.OwnerName,
		Balance:   agg.Balance,
		Status:    agg.Status,
		Version:   agg.Version(),
	}, nil
}

// ListAccounts handles the ListAccounts query
func (h *AccountHandler) ListAccounts(ctx context.Context, query *accountv1.ListAccountsRequest) (*accountv1.ListAccountsResponse, error) {
	// For now, return empty list (would need proper read model implementation)
	return &accountv1.ListAccountsResponse{
		Accounts:      []*accountv1.AccountView{},
		NextPageToken: "",
		TotalCount:    0,
	}, nil
}

// GetAccountBalance handles the GetAccountBalance query
func (h *AccountHandler) GetAccountBalance(ctx context.Context, query *accountv1.GetAccountBalanceRequest) (*accountv1.BalanceView, error) {
	if query.AccountId == "" {
		return nil, fmt.Errorf("account ID is required")
	}

	// Load aggregate
	agg, err := h.repo.Load(query.AccountId)
	if err != nil {
		return nil, fmt.Errorf("account not found: %w", err)
	}

	return &accountv1.BalanceView{
		AccountId: agg.AccountId,
		Balance:   agg.Balance,
		Version:   agg.Version(),
	}, nil
}

// GetAccountHistory handles the GetAccountHistory query
func (h *AccountHandler) GetAccountHistory(ctx context.Context, query *accountv1.GetAccountHistoryRequest) (*accountv1.AccountHistoryResponse, error) {
	// For now, return empty history (would need proper event store query)
	return &accountv1.AccountHistoryResponse{
		Transactions: []*accountv1.TransactionView{},
	}, nil
}
