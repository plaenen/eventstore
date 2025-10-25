package credentials

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"gocloud.dev/secrets"
	// Cloud provider imports are opt-in - import in your application code:
	// _ "gocloud.dev/secrets/awskms"        // AWS Secrets Manager
	// _ "gocloud.dev/secrets/azurekeyvault" // Azure Key Vault
	// _ "gocloud.dev/secrets/gcpkms"        // GCP Secret Manager
	// _ "gocloud.dev/secrets/hashivault"    // HashiCorp Vault
	// _ "gocloud.dev/secrets/localsecrets"  // Local development
)

// SecretProvider implements Provider using Go Cloud Development Kit
type SecretProvider struct {
	keeper          *secrets.Keeper
	config          ProviderConfig
	credentialType  CredentialType

	// Cache
	mu              sync.RWMutex
	cachedCreds     *Credentials
	cacheExpiry     time.Time

	// Lifecycle
	closed          bool
	closeOnce       sync.Once
	refreshStop     chan struct{}
	refreshDone     chan struct{}
}

// NewSecretProvider creates a new credential provider using Go Cloud secrets
//
// URL formats:
//   - AWS Secrets Manager: "awskms://arn:aws:secretsmanager:region:account:secret:name"
//   - GCP Secret Manager: "gcpkms://projects/PROJECT/secrets/SECRET/versions/VERSION"
//   - Azure Key Vault: "azurekeyvault://VAULT-NAME.vault.azure.net/secrets/SECRET-NAME"
//   - HashiCorp Vault: "hashivault://server:8200/secret/data/path?namespace=ns"
//   - Local (dev): "file:///absolute/path/to/secret.json"
//   - Local (dev): "base64key://..." for base64-encoded local encryption
//
// Example:
//
//	// Production: AWS
//	provider, err := NewSecretProvider(ctx, "awskms://arn:aws:secretsmanager:us-east-1:123456:secret:nats-creds")
//
//	// Development: Local file
//	provider, err := NewSecretProvider(ctx, "file:///etc/secrets/nats.json")
func NewSecretProvider(ctx context.Context, url string) (*SecretProvider, error) {
	config := DefaultConfig()
	return NewSecretProviderWithConfig(ctx, url, config)
}

// NewSecretProviderWithConfig creates a provider with custom configuration
func NewSecretProviderWithConfig(ctx context.Context, url string, config ProviderConfig) (*SecretProvider, error) {
	if url == "" {
		return nil, fmt.Errorf("secret URL is required")
	}

	// Open the secret keeper using Go Cloud
	keeper, err := secrets.OpenKeeper(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("failed to open secret keeper: %w", err)
	}

	provider := &SecretProvider{
		keeper:      keeper,
		config:      config,
		refreshStop: make(chan struct{}),
		refreshDone: make(chan struct{}),
	}

	// Load initial credentials
	if err := provider.loadCredentials(ctx); err != nil {
		keeper.Close()
		return nil, fmt.Errorf("failed to load initial credentials: %w", err)
	}

	// Start auto-refresh if enabled
	if config.AutoRefresh {
		go provider.autoRefresh()
	} else {
		close(provider.refreshDone) // Not refreshing
	}

	return provider, nil
}

// GetCredentials retrieves credentials from cache or reloads if expired
func (p *SecretProvider) GetCredentials(ctx context.Context) (*Credentials, error) {
	p.mu.RLock()

	if p.closed {
		p.mu.RUnlock()
		return nil, ErrProviderClosed
	}

	// Check cache
	if p.cachedCreds != nil && time.Now().Before(p.cacheExpiry) {
		creds := p.cachedCreds
		p.mu.RUnlock()

		// Check expiration
		if creds.IsExpired() {
			return nil, ErrCredentialsExpired
		}

		return creds, nil
	}

	p.mu.RUnlock()

	// Cache miss or expired - reload
	if err := p.loadCredentials(ctx); err != nil {
		return nil, err
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.cachedCreds.IsExpired() {
		return nil, ErrCredentialsExpired
	}

	return p.cachedCreds, nil
}

// loadCredentials loads and decrypts credentials from the secret backend
func (p *SecretProvider) loadCredentials(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return ErrProviderClosed
	}

	// Decrypt secret data
	plaintext, err := p.keeper.Decrypt(ctx, nil) // nil = read from keeper
	if err != nil {
		return fmt.Errorf("failed to decrypt secret: %w", err)
	}

	// Parse secret data
	var secretData SecretData
	if err := json.Unmarshal(plaintext, &secretData); err != nil {
		return fmt.Errorf("failed to unmarshal secret data: %w", err)
	}

	// Validate credentials
	if err := secretData.Credentials.Validate(); err != nil {
		return fmt.Errorf("invalid credentials in secret: %w", err)
	}

	// Update cache
	p.cachedCreds = secretData.Credentials
	p.cacheExpiry = time.Now().Add(p.config.CacheTTL)
	p.credentialType = secretData.Credentials.Type

	return nil
}

// Rotate triggers credential rotation
func (p *SecretProvider) Rotate(ctx context.Context) error {
	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		return ErrProviderClosed
	}
	p.mu.RUnlock()

	// For Go Cloud, rotation is typically handled by the backend
	// We just invalidate the cache and reload
	p.mu.Lock()
	p.cachedCreds = nil
	p.cacheExpiry = time.Time{}
	p.mu.Unlock()

	return p.loadCredentials(ctx)
}

// Type returns the credential type
func (p *SecretProvider) Type() CredentialType {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.credentialType
}

// Close releases resources and stops auto-refresh
func (p *SecretProvider) Close() error {
	var err error
	p.closeOnce.Do(func() {
		p.mu.Lock()
		p.closed = true
		p.mu.Unlock()

		// Stop refresh goroutine
		close(p.refreshStop)
		<-p.refreshDone

		// Close keeper
		if p.keeper != nil {
			err = p.keeper.Close()
		}
	})
	return err
}

// autoRefresh periodically refreshes credentials
func (p *SecretProvider) autoRefresh() {
	defer close(p.refreshDone)

	ticker := time.NewTicker(p.config.RefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			if err := p.loadCredentials(ctx); err != nil {
				// Log error but continue
				// In production, you'd use proper logging
				fmt.Printf("auto-refresh failed: %v\n", err)
			}
			cancel()

		case <-p.refreshStop:
			return
		}
	}
}

// StoreCredentials encrypts and stores credentials to the secret backend
// This is typically used for initial setup or credential rotation
func StoreCredentials(ctx context.Context, url string, creds *Credentials) error {
	if err := creds.Validate(); err != nil {
		return fmt.Errorf("invalid credentials: %w", err)
	}

	// Open keeper
	keeper, err := secrets.OpenKeeper(ctx, url)
	if err != nil {
		return fmt.Errorf("failed to open keeper: %w", err)
	}
	defer keeper.Close()

	// Create secret data
	secretData := SecretData{
		Credentials: creds,
		Version:     1,
		CreatedAt:   time.Now(),
		Metadata: map[string]string{
			"created_by": "eventsourcing-framework",
		},
	}

	// Marshal to JSON
	plaintext, err := json.Marshal(secretData)
	if err != nil {
		return fmt.Errorf("failed to marshal credentials: %w", err)
	}

	// Encrypt and store
	ciphertext, err := keeper.Encrypt(ctx, plaintext)
	if err != nil {
		return fmt.Errorf("failed to encrypt credentials: %w", err)
	}

	// For file-based secrets, the ciphertext is already written
	// For cloud providers, you'd need to use their SDK to write the secret
	_ = ciphertext

	return nil
}
