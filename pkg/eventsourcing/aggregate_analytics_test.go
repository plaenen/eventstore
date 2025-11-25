package eventsourcing_test

import (
	"testing"
	"time"

	"github.com/plaenen/eventstore/pkg/eventsourcing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAggregateRoot_AnalyticsTracking(t *testing.T) {
	agg := eventsourcing.NewAggregateRoot("test-1", "TestAggregate")

	// Create events
	events := []*eventsourcing.Event{
		{
			ID:            "evt-1",
			AggregateID:   "test-1",
			AggregateType: "TestAggregate",
			EventType:     "Created",
			Version:       1,
			Timestamp:     time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			Data:          []byte(`{}`),
		},
		{
			ID:            "evt-2",
			AggregateID:   "test-1",
			AggregateType: "TestAggregate",
			EventType:     "Updated",
			Version:       2,
			Timestamp:     time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
			Data:          []byte(`{}`),
		},
		{
			ID:            "evt-3",
			AggregateID:   "test-1",
			AggregateType: "TestAggregate",
			EventType:     "Created", // Duplicate type
			Version:       3,
			Timestamp:     time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC),
			Data:          []byte(`{}`),
		},
	}

	// Load from history
	err := agg.LoadFromHistory(events)
	require.NoError(t, err)

	// Check analytics
	analytics := agg.Analytics()
	require.NotNil(t, analytics)

	assert.Equal(t, int64(3), analytics.TotalEvents)
	assert.Equal(t, int64(2), analytics.GetCount("Created"))
	assert.Equal(t, int64(1), analytics.GetCount("Updated"))

	// Check event types
	types := analytics.GetEventTypes()
	assert.Len(t, types, 2)
	assert.Contains(t, types, "Created")
	assert.Contains(t, types, "Updated")
}

func TestAggregateRoot_AnalyticsStats(t *testing.T) {
	agg := eventsourcing.NewAggregateRoot("user-123", "User")

	firstTime := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	secondTime := time.Date(2025, 1, 15, 14, 30, 0, 0, time.UTC)

	events := []*eventsourcing.Event{
		{
			ID:            "evt-1",
			AggregateID:   "user-123",
			AggregateType: "User",
			EventType:     "UserCreated",
			Version:       1,
			Timestamp:     firstTime,
			Data:          []byte(`{}`),
		},
		{
			ID:            "evt-2",
			AggregateID:   "user-123",
			AggregateType: "User",
			EventType:     "EmailVerified",
			Version:       2,
			Timestamp:     secondTime,
			Data:          []byte(`{}`),
		},
	}

	err := agg.LoadFromHistory(events)
	require.NoError(t, err)

	// Check UserCreated stats
	analytics := agg.Analytics()
	stats := analytics.GetStats("UserCreated")
	require.NotNil(t, stats)
	assert.Equal(t, int64(1), stats.Count)
	assert.Equal(t, firstTime, stats.FirstApplied)
	assert.Equal(t, firstTime, stats.LastApplied)

	// Check EmailVerified stats
	stats = analytics.GetStats("EmailVerified")
	require.NotNil(t, stats)
	assert.Equal(t, int64(1), stats.Count)
	assert.Equal(t, secondTime, stats.FirstApplied)
	assert.Equal(t, secondTime, stats.LastApplied)
}

func TestAggregateRoot_SetAndGetAnalytics(t *testing.T) {
	agg := eventsourcing.NewAggregateRoot("test-1", "TestAggregate")

	// Create custom analytics
	customAnalytics := eventsourcing.NewEventAnalytics()
	now := time.Now()
	customAnalytics.RecordEvent("Event1", now)
	customAnalytics.RecordEvent("Event2", now)

	// Set analytics
	agg.SetAnalytics(customAnalytics)

	// Get analytics
	analytics := agg.Analytics()
	assert.Equal(t, int64(2), analytics.TotalEvents)
	assert.Equal(t, int64(1), analytics.GetCount("Event1"))
	assert.Equal(t, int64(1), analytics.GetCount("Event2"))
}

func TestAggregateRoot_ResetAnalytics(t *testing.T) {
	agg := eventsourcing.NewAggregateRoot("test-1", "TestAggregate")

	events := []*eventsourcing.Event{
		{
			ID:            "evt-1",
			AggregateID:   "test-1",
			AggregateType: "TestAggregate",
			EventType:     "Created",
			Version:       1,
			Timestamp:     time.Now(),
			Data:          []byte(`{}`),
		},
	}

	err := agg.LoadFromHistory(events)
	require.NoError(t, err)

	assert.Equal(t, int64(1), agg.Analytics().TotalEvents)

	// Reset
	agg.ResetAnalytics()

	assert.Equal(t, int64(0), agg.Analytics().TotalEvents)
}

