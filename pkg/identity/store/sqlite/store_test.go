package sqlite

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/plaenen/eventstore/pkg/errorx"
	"github.com/plaenen/eventstore/pkg/security/encryption"
	_ "modernc.org/sqlite"
)

func TestSQLiteKeyStore(t *testing.T) {
	// Setup In-Memory SQLite
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open db: %v", err)
	}
	defer db.Close()

	// Setup Encryption Service
	key, _ := encryption.GenerateKey(32)
	encService, _ := encryption.NewService(key)

	ctx := context.Background()
	store, err := NewSQLiteKeyStore(ctx, db, encService)
	if err != nil {
		t.Fatalf("Failed to create SQLiteKeyStore: %v", err)
	}

	// Test Save and Get
	id := "test-id"
	seed := []byte("test-seed")

	if err := store.SaveSeed(ctx, id, seed); err != nil {
		t.Fatalf("SaveSeed failed: %v", err)
	}

	// Verify encryption (content should not contain seed)
	var encryptedSeed string
	err = db.QueryRow("SELECT seed FROM identity_seeds WHERE id = ?", id).Scan(&encryptedSeed)
	if err != nil {
		t.Fatalf("Failed to query seed from db: %v", err)
	}
	if strings.Contains(encryptedSeed, string(seed)) {
		t.Error("DB content should be encrypted, but contains plain seed")
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
