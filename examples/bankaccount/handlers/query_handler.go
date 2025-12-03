package handlers

import (
	"context"
	"fmt"

	accountdomainv1 "github.com/plaenen/eventstore/examples/pb/account/domain/v1"
	accountservicev1 "github.com/plaenen/eventstore/examples/pb/account/service/v1"
	"github.com/plaenen/eventstore/pkg/protocol"
)

// AccountQueryHandler implements the AccountQueryServiceHandler interface
type AccountQueryHandler struct {
	repo *accountdomainv1.AccountRepository[*accountdomainv1.AccountAggregateBase]
}

// NewAccountQueryHandler creates a new query handler
func NewAccountQueryHandler(repo *accountdomainv1.AccountRepository[*accountdomainv1.AccountAggregateBase]) *AccountQueryHandler {
	return &AccountQueryHandler{
		repo: repo,
	}
}

// GetAccount handles the GetAccount query
func (h *AccountQueryHandler) GetAccount(ctx context.Context, query *accountservicev1.GetAccountRequest) (*accountservicev1.AccountView, error) {
	if query.AccountId == "" {
		return nil, protocol.ErrInvalidArgument("Account ID is required")
	}

	// Load aggregate
	agg, err := h.repo.Load(query.AccountId)
	if err != nil {
		return nil, protocol.ErrNotFound(fmt.Sprintf("Account not found: %v", err))
	}

	// Convert to view
	return &accountservicev1.AccountView{
		AccountId: agg.State.AccountId,
		OwnerName: agg.State.OwnerName,
		Balance:   agg.State.Balance,
		Status:    agg.State.Status,
		Version:   agg.Version(),
	}, nil
}

// ListAccounts handles the ListAccounts query
func (h *AccountQueryHandler) ListAccounts(ctx context.Context, query *accountservicev1.ListAccountsRequest) (*accountservicev1.ListAccountsResponse, error) {
	// For now, return empty list (would need proper read model implementation)
	return &accountservicev1.ListAccountsResponse{
		Accounts:      []*accountservicev1.AccountView{},
		NextPageToken: "",
		TotalCount:    0,
	}, nil
}

// GetAccountBalance handles the GetAccountBalance query
func (h *AccountQueryHandler) GetAccountBalance(ctx context.Context, query *accountservicev1.GetAccountBalanceRequest) (*accountservicev1.BalanceView, error) {
	if query.AccountId == "" {
		return nil, protocol.ErrInvalidArgument("Account ID is required")
	}

	// Load aggregate
	agg, err := h.repo.Load(query.AccountId)
	if err != nil {
		return nil, protocol.ErrNotFound(fmt.Sprintf("Account not found: %v", err))
	}

	return &accountservicev1.BalanceView{
		AccountId: agg.State.AccountId,
		Balance:   agg.State.Balance,
		Version:   agg.Version(),
	}, nil
}

// GetAccountHistory handles the GetAccountHistory query
func (h *AccountQueryHandler) GetAccountHistory(ctx context.Context, query *accountservicev1.GetAccountHistoryRequest) (*accountservicev1.AccountHistoryResponse, error) {
	// For now, return empty history (would need proper event store query)
	return &accountservicev1.AccountHistoryResponse{
		Transactions: []*accountservicev1.TransactionView{},
	}, nil
}
