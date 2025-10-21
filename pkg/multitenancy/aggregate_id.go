package multitenancy

import (
	"fmt"
	"strings"
)

const (
	// TenantSeparator is used to separate tenant ID from aggregate ID
	TenantSeparator = "::"
)

// ComposeAggregateID creates a tenant-scoped aggregate ID
// Format: {tenantID}::{aggregateID}
// Example: tenant-abc::acc-123
func ComposeAggregateID(tenantID, aggregateID string) string {
	if tenantID == "" {
		return aggregateID // No tenant ID, return as-is
	}
	return fmt.Sprintf("%s%s%s", tenantID, TenantSeparator, aggregateID)
}

// DecomposeAggregateID splits a tenant-scoped aggregate ID into tenant ID and aggregate ID
// Returns (tenantID, aggregateID, nil) or ("", aggregateID, nil) if no tenant prefix
func DecomposeAggregateID(compositeID string) (string, string, error) {
	parts := strings.SplitN(compositeID, TenantSeparator, 2)

	if len(parts) == 1 {
		// No tenant prefix
		return "", parts[0], nil
	}

	if len(parts) == 2 {
		return parts[0], parts[1], nil
	}

	return "", "", fmt.Errorf("invalid composite aggregate ID: %s", compositeID)
}

// ExtractTenantID extracts just the tenant ID from a composite aggregate ID
func ExtractTenantID(compositeID string) (string, error) {
	tenantID, _, err := DecomposeAggregateID(compositeID)
	return tenantID, err
}

// ExtractAggregateID extracts just the aggregate ID (without tenant prefix)
func ExtractAggregateID(compositeID string) (string, error) {
	_, aggregateID, err := DecomposeAggregateID(compositeID)
	return aggregateID, err
}

// ValidateTenantID checks if an aggregate ID belongs to a specific tenant
func ValidateTenantID(compositeID, expectedTenantID string) error {
	tenantID, err := ExtractTenantID(compositeID)
	if err != nil {
		return err
	}

	if tenantID != "" && tenantID != expectedTenantID {
		return fmt.Errorf("tenant mismatch: expected %s, got %s", expectedTenantID, tenantID)
	}

	return nil
}
