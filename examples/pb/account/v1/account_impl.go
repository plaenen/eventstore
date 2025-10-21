// Developer-implemented event appliers for Account aggregate
// This file contains the business logic for how events change aggregate state

package accountv1

// ApplyAccountOpenedEvent applies the AccountOpenedEvent to the aggregate state
func (a *AccountAggregate) ApplyAccountOpenedEvent(e *AccountOpenedEvent) error {
	a.AccountId = e.AccountId
	a.OwnerName = e.OwnerName
	a.Balance = e.InitialBalance
	a.Status = AccountStatus_ACCOUNT_STATUS_OPEN
	return nil
}

// ApplyMoneyDepositedEvent applies the MoneyDepositedEvent to the aggregate state
func (a *AccountAggregate) ApplyMoneyDepositedEvent(e *MoneyDepositedEvent) error {
	a.Balance = e.NewBalance
	return nil
}

// ApplyMoneyWithdrawnEvent applies the MoneyWithdrawnEvent to the aggregate state
func (a *AccountAggregate) ApplyMoneyWithdrawnEvent(e *MoneyWithdrawnEvent) error {
	a.Balance = e.NewBalance
	return nil
}

// ApplyAccountClosedEvent applies the AccountClosedEvent to the aggregate state
func (a *AccountAggregate) ApplyAccountClosedEvent(e *AccountClosedEvent) error {
	a.Balance = e.FinalBalance
	a.Status = AccountStatus_ACCOUNT_STATUS_CLOSED
	return nil
}
