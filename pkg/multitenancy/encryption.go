package multitenancy

import (
	"context"
	"errors"
)

var (
	// ErrKeyNotFound is returned when a requested key cannot be found.
	ErrKeyNotFound = errors.New("encryption key not found")
)

// TenantKeyProvider defines the interface for retrieving tenant-specific encryption keys.
// Implementations should handle key storage, rotation, and access control.
type TenantKeyProvider interface {
	// GetCurrentKey returns the current active key for a tenant, used for encryption.
	// It returns the key bytes and the key ID (version).
	GetCurrentKey(ctx context.Context, tenantID string) (key []byte, keyID string, err error)

	// GetKey returns a specific key by ID for a tenant, used for decryption.
	GetKey(ctx context.Context, tenantID, keyID string) (key []byte, err error)
}
