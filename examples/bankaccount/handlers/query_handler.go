package handlers

import (
	"context"
	"fmt"

	accountv1 "github.com/plaenen/eventsourcing/examples/pb/account/v1"
	"github.com/plaenen/eventsourcing/pkg/eventsourcing"
)

// AccountQueryHandler implements the AccountQueryServiceHandler interface
type AccountQueryHandler struct {
	repo *accountv1.AccountRepository
}

// NewAccountQueryHandler creates a new query handler
func NewAccountQueryHandler(repo *accountv1.AccountRepository) *AccountQueryHandler {
	return &AccountQueryHandler{
		repo: repo,
	}
}

// GetAccount handles the GetAccount query
func (h *AccountQueryHandler) GetAccount(ctx context.Context, query *accountv1.GetAccountRequest) (*accountv1.AccountView, *eventsourcing.AppError) {
	if query.AccountId == "" {
		return nil, &eventsourcing.AppError{
			Code:     "INVALID_ACCOUNT_ID",
			Message:  "Account ID is required",
			Solution: "Provide a valid account ID",
		}
	}

	// Load aggregate
	agg, err := h.repo.Load(query.AccountId)
	if err != nil {
		return nil, &eventsourcing.AppError{
			Code:    "ACCOUNT_NOT_FOUND",
			Message: fmt.Sprintf("Account not found: %v", err),
		}
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
func (h *AccountQueryHandler) ListAccounts(ctx context.Context, query *accountv1.ListAccountsRequest) (*accountv1.ListAccountsResponse, *eventsourcing.AppError) {
	// For now, return empty list (would need proper read model implementation)
	return &accountv1.ListAccountsResponse{
		Accounts:      []*accountv1.AccountView{},
		NextPageToken: "",
		TotalCount:    0,
	}, nil
}

// GetAccountBalance handles the GetAccountBalance query
func (h *AccountQueryHandler) GetAccountBalance(ctx context.Context, query *accountv1.GetAccountBalanceRequest) (*accountv1.BalanceView, *eventsourcing.AppError) {
	if query.AccountId == "" {
		return nil, &eventsourcing.AppError{
			Code:     "INVALID_ACCOUNT_ID",
			Message:  "Account ID is required",
			Solution: "Provide a valid account ID",
		}
	}

	// Load aggregate
	agg, err := h.repo.Load(query.AccountId)
	if err != nil {
		return nil, &eventsourcing.AppError{
			Code:    "ACCOUNT_NOT_FOUND",
			Message: fmt.Sprintf("Account not found: %v", err),
		}
	}

	return &accountv1.BalanceView{
		AccountId: agg.AccountId,
		Balance:   agg.Balance,
		Version:   agg.Version(),
	}, nil
}

// GetAccountHistory handles the GetAccountHistory query
func (h *AccountQueryHandler) GetAccountHistory(ctx context.Context, query *accountv1.GetAccountHistoryRequest) (*accountv1.AccountHistoryResponse, *eventsourcing.AppError) {
	// For now, return empty history (would need proper event store query)
	return &accountv1.AccountHistoryResponse{
		Transactions: []*accountv1.TransactionView{},
	}, nil
}
