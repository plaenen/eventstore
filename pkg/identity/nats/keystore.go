package nats

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/plaenen/eventstore/pkg/errorx"
	"github.com/plaenen/eventstore/pkg/security/encryption"
)

// KeyStore defines the interface for storing and retrieving NATS keys (seeds).
// Implementations must ensure secure storage of these sensitive values.
type KeyStore interface {
	// SaveSeed stores a seed (private key) for a given ID (e.g., tenant ID).
	SaveSeed(ctx context.Context, id string, seed []byte) error

	// GetSeed retrieves the seed for a given ID.
	GetSeed(ctx context.Context, id string) ([]byte, error)
}

// MemoryKeyStore is an in-memory implementation of KeyStore for testing.
type MemoryKeyStore struct {
	mu    sync.RWMutex
	seeds map[string][]byte
}

// NewMemoryKeyStore creates a new MemoryKeyStore.
func NewMemoryKeyStore() *MemoryKeyStore {
	return &MemoryKeyStore{
		seeds: make(map[string][]byte),
	}
}

// SaveSeed stores a seed in memory.
func (m *MemoryKeyStore) SaveSeed(ctx context.Context, id string, seed []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seeds[id] = seed
	return nil
}

// GetSeed retrieves a seed from memory.
func (m *MemoryKeyStore) GetSeed(ctx context.Context, id string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	seed, ok := m.seeds[id]
	if !ok {
		return nil, errorx.ErrNotFound
	}
	return seed, nil
}

// FileKeyStore is a file-based implementation of KeyStore.
// It stores seeds in a directory, with one file per ID.
// Seeds are encrypted at rest using the provided encryption service.
type FileKeyStore struct {
	baseDir    string
	encryption *encryption.Service
}

// NewFileKeyStore creates a new FileKeyStore.
func NewFileKeyStore(baseDir string, encService *encryption.Service) (*FileKeyStore, error) {
	if err := os.MkdirAll(baseDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create keystore directory: %w", err)
	}
	return &FileKeyStore{
		baseDir:    baseDir,
		encryption: encService,
	}, nil
}

// SaveSeed stores a seed in a file, encrypted.
func (f *FileKeyStore) SaveSeed(ctx context.Context, id string, seed []byte) error {
	path := filepath.Join(f.baseDir, id+".nk")

	// Encrypt the seed
	encryptedSeed, err := f.encryption.Encrypt(seed)
	if err != nil {
		return errorx.Wrap(err, "failed to encrypt seed")
	}

	// Write with 0600 permissions (read/write only by owner)
	if err := os.WriteFile(path, []byte(encryptedSeed), 0600); err != nil {
		return fmt.Errorf("failed to write seed file: %w", err)
	}
	return nil
}

// GetSeed retrieves a seed from a file and decrypts it.
func (f *FileKeyStore) GetSeed(ctx context.Context, id string) ([]byte, error) {
	path := filepath.Join(f.baseDir, id+".nk")
	encryptedSeed, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errorx.ErrNotFound
		}
		return nil, fmt.Errorf("failed to read seed file: %w", err)
	}

	// Decrypt the seed
	seed, err := f.encryption.Decrypt(string(encryptedSeed))
	if err != nil {
		return nil, errorx.Wrap(err, "failed to decrypt seed")
	}

	return seed, nil
}
