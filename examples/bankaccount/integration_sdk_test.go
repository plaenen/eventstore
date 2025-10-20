package bankaccount_test

import (
	"context"
	"testing"
	"time"

	"github.com/plaenen/eventsourcing/pkg/eventsourcing"
	"github.com/plaenen/eventsourcing/pkg/sdk"
	accountv1 "github.com/plaenen/eventsourcing/examples/pb/account/v1"
)

// TestSDK_OpenAccount demonstrates using the SDK for a simple command
func TestSDK_OpenAccount(t *testing.T) {
	// 1. Create SDK client - handles all infrastructure
	client, err := sdk.NewBuilder().
		WithMode(sdk.DevelopmentMode).
		WithSQLiteDSN(":memory:").
		WithWALMode(false).
		Build()
	if err != nil {
		t.Fatalf("failed to create SDK client: %v", err)
	}
	defer client.Close()

	// 2. Register command handler
	repo := accountv1.NewAccountRepository(client.EventStore())

	client.RegisterCommandHandler("account.v1.OpenAccountCommand",
		eventsourcing.CommandHandlerFunc(func(ctx context.Context, cmd *eventsourcing.CommandEnvelope) ([]*eventsourcing.Event, error) {
			openCmd := cmd.Command.(*accountv1.OpenAccountCommand)

			account := accountv1.NewAccount(openCmd.AccountId)
			account.SetCommandID(cmd.Metadata.CommandID)

			metadata := eventsourcing.EventMetadata{
				CausationID:   cmd.Metadata.CommandID,
				CorrelationID: cmd.Metadata.CorrelationID,
				PrincipalID:   cmd.Metadata.PrincipalID,
			}

			if err := account.OpenAccount(ctx, openCmd, metadata); err != nil {
				return nil, err
			}

			result, err := repo.SaveWithCommand(account, cmd.Metadata.CommandID)
			if err != nil {
				return nil, err
			}

			return result.Events, nil
		}),
	)

	// 3. Use the GENERATED AccountClient for type-safe commands
	accountClient := accountv1.NewAccountClient(client)

	ctx := context.Background()

	// Send command with clean API
	_, err = accountClient.OpenAccount(ctx, &accountv1.OpenAccountCommand{
		AccountId:      "acc-sdk-123",
		OwnerName:      "Jane Doe",
		InitialBalance: "2000.00",
	}, "user-jane")
	if err != nil {
		t.Fatalf("OpenAccount failed: %v", err)
	}

	// Give async processing time
	time.Sleep(100 * time.Millisecond)

	// 4. Verify state
	account, err := repo.Load("acc-sdk-123")
	if err != nil {
		t.Fatalf("failed to load account: %v", err)
	}

	if account.AccountId != "acc-sdk-123" {
		t.Errorf("expected ID 'acc-sdk-123', got '%s'", account.AccountId)
	}

	if account.OwnerName != "Jane Doe" {
		t.Errorf("expected owner 'Jane Doe', got '%s'", account.OwnerName)
	}

	if account.Balance != "2000.00" {
		t.Errorf("expected balance '2000.00', got '%s'", account.Balance)
	}
}

