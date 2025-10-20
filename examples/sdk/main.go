package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/plaenen/eventsourcing/pkg/sdk"
	"github.com/plaenen/eventsourcing/pkg/eventsourcing"
	accountv1 "github.com/plaenen/eventsourcing/examples/pb/account/v1"
)

func main() {
	// Example 1: Development mode (in-memory command bus)
	if err := developmentExample(); err != nil {
		log.Fatalf("Development example failed: %v", err)
	}

	// Example 2: Production mode (NATS command bus) - commented out as it requires NATS
	// if err := productionExample(); err != nil {
	// 	log.Fatalf("Production example failed: %v", err)
	// }
}

func developmentExample() error {
	fmt.Println("=== Development Mode Example ===")
	fmt.Println("Uses in-memory command bus for local development")
	fmt.Println()

	// 1. Create SDK client in development mode
	client, err := sdk.NewBuilder().
		WithMode(sdk.DevelopmentMode).
		WithSQLiteDSN(":memory:").
		WithWALMode(false).
		Build()
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}
	defer client.Close()

	// 2. Register command handlers
	repo := accountv1.NewAccountRepository(client.EventStore())

	client.RegisterCommandHandler("account.v1.OpenAccountCommand",
		eventsourcing.CommandHandlerFunc(func(ctx context.Context, cmd *eventsourcing.CommandEnvelope) ([]*eventsourcing.Event, error) {
			// Extract command (in real app, you'd deserialize properly)
			openCmd := &accountv1.OpenAccountCommand{
				AccountId:      "acc-123",
				OwnerName:      "John Doe",
				InitialBalance: "1000.00",
			}

			// Load or create aggregate
			account := accountv1.NewAccount(openCmd.AccountId)
			account.SetCommandID(cmd.Metadata.CommandID)

			// Execute business logic
			if err := account.OpenAccount(ctx, openCmd, eventsourcing.EventMetadata{
				CausationID:   cmd.Metadata.CommandID,
				CorrelationID: cmd.Metadata.CorrelationID,
				PrincipalID:   cmd.Metadata.PrincipalID,
			}); err != nil {
				return nil, err
			}

			// Save aggregate
			result, err := repo.SaveWithCommand(account, cmd.Metadata.CommandID)
			if err != nil {
				return nil, err
			}

			return result.Events, nil
		}),
	)

	// 3. Subscribe to events
	sub, err := client.SubscribeToEvents(
		eventsourcing.EventFilter{
			AggregateTypes: []string{"Account"},
		},
		func(event *eventsourcing.EventEnvelope) error {
			fmt.Printf("📨 Event received: %s (aggregate: %s)\n",
				event.Event.EventType,
				event.Event.AggregateID)
			return nil
		},
	)
	if err != nil {
		return fmt.Errorf("failed to subscribe: %w", err)
	}
	defer sub.Unsubscribe()

	// 4. Send commands using the SDK
	ctx := context.Background()

	fmt.Println("📤 Sending OpenAccount command...")
	err = client.SendCommand(ctx, "acc-123", &accountv1.OpenAccountCommand{
		AccountId:      "acc-123",
		OwnerName:      "John Doe",
		InitialBalance: "1000.00",
	}, eventsourcing.CommandMetadata{
		PrincipalID: "user-123",
	})
	if err != nil {
		return fmt.Errorf("failed to send command: %w", err)
	}

	fmt.Println("✅ Command processed successfully!")
	fmt.Println()

	// Give events time to propagate
	time.Sleep(100 * time.Millisecond)

	// 5. Verify state
	fmt.Println("🔍 Loading aggregate to verify state...")
	account, err := repo.Load("acc-123")
	if err != nil {
		return fmt.Errorf("failed to load account: %w", err)
	}

	fmt.Printf("✅ Account loaded: %s (owner: %s, balance: %s)\n",
		account.AccountId,
		account.OwnerName,
		account.Balance,
	)

	return nil
}

func productionExample() error {
	fmt.Println("=== Production Mode Example ===")
	fmt.Println("Uses NATS for distributed command processing")
	fmt.Println()

	// 1. Create SDK client in production mode
	client, err := sdk.NewBuilder().
		WithMode(sdk.ProductionMode).
		WithNATSURL("nats://localhost:4222").
		WithSQLiteDSN("./data/events.db").
		WithWALMode(true).
		WithCommandTimeout(30 * time.Second).
		Build()
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}
	defer client.Close()

	// 2. Register command handlers (these will subscribe to NATS)
	repo := accountv1.NewAccountRepository(client.EventStore())

	client.RegisterCommandHandler("account.v1.OpenAccountCommand",
		eventsourcing.CommandHandlerFunc(func(ctx context.Context, cmd *eventsourcing.CommandEnvelope) ([]*eventsourcing.Event, error) {
			// Same handler logic as development mode
			// This will now be invoked via NATS
			openCmd := &accountv1.OpenAccountCommand{
				AccountId:      "acc-456",
				OwnerName:      "Jane Smith",
				InitialBalance: "2000.00",
			}

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

	// 3. Send command - this will go through NATS
	ctx := context.Background()

	fmt.Println("📤 Sending OpenAccount command via NATS...")
	err = client.SendCommand(ctx, "acc-456", &accountv1.OpenAccountCommand{
		AccountId:      "acc-456",
		OwnerName:      "Jane Smith",
		InitialBalance: "2000.00",
	}, eventsourcing.CommandMetadata{
		PrincipalID: "user-456",
	})
	if err != nil {
		return fmt.Errorf("failed to send command: %w", err)
	}

	fmt.Println("✅ Command sent via NATS and processed!")

	return nil
}
