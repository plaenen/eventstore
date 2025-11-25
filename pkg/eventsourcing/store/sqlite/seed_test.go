package sqlite_test

import (
	"testing"
	"time"

	"github.com/plaenen/eventstore/pkg/eventsourcing"
	"github.com/plaenen/eventstore/pkg/eventsourcing/store/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeedEvents_Basic(t *testing.T) {
	store, err := sqlite.NewEventStore(
		sqlite.WithMemoryDatabase(),
		sqlite.WithWALMode(false),
	)
	require.NoError(t, err)
	defer store.Close()

	aggregateID := "test-aggregate-1"
	events := []*eventsourcing.Event{
		{
			ID:            "seed-event-1",
			AggregateID:   aggregateID,
			AggregateType: "TestAggregate",
			EventType:     "TestCreated",
			Version:       1,
			Timestamp:     time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			Data:          []byte("test data 1"),
		},
		{
			ID:            "seed-event-2",
			AggregateID:   aggregateID,
			AggregateType: "TestAggregate",
			EventType:     "TestUpdated",
			Version:       2,
			Timestamp:     time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
			Data:          []byte("test data 2"),
		},
	}

	result, err := store.SeedEvents(aggregateID, 0, events, eventsourcing.DefaultSeedOptions())
	require.NoError(t, err)
	assert.True(t, result.Success())
	assert.Equal(t, 2, result.Saved)
	assert.Equal(t, 0, result.Skipped)
	assert.Equal(t, 0, result.Failed)
	assert.Len(t, result.EventIDs, 2)

	// Verify events were saved
	loaded, err := store.LoadEvents(aggregateID, 0)
	require.NoError(t, err)
	assert.Len(t, loaded, 2)
	assert.Equal(t, "seed-event-1", loaded[0].ID)
	assert.Equal(t, "seed-event-2", loaded[1].ID)
}

func TestSeedEvents_Idempotency(t *testing.T) {
	store, err := sqlite.NewEventStore(
		sqlite.WithMemoryDatabase(),
		sqlite.WithWALMode(false),
	)
	require.NoError(t, err)
	defer store.Close()

	aggregateID := "test-aggregate-2"
	events := []*eventsourcing.Event{
		{
			ID:            "seed-event-3",
			AggregateID:   aggregateID,
			AggregateType: "TestAggregate",
			EventType:     "TestCreated",
			Version:       1,
			Timestamp:     time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			Data:          []byte("test data"),
		},
	}

	// First seed
	result1, err := store.SeedEvents(aggregateID, 0, events, eventsourcing.DefaultSeedOptions())
	require.NoError(t, err)
	assert.Equal(t, 1, result1.Saved)
	assert.Equal(t, 0, result1.Skipped)

	// Second seed (should skip)
	result2, err := store.SeedEvents(aggregateID, 0, events, eventsourcing.DefaultSeedOptions())
	require.NoError(t, err)
	assert.Equal(t, 0, result2.Saved)
	assert.Equal(t, 1, result2.Skipped)
	assert.True(t, result2.Success())

	// Verify only one event exists
	loaded, err := store.LoadEvents(aggregateID, 0)
	require.NoError(t, err)
	assert.Len(t, loaded, 1)
}

func TestSeedEvents_DeterministicIDs(t *testing.T) {
	store, err := sqlite.NewEventStore(
		sqlite.WithMemoryDatabase(),
		sqlite.WithWALMode(false),
	)
	require.NoError(t, err)
	defer store.Close()

	aggregateID := "test-aggregate-3"

	// Event without ID - should be auto-generated
	events := []*eventsourcing.Event{
		{
			AggregateID:   aggregateID,
			AggregateType: "TestAggregate",
			EventType:     "TestCreated",
			Version:       1,
			Timestamp:     time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), // Fixed timestamp
			Data:          []byte("deterministic data"),
		},
	}

	// First seed - ID should be generated
	result1, err := store.SeedEvents(aggregateID, 0, events, eventsourcing.DefaultSeedOptions())
	require.NoError(t, err)
	assert.Equal(t, 1, result1.Saved)
	assert.Len(t, result1.EventIDs, 1)
	generatedID1 := result1.EventIDs[0]
	assert.NotEmpty(t, generatedID1)
	assert.Contains(t, generatedID1, "seed-") // Should have seed prefix

	// Second seed with same event - should generate same ID and skip
	result2, err := store.SeedEvents(aggregateID, 0, events, eventsourcing.DefaultSeedOptions())
	require.NoError(t, err)
	assert.Equal(t, 0, result2.Saved)
	assert.Equal(t, 1, result2.Skipped)
	assert.Equal(t, generatedID1, result2.EventIDs[0], "Same event should generate same ID")
}

