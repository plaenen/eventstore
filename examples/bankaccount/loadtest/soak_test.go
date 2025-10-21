package loadtest

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	accountv1 "github.com/plaenen/eventstore/examples/pb/account/v1"
	"github.com/plaenen/eventstore/pkg/eventsourcing"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSoakShort runs a short soak test (1 minute)
func TestSoakShort(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping soak test in short mode")
	}

	runSoakTest(t, 1*time.Minute, 10, 50)
}

// TestSoakMedium runs a medium soak test (5 minutes)
// Run with: go test -v -run TestSoakMedium -timeout 10m
func TestSoakMedium(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping soak test in short mode")
	}

	// Only run if explicitly requested
	t.Skip("Soak test - run explicitly with: go test -v -run TestSoakMedium -timeout 10m")

	runSoakTest(t, 5*time.Minute, 20, 100)
}

// TestSoakLong runs a long soak test (1 hour)
// Run with: go test -v -run TestSoakLong -timeout 2h
func TestSoakLong(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping soak test in short mode")
	}

	// Only run if explicitly requested
	t.Skip("Soak test - run explicitly with: go test -v -run TestSoakLong -timeout 2h")

	runSoakTest(t, 1*time.Hour, 50, 200)
}

// runSoakTest runs a soak test for the specified duration
func runSoakTest(t *testing.T, duration time.Duration, numAccounts int, concurrency int) {
	deps := SetupTestDeps(t)
	defer deps.Cleanup()

	t.Logf("=== Starting Soak Test ===")
	t.Logf("Duration: %v", duration)
	t.Logf("Accounts: %d", numAccounts)
	t.Logf("Concurrency: %d", concurrency)

	// Create accounts
	t.Logf("Creating %d test accounts...", numAccounts)
	accountIDs := CreateAccounts(t, deps.Handler, numAccounts, "100000.00")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	metrics := NewMetrics()
	var stopFlag int32

	// Start workers
	var wg sync.WaitGroup
	t.Logf("Starting %d concurrent workers...", concurrency)

	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			opCount := 0
			for atomic.LoadInt32(&stopFlag) == 0 {
				// Pick random account
				accountID := accountIDs[opCount%numAccounts]

				// Alternate between operations
				start := time.Now()
				var err *eventsourcing.AppError

				switch opCount % 4 {
				case 0:
					_, err = deps.Handler.Deposit(ctx, &accountv1.DepositCommand{
						AccountId: accountID,
						Amount:    "10.00",
					})
				case 1:
					_, err = deps.Handler.Withdraw(ctx, &accountv1.WithdrawCommand{
						AccountId: accountID,
						Amount:    "5.00",
					})
				case 2:
					_, err = deps.QueryHandler.GetAccountBalance(ctx, &accountv1.GetAccountBalanceRequest{
						AccountId: accountID,
					})
				case 3:
					_, err = deps.Handler.Deposit(ctx, &accountv1.DepositCommand{
						AccountId: accountID,
						Amount:    "15.00",
					})
				}

				latency := time.Since(start)
				isBusinessError := err != nil && (err.Code == "INSUFFICIENT_FUNDS" || err.Code == "ACCOUNT_CLOSED")
				metrics.RecordOperation(err == nil, isBusinessError, latency)

				opCount++

				// Brief pause to avoid overwhelming the system
				time.Sleep(10 * time.Millisecond)
			}

			t.Logf("Worker %d completed %d operations", workerID, opCount)
		}(w)
	}

	// Monitor progress
	monitorTicker := time.NewTicker(10 * time.Second)
	defer monitorTicker.Stop()

	soakTimer := time.NewTimer(duration)
	defer soakTimer.Stop()

	go func() {
		for {
			select {
			case <-monitorTicker.C:
				elapsed := time.Since(metrics.StartTime)
				total := atomic.LoadInt64(&metrics.TotalOperations)
				successful := atomic.LoadInt64(&metrics.SuccessfulOps)
				failed := atomic.LoadInt64(&metrics.FailedOps)
				throughput := float64(total) / elapsed.Seconds()

				t.Logf("[%v] Operations: %d (success: %d, failed: %d) | Throughput: %.2f ops/sec",
					elapsed.Round(time.Second), total, successful, failed, throughput)

				LogResourceUsage(t)

			case <-soakTimer.C:
				t.Log("Soak test duration reached, stopping workers...")
				atomic.StoreInt32(&stopFlag, 1)
				return
			}
		}
	}()

	// Wait for timer to expire
	<-soakTimer.C

	// Wait for workers to finish
	t.Log("Waiting for workers to complete...")
	wg.Wait()

	// Final report
	t.Log("\n=== Soak Test Complete ===")
	metrics.Report(t)

	// Verify system stability
	successRate := float64(metrics.SuccessfulOps) / float64(metrics.TotalOperations) * 100
	assert.Greater(t, successRate, 95.0,
		"Success rate should remain > 95%% during soak test")

	// Verify data consistency
	t.Log("Verifying data consistency across all accounts...")
	verifyAllAccounts(t, deps, accountIDs)

	t.Logf("✓ Soak test completed successfully")
}

