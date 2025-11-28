package multitenancy

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestMultiTenantEventStore_LRU(t *testing.T) {
	// Create temp directory for DBs
	tmpDir, err := os.MkdirTemp("", "multitenancy_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Config with MaxOpenTenants = 2
	config := MultiTenantConfig{
		Strategy:             DatabasePerTenant,
		DatabasePathTemplate: "file:" + filepath.Join(tmpDir, "tenant_%s.db"),
		MaxOpenTenants:       2,
		WALMode:              false,
	}

	store, err := NewMultiTenantEventStore(config)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Helper to access a tenant store
	accessTenant := func(id string) {
		t.Helper()
		tenantCtx := WithTenantID(ctx, id)
		s, err := store.GetStore(tenantCtx)
		if err != nil {
			t.Fatalf("failed to get store for %s: %v", id, err)
		}
		// Verify it's open (we can't easily check internal state, but we can use it)
		_, err = s.GetAggregateVersion("test-agg")
		if err != nil {
			t.Fatalf("failed to use store for %s: %v", id, err)
		}
	}

	// 1. Access Tenant A (Count: 1)
	accessTenant("tenant-a")
	if store.lruList.Len() != 1 {
		t.Errorf("expected 1 open store, got %d", store.lruList.Len())
	}

	// 2. Access Tenant B (Count: 2)
	accessTenant("tenant-b")
	if store.lruList.Len() != 2 {
		t.Errorf("expected 2 open stores, got %d", store.lruList.Len())
	}

	// 3. Access Tenant C (Count: 2, Tenant A should be evicted)
	accessTenant("tenant-c")
	if store.lruList.Len() != 2 {
		t.Errorf("expected 2 open stores, got %d", store.lruList.Len())
	}

	// Verify Tenant A is evicted (not in map)
	store.tenantStoresMu.Lock()
	_, existsA := store.tenantStores["tenant-a"]
	_, existsB := store.tenantStores["tenant-b"]
	_, existsC := store.tenantStores["tenant-c"]
	store.tenantStoresMu.Unlock()

	if existsA {
		t.Error("expected tenant-a to be evicted")
	}
	if !existsB {
		t.Error("expected tenant-b to remain")
	}
	if !existsC {
		t.Error("expected tenant-c to remain")
	}

	// 4. Access Tenant A again (Count: 2, Tenant B should be evicted as it was least recently used before C?)
	// Wait, order was A, B. C accessed.
	// LRU state before C: [B, A] (Front -> Back)
	// Access C: Evict A. State: [C, B]
	// Access A: Evict B. State: [A, C]
	accessTenant("tenant-a")

	store.tenantStoresMu.Lock()
	_, existsA = store.tenantStores["tenant-a"]
	_, existsB = store.tenantStores["tenant-b"]
	_, existsC = store.tenantStores["tenant-c"]
	store.tenantStoresMu.Unlock()

	if !existsA {
		t.Error("expected tenant-a to be re-opened")
	}
	if existsB {
		t.Error("expected tenant-b to be evicted")
	}
	if !existsC {
		t.Error("expected tenant-c to remain")
	}
}

func TestMultiTenantEventStore_SharedDatabase(t *testing.T) {
	// Create temp directory for DBs
	tmpDir, err := os.MkdirTemp("", "multitenancy_shared_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	config := MultiTenantConfig{
		Strategy:  SharedDatabase,
		SharedDSN: "file:" + filepath.Join(tmpDir, "shared.db"),
		WALMode:   false,
	}

	store, err := NewMultiTenantEventStore(config)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	tenantCtx := WithTenantID(ctx, "tenant-a")

	s, err := store.GetStore(tenantCtx)
	if err != nil {
		t.Fatalf("failed to get store: %v", err)
	}

	if s == nil {
		t.Fatal("expected store, got nil")
	}
}
