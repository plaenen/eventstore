package store

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/plaenen/eventstore/pkg/eventsourcing"
)

// Repository provides persistence operations for aggregates.
type Repository[T eventsourcing.Aggregate] interface {
	// Load loads an aggregate by ID from the event store.
	Load(id string) (T, error)

	// Save persists an aggregate's uncommitted events to the event store.
	Save(aggregate T) error

	// SaveWithCommand persists events with command-level idempotency.
	SaveWithCommand(aggregate T, commandID string) (*eventsourcing.CommandResult, error)

	// Exists checks if an aggregate exists.
	Exists(id string) (bool, error)
}

// BaseRepository provides a basic implementation of Repository.
type BaseRepository[T eventsourcing.Aggregate] struct {
	eventStore    eventsourcing.EventStore
	snapshotStore eventsourcing.SnapshotStore
	aggregateType string
	factory       func(id string) T
	applier       func(aggregate T, event *eventsourcing.Event) error
}

// NewRepository creates a new repository for the given aggregate type.
// factory creates a new aggregate instance.
// applier applies an event to the aggregate.
func NewRepository[T eventsourcing.Aggregate](
	eventStore eventsourcing.EventStore,
	aggregateType string,
	factory func(id string) T,
	applier func(aggregate T, event *eventsourcing.Event) error,
) *BaseRepository[T] {
	return &BaseRepository[T]{
		eventStore:    eventStore,
		snapshotStore: nil, // Optional, set via WithSnapshotStore
		aggregateType: aggregateType,
		factory:       factory,
		applier:       applier,
	}
}

// WithSnapshotStore configures the repository to use snapshots.
// This enables the LoadWithSnapshot method for performance optimization.
func (r *BaseRepository[T]) WithSnapshotStore(snapshotStore eventsourcing.SnapshotStore) *BaseRepository[T] {
	r.snapshotStore = snapshotStore
	return r
}

// Load loads an aggregate by ID from the event store.
// If a snapshot store is configured, it will automatically use LoadWithSnapshot instead.
func (r *BaseRepository[T]) Load(id string) (T, error) {
	// If snapshot store is configured, use it for better performance
	if r.snapshotStore != nil {
		return r.LoadWithSnapshot(id)
	}

	var zero T

	// Load events from store
	events, err := r.eventStore.LoadEvents(id, 0)
	if err != nil {
		return zero, fmt.Errorf("failed to load events: %w", err)
	}

	if len(events) == 0 {
		return zero, eventsourcing.ErrAggregateNotFound
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
		if agg, ok := interface{}(aggregate).(interface{ LoadFromHistory([]*eventsourcing.Event) error }); ok {
			if err := agg.LoadFromHistory(events); err != nil {
				return zero, fmt.Errorf("failed to load history: %w", err)
			}
		}
	}

	return aggregate, nil
}

