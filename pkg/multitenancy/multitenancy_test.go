package multitenancy

import (
	"context"
	"testing"

	accountv1 "github.com/plaenen/eventstore/examples/pb/account/v1"
	"github.com/plaenen/eventstore/pkg/eventsourcing"
)

func TestSharedDatabaseTenantIsolation(t *testing.T) {
	// Create shared database multi-tenant store
	multiStore, err := NewMultiTenantEventStore(MultiTenantConfig{
		Strategy:  SharedDatabase,
		SharedDSN: ":memory:",
		WALMode:   true,
	})
	if err != nil {
		t.Fatalf("Failed to create multi-tenant store: %v", err)
	}
	defer multiStore.Close()

	// Tenant A context
	tenantACtx := WithTenantID(context.Background(), "tenant-a")

	// Tenant A: Create account with local ID "acc-001"
	aggregateIDA := ComposeAggregateID("tenant-a", "acc-001")

	storeA, err := multiStore.GetStore(tenantACtx)
	if err != nil {
		t.Fatalf("Failed to get store for tenant A: %v", err)
	}

	repoA := accountv1.NewAccountRepository(storeA)
	accountA := accountv1.NewAccount(aggregateIDA)

	commandIDA := eventsourcing.GenerateID()
	accountA.SetCommandID(commandIDA)

	err = accountA.OpenAccount(tenantACtx, &accountv1.OpenAccountCommand{
		AccountId:      aggregateIDA,
		OwnerName:      "Alice",
		InitialBalance: "1000.00",
	}, eventsourcing.EventMetadata{
		CausationID:   commandIDA,
		CorrelationID: eventsourcing.GenerateID(),
		PrincipalID:   "user-alice",
		TenantID:      "tenant-a",
	})
	if err != nil {
		t.Fatalf("Failed to open account for tenant A: %v", err)
	}

	_, err = repoA.SaveWithCommand(accountA, commandIDA)
	if err != nil {
		t.Fatalf("Failed to save account for tenant A: %v", err)
	}

	// Tenant B context
	tenantBCtx := WithTenantID(context.Background(), "tenant-b")

	// Tenant B: Create account with SAME local ID "acc-001"
	aggregateIDB := ComposeAggregateID("tenant-b", "acc-001")

	storeB, err := multiStore.GetStore(tenantBCtx)
	if err != nil {
		t.Fatalf("Failed to get store for tenant B: %v", err)
	}

	repoB := accountv1.NewAccountRepository(storeB)
	accountB := accountv1.NewAccount(aggregateIDB)

	commandIDB := eventsourcing.GenerateID()
	accountB.SetCommandID(commandIDB)

	err = accountB.OpenAccount(tenantBCtx, &accountv1.OpenAccountCommand{
		AccountId:      aggregateIDB,
		OwnerName:      "Bob",
		InitialBalance: "2000.00",
	}, eventsourcing.EventMetadata{
		CausationID:   commandIDB,
		CorrelationID: eventsourcing.GenerateID(),
		PrincipalID:   "user-bob",
		TenantID:      "tenant-b",
	})
	if err != nil {
		t.Fatalf("Failed to open account for tenant B: %v", err)
	}

	_, err = repoB.SaveWithCommand(accountB, commandIDB)
	if err != nil {
		t.Fatalf("Failed to save account for tenant B: %v", err)
	}

	// Verify tenant A's account
	loadedA, err := repoA.Load(aggregateIDA)
	if err != nil {
		t.Fatalf("Failed to load tenant A account: %v", err)
	}

	if loadedA.OwnerName != "Alice" {
		t.Errorf("Expected owner Alice, got %s", loadedA.OwnerName)
	}

	if loadedA.Balance != "1000.00" {
		t.Errorf("Expected balance 1000.00, got %s", loadedA.Balance)
	}

	// Verify tenant B's account
	loadedB, err := repoB.Load(aggregateIDB)
	if err != nil {
		t.Fatalf("Failed to load tenant B account: %v", err)
	}

	if loadedB.OwnerName != "Bob" {
		t.Errorf("Expected owner Bob, got %s", loadedB.OwnerName)
	}

	if loadedB.Balance != "2000.00" {
		t.Errorf("Expected balance 2000.00, got %s", loadedB.Balance)
	}

	// Verify aggregate IDs are properly scoped
	if loadedA.AccountId != "tenant-a::acc-001" {
		t.Errorf("Expected aggregate ID tenant-a::acc-001, got %s", loadedA.AccountId)
	}

	if loadedB.AccountId != "tenant-b::acc-001" {
		t.Errorf("Expected aggregate ID tenant-b::acc-001, got %s", loadedB.AccountId)
	}
}