func TestSeedEvents_WithConstraints_OwnershipCheck(t *testing.T) {
	store, err := sqlite.NewEventStore(
		sqlite.WithMemoryDatabase(),
		sqlite.WithWALMode(false),
	)
	require.NoError(t, err)
	defer store.Close()

	aggregateID1 := "user-1"
	aggregateID2 := "user-2"

	// Seed first user with email
	events1 := []*eventsourcing.Event{
		{
			ID:            "seed-user1-created",
			AggregateID:   aggregateID1,
			AggregateType: "User",
			EventType:     "UserCreated",
			Version:       1,
			Timestamp:     time.Now(),
			Data:          []byte("user1 data"),
			UniqueConstraints: []eventsourcing.UniqueConstraint{
				{
					IndexName: "user_email",
					Value:     "admin@example.com",
					Operation: eventsourcing.ConstraintClaim,
				},
			},
		},
	}

	result1, err := store.SeedEvents(aggregateID1, 0, events1, eventsourcing.DefaultSeedOptions())
	require.NoError(t, err)
	assert.Equal(t, 1, result1.Saved)

	// Try to seed second user with same email - should fail
	events2 := []*eventsourcing.Event{
		{
			ID:            "seed-user2-created",
			AggregateID:   aggregateID2,
			AggregateType: "User",
			EventType:     "UserCreated",
			Version:       1,
			Timestamp:     time.Now(),
			Data:          []byte("user2 data"),
			UniqueConstraints: []eventsourcing.UniqueConstraint{
				{
					IndexName: "user_email",
					Value:     "admin@example.com", // Same email
					Operation: eventsourcing.ConstraintClaim,
				},
			},
		},
	}

	result2, err := store.SeedEvents(aggregateID2, 0, events2, eventsourcing.DefaultSeedOptions())
	require.NoError(t, err)
	assert.Equal(t, 0, result2.Saved)
	assert.Equal(t, 1, result2.Failed)
	assert.True(t, result2.HasErrors())
	assert.Contains(t, result2.Errors[0].Reason, "constraint")

	// Re-seed same user with same constraint - should be idempotent (skip)
	result3, err := store.SeedEvents(aggregateID1, 0, events1, eventsourcing.DefaultSeedOptions())
	require.NoError(t, err)
	assert.Equal(t, 0, result3.Saved)
	assert.Equal(t, 1, result3.Skipped)
}

func TestSeedEvents_CustomTags(t *testing.T) {
	store, err := sqlite.NewEventStore(
		sqlite.WithMemoryDatabase(),
		sqlite.WithWALMode(false),
	)
	require.NoError(t, err)
	defer store.Close()

	aggregateID := "test-aggregate-4"
	events := []*eventsourcing.Event{
		{
			ID:            "seed-event-with-tags",
			AggregateID:   aggregateID,
			AggregateType: "TestAggregate",
			EventType:     "TestCreated",
			Version:       1,
			Timestamp:     time.Now(),
			Data:          []byte("test data"),
		},
	}

	opts := eventsourcing.DefaultSeedOptions()
	opts.CustomTags = map[string]string{
		"migration": "v1.0.0",
		"source":    "legacy-database",
		"batch":     "2025-01-15",
	}

	result, err := store.SeedEvents(aggregateID, 0, events, opts)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Saved)

	// Verify custom tags were added
	loaded, err := store.LoadEvents(aggregateID, 0)
	require.NoError(t, err)
	require.Len(t, loaded, 1)

	metadata := loaded[0].Metadata
	assert.Equal(t, "true", metadata.Custom["_seeded"])
	assert.NotEmpty(t, metadata.Custom["_seeded_at"])
	assert.Equal(t, "v1.0.0", metadata.Custom["migration"])
	assert.Equal(t, "legacy-database", metadata.Custom["source"])
	assert.Equal(t, "2025-01-15", metadata.Custom["batch"])
}

