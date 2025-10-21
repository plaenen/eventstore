package loadtest

import (
	"context"
	"testing"

	accountv1 "github.com/plaenen/eventstore/examples/pb/account/v1"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEventSourcingConsistency verifies event sourcing consistency
func TestEventSourcingConsistency(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping consistency test in short mode")
	}

	deps := SetupTestDeps(t)
	defer deps.Cleanup()

	const numOperations = 1000

	accountID := "consistency-test"
	ctx := context.Background()

	// Open account
	_, err := deps.Handler.OpenAccount(ctx, &accountv1.OpenAccountCommand{
		AccountId:      accountID,
		OwnerName:      "Consistency Test",
		InitialBalance: "10000.00",
	})
	require.Nil(t, err)

	// Track expected balance
	expectedBalance := decimal.NewFromInt(10000)

	// Perform many operations
	t.Logf("Performing %d operations...", numOperations)

	for i := 0; i < numOperations; i++ {
		if i%2 == 0 {
			_, err := deps.Handler.Deposit(ctx, &accountv1.DepositCommand{
				AccountId: accountID,
				Amount:    "5.00",
			})
			require.Nil(t, err, "Deposit failed at operation %d", i)
			expectedBalance = expectedBalance.Add(decimal.NewFromInt(5))
		} else {
			_, err := deps.Handler.Withdraw(ctx, &accountv1.WithdrawCommand{
				AccountId: accountID,
				Amount:    "3.00",
			})
			require.Nil(t, err, "Withdraw failed at operation %d", i)
			expectedBalance = expectedBalance.Sub(decimal.NewFromInt(3))
		}
	}

	// Verify by loading aggregate
	t.Log("Verifying aggregate state...")
	agg, loadErr := deps.Repo.Load(accountID)
	require.NoError(t, loadErr)

	actualBalance, _ := decimal.NewFromString(agg.Balance)
	assert.True(t, expectedBalance.Equal(actualBalance),
		"Aggregate balance mismatch: expected %s, got %s", expectedBalance, actualBalance)

	// Verify by replaying all events
	t.Log("Verifying event replay...")
	events, loadErr := deps.EventStore.LoadEvents(accountID, 0)
	require.NoError(t, loadErr)

	replayedBalance := CalculateBalanceFromEvents(events)
	assert.True(t, expectedBalance.Equal(replayedBalance),
		"Replayed balance mismatch: expected %s, got %s", expectedBalance, replayedBalance)

	// Verify event count
	expectedEvents := 1 + numOperations // 1 open + operations
	assert.Equal(t, expectedEvents, len(events),
		"Event count mismatch: expected %d, got %d", expectedEvents, len(events))

	// Verify event versions are sequential
	t.Log("Verifying event version sequence...")
	for i, event := range events {
		expectedVersion := int64(i + 1)
		assert.Equal(t, expectedVersion, event.Version,
			"Event %d has wrong version: expected %d, got %d", i, expectedVersion, event.Version)
	}

	t.Logf("✓ Consistency verified:")
	t.Logf("  - Balance: %s", actualBalance)
	t.Logf("  - Events: %d", len(events))
	t.Logf("  - All versions sequential")
}

// TestIdempotency verifies command idempotency
func TestIdempotency(t *testing.T) {
	deps := SetupTestDeps(t)
	defer deps.Cleanup()

	accountID := "idempotency-test"
	ctx := context.Background()

	// Open account
	resp1, err := deps.Handler.OpenAccount(ctx, &accountv1.OpenAccountCommand{
		AccountId:      accountID,
		OwnerName:      "Test",
		InitialBalance: "1000.00",
	})
	require.Nil(t, err)
	version1 := resp1.Version

	// Try to open again - should fail (not idempotent by design for this command)
	_, err = deps.Handler.OpenAccount(ctx, &accountv1.OpenAccountCommand{
		AccountId:      accountID,
		OwnerName:      "Test",
		InitialBalance: "1000.00",
	})
	require.NotNil(t, err, "Opening same account twice should fail")

	// Perform deposit
	resp2, err := deps.Handler.Deposit(ctx, &accountv1.DepositCommand{
		AccountId: accountID,
		Amount:    "100.00",
	})
	require.Nil(t, err)
	version2 := resp2.Version

	// Version should have incremented
	assert.Equal(t, version1+1, version2, "Version should increment")

	// Verify final state
	agg, loadErr := deps.Repo.Load(accountID)
	require.NoError(t, loadErr)
	assert.Equal(t, "1100.00", agg.Balance)

	t.Log("✓ Idempotency behavior verified")
}

