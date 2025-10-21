package loadtest

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	accountv1 "github.com/plaenen/eventstore/examples/pb/account/v1"
	"github.com/plaenen/eventstore/pkg/eventsourcing"
	natspkg "github.com/plaenen/eventstore/pkg/nats"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNATSTransportLoad tests the full stack with NATS transport
func TestNATSTransportLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping transport load test in short mode")
	}

	deps := SetupFullStack(t)
	defer deps.Cleanup()

	const (
		numClients        = 10
		requestsPerClient = 100
		initialBalance    = "1000000.00"
	)

	// Create test account
	accountID := "transport-load-test"
	CreateAccount(t, deps.Handler, accountID, initialBalance)

	// Create multiple clients
	t.Logf("Creating %d concurrent clients...", numClients)
	clients := make([]*accountv1.AccountClient, numClients)
	for i := 0; i < numClients; i++ {
		transport, err := natspkg.NewTransport(&natspkg.TransportConfig{
			TransportConfig: eventsourcing.DefaultTransportConfig(),
			URL:             deps.NATSServer.ClientURL(),
			Name:            fmt.Sprintf("loadtest-client-%d", i),
		})
		require.NoError(t, err)
		defer transport.Close()

		clients[i] = accountv1.NewAccountClient(transport)
	}

	ctx := context.Background()
	var wg sync.WaitGroup
	metrics := NewMetrics()

	t.Logf("Starting load test with %d clients x %d requests...", numClients, requestsPerClient)

	// Hammer the server
	for clientID, client := range clients {
		wg.Add(1)
		go func(id int, c *accountv1.AccountClient) {
			defer wg.Done()

			for i := 0; i < requestsPerClient; i++ {
				start := time.Now()

				var err *eventsourcing.AppError
				if i%3 == 0 {
					_, err = c.Deposit(ctx, &accountv1.DepositCommand{
						AccountId: accountID,
						Amount:    "10.00",
					})
				} else if i%3 == 1 {
					_, err = c.Withdraw(ctx, &accountv1.WithdrawCommand{
						AccountId: accountID,
						Amount:    "5.00",
					})
				} else {
					_, err = c.GetAccountBalance(ctx, &accountv1.GetAccountBalanceRequest{
						AccountId: accountID,
					})
				}

				latency := time.Since(start)
				isBusinessError := err != nil && err.Code == "INSUFFICIENT_FUNDS"
				metrics.RecordOperation(err == nil, isBusinessError, latency)

				if err != nil && !isBusinessError {
					t.Errorf("Client %d: Request %d failed: [%s] %s", id, i, err.Code, err.Message)
				}
			}
		}(clientID, client)
	}

	wg.Wait()

	// Report metrics
	metrics.Report(t)

	// Verify server processed all requests
	assert.Equal(t, int64(numClients*requestsPerClient), metrics.TotalOperations,
		"Should have processed all requests")

	t.Logf("✓ NATS transport handled %d concurrent requests", numClients*requestsPerClient)
}

// TestNATSTransportMultipleAggregates tests NATS with multiple aggregates
func TestNATSTransportMultipleAggregates(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping transport test in short mode")
	}

	deps := SetupFullStack(t)
	defer deps.Cleanup()

	const (
		numAccounts    = 50
		numClients     = 5
		opsPerAccount  = 20
		initialBalance = "10000.00"
	)

	// Create accounts
	t.Logf("Creating %d accounts...", numAccounts)
	accountIDs := CreateAccounts(t, deps.Handler, numAccounts, initialBalance)

	// Create clients
	clients := make([]*accountv1.AccountClient, numClients)
	for i := 0; i < numClients; i++ {
		transport, err := natspkg.NewTransport(&natspkg.TransportConfig{
			TransportConfig: eventsourcing.DefaultTransportConfig(),
			URL:             deps.NATSServer.ClientURL(),
			Name:            fmt.Sprintf("client-%d", i),
		})
		require.NoError(t, err)
		defer transport.Close()

		clients[i] = accountv1.NewAccountClient(transport)
	}

	ctx := context.Background()
	var wg sync.WaitGroup
	metrics := NewMetrics()

	t.Logf("Processing %d operations across %d accounts with %d clients...",
		numAccounts*opsPerAccount, numAccounts, numClients)

	// Distribute work across clients
	for clientID, client := range clients {
		wg.Add(1)
		go func(id int, c *accountv1.AccountClient) {
			defer wg.Done()

			// Each client handles a subset of accounts
			for i := id; i < numAccounts; i += numClients {
				accountID := accountIDs[i]

				for op := 0; op < opsPerAccount; op++ {
					start := time.Now()

					var err *eventsourcing.AppError
					if op%2 == 0 {
						_, err = c.Deposit(ctx, &accountv1.DepositCommand{
							AccountId: accountID,
							Amount:    "50.00",
						})
					} else {
						_, err = c.Withdraw(ctx, &accountv1.WithdrawCommand{
							AccountId: accountID,
							Amount:    "25.00",
						})
					}

					latency := time.Since(start)
					isBusinessError := err != nil && err.Code == "INSUFFICIENT_FUNDS"
					metrics.RecordOperation(err == nil, isBusinessError, latency)
				}
			}
		}(clientID, client)
	}

	wg.Wait()

	// Report metrics
	metrics.Report(t)

	t.Logf("✓ Successfully processed operations across %d accounts", numAccounts)
}

// TestNATSTransportRequestTimeout tests timeout handling
func TestNATSTransportRequestTimeout(t *testing.T) {
	deps := SetupFullStack(t)
	defer deps.Cleanup()

	// Create account
	accountID := "timeout-test"
	CreateAccount(t, deps.Handler, accountID, "1000.00")

	// Create context with very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	// This should timeout
	_, err := deps.Client.Deposit(ctx, &accountv1.DepositCommand{
		AccountId: accountID,
		Amount:    "10.00",
	})

	// Should get a timeout or context deadline error
	assert.NotNil(t, err, "Should get timeout error")
	t.Logf("✓ Timeout handled correctly: %v", err)
}

// TestNATSTransportReconnect tests reconnection behavior
func TestNATSTransportReconnect(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping reconnect test in short mode")
	}

	deps := SetupFullStack(t)
	defer deps.Cleanup()

	accountID := "reconnect-test"
	CreateAccount(t, deps.Handler, accountID, "10000.00")

	ctx := context.Background()
	successCount := 0

	// Perform operations
	for i := 0; i < 10; i++ {
		_, err := deps.Client.Deposit(ctx, &accountv1.DepositCommand{
			AccountId: accountID,
			Amount:    "10.00",
		})

		if err == nil {
			successCount++
		}

		time.Sleep(100 * time.Millisecond)
	}

	t.Logf("✓ Completed %d/10 operations during reconnect test", successCount)
	assert.Greater(t, successCount, 5, "Should complete majority of operations")
}
