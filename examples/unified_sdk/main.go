package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/plaenen/eventsourcing/pkg/unifiedsdk"
	"github.com/plaenen/eventsourcing/pkg/sdk"
	"github.com/plaenen/eventsourcing/pkg/eventsourcing"
	accountv1 "github.com/plaenen/eventsourcing/examples/pb/account/v1"
)

// This example demonstrates the UNIFIED SDK API
// All services accessible from a single SDK instance!

func main() {
	fmt.Println("=== Unified SDK Example ===")
	fmt.Println()

	// 1. Create unified SDK - one line!
	s, err := unifiedsdk.New(
		unifiedsdk.WithMode(sdk.DevelopmentMode),
		unifiedsdk.WithSQLiteDSN(":memory:"),
	)
	if err != nil {
		log.Fatalf("Failed to create SDK: %v", err)
	}
	defer s.Close()

	// Register command handlers (in a real app, this would be in your service layer)
	registerHandlers(s)

	// 2. Access services via properties!
	fmt.Println("✨ Using unified SDK: s.Account.OpenAccount(...)")
	fmt.Println()

	ctx := context.Background()

	// Open an account - beautiful, clean API!
	_, err = s.Account.OpenAccount(ctx, &accountv1.OpenAccountCommand{
		AccountId:      "acc-unified-001",
		OwnerName:      "Alice Johnson",
		InitialBalance: "5000.00",
	}, "user-alice")
	if err != nil {
		log.Fatalf("Failed to open account: %v", err)
	}
	fmt.Println("✅ Account opened: acc-unified-001")
	time.Sleep(100 * time.Millisecond)

	// Deposit money
	_, err = s.Account.Deposit(ctx, &accountv1.DepositCommand{
		AccountId: "acc-unified-001",
		Amount:    "1000.00",
	}, "user-alice")
	if err != nil {
		log.Fatalf("Failed to deposit: %v", err)
	}
	fmt.Println("✅ Deposited: $1000.00")
	time.Sleep(100 * time.Millisecond)

	// Withdraw money
	_, err = s.Account.Withdraw(ctx, &accountv1.WithdrawCommand{
		AccountId: "acc-unified-001",
		Amount:    "500.00",
	}, "user-alice")
	if err != nil {
		log.Fatalf("Failed to withdraw: %v", err)
	}
	fmt.Println("✅ Withdrew: $500.00")

	fmt.Println()
	fmt.Println("🎉 Perfect! All operations completed successfully!")
	fmt.Println()
	fmt.Println("Key Benefits of Unified SDK:")
	fmt.Println("  ✅ Single entry point: unifiedsdk.New()")
	fmt.Println("  ✅ All services accessible: s.Account, s.Order, s.User, etc.")
	fmt.Println("  ✅ Type-safe API with IntelliSense")
	fmt.Println("  ✅ Auto-generated from proto files")
	fmt.Println("  ✅ Works in Dev and Production modes")
	fmt.Println()
	fmt.Println("When you add more services (Order, User, etc.), they'll automatically")
	fmt.Println("appear as properties on the SDK!")
}

func registerHandlers(s *unifiedsdk.SDK) {
	repo := accountv1.NewAccountRepository(s.Client().EventStore())

	s.Client().RegisterCommandHandler("account.v1.OpenAccountCommand",
		eventsourcing.CommandHandlerFunc(func(ctx context.Context, cmd *eventsourcing.CommandEnvelope) ([]*eventsourcing.Event, error) {
			openCmd := cmd.Command.(*accountv1.OpenAccountCommand)
			account := accountv1.NewAccount(openCmd.AccountId)
			account.SetCommandID(cmd.Metadata.CommandID)

			if err := account.OpenAccount(ctx, openCmd, eventsourcing.EventMetadata{
				CausationID:   cmd.Metadata.CommandID,
				CorrelationID: cmd.Metadata.CorrelationID,
				PrincipalID:   cmd.Metadata.PrincipalID,
			}); err != nil {
				return nil, err
			}

			result, err := repo.SaveWithCommand(account, cmd.Metadata.CommandID)
			if err != nil {
				return nil, err
			}
			return result.Events, nil
		}),
	)

	s.Client().RegisterCommandHandler("account.v1.DepositCommand",
		eventsourcing.CommandHandlerFunc(func(ctx context.Context, cmd *eventsourcing.CommandEnvelope) ([]*eventsourcing.Event, error) {
			depositCmd := cmd.Command.(*accountv1.DepositCommand)
			account, err := repo.Load(depositCmd.AccountId)
			if err != nil {
				return nil, err
			}
			account.SetCommandID(cmd.Metadata.CommandID)

			if err := account.Deposit(ctx, depositCmd, eventsourcing.EventMetadata{
				CausationID:   cmd.Metadata.CommandID,
				CorrelationID: cmd.Metadata.CorrelationID,
				PrincipalID:   cmd.Metadata.PrincipalID,
			}); err != nil {
				return nil, err
			}

			result, err := repo.SaveWithCommand(account, cmd.Metadata.CommandID)
			if err != nil {
				return nil, err
			}
			return result.Events, nil
		}),
	)

	s.Client().RegisterCommandHandler("account.v1.WithdrawCommand",
		eventsourcing.CommandHandlerFunc(func(ctx context.Context, cmd *eventsourcing.CommandEnvelope) ([]*eventsourcing.Event, error) {
			withdrawCmd := cmd.Command.(*accountv1.WithdrawCommand)
			account, err := repo.Load(withdrawCmd.AccountId)
			if err != nil {
				return nil, err
			}
			account.SetCommandID(cmd.Metadata.CommandID)

			if err := account.Withdraw(ctx, withdrawCmd, eventsourcing.EventMetadata{
				CausationID:   cmd.Metadata.CommandID,
				CorrelationID: cmd.Metadata.CorrelationID,
				PrincipalID:   cmd.Metadata.PrincipalID,
			}); err != nil {
				return nil, err
			}

			result, err := repo.SaveWithCommand(account, cmd.Metadata.CommandID)
			if err != nil {
				return nil, err
			}
			return result.Events, nil
		}),
	)
}
