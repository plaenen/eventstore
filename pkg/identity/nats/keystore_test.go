package nats

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/plaenen/eventstore/pkg/errorx"
	"github.com/plaenen/eventstore/pkg/security/encryption"
)

func TestMemoryKeyStore(t *testing.T) {
	store := NewMemoryKeyStore()
	ctx := context.Background()

	// Test Save and Get
	id := "test-id"
	seed := []byte("test-seed")

	if err := store.SaveSeed(ctx, id, seed); err != nil {
		t.Fatalf("SaveSeed failed: %v", err)
	}

	got, err := store.GetSeed(ctx, id)
	if err != nil {
		t.Fatalf("GetSeed failed: %v", err)
	}

	if string(got) != string(seed) {
		t.Errorf("Expected seed %s, got %s", seed, got)
	}

	// Test Get Non-Existent
	_, err = store.GetSeed(ctx, "non-existent")
	if err != errorx.ErrNotFound {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}

func TestFileKeyStore(t *testing.T) {
	// Setup temp dir
	tmpDir, err := os.MkdirTemp("", "keystore-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Setup Encryption Service
	key, _ := encryption.GenerateKey(32)
	encService, _ := encryption.NewService(key)

	store, err := NewFileKeyStore(tmpDir, encService)
	if err != nil {
		t.Fatalf("Failed to create FileKeyStore: %v", err)
	}
	ctx := context.Background()

	// Test Save and Get
	id := "test-id"
	seed := []byte("test-seed")

	if err := store.SaveSeed(ctx, id, seed); err != nil {
		t.Fatalf("SaveSeed failed: %v", err)
	}

	// Verify file permissions
	path := filepath.Join(tmpDir, id+".nk")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Failed to stat seed file: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("Expected file permissions 0600, got %v", info.Mode().Perm())
	}

	// Verify encryption (content should not contain seed)
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read seed file: %v", err)
	}
	if strings.Contains(string(content), string(seed)) {
		t.Error("File content should be encrypted, but contains plain seed")
	}

	got, err := store.GetSeed(ctx, id)
	if err != nil {
		t.Fatalf("GetSeed failed: %v", err)
	}

	if string(got) != string(seed) {
		t.Errorf("Expected seed %s, got %s", seed, got)
	}

	// Test Get Non-Existent
	_, err = store.GetSeed(ctx, "non-existent")
	if err != errorx.ErrNotFound {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}
