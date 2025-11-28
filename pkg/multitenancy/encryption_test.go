package multitenancy

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/plaenen/eventstore/pkg/eventsourcing"
)

// MockKeyProvider implements TenantKeyProvider for testing
type MockKeyProvider struct {
	keys        map[string]map[string][]byte // tenantID -> keyID -> key
	currentKeys map[string]string            // tenantID -> currentKeyID
}

func NewMockKeyProvider() *MockKeyProvider {
	return &MockKeyProvider{
		keys:        make(map[string]map[string][]byte),
		currentKeys: make(map[string]string),
	}
}

func (m *MockKeyProvider) AddKey(tenantID, keyID string, key []byte) {
	if _, ok := m.keys[tenantID]; !ok {
		m.keys[tenantID] = make(map[string][]byte)
	}
	m.keys[tenantID][keyID] = key
}

func (m *MockKeyProvider) SetCurrentKeyID(tenantID, keyID string) {
	m.currentKeys[tenantID] = keyID
}

func (m *MockKeyProvider) GetCurrentKey(ctx context.Context, tenantID string) ([]byte, string, error) {
	keyID, ok := m.currentKeys[tenantID]
	if !ok {
		return nil, "", errors.New("no current key for tenant")
	}
	key, ok := m.keys[tenantID][keyID]
	if !ok {
		return nil, "", errors.New("current key not found")
	}
	return key, keyID, nil
}

func (m *MockKeyProvider) GetKey(ctx context.Context, tenantID, keyID string) ([]byte, error) {
	if keys, ok := m.keys[tenantID]; ok {
		if key, ok := keys[keyID]; ok {
			return key, nil
		}
	}
	return nil, errors.New("key not found")
}

// MockEventStore implements EventStore for testing
type MockEventStore struct {
	events map[string][]*eventsourcing.Event // aggregateID -> events
}

func NewMockEventStore() *MockEventStore {
	return &MockEventStore{
		events: make(map[string][]*eventsourcing.Event),
	}
}

func (m *MockEventStore) AppendEvents(aggregateID string, expectedVersion int64, events []*eventsourcing.Event) error {
	m.events[aggregateID] = append(m.events[aggregateID], events...)
	return nil
}

func (m *MockEventStore) LoadEvents(aggregateID string, afterVersion int64) ([]*eventsourcing.Event, error) {
	stored := m.events[aggregateID]
	result := make([]*eventsourcing.Event, len(stored))
	for i, event := range stored {
		// Deep copy
		clone := *event
		if event.Data != nil {
			clone.Data = make([]byte, len(event.Data))
			copy(clone.Data, event.Data)
		}
		// Metadata copy
		clone.Metadata = event.Metadata
		if event.Metadata.Custom != nil {
			clone.Metadata.Custom = make(map[string]string)
			for k, v := range event.Metadata.Custom {
				clone.Metadata.Custom[k] = v
			}
		}
		result[i] = &clone
	}
	return result, nil
}

// Implement other methods as no-ops or panics if not used
func (m *MockEventStore) AppendEventsIdempotent(aggregateID string, expectedVersion int64, events []*eventsourcing.Event, commandID string, ttl time.Duration) (*eventsourcing.CommandResult, error) {
	return nil, nil
}
func (m *MockEventStore) GetCommandResult(commandID string) (*eventsourcing.CommandResult, error) {
	return nil, nil
}
func (m *MockEventStore) LoadAllEvents(fromPosition int64, limit int) ([]*eventsourcing.Event, error) {
	return nil, nil
}
func (m *MockEventStore) GetAggregateVersion(aggregateID string) (int64, error) { return 0, nil }
func (m *MockEventStore) CheckUniqueness(indexName, value string) (bool, string, error) {
	return true, "", nil
}
func (m *MockEventStore) GetConstraintOwner(indexName, value string) (string, error) { return "", nil }
func (m *MockEventStore) RebuildConstraints() error                                  { return nil }
func (m *MockEventStore) SeedEvents(aggregateID string, expectedVersion int64, events []*eventsourcing.Event, opts *eventsourcing.SeedOptions) (*eventsourcing.SeedResult, error) {
	return nil, nil
}
func (m *MockEventStore) LoadUnpublishedEvents(limit int) ([]*eventsourcing.EventEnvelope, error) {
	return nil, nil
}
func (m *MockEventStore) MarkEventsPublished(eventIDs []string) error          { return nil }
func (m *MockEventStore) RecordPublishFailure(eventID string, err error) error { return nil }
func (m *MockEventStore) Close() error                                         { return nil }