// TestMemoryLeak runs operations and monitors for memory leaks
func TestMemoryLeak(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory leak test in short mode")
	}

	// Only run if explicitly requested
	t.Skip("Memory leak test - run explicitly with: go test -v -run TestMemoryLeak -timeout 30m")

	deps := SetupTestDeps(t)
	defer deps.Cleanup()

	const (
		iterations   = 10
		opsPerIter   = 1000
		accountCount = 100
	)

	t.Logf("=== Memory Leak Test ===")
	t.Logf("Iterations: %d", iterations)
	t.Logf("Operations per iteration: %d", opsPerIter)

	accountIDs := CreateAccounts(t, deps.Handler, accountCount, "100000.00")
	ctx := context.Background()

	// Track memory usage across iterations
	type memSnapshot struct {
		iteration  int
		alloc      uint64
		totalAlloc uint64
		sys        uint64
		numGC      uint32
	}

	snapshots := make([]memSnapshot, 0, iterations)

	for iter := 0; iter < iterations; iter++ {
		t.Logf("\n--- Iteration %d/%d ---", iter+1, iterations)

		// Perform operations
		for op := 0; op < opsPerIter; op++ {
			accountID := accountIDs[op%accountCount]

			if op%2 == 0 {
				_, _ = deps.Handler.Deposit(ctx, &accountv1.DepositCommand{
					AccountId: accountID,
					Amount:    "10.00",
				})
			} else {
				_, _ = deps.Handler.Withdraw(ctx, &accountv1.WithdrawCommand{
					AccountId: accountID,
					Amount:    "5.00",
				})
			}
		}

		// Take memory snapshot
		var m runtime.MemStats
		runtime.ReadMemStats(&m)

		snapshot := memSnapshot{
			iteration:  iter,
			alloc:      m.Alloc,
			totalAlloc: m.TotalAlloc,
			sys:        m.Sys,
			numGC:      m.NumGC,
		}

		snapshots = append(snapshots, snapshot)

		t.Logf("Memory: Alloc=%v MB, TotalAlloc=%v MB, Sys=%v MB, NumGC=%v",
			m.Alloc/1024/1024, m.TotalAlloc/1024/1024, m.Sys/1024/1024, m.NumGC)

		// Force GC between iterations to check if memory is released
		runtime.GC()
		time.Sleep(1 * time.Second)
	}

	// Analyze memory growth
	t.Log("\n=== Memory Analysis ===")
	firstSnapshot := snapshots[0]
	lastSnapshot := snapshots[len(snapshots)-1]

	allocGrowth := float64(lastSnapshot.alloc-firstSnapshot.alloc) / float64(firstSnapshot.alloc) * 100
	sysGrowth := float64(lastSnapshot.sys-firstSnapshot.sys) / float64(firstSnapshot.sys) * 100

	t.Logf("Alloc growth: %.2f%%", allocGrowth)
	t.Logf("Sys growth: %.2f%%", sysGrowth)
	t.Logf("GC runs: %d", lastSnapshot.numGC-firstSnapshot.numGC)

	// Memory growth should be bounded
	assert.Less(t, allocGrowth, 50.0,
		"Allocated memory growth should be < 50%% (current: %.2f%%)", allocGrowth)

	t.Log("✓ No significant memory leak detected")
}

