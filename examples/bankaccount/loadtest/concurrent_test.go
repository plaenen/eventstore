package loadtest

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	accountv1 "github.com/plaenen/eventsourcing/examples/pb/account/v1"
	"github.com/plaenen/eventsourcing/pkg/eventsourcing"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConcurrentCommandsSingleAggregate tests high concurrency on a single aggregate
// This is the most important test for optimistic locking
func TestConcurrentCommandsSingleAggregate(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrent test in short mode")
	}

	deps := SetupTestDeps(t)
	defer deps.Cleanup()

	const (
		numWorkers      = 50
		opsPerWorker    = 100
		depositAmount   = "10.00"
		withdrawAmount  = "5.00"
		initialBalance  = "100000.00" // Large enough to avoid insufficient funds
	)

	accountID := "concurrent-test-account"
	CreateAccount(t, deps.Handler, accountID, initialBalance)

	ctx := context.Background()
	var wg sync.WaitGroup
	metrics := NewMetrics()

	// Start concurrent workers
	t.Logf("Starting %d workers with %d operations each...", numWorkers, opsPerWorker)

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for i := 0; i < opsPerWorker; i++ {
				start := time.Now()

				var err *eventsourcing.AppError
				if i%2 == 0 {
					// Deposit
					_, err = deps.Handler.Deposit(ctx, &accountv1.DepositCommand{
						AccountId: accountID,
						Amount:    depositAmount,
					})
				} else {
					// Withdraw
					_, err = deps.Handler.Withdraw(ctx, &accountv1.WithdrawCommand{
						AccountId: accountID,
						Amount:    withdrawAmount,
					})
				}

				latency := time.Since(start)
				isBusinessError := err != nil && (err.Code == "INSUFFICIENT_FUNDS" || err.Code == "ACCOUNT_CLOSED")
				metrics.RecordOperation(err == nil, isBusinessError, latency)

				if err != nil && !isBusinessError {
					t.Errorf("Worker %d: System error on operation %d: [%s] %s",
						workerID, i, err.Code, err.Message)
				}
			}
		}(w)
	}

	wg.Wait()

	// Report metrics
	metrics.Report(t)

	// Verify final balance is correct
	expectedOps := int64(numWorkers * opsPerWorker)
	expectedDeposits := expectedOps / 2
	expectedWithdrawals := expectedOps - expectedDeposits

	deposit, _ := decimal.NewFromString(depositAmount)
	withdraw, _ := decimal.NewFromString(withdrawAmount)
	initial, _ := decimal.NewFromString(initialBalance)

	expectedBalance := initial.
		Add(deposit.Mul(decimal.NewFromInt(expectedDeposits))).
		Sub(withdraw.Mul(decimal.NewFromInt(expectedWithdrawals)))

	agg, err := deps.Repo.Load(accountID)
	require.NoError(t, err)

	actualBalance, _ := decimal.NewFromString(agg.Balance)

	assert.True(t, expectedBalance.Equal(actualBalance),
		"Balance mismatch: expected %s, got %s", expectedBalance, actualBalance)

	// Verify event count
	events, err := deps.EventStore.LoadEvents(accountID, 0)
	require.NoError(t, err)

	// Should be: 1 AccountOpened + deposits + withdrawals
	expectedEvents := 1 + int(expectedOps)
	assert.Equal(t, expectedEvents, len(events),
		"Event count mismatch: expected %d, got %d", expectedEvents, len(events))

	t.Logf("✓ Final balance verified: %s", actualBalance)
	t.Logf("✓ Event count verified: %d", len(events))
}

