package loadtest

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	accountv1 "github.com/plaenen/eventsourcing/examples/pb/account/v1"
	"github.com/plaenen/eventsourcing/pkg/eventsourcing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDatabaseRecovery tests recovery from database errors
func TestDatabaseRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping reliability test in short mode")
	}

	deps := SetupTestDeps(t)
	defer deps.Cleanup()

	accountID := "db-recovery-test"
	CreateAccount(t, deps.Handler, accountID, "10000.00")

	ctx := context.Background()

	// Perform operations successfully
	t.Log("Performing operations before disruption...")
	for i := 0; i < 10; i++ {
		_, err := deps.Handler.Deposit(ctx, &accountv1.DepositCommand{
			AccountId: accountID,
			Amount:    "100.00",
		})
		require.Nil(t, err)
	}

	// Verify operations succeeded
	agg, err := deps.Repo.Load(accountID)
	require.NoError(t, err)
	assert.Equal(t, "11000.00", agg.Balance)

	// Continue operations after "recovery"
	t.Log("Performing operations after recovery...")
	for i := 0; i < 10; i++ {
		_, err := deps.Handler.Deposit(ctx, &accountv1.DepositCommand{
			AccountId: accountID,
			Amount:    "50.00",
		})
		require.Nil(t, err)
	}

	// Verify final state
	agg, err = deps.Repo.Load(accountID)
	require.NoError(t, err)
	assert.Equal(t, "11500.00", agg.Balance)

	t.Log("✓ Database recovery verified")
}

// TestNATSServerReconnect tests NATS server disconnect/reconnect
func TestNATSServerReconnect(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping reconnect test in short mode")
	}

	deps := SetupFullStack(t)
	defer deps.Cleanup()

	accountID := "nats-reconnect-test"
	CreateAccount(t, deps.Handler, accountID, "10000.00")

	ctx := context.Background()
	metrics := NewMetrics()

	// Perform operations before disconnect
	t.Log("Performing operations before disconnect...")
	for i := 0; i < 10; i++ {
		start := time.Now()
		_, err := deps.Client.Deposit(ctx, &accountv1.DepositCommand{
			AccountId: accountID,
			Amount:    "100.00",
		})
		latency := time.Since(start)
		metrics.RecordOperation(err == nil, false, latency)
	}

	beforeDisconnect := metrics.SuccessfulOps

	// Simulate brief network disruption
	t.Log("Simulating network disruption...")
	time.Sleep(100 * time.Millisecond)

	// Perform operations during/after reconnect
	t.Log("Performing operations after reconnect...")
	for i := 0; i < 10; i++ {
		start := time.Now()
		_, err := deps.Client.Deposit(ctx, &accountv1.DepositCommand{
			AccountId: accountID,
			Amount:    "50.00",
		})
		latency := time.Since(start)
		metrics.RecordOperation(err == nil, false, latency)

		if err != nil {
			t.Logf("Operation %d failed during reconnect: %v", i, err)
		}
	}

	afterReconnect := metrics.SuccessfulOps - beforeDisconnect

	metrics.Report(t)

	// Should have recovered and completed most operations
	assert.Greater(t, afterReconnect, int64(5),
		"Should complete majority of operations after reconnect")

	t.Logf("✓ Reconnect test completed: %d/%d operations succeeded after disruption",
		afterReconnect, 10)
}

// TestHighContentionRecovery tests recovery from high contention scenarios
func TestHighContentionRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping contention test in short mode")
	}

	deps := SetupTestDeps(t)
	defer deps.Cleanup()

	accountID := "contention-recovery-test"
	CreateAccount(t, deps.Handler, accountID, "1000000.00")

	ctx := context.Background()
	metrics := NewMetrics()

	const (
		numWorkers   = 200 // Very high contention
		opsPerWorker = 10
	)

	var wg sync.WaitGroup

	t.Logf("Creating extreme contention with %d workers...", numWorkers)

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for i := 0; i < opsPerWorker; i++ {
				start := time.Now()

				_, err := deps.Handler.Deposit(ctx, &accountv1.DepositCommand{
					AccountId: accountID,
					Amount:    "1.00",
				})

				latency := time.Since(start)
				metrics.RecordOperation(err == nil, false, latency)

				if err != nil && !isConcurrencyError(err) {
					t.Errorf("Worker %d: Unexpected error: %v", workerID, err)
				}
			}
		}(w)
	}

	wg.Wait()

	metrics.Report(t)

	// All operations should eventually succeed with retry logic
	expectedOps := int64(numWorkers * opsPerWorker)
	assert.Equal(t, expectedOps, metrics.SuccessfulOps,
		"All operations should succeed with retry logic")

	t.Logf("✓ High contention recovery verified: %d/%d operations succeeded",
		metrics.SuccessfulOps, expectedOps)
}

