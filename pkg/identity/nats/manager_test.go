package nats

import (
	"context"
	"errors"
	"testing"

	"github.com/nats-io/jwt/v2"
	natsserver "github.com/nats-io/nats-server/v2/test"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"github.com/plaenen/eventstore/pkg/runner"
)

// MockKeyStore implements KeyStore for testing
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

func TestAccountManager(t *testing.T) {
	// 1. Setup NATS Server (for Publish)
	opts := natsserver.DefaultTestOptions
	opts.Port = -1
	srv := natsserver.RunServer(&opts)
	defer srv.Shutdown()

	nc, err := nats.Connect(srv.ClientURL())
	if err != nil {
		t.Fatalf("Failed to connect to NATS: %v", err)
	}
	defer nc.Close()

	// 2. Generate Operator Key
	opKp, _ := nkeys.CreateOperator()
	opSeed, _ := opKp.Seed()
	opPub, _ := opKp.PublicKey()

	// 3. Setup Manager
	keyStore := NewMockKeyStore()
	manager := NewAccountManager(string(opSeed), keyStore, nc, runner.NewNoopLogger())

	ctx := context.Background()
	tenantID := "tenant-a"

	// 4. Test CreateAccount
	accJWT, err := manager.CreateAccount(ctx, tenantID)
	if err != nil {
		t.Fatalf("CreateAccount failed: %v", err)
	}

	// Verify Account JWT
	claims, err := jwt.DecodeAccountClaims(accJWT)
	if err != nil {
		t.Fatalf("Failed to decode account JWT: %v", err)
	}

	if claims.Name != tenantID {
		t.Errorf("Expected account name %s, got %s", tenantID, claims.Name)
	}
	if claims.Issuer != opPub {
		t.Errorf("Expected issuer %s, got %s", opPub, claims.Issuer)
	}

	// Verify Seed Stored
	_, err = keyStore.GetSeed(ctx, tenantID)
	if err != nil {
		t.Error("Account seed was not stored")
	}

	// 5. Test CreateUser
	userID := "user-1"
	userJWT, creds, err := manager.CreateUser(ctx, tenantID, userID)
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	// Verify User JWT
	userClaims, err := jwt.DecodeUserClaims(userJWT)
	if err != nil {
		t.Fatalf("Failed to decode user JWT: %v", err)
	}

	if userClaims.Name != userID {
		t.Errorf("Expected user name %s, got %s", userID, userClaims.Name)
	}
	if userClaims.Issuer != claims.Subject {
		t.Errorf("Expected issuer %s (Account Pub), got %s", claims.Subject, userClaims.Issuer)
	}

	// Verify Permissions
	expectedPubEvent := "events.tenant-a.>"
	if !contains(userClaims.Pub.Allow, expectedPubEvent) {
		t.Errorf("Expected pub permission %s, got %v", expectedPubEvent, userClaims.Pub.Allow)
	}

	expectedPubCmd := "commands.tenant-a.>"
	if !contains(userClaims.Pub.Allow, expectedPubCmd) {
		t.Errorf("Expected pub permission %s, got %v", expectedPubCmd, userClaims.Pub.Allow)
	}

	// Verify Creds
	if len(creds) == 0 {
		t.Error("Creds are empty")
	}
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
