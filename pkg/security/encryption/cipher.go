// Package encryption provides data encryption at rest for the EventSourcing framework.
//
// This package implements SEC-103 (Data Encryption at Rest) from the security roadmap,
// providing comprehensive encryption support for events, snapshots, and sensitive data.
//
// Example usage:
//
//	// Create encryption service with master key
//	masterKey := []byte("my-32-byte-master-key-here!!!")
//	encryptor, err := encryption.NewService(masterKey)
//
//	// Encrypt data
//	ciphertext, err := encryptor.Encrypt([]byte("sensitive data"))
//
//	// Decrypt data
//	plaintext, err := encryptor.Decrypt(ciphertext)
package encryption

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/pbkdf2"
)

var (
	// ErrInvalidCiphertext is returned when ciphertext is invalid or corrupted
	ErrInvalidCiphertext = errors.New("invalid ciphertext")

	// ErrInvalidKey is returned when encryption key is invalid
	ErrInvalidKey = errors.New("invalid encryption key")

	// ErrDecryptionFailed is returned when decryption fails
	ErrDecryptionFailed = errors.New("decryption failed")

	// ErrEncryptionFailed is returned when encryption fails
	ErrEncryptionFailed = errors.New("encryption failed")
)

// Algorithm represents the encryption algorithm
type Algorithm string

const (
	// AlgorithmAES256GCM uses AES-256 in GCM mode (recommended)
	AlgorithmAES256GCM Algorithm = "AES-256-GCM"

	// AlgorithmAES128GCM uses AES-128 in GCM mode
	AlgorithmAES128GCM Algorithm = "AES-128-GCM"
)

// KeyDerivation represents the key derivation function
type KeyDerivation string

const (
	// KeyDerivationArgon2 uses Argon2id (recommended for new applications)
	KeyDerivationArgon2 KeyDerivation = "argon2id"

	// KeyDerivationPBKDF2 uses PBKDF2-SHA256 (compatible with legacy systems)
	KeyDerivationPBKDF2 KeyDerivation = "pbkdf2-sha256"

	// KeyDerivationNone uses the key directly (only if key is already derived)
	KeyDerivationNone KeyDerivation = "none"
)

// Config represents encryption configuration
type Config struct {
	// Algorithm to use for encryption
	Algorithm Algorithm

	// KeyDerivation function to use
	KeyDerivation KeyDerivation

	// KeySize in bytes (16 for AES-128, 32 for AES-256)
	KeySize int

	// Argon2 parameters (used if KeyDerivation is argon2id)
	Argon2Time    uint32 // Number of iterations
	Argon2Memory  uint32 // Memory in KiB
	Argon2Threads uint8  // Number of threads

	// PBKDF2 parameters (used if KeyDerivation is pbkdf2)
	PBKDF2Iterations int
}

// DefaultConfig returns secure default encryption configuration
func DefaultConfig() *Config {
	return &Config{
		Algorithm:     AlgorithmAES256GCM,
		KeyDerivation: KeyDerivationArgon2,
		KeySize:       32, // AES-256

		// Argon2id parameters (OWASP recommendations)
		Argon2Time:    3,     // 3 iterations
		Argon2Memory:  64 * 1024, // 64 MiB
		Argon2Threads: 4,

		// PBKDF2 parameters (if used)
		PBKDF2Iterations: 600000, // OWASP recommendation 2023
	}
}

// FastConfig returns a faster but less secure configuration
// Suitable for development/testing only
func FastConfig() *Config {
	return &Config{
		Algorithm:     AlgorithmAES256GCM,
		KeyDerivation: KeyDerivationArgon2,
		KeySize:       32,

		// Reduced parameters for faster encryption
		Argon2Time:    1,
		Argon2Memory:  16 * 1024, // 16 MiB
		Argon2Threads: 2,

		PBKDF2Iterations: 10000,
	}
}

// Cipher provides encryption and decryption operations
type Cipher struct {
	config *Config
	gcm    cipher.AEAD
	keyID  string // Optional key identifier for key rotation
}

