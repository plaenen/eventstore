package identity

import (
	"context"
	"errors"
	"testing"

	"connectrpc.com/connect"
	"github.com/nats-io/nats-server/v2/test"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	identitynats "github.com/plaenen/eventstore/pkg/identity/nats"
	identityv1 "github.com/plaenen/eventstore/pkg/identity/v1"
	"github.com/plaenen/eventstore/pkg/runner"
)

// MockKeyStore from manager_test.go
type MockKeyStore struct {
	seeds map[string][]byte
}

func NewMockKeyStore() *MockKeyStore {
	return &MockKeyStore{
		seeds: make(map[string][]byte),
	}
}

func (m *MockKeyStore) SaveSeed(ctx context.Context, id string, seed []byte) error {
	m.seeds[id] = seed
	return nil
}

func (m *MockKeyStore) GetSeed(ctx context.Context, id string) ([]byte, error) {
	seed, ok := m.seeds[id]
	if !ok {
		return nil, errors.New("seed not found")
	}
	return seed, nil
}

func TestExchangeToken(t *testing.T) {
	// 1. Setup NATS
	opts := test.DefaultTestOptions
	opts.Port = -1
	srv := test.RunServer(&opts)
	defer srv.Shutdown()

	nc, err := nats.Connect(srv.ClientURL())
	if err != nil {
		t.Fatalf("Failed to connect to NATS: %v", err)
	}
	defer nc.Close()

	// 2. Setup Manager
	opKp, _ := nkeys.CreateOperator()
	opSeed, _ := opKp.Seed()

	keyStore := NewMockKeyStore()
	manager := identitynats.NewAccountManager(string(opSeed), keyStore, nc, runner.NewNoopLogger())

	// 3. Setup Service
	svc := NewService(manager, runner.NewNoopLogger())

	// 4. Pre-provision Account (since CreateUser needs it)
	tenantID := "tenant-1"
	_, err = manager.CreateAccount(context.Background(), tenantID)
	if err != nil {
		t.Fatalf("Failed to create account: %v", err)
	}

	// 5. Test ExchangeToken
	req := connect.NewRequest(&identityv1.ExchangeTokenRequest{
		TenantId: tenantID,
		UserId:   "user-1",
		Token:    "valid-token",
	})

	res, err := svc.ExchangeToken(context.Background(), req)
	if err != nil {
		t.Fatalf("ExchangeToken failed: %v", err)
	}

	if res.Msg.Jwt == "" {
		t.Error("Expected JWT in response")
	}
	if res.Msg.Creds == "" {
		t.Error("Expected Creds in response")
	}
}