// LoadWithSnapshot loads an aggregate using snapshots for optimization.
// It loads the latest snapshot, restores the state and analytics, then replays
// only the events that occurred after the snapshot.
func (r *BaseRepository[T]) LoadWithSnapshot(id string) (T, error) {
	var zero T

	if r.snapshotStore == nil {
		return zero, fmt.Errorf("snapshot store not configured")
	}

	// Try to load latest snapshot
	snapshot, err := r.snapshotStore.GetLatestSnapshot(id)
	var startVersion int64 = 0

	if err == nil && snapshot != nil {
		// Snapshot exists, restore from it
		aggregate := r.factory(id)

		// Check if aggregate supports snapshots
		snapshotable, ok := interface{}(aggregate).(eventsourcing.Snapshotable)
		if !ok {
			return zero, fmt.Errorf("aggregate does not implement Snapshotable interface")
		}

		// Unmarshal snapshot data
		if err := snapshotable.UnmarshalSnapshot(snapshot.Data); err != nil {
			return zero, fmt.Errorf("failed to unmarshal snapshot: %w", err)
		}

		// Restore analytics from snapshot metadata
		if snapshot.Metadata != nil && snapshot.Metadata.Analytics != "" {
			analyticsData, err := snapshot.Metadata.GetAnalytics()
			if err == nil && analyticsData != nil {
				// Parse analytics and restore to aggregate
				var analytics eventsourcing.EventAnalytics
				analyticsJSON, _ := json.Marshal(analyticsData)
				if err := json.Unmarshal(analyticsJSON, &analytics); err == nil {
					// Set analytics on aggregate root
					if aggRoot, ok := interface{}(aggregate).(interface{ SetAnalytics(*eventsourcing.EventAnalytics) }); ok {
						aggRoot.SetAnalytics(&analytics)
					}
				}
			}
		}

		// Load events after snapshot
		startVersion = snapshot.Version
		events, err := r.eventStore.LoadEvents(id, startVersion)
		if err != nil {
			return zero, fmt.Errorf("failed to load events after snapshot: %w", err)
		}

		// Apply events after snapshot (this will update analytics automatically)
		for _, event := range events {
			if err := r.applier(aggregate, event); err != nil {
				return zero, fmt.Errorf("failed to apply event: %w", err)
			}
		}

		// Update version and analytics from loaded events
		if len(events) > 0 {
			if agg, ok := interface{}(aggregate).(interface{ LoadFromHistory([]*eventsourcing.Event) error }); ok {
				if err := agg.LoadFromHistory(events); err != nil {
					return zero, fmt.Errorf("failed to load history: %w", err)
				}
			}
		}

		return aggregate, nil
	}

	// No snapshot found or error loading snapshot, fall back to full event replay
	// NOTE: Don't call r.Load() to avoid infinite loop, do direct event replay

	// Load events from store
	events, err := r.eventStore.LoadEvents(id, 0)
	if err != nil {
		return zero, fmt.Errorf("failed to load events: %w", err)
	}

	if len(events) == 0 {
		return zero, eventsourcing.ErrAggregateNotFound
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
		if agg, ok := interface{}(aggregate).(interface{ LoadFromHistory([]*eventsourcing.Event) error }); ok {
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
func (r *BaseRepository[T]) SaveWithCommand(aggregate T, commandID string) (*eventsourcing.CommandResult, error) {
	uncommittedEvents := aggregate.UncommittedEvents()
	if len(uncommittedEvents) == 0 {
		return &eventsourcing.CommandResult{
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
		eventsourcing.DefaultCommandTTL,
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

// SeedAggregate seeds an aggregate's uncommitted events using special seeding semantics.
// This is a convenience wrapper around EventStore.SeedEvents() that works with aggregates.
//
// Unlike Save(), SeedAggregate:
//   - Is idempotent (skips events that already exist)
//   - Generates deterministic IDs for events without IDs
//   - Optionally skips version checking
//   - Checks constraint ownership rather than failing on conflicts
//   - Adds metadata tracking for data lineage
//
// Use cases:
//   - Database migrations (seeding historical data)
//   - Bootstrap data (seeding admin users, system configs)
//   - Test data setup (seeding deterministic test fixtures)
//
// Example:
//   admin := NewUser("admin-001")
//   admin.Create("admin@example.com", "Admin User")
//   admin.AssignRole("super_admin")
//
//   opts := eventsourcing.DefaultSeedOptions()
//   opts.CustomTags = map[string]string{"source": "bootstrap"}
//   result, err := repo.SeedAggregate(admin, 0, opts)
//
func (r *BaseRepository[T]) SeedAggregate(aggregate T, expectedVersion int64, opts *eventsourcing.SeedOptions) (*eventsourcing.SeedResult, error) {
	uncommittedEvents := aggregate.UncommittedEvents()
	if len(uncommittedEvents) == 0 {
		return &eventsourcing.SeedResult{}, nil
	}

	// Seed events using the event store
	result, err := r.eventStore.SeedEvents(aggregate.ID(), expectedVersion, uncommittedEvents, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to seed events: %w", err)
	}

	// Clear uncommitted events if any were saved
	if result.Saved > 0 {
		aggregate.ClearUncommittedEvents()
	}

	return result, nil
}

// SaveSnapshot creates and persists a snapshot of the aggregate's current state.
// The snapshot includes the aggregate state and analytics for debugging purposes.
//
// This should typically be called after saving events when the aggregate reaches
// a snapshotting threshold (e.g., every 100 events).
//
// Example:
//   user, _ := repo.Load("user-123")
//   // ... make changes to user ...
//   err := repo.Save(user)
//
//   // Create snapshot every 100 events
//   if user.Version() % 100 == 0 {
//       err := repo.SaveSnapshot(user)
//   }
//
func (r *BaseRepository[T]) SaveSnapshot(aggregate T) error {
	if r.snapshotStore == nil {
		return fmt.Errorf("snapshot store not configured")
	}

	// Check if aggregate supports snapshots
	snapshotable, ok := interface{}(aggregate).(eventsourcing.Snapshotable)
	if !ok {
		return fmt.Errorf("aggregate does not implement Snapshotable interface")
	}

	// Marshal aggregate state
	data, err := snapshotable.MarshalSnapshot()
	if err != nil {
		return fmt.Errorf("failed to marshal snapshot: %w", err)
	}

	// Create metadata
	metadata := &eventsourcing.SnapshotMetadata{
		Size:          int64(len(data)),
		EventCount:    aggregate.Version(),
		SnapshotType:  "protobuf",
		SchemaVersion: "1.0.0",
	}

	// Include analytics in metadata
	if aggRoot, ok := interface{}(aggregate).(interface{ Analytics() *eventsourcing.EventAnalytics }); ok {
		analytics := aggRoot.Analytics()
		if err := metadata.SetAnalyticsFromAggregate(analytics); err != nil {
			return fmt.Errorf("failed to set analytics: %w", err)
		}
	}

	// Create snapshot
	snapshot := &eventsourcing.Snapshot{
		AggregateID:   aggregate.ID(),
		AggregateType: aggregate.Type(),
		Version:       aggregate.Version(),
		Data:          data,
		CreatedAt:     time.Now(),
		Metadata:      metadata,
	}

	// Save snapshot
	if err := r.snapshotStore.SaveSnapshot(snapshot); err != nil {
		return fmt.Errorf("failed to save snapshot: %w", err)
	}

	return nil
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
