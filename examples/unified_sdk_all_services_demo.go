package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/plaenen/eventstore/examples/bankaccount/handlers"
	accountv1 "github.com/plaenen/eventstore/examples/pb/account/v1"
	"github.com/plaenen/eventstore/examples/sdk"
	"github.com/plaenen/eventstore/pkg/eventsourcing"
	natspkg "github.com/plaenen/eventstore/pkg/nats"
	"github.com/plaenen/eventstore/pkg/sqlite"
)

func main() {
	fmt.Println("=== Unified SDK (All Services) Demo ===\n")

	// 1. Setup infrastructure
	fmt.Println("1. Setting up infrastructure...")

	eventStore, err := sqlite.NewEventStore(
		sqlite.WithMemoryDatabase(),
		sqlite.WithWALMode(true),
	)
	if err != nil {
		log.Fatalf("Failed to create event store: %v", err)
	}
	defer eventStore.Close()

	repo := accountv1.NewAccountRepository(eventStore)
	commandHandler := handlers.NewAccountCommandHandler(repo)
	queryHandler := handlers.NewAccountQueryHandler(repo)

	natsOpts := &server.Options{
		Host: "127.0.0.1",
		Port: -1,
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

	ctx := context.Background()
	natsServer, err := natspkg.NewServer(&natspkg.ServerConfig{
		ServerConfig: &eventsourcing.ServerConfig{
			QueueGroup:     "demo-handlers",
			MaxConcurrent:  10,
			HandlerTimeout: 30 * time.Second,
		},
		URL:         ns.ClientURL(),
		Name:        "AllServices",
		Version:     "1.0.0",
		Description: "All services demo",
	})
	if err != nil {
		log.Fatalf("Failed to create NATS server: %v", err)
	}
	defer natsServer.Close()

	commandService := accountv1.NewAccountCommandServiceServer(natsServer, commandHandler)
	queryService := accountv1.NewAccountQueryServiceServer(natsServer, queryHandler)

	if err := commandService.Start(ctx); err != nil {
		log.Fatalf("Failed to start command service: %v", err)
	}

	if err := queryService.Start(ctx); err != nil {
		log.Fatalf("Failed to start query service: %v", err)
	}

	fmt.Println("✓ Infrastructure ready\n")

	// 2. Create the TOP-LEVEL Unified SDK
	fmt.Println("2. Creating Top-Level Unified SDK...")

	transport, err := natspkg.NewTransport(&natspkg.TransportConfig{
		TransportConfig: eventsourcing.DefaultTransportConfig(),
		URL:             ns.ClientURL(),
		Name:            "unified-all-services-demo",
	})
	if err != nil {
		log.Fatalf("Failed to create transport: %v", err)
	}
	defer transport.Close()

	// Single SDK for ALL services!
	allServices := sdk.NewSDK(transport)
	defer allServices.Close()

	fmt.Println("✓ Single SDK created for ALL services\n")

	// 3. Use the unified SDK
	fmt.Println("3. Using services through unified SDK...\n")

	accountID := "all-services-demo"

	// Notice: sdk.Account.* - everything goes through one SDK!
	fmt.Println("📝 Account Service Operations:")

	openResp, appErr := allServices.Account.OpenAccount(ctx, &accountv1.OpenAccountCommand{
		AccountId:      accountID,
		OwnerName:      "Unified SDK User",
		InitialBalance: "5000.00",
	})
	if appErr != nil {
		log.Fatalf("Failed to open account: %v", appErr)
	}
	fmt.Printf("  ✓ Opened account (version: %d)\n", openResp.Version)

	depositResp, appErr := allServices.Account.Deposit(ctx, &accountv1.DepositCommand{
		AccountId: accountID,
		Amount:    "1000.00",
	})
	if appErr != nil {
		log.Fatalf("Failed to deposit: %v", appErr)
	}
	fmt.Printf("  ✓ Deposited $1000 - New balance: $%s\n", depositResp.NewBalance)

	balance, appErr := allServices.Account.GetAccountBalance(ctx, &accountv1.GetAccountBalanceRequest{
		AccountId: accountID,
	})
	if appErr != nil {
		log.Fatalf("Failed to get balance: %v", appErr)
	}
	fmt.Printf("  ✓ Current balance: $%s\n", balance.Balance)

	fmt.Println()

	// When you add more services, they work the same way:
	// allServices.User.CreateUser(ctx, cmd)
	// allServices.Document.CreateDocument(ctx, cmd)
	// allServices.Order.PlaceOrder(ctx, cmd)

	fmt.Println("=== Demo Complete ===")
	fmt.Println()
	fmt.Println("✨ Key Benefits:")
	fmt.Println("  • Single SDK for ALL services: sdk.NewSDK(transport)")
	fmt.Println("  • One transport, multiple services: sdk.Account, sdk.User, sdk.Document, etc.")
	fmt.Println("  • Single Close() for everything")
	fmt.Println("  • Type-safe access to all operations")
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  1. Add more proto files (user/v1, document/v1, etc.)")
	fmt.Println("  2. Run: buf generate")
	fmt.Println("  3. Run: ./bin/generate-unified-sdk ./examples/pb ./examples/sdk/unified.go")
	fmt.Println("  4. Use: sdk.User.CreateUser(...), sdk.Document.Upload(...), etc.")
}
