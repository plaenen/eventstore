package domain

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"
)

// Aggregate defines the interface that all aggregates must implement.
type Aggregate interface {
	// ID returns the unique identifier of the aggregate.
	ID() string

	// Type returns the type name of the aggregate.
	Type() string

	// Version returns the current version of the aggregate.
	Version() int64

	// ApplyEvent applies an event to the aggregate's state.
	// This is called when loading events from the event store.
	ApplyEvent(event proto.Message) error

	// UncommittedEvents returns events that have been applied but not yet persisted.
	UncommittedEvents() []*Event

	// ClearUncommittedEvents clears the uncommitted events after they've been persisted.
	ClearUncommittedEvents()
}

// EventUpcaster is an optional interface that aggregates can implement
// to convert old event versions to current versions.
//
// The generated ApplyEvent method will call this before routing events.
type EventUpcaster interface {
	UpcastEvent(event proto.Message) proto.Message
}

// SnapshotUpcaster is an optional interface that aggregates can implement
// to convert old snapshot versions to current versions.
//
// The generated UnmarshalSnapshot method will call this after deserializing.
type SnapshotUpcaster interface {
	UpcastSnapshot(state proto.Message) proto.Message
}

// AggregateRoot provides base functionality for all aggregates.
// Use this as an embedded type in your aggregate implementations.
type AggregateRoot struct {
	id                string
	aggregateType     string
	version           int64
	uncommittedEvents []*Event
	commandID         string // Current command being processed (for deterministic event IDs)
}

// NewAggregateRoot creates a new aggregate root with the given ID and type.
func NewAggregateRoot(id, aggregateType string) AggregateRoot {
	return AggregateRoot{
		id:                id,
		aggregateType:     aggregateType,
		version:           0,
		uncommittedEvents: make([]*Event, 0),
	}
}

// ID returns the aggregate's unique identifier.
func (a *AggregateRoot) ID() string {
	return a.id
}

// Type returns the aggregate's type name.
func (a *AggregateRoot) Type() string {
	return a.aggregateType
}

// Version returns the aggregate's current version.
func (a *AggregateRoot) Version() int64 {
	return a.version
}

// UncommittedEvents returns events that haven't been persisted yet.
func (a *AggregateRoot) UncommittedEvents() []*Event {
	return a.uncommittedEvents
}

// ClearUncommittedEvents clears the uncommitted events list.
func (a *AggregateRoot) ClearUncommittedEvents() {
	a.uncommittedEvents = make([]*Event, 0)
}

// SetCommandID sets the command ID for deterministic event ID generation.
// This should be called before processing a command.
func (a *AggregateRoot) SetCommandID(commandID string) {
	a.commandID = commandID
}

// ApplyChange applies a new event to the aggregate.
// This is called when the aggregate produces a new event.
func (a *AggregateRoot) ApplyChange(event proto.Message, eventType string, metadata EventMetadata) error {
	return a.ApplyChangeWithConstraints(event, eventType, metadata, nil)
}

// ApplyChangeWithConstraints applies a new event with unique constraints.
// The constraints will be validated atomically when the event is persisted.
func (a *AggregateRoot) ApplyChangeWithConstraints(
	event proto.Message,
	eventType string,
	metadata EventMetadata,
	constraints []UniqueConstraint,
) error {
	// Serialize the event
	data, err := proto.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Generate deterministic event ID from command context
	var eventID string
	if a.commandID != "" {
		// Deterministic ID for idempotency
		eventID = GenerateDeterministicEventID(a.commandID, a.id, len(a.uncommittedEvents))
	} else {
		// Fallback to random ID (for legacy or non-command events)
		eventID = generateRandomID()
	}

	// Create event envelope
	evt := &Event{
		ID:                eventID,
		AggregateID:       a.id,
		AggregateType:     a.aggregateType,
		EventType:         eventType,
		Version:           a.version + 1,
		Timestamp:         Now(),
		Data:              data,
		Metadata:          metadata,
		UniqueConstraints: constraints,
	}

	// Add to uncommitted events
	a.uncommittedEvents = append(a.uncommittedEvents, evt)

	// Increment version
	a.version++

	return nil
}

// LoadFromHistory reconstructs aggregate state from historical events.
func (a *AggregateRoot) LoadFromHistory(events []*Event) error {
	for _, evt := range events {
		if evt.Version <= a.version {
			continue
		}
		a.version = evt.Version
	}
	return nil
}

// TimeFunc is a function that returns the current time.
// This can be overridden for testing.
var TimeFunc = time.Now

// Now returns the current time using the configured TimeFunc.
func Now() time.Time {
	return TimeFunc()
}

// generateRandomID generates a random unique ID.
func generateRandomID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err) // Should never happen
	}
	return hex.EncodeToString(b)
}

// GenerateID generates a unique identifier.
func GenerateID() string {
	return generateRandomID()
}

// TimeFromUnix creates a time.Time from a Unix timestamp.
func TimeFromUnix(sec int64) time.Time {
	return time.Unix(sec, 0)
}
