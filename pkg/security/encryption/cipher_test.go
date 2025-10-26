package encryption

import (
	"bytes"
	"testing"
)

func TestGenerateKey(t *testing.T) {
	tests := []struct {
		name    string
		size    int
		wantErr bool
	}{
		{
			name:    "AES-128 (16 bytes)",
			size:    16,
			wantErr: false,
		},
		{
			name:    "AES-256 (32 bytes)",
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
			key, err := GenerateKey(tt.size)
			if (err != nil) != tt.wantErr {
				t.Errorf("GenerateKey() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && len(key) != tt.size {
				t.Errorf("GenerateKey() returned key of size %d, want %d", len(key), tt.size)
			}
		})
	}
}

func TestGenerateSalt(t *testing.T) {
	salt1, err := GenerateSalt()
	if err != nil {
		t.Fatalf("GenerateSalt() error = %v", err)
	}

	if len(salt1) != 32 {
		t.Errorf("GenerateSalt() returned salt of size %d, want 32", len(salt1))
	}

	// Generate another salt and ensure they're different
	salt2, err := GenerateSalt()
	if err != nil {
		t.Fatalf("GenerateSalt() error = %v", err)
	}

	if bytes.Equal(salt1, salt2) {
		t.Error("GenerateSalt() returned identical salts")
	}
}

func TestDeriveKey(t *testing.T) {
	salt, _ := GenerateSalt()
	password := "test-password-123"

	tests := []struct {
		name       string
		config     *Config
		wantKeyLen int
		wantErr    bool
	}{
		{
			name: "Argon2id default",
			config: &Config{
				KeyDerivation: KeyDerivationArgon2,
				KeySize:       32,
				Argon2Time:    1,
				Argon2Memory:  64 * 1024,
				Argon2Threads: 4,
			},
			wantKeyLen: 32,
			wantErr:    false,
		},
		{
			name: "PBKDF2",
			config: &Config{
				KeyDerivation:    KeyDerivationPBKDF2,
				KeySize:          32,
				PBKDF2Iterations: 10000,
			},
			wantKeyLen: 32,
			wantErr:    false,
		},
		{
			name: "AES-128",
			config: &Config{
				KeyDerivation: KeyDerivationArgon2,
				KeySize:       16,
				Argon2Time:    1,
				Argon2Memory:  64 * 1024,
				Argon2Threads: 2,
			},
			wantKeyLen: 16,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, err := DeriveKey(password, salt, tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("DeriveKey() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && len(key) != tt.wantKeyLen {
				t.Errorf("DeriveKey() returned key of size %d, want %d", len(key), tt.wantKeyLen)
			}
		})
	}
}

func TestCipher_EncryptDecrypt(t *testing.T) {
	key, _ := GenerateKey(32)
	cipher, err := NewCipher(key, DefaultConfig())
	if err != nil {
		t.Fatalf("NewCipher() error = %v", err)
	}

	tests := []struct {
		name      string
		plaintext []byte
	}{
		{
			name:      "short message",
			plaintext: []byte("Hello, World!"),
		},
		{
			name:      "empty message",
			plaintext: []byte(""),
		},
		{
			name:      "long message",
			plaintext: bytes.Repeat([]byte("test"), 1000),
		},
		{
			name:      "binary data",
			plaintext: []byte{0, 1, 2, 3, 255, 254, 253},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Encrypt
			ciphertext, err := cipher.Encrypt(tt.plaintext)
			if err != nil {
				t.Fatalf("Encrypt() error = %v", err)
			}

			// Verify ciphertext is different from plaintext
			if len(tt.plaintext) > 0 && bytes.Equal(ciphertext, tt.plaintext) {
				t.Error("Ciphertext equals plaintext")
			}

			// Decrypt
			decrypted, err := cipher.Decrypt(ciphertext)
			if err != nil {
				t.Fatalf("Decrypt() error = %v", err)
			}

			// Verify decrypted equals original
			if !bytes.Equal(decrypted, tt.plaintext) {
				t.Errorf("Decrypted text doesn't match original.\nGot:  %v\nWant: %v", decrypted, tt.plaintext)
			}
		})
	}
}

func TestCipher_EncryptDecryptString(t *testing.T) {
	key, _ := GenerateKey(32)
	cipher, err := NewCipher(key, DefaultConfig())
	if err != nil {
		t.Fatalf("NewCipher() error = %v", err)
	}

	testStrings := []string{
		"Hello, World!",
		"",
		"Unicode: 你好世界 🌍",
		"Special chars: !@#$%^&*()",
	}

	for _, plaintext := range testStrings {
		t.Run(plaintext, func(t *testing.T) {
			// Encrypt
			ciphertext, err := cipher.EncryptString(plaintext)
			if err != nil {
				t.Fatalf("EncryptString() error = %v", err)
			}

			// Decrypt
			decrypted, err := cipher.DecryptString(ciphertext)
			if err != nil {
				t.Fatalf("DecryptString() error = %v", err)
			}

			// Verify
			if decrypted != plaintext {
				t.Errorf("Decrypted string doesn't match.\nGot:  %s\nWant: %s", decrypted, plaintext)
			}
		})
	}
}