func TestAggregateRoot_AnalyticsInitialization(t *testing.T) {
	agg := eventsourcing.NewAggregateRoot("test-1", "TestAggregate")

	// Analytics should be initialized
	analytics := agg.Analytics()
	require.NotNil(t, analytics)
	assert.Equal(t, int64(0), analytics.TotalEvents)
}

func TestAggregateRoot_SetNilAnalytics(t *testing.T) {
	agg := eventsourcing.NewAggregateRoot("test-1", "TestAggregate")

	// Set nil analytics (should create new empty one)
	agg.SetAnalytics(nil)

	analytics := agg.Analytics()
	require.NotNil(t, analytics)
	assert.Equal(t, int64(0), analytics.TotalEvents)
}

func TestAggregateRoot_LoadHistoryPreservesAnalytics(t *testing.T) {
	agg := eventsourcing.NewAggregateRoot("test-1", "TestAggregate")

	// Load first batch
	events1 := []*eventsourcing.Event{
		{
			ID:            "evt-1",
			AggregateID:   "test-1",
			AggregateType: "TestAggregate",
			EventType:     "Created",
			Version:       1,
			Timestamp:     time.Now(),
			Data:          []byte(`{}`),
		},
	}

	err := agg.LoadFromHistory(events1)
	require.NoError(t, err)
	assert.Equal(t, int64(1), agg.Analytics().TotalEvents)

	// Load second batch (should add to existing analytics)
	events2 := []*eventsourcing.Event{
		{
			ID:            "evt-2",
			AggregateID:   "test-1",
			AggregateType: "TestAggregate",
			EventType:     "Updated",
			Version:       2,
			Timestamp:     time.Now(),
			Data:          []byte(`{}`),
		},
	}

	err = agg.LoadFromHistory(events2)
	require.NoError(t, err)
	assert.Equal(t, int64(2), agg.Analytics().TotalEvents)
}

func TestAggregateRoot_AnalyticsWithManyEvents(t *testing.T) {
	agg := eventsourcing.NewAggregateRoot("order-1", "Order")

	// Create many events of different types
	var events []*eventsourcing.Event
	eventTypes := []string{"OrderPlaced", "PaymentReceived", "OrderShipped", "OrderDelivered"}

	now := time.Now()
	version := int64(1)

	for i := 0; i < 100; i++ {
		eventType := eventTypes[i%len(eventTypes)]
		events = append(events, &eventsourcing.Event{
			ID:            eventsourcing.GenerateID(),
			AggregateID:   "order-1",
			AggregateType: "Order",
			EventType:     eventType,
			Version:       version,
			Timestamp:     now.Add(time.Duration(i) * time.Minute),
			Data:          []byte(`{}`),
		})
		version++
	}

	err := agg.LoadFromHistory(events)
	require.NoError(t, err)

	analytics := agg.Analytics()
	assert.Equal(t, int64(100), analytics.TotalEvents)

	// Each event type should have 25 occurrences
	for _, eventType := range eventTypes {
		assert.Equal(t, int64(25), analytics.GetCount(eventType))
	}

	// Check distribution
	distribution := analytics.GetDistribution()
	for _, percentage := range distribution {
		assert.InDelta(t, 25.0, percentage, 0.01)
	}
}

func TestAggregateRoot_AnalyticsSkipsOldVersions(t *testing.T) {
	agg := eventsourcing.NewAggregateRoot("test-1", "TestAggregate")

	// Load initial events
	events1 := []*eventsourcing.Event{
		{
			ID:            "evt-1",
			AggregateID:   "test-1",
			AggregateType: "TestAggregate",
			EventType:     "Created",
			Version:       1,
			Timestamp:     time.Now(),
			Data:          []byte(`{}`),
		},
		{
			ID:            "evt-2",
			AggregateID:   "test-1",
			AggregateType: "TestAggregate",
			EventType:     "Updated",
			Version:       2,
			Timestamp:     time.Now(),
			Data:          []byte(`{}`),
		},
	}

	err := agg.LoadFromHistory(events1)
	require.NoError(t, err)
	assert.Equal(t, int64(2), agg.Analytics().TotalEvents)

	// Try to load events with older versions (should be skipped)
	events2 := []*eventsourcing.Event{
		{
			ID:            "evt-1",
			AggregateID:   "test-1",
			AggregateType: "TestAggregate",
			EventType:     "Created",
			Version:       1, // Old version
			Timestamp:     time.Now(),
			Data:          []byte(`{}`),
		},
	}

	err = agg.LoadFromHistory(events2)
	require.NoError(t, err)

	// Analytics should not change
	assert.Equal(t, int64(2), agg.Analytics().TotalEvents)
}
