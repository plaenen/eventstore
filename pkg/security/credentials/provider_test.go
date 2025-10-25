package credentials

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCredentials_Validate(t *testing.T) {
	tests := []struct {
		name    string
		creds   *Credentials
		wantErr bool
	}{
		{
			name: "valid token credential",
			creds: &Credentials{
				Type:  CredentialTypeToken,
				Token: "test-token",
			},
			wantErr: false,
		},
		{
			name: "valid user/password credential",
			creds: &Credentials{
				Type:     CredentialTypeUserPassword,
				User:     "admin",
				Password: "secret",
			},
			wantErr: false,
		},
		{
			name: "valid nkey credential",
			creds: &Credentials{
				Type:      CredentialTypeNKey,
				PublicKey: "UABC123",
				Seed:      "SUABC123",
			},
			wantErr: false,
		},
		{
			name: "missing type",
			creds: &Credentials{
				Token: "test-token",
			},
			wantErr: true,
		},
		{
			name: "token credential missing token",
			creds: &Credentials{
				Type: CredentialTypeToken,
			},
			wantErr: true,
		},
		{
			name: "user/password missing user",
			creds: &Credentials{
				Type:     CredentialTypeUserPassword,
				Password: "secret",
			},
			wantErr: true,
		},
		{
			name: "user/password missing password",
			creds: &Credentials{
				Type: CredentialTypeUserPassword,
				User: "admin",
			},
			wantErr: true,
		},
		{
			name: "nkey missing public key",
			creds: &Credentials{
				Type: CredentialTypeNKey,
				Seed: "SUABC123",
			},
			wantErr: true,
		},
		{
			name: "nkey missing seed",
			creds: &Credentials{
				Type:      CredentialTypeNKey,
				PublicKey: "UABC123",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.creds.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCredentials_IsExpired(t *testing.T) {
	tests := []struct {
		name    string
		creds   *Credentials
		expired bool
	}{
		{
			name: "not expired",
			creds: &Credentials{
				Type:      CredentialTypeToken,
				Token:     "test-token",
				ExpiresAt: timePtr(time.Now().Add(1 * time.Hour)),
			},
			expired: false,
		},
		{
			name: "expired",
			creds: &Credentials{
				Type:      CredentialTypeToken,
				Token:     "test-token",
				ExpiresAt: timePtr(time.Now().Add(-1 * time.Hour)),
			},
			expired: true,
		},
		{
			name: "no expiration",
			creds: &Credentials{
				Type:  CredentialTypeToken,
				Token: "test-token",
			},
			expired: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expired, tt.creds.IsExpired())
		})
	}
}

func TestCredentials_MarshalJSON(t *testing.T) {
	creds := &Credentials{
		Type:      CredentialTypeUserPassword,
		User:      "admin",
		Password:  "super-secret",
		Token:     "also-secret",
		ExpiresAt: timePtr(time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC)),
		Metadata: map[string]string{
			"environment": "production",
		},
	}

	data, err := json.Marshal(creds)
	require.NoError(t, err)

	// Verify sensitive fields are redacted
	assert.Contains(t, string(data), `"password":"***"`)
	assert.NotContains(t, string(data), "super-secret")

	// Verify non-sensitive fields are present
	assert.Contains(t, string(data), `"user":"admin"`)
	assert.Contains(t, string(data), `"type":"user_password"`)
}

func TestSecretData_Validation(t *testing.T) {
	secretData := &SecretData{
		Credentials: &Credentials{
			Type:  CredentialTypeToken,
			Token: "test-token",
		},
		Version:   1,
		CreatedAt: time.Now(),
		Metadata: map[string]string{
			"created_by": "test",
		},
	}

	// Test JSON marshaling/unmarshaling
	data, err := json.Marshal(secretData)
	require.NoError(t, err)

	var decoded SecretData
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, secretData.Version, decoded.Version)
	assert.Equal(t, secretData.Credentials.Type, decoded.Credentials.Type)
	assert.Equal(t, secretData.Credentials.Token, decoded.Credentials.Token)
}

func TestCredentialTypes(t *testing.T) {
	// Verify credential type constants
	assert.Equal(t, CredentialType("token"), CredentialTypeToken)
	assert.Equal(t, CredentialType("user_password"), CredentialTypeUserPassword)
	assert.Equal(t, CredentialType("nkey"), CredentialTypeNKey)
	assert.Equal(t, CredentialType("jwt"), CredentialTypeJWT)
	assert.Equal(t, CredentialType("mtls"), CredentialTypeMTLS)
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	assert.Equal(t, 5*time.Minute, config.CacheTTL)
	assert.True(t, config.AutoRefresh)
	assert.Equal(t, 2*time.Minute+30*time.Second, config.RefreshInterval)
}

// Helper function
func timePtr(t time.Time) *time.Time {
	return &t
}
