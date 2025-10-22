package eventsourcing

import (
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
		eventID = generateRandomEventID()
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

// Repository provides persistence operations for aggregates.
type Repository[T Aggregate] interface {
	// Load loads an aggregate by ID from the event store.
	Load(id string) (T, error)

	// Save persists an aggregate's uncommitted events to the event store.
	Save(aggregate T) error

	// SaveWithCommand persists events with command-level idempotency.
	SaveWithCommand(aggregate T, commandID string) (*CommandResult, error)

	// Exists checks if an aggregate exists.
	Exists(id string) (bool, error)
}

// BaseRepository provides a basic implementation of Repository.
type BaseRepository[T Aggregate] struct {
	eventStore    EventStore
	aggregateType string
	factory       func(id string) T
	applier       func(aggregate T, event *Event) error
}

// NewRepository creates a new repository for the given aggregate type.
// factory creates a new aggregate instance.
// applier applies an event to the aggregate.
func NewRepository[T Aggregate](
	eventStore EventStore,
	aggregateType string,
	factory func(id string) T,
	applier func(aggregate T, event *Event) error,
) *BaseRepository[T] {
	return &BaseRepository[T]{
		eventStore:    eventStore,
		aggregateType: aggregateType,
		factory:       factory,
		applier:       applier,
	}
}

// Load loads an aggregate by ID from the event store.
func (r *BaseRepository[T]) Load(id string) (T, error) {
	var zero T

	// Load events from store
	events, err := r.eventStore.LoadEvents(id, 0)
	if err != nil {
		return zero, fmt.Errorf("failed to load events: %w", err)
	}

	if len(events) == 0 {
		return zero, ErrAggregateNotFound
	}

	// Create new aggregate instance
	aggregate := r.factory(id)

	// Apply all events to rebuild state
	for _, event := range events {
		if err := r.applier(aggregate, event); err != nil {
			return zero, fmt.Errorf("failed to apply event: %w", err)
		}
	}

	// Update version from loaded events
	if len(events) > 0 {
		// Set the aggregate version from the loaded history
		if agg, ok := interface{}(aggregate).(interface{ LoadFromHistory([]*Event) error }); ok {
			if err := agg.LoadFromHistory(events); err != nil {
				return zero, fmt.Errorf("failed to load history: %w", err)
			}
		}
	}

	return aggregate, nil
}

// Save persists an aggregate's uncommitted events.
func (r *BaseRepository[T]) Save(aggregate T) error {
	uncommittedEvents := aggregate.UncommittedEvents()
	if len(uncommittedEvents) == 0 {
		return nil // Nothing to save
	}

	// Calculate expected version (version before new events)
	expectedVersion := aggregate.Version() - int64(len(uncommittedEvents))

	// Append events atomically with constraint validation
	if err := r.eventStore.AppendEvents(aggregate.ID(), expectedVersion, uncommittedEvents); err != nil {
		return fmt.Errorf("failed to append events: %w", err)
	}

	// Clear uncommitted events
	aggregate.ClearUncommittedEvents()

	return nil
}

// SaveWithCommand persists events with command-level idempotency.
// Returns CommandResult which includes whether command was already processed.
func (r *BaseRepository[T]) SaveWithCommand(aggregate T, commandID string) (*CommandResult, error) {
	uncommittedEvents := aggregate.UncommittedEvents()
	if len(uncommittedEvents) == 0 {
		return &CommandResult{
			CommandID: commandID,
			Events:    nil,
		}, nil
	}

	// Calculate expected version (version before new events)
	expectedVersion := aggregate.Version() - int64(len(uncommittedEvents))

	// Append events with idempotency
	result, err := r.eventStore.AppendEventsIdempotent(
		aggregate.ID(),
		expectedVersion,
		uncommittedEvents,
		commandID,
		DefaultCommandTTL,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to append events: %w", err)
	}

	// Clear uncommitted events only if we actually persisted them
	if !result.AlreadyProcessed {
		aggregate.ClearUncommittedEvents()
	}

	return result, nil
}

// Exists checks if an aggregate exists in the event store.
func (r *BaseRepository[T]) Exists(id string) (bool, error) {
	version, err := r.eventStore.GetAggregateVersion(id)
	if err != nil {
		return false, fmt.Errorf("failed to check aggregate existence: %w", err)
	}
	return version > 0, nil
}

// RetryOnConflict executes a function with retry logic for optimistic concurrency conflicts.
// The function receives a freshly loaded aggregate on each attempt.
// This is useful for command handlers that need to retry on version mismatch.
func (r *BaseRepository[T]) RetryOnConflict(id string, maxRetries int, fn func(T) error) error {
	for attempt := 0; attempt <= maxRetries; attempt++ {
		// Load fresh aggregate
		agg, err := r.Load(id)
		if err != nil {
			return err
		}

		// Execute the function
		err = fn(agg)
		if err == nil {
			return nil
		}

		// Check if this is a concurrency conflict
		if !isConcurrencyConflict(err) {
			return err // Not a conflict, return error
		}

		// If last attempt, return the error
		if attempt == maxRetries {
			return err
		}

		// Brief backoff before retry (10ms, 20ms, 40ms)
		backoff := time.Duration(10*(1<<uint(attempt))) * time.Millisecond
		time.Sleep(backoff)
	}
	return fmt.Errorf("max retries exceeded")
}

// isConcurrencyConflict checks if an error is due to optimistic locking failure
func isConcurrencyConflict(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return len(msg) > 0 && (
		contains(msg, "concurrency conflict") ||
		contains(msg, "version mismatch") ||
		contains(msg, "optimistic lock"))
}

// contains checks if a string contains a substring
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
