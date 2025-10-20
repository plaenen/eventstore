package domain_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/plaenen/eventsourcing/pkg/eventsourcing"
	accountv1 "github.com/plaenen/eventsourcing/examples/pb/account/v1"
	"google.golang.org/protobuf/proto"
)

func TestAccount_OpenAccount(t *testing.T) {
	tests := []struct {
		name    string
		cmd     *accountv1.OpenAccountCommand
		wantErr bool
	}{
		{
			name: "successful account opening",
			cmd: &accountv1.OpenAccountCommand{
				AccountId:      "acc-123",
				OwnerName:      "John Doe",
				InitialBalance: "1000.00",
			},
			wantErr: false,
		},
		{
			name: "negative initial balance should fail",
			cmd: &accountv1.OpenAccountCommand{
				AccountId:      "acc-456",
				OwnerName:      "Jane Doe",
				InitialBalance: "-100.00",
			},
			wantErr: true,
		},
		{
			name: "zero balance is allowed",
			cmd: &accountv1.OpenAccountCommand{
				AccountId:      "acc-789",
				OwnerName:      "Bob Smith",
				InitialBalance: "0.00",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := accountv1.NewAccount(tt.cmd.AccountId)

			metadata := eventsourcing.EventMetadata{
				CausationID:     eventsourcing.GenerateID(),
				CorrelationID: eventsourcing.GenerateID(),
				PrincipalID:   "test-user",
			}

			err := account.OpenAccount(context.Background(), tt.cmd, metadata)

			if (err != nil) != tt.wantErr {
				t.Errorf("OpenAccount() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// Verify uncommitted events
				events := account.UncommittedEvents()
				if len(events) != 1 {
					t.Errorf("expected 1 event, got %d", len(events))
				}

				// Verify event details
				if events[0].AggregateID != tt.cmd.AccountId {
					t.Errorf("expected aggregate ID %s, got %s", tt.cmd.AccountId, events[0].AggregateID)
				}

				if events[0].EventType != "accountv1.AccountOpenedEvent" {
					t.Errorf("expected event type accountv1.AccountOpenedEvent, got %s", events[0].EventType)
				}

				// Note: Unique constraints are validated at the repository/event store level
			}
		})
	}
}

func TestAccount_Deposit(t *testing.T) {
	tests := []struct {
		name           string
		initialBalance string
		depositAmount  string
		expectedNew    string
		wantErr        bool
	}{
		{
			name:           "deposit to existing balance",
			initialBalance: "1000.00",
			depositAmount:  "250.50",
			expectedNew:    "1250.50",
			wantErr:        false,
		},
		{
			name:           "deposit to zero balance",
			initialBalance: "0.00",
			depositAmount:  "100.00",
			expectedNew:    "100.00",
			wantErr:        false,
		},
		{
			name:           "negative deposit should fail",
			initialBalance: "1000.00",
			depositAmount:  "-50.00",
			expectedNew:    "",
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup account with initial state
			account := accountv1.NewAccount("acc-123")
			account.Balance = tt.initialBalance
			account.Status = accountv1.AccountStatus_ACCOUNT_STATUS_OPEN

			cmd := &accountv1.DepositCommand{
				AccountId: "acc-123",
				Amount:    tt.depositAmount,
			}

			metadata := eventsourcing.EventMetadata{
				CausationID:     eventsourcing.GenerateID(),
				CorrelationID:   eventsourcing.GenerateID(),
				PrincipalID:     "test-user",
			}

			err := account.Deposit(context.Background(), cmd, metadata)

			if (err != nil) != tt.wantErr {
				t.Errorf("Deposit() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				events := account.UncommittedEvents()
				if len(events) != 1 {
					t.Errorf("expected 1 event, got %d", len(events))
				}

				if events[0].EventType != "accountv1.MoneyDepositedEvent" {
					t.Errorf("expected MoneyDepositedEvent, got %s", events[0].EventType)
				}
			}
		})
	}
}

func TestAccount_Withdraw(t *testing.T) {
	tests := []struct {
		name            string
		initialBalance  string
		withdrawAmount  string
		expectedBalance string
		wantErr         bool
		errorMsg        string
	}{
		{
			name:            "successful withdrawal",
			initialBalance:  "1000.00",
			withdrawAmount:  "250.00",
			expectedBalance: "750.00",
			wantErr:         false,
		},
		{
			name:            "withdraw all funds",
			initialBalance:  "500.00",
			withdrawAmount:  "500.00",
			expectedBalance: "0.00",
			wantErr:         false,
		},
		{
			name:           "insufficient funds",
			initialBalance: "100.00",
			withdrawAmount: "150.00",
			wantErr:        true,
			errorMsg:       "insufficient balance",
		},
		{
			name:           "negative withdrawal",
			initialBalance: "1000.00",
			withdrawAmount: "-50.00",
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := accountv1.NewAccount("acc-123")
			account.Balance = tt.initialBalance
			account.Status = accountv1.AccountStatus_ACCOUNT_STATUS_OPEN

			cmd := &accountv1.WithdrawCommand{
				AccountId: "acc-123",
				Amount:    tt.withdrawAmount,
			}

			metadata := eventsourcing.EventMetadata{
				CausationID:     eventsourcing.GenerateID(),
				CorrelationID:   eventsourcing.GenerateID(),
				PrincipalID:     "test-user",
			}

			err := account.Withdraw(context.Background(), cmd, metadata)

			if (err != nil) != tt.wantErr {
				t.Errorf("Withdraw() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.errorMsg != "" {
				if err == nil || !contains(err.Error(), tt.errorMsg) {
					t.Errorf("expected error to contain '%s', got '%v'", tt.errorMsg, err)
				}
			}

			if !tt.wantErr {
				events := account.UncommittedEvents()
				if len(events) != 1 {
					t.Errorf("expected 1 event, got %d", len(events))
				}
			}
		})
	}
}

func TestAccount_EventSourcing_Replay(t *testing.T) {
	// Test that events can be replayed to reconstruct state
	account := accountv1.NewAccount("acc-123")

	// Simulate event stream
	events := []*eventsourcing.Event{
		{
			ID:            "evt-1",
			AggregateID:   "acc-123",
			AggregateType: "Account",
			EventType:     "accountv1.AccountOpenedEvent",
			Version:       1,
			Timestamp:     time.Now(),
			Data:          mustMarshal(&accountv1.AccountOpenedEvent{
				AccountId:      "acc-123",
				OwnerName:      "John Doe",
				InitialBalance: "1000.00",
			}),
		},
		{
			ID:            "evt-2",
			AggregateID:   "acc-123",
			AggregateType: "Account",
			EventType:     "accountv1.MoneyDepositedEvent",
			Version:       2,
			Timestamp:     time.Now(),
			Data:          mustMarshal(&accountv1.MoneyDepositedEvent{
				AccountId:  "acc-123",
				Amount:     "500.00",
				NewBalance: "1500.00",
			}),
		},
		{
			ID:            "evt-3",
			AggregateID:   "acc-123",
			AggregateType: "Account",
			EventType:     "accountv1.MoneyWithdrawnEvent",
			Version:       3,
			Timestamp:     time.Now(),
			Data:          mustMarshal(&accountv1.MoneyWithdrawnEvent{
				AccountId:  "acc-123",
				Amount:     "300.00",
				NewBalance: "1200.00",
			}),
		},
	}

	// Replay events
	for _, evt := range events {
		msg, err := deserializeEvent(evt)
		if err != nil {
			t.Fatalf("failed to deserialize event: %v", err)
		}

		if err := account.ApplyEvent(msg); err != nil {
			t.Fatalf("failed to apply event: %v", err)
		}

		// Update version from historical event
		account.LoadFromHistory([]*eventsourcing.Event{evt})
	}

	// Verify final state
	if account.ID() != "acc-123" {
		t.Errorf("expected ID 'acc-123', got '%s'", account.ID())
	}

	if account.OwnerName != "John Doe" {
		t.Errorf("expected owner 'John Doe', got '%s'", account.OwnerName)
	}

	if account.Balance != "1200.00" {
		t.Errorf("expected balance '1200.00', got '%s'", account.Balance)
	}

	if account.Version() != 3 {
		t.Errorf("expected version 3, got %d", account.Version())
	}
}

func TestAccount_Idempotency(t *testing.T) {
	// Test that same command ID produces same event ID
	account := accountv1.NewAccount("acc-123")
	commandID := eventsourcing.GenerateID()

	account.SetCommandID(commandID)

	cmd := &accountv1.OpenAccountCommand{
		AccountId:      "acc-123",
		OwnerName:      "John Doe",
		InitialBalance: "1000.00",
	}

	metadata := eventsourcing.EventMetadata{
		CausationID:   commandID,
		PrincipalID: "test-user",
	}

	// Execute command twice
	err1 := account.OpenAccount(context.Background(), cmd, metadata)
	if err1 != nil {
		t.Fatalf("first command failed: %v", err1)
	}

	eventID1 := account.UncommittedEvents()[0].ID

	// Clear and retry
	account.ClearUncommittedEvents()
	account2 := accountv1.NewAccount("acc-123")
	account2.SetCommandID(commandID)

	err2 := account2.OpenAccount(context.Background(), cmd, metadata)
	if err2 != nil {
		t.Fatalf("second command failed: %v", err2)
	}

	eventID2 := account2.UncommittedEvents()[0].ID

	// Event IDs should be identical (deterministic)
	if eventID1 != eventID2 {
		t.Errorf("event IDs not deterministic: %s != %s", eventID1, eventID2)
	}
}

// Helper functions

func mustMarshal(msg proto.Message) []byte {
	data, err := proto.Marshal(msg)
	if err != nil {
		panic(err)
	}
	return data
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func deserializeEvent(event *eventsourcing.Event) (proto.Message, error) {
	switch event.EventType {
	case "accountv1.AccountOpenedEvent":
		msg := &accountv1.AccountOpenedEvent{}
		err := proto.Unmarshal(event.Data, msg)
		return msg, err
	case "accountv1.MoneyDepositedEvent":
		msg := &accountv1.MoneyDepositedEvent{}
		err := proto.Unmarshal(event.Data, msg)
		return msg, err
	case "accountv1.MoneyWithdrawnEvent":
		msg := &accountv1.MoneyWithdrawnEvent{}
		err := proto.Unmarshal(event.Data, msg)
		return msg, err
	default:
		return nil, fmt.Errorf("unknown event type: %s", event.EventType)
	}
}
