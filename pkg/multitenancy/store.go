package multitenancy

import (
	"container/list"
	"context"
	"fmt"
	"sync"

	"github.com/plaenen/eventstore/pkg/eventsourcing"
	"github.com/plaenen/eventstore/pkg/eventsourcing/store/sqlite"
)

// TenantStoreStrategy defines how tenants are isolated at storage level
type TenantStoreStrategy int

const (
	// SharedDatabase - All tenants in same database with tenant-prefixed aggregate IDs
	SharedDatabase TenantStoreStrategy = iota

	// DatabasePerTenant - Each tenant gets their own database file
	DatabasePerTenant
)

// MultiTenantEventStore wraps an event store with multi-tenancy support
type MultiTenantEventStore struct {
	strategy       TenantStoreStrategy
	sharedStore    eventsourcing.EventStore // Used for SharedDatabase strategy
	tenantStores   map[string]*list.Element // Map tenantID to LRU element
	lruList        *list.List               // LRU list of tenant stores
	tenantStoresMu sync.Mutex               // Mutex for both map and list
	config         MultiTenantConfig
}

type MultiTenantConfig struct {
	Strategy TenantStoreStrategy

	// For SharedDatabase strategy
	SharedDSN string
	WALMode   bool

	// For DatabasePerTenant strategy
	DatabasePathTemplate string // e.g., "./data/tenant_%s.db"
	MaxOpenTenants       int    // Maximum number of open tenant databases (default: 100)
}

// tenantStoreEntry holds the store and its ID for the LRU list
type tenantStoreEntry struct {
	tenantID string
	store    eventsourcing.EventStore
}

// NewMultiTenantEventStore creates a new multi-tenant event store
func NewMultiTenantEventStore(config MultiTenantConfig) (*MultiTenantEventStore, error) {
	if config.MaxOpenTenants <= 0 {
		config.MaxOpenTenants = 100
	}

	mtStore := &MultiTenantEventStore{
		strategy:     config.Strategy,
		tenantStores: make(map[string]*list.Element),
		lruList:      list.New(),
		config:       config,
	}

	if config.Strategy == SharedDatabase {
		// Create single shared store
		sharedStore, err := sqlite.NewEventStore(
			sqlite.WithDSN(config.SharedDSN),
			sqlite.WithWALMode(config.WALMode),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create shared event store: %w", err)
		}
		mtStore.sharedStore = sharedStore
	}

	return mtStore, nil
}

// GetStore returns the event store for a specific tenant
func (m *MultiTenantEventStore) GetStore(ctx context.Context) (eventsourcing.EventStore, error) {
	if m.strategy == SharedDatabase {
		return m.sharedStore, nil
	}

	// DatabasePerTenant strategy
	tenantID, err := GetTenantID(ctx)
	if err != nil {
		return nil, err
	}

	return m.getOrCreateTenantStore(tenantID)
}

// getOrCreateTenantStore gets or creates a per-tenant database
func (m *MultiTenantEventStore) getOrCreateTenantStore(tenantID string) (eventsourcing.EventStore, error) {
	m.tenantStoresMu.Lock()
	defer m.tenantStoresMu.Unlock()

	// Check if store exists
	if element, exists := m.tenantStores[tenantID]; exists {
		m.lruList.MoveToFront(element)
		return element.Value.(*tenantStoreEntry).store, nil
	}

	// Check if we need to evict
	if m.lruList.Len() >= m.config.MaxOpenTenants {
		// Remove oldest
		oldest := m.lruList.Back()
		if oldest != nil {
			entry := oldest.Value.(*tenantStoreEntry)
			// Close the store
			if err := entry.store.Close(); err != nil {
				// Log error but continue
				fmt.Printf("failed to close evicted store for tenant %s: %v\n", entry.tenantID, err)
			}
			delete(m.tenantStores, entry.tenantID)
			m.lruList.Remove(oldest)
		}
	}

	// Create new tenant database
	dsn := fmt.Sprintf(m.config.DatabasePathTemplate, tenantID)
	tenantStore, err := sqlite.NewEventStore(
		sqlite.WithDSN(dsn),
		sqlite.WithWALMode(m.config.WALMode),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create tenant store for %s: %w", tenantID, err)
	}

	// Add to LRU
	entry := &tenantStoreEntry{
		tenantID: tenantID,
		store:    tenantStore,
	}
	element := m.lruList.PushFront(entry)
	m.tenantStores[tenantID] = element

	return tenantStore, nil
}

// Close closes all tenant stores
func (m *MultiTenantEventStore) Close() error {
	if m.sharedStore != nil {
		if err := m.sharedStore.Close(); err != nil {
			return err
		}
	}

	m.tenantStoresMu.Lock()
	defer m.tenantStoresMu.Unlock()

	for e := m.lruList.Front(); e != nil; e = e.Next() {
		entry := e.Value.(*tenantStoreEntry)
		if err := entry.store.Close(); err != nil {
			return fmt.Errorf("failed to close store for tenant %s: %w", entry.tenantID, err)
		}
	}

	return nil
}

// GetTenantEventStore returns the appropriate event store for the tenant in the context
// This is a helper function to get the correct event store based on tenant context
func (m *MultiTenantEventStore) GetTenantEventStore(ctx context.Context) (eventsourcing.EventStore, error) {
	return m.GetStore(ctx)
}
