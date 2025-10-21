package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/plaenen/eventstore/examples/bankaccount/handlers"
	accountv1 "github.com/plaenen/eventstore/examples/pb/account/v1"
	"github.com/plaenen/eventstore/pkg/eventsourcing"
	natspkg "github.com/plaenen/eventstore/pkg/nats"
	"github.com/plaenen/eventstore/pkg/sqlite"
)

func main() {
	fmt.Println("=== Bank Account End-to-End Demo ===")
	fmt.Println()

	ctx := context.Background()

	// 1. Start Embedded NATS Server
	fmt.Println("1️⃣  Starting embedded NATS server...")
	opts := &server.Options{
		Host: "127.0.0.1",
		Port: 4222,
	}
	ns, err := server.NewServer(opts)
	if err != nil {
		log.Fatalf("Failed to create NATS server: %v", err)
	}

	// Start the server in a goroutine
	go ns.Start()

	// Wait for server to be ready
	if !ns.ReadyForConnections(4 * time.Second) {
		log.Fatal("NATS server not ready")
	}
	defer ns.Shutdown()
	fmt.Println("   ✅ Embedded NATS server ready")
	fmt.Println()

	// 2. Setup SQLite Event Store
	fmt.Println("2️⃣  Setting up SQLite event store...")
	eventStore, err := sqlite.NewEventStore(sqlite.WithMemoryDatabase())
	if err != nil {
		log.Fatalf("Failed to create event store: %v", err)
	}
	defer eventStore.Close()
	fmt.Println("   ✅ Event store ready")
	fmt.Println()

	// 3. Create Repository
	fmt.Println("3️⃣  Creating account repository...")
	repo := accountv1.NewAccountRepository(eventStore)
	fmt.Println("   ✅ Repository created")
	fmt.Println()

	// 4. Create Handlers
	fmt.Println("4️⃣  Creating command and query handlers...")
	commandHandler := handlers.NewAccountCommandHandler(repo)
	queryHandler := handlers.NewAccountQueryHandler(repo)
	fmt.Println("   ✅ Handlers created")
	fmt.Println()

	// 5. Create NATS Server
	fmt.Println("5️⃣  Starting NATS server...")
	natsServer, err := natspkg.NewServer(&natspkg.ServerConfig{
		ServerConfig: &eventsourcing.ServerConfig{
			QueueGroup:     "bankaccount-handlers",
			MaxConcurrent:  10,
			HandlerTimeout: 5 * time.Second,
		},
		URL:         "nats://localhost:4222",
		Name:        "BankAccountService",
		Version:     "1.0.0",
		Description: "Bank account management service",
	})
	if err != nil {
		log.Fatalf("Failed to create NATS server: %v", err)
	}
	defer natsServer.Close()
	fmt.Println("   ✅ NATS server created")
	fmt.Println()

	// 6. Create and Start Generated Server Services
	fmt.Println("6️⃣  Starting server services...")

	// Command service
	commandService := accountv1.NewAccountCommandServiceServer(natsServer, commandHandler)
	if err := commandService.Start(ctx); err != nil {
		log.Fatalf("Failed to start command service: %v", err)
	}
	fmt.Println("   ✅ Command service started")

	// Query service
	queryService := accountv1.NewAccountQueryServiceServer(natsServer, queryHandler)
	if err := queryService.Start(ctx); err != nil {
		log.Fatalf("Failed to start query service: %v", err)
	}
	fmt.Println("   ✅ Query service started")
	fmt.Println()

	// Give services time to start
	time.Sleep(500 * time.Millisecond)

	// 7. Create Client Transport
	fmt.Println("7️⃣  Creating client transport...")
	transport, err := natspkg.NewTransport(&natspkg.TransportConfig{
		TransportConfig: &eventsourcing.TransportConfig{
			Timeout:              5 * time.Second,
			MaxReconnectAttempts: 3,
			ReconnectWait:        1 * time.Second,
		},
		URL:  "nats://localhost:4222",
		Name: "bankaccount-client",
	})
	if err != nil {
		log.Fatalf("Failed to create transport: %v", err)
	}
	defer transport.Close()
	fmt.Println("   ✅ Client transport ready")
	fmt.Println()

	// 8. Create SDK Client
	fmt.Println("8️⃣  Creating SDK client...")
	client := accountv1.NewAccountClient(transport)
	fmt.Println("   ✅ SDK client created")
	fmt.Println()

	// 9. Test Complete Flow
	fmt.Println("9️⃣  Testing complete flow...")
	fmt.Println()

	accountID := "acc-demo-123"

	// 8a. Open Account
	fmt.Println("   📝 Opening account...")
	openResp, appErr := client.OpenAccount(ctx, &accountv1.OpenAccountCommand{
		AccountId:      accountID,
		OwnerName:      "Alice Johnson",
		InitialBalance: "1000.00",
	})
	if appErr != nil {
		fmt.Printf("   ❌ Error opening account: [%s] %s\n", appErr.Code, appErr.Message)
	} else {
		fmt.Printf("   ✅ Account opened: %s (version %d)\n", openResp.AccountId, openResp.Version)
	}
	fmt.Println()

	// 8b. Get Account
	fmt.Println("   🔍 Getting account details...")
	account, appErr := client.GetAccount(ctx, &accountv1.GetAccountRequest{
		AccountId: accountID,
	})
	if appErr != nil {
		fmt.Printf("   ❌ Error getting account: [%s] %s\n", appErr.Code, appErr.Message)
	} else {
		fmt.Printf("   ✅ Account: %s\n", account.OwnerName)
		fmt.Printf("      Balance: %s\n", account.Balance)
		fmt.Printf("      Status: %s\n", account.Status)
		fmt.Printf("      Version: %d\n", account.Version)
	}
	fmt.Println()

	// 8c. Deposit
	fmt.Println("   💵 Depositing $500...")
	depositResp, appErr := client.Deposit(ctx, &accountv1.DepositCommand{
		AccountId: accountID,
		Amount:    "500.00",
	})
	if appErr != nil {
		fmt.Printf("   ❌ Error depositing: [%s] %s\n", appErr.Code, appErr.Message)
	} else {
		fmt.Printf("   ✅ Deposit successful. New balance: %s (version %d)\n", depositResp.NewBalance, depositResp.Version)
	}
	fmt.Println()

	// 8d. Get Balance
	fmt.Println("   🔍 Checking balance...")
	balance, appErr := client.GetAccountBalance(ctx, &accountv1.GetAccountBalanceRequest{
		AccountId: accountID,
	})
	if appErr != nil {
		fmt.Printf("   ❌ Error getting balance: [%s] %s\n", appErr.Code, appErr.Message)
	} else {
		fmt.Printf("   ✅ Current balance: %s (version %d)\n", balance.Balance, balance.Version)
	}
	fmt.Println()

	// 8e. Withdraw
	fmt.Println("   💸 Withdrawing $200...")
	withdrawResp, appErr := client.Withdraw(ctx, &accountv1.WithdrawCommand{
		AccountId: accountID,
		Amount:    "200.00",
	})
	if appErr != nil {
		fmt.Printf("   ❌ Error withdrawing: [%s] %s\n", appErr.Code, appErr.Message)
	} else {
		fmt.Printf("   ✅ Withdrawal successful. New balance: %s (version %d)\n", withdrawResp.NewBalance, withdrawResp.Version)
	}
	fmt.Println()

	// 8f. Try to withdraw more than balance (should fail)
	fmt.Println("   💸 Attempting to withdraw $5000 (should fail)...")
	_, appErr = client.Withdraw(ctx, &accountv1.WithdrawCommand{
		AccountId: accountID,
		Amount:    "5000.00",
	})
	if appErr != nil {
		fmt.Printf("   ✅ Expected error: [%s] %s\n", appErr.Code, appErr.Message)
	} else {
		fmt.Println("   ❌ Should have failed but didn't")
	}
	fmt.Println()

	// 8g. Get final balance
	fmt.Println("   🔍 Getting final balance...")
	finalBalance, appErr := client.GetAccountBalance(ctx, &accountv1.GetAccountBalanceRequest{
		AccountId: accountID,
	})
	if appErr != nil {
		fmt.Printf("   ❌ Error getting balance: [%s] %s\n", appErr.Code, appErr.Message)
	} else {
		fmt.Printf("   ✅ Final balance: %s (version %d)\n", finalBalance.Balance, finalBalance.Version)
	}
	fmt.Println()

	// 8h. Close Account
	fmt.Println("   🔒 Closing account...")
	closeResp, appErr := client.CloseAccount(ctx, &accountv1.CloseAccountCommand{
		AccountId: accountID,
	})
	if appErr != nil {
		fmt.Printf("   ❌ Error closing account: [%s] %s\n", appErr.Code, appErr.Message)
	} else {
		fmt.Printf("   ✅ Account closed. Final balance: %s (version %d)\n", closeResp.FinalBalance, closeResp.Version)
	}
	fmt.Println()

	// 8i. Try to deposit to closed account (should fail)
	fmt.Println("   💵 Attempting to deposit to closed account (should fail)...")
	_, appErr = client.Deposit(ctx, &accountv1.DepositCommand{
		AccountId: accountID,
		Amount:    "100.00",
	})
	if appErr != nil {
		fmt.Printf("   ✅ Expected error: [%s] %s\n", appErr.Code, appErr.Message)
	} else {
		fmt.Println("   ❌ Should have failed but didn't")
	}
	fmt.Println()

	fmt.Println("🎉 Bank Account Demo Complete!")
	fmt.Println()
	fmt.Println("Summary:")
	fmt.Println("  • Client → NATS → Server → Handler → Aggregate → Event Store")
	fmt.Println("  • All operations went through discoverable NATS microservices")
	fmt.Println("  • Event sourcing with proper aggregate state management")
	fmt.Println("  • Business rules enforced (insufficient funds, closed account)")
}
