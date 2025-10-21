package loadtest

import (
	"context"
	"fmt"
	"testing"

	accountv1 "github.com/plaenen/eventsourcing/examples/pb/account/v1"
)

// BenchmarkCommandProcessing benchmarks command processing performance
func BenchmarkCommandProcessing(b *testing.B) {
	deps := SetupTestDeps(b)
	defer deps.Cleanup()

	accountID := "bench-account"
	CreateAccount(b, deps.Handler, accountID, "1000000.00")

	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = deps.Handler.Deposit(ctx, &accountv1.DepositCommand{
			AccountId: accountID,
			Amount:    "1.00",
		})
	}
}

// BenchmarkCommandProcessingParallel benchmarks parallel command processing
func BenchmarkCommandProcessingParallel(b *testing.B) {
	deps := SetupTestDeps(b)
	defer deps.Cleanup()

	accountID := "bench-parallel"
	CreateAccount(b, deps.Handler, accountID, "10000000.00")

	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = deps.Handler.Deposit(ctx, &accountv1.DepositCommand{
				AccountId: accountID,
				Amount:    "1.00",
			})
		}
	})
}

// BenchmarkEventReplay benchmarks aggregate reconstruction from events
func BenchmarkEventReplay(b *testing.B) {
	deps := SetupTestDeps(b)
	defer deps.Cleanup()

	// Create account with many events
	accountID := "bench-replay"
	CreateAccount(b, deps.Handler, accountID, "1000.00")

	ctx := context.Background()

	// Create 1000 events
	for i := 0; i < 1000; i++ {
		_, _ = deps.Handler.Deposit(ctx, &accountv1.DepositCommand{
			AccountId: accountID,
			Amount:    "1.00",
		})
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = deps.Repo.Load(accountID)
	}
}

// BenchmarkEventReplayVaryingSize benchmarks replay with different event counts
func BenchmarkEventReplayVaryingSize(b *testing.B) {
	sizes := []int{10, 100, 1000, 10000}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("Events_%d", size), func(b *testing.B) {
			deps := SetupTestDeps(b)
			defer deps.Cleanup()

			accountID := fmt.Sprintf("bench-replay-%d", size)
			CreateAccount(b, deps.Handler, accountID, "1000000.00")

			ctx := context.Background()

			// Create N events
			for i := 0; i < size; i++ {
				_, _ = deps.Handler.Deposit(ctx, &accountv1.DepositCommand{
					AccountId: accountID,
					Amount:    "1.00",
				})
			}

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				_, _ = deps.Repo.Load(accountID)
			}
		})
	}
}

// BenchmarkRepositorySave benchmarks the save operation
func BenchmarkRepositorySave(b *testing.B) {
	deps := SetupTestDeps(b)
	defer deps.Cleanup()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		accountID := fmt.Sprintf("bench-save-%d", i)
		CreateAccount(b, deps.Handler, accountID, "1000.00")
	}
}

// BenchmarkNATSTransport benchmarks full stack with NATS
func BenchmarkNATSTransport(b *testing.B) {
	deps := SetupFullStack(b)
	defer deps.Cleanup()

	accountID := "bench-nats"
	CreateAccount(b, deps.Handler, accountID, "1000000.00")

	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = deps.Client.Deposit(ctx, &accountv1.DepositCommand{
			AccountId: accountID,
			Amount:    "1.00",
		})
	}
}

// BenchmarkNATSTransportParallel benchmarks parallel NATS requests
func BenchmarkNATSTransportParallel(b *testing.B) {
	deps := SetupFullStack(b)
	defer deps.Cleanup()

	accountID := "bench-nats-parallel"
	CreateAccount(b, deps.Handler, accountID, "10000000.00")

	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = deps.Client.Deposit(ctx, &accountv1.DepositCommand{
				AccountId: accountID,
				Amount:    "1.00",
			})
		}
	})
}

// BenchmarkQueryOperation benchmarks query performance
func BenchmarkQueryOperation(b *testing.B) {
	deps := SetupTestDeps(b)
	defer deps.Cleanup()

	accountID := "bench-query"
	CreateAccount(b, deps.Handler, accountID, "1000.00")

	// Add some history
	ctx := context.Background()
	for i := 0; i < 100; i++ {
		_, _ = deps.Handler.Deposit(ctx, &accountv1.DepositCommand{
			AccountId: accountID,
			Amount:    "1.00",
		})
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = deps.QueryHandler.GetAccountBalance(ctx, &accountv1.GetAccountBalanceRequest{
			AccountId: accountID,
		})
	}
}

// BenchmarkEventStoreWrite benchmarks raw event store write performance
func BenchmarkEventStoreWrite(b *testing.B) {
	deps := SetupTestDeps(b)
	defer deps.Cleanup()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		accountID := fmt.Sprintf("bench-eventstore-%d", i)
		CreateAccount(b, deps.Handler, accountID, "1000.00")
	}
}

// BenchmarkEventStoreRead benchmarks raw event store read performance
func BenchmarkEventStoreRead(b *testing.B) {
	deps := SetupTestDeps(b)
	defer deps.Cleanup()

	accountID := "bench-read"
	CreateAccount(b, deps.Handler, accountID, "1000.00")

	// Create some events
	ctx := context.Background()
	for i := 0; i < 100; i++ {
		_, _ = deps.Handler.Deposit(ctx, &accountv1.DepositCommand{
			AccountId: accountID,
			Amount:    "1.00",
		})
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = deps.EventStore.LoadEvents(accountID, 0)
	}
}