func TestCipher_DecryptInvalid(t *testing.T) {
	key, _ := GenerateKey(32)
	cipher, err := NewCipher(key, DefaultConfig())
	if err != nil {
		t.Fatalf("NewCipher() error = %v", err)
	}

	tests := []struct {
		name       string
		ciphertext []byte
	}{
		{
			name:       "empty ciphertext",
			ciphertext: []byte{},
		},
		{
			name:       "too short",
			ciphertext: []byte{1, 2, 3},
		},
		{
			name:       "random data",
			ciphertext: bytes.Repeat([]byte{0xFF}, 100),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := cipher.Decrypt(tt.ciphertext)
			if err == nil {
				t.Error("Decrypt() expected error, got nil")
			}
		})
	}
}

func TestCipher_WrongKey(t *testing.T) {
	key1, _ := GenerateKey(32)
	key2, _ := GenerateKey(32)

	cipher1, _ := NewCipher(key1, DefaultConfig())
	cipher2, _ := NewCipher(key2, DefaultConfig())

	plaintext := []byte("secret message")

	// Encrypt with key1
	ciphertext, err := cipher1.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}

	// Try to decrypt with key2 (should fail)
	_, err = cipher2.Decrypt(ciphertext)
	if err == nil {
		t.Error("Decrypt() with wrong key should fail")
	}
}

func TestNewCipherWithPassword(t *testing.T) {
	password := "strong-password-123"
	salt, _ := GenerateSalt()

	cipher, err := NewCipherWithPassword(password, salt, DefaultConfig())
	if err != nil {
		t.Fatalf("NewCipherWithPassword() error = %v", err)
	}

	// Test encryption/decryption
	plaintext := "test message"
	ciphertext, err := cipher.EncryptString(plaintext)
	if err != nil {
		t.Fatalf("EncryptString() error = %v", err)
	}

	decrypted, err := cipher.DecryptString(ciphertext)
	if err != nil {
		t.Fatalf("DecryptString() error = %v", err)
	}

	if decrypted != plaintext {
		t.Errorf("Decrypted doesn't match. Got: %s, Want: %s", decrypted, plaintext)
	}
}

func TestCipher_KeyID(t *testing.T) {
	key, _ := GenerateKey(32)
	cipher, _ := NewCipher(key, DefaultConfig())

	// Test initial key ID (should be empty)
	if cipher.KeyID() != "" {
		t.Errorf("Initial KeyID should be empty, got: %s", cipher.KeyID())
	}

	// Set key ID
	cipher.SetKeyID("test-key-123")

	// Verify
	if cipher.KeyID() != "test-key-123" {
		t.Errorf("KeyID = %s, want test-key-123", cipher.KeyID())
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.Algorithm != AlgorithmAES256GCM {
		t.Errorf("Default algorithm should be AES-256-GCM, got %s", config.Algorithm)
	}

	if config.KeyDerivation != KeyDerivationArgon2 {
		t.Errorf("Default key derivation should be Argon2, got %s", config.KeyDerivation)
	}

	if config.KeySize != 32 {
		t.Errorf("Default key size should be 32, got %d", config.KeySize)
	}
}

func TestFastConfig(t *testing.T) {
	config := FastConfig()

	if config.Argon2Memory >= 64*1024 {
		t.Errorf("Fast config should use less memory than default")
	}

	if config.Argon2Time >= 3 {
		t.Errorf("Fast config should use fewer iterations than default")
	}
}

func BenchmarkEncrypt(b *testing.B) {
	key, _ := GenerateKey(32)
	cipher, _ := NewCipher(key, DefaultConfig())
	plaintext := []byte("This is a test message for benchmarking")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = cipher.Encrypt(plaintext)
	}
}

func BenchmarkDecrypt(b *testing.B) {
	key, _ := GenerateKey(32)
	cipher, _ := NewCipher(key, DefaultConfig())
	plaintext := []byte("This is a test message for benchmarking")
	ciphertext, _ := cipher.Encrypt(plaintext)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = cipher.Decrypt(ciphertext)
	}
}

func BenchmarkDeriveKey_Argon2(b *testing.B) {
	password := "test-password"
	salt, _ := GenerateSalt()
	config := DefaultConfig()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = DeriveKey(password, salt, config)
	}
}

func BenchmarkDeriveKey_PBKDF2(b *testing.B) {
	password := "test-password"
	salt, _ := GenerateSalt()
	config := &Config{
		KeyDerivation:    KeyDerivationPBKDF2,
		KeySize:          32,
		PBKDF2Iterations: 100000,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = DeriveKey(password, salt, config)
	}
}
