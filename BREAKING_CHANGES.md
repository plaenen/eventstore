# Breaking Changes - Rich Domain Model Support (v0.1.0)

This release introduces the "User-Owned Aggregate" pattern to support Rich Domain Models. This allows you to define your aggregate logic directly on your domain structs, improving encapsulation and developer experience.

## Key Changes

1.  **AggregateBase**: The generator now produces `*AggregateBase` structs (e.g., `AccountAggregateBase`) instead of concrete aggregates. These handle the plumbing (event sourcing, state management).
2.  **User-Owned Aggregate**: You define your own struct (e.g., `Account`) that embeds `AggregateBase`. You can add domain methods (`Open`, `Deposit`) directly to this struct.
3.  **Generic Repository**: The generated repository is now generic (e.g., `AccountRepository[T AccountAggregate]`), allowing it to work with your custom aggregate struct.
4.  **Immediate State Consistency**: The generated `Apply<Event>` methods now update the in-memory state immediately via `ApplyEvent` before recording the event, ensuring your aggregate state is always consistent.

## Migration Guide

If you have existing code using the generated aggregates, you will need to update it to use the new pattern.

### 1. Update Aggregate Definition

Instead of using the generated aggregate struct directly, define your own struct embedding the base:

```go
// OLD
// agg := domainv1.NewAccountAggregate(id)

// NEW
type Account struct {
	*domainv1.AccountAggregateBase
}

func NewAccount(id string) *Account {
    // You need to implement and inject your applier logic
	applier := &AccountAppliers{} 
	return &Account{
		AccountAggregateBase: domainv1.NewAccountAggregateBase(id, applier),
	}
}
```

### 2. Update Repository Usage

The repository is now generic. You need to specify your aggregate type and provide a factory function:

```go
// OLD
// repo := domainv1.NewAccountRepository(eventStore)

// NEW
repo := domainv1.NewAccountRepository[*Account](eventStore, NewAccount)
```

### 3. Update Event Application

When applying events, use the type-safe methods on your aggregate. Note that state is now accessed via the `.State` field on the base:

```go
// OLD
// agg.Balance = newBalance

// NEW
// agg.State.Balance = newBalance
```

## Example Usage

```go
// Account is the Domain Aggregate Root.
// The user OWNS this struct and can add methods.
type Account struct {
	*domainv1.AccountAggregateBase
}

// Ensure Account implements AccountAggregate interface
var _ domainv1.AccountAggregate = &Account{}

// Factory
func NewAccount(id string) *Account {
	applier := &AccountAppliers{} // Implement your applier logic
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
```