// TestAggregateVersioning verifies version management
func TestAggregateVersioning(t *testing.T) {
	deps := SetupTestDeps(t)
	defer deps.Cleanup()

	accountID := "version-test"
	ctx := context.Background()

	// Open account (version 1)
	resp, err := deps.Handler.OpenAccount(ctx, &accountv1.OpenAccountCommand{
		AccountId:      accountID,
		OwnerName:      "Test",
		InitialBalance: "1000.00",
	})
	require.Nil(t, err)
	assert.Equal(t, int64(1), resp.Version)

	// Deposit (version 2)
	resp2, err := deps.Handler.Deposit(ctx, &accountv1.DepositCommand{
		AccountId: accountID,
		Amount:    "100.00",
	})
	require.Nil(t, err)
	assert.Equal(t, int64(2), resp2.Version)

	// Withdraw (version 3)
	resp3, err := deps.Handler.Withdraw(ctx, &accountv1.WithdrawCommand{
		AccountId: accountID,
		Amount:    "50.00",
	})
	require.Nil(t, err)
	assert.Equal(t, int64(3), resp3.Version)

	// Load and verify version
	agg, loadErr := deps.Repo.Load(accountID)
	require.NoError(t, loadErr)
	assert.Equal(t, int64(3), agg.Version())

	// Verify events
	events, loadErr := deps.EventStore.LoadEvents(accountID, 0)
	require.NoError(t, loadErr)
	assert.Equal(t, 3, len(events))
	assert.Equal(t, int64(1), events[0].Version)
	assert.Equal(t, int64(2), events[1].Version)
	assert.Equal(t, int64(3), events[2].Version)

	t.Log("✓ Version management verified")
}

// TestEventOrdering verifies events are ordered correctly
func TestEventOrdering(t *testing.T) {
	deps := SetupTestDeps(t)
	defer deps.Cleanup()

	accountID := "ordering-test"
	ctx := context.Background()

	// Create account
	CreateAccount(t, deps.Handler, accountID, "1000.00")

	// Perform sequential operations
	operations := []struct {
		op     string
		amount string
	}{
		{"deposit", "100.00"},
		{"withdraw", "50.00"},
		{"deposit", "200.00"},
		{"withdraw", "75.00"},
		{"deposit", "150.00"},
	}

	for _, op := range operations {
		if op.op == "deposit" {
			_, err := deps.Handler.Deposit(ctx, &accountv1.DepositCommand{
				AccountId: accountID,
				Amount:    op.amount,
			})
			require.Nil(t, err)
		} else {
			_, err := deps.Handler.Withdraw(ctx, &accountv1.WithdrawCommand{
				AccountId: accountID,
				Amount:    op.amount,
			})
			require.Nil(t, err)
		}
	}

	// Load events
	events, loadErr := deps.EventStore.LoadEvents(accountID, 0)
	require.NoError(t, loadErr)

	// Verify events are in correct order
	assert.Equal(t, "accountv1.AccountOpenedEvent", events[0].EventType)
	assert.Equal(t, "accountv1.MoneyDepositedEvent", events[1].EventType)
	assert.Equal(t, "accountv1.MoneyWithdrawnEvent", events[2].EventType)
	assert.Equal(t, "accountv1.MoneyDepositedEvent", events[3].EventType)
	assert.Equal(t, "accountv1.MoneyWithdrawnEvent", events[4].EventType)
	assert.Equal(t, "accountv1.MoneyDepositedEvent", events[5].EventType)

	// Verify timestamps are monotonic
	for i := 1; i < len(events); i++ {
		assert.GreaterOrEqual(t, events[i].Timestamp, events[i-1].Timestamp,
			"Event %d timestamp should be >= event %d", i, i-1)
	}

	t.Log("✓ Event ordering verified")
}

// TestBusinessRuleEnforcement verifies business rules are enforced
func TestBusinessRuleEnforcement(t *testing.T) {
	deps := SetupTestDeps(t)
	defer deps.Cleanup()

	accountID := "business-rules-test"
	ctx := context.Background()

	// Create account with small balance
	CreateAccount(t, deps.Handler, accountID, "100.00")

	// Try to withdraw more than balance
	_, err := deps.Handler.Withdraw(ctx, &accountv1.WithdrawCommand{
		AccountId: accountID,
		Amount:    "200.00",
	})
	require.NotNil(t, err, "Should not allow overdraft")
	assert.Equal(t, "INSUFFICIENT_FUNDS", err.Code)

	// Verify balance unchanged
	agg, loadErr := deps.Repo.Load(accountID)
	require.NoError(t, loadErr)
	assert.Equal(t, "100.00", agg.Balance)

	// Verify no event was created
	events, loadErr := deps.EventStore.LoadEvents(accountID, 0)
	require.NoError(t, loadErr)
	assert.Equal(t, 1, len(events), "Should only have AccountOpened event")

	t.Log("✓ Business rules enforced correctly")
}