// TestConcurrentCommandsMultipleAggregates tests concurrency across many aggregates
func TestConcurrentCommandsMultipleAggregates(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrent test in short mode")
	}

	deps := SetupTestDeps(t)
	defer deps.Cleanup()

	const (
		numAccounts     = 100
		opsPerAccount   = 50
		concurrency     = 20
		initialBalance  = "10000.00"
	)

	// Create accounts
	t.Logf("Creating %d accounts...", numAccounts)
	accountIDs := CreateAccounts(t, deps.Handler, numAccounts, initialBalance)

	ctx := context.Background()
	metrics := NewMetrics()

	// Create work queue
	type work struct {
		accountID string
		opIndex   int
	}
	workQueue := make(chan work, numAccounts*opsPerAccount)

	// Fill work queue
	for _, accountID := range accountIDs {
		for i := 0; i < opsPerAccount; i++ {
			workQueue <- work{accountID: accountID, opIndex: i}
		}
	}
	close(workQueue)

	// Start workers
	t.Logf("Processing %d operations with %d workers...", numAccounts*opsPerAccount, concurrency)
	var wg sync.WaitGroup

	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for w := range workQueue {
				start := time.Now()

				var err *eventsourcing.AppError
				if w.opIndex%3 == 0 {
					_, err = deps.Handler.Deposit(ctx, &accountv1.DepositCommand{
						AccountId: w.accountID,
						Amount:    "100.00",
					})
				} else if w.opIndex%3 == 1 {
					_, err = deps.Handler.Withdraw(ctx, &accountv1.WithdrawCommand{
						AccountId: w.accountID,
						Amount:    "50.00",
					})
				} else {
					// Query doesn't change state
					_, err = deps.QueryHandler.GetAccountBalance(ctx, &accountv1.GetAccountBalanceRequest{
						AccountId: w.accountID,
					})
				}

				latency := time.Since(start)
				isBusinessError := err != nil && (err.Code == "INSUFFICIENT_FUNDS" || err.Code == "ACCOUNT_CLOSED")
				metrics.RecordOperation(err == nil, isBusinessError, latency)
			}
		}(w)
	}

	wg.Wait()

	// Report metrics
	metrics.Report(t)

	// Verify all accounts have consistent state
	t.Logf("Verifying consistency of %d accounts...", numAccounts)
	var verifyWg sync.WaitGroup
	errors := make(chan error, numAccounts)

	for _, accountID := range accountIDs {
		verifyWg.Add(1)
		go func(id string) {
			defer verifyWg.Done()

			agg, err := deps.Repo.Load(id)
			if err != nil {
				errors <- fmt.Errorf("failed to load account %s: %v", id, err)
				return
			}

			// Verify balance from events
			events, err := deps.EventStore.LoadEvents(id, 0)
			if err != nil {
				errors <- fmt.Errorf("failed to load events for %s: %v", id, err)
				return
			}

			expectedBalance := CalculateBalanceFromEvents(events)
			actualBalance, _ := decimal.NewFromString(agg.Balance)

			if !expectedBalance.Equal(actualBalance) {
				errors <- fmt.Errorf("balance mismatch for %s: expected %s, got %s",
					id, expectedBalance, actualBalance)
			}
		}(accountID)
	}

	verifyWg.Wait()
	close(errors)

	var allErrors []error
	for err := range errors {
		allErrors = append(allErrors, err)
	}

	require.Empty(t, allErrors, "Consistency verification failed")
	t.Logf("✓ All %d accounts verified for consistency", numAccounts)
}

// TestConcurrentCommandsWithRetries verifies retry logic under contention
func TestConcurrentCommandsWithRetries(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping retry test in short mode")
	}

	deps := SetupTestDeps(t)
	defer deps.Cleanup()

	const (
		numWorkers = 100 // High contention
		opsPerWorker = 10
		initialBalance = "100000.00"
	)

	accountID := "retry-test-account"
	CreateAccount(t, deps.Handler, accountID, initialBalance)

	ctx := context.Background()
	var wg sync.WaitGroup
	metrics := NewMetrics()

	t.Logf("Starting %d workers (high contention)...", numWorkers)

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for i := 0; i < opsPerWorker; i++ {
				start := time.Now()

				// All workers deposit the same amount
				_, err := deps.Handler.Deposit(ctx, &accountv1.DepositCommand{
					AccountId: accountID,
					Amount:    "1.00",
				})

				latency := time.Since(start)
				metrics.RecordOperation(err == nil, false, latency)

				if err != nil {
					t.Errorf("Deposit failed even with retries: [%s] %s", err.Code, err.Message)
				}
			}
		}()
	}

	wg.Wait()

	// Report metrics
	metrics.Report(t)

	// All operations should succeed with retries
	assert.Equal(t, int64(numWorkers*opsPerWorker), metrics.SuccessfulOps,
		"All operations should succeed with retry logic")

	// Verify final balance
	expectedOps := numWorkers * opsPerWorker
	initial, _ := decimal.NewFromString(initialBalance)
	expectedBalance := initial.Add(decimal.NewFromInt(int64(expectedOps)))

	VerifyAccountBalance(t, deps.Repo, accountID, expectedBalance.String())

	t.Logf("✓ All %d operations succeeded with retries", expectedOps)
}
