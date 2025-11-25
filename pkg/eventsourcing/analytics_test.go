package eventsourcing_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/plaenen/eventstore/pkg/eventsourcing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventAnalytics_Basic(t *testing.T) {
	analytics := eventsourcing.NewEventAnalytics()

	// Record some events
	now := time.Now()
	analytics.RecordEvent("UserCreated", now)
	analytics.RecordEvent("EmailVerified", now.Add(time.Hour))
	analytics.RecordEvent("UserCreated", now.Add(2*time.Hour))

	// Check total
	assert.Equal(t, int64(3), analytics.TotalEvents)

	// Check counts
	assert.Equal(t, int64(2), analytics.GetCount("UserCreated"))
	assert.Equal(t, int64(1), analytics.GetCount("EmailVerified"))
	assert.Equal(t, int64(0), analytics.GetCount("NonExistent"))

	// Check event types
	types := analytics.GetEventTypes()
	assert.Len(t, types, 2)
	assert.Contains(t, types, "UserCreated")
	assert.Contains(t, types, "EmailVerified")
}

func TestEventAnalytics_Stats(t *testing.T) {
	analytics := eventsourcing.NewEventAnalytics()

	firstTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	secondTime := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
	thirdTime := time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC)

	// Record events
	analytics.RecordEvent("OrderPlaced", firstTime)
	analytics.RecordEvent("OrderPlaced", secondTime)
	analytics.RecordEvent("OrderShipped", thirdTime)

	// Check OrderPlaced stats
	stats := analytics.GetStats("OrderPlaced")
	require.NotNil(t, stats)
	assert.Equal(t, "OrderPlaced", stats.EventType)
	assert.Equal(t, int64(2), stats.Count)
	assert.Equal(t, firstTime, stats.FirstApplied)
	assert.Equal(t, secondTime, stats.LastApplied)

	// Check OrderShipped stats
	stats = analytics.GetStats("OrderShipped")
	require.NotNil(t, stats)
	assert.Equal(t, int64(1), stats.Count)
	assert.Equal(t, thirdTime, stats.FirstApplied)
	assert.Equal(t, thirdTime, stats.LastApplied)

	// Check non-existent event
	stats = analytics.GetStats("NonExistent")
	assert.Nil(t, stats)
}

func TestEventAnalytics_MostLeastFrequent(t *testing.T) {
	analytics := eventsourcing.NewEventAnalytics()

	now := time.Now()

	// Record events with different frequencies
	analytics.RecordEvent("A", now)
	analytics.RecordEvent("A", now)
	analytics.RecordEvent("A", now)

	analytics.RecordEvent("B", now)
	analytics.RecordEvent("B", now)

	analytics.RecordEvent("C", now)

	// Most frequent
	assert.Equal(t, "A", analytics.GetMostFrequent())

	// Least frequent
	assert.Equal(t, "C", analytics.GetLeastFrequent())
}

func TestEventAnalytics_Distribution(t *testing.T) {
	analytics := eventsourcing.NewEventAnalytics()

	now := time.Now()

	// Record 100 events total
	for i := 0; i < 50; i++ {
		analytics.RecordEvent("TypeA", now)
	}
	for i := 0; i < 30; i++ {
		analytics.RecordEvent("TypeB", now)
	}
	for i := 0; i < 20; i++ {
		analytics.RecordEvent("TypeC", now)
	}

	distribution := analytics.GetDistribution()
	assert.Len(t, distribution, 3)
	assert.InDelta(t, 50.0, distribution["TypeA"], 0.01)
	assert.InDelta(t, 30.0, distribution["TypeB"], 0.01)
	assert.InDelta(t, 20.0, distribution["TypeC"], 0.01)
}

func TestEventAnalytics_JSON(t *testing.T) {
	analytics := eventsourcing.NewEventAnalytics()

	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	analytics.RecordEvent("UserCreated", now)
	analytics.RecordEvent("EmailVerified", now.Add(time.Hour))

	// Marshal
	data, err := json.Marshal(analytics)
	require.NoError(t, err)

	// Unmarshal
	var restored eventsourcing.EventAnalytics
	err = json.Unmarshal(data, &restored)
	require.NoError(t, err)

	// Verify
	assert.Equal(t, analytics.TotalEvents, restored.TotalEvents)
	assert.Len(t, restored.Stats, 2)
	assert.Equal(t, int64(1), restored.GetCount("UserCreated"))
	assert.Equal(t, int64(1), restored.GetCount("EmailVerified"))
}

func TestEventAnalytics_Clone(t *testing.T) {
	analytics := eventsourcing.NewEventAnalytics()

	now := time.Now()
	analytics.RecordEvent("Event1", now)
	analytics.RecordEvent("Event2", now)

	// Clone
	clone := analytics.Clone()
	require.NotNil(t, clone)

	// Verify deep copy
	assert.Equal(t, analytics.TotalEvents, clone.TotalEvents)
	assert.Equal(t, analytics.GetCount("Event1"), clone.GetCount("Event1"))
	assert.Equal(t, analytics.GetCount("Event2"), clone.GetCount("Event2"))

	// Modify original
	analytics.RecordEvent("Event1", now)

	// Clone should not be affected
	assert.NotEqual(t, analytics.GetCount("Event1"), clone.GetCount("Event1"))
}

func TestEventAnalytics_Reset(t *testing.T) {
	analytics := eventsourcing.NewEventAnalytics()

	now := time.Now()
	analytics.RecordEvent("Event1", now)
	analytics.RecordEvent("Event2", now)

	assert.Equal(t, int64(2), analytics.TotalEvents)

	// Reset
	analytics.Reset()

	assert.Equal(t, int64(0), analytics.TotalEvents)
	assert.Len(t, analytics.GetEventTypes(), 0)
}

func TestEventAnalytics_Merge(t *testing.T) {
	analytics1 := eventsourcing.NewEventAnalytics()
	analytics2 := eventsourcing.NewEventAnalytics()

	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	later := now.Add(24 * time.Hour)
	evenLater := later.Add(24 * time.Hour)

	// First analytics
	analytics1.RecordEvent("TypeA", now)
	analytics1.RecordEvent("TypeB", later)

	// Second analytics
	analytics2.RecordEvent("TypeA", evenLater) // Same type, later time
	analytics2.RecordEvent("TypeC", later)

	// Merge
	analytics1.Merge(analytics2)

	// Verify merged counts
	assert.Equal(t, int64(2), analytics1.GetCount("TypeA"))
	assert.Equal(t, int64(1), analytics1.GetCount("TypeB"))
	assert.Equal(t, int64(1), analytics1.GetCount("TypeC"))
	assert.Equal(t, int64(4), analytics1.TotalEvents)

	// Verify TypeA has correct first/last applied
	stats := analytics1.GetStats("TypeA")
	require.NotNil(t, stats)
	assert.Equal(t, now, stats.FirstApplied) // Earliest
	assert.Equal(t, evenLater, stats.LastApplied) // Latest
}

func TestEventAnalytics_EmptyDistribution(t *testing.T) {
	analytics := eventsourcing.NewEventAnalytics()

	distribution := analytics.GetDistribution()
	assert.NotNil(t, distribution)
	assert.Len(t, distribution, 0)
}

func TestEventAnalytics_NilClone(t *testing.T) {
	var analytics *eventsourcing.EventAnalytics
	clone := analytics.Clone()
	assert.Nil(t, clone)
}