// TestLongRunningTransactions tests system with long-running operations
func TestLongRunningTransactions(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping long-running test in short mode")
	}

	deps := SetupTestDeps(t)
	defer deps.Cleanup()

	const numAccounts = 50

	accountIDs := CreateAccounts(t, deps.Handler, numAccounts, "10000.00")
	ctx := context.Background()

	t.Log("Testing system stability with mixed operation durations...")

	var wg sync.WaitGroup
	metrics := NewMetrics()

	// Simulate different operation patterns
	for i := 0; i < numAccounts; i++ {
		wg.Add(1)
		go func(accountIdx int) {
			defer wg.Done()

			accountID := accountIDs[accountIdx]

			// Different accounts have different patterns
			switch accountIdx % 3 {
			case 0:
				// Rapid operations
				for op := 0; op < 100; op++ {
					start := time.Now()
					_, err := deps.Handler.Deposit(ctx, &accountv1.DepositCommand{
						AccountId: accountID,
						Amount:    "1.00",
					})
					latency := time.Since(start)
					metrics.RecordOperation(err == nil, false, latency)
				}

			case 1:
				// Slow operations with delays
				for op := 0; op < 20; op++ {
					time.Sleep(100 * time.Millisecond)
					start := time.Now()
					_, err := deps.Handler.Deposit(ctx, &accountv1.DepositCommand{
						AccountId: accountID,
						Amount:    "5.00",
					})
					latency := time.Since(start)
					metrics.RecordOperation(err == nil, false, latency)
				}

			case 2:
				// Mixed operations
				for op := 0; op < 50; op++ {
					if op%5 == 0 {
						time.Sleep(50 * time.Millisecond)
					}

					start := time.Now()
					var err *eventsourcing.AppError
					if op%2 == 0 {
						_, err = deps.Handler.Deposit(ctx, &accountv1.DepositCommand{
							AccountId: accountID,
							Amount:    "3.00",
						})
					} else {
						_, err = deps.Handler.Withdraw(ctx, &accountv1.WithdrawCommand{
							AccountId: accountID,
							Amount:    "2.00",
						})
					}
					latency := time.Since(start)
					isBusinessError := err != nil && err.Code == "INSUFFICIENT_FUNDS"
					metrics.RecordOperation(err == nil, isBusinessError, latency)
				}
			}
		}(i)
	}

	wg.Wait()

	metrics.Report(t)

	// System should handle all patterns gracefully
	successRate := float64(metrics.SuccessfulOps) / float64(metrics.TotalOperations) * 100
	assert.Greater(t, successRate, 95.0,
		"Success rate should be > 95%% with mixed operation patterns")

	// Verify consistency
	verifyAllAccounts(t, deps, accountIDs)

	t.Log("✓ Long-running transaction test completed")
}

// verifyAllAccounts checks consistency of all accounts
func verifyAllAccounts(t *testing.T, deps *TestDeps, accountIDs []string) {
	var wg sync.WaitGroup
	errors := make(chan error, len(accountIDs))

	for _, accountID := range accountIDs {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()

			// Load aggregate
			agg, err := deps.Repo.Load(id)
			if err != nil {
				errors <- fmt.Errorf("failed to load %s: %v", id, err)
				return
			}

			// Load events
			events, err := deps.EventStore.LoadEvents(id, 0)
			if err != nil {
				errors <- fmt.Errorf("failed to load events for %s: %v", id, err)
				return
			}

			// Verify balance matches events
			expectedBalance := CalculateBalanceFromEvents(events)
			actualBalance, _ := decimal.NewFromString(agg.Balance)

			if !expectedBalance.Equal(actualBalance) {
				errors <- fmt.Errorf("balance mismatch for %s: expected %s, got %s",
					id, expectedBalance, actualBalance)
			}
		}(accountID)
	}

	wg.Wait()
	close(errors)

	var allErrors []error
	for err := range errors {
		allErrors = append(allErrors, err)
	}

	require.Empty(t, allErrors, "Consistency verification failed")
}
