package multitenancy

import (
	"context"
	"fmt"
	"sync"

	"github.com/plaenen/eventstore/pkg/eventsourcing"
	"github.com/plaenen/eventstore/pkg/sqlite"
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
	tenantStores   map[string]eventsourcing.EventStore
	tenantStoresMu sync.RWMutex
	config         MultiTenantConfig
}

type MultiTenantConfig struct {
	Strategy TenantStoreStrategy

	// For SharedDatabase strategy
	SharedDSN string
	WALMode   bool

	// For DatabasePerTenant strategy
	DatabasePathTemplate string // e.g., "./data/tenant_%s.db"
}

// NewMultiTenantEventStore creates a new multi-tenant event store
func NewMultiTenantEventStore(config MultiTenantConfig) (*MultiTenantEventStore, error) {
	store := &MultiTenantEventStore{
		strategy:     config.Strategy,
		tenantStores: make(map[string]eventsourcing.EventStore),
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
		store.sharedStore = sharedStore
	}

	return store, nil
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
	// Try read lock first
	m.tenantStoresMu.RLock()
	store, exists := m.tenantStores[tenantID]
	m.tenantStoresMu.RUnlock()

	if exists {
		return store, nil
	}

	// Need to create store - acquire write lock
	m.tenantStoresMu.Lock()
	defer m.tenantStoresMu.Unlock()

	// Double-check after acquiring write lock
	store, exists = m.tenantStores[tenantID]
	if exists {
		return store, nil
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

	m.tenantStores[tenantID] = tenantStore
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

	for tenantID, store := range m.tenantStores {
		if err := store.Close(); err != nil {
			return fmt.Errorf("failed to close store for tenant %s: %w", tenantID, err)
		}
	}

	return nil
}

// GetTenantEventStore returns the appropriate event store for the tenant in the context
// This is a helper function to get the correct event store based on tenant context
func (m *MultiTenantEventStore) GetTenantEventStore(ctx context.Context) (eventsourcing.EventStore, error) {
	return m.GetStore(ctx)
}
