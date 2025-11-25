package multitenancy

import (
	"context"
	"testing"
)

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