func TestComposeDecomposeAggregateID(t *testing.T) {
	tests := []struct {
		name        string
		tenantID    string
		aggregateID string
		compositeID string
	}{
		{
			name:        "Simple tenant and aggregate",
			tenantID:    "tenant-a",
			aggregateID: "acc-123",
			compositeID: "tenant-a::acc-123",
		},
		{
			name:        "UUID-style IDs",
			tenantID:    "550e8400-e29b-41d4-a716-446655440000",
			aggregateID: "123e4567-e89b-12d3-a456-426614174000",
			compositeID: "550e8400-e29b-41d4-a716-446655440000::123e4567-e89b-12d3-a456-426614174000",
		},
		{
			name:        "Empty tenant ID",
			tenantID:    "",
			aggregateID: "acc-123",
			compositeID: "acc-123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test composition
			compositeID := ComposeAggregateID(tt.tenantID, tt.aggregateID)
			if compositeID != tt.compositeID {
				t.Errorf("ComposeAggregateID() = %v, want %v", compositeID, tt.compositeID)
			}

			// Test decomposition
			tenantID, aggregateID, err := DecomposeAggregateID(compositeID)
			if err != nil {
				t.Fatalf("DecomposeAggregateID() error = %v", err)
			}

			if tenantID != tt.tenantID {
				t.Errorf("DecomposeAggregateID() tenantID = %v, want %v", tenantID, tt.tenantID)
			}

			if aggregateID != tt.aggregateID {
				t.Errorf("DecomposeAggregateID() aggregateID = %v, want %v", aggregateID, tt.aggregateID)
			}
		})
	}
}

func TestValidateTenantID(t *testing.T) {
	tests := []struct {
		name           string
		compositeID    string
		expectedTenant string
		wantErr        bool
	}{
		{
			name:           "Matching tenant",
			compositeID:    "tenant-a::acc-123",
			expectedTenant: "tenant-a",
			wantErr:        false,
		},
		{
			name:           "Mismatched tenant",
			compositeID:    "tenant-b::acc-123",
			expectedTenant: "tenant-a",
			wantErr:        true,
		},
		{
			name:           "No tenant prefix",
			compositeID:    "acc-123",
			expectedTenant: "tenant-a",
			wantErr:        false, // Empty tenant ID is allowed (single-tenant mode)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTenantID(tt.compositeID, tt.expectedTenant)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTenantID() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTenantContext(t *testing.T) {
	ctx := context.Background()

	// No tenant ID in context
	if HasTenantID(ctx) {
		t.Error("Expected no tenant ID in empty context")
	}

	_, err := GetTenantID(ctx)
	if err == nil {
		t.Error("Expected error when getting tenant ID from empty context")
	}

	// Add tenant ID
	ctx = WithTenantID(ctx, "tenant-abc")

	if !HasTenantID(ctx) {
		t.Error("Expected tenant ID in context")
	}

	tenantID, err := GetTenantID(ctx)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if tenantID != "tenant-abc" {
		t.Errorf("Expected tenant-abc, got %s", tenantID)
	}

	// MustGetTenantID should not panic
	tenantID = MustGetTenantID(ctx)
	if tenantID != "tenant-abc" {
		t.Errorf("Expected tenant-abc, got %s", tenantID)
	}

	// MustGetTenantID should panic on empty context
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic when calling MustGetTenantID on empty context")
		}
	}()

	emptyCtx := context.Background()
	MustGetTenantID(emptyCtx)
}