// TestSDK_AccountLifecycle demonstrates a full account lifecycle using SDK
func TestSDK_AccountLifecycle(t *testing.T) {
	// Setup SDK client
	client, err := sdk.NewBuilder().
		WithMode(sdk.DevelopmentMode).
		WithSQLiteDSN(":memory:").
		Build()
	if err != nil {
		t.Fatalf("failed to create SDK client: %v", err)
	}
	defer client.Close()

	repo := accountv1.NewAccountRepository(client.EventStore())

	// Register all command handlers
	registerCommandHandlers(t, client, repo)

	// Use generated client
	accountClient := accountv1.NewAccountClient(client)
	ctx := context.Background()
	accountID := "acc-lifecycle-sdk"

	// 1. Open account
	t.Run("OpenAccount", func(t *testing.T) {
		_, err := accountClient.OpenAccount(ctx, &accountv1.OpenAccountCommand{
			AccountId:      accountID,
			OwnerName:      "Bob Smith",
			InitialBalance: "1000.00",
		}, "user-bob")
		if err != nil {
			t.Fatalf("OpenAccount failed: %v", err)
		}

		time.Sleep(200 * time.Millisecond)

		// Verify
		account, err := repo.Load(accountID)
		if err != nil {
			t.Fatalf("failed to load account: %v", err)
		}
		if account.Balance != "1000.00" {
			t.Errorf("expected balance 1000.00, got %s", account.Balance)
		}
	})

	// 2. Deposit money
	t.Run("Deposit", func(t *testing.T) {
		_, err := accountClient.Deposit(ctx, &accountv1.DepositCommand{
			AccountId: accountID,
			Amount:    "500.00",
		}, "user-bob")
		if err != nil {
			t.Fatalf("Deposit failed: %v", err)
		}

		time.Sleep(200 * time.Millisecond)

		// Verify
		account, err := repo.Load(accountID)
		if err != nil {
			t.Fatalf("failed to load account: %v", err)
		}
		// Balance might be "1500" or "1500.00" depending on calculation
		if account.Balance != "1500.00" && account.Balance != "1500" {
			t.Errorf("expected balance 1500.00, got %s", account.Balance)
		}
	})

	// 3. Withdraw money
	t.Run("Withdraw", func(t *testing.T) {
		_, err := accountClient.Withdraw(ctx, &accountv1.WithdrawCommand{
			AccountId: accountID,
			Amount:    "300.00",
		}, "user-bob")
		if err != nil {
			t.Fatalf("Withdraw failed: %v", err)
		}

		time.Sleep(200 * time.Millisecond)

		// Verify
		account, err := repo.Load(accountID)
		if err != nil {
			t.Fatalf("failed to load account: %v", err)
		}
		if account.Balance != "1200.00" && account.Balance != "1200" {
			t.Errorf("expected balance 1200.00, got %s", account.Balance)
		}
	})

	// 4. Attempt overdraft (should fail)
	t.Run("OverdraftPrevention", func(t *testing.T) {
		_, err := accountClient.Withdraw(ctx, &accountv1.WithdrawCommand{
			AccountId: accountID,
			Amount:    "2000.00", // More than balance
		}, "user-bob")
		if err == nil {
			t.Error("expected overdraft to fail, but it succeeded")
		}
	})

	// 5. Close account (must withdraw all money first)
	t.Run("CloseAccount", func(t *testing.T) {
		// First withdraw remaining balance
		_, err := accountClient.Withdraw(ctx, &accountv1.WithdrawCommand{
			AccountId: accountID,
			Amount:    "1200.00", // Withdraw all remaining money
		}, "user-bob")
		if err != nil {
			t.Fatalf("Final withdrawal failed: %v", err)
		}

		time.Sleep(200 * time.Millisecond)

		// Now close the account
		_, err = accountClient.CloseAccount(ctx, &accountv1.CloseAccountCommand{
			AccountId: accountID,
		}, "user-bob")
		if err != nil {
			t.Fatalf("CloseAccount failed: %v", err)
		}

		time.Sleep(200 * time.Millisecond)

		// Verify
		account, err := repo.Load(accountID)
		if err != nil {
			t.Fatalf("failed to load account: %v", err)
		}
		if account.Status != accountv1.AccountStatus_ACCOUNT_STATUS_CLOSED {
			t.Errorf("expected account to be closed")
		}
	})
}

// TestSDK_Idempotency verifies command idempotency works with SDK
func TestSDK_Idempotency(t *testing.T) {
	client, err := sdk.NewBuilder().
		WithMode(sdk.DevelopmentMode).
		WithSQLiteDSN(":memory:").
		Build()
	if err != nil {
		t.Fatalf("failed to create SDK client: %v", err)
	}
	defer client.Close()

	repo := accountv1.NewAccountRepository(client.EventStore())
	registerCommandHandlers(t, client, repo)

	ctx := context.Background()

	// To test idempotency, we need to use SendCommand directly with a fixed command ID
	cmd := &accountv1.OpenAccountCommand{
		AccountId:      "acc-idempotent",
		OwnerName:      "Test User",
		InitialBalance: "100.00",
	}

	// Fixed command ID for idempotency
	commandID := eventsourcing.GenerateID()

	metadata := eventsourcing.CommandMetadata{
		CommandID:     commandID,
		CorrelationID: eventsourcing.GenerateID(),
		PrincipalID:   "user-test",
	}
	metadata.Custom = map[string]string{
		"command_type": "account.v1.OpenAccountCommand",
		"aggregate_id": "acc-idempotent",
	}

	// First execution
	err = client.SendCommand(ctx, "acc-idempotent", cmd, metadata)
	if err != nil {
		t.Fatalf("first OpenAccount failed: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	// Second execution with SAME command ID should be idempotent
	err = client.SendCommand(ctx, "acc-idempotent", cmd, metadata)
	if err != nil {
		t.Fatalf("second OpenAccount failed: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	// Should still have only version 1 (not 2)
	account, err := repo.Load("acc-idempotent")
	if err != nil {
		t.Fatalf("failed to load account: %v", err)
	}
	if account.Version() != 1 {
		t.Errorf("expected version 1 (idempotent), got %d", account.Version())
	}
}

// registerCommandHandlers is a helper to register all command handlers
func registerCommandHandlers(t *testing.T, client *sdk.Client, repo *accountv1.AccountRepository) {
	// OpenAccount handler
	client.RegisterCommandHandler("account.v1.OpenAccountCommand",
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

	// Deposit handler
	client.RegisterCommandHandler("account.v1.DepositCommand",
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

	// Withdraw handler
	client.RegisterCommandHandler("account.v1.WithdrawCommand",
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

	// CloseAccount handler
	client.RegisterCommandHandler("account.v1.CloseAccountCommand",
		eventsourcing.CommandHandlerFunc(func(ctx context.Context, cmd *eventsourcing.CommandEnvelope) ([]*eventsourcing.Event, error) {
			closeCmd := cmd.Command.(*accountv1.CloseAccountCommand)
			account, err := repo.Load(closeCmd.AccountId)
			if err != nil {
				return nil, err
			}
			account.SetCommandID(cmd.Metadata.CommandID)

			if err := account.CloseAccount(ctx, closeCmd, eventsourcing.EventMetadata{
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
