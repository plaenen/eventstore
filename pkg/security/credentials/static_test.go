package credentials

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStaticTokenProvider(t *testing.T) {
	provider := NewStaticTokenProvider("test-token", 1*time.Hour)
	defer provider.Close()

	assert.Equal(t, CredentialTypeToken, provider.Type())

	creds, err := provider.GetCredentials(context.Background())
	require.NoError(t, err)
	assert.Equal(t, CredentialTypeToken, creds.Type)
	assert.Equal(t, "test-token", creds.Token)
	assert.False(t, creds.IsExpired())
}

func TestStaticUserPasswordProvider(t *testing.T) {
	provider := NewStaticUserPasswordProvider("admin", "secret")
	defer provider.Close()

	assert.Equal(t, CredentialTypeUserPassword, provider.Type())

	creds, err := provider.GetCredentials(context.Background())
	require.NoError(t, err)
	assert.Equal(t, CredentialTypeUserPassword, creds.Type)
	assert.Equal(t, "admin", creds.User)
	assert.Equal(t, "secret", creds.Password)
	assert.False(t, creds.IsExpired())
}

func TestStaticProvider_Expiration(t *testing.T) {
	// Create provider with very short TTL
	provider := NewStaticTokenProvider("test-token", 1*time.Millisecond)
	defer provider.Close()

	// Wait for expiration
	time.Sleep(10 * time.Millisecond)

	// GetCredentials should return an error for expired credentials
	_, err := provider.GetCredentials(context.Background())
	assert.ErrorIs(t, err, ErrCredentialsExpired)
}

func TestStaticProvider_Rotate(t *testing.T) {
	provider := NewStaticTokenProvider("test-token", 1*time.Hour)
	defer provider.Close()

	// Rotation not supported for static provider
	err := provider.Rotate(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "rotation not supported")
}

func TestEnvTokenProvider(t *testing.T) {
	// Set environment variable
	envKey := "TEST_NATS_TOKEN_" + time.Now().Format("20060102150405")
	os.Setenv(envKey, "env-test-token")
	defer os.Unsetenv(envKey)

	provider := NewEnvTokenProvider(envKey, 5*time.Minute)
	defer provider.Close()

	assert.Equal(t, CredentialTypeToken, provider.Type())

	creds, err := provider.GetCredentials(context.Background())
	require.NoError(t, err)
	assert.Equal(t, CredentialTypeToken, creds.Type)
	assert.Equal(t, "env-test-token", creds.Token)
	assert.False(t, creds.IsExpired())
}

func TestEnvTokenProvider_MissingVariable(t *testing.T) {
	provider := NewEnvTokenProvider("NONEXISTENT_VAR", 5*time.Minute)
	defer provider.Close()

	_, err := provider.GetCredentials(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not set")
}

func TestEnvUserPasswordProvider(t *testing.T) {
	// Set environment variables
	userKey := "TEST_NATS_USER_" + time.Now().Format("20060102150405")
	passKey := "TEST_NATS_PASS_" + time.Now().Format("20060102150405")
	os.Setenv(userKey, "env-user")
	os.Setenv(passKey, "env-pass")
	defer os.Unsetenv(userKey)
	defer os.Unsetenv(passKey)

	provider := NewEnvUserPasswordProvider(userKey, passKey)
	defer provider.Close()

	assert.Equal(t, CredentialTypeUserPassword, provider.Type())

	creds, err := provider.GetCredentials(context.Background())
	require.NoError(t, err)
	assert.Equal(t, CredentialTypeUserPassword, creds.Type)
	assert.Equal(t, "env-user", creds.User)
	assert.Equal(t, "env-pass", creds.Password)
	assert.False(t, creds.IsExpired())
}

func TestEnvUserPasswordProvider_MissingUser(t *testing.T) {
	passKey := "TEST_NATS_PASS_" + time.Now().Format("20060102150405")
	os.Setenv(passKey, "env-pass")
	defer os.Unsetenv(passKey)

	provider := NewEnvUserPasswordProvider("NONEXISTENT_USER", passKey)
	defer provider.Close()

	_, err := provider.GetCredentials(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must be set")
}

func TestEnvUserPasswordProvider_MissingPassword(t *testing.T) {
	userKey := "TEST_NATS_USER_" + time.Now().Format("20060102150405")
	os.Setenv(userKey, "env-user")
	defer os.Unsetenv(userKey)

	provider := NewEnvUserPasswordProvider(userKey, "NONEXISTENT_PASS")
	defer provider.Close()

	_, err := provider.GetCredentials(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must be set")
}

func TestChainProvider_Success(t *testing.T) {
	// Create a chain with multiple providers
	provider1 := NewStaticTokenProvider("token1", 1*time.Hour)
	provider2 := NewStaticTokenProvider("token2", 1*time.Hour)
	provider3 := NewStaticTokenProvider("token3", 1*time.Hour)

	chain := NewChainProvider(provider1, provider2, provider3)
	defer chain.Close()

	// Should get credentials from first provider
	creds, err := chain.GetCredentials(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "token1", creds.Token)
}

func TestChainProvider_Fallback(t *testing.T) {
	// Create a chain where first provider fails
	envProvider := NewEnvTokenProvider("NONEXISTENT_VAR", 5*time.Minute)
	staticProvider := NewStaticTokenProvider("fallback-token", 1*time.Hour)

	chain := NewChainProvider(envProvider, staticProvider)
	defer chain.Close()

	// Should fall back to static provider
	creds, err := chain.GetCredentials(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "fallback-token", creds.Token)
}

func TestChainProvider_AllFail(t *testing.T) {
	// Create a chain where all providers fail
	provider1 := NewEnvTokenProvider("NONEXISTENT_VAR1", 5*time.Minute)
	provider2 := NewEnvTokenProvider("NONEXISTENT_VAR2", 5*time.Minute)

	chain := NewChainProvider(provider1, provider2)
	defer chain.Close()

	_, err := chain.GetCredentials(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "all providers failed")
}

func TestChainProvider_Empty(t *testing.T) {
	chain := NewChainProvider()
	defer chain.Close()

	_, err := chain.GetCredentials(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no providers")
}

func TestChainProvider_Type(t *testing.T) {
	provider1 := NewStaticTokenProvider("token1", 1*time.Hour)
	provider2 := NewStaticUserPasswordProvider("user", "pass")

	chain := NewChainProvider(provider1, provider2)
	defer chain.Close()

	// Type should return the type of the first provider
	assert.Equal(t, CredentialTypeToken, chain.Type())
}

func TestChainProvider_Rotate(t *testing.T) {
	provider1 := NewStaticTokenProvider("token1", 1*time.Hour)
	provider2 := NewStaticTokenProvider("token2", 1*time.Hour)

	chain := NewChainProvider(provider1, provider2)
	defer chain.Close()

	// Rotation should propagate to all providers
	err := chain.Rotate(context.Background())
	// Static providers don't support rotation, so this should fail
	assert.Error(t, err)
}

func TestChainProvider_Close(t *testing.T) {
	provider1 := NewStaticTokenProvider("token1", 1*time.Hour)
	provider2 := NewStaticTokenProvider("token2", 1*time.Hour)

	chain := NewChainProvider(provider1, provider2)

	// Close should succeed
	err := chain.Close()
	assert.NoError(t, err)
}