// NewCipher creates a new cipher with the given key
// The key should be properly derived using DeriveKey if it's a password
func NewCipher(key []byte, config *Config) (*Cipher, error) {
	if config == nil {
		config = DefaultConfig()
	}

	// Validate key size
	if len(key) != config.KeySize {
		return nil, fmt.Errorf("%w: expected %d bytes, got %d", ErrInvalidKey, config.KeySize, len(key))
	}

	// Create AES cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	return &Cipher{
		config: config,
		gcm:    gcm,
	}, nil
}

// NewCipherWithPassword creates a new cipher by deriving a key from a password
func NewCipherWithPassword(password string, salt []byte, config *Config) (*Cipher, error) {
	if config == nil {
		config = DefaultConfig()
	}

	// Derive key from password
	key, err := DeriveKey(password, salt, config)
	if err != nil {
		return nil, err
	}

	return NewCipher(key, config)
}

// Encrypt encrypts plaintext and returns ciphertext
// Format: nonce || ciphertext || tag
func (c *Cipher) Encrypt(plaintext []byte) ([]byte, error) {
	// Generate random nonce
	nonce := make([]byte, c.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("%w: failed to generate nonce: %v", ErrEncryptionFailed, err)
	}

	// Encrypt and authenticate
	ciphertext := c.gcm.Seal(nonce, nonce, plaintext, nil)

	return ciphertext, nil
}

// EncryptString encrypts a string and returns base64-encoded ciphertext
func (c *Cipher) EncryptString(plaintext string) (string, error) {
	ciphertext, err := c.Encrypt([]byte(plaintext))
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts ciphertext and returns plaintext
func (c *Cipher) Decrypt(ciphertext []byte) ([]byte, error) {
	nonceSize := c.gcm.NonceSize()

	// Validate ciphertext length
	if len(ciphertext) < nonceSize {
		return nil, ErrInvalidCiphertext
	}

	// Extract nonce and ciphertext
	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]

	// Decrypt and verify
	plaintext, err := c.gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDecryptionFailed, err)
	}

	return plaintext, nil
}

// DecryptString decrypts a base64-encoded ciphertext string
func (c *Cipher) DecryptString(ciphertext string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("%w: invalid base64: %v", ErrInvalidCiphertext, err)
	}

	plaintext, err := c.Decrypt(data)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

// SetKeyID sets an optional key identifier for key rotation
func (c *Cipher) SetKeyID(keyID string) {
	c.keyID = keyID
}

// KeyID returns the key identifier
func (c *Cipher) KeyID() string {
	return c.keyID
}

// DeriveKey derives an encryption key from a password using the configured KDF
func DeriveKey(password string, salt []byte, config *Config) ([]byte, error) {
	if config == nil {
		config = DefaultConfig()
	}

	// Validate salt
	if len(salt) < 16 {
		return nil, fmt.Errorf("salt must be at least 16 bytes")
	}

	switch config.KeyDerivation {
	case KeyDerivationArgon2:
		// Argon2id - recommended for new applications
		key := argon2.IDKey(
			[]byte(password),
			salt,
			config.Argon2Time,
			config.Argon2Memory,
			config.Argon2Threads,
			uint32(config.KeySize),
		)
		return key, nil

	case KeyDerivationPBKDF2:
		// PBKDF2-SHA256 - compatible with legacy systems
		key := pbkdf2.Key(
			[]byte(password),
			salt,
			config.PBKDF2Iterations,
			config.KeySize,
			sha256.New,
		)
		return key, nil

	case KeyDerivationNone:
		// No derivation - password must be exact key size
		if len(password) != config.KeySize {
			return nil, fmt.Errorf("password must be exactly %d bytes when using no derivation", config.KeySize)
		}
		return []byte(password), nil

	default:
		return nil, fmt.Errorf("unsupported key derivation: %s", config.KeyDerivation)
	}
}

// GenerateKey generates a random encryption key of the specified size
func GenerateKey(size int) ([]byte, error) {
	if size != 16 && size != 32 {
		return nil, fmt.Errorf("key size must be 16 or 32 bytes, got %d", size)
	}

	key := make([]byte, size)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("failed to generate key: %w", err)
	}

	return key, nil
}

// GenerateSalt generates a random salt for key derivation
func GenerateSalt() ([]byte, error) {
	salt := make([]byte, 32) // 256 bits
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("failed to generate salt: %w", err)
	}

	return salt, nil
}