func TestSeedEvents_SkipConstraintChecking(t *testing.T) {
	store, err := sqlite.NewEventStore(
		sqlite.WithMemoryDatabase(),
		sqlite.WithWALMode(false),
	)
	require.NoError(t, err)
	defer store.Close()

	aggregateID1 := "user-1"
	aggregateID2 := "user-2"

	// Seed first user with email
	events1 := []*eventsourcing.Event{
		{
			ID:            "seed-user1",
			AggregateID:   aggregateID1,
			AggregateType: "User",
			EventType:     "UserCreated",
			Version:       1,
			Timestamp:     time.Now(),
			Data:          []byte("user1"),
			UniqueConstraints: []eventsourcing.UniqueConstraint{
				{
					IndexName: "user_email",
					Value:     "test@example.com",
					Operation: eventsourcing.ConstraintClaim,
				},
			},
		},
	}

	// Seed with constraint checking enabled
	result1, err := store.SeedEvents(aggregateID1, 0, events1, eventsourcing.DefaultSeedOptions())
	require.NoError(t, err)
	assert.Equal(t, 1, result1.Saved)

	// Seed second user with same email but skip constraint checking
	events2 := []*eventsourcing.Event{
		{
			ID:            "seed-user2",
			AggregateID:   aggregateID2,
			AggregateType: "User",
			EventType:     "UserCreated",
			Version:       1,
			Timestamp:     time.Now(),
			Data:          []byte("user2"),
			UniqueConstraints: []eventsourcing.UniqueConstraint{
				{
					IndexName: "user_email",
					Value:     "test@example.com", // Same email!
					Operation: eventsourcing.ConstraintClaim,
				},
			},
		},
	}

	opts := eventsourcing.DefaultSeedOptions()
	opts.CheckConstraintOwnership = false // Skip constraint checking

	result2, err := store.SeedEvents(aggregateID2, 0, events2, opts)
	require.NoError(t, err)
	assert.Equal(t, 1, result2.Saved, "Should save even with constraint conflict when checking is disabled")
}

func TestSeedEvents_EmptyEventList(t *testing.T) {
	store, err := sqlite.NewEventStore(
		sqlite.WithMemoryDatabase(),
		sqlite.WithWALMode(false),
	)
	require.NoError(t, err)
	defer store.Close()

	result, err := store.SeedEvents("test-id", 0, []*eventsourcing.Event{}, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, result.Saved)
	assert.Equal(t, 0, result.Skipped)
	assert.Equal(t, 0, result.Failed)
}

func TestSeedEvents_NilOptions(t *testing.T) {
	store, err := sqlite.NewEventStore(
		sqlite.WithMemoryDatabase(),
		sqlite.WithWALMode(false),
	)
	require.NoError(t, err)
	defer store.Close()

	aggregateID := "test-aggregate-5"
	events := []*eventsourcing.Event{
		{
			ID:            "seed-event-nil-opts",
			AggregateID:   aggregateID,
			AggregateType: "TestAggregate",
			EventType:     "TestCreated",
			Version:       1,
			Timestamp:     time.Now(),
			Data:          []byte("test data"),
		},
	}

	// Should use defaults when opts is nil
	result, err := store.SeedEvents(aggregateID, 0, events, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Saved)
}

func TestSeedEvents_PartialFailure(t *testing.T) {
	store, err := sqlite.NewEventStore(
		sqlite.WithMemoryDatabase(),
		sqlite.WithWALMode(false),
	)
	require.NoError(t, err)
	defer store.Close()

	aggregateID1 := "user-1"
	aggregateID2 := "user-2"

	// Seed first user with unique email
	firstEvent := []*eventsourcing.Event{
		{
			ID:            "seed-user1",
			AggregateID:   aggregateID1,
			AggregateType: "User",
			EventType:     "UserCreated",
			Version:       1,
			Timestamp:     time.Now(),
			Data:          []byte("user1"),
			UniqueConstraints: []eventsourcing.UniqueConstraint{
				{
					IndexName: "user_email",
					Value:     "unique@example.com",
					Operation: eventsourcing.ConstraintClaim,
				},
			},
		},
	}

	_, err = store.SeedEvents(aggregateID1, 0, firstEvent, eventsourcing.DefaultSeedOptions())
	require.NoError(t, err)

	// Try to seed multiple events for user-2 where one will fail due to constraint
	events := []*eventsourcing.Event{
		{
			ID:            "seed-user2-created",
			AggregateID:   aggregateID2,
			AggregateType: "User",
			EventType:     "UserCreated",
			Version:       1,
			Timestamp:     time.Now(),
			Data:          []byte("user2"),
			UniqueConstraints: []eventsourcing.UniqueConstraint{
				{
					IndexName: "user_email",
					Value:     "unique@example.com", // Already claimed by user1!
					Operation: eventsourcing.ConstraintClaim,
				},
			},
		},
		{
			ID:            "seed-user2-updated",
			AggregateID:   aggregateID2,
			AggregateType: "User",
			EventType:     "UserUpdated",
			Version:       2,
			Timestamp:     time.Now(),
			Data:          []byte("user2 updated"),
		},
	}

	result, err := store.SeedEvents(aggregateID2, 0, events, eventsourcing.DefaultSeedOptions())
	require.NoError(t, err)
	assert.Equal(t, 1, result.Failed) // First event failed due to constraint
	assert.Equal(t, 1, result.Saved)  // Second event saved
	assert.Len(t, result.Errors, 1)
	assert.Contains(t, result.Errors[0].Reason, "constraint")
}
