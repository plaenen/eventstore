package multitenancy

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
	"time"

	"github.com/plaenen/eventstore/pkg/eventsourcing"
)

const (
	// EncryptionAlgoAESGCM is the identifier for AES-GCM encryption
	EncryptionAlgoAESGCM = "AES-GCM"

	// MetadataKeyID is the metadata key for the encryption key ID
	MetadataKeyID = "encryption_key_id"

	// MetadataAlgo is the metadata key for the encryption algorithm
	MetadataAlgo = "encryption_algo"
)

// EncryptingEventStore is a decorator that encrypts event payloads at rest.
type EncryptingEventStore struct {
	store       eventsourcing.EventStore
	keyProvider TenantKeyProvider
}

// NewEncryptingEventStore creates a new EncryptingEventStore.
func NewEncryptingEventStore(store eventsourcing.EventStore, keyProvider TenantKeyProvider) *EncryptingEventStore {
	return &EncryptingEventStore{
		store:       store,
		keyProvider: keyProvider,
	}
}

// encryptEvent encrypts the event data in place.
func (s *EncryptingEventStore) encryptEvent(event *eventsourcing.Event) error {
	tenantID := event.Metadata.TenantID
	if tenantID == "" {
		// Skip encryption for non-tenant events (or use a default system key?)
		// For now, we skip.
		return nil
	}

	// Get current key for tenant
	// Note: EventStore interface doesn't pass context, so we use Background
	ctx := context.Background()
	key, keyID, err := s.keyProvider.GetCurrentKey(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("failed to get encryption key for tenant %s: %w", tenantID, err)
	}

	// Create cipher block
	block, err := aes.NewCipher(key)
	if err != nil {
		return fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("failed to create GCM: %w", err)
	}

	// Generate nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt
	ciphertext := gcm.Seal(nonce, nonce, event.Data, nil)

	// Update event
	event.Data = ciphertext

	// Update metadata
	if event.Metadata.Custom == nil {
		event.Metadata.Custom = make(map[string]string)
	}
	event.Metadata.Custom[MetadataKeyID] = keyID
	event.Metadata.Custom[MetadataAlgo] = EncryptionAlgoAESGCM

	return nil
}

// decryptEvent decrypts the event data in place.
func (s *EncryptingEventStore) decryptEvent(event *eventsourcing.Event) error {
	// Check if encrypted
	if event.Metadata.Custom == nil {
		return nil
	}

	algo, ok := event.Metadata.Custom[MetadataAlgo]
	if !ok || algo != EncryptionAlgoAESGCM {
		return nil // Not encrypted or unknown algo
	}

	keyID, ok := event.Metadata.Custom[MetadataKeyID]
	if !ok {
		return fmt.Errorf("missing encryption key ID for event %s", event.ID)
	}

	tenantID := event.Metadata.TenantID
	if tenantID == "" {
		return fmt.Errorf("missing tenant ID for encrypted event %s", event.ID)
	}

	// Get key
	ctx := context.Background()
	key, err := s.keyProvider.GetKey(ctx, tenantID, keyID)
	if err != nil {
		return fmt.Errorf("failed to get decryption key %s for tenant %s: %w", keyID, tenantID, err)
	}

	// Create cipher block
	block, err := aes.NewCipher(key)
	if err != nil {
		return fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("failed to create GCM: %w", err)
	}

	// Decrypt
	nonceSize := gcm.NonceSize()
	if len(event.Data) < nonceSize {
		return fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := event.Data[:nonceSize], event.Data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return fmt.Errorf("failed to decrypt event %s: %w", event.ID, err)
	}

	// Update event
	event.Data = plaintext

	// We don't remove metadata so we know it was encrypted,
	// but maybe we should? For now keep it.

	return nil
}

// AppendEvents implements EventStore.AppendEvents
func (s *EncryptingEventStore) AppendEvents(aggregateID string, expectedVersion int64, events []*eventsourcing.Event) error {
	// Clone and encrypt events to avoid side effects
	encryptedEvents := make([]*eventsourcing.Event, len(events))
	for i, event := range events {
		cloned := s.cloneEvent(event)
		if err := s.encryptEvent(cloned); err != nil {
			return err
		}
		encryptedEvents[i] = cloned
	}
	return s.store.AppendEvents(aggregateID, expectedVersion, encryptedEvents)
}

// AppendEventsIdempotent implements EventStore.AppendEventsIdempotent
func (s *EncryptingEventStore) AppendEventsIdempotent(
	aggregateID string,
	expectedVersion int64,
	events []*eventsourcing.Event,
	commandID string,
	ttl time.Duration,
) (*eventsourcing.CommandResult, error) {
	// Clone and encrypt events
	encryptedEvents := make([]*eventsourcing.Event, len(events))
	for i, event := range events {
		cloned := s.cloneEvent(event)
		if err := s.encryptEvent(cloned); err != nil {
			return nil, err
		}
		encryptedEvents[i] = cloned
	}

	result, err := s.store.AppendEventsIdempotent(aggregateID, expectedVersion, encryptedEvents, commandID, ttl)
	if err != nil {
		return nil, err
	}

	// Decrypt events in result if present (CommandResult contains events)
	if result != nil && result.Events != nil {
		for _, event := range result.Events {
			if err := s.decryptEvent(event); err != nil {
				return nil, err
			}
		}
	}

	return result, nil
}

