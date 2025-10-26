package encryption

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/plaenen/eventstore/pkg/security/credentials"
)

// NewServiceFromCredentialProvider creates an encryption service using a credential provider
// The credential's Token field should contain a base64-encoded encryption key
//
// Example:
//
//	// AWS Secrets Manager
//	provider, _ := credentials.NewSecretProvider(ctx,
//	    "awsparamstore:///prod/encryption/master-key")
//	service, _ := encryption.NewServiceFromCredentialProvider(ctx, provider)
//
//	// Environment variable
//	provider := credentials.NewEnvProvider("ENCRYPTION_KEY", nil)
//	service, _ := encryption.NewServiceFromCredentialProvider(ctx, provider)
func NewServiceFromCredentialProvider(ctx context.Context, provider credentials.Provider) (*Service, error) {
	// Get credentials
	creds, err := provider.GetCredentials(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get credentials: %w", err)
	}

	// Decode base64 key
	key, err := base64.StdEncoding.DecodeString(creds.Token)
	if err != nil {
		return nil, fmt.Errorf("failed to decode encryption key: %w", err)
	}

	// Validate key size
	if len(key) != 16 && len(key) != 32 {
		return nil, fmt.Errorf("invalid key size: got %d bytes, expected 16 or 32", len(key))
	}

	// Determine config based on key size
	config := DefaultConfig()
	if len(key) == 16 {
		config.Algorithm = AlgorithmAES128GCM
		config.KeySize = 16
	}

	// Create service
	return NewServiceWithConfig(key, config)
}

// NewServiceFromCredentialProviderWithConfig creates an encryption service with custom config
func NewServiceFromCredentialProviderWithConfig(ctx context.Context, provider credentials.Provider, config *Config) (*Service, error) {
	// Get credentials
	creds, err := provider.GetCredentials(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get credentials: %w", err)
	}

	// Decode base64 key
	key, err := base64.StdEncoding.DecodeString(creds.Token)
	if err != nil {
		return nil, fmt.Errorf("failed to decode encryption key: %w", err)
	}

	// Validate key size
	if len(key) != config.KeySize {
		return nil, fmt.Errorf("invalid key size: got %d bytes, expected %d", len(key), config.KeySize)
	}

	// Create service
	return NewServiceWithConfig(key, config)
}

// KeyHelper provides utilities for key management with credential providers
type KeyHelper struct {
	provider credentials.Provider
	config   *Config
}

// NewKeyHelper creates a new key helper
func NewKeyHelper(provider credentials.Provider, config *Config) *KeyHelper {
	if config == nil {
		config = DefaultConfig()
	}

	return &KeyHelper{
		provider: provider,
		config:   config,
	}
}

// LoadService loads an encryption service from the credential provider
func (kh *KeyHelper) LoadService(ctx context.Context) (*Service, error) {
	return NewServiceFromCredentialProviderWithConfig(ctx, kh.provider, kh.config)
}

// EncodeKey encodes a raw key to base64 for storage
func EncodeKey(key []byte) string {
	return base64.StdEncoding.EncodeToString(key)
}

// DecodeKey decodes a base64-encoded key
func DecodeKey(encodedKey string) ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(encodedKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decode key: %w", err)
	}

	if len(key) != 16 && len(key) != 32 {
		return nil, fmt.Errorf("invalid key size: got %d bytes, expected 16 or 32", len(key))
	}

	return key, nil
}

// GenerateAndEncodeKey generates a new key and returns it base64-encoded
func GenerateAndEncodeKey(size int) (string, error) {
	key, err := GenerateKey(size)
	if err != nil {
		return "", err
	}

	return EncodeKey(key), nil
}
