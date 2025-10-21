package multitenancy

import (
	"context"
	"fmt"
)

// contextKey is a private type for context keys to avoid collisions
type contextKey string

const (
	tenantIDKey contextKey = "tenant_id"
)

// WithTenantID adds a tenant ID to the context
func WithTenantID(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, tenantIDKey, tenantID)
}

// GetTenantID retrieves the tenant ID from the context
func GetTenantID(ctx context.Context) (string, error) {
	tenantID, ok := ctx.Value(tenantIDKey).(string)
	if !ok || tenantID == "" {
		return "", fmt.Errorf("tenant ID not found in context")
	}
	return tenantID, nil
}

// MustGetTenantID retrieves the tenant ID from the context or panics
func MustGetTenantID(ctx context.Context) string {
	tenantID, err := GetTenantID(ctx)
	if err != nil {
		panic(err)
	}
	return tenantID
}

// HasTenantID checks if the context contains a tenant ID
func HasTenantID(ctx context.Context) bool {
	_, err := GetTenantID(ctx)
	return err == nil
}
