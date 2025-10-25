// Package credentials provides secure credential management using Go Cloud Development Kit.
//
// This package wraps gocloud.dev/secrets to provide a vendor-agnostic credential storage
// solution that works across AWS Secrets Manager, GCP Secret Manager, Azure Key Vault,
// HashiCorp Vault, and local development.
//
// Example usage:
//
//	// Production: AWS Secrets Manager
//	provider, err := credentials.NewSecretProvider(ctx, "awskms://arn:aws:secretsmanager:...")
//
//	// Development: Local file
//	provider, err := credentials.NewSecretProvider(ctx, "file:///path/to/secret")
//
//	// Get credentials
//	creds, err := provider.GetCredentials(ctx)
package credentials

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var (
	// ErrCredentialsExpired is returned when credentials have expired
	ErrCredentialsExpired = errors.New("credentials expired")

	// ErrInvalidCredentials is returned when credentials are malformed
	ErrInvalidCredentials = errors.New("invalid credentials")

	// ErrProviderClosed is returned when attempting to use a closed provider
	ErrProviderClosed = errors.New("provider is closed")
)

// CredentialType defines the type of credential
type CredentialType string

const (
	// CredentialTypeToken represents a simple bearer token
	CredentialTypeToken CredentialType = "token"

	// CredentialTypeNKey represents NATS NKey authentication
	CredentialTypeNKey CredentialType = "nkey"

	// CredentialTypeJWT represents JWT-based authentication
	CredentialTypeJWT CredentialType = "jwt"

	// CredentialTypeUserPassword represents username/password authentication
	CredentialTypeUserPassword CredentialType = "user_password"

	// CredentialTypeMTLS represents mutual TLS authentication
	CredentialTypeMTLS CredentialType = "mtls"
)

// Credentials represents authentication credentials with metadata
type Credentials struct {
	// Type specifies the credential type
	Type CredentialType `json:"type"`

	// Token for token-based authentication
	Token string `json:"token,omitempty"`

	// User for username/password authentication
	User string `json:"user,omitempty"`

	// Password for username/password authentication (stored encrypted)
	Password string `json:"password,omitempty"`

	// PublicKey for NKey authentication
	PublicKey string `json:"public_key,omitempty"`

	// Seed for NKey authentication (stored encrypted)
	Seed string `json:"seed,omitempty"`

	// JWTToken for JWT authentication
	JWTToken string `json:"jwt_token,omitempty"`

	// CertPEM for mTLS authentication
	CertPEM string `json:"cert_pem,omitempty"`

	// KeyPEM for mTLS authentication (stored encrypted)
	KeyPEM string `json:"key_pem,omitempty"`

	// ExpiresAt indicates when credentials expire (optional)
	ExpiresAt *time.Time `json:"expires_at,omitempty"`

	// Metadata for additional context
	Metadata map[string]string `json:"metadata,omitempty"`
}

// IsExpired checks if the credentials have expired
func (c *Credentials) IsExpired() bool {
	if c.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*c.ExpiresAt)
}

// Validate ensures credentials are well-formed for their type
func (c *Credentials) Validate() error {
	if c.Type == "" {
		return fmt.Errorf("%w: type is required", ErrInvalidCredentials)
	}

	switch c.Type {
	case CredentialTypeToken:
		if c.Token == "" {
			return fmt.Errorf("%w: token is required", ErrInvalidCredentials)
		}

	case CredentialTypeUserPassword:
		if c.User == "" || c.Password == "" {
			return fmt.Errorf("%w: user and password are required", ErrInvalidCredentials)
		}

	case CredentialTypeNKey:
		if c.PublicKey == "" || c.Seed == "" {
			return fmt.Errorf("%w: public_key and seed are required", ErrInvalidCredentials)
		}

	case CredentialTypeJWT:
		if c.JWTToken == "" {
			return fmt.Errorf("%w: jwt_token is required", ErrInvalidCredentials)
		}

	case CredentialTypeMTLS:
		if c.CertPEM == "" || c.KeyPEM == "" {
			return fmt.Errorf("%w: cert_pem and key_pem are required", ErrInvalidCredentials)
		}
	}

	return nil
}

// MarshalJSON implements custom JSON marshaling to prevent sensitive data leakage
func (c *Credentials) MarshalJSON() ([]byte, error) {
	// Create a sanitized copy for marshaling
	type Alias Credentials
	sanitized := &struct {
		Password string `json:"password,omitempty"`
		Seed     string `json:"seed,omitempty"`
		KeyPEM   string `json:"key_pem,omitempty"`
		*Alias
	}{
		Password: "***",  // Redact password
		Seed:     "***",  // Redact seed
		KeyPEM:   "***",  // Redact private key
		Alias:    (*Alias)(c),
	}

	return json.Marshal(sanitized)
}

// Provider defines the interface for credential providers
type Provider interface {
	// GetCredentials retrieves the current credentials
	GetCredentials(ctx context.Context) (*Credentials, error)

	// Rotate triggers credential rotation (if supported)
	Rotate(ctx context.Context) error

	// Type returns the credential type this provider manages
	Type() CredentialType

	// Close releases any resources held by the provider
	Close() error
}

// SecretData represents the structure stored in the secret backend
type SecretData struct {
	Credentials *Credentials          `json:"credentials"`
	Version     int                   `json:"version"`
	CreatedAt   time.Time             `json:"created_at"`
	Metadata    map[string]string     `json:"metadata,omitempty"`
}

// ProviderConfig configures credential providers
type ProviderConfig struct {
	// URL is the secret backend URL (e.g., "awskms://...", "file://...")
	URL string

	// CacheTTL is how long to cache credentials (default: 5 minutes)
	CacheTTL time.Duration

	// AutoRefresh enables automatic credential refresh
	AutoRefresh bool

	// RefreshInterval is how often to refresh (default: CacheTTL / 2)
	RefreshInterval time.Duration
}

// DefaultConfig returns a default provider configuration
func DefaultConfig() ProviderConfig {
	return ProviderConfig{
		CacheTTL:        5 * time.Minute,
		AutoRefresh:     true,
		RefreshInterval: 2*time.Minute + 30*time.Second, // Refresh at 50% of TTL
	}
}
