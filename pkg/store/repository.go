package store

import (
	"fmt"
	"time"

	"github.com/plaenen/eventstore/pkg/domain"
)

// Repository provides persistence operations for aggregates.
type Repository[T domain.Aggregate] interface {
	// Load loads an aggregate by ID from the event store.
	Load(id string) (T, error)

	// Save persists an aggregate's uncommitted events to the event store.
	Save(aggregate T) error

	// SaveWithCommand persists events with command-level idempotency.
	SaveWithCommand(aggregate T, commandID string) (*domain.CommandResult, error)

	// Exists checks if an aggregate exists.
	Exists(id string) (bool, error)
}

// BaseRepository provides a basic implementation of Repository.
type BaseRepository[T domain.Aggregate] struct {
	eventStore    EventStore
	aggregateType string
	factory       func(id string) T
	applier       func(aggregate T, event *domain.Event) error
}

// NewRepository creates a new repository for the given aggregate type.
// factory creates a new aggregate instance.
// applier applies an event to the aggregate.
func NewRepository[T domain.Aggregate](
	eventStore EventStore,
	aggregateType string,
	factory func(id string) T,
	applier func(aggregate T, event *domain.Event) error,
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
		return zero, domain.ErrAggregateNotFound
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
		if agg, ok := interface{}(aggregate).(interface{ LoadFromHistory([]*domain.Event) error }); ok {
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
func (r *BaseRepository[T]) SaveWithCommand(aggregate T, commandID string) (*domain.CommandResult, error) {
	uncommittedEvents := aggregate.UncommittedEvents()
	if len(uncommittedEvents) == 0 {
		return &domain.CommandResult{
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
		domain.DefaultCommandTTL,
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