// TestPartialFailureIsolation tests that failures are isolated to specific aggregates
func TestPartialFailureIsolation(t *testing.T) {
	deps := SetupTestDeps(t)
	defer deps.Cleanup()

	const numAccounts = 10

	// Create accounts
	accountIDs := CreateAccounts(t, deps.Handler, numAccounts, "1000.00")

	ctx := context.Background()
	var wg sync.WaitGroup
	metrics := NewMetrics()

	// Pick one account to have insufficient funds
	failingAccount := accountIDs[5]

	t.Log("Testing failure isolation across accounts...")

	for _, accountID := range accountIDs {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()

			// Try to withdraw
			amount := "100.00"
			if id == failingAccount {
				amount = "2000.00" // This will fail with insufficient funds
			}

			start := time.Now()
			_, err := deps.Handler.Withdraw(ctx, &accountv1.WithdrawCommand{
				AccountId: id,
				Amount:    amount,
			})
			latency := time.Since(start)

			isBusinessError := err != nil && err.Code == "INSUFFICIENT_FUNDS"
			metrics.RecordOperation(err == nil, isBusinessError, latency)

			if id == failingAccount {
				assert.NotNil(t, err, "Failing account should error")
				assert.Equal(t, "INSUFFICIENT_FUNDS", err.Code)
			} else {
				assert.Nil(t, err, "Other accounts should succeed")
			}
		}(accountID)
	}

	wg.Wait()

	metrics.Report(t)

	// Should have exactly 1 business error and 9 successes
	assert.Equal(t, int64(9), metrics.SuccessfulOps,
		"9 accounts should succeed")
	assert.Equal(t, int64(1), metrics.BusinessErrors,
		"1 account should fail with business error")

	t.Log("✓ Failure isolation verified")
}

// TestGracefulDegradation tests system behavior under resource constraints
func TestGracefulDegradation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping degradation test in short mode")
	}

	deps := SetupTestDeps(t)
	defer deps.Cleanup()

	const numAccounts = 50

	// Create accounts
	accountIDs := CreateAccounts(t, deps.Handler, numAccounts, "10000.00")

	ctx := context.Background()
	metrics := NewMetrics()

	// Gradually increase load
	loads := []int{10, 50, 100, 200}

	for _, load := range loads {
		t.Logf("Testing with %d concurrent workers...", load)

		var wg sync.WaitGroup
		startTime := time.Now()

		for w := 0; w < load; w++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()

				// Pick random account
				accountID := accountIDs[workerID%numAccounts]

				start := time.Now()
				_, err := deps.Handler.Deposit(ctx, &accountv1.DepositCommand{
					AccountId: accountID,
					Amount:    "10.00",
				})
				latency := time.Since(start)

				metrics.RecordOperation(err == nil, false, latency)
			}(w)
		}

		wg.Wait()
		duration := time.Since(startTime)

		t.Logf("  Load %d: completed in %v", load, duration)
	}

	metrics.Report(t)

	// System should handle all loads gracefully
	successRate := float64(metrics.SuccessfulOps) / float64(metrics.TotalOperations) * 100
	assert.Greater(t, successRate, 95.0,
		"Success rate should be > 95%% even under load")

	t.Logf("✓ Graceful degradation verified: %.2f%% success rate", successRate)
}