func TestDatabasePerTenant(t *testing.T) {
	// Create database-per-tenant store
	multiStore, err := NewMultiTenantEventStore(MultiTenantConfig{
		Strategy:             DatabasePerTenant,
		DatabasePathTemplate: "/tmp/test_tenant_%s.db",
		WALMode:              true,
	})
	if err != nil {
		t.Fatalf("Failed to create multi-tenant store: %v", err)
	}
	defer multiStore.Close()

	// Tenant X context
	tenantXCtx := WithTenantID(context.Background(), "tenant-x")

	// Tenant X: Create account with simple local ID
	storeX, err := multiStore.GetStore(tenantXCtx)
	if err != nil {
		t.Fatalf("Failed to get store for tenant X: %v", err)
	}

	repoX := accountv1.NewAccountRepository(storeX)
	accountX := accountv1.NewAccount("acc-001") // No tenant prefix needed!

	commandIDX := eventsourcing.GenerateID()
	accountX.SetCommandID(commandIDX)

	err = accountX.OpenAccount(tenantXCtx, &accountv1.OpenAccountCommand{
		AccountId:      "acc-001",
		OwnerName:      "Xavier",
		InitialBalance: "5000.00",
	}, eventsourcing.EventMetadata{
		CausationID:   commandIDX,
		CorrelationID: eventsourcing.GenerateID(),
		PrincipalID:   "user-xavier",
		TenantID:      "tenant-x",
	})
	if err != nil {
		t.Fatalf("Failed to open account for tenant X: %v", err)
	}

	_, err = repoX.SaveWithCommand(accountX, commandIDX)
	if err != nil {
		t.Fatalf("Failed to save account for tenant X: %v", err)
	}

	// Tenant Y context - same local ID as tenant X
	tenantYCtx := WithTenantID(context.Background(), "tenant-y")

	storeY, err := multiStore.GetStore(tenantYCtx)
	if err != nil {
		t.Fatalf("Failed to get store for tenant Y: %v", err)
	}

	repoY := accountv1.NewAccountRepository(storeY)
	accountY := accountv1.NewAccount("acc-001") // Same ID!

	commandIDY := eventsourcing.GenerateID()
	accountY.SetCommandID(commandIDY)

	err = accountY.OpenAccount(tenantYCtx, &accountv1.OpenAccountCommand{
		AccountId:      "acc-001",
		OwnerName:      "Yolanda",
		InitialBalance: "6000.00",
	}, eventsourcing.EventMetadata{
		CausationID:   commandIDY,
		CorrelationID: eventsourcing.GenerateID(),
		PrincipalID:   "user-yolanda",
		TenantID:      "tenant-y",
	})
	if err != nil {
		t.Fatalf("Failed to open account for tenant Y: %v", err)
	}

	_, err = repoY.SaveWithCommand(accountY, commandIDY)
	if err != nil {
		t.Fatalf("Failed to save account for tenant Y: %v", err)
	}

	// Verify tenant X's account
	loadedX, err := repoX.Load("acc-001")
	if err != nil {
		t.Fatalf("Failed to load tenant X account: %v", err)
	}

	if loadedX.OwnerName != "Xavier" {
		t.Errorf("Expected owner Xavier, got %s", loadedX.OwnerName)
	}

	// Verify tenant Y's account
	loadedY, err := repoY.Load("acc-001")
	if err != nil {
		t.Fatalf("Failed to load tenant Y account: %v", err)
	}

	if loadedY.OwnerName != "Yolanda" {
		t.Errorf("Expected owner Yolanda, got %s", loadedY.OwnerName)
	}
}
