package encryption

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	// ErrKeyNotFound is returned when a key is not found
	ErrKeyNotFound = errors.New("encryption key not found")

	// ErrNoActiveKey is returned when no active key is configured
	ErrNoActiveKey = errors.New("no active encryption key")
)

// KeyInfo contains metadata about an encryption key
type KeyInfo struct {
	// ID is a unique identifier for the key
	ID string `json:"id"`

	// Version of the key (increments on rotation)
	Version int `json:"version"`

	// CreatedAt is when the key was created
	CreatedAt time.Time `json:"created_at"`

	// RotatedAt is when the key was last rotated
	RotatedAt time.Time `json:"rotated_at,omitempty"`

	// ExpiresAt is when the key expires (optional)
	ExpiresAt time.Time `json:"expires_at,omitempty"`

	// Active indicates if this is the current encryption key
	Active bool `json:"active"`

	// Algorithm used with this key
	Algorithm Algorithm `json:"algorithm"`

	// Purpose describes what this key is used for
	Purpose string `json:"purpose,omitempty"`
}

// Key represents an encryption key with metadata
type Key struct {
	Info   KeyInfo
	Cipher *Cipher
}

// KeyManager manages encryption keys with support for key rotation
type KeyManager struct {
	mu      sync.RWMutex
	keys    map[string]*Key // key ID -> Key
	active  string          // Active key ID
	config  *Config
}

// NewKeyManager creates a new key manager
func NewKeyManager(config *Config) *KeyManager {
	if config == nil {
		config = DefaultConfig()
	}

	return &KeyManager{
		keys:   make(map[string]*Key),
		config: config,
	}
}

// AddKey adds a new encryption key
func (km *KeyManager) AddKey(id string, key []byte, active bool) error {
	km.mu.Lock()
	defer km.mu.Unlock()

	// Create cipher
	cipher, err := NewCipher(key, km.config)
	if err != nil {
		return err
	}
	cipher.SetKeyID(id)

	// Create key info
	info := KeyInfo{
		ID:        id,
		Version:   1,
		CreatedAt: time.Now(),
		Active:    active,
		Algorithm: km.config.Algorithm,
	}

	// If this key is active, deactivate others
	if active {
		for _, k := range km.keys {
			k.Info.Active = false
		}
		km.active = id
	}

	km.keys[id] = &Key{
		Info:   info,
		Cipher: cipher,
	}

	return nil
}

// AddKeyWithPassword adds a new encryption key derived from a password
func (km *KeyManager) AddKeyWithPassword(id, password string, salt []byte, active bool) error {
	key, err := DeriveKey(password, salt, km.config)
	if err != nil {
		return err
	}

	return km.AddKey(id, key, active)
}

// GetKey retrieves a key by ID
func (km *KeyManager) GetKey(id string) (*Key, error) {
	km.mu.RLock()
	defer km.mu.RUnlock()

	key, exists := km.keys[id]
	if !exists {
		return nil, ErrKeyNotFound
	}

	return key, nil
}

// GetActiveKey returns the current active encryption key
func (km *KeyManager) GetActiveKey() (*Key, error) {
	km.mu.RLock()
	defer km.mu.RUnlock()

	if km.active == "" {
		return nil, ErrNoActiveKey
	}

	key, exists := km.keys[km.active]
	if !exists {
		return nil, ErrNoActiveKey
	}

	return key, nil
}

// SetActiveKey sets the active encryption key
func (km *KeyManager) SetActiveKey(id string) error {
	km.mu.Lock()
	defer km.mu.Unlock()

	key, exists := km.keys[id]
	if !exists {
		return ErrKeyNotFound
	}

	// Deactivate all keys
	for _, k := range km.keys {
		k.Info.Active = false
	}

	// Activate the specified key
	key.Info.Active = true
	key.Info.RotatedAt = time.Now()
	km.active = id

	return nil
}

// RotateKey generates a new key and sets it as active
// Old keys are kept for decryption of existing data
func (km *KeyManager) RotateKey() (string, error) {
	km.mu.Lock()
	defer km.mu.Unlock()

	// Generate new key
	newKey, err := GenerateKey(km.config.KeySize)
	if err != nil {
		return "", fmt.Errorf("failed to generate new key: %w", err)
	}

	// Create cipher
	cipher, err := NewCipher(newKey, km.config)
	if err != nil {
		return "", err
	}

	// Generate key ID
	newKeyID := fmt.Sprintf("key-%d", time.Now().Unix())
	cipher.SetKeyID(newKeyID)

	// Determine version
	version := 1
	if km.active != "" {
		if oldKey, exists := km.keys[km.active]; exists {
			version = oldKey.Info.Version + 1
		}
	}

	// Create key info
	info := KeyInfo{
		ID:        newKeyID,
		Version:   version,
		CreatedAt: time.Now(),
		RotatedAt: time.Now(),
		Active:    true,
		Algorithm: km.config.Algorithm,
	}

	// Deactivate old keys
	for _, k := range km.keys {
		k.Info.Active = false
	}

	// Add new key
	km.keys[newKeyID] = &Key{
		Info:   info,
		Cipher: cipher,
	}

	km.active = newKeyID

	return newKeyID, nil
}

// ListKeys returns all keys
func (km *KeyManager) ListKeys() []KeyInfo {
	km.mu.RLock()
	defer km.mu.RUnlock()

	keys := make([]KeyInfo, 0, len(km.keys))
	for _, k := range km.keys {
		keys = append(keys, k.Info)
	}

	return keys
}