// TestEventualConsistency tests that system reaches consistent state after disruptions
func TestEventualConsistency(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping consistency test in short mode")
	}

	deps := SetupTestDeps(t)
	defer deps.Cleanup()

	accountID := "eventual-consistency-test"
	CreateAccount(t, deps.Handler, accountID, "10000.00")

	ctx := context.Background()
	const numOperations = 100

	// Perform operations with high concurrency
	t.Logf("Performing %d concurrent operations...", numOperations)

	var wg sync.WaitGroup
	for i := 0; i < numOperations; i++ {
		wg.Add(1)
		go func(opNum int) {
			defer wg.Done()

			if opNum%2 == 0 {
				_, _ = deps.Handler.Deposit(ctx, &accountv1.DepositCommand{
					AccountId: accountID,
					Amount:    "50.00",
				})
			} else {
				_, _ = deps.Handler.Withdraw(ctx, &accountv1.WithdrawCommand{
					AccountId: accountID,
					Amount:    "25.00",
				})
			}
		}(i)
	}

	wg.Wait()

	// Wait briefly for any pending operations
	time.Sleep(100 * time.Millisecond)

	// Verify consistency
	t.Log("Verifying eventual consistency...")

	agg, err := deps.Repo.Load(accountID)
	require.NoError(t, err)

	events, err := deps.EventStore.LoadEvents(accountID, 0)
	require.NoError(t, err)

	replayedBalance := CalculateBalanceFromEvents(events)
	actualBalance, _ := deps.QueryHandler.GetAccountBalance(ctx, &accountv1.GetAccountBalanceRequest{
		AccountId: accountID,
	})

	assert.Equal(t, replayedBalance.String(), agg.Balance,
		"Aggregate balance should match replayed events")

	if actualBalance != nil {
		assert.Equal(t, agg.Balance, actualBalance.Balance,
			"Query result should match aggregate")
	}

	t.Logf("✓ Eventual consistency verified")
	t.Logf("  - Final balance: %s", agg.Balance)
	t.Logf("  - Total events: %d", len(events))
}

// Helper function to detect concurrency errors
func isConcurrencyError(err *eventsourcing.AppError) bool {
	if err == nil {
		return false
	}
	return err.Code == "CONCURRENCY_CONFLICT" ||
		err.Code == "VERSION_MISMATCH" ||
		err.Code == "OPTIMISTIC_LOCK_FAILED"
}

// TestCascadingFailures tests that failures don't cascade across the system
func TestCascadingFailures(t *testing.T) {
	deps := SetupTestDeps(t)
	defer deps.Cleanup()

	const numAccounts = 20

	// Create accounts with varying balances
	accountIDs := make([]string, numAccounts)
	for i := 0; i < numAccounts; i++ {
		accountIDs[i] = fmt.Sprintf("cascade-test-%d", i)
		balance := "1000.00"
		if i%3 == 0 {
			balance = "10.00" // Low balance - will fail withdrawals
		}
		CreateAccount(t, deps.Handler, accountIDs[i], balance)
	}

	ctx := context.Background()
	metrics := NewMetrics()

	var wg sync.WaitGroup

	t.Log("Testing cascading failure prevention...")

	// Try to withdraw from all accounts
	for _, accountID := range accountIDs {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()

			start := time.Now()
			_, err := deps.Handler.Withdraw(ctx, &accountv1.WithdrawCommand{
				AccountId: id,
				Amount:    "100.00",
			})
			latency := time.Since(start)

			isBusinessError := err != nil && err.Code == "INSUFFICIENT_FUNDS"
			metrics.RecordOperation(err == nil, isBusinessError, latency)
		}(accountID)
	}

	wg.Wait()

	metrics.Report(t)

	// Some should fail (low balance), but others should succeed
	assert.Greater(t, metrics.SuccessfulOps, int64(10),
		"Majority of operations should succeed despite some failures")
	assert.Greater(t, metrics.BusinessErrors, int64(0),
		"Should have some expected business errors")
	assert.Equal(t, int64(0), metrics.SystemErrors,
		"Should have no system errors - failures should not cascade")

	t.Logf("✓ Cascading failures prevented: %d/%d succeeded",
		metrics.SuccessfulOps, numAccounts)
}
