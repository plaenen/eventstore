package credentials

import (
	"context"
	"fmt"
	"os"
	"time"
)

// StaticProvider provides credentials from a static value
// USE ONLY FOR DEVELOPMENT - NOT FOR PRODUCTION
type StaticProvider struct {
	creds *Credentials
}

// NewStaticTokenProvider creates a provider with a static token
// USE ONLY FOR DEVELOPMENT - tokens should come from secure storage in production
func NewStaticTokenProvider(token string, ttl time.Duration) *StaticProvider {
	var expiresAt *time.Time
	if ttl > 0 {
		exp := time.Now().Add(ttl)
		expiresAt = &exp
	}

	return &StaticProvider{
		creds: &Credentials{
			Type:      CredentialTypeToken,
			Token:     token,
			ExpiresAt: expiresAt,
			Metadata: map[string]string{
				"provider": "static",
				"warning":  "USE ONLY FOR DEVELOPMENT",
			},
		},
	}
}

// NewStaticUserPasswordProvider creates a provider with static username/password
// USE ONLY FOR DEVELOPMENT
func NewStaticUserPasswordProvider(user, password string) *StaticProvider {
	return &StaticProvider{
		creds: &Credentials{
			Type:     CredentialTypeUserPassword,
			User:     user,
			Password: password,
			Metadata: map[string]string{
				"provider": "static",
				"warning":  "USE ONLY FOR DEVELOPMENT",
			},
		},
	}
}

// GetCredentials returns the static credentials
func (p *StaticProvider) GetCredentials(ctx context.Context) (*Credentials, error) {
	if p.creds.IsExpired() {
		return nil, ErrCredentialsExpired
	}
	return p.creds, nil
}

// Rotate is not supported for static providers
func (p *StaticProvider) Rotate(ctx context.Context) error {
	return fmt.Errorf("rotation not supported for static provider")
}

// Type returns the credential type
func (p *StaticProvider) Type() CredentialType {
	return p.creds.Type
}

// Close releases resources (no-op for static)
func (p *StaticProvider) Close() error {
	return nil
}

// EnvProvider provides credentials from environment variables
// More secure than static since env vars can be injected at runtime
type EnvProvider struct {
	tokenVar     string
	userVar      string
	passwordVar  string
	credType     CredentialType
	cacheTTL     time.Duration
}

// NewEnvTokenProvider creates a provider that reads token from environment
func NewEnvTokenProvider(tokenEnvVar string, cacheTTL time.Duration) *EnvProvider {
	return &EnvProvider{
		tokenVar: tokenEnvVar,
		credType: CredentialTypeToken,
		cacheTTL: cacheTTL,
	}
}

// NewEnvUserPasswordProvider creates a provider that reads user/password from environment
func NewEnvUserPasswordProvider(userVar, passwordVar string) *EnvProvider {
	return &EnvProvider{
		userVar:     userVar,
		passwordVar: passwordVar,
		credType:    CredentialTypeUserPassword,
	}
}

// GetCredentials reads credentials from environment variables
func (p *EnvProvider) GetCredentials(ctx context.Context) (*Credentials, error) {
	var creds *Credentials

	switch p.credType {
	case CredentialTypeToken:
		token := os.Getenv(p.tokenVar)
		if token == "" {
			return nil, fmt.Errorf("environment variable %s not set", p.tokenVar)
		}

		var expiresAt *time.Time
		if p.cacheTTL > 0 {
			exp := time.Now().Add(p.cacheTTL)
			expiresAt = &exp
		}

		creds = &Credentials{
			Type:      CredentialTypeToken,
			Token:     token,
			ExpiresAt: expiresAt,
			Metadata: map[string]string{
				"provider": "environment",
				"env_var":  p.tokenVar,
			},
		}

	case CredentialTypeUserPassword:
		user := os.Getenv(p.userVar)
		password := os.Getenv(p.passwordVar)

		if user == "" || password == "" {
			return nil, fmt.Errorf("environment variables %s and %s must be set", p.userVar, p.passwordVar)
		}

		creds = &Credentials{
			Type:     CredentialTypeUserPassword,
			User:     user,
			Password: password,
			Metadata: map[string]string{
				"provider":      "environment",
				"user_var":      p.userVar,
				"password_var":  p.passwordVar,
			},
		}

	default:
		return nil, fmt.Errorf("unsupported credential type: %s", p.credType)
	}

	return creds, nil
}

// Rotate re-reads from environment (allows runtime updates)
func (p *EnvProvider) Rotate(ctx context.Context) error {
	// Environment variables can be updated at runtime
	// Just return nil to allow GetCredentials to re-read
	return nil
}

// Type returns the credential type
func (p *EnvProvider) Type() CredentialType {
	return p.credType
}

// Close releases resources (no-op for env)
func (p *EnvProvider) Close() error {
	return nil
}

// ChainProvider tries multiple providers in order until one succeeds
// Useful for fallback scenarios (e.g., try secret manager, fall back to env)
type ChainProvider struct {
	providers []Provider
}

// NewChainProvider creates a provider that chains multiple providers
func NewChainProvider(providers ...Provider) *ChainProvider {
	return &ChainProvider{
		providers: providers,
	}
}

// GetCredentials tries each provider in order
func (p *ChainProvider) GetCredentials(ctx context.Context) (*Credentials, error) {
	var lastErr error

	for i, provider := range p.providers {
		creds, err := provider.GetCredentials(ctx)
		if err == nil {
			return creds, nil
		}
		lastErr = fmt.Errorf("provider %d failed: %w", i, err)
	}

	if lastErr != nil {
		return nil, fmt.Errorf("all providers failed: %w", lastErr)
	}

	return nil, fmt.Errorf("no providers configured")
}

// Rotate rotates the first successful provider
func (p *ChainProvider) Rotate(ctx context.Context) error {
	var lastErr error

	for i, provider := range p.providers {
		if err := provider.Rotate(ctx); err == nil {
			return nil
		} else {
			lastErr = fmt.Errorf("provider %d rotation failed: %w", i, err)
		}
	}

	if lastErr != nil {
		return lastErr
	}

	return fmt.Errorf("no providers configured")
}

// Type returns the type from the first provider
func (p *ChainProvider) Type() CredentialType {
	if len(p.providers) > 0 {
		return p.providers[0].Type()
	}
	return ""
}

// Close closes all providers
func (p *ChainProvider) Close() error {
	var errs []error

	for _, provider := range p.providers {
		if err := provider.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to close %d provider(s): %v", len(errs), errs)
	}

	return nil
}