func TestEncryptingEventStore(t *testing.T) {
	// Setup
	mockStore := NewMockEventStore()
	keyProvider := NewMockKeyProvider()

	// Key 1 (32 bytes for AES-256)
	key1 := make([]byte, 32)
	copy(key1, []byte("12345678901234567890123456789012"))
	keyProvider.AddKey("tenant-a", "v1", key1)
	keyProvider.SetCurrentKeyID("tenant-a", "v1")

	store := NewEncryptingEventStore(mockStore, keyProvider)

	// Test Data
	originalData := []byte("secret-payload")
	event := &eventsourcing.Event{
		ID:          "evt-1",
		AggregateID: "agg-1",
		Data:        originalData,
		Metadata: eventsourcing.EventMetadata{
			TenantID: "tenant-a",
		},
	}

	// 1. Test Append (Encryption)
	err := store.AppendEvents("agg-1", 0, []*eventsourcing.Event{event})
	if err != nil {
		t.Fatalf("AppendEvents failed: %v", err)
	}

	// Verify inner store has encrypted data
	storedEvents := mockStore.events["agg-1"]
	if len(storedEvents) != 1 {
		t.Fatalf("Expected 1 stored event, got %d", len(storedEvents))
	}
	storedEvent := storedEvents[0]

	if bytes.Equal(storedEvent.Data, originalData) {
		t.Error("Stored event data matches plaintext! Encryption failed.")
	}

	if storedEvent.Metadata.Custom[MetadataKeyID] != "v1" {
		t.Errorf("Expected key ID v1, got %s", storedEvent.Metadata.Custom[MetadataKeyID])
	}
	if storedEvent.Metadata.Custom[MetadataAlgo] != EncryptionAlgoAESGCM {
		t.Errorf("Expected algo %s, got %s", EncryptionAlgoAESGCM, storedEvent.Metadata.Custom[MetadataAlgo])
	}

	// 2. Test Load (Decryption)
	loadedEvents, err := store.LoadEvents("agg-1", 0)
	if err != nil {
		t.Fatalf("LoadEvents failed: %v", err)
	}
	if len(loadedEvents) != 1 {
		t.Fatalf("Expected 1 loaded event, got %d", len(loadedEvents))
	}

	if !bytes.Equal(loadedEvents[0].Data, originalData) {
		t.Errorf("Decrypted data mismatch. Got %s, want %s", loadedEvents[0].Data, originalData)
	}

	// 3. Test Key Rotation
	// Add Key 2
	key2 := make([]byte, 32)
	copy(key2, []byte("abcdefabcdefabcdefabcdefabcdefab"))
	keyProvider.AddKey("tenant-a", "v2", key2)
	keyProvider.SetCurrentKeyID("tenant-a", "v2")

	// Append new event with Key 2
	event2 := &eventsourcing.Event{
		ID:          "evt-2",
		AggregateID: "agg-1",
		Data:        []byte("secret-payload-2"),
		Metadata: eventsourcing.EventMetadata{
			TenantID: "tenant-a",
		},
	}
	err = store.AppendEvents("agg-1", 1, []*eventsourcing.Event{event2})
	if err != nil {
		t.Fatalf("AppendEvents (v2) failed: %v", err)
	}

	// Load both events
	loadedEvents, err = store.LoadEvents("agg-1", 0)
	if err != nil {
		t.Fatalf("LoadEvents (all) failed: %v", err)
	}
	if len(loadedEvents) != 2 {
		t.Fatalf("Expected 2 loaded events, got %d", len(loadedEvents))
	}

	// Verify Event 1 (decrypted with v1)
	if !bytes.Equal(loadedEvents[0].Data, originalData) {
		t.Errorf("Event 1 decryption failed after rotation")
	}
	// Verify Event 2 (decrypted with v2)
	if !bytes.Equal(loadedEvents[1].Data, []byte("secret-payload-2")) {
		t.Errorf("Event 2 decryption failed")
	}

	// Verify metadata on stored events
	if mockStore.events["agg-1"][0].Metadata.Custom[MetadataKeyID] != "v1" {
		t.Error("Event 1 should still have key ID v1")
	}
	if mockStore.events["agg-1"][1].Metadata.Custom[MetadataKeyID] != "v2" {
		t.Error("Event 2 should have key ID v2")
	}
}
