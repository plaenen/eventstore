package encryption

import (
	"context"
	"testing"

	"github.com/plaenen/eventstore/pkg/security/credentials"
)

func TestNewServiceFromCredentialProvider(t *testing.T) {
	ctx := context.Background()

	// Generate a test key
	key, _ := GenerateKey(32)
	encodedKey := EncodeKey(key)

	// Create static provider with the key
	provider := credentials.NewStaticTokenProvider(encodedKey, 0)

	// Create service from provider
	service, err := NewServiceFromCredentialProvider(ctx, provider)
	if err != nil {
		t.Fatalf("NewServiceFromCredentialProvider() error = %v", err)
	}

	// Test encryption/decryption
	plaintext := "test data"
	ciphertext, err := service.EncryptString(plaintext)
	if err != nil {
		t.Fatalf("EncryptString() error = %v", err)
	}

	decrypted, err := service.DecryptString(ciphertext)
	if err != nil {
		t.Fatalf("DecryptString() error = %v", err)
	}

	if decrypted != plaintext {
		t.Errorf("Decrypted = %s, want %s", decrypted, plaintext)
	}
}

func TestNewServiceFromCredentialProvider_InvalidKey(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name      string
		token     string
		wantError bool
	}{
		{
			name:      "invalid base64",
			token:     "not-valid-base64!!!",
			wantError: true,
		},
		{
			name:      "wrong size key",
			token:     EncodeKey([]byte("too short")),
			wantError: true,
		},
		{
			name:      "valid 32-byte key",
			token:     EncodeKey(make([]byte, 32)),
			wantError: false,
		},
		{
			name:      "valid 16-byte key",
			token:     EncodeKey(make([]byte, 16)),
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := credentials.NewStaticTokenProvider(tt.token, 0)

			_, err := NewServiceFromCredentialProvider(ctx, provider)
			if (err != nil) != tt.wantError {
				t.Errorf("NewServiceFromCredentialProvider() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestEncodeDecodeKey(t *testing.T) {
	// Generate test key
	originalKey, _ := GenerateKey(32)

	// Encode
	encoded := EncodeKey(originalKey)

	// Decode
	decoded, err := DecodeKey(encoded)
	if err != nil {
		t.Fatalf("DecodeKey() error = %v", err)
	}

	// Verify
	if len(decoded) != len(originalKey) {
		t.Errorf("Decoded key length = %d, want %d", len(decoded), len(originalKey))
	}

	for i := range originalKey {
		if decoded[i] != originalKey[i] {
			t.Errorf("Decoded key differs at byte %d", i)
			break
		}
	}
}

func TestGenerateAndEncodeKey(t *testing.T) {
	tests := []struct {
		name    string
		size    int
		wantErr bool
	}{
		{
			name:    "AES-128",
			size:    16,
			wantErr: false,
		},
		{
			name:    "AES-256",
			size:    32,
			wantErr: false,
		},
		{
			name:    "invalid size",
			size:    24,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := GenerateAndEncodeKey(tt.size)
			if (err != nil) != tt.wantErr {
				t.Errorf("GenerateAndEncodeKey() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// Verify we can decode it
				decoded, err := DecodeKey(encoded)
				if err != nil {
					t.Errorf("DecodeKey() error = %v", err)
				}

				if len(decoded) != tt.size {
					t.Errorf("Decoded key size = %d, want %d", len(decoded), tt.size)
				}
			}
		})
	}
}

func TestKeyHelper(t *testing.T) {
	ctx := context.Background()

	// Generate and encode a key
	key, _ := GenerateKey(32)
	encodedKey := EncodeKey(key)

	// Create provider
	provider := credentials.NewStaticTokenProvider(encodedKey, 0)

	// Create key helper
	helper := NewKeyHelper(provider, DefaultConfig())

	// Load service
	service, err := helper.LoadService(ctx)
	if err != nil {
		t.Fatalf("LoadService() error = %v", err)
	}

	// Test encryption
	plaintext := "helper test data"
	ciphertext, err := service.EncryptString(plaintext)
	if err != nil {
		t.Fatalf("EncryptString() error = %v", err)
	}

	decrypted, err := service.DecryptString(ciphertext)
	if err != nil {
		t.Fatalf("DecryptString() error = %v", err)
	}

	if decrypted != plaintext {
		t.Errorf("Decrypted = %s, want %s", decrypted, plaintext)
	}
}

func TestNewServiceFromCredentialProviderWithConfig(t *testing.T) {
	ctx := context.Background()

	// Generate 16-byte key for AES-128
	key, _ := GenerateKey(16)
	encodedKey := EncodeKey(key)

	provider := credentials.NewStaticTokenProvider(encodedKey, 0)

	// Custom config for AES-128
	config := &Config{
		Algorithm: AlgorithmAES128GCM,
		KeySize:   16,
	}

	service, err := NewServiceFromCredentialProviderWithConfig(ctx, provider, config)
	if err != nil {
		t.Fatalf("NewServiceFromCredentialProviderWithConfig() error = %v", err)
	}

	// Test it works
	plaintext := "test data with custom config"
	ciphertext, err := service.EncryptString(plaintext)
	if err != nil {
		t.Fatalf("EncryptString() error = %v", err)
	}

	decrypted, err := service.DecryptString(ciphertext)
	if err != nil {
		t.Fatalf("DecryptString() error = %v", err)
	}

	if decrypted != plaintext {
		t.Errorf("Decrypted = %s, want %s", decrypted, plaintext)
	}
}