// RemoveKey removes a key (cannot remove active key)
func (km *KeyManager) RemoveKey(id string) error {
	km.mu.Lock()
	defer km.mu.Unlock()

	if id == km.active {
		return errors.New("cannot remove active key")
	}

	if _, exists := km.keys[id]; !exists {
		return ErrKeyNotFound
	}

	delete(km.keys, id)
	return nil
}

// ExportKeys exports key metadata (not the actual keys!) as JSON
// This is useful for backup and disaster recovery documentation
func (km *KeyManager) ExportKeys() (string, error) {
	km.mu.RLock()
	defer km.mu.RUnlock()

	keys := make([]KeyInfo, 0, len(km.keys))
	for _, k := range km.keys {
		keys = append(keys, k.Info)
	}

	data, err := json.MarshalIndent(keys, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to export keys: %w", err)
	}

	return string(data), nil
}

// Service provides high-level encryption/decryption operations with key management
type Service struct {
	keyManager *KeyManager
}

// NewService creates a new encryption service with a master key
func NewService(masterKey []byte) (*Service, error) {
	return NewServiceWithConfig(masterKey, DefaultConfig())
}

// NewServiceWithConfig creates a new encryption service with custom configuration
func NewServiceWithConfig(masterKey []byte, config *Config) (*Service, error) {
	km := NewKeyManager(config)

	// Add master key as the active key
	err := km.AddKey("master", masterKey, true)
	if err != nil {
		return nil, fmt.Errorf("failed to add master key: %w", err)
	}

	return &Service{
		keyManager: km,
	}, nil
}

// NewServiceWithPassword creates a new encryption service with a password
func NewServiceWithPassword(password string, salt []byte) (*Service, error) {
	return NewServiceWithPasswordAndConfig(password, salt, DefaultConfig())
}

// NewServiceWithPasswordAndConfig creates a new encryption service with password and config
func NewServiceWithPasswordAndConfig(password string, salt []byte, config *Config) (*Service, error) {
	km := NewKeyManager(config)

	// Derive key from password and add as master
	err := km.AddKeyWithPassword("master", password, salt, true)
	if err != nil {
		return nil, fmt.Errorf("failed to add master key: %w", err)
	}

	return &Service{
		keyManager: km,
	}, nil
}

// Encrypt encrypts data using the active encryption key
// Returns: base64(keyID:ciphertext)
func (s *Service) Encrypt(plaintext []byte) (string, error) {
	key, err := s.keyManager.GetActiveKey()
	if err != nil {
		return "", err
	}

	ciphertext, err := key.Cipher.Encrypt(plaintext)
	if err != nil {
		return "", err
	}

	// Prepend key ID to ciphertext for key rotation support
	// Format: keyID:base64(ciphertext)
	result := fmt.Sprintf("%s:%s", key.Info.ID, base64.StdEncoding.EncodeToString(ciphertext))

	return result, nil
}

// EncryptString encrypts a string
func (s *Service) EncryptString(plaintext string) (string, error) {
	return s.Encrypt([]byte(plaintext))
}

// Decrypt decrypts data encrypted by this service
// Automatically determines which key to use based on the key ID in the ciphertext
func (s *Service) Decrypt(ciphertext string) ([]byte, error) {
	// Parse keyID:ciphertext format
	keyID, encryptedData, err := parseEncryptedData(ciphertext)
	if err != nil {
		return nil, err
	}

	// Get the appropriate key
	key, err := s.keyManager.GetKey(keyID)
	if err != nil {
		return nil, fmt.Errorf("key %s not found: %w", keyID, err)
	}

	// Decrypt
	plaintext, err := key.Cipher.Decrypt(encryptedData)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}

// DecryptString decrypts a string
func (s *Service) DecryptString(ciphertext string) (string, error) {
	plaintext, err := s.Decrypt(ciphertext)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

// RotateKey rotates the encryption key
// Returns the new key ID
func (s *Service) RotateKey() (string, error) {
	return s.keyManager.RotateKey()
}

// ReEncrypt re-encrypts data with the current active key
// Use this after key rotation to update encrypted data
func (s *Service) ReEncrypt(oldCiphertext string) (string, error) {
	// Decrypt with old key
	plaintext, err := s.Decrypt(oldCiphertext)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt: %w", err)
	}

	// Encrypt with current active key
	newCiphertext, err := s.Encrypt(plaintext)
	if err != nil {
		return "", fmt.Errorf("failed to re-encrypt: %w", err)
	}

	return newCiphertext, nil
}

// KeyManager returns the underlying key manager
func (s *Service) KeyManager() *KeyManager {
	return s.keyManager
}

// parseEncryptedData parses the keyID:ciphertext format
func parseEncryptedData(data string) (keyID string, ciphertext []byte, err error) {
	// Find the separator
	sepIndex := -1
	for i, c := range data {
		if c == ':' {
			sepIndex = i
			break
		}
	}

	if sepIndex == -1 {
		return "", nil, fmt.Errorf("%w: invalid format, expected keyID:ciphertext", ErrInvalidCiphertext)
	}

	keyID = data[:sepIndex]
	encodedCiphertext := data[sepIndex+1:]

	// Decode base64
	ciphertext, err = base64.StdEncoding.DecodeString(encodedCiphertext)
	if err != nil {
		return "", nil, fmt.Errorf("%w: invalid base64: %v", ErrInvalidCiphertext, err)
	}

	return keyID, ciphertext, nil
}
