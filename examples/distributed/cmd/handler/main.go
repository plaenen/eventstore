package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/plaenen/eventsourcing/pkg/eventsourcing"
	"github.com/plaenen/eventsourcing/pkg/sdk"
	"github.com/plaenen/eventsourcing/pkg/unifiedsdk"
	accountv1 "github.com/plaenen/eventsourcing/examples/pb/account/v1"
)

// Command Handler Service - Has Event Store
// This service receives commands from NATS and processes them

func main() {
	fmt.Println("🔧 Command Handler Service Starting...")
	fmt.Println()

	// Get configuration from environment
	natsURL := getEnv("NATS_URL", "nats://localhost:4222")
	dbPath := getEnv("DB_PATH", "./data/handler.db")

	// Create HANDLER CLIENT - Has database!
	client, err := unifiedsdk.New(
		unifiedsdk.WithMode(sdk.ProductionMode),
		unifiedsdk.WithRole(sdk.RoleCommandHandler), // ← HANDLER!
		unifiedsdk.WithNATSURL(natsURL),
		unifiedsdk.WithSQLiteDSN(dbPath), // ← Event store required
		unifiedsdk.WithWALMode(true),
	)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	fmt.Printf("✅ Connected to NATS at %s\n", natsURL)
	fmt.Printf("✅ Event store initialized: %s\n", dbPath)
	fmt.Println()

	// Register command handlers
	registerHandlers(client)
	fmt.Println("✅ Registered command handlers")
	fmt.Println("⏳ Listening for commands...")
	fmt.Println()

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	fmt.Println("Press Ctrl+C to shutdown")
	<-sigChan

	fmt.Println()
	fmt.Println("🛑 Shutting down...")
}

func registerHandlers(s *unifiedsdk.SDK) {
	repo := accountv1.NewAccountRepository(s.Client().EventStore())

	// OpenAccount handler
	s.Client().RegisterCommandHandler("account.v1.OpenAccountCommand",
		eventsourcing.CommandHandlerFunc(func(ctx context.Context, cmd *eventsourcing.CommandEnvelope) ([]*eventsourcing.Event, error) {
			openCmd := cmd.Command.(*accountv1.OpenAccountCommand)

			fmt.Printf("📥 Received OpenAccount: %s (%s)\n", openCmd.AccountId, openCmd.OwnerName)

			account := accountv1.NewAccount(openCmd.AccountId)
			account.SetCommandID(cmd.Metadata.CommandID)

			if err := account.OpenAccount(ctx, openCmd, eventsourcing.EventMetadata{
				CausationID:   cmd.Metadata.CommandID,
				CorrelationID: cmd.Metadata.CorrelationID,
				PrincipalID:   cmd.Metadata.PrincipalID,
			}); err != nil {
				fmt.Printf("❌ Business logic failed: %v\n", err)
				return nil, err
			}

			result, err := repo.SaveWithCommand(account, cmd.Metadata.CommandID)
			if err != nil {
				fmt.Printf("❌ Save failed: %v\n", err)
				return nil, err
			}

			fmt.Printf("✅ Account created: %s (version %d)\n", openCmd.AccountId, account.Version())
			return result.Events, nil
		}),
	)

	// Deposit handler
	s.Client().RegisterCommandHandler("account.v1.DepositCommand",
		eventsourcing.CommandHandlerFunc(func(ctx context.Context, cmd *eventsourcing.CommandEnvelope) ([]*eventsourcing.Event, error) {
			depositCmd := cmd.Command.(*accountv1.DepositCommand)

			fmt.Printf("📥 Received Deposit: %s (amount %s)\n", depositCmd.AccountId, depositCmd.Amount)

			account, err := repo.Load(depositCmd.AccountId)
			if err != nil {
				fmt.Printf("❌ Load failed: %v\n", err)
				return nil, err
			}
			account.SetCommandID(cmd.Metadata.CommandID)

			if err := account.Deposit(ctx, depositCmd, eventsourcing.EventMetadata{
				CausationID:   cmd.Metadata.CommandID,
				CorrelationID: cmd.Metadata.CorrelationID,
				PrincipalID:   cmd.Metadata.PrincipalID,
			}); err != nil {
				fmt.Printf("❌ Business logic failed: %v\n", err)
				return nil, err
			}

			result, err := repo.SaveWithCommand(account, cmd.Metadata.CommandID)
			if err != nil {
				fmt.Printf("❌ Save failed: %v\n", err)
				return nil, err
			}

			fmt.Printf("✅ Deposit processed: %s (version %d)\n", depositCmd.AccountId, account.Version())
			return result.Events, nil
		}),
	)

	// Withdraw handler
	s.Client().RegisterCommandHandler("account.v1.WithdrawCommand",
		eventsourcing.CommandHandlerFunc(func(ctx context.Context, cmd *eventsourcing.CommandEnvelope) ([]*eventsourcing.Event, error) {
			withdrawCmd := cmd.Command.(*accountv1.WithdrawCommand)

			fmt.Printf("📥 Received Withdraw: %s (amount %s)\n", withdrawCmd.AccountId, withdrawCmd.Amount)

			account, err := repo.Load(withdrawCmd.AccountId)
			if err != nil {
				fmt.Printf("❌ Load failed: %v\n", err)
				return nil, err
			}
			account.SetCommandID(cmd.Metadata.CommandID)

			if err := account.Withdraw(ctx, withdrawCmd, eventsourcing.EventMetadata{
				CausationID:   cmd.Metadata.CommandID,
				CorrelationID: cmd.Metadata.CorrelationID,
				PrincipalID:   cmd.Metadata.PrincipalID,
			}); err != nil {
				fmt.Printf("❌ Business logic failed: %v\n", err)
				return nil, err
			}

			result, err := repo.SaveWithCommand(account, cmd.Metadata.CommandID)
			if err != nil {
				fmt.Printf("❌ Save failed: %v\n", err)
				return nil, err
			}

			fmt.Printf("✅ Withdraw processed: %s (version %d)\n", withdrawCmd.AccountId, account.Version())
			return result.Events, nil
		}),
	)
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
