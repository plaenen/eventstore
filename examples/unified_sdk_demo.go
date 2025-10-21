package main

import (
	"context"
	"fmt"
	"log"
	"time"

	accountv1 "github.com/plaenen/eventsourcing/examples/pb/account/v1"
	"github.com/plaenen/eventsourcing/examples/bankaccount/handlers"
	"github.com/plaenen/eventsourcing/pkg/eventsourcing"
	natspkg "github.com/plaenen/eventsourcing/pkg/nats"
	"github.com/plaenen/eventsourcing/pkg/sqlite"
	"github.com/nats-io/nats-server/v2/server"
)

func main() {
	fmt.Println("=== Unified SDK Demo ===\n")

	// 1. Setup infrastructure (in production, this would already be running)
	fmt.Println("1. Setting up infrastructure...")

	// Create event store
	eventStore, err := sqlite.NewEventStore(
		sqlite.WithMemoryDatabase(),
		sqlite.WithWALMode(true),
	)
	if err != nil {
		log.Fatalf("Failed to create event store: %v", err)
	}
	defer eventStore.Close()

	// Create repository and handlers
	repo := accountv1.NewAccountRepository(eventStore)
	commandHandler := handlers.NewAccountCommandHandler(repo)
	queryHandler := handlers.NewAccountQueryHandler(repo)

	// Start embedded NATS server
	natsOpts := &server.Options{
		Host: "127.0.0.1",
		Port: -1, // Random port
	}
	ns, err := server.NewServer(natsOpts)
	if err != nil {
		log.Fatalf("Failed to create NATS server: %v", err)
	}
	go ns.Start()
	defer ns.Shutdown()

	if !ns.ReadyForConnections(4 * time.Second) {
		log.Fatal("NATS server not ready")
	}

	// Create NATS server with handlers
	ctx := context.Background()
	natsServer, err := natspkg.NewServer(&natspkg.ServerConfig{
		ServerConfig: &eventsourcing.ServerConfig{
			QueueGroup:     "demo-handlers",
			MaxConcurrent:  10,
			HandlerTimeout: 30 * time.Second,
		},
		URL:         ns.ClientURL(),
		Name:        "AccountService",
		Version:     "1.0.0",
		Description: "Account management service",
	})
	if err != nil {
		log.Fatalf("Failed to create NATS server: %v", err)
	}
	defer natsServer.Close()

	// Register handlers
	commandService := accountv1.NewAccountCommandServiceServer(natsServer, commandHandler)
	queryService := accountv1.NewAccountQueryServiceServer(natsServer, queryHandler)

	if err := commandService.Start(ctx); err != nil {
		log.Fatalf("Failed to start command service: %v", err)
	}

	if err := queryService.Start(ctx); err != nil {
		log.Fatalf("Failed to start query service: %v", err)
	}

	fmt.Println("✓ Infrastructure ready\n")

	// 2. Create the Unified SDK (THIS IS THE KEY PART!)
	fmt.Println("2. Creating Unified SDK...")

	transport, err := natspkg.NewTransport(&natspkg.TransportConfig{
		TransportConfig: eventsourcing.DefaultTransportConfig(),
		URL:             ns.ClientURL(),
		Name:            "unified-sdk-demo",
	})
	if err != nil {
		log.Fatalf("Failed to create transport: %v", err)
	}
	defer transport.Close()

	// Create the unified SDK - only needs transport!
	sdk := accountv1.NewAccountSDK(transport)

	fmt.Println("✓ SDK created with single transport\n")

	// 3. Use the SDK - clean and simple API
	fmt.Println("3. Using the Unified SDK...")
	fmt.Println()

	accountID := "sdk-demo-account"

	// Open account
	fmt.Println("Opening account...")
	openResp, appErr := sdk.OpenAccount(ctx, &accountv1.OpenAccountCommand{
		AccountId:      accountID,
		OwnerName:      "Jane Doe",
		InitialBalance: "1000.00",
	})
	if appErr != nil {
		log.Fatalf("Failed to open account: [%s] %s", appErr.Code, appErr.Message)
	}
	fmt.Printf("  ✓ Account opened (version: %d)\n\n", openResp.Version)

	// Deposit money
	fmt.Println("Depositing $500...")
	depositResp, appErr := sdk.Deposit(ctx, &accountv1.DepositCommand{
		AccountId: accountID,
		Amount:    "500.00",
	})
	if appErr != nil {
		log.Fatalf("Failed to deposit: [%s] %s", appErr.Code, appErr.Message)
	}
	fmt.Printf("  ✓ Deposited successfully\n")
	fmt.Printf("  New balance: $%s (version: %d)\n\n", depositResp.NewBalance, depositResp.Version)

	// Withdraw money
	fmt.Println("Withdrawing $200...")
	withdrawResp, appErr := sdk.Withdraw(ctx, &accountv1.WithdrawCommand{
		AccountId: accountID,
		Amount:    "200.00",
	})
	if appErr != nil {
		log.Fatalf("Failed to withdraw: [%s] %s", appErr.Code, appErr.Message)
	}
	fmt.Printf("  ✓ Withdrawn successfully\n")
	fmt.Printf("  New balance: $%s (version: %d)\n\n", withdrawResp.NewBalance, withdrawResp.Version)

	// Get account details
	fmt.Println("Getting account details...")
	account, appErr := sdk.GetAccount(ctx, &accountv1.GetAccountRequest{
		AccountId: accountID,
	})
	if appErr != nil {
		log.Fatalf("Failed to get account: [%s] %s", appErr.Code, appErr.Message)
	}
	fmt.Printf("  ✓ Account details:\n")
	fmt.Printf("    Owner: %s\n", account.OwnerName)
	fmt.Printf("    Balance: $%s\n", account.Balance)
	fmt.Printf("    Status: %s\n", account.Status)
	fmt.Printf("    Version: %d\n\n", account.Version)

	// Get account balance (lightweight query)
	fmt.Println("Getting account balance...")
	balance, appErr := sdk.GetAccountBalance(ctx, &accountv1.GetAccountBalanceRequest{
		AccountId: accountID,
	})
	if appErr != nil {
		log.Fatalf("Failed to get balance: [%s] %s", appErr.Code, appErr.Message)
	}
	fmt.Printf("  ✓ Current balance: $%s\n\n", balance.Balance)

	// Get account history
	fmt.Println("Getting account history...")
	history, appErr := sdk.GetAccountHistory(ctx, &accountv1.GetAccountHistoryRequest{
		AccountId: accountID,
	})
	if appErr != nil {
		log.Fatalf("Failed to get history: [%s] %s", appErr.Code, appErr.Message)
	}
	fmt.Printf("  ✓ Transaction history (%d transactions):\n", len(history.Transactions))
	for i, tx := range history.Transactions {
		fmt.Printf("    %d. Type: %s, Amount: $%s\n", i+1, tx.Type, tx.Amount)
	}
	fmt.Println()

	// Close account
	fmt.Println("Closing account...")
	closeResp, appErr := sdk.CloseAccount(ctx, &accountv1.CloseAccountCommand{
		AccountId: accountID,
	})
	if appErr != nil {
		log.Fatalf("Failed to close account: [%s] %s", appErr.Code, appErr.Message)
	}
	fmt.Printf("  ✓ Account closed (version: %d)\n\n", closeResp.Version)

	// Try to deposit to closed account (should fail)
	fmt.Println("Attempting to deposit to closed account...")
	_, appErr = sdk.Deposit(ctx, &accountv1.DepositCommand{
		AccountId: accountID,
		Amount:    "100.00",
	})
	if appErr != nil {
		fmt.Printf("  ✓ Failed as expected: [%s] %s\n\n", appErr.Code, appErr.Message)
	}

	fmt.Println("=== Demo Complete ===")
	fmt.Println()
	fmt.Println("Key Benefits of Unified SDK:")
	fmt.Println("  • Single import: accountv1.NewAccountSDK(transport)")
	fmt.Println("  • Only needs transport - no separate command/query clients")
	fmt.Println("  • Type-safe methods for all operations")
	fmt.Println("  • Clean, developer-friendly API")
	fmt.Println("  • Built-in error handling")
}
