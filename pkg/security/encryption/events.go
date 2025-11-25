package encryption

import (
	"encoding/json"
	"fmt"

	"github.com/plaenen/eventstore/pkg/eventsourcing"
)

// EventEncryptor provides encryption for event data
type EventEncryptor struct {
	service *Service
	config  *EventEncryptionConfig
}

// EventEncryptionConfig configures event encryption
type EventEncryptionConfig struct {
	// EncryptData encrypts the event data payload
	EncryptData bool

	// FieldEncryption enables field-level encryption
	// Only specified fields are encrypted, others remain plaintext
	FieldEncryption bool

	// EncryptedFields lists which fields to encrypt (if FieldEncryption is true)
	// Example: []string{"password", "ssn", "credit_card"}
	EncryptedFields []string
}

// DefaultEventEncryptionConfig returns default event encryption configuration
func DefaultEventEncryptionConfig() *EventEncryptionConfig {
	return &EventEncryptionConfig{
		EncryptData:     true,  // Encrypt all event data
		FieldEncryption: false, // Full encryption by default
	}
}

// NewEventEncryptor creates a new event encryptor
func NewEventEncryptor(service *Service, config *EventEncryptionConfig) *EventEncryptor {
	if config == nil {
		config = DefaultEventEncryptionConfig()
	}

	return &EventEncryptor{
		service: service,
		config:  config,
	}
}

// EncryptEvent encrypts event data based on configuration
// Returns a new event with encrypted data
func (ee *EventEncryptor) EncryptEvent(event *eventsourcing.Event) (*eventsourcing.Event, error) {
	if !ee.config.EncryptData || len(event.Data) == 0 {
		return event, nil
	}

	// Create a copy to avoid modifying the original
	encrypted := &eventsourcing.Event{
		ID:            event.ID,
		AggregateID:   event.AggregateID,
		AggregateType: event.AggregateType,
		EventType:     event.EventType,
		Version:       event.Version,
		Timestamp:     event.Timestamp,
		Metadata:      event.Metadata,
		Data:          event.Data,
	}

	var err error
	if ee.config.FieldEncryption {
		// Field-level encryption
		encrypted.Data, err = ee.encryptFields(event.Data)
	} else {
		// Full data encryption
		encryptedData, encErr := ee.service.Encrypt(event.Data)
		if encErr != nil {
			return nil, fmt.Errorf("failed to encrypt data: %w", encErr)
		}
		encrypted.Data = []byte(encryptedData)
	}

	if err != nil {
		return nil, err
	}

	return encrypted, nil
}

// DecryptEvent decrypts event data
// Returns a new event with decrypted data
func (ee *EventEncryptor) DecryptEvent(event *eventsourcing.Event) (*eventsourcing.Event, error) {
	if !ee.config.EncryptData || len(event.Data) == 0 {
		return event, nil
	}

	// Create a copy
	decrypted := &eventsourcing.Event{
		ID:            event.ID,
		AggregateID:   event.AggregateID,
		AggregateType: event.AggregateType,
		EventType:     event.EventType,
		Version:       event.Version,
		Timestamp:     event.Timestamp,
		Metadata:      event.Metadata,
		Data:          event.Data,
	}

	var err error
	if ee.config.FieldEncryption {
		// Field-level decryption
		decrypted.Data, err = ee.decryptFields(event.Data)
	} else {
		// Full data decryption
		// Check if data looks encrypted
		if isEncrypted(string(event.Data)) {
			decryptedData, decErr := ee.service.Decrypt(string(event.Data))
			if decErr != nil {
				return nil, fmt.Errorf("failed to decrypt data: %w", decErr)
			}
			decrypted.Data = decryptedData
		}
	}

	if err != nil {
		return nil, err
	}

	return decrypted, nil
}

// EncryptEvents encrypts multiple events
func (ee *EventEncryptor) EncryptEvents(events []*eventsourcing.Event) ([]*eventsourcing.Event, error) {
	encrypted := make([]*eventsourcing.Event, len(events))
	for i, event := range events {
		enc, err := ee.EncryptEvent(event)
		if err != nil {
			return nil, fmt.Errorf("failed to encrypt event %d: %w", i, err)
		}
		encrypted[i] = enc
	}
	return encrypted, nil
}

// DecryptEvents decrypts multiple events
func (ee *EventEncryptor) DecryptEvents(events []*eventsourcing.Event) ([]*eventsourcing.Event, error) {
	decrypted := make([]*eventsourcing.Event, len(events))
	for i, event := range events {
		dec, err := ee.DecryptEvent(event)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt event %d: %w", i, err)
		}
		decrypted[i] = dec
	}
	return decrypted, nil
}

// encryptFields encrypts specific fields in JSON data
func (ee *EventEncryptor) encryptFields(data []byte) ([]byte, error) {
	// Parse JSON
	var obj map[string]interface{}
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	// Encrypt specified fields
	for _, field := range ee.config.EncryptedFields {
		if value, exists := obj[field]; exists {
			// Convert value to JSON
			valueBytes, err := json.Marshal(value)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal field %s: %w", field, err)
			}

			// Encrypt
			encrypted, err := ee.service.Encrypt(valueBytes)
			if err != nil {
				return nil, fmt.Errorf("failed to encrypt field %s: %w", field, err)
			}

			// Replace with encrypted value
			obj[field] = encrypted
		}
	}

	// Marshal back to JSON
	return json.Marshal(obj)
}

// decryptFields decrypts specific fields in JSON data
func (ee *EventEncryptor) decryptFields(data []byte) ([]byte, error) {
	// Parse JSON
	var obj map[string]interface{}
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	// Decrypt specified fields
	for _, field := range ee.config.EncryptedFields {
		if value, exists := obj[field]; exists {
			// Convert to string
			valueStr, ok := value.(string)
			if !ok {
				continue // Skip if not a string
			}

			// Check if it looks encrypted
			if isEncrypted(valueStr) {
				// Decrypt
				decrypted, err := ee.service.Decrypt(valueStr)
				if err != nil {
					return nil, fmt.Errorf("failed to decrypt field %s: %w", field, err)
				}

				// Try to unmarshal back to original type
				var decryptedValue interface{}
				if err := json.Unmarshal(decrypted, &decryptedValue); err != nil {
					// If unmarshal fails, use as string
					obj[field] = string(decrypted)
				} else {
					obj[field] = decryptedValue
				}
			}
		}
	}

	// Marshal back to JSON
	return json.Marshal(obj)
}

// isEncrypted checks if a string looks like encrypted data
// Encrypted data has the format: keyID:base64data
func isEncrypted(s string) bool {
	// Simple heuristic: check for keyID separator and base64-like characters
	sepIndex := -1
	for i, c := range s {
		if c == ':' {
			sepIndex = i
			break
		}
	}

	return sepIndex > 0 && sepIndex < len(s)-1
}
