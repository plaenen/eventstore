// Package domain contains business logic and domain implementations.
// This keeps domain code completely separate from generated protocol buffer code.
package domain

import (
	accountv1 "github.com/plaenen/eventstore/examples/pb/account/v1"
)

// AccountAppliers implements the AccountEventApplier interface.
// This struct can hold any dependencies needed for applying events (e.g., validators, calculators).
type AccountAppliers struct {
	// Add dependencies here if needed
}

// NewAccountAppliers creates a new AccountAppliers instance.
func NewAccountAppliers() *AccountAppliers {
	return &AccountAppliers{}
}

// NewAccount creates a new Account aggregate with appliers already injected.
// This is the domain-level factory that handles dependency injection.
func NewAccount(id string) *accountv1.AccountAggregate {
	return accountv1.NewAccount(id, NewAccountAppliers())
}

// ApplyAccountOpenedEvent applies the AccountOpenedEvent to the aggregate state.
func (ap *AccountAppliers) ApplyAccountOpenedEvent(agg *accountv1.AccountAggregate, e *accountv1.AccountOpenedEvent) error {
	agg.AccountId = e.AccountId
	agg.OwnerName = e.OwnerName

	// Handle both old and new field names for backward compatibility
	if e.OpeningAmount != "" {
		agg.Balance = e.OpeningAmount // V2: New field
	} else {
		agg.Balance = e.InitialBalance // V1: Old field (deprecated)
	}

	// Handle new fields with defaults
	if e.Currency != "" {
		agg.Currency = e.Currency
	} else {
		agg.Currency = "USD" // Default currency
	}

	if e.CreatedAt != 0 {
		agg.CreatedAt = e.CreatedAt
	} else {
		agg.CreatedAt = e.Timestamp // Use timestamp as fallback
	}

	agg.Status = accountv1.AccountStatus_ACCOUNT_STATUS_OPEN
	return nil
}

// ApplyMoneyDepositedEvent applies the MoneyDepositedEvent to the aggregate state.
func (ap *AccountAppliers) ApplyMoneyDepositedEvent(agg *accountv1.AccountAggregate, e *accountv1.MoneyDepositedEvent) error {
	agg.Balance = e.NewBalance
	return nil
}

// ApplyMoneyWithdrawnEvent applies the MoneyWithdrawnEvent to the aggregate state.
func (ap *AccountAppliers) ApplyMoneyWithdrawnEvent(agg *accountv1.AccountAggregate, e *accountv1.MoneyWithdrawnEvent) error {
	agg.Balance = e.NewBalance
	return nil
}

// ApplyAccountClosedEvent applies the AccountClosedEvent to the aggregate state.
func (ap *AccountAppliers) ApplyAccountClosedEvent(agg *accountv1.AccountAggregate, e *accountv1.AccountClosedEvent) error {
	agg.Status = accountv1.AccountStatus_ACCOUNT_STATUS_CLOSED
	return nil
}