// GetCommandResult implements EventStore.GetCommandResult
func (s *EncryptingEventStore) GetCommandResult(commandID string) (*eventsourcing.CommandResult, error) {
	result, err := s.store.GetCommandResult(commandID)
	if err != nil {
		return nil, err
	}

	if result != nil && result.Events != nil {
		for _, event := range result.Events {
			if err := s.decryptEvent(event); err != nil {
				return nil, err
			}
		}
	}

	return result, nil
}

// LoadEvents implements EventStore.LoadEvents
func (s *EncryptingEventStore) LoadEvents(aggregateID string, afterVersion int64) ([]*eventsourcing.Event, error) {
	events, err := s.store.LoadEvents(aggregateID, afterVersion)
	if err != nil {
		return nil, err
	}

	for _, event := range events {
		if err := s.decryptEvent(event); err != nil {
			return nil, err
		}
	}

	return events, nil
}

// LoadAllEvents implements EventStore.LoadAllEvents
func (s *EncryptingEventStore) LoadAllEvents(fromPosition int64, limit int) ([]*eventsourcing.Event, error) {
	events, err := s.store.LoadAllEvents(fromPosition, limit)
	if err != nil {
		return nil, err
	}

	for _, event := range events {
		if err := s.decryptEvent(event); err != nil {
			return nil, err
		}
	}

	return events, nil
}

// GetAggregateVersion implements EventStore.GetAggregateVersion
func (s *EncryptingEventStore) GetAggregateVersion(aggregateID string) (int64, error) {
	return s.store.GetAggregateVersion(aggregateID)
}

// CheckUniqueness implements EventStore.CheckUniqueness
func (s *EncryptingEventStore) CheckUniqueness(indexName, value string) (bool, string, error) {
	return s.store.CheckUniqueness(indexName, value)
}

// GetConstraintOwner implements EventStore.GetConstraintOwner
func (s *EncryptingEventStore) GetConstraintOwner(indexName, value string) (string, error) {
	return s.store.GetConstraintOwner(indexName, value)
}

// RebuildConstraints implements EventStore.RebuildConstraints
func (s *EncryptingEventStore) RebuildConstraints() error {
	return s.store.RebuildConstraints()
}

// SeedEvents implements EventStore.SeedEvents
func (s *EncryptingEventStore) SeedEvents(aggregateID string, expectedVersion int64, events []*eventsourcing.Event, opts *eventsourcing.SeedOptions) (*eventsourcing.SeedResult, error) {
	// Clone and encrypt events
	encryptedEvents := make([]*eventsourcing.Event, len(events))
	for i, event := range events {
		cloned := s.cloneEvent(event)
		if err := s.encryptEvent(cloned); err != nil {
			return nil, err
		}
		encryptedEvents[i] = cloned
	}

	return s.store.SeedEvents(aggregateID, expectedVersion, encryptedEvents, opts)
}

// LoadUnpublishedEvents implements EventStore.LoadUnpublishedEvents
func (s *EncryptingEventStore) LoadUnpublishedEvents(limit int) ([]*eventsourcing.EventEnvelope, error) {
	envelopes, err := s.store.LoadUnpublishedEvents(limit)
	if err != nil {
		return nil, err
	}

	for _, envelope := range envelopes {
		if err := s.decryptEvent(&envelope.Event); err != nil {
			return nil, err
		}
	}

	return envelopes, nil
}

// MarkEventsPublished implements EventStore.MarkEventsPublished
func (s *EncryptingEventStore) MarkEventsPublished(eventIDs []string) error {
	return s.store.MarkEventsPublished(eventIDs)
}

// RecordPublishFailure implements EventStore.RecordPublishFailure
func (s *EncryptingEventStore) RecordPublishFailure(eventID string, err error) error {
	return s.store.RecordPublishFailure(eventID, err)
}

// Close implements EventStore.Close
func (s *EncryptingEventStore) Close() error {
	return s.store.Close()
}

// cloneEvent creates a deep copy of an event
func (s *EncryptingEventStore) cloneEvent(event *eventsourcing.Event) *eventsourcing.Event {
	clone := *event

	// Copy Data
	if event.Data != nil {
		clone.Data = make([]byte, len(event.Data))
		copy(clone.Data, event.Data)
	}

	// Copy Metadata
	clone.Metadata = event.Metadata
	if event.Metadata.Custom != nil {
		clone.Metadata.Custom = make(map[string]string)
		for k, v := range event.Metadata.Custom {
			clone.Metadata.Custom[k] = v
		}
	}

	// Copy UniqueConstraints
	if event.UniqueConstraints != nil {
		clone.UniqueConstraints = make([]eventsourcing.UniqueConstraint, len(event.UniqueConstraints))
		copy(clone.UniqueConstraints, event.UniqueConstraints)
	}

	return &clone
}
