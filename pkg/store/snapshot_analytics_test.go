package store_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/plaenen/eventstore/pkg/domain"
	"github.com/plaenen/eventstore/pkg/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSnapshotMetadata_SetAndGetAnalytics(t *testing.T) {
	metadata := &store.SnapshotMetadata{
		Size:          1024,
		EventCount:    10,
		CreationTime:  100,
		SnapshotType:  "protobuf",
		SchemaVersion: "1.0.0",
	}

	// Create analytics
	analytics := domain.NewEventAnalytics()
	now := time.Now()
	analytics.RecordEvent("UserCreated", now)
	analytics.RecordEvent("EmailVerified", now.Add(time.Hour))

	// Set analytics
	err := metadata.SetAnalyticsFromAggregate(analytics)
	require.NoError(t, err)
	assert.NotEmpty(t, metadata.Analytics)

	// Get analytics
	retrieved, err := metadata.GetAnalytics()
	require.NoError(t, err)
	require.NotNil(t, retrieved)

	// Verify the analytics data is present
	assert.NotNil(t, retrieved["stats"])
	assert.NotNil(t, retrieved["total_events"])
}

func TestSnapshotMetadata_AnalyticsRoundTrip(t *testing.T) {
	metadata := &store.SnapshotMetadata{
		EventCount: 5,
	}

	// Create analytics with specific data
	analytics := domain.NewEventAnalytics()
	firstTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	secondTime := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)

	analytics.RecordEvent("OrderPlaced", firstTime)
	analytics.RecordEvent("OrderPlaced", secondTime)
	analytics.RecordEvent("PaymentReceived", firstTime)

	// Set analytics
	err := metadata.SetAnalyticsFromAggregate(analytics)
	require.NoError(t, err)

	// Marshal metadata
	metadataJSON, err := metadata.MarshalMetadata()
	require.NoError(t, err)

	// Unmarshal metadata
	restoredMetadata, err := store.UnmarshalMetadata(metadataJSON)
	require.NoError(t, err)
	require.NotNil(t, restoredMetadata)

	// Get analytics from restored metadata
	retrievedAnalytics, err := restoredMetadata.GetAnalytics()
	require.NoError(t, err)
	require.NotNil(t, retrievedAnalytics)

	// Verify total events
	totalEvents := retrievedAnalytics["total_events"]
	assert.Equal(t, float64(3), totalEvents) // JSON unmarshals numbers as float64

	// Verify stats exist
	stats, ok := retrievedAnalytics["stats"].(map[string]interface{})
	require.True(t, ok)
	assert.Len(t, stats, 2)

	// Verify OrderPlaced exists
	orderPlaced, ok := stats["OrderPlaced"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "OrderPlaced", orderPlaced["event_type"])
	assert.Equal(t, float64(2), orderPlaced["count"])
}

func TestSnapshotMetadata_NilAnalytics(t *testing.T) {
	metadata := &store.SnapshotMetadata{}

	// Set nil analytics
	err := metadata.SetAnalyticsFromAggregate(nil)
	require.NoError(t, err)
	assert.Empty(t, metadata.Analytics)

	// Get analytics from empty metadata
	retrieved, err := metadata.GetAnalytics()
	require.NoError(t, err)
	assert.Nil(t, retrieved)
}

func TestSnapshotMetadata_EmptyAnalytics(t *testing.T) {
	metadata := &store.SnapshotMetadata{}

	analytics := domain.NewEventAnalytics()

	// Set empty analytics
	err := metadata.SetAnalyticsFromAggregate(analytics)
	require.NoError(t, err)

	// Get analytics
	retrieved, err := metadata.GetAnalytics()
	require.NoError(t, err)
	require.NotNil(t, retrieved)

	// Verify zero events
	totalEvents := retrieved["total_events"]
	assert.Equal(t, float64(0), totalEvents)
}

func TestSnapshotMetadata_AnalyticsWithAggregateRoot(t *testing.T) {
	// Create aggregate with history
	agg := domain.NewAggregateRoot("user-123", "User")
	events := []*domain.Event{
		{
			ID:            "evt-1",
			AggregateID:   "user-123",
			AggregateType: "User",
			EventType:     "UserCreated",
			Version:       1,
			Timestamp:     time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			Data:          []byte(`{}`),
		},
		{
			ID:            "evt-2",
			AggregateID:   "user-123",
			AggregateType: "User",
			EventType:     "EmailVerified",
			Version:       2,
			Timestamp:     time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
			Data:          []byte(`{}`),
		},
		{
			ID:            "evt-3",
			AggregateID:   "user-123",
			AggregateType: "User",
			EventType:     "ProfileUpdated",
			Version:       3,
			Timestamp:     time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC),
			Data:          []byte(`{}`),
		},
	}

	err := agg.LoadFromHistory(events)
	require.NoError(t, err)

	// Create snapshot metadata with analytics
	metadata := &store.SnapshotMetadata{
		EventCount:    int64(len(events)),
		SnapshotType:  "protobuf",
		SchemaVersion: "1.0.0",
	}

	// Set analytics from aggregate
	err = metadata.SetAnalyticsFromAggregate(agg.Analytics())
	require.NoError(t, err)

	// Verify analytics was set
	assert.NotEmpty(t, metadata.Analytics)

	// Retrieve and verify
	retrieved, err := metadata.GetAnalytics()
	require.NoError(t, err)
	require.NotNil(t, retrieved)

	// Verify total events matches
	assert.Equal(t, float64(3), retrieved["total_events"])

	// Verify all event types are present
	stats, ok := retrieved["stats"].(map[string]interface{})
	require.True(t, ok)
	assert.Len(t, stats, 3)
	assert.Contains(t, stats, "UserCreated")
	assert.Contains(t, stats, "EmailVerified")
	assert.Contains(t, stats, "ProfileUpdated")
}

func TestSnapshotMetadata_LargeAnalytics(t *testing.T) {
	// Create analytics with many event types
	analytics := domain.NewEventAnalytics()
	now := time.Now()

	for i := 0; i < 100; i++ {
		eventType := "EventType" + string(rune('A'+i%26))
		for j := 0; j < 10; j++ {
			analytics.RecordEvent(eventType, now.Add(time.Duration(i*j)*time.Second))
		}
	}

	metadata := &store.SnapshotMetadata{}
	err := metadata.SetAnalyticsFromAggregate(analytics)
	require.NoError(t, err)

	// Verify it can be retrieved
	retrieved, err := metadata.GetAnalytics()
	require.NoError(t, err)
	require.NotNil(t, retrieved)

	// Verify total events
	assert.Equal(t, float64(1000), retrieved["total_events"])
}

func TestSnapshotMetadata_AnalyticsJSONMarshaling(t *testing.T) {
	metadata := &store.SnapshotMetadata{
		Size:          1024,
		EventCount:    5,
		CreationTime:  100,
		SnapshotType:  "protobuf",
		SchemaVersion: "1.0.0",
	}

	analytics := domain.NewEventAnalytics()
	analytics.RecordEvent("TestEvent", time.Now())

	err := metadata.SetAnalyticsFromAggregate(analytics)
	require.NoError(t, err)

	// Marshal to JSON
	data, err := json.Marshal(metadata)
	require.NoError(t, err)

	// Unmarshal from JSON
	var restored store.SnapshotMetadata
	err = json.Unmarshal(data, &restored)
	require.NoError(t, err)

	// Verify analytics is preserved
	assert.Equal(t, metadata.Analytics, restored.Analytics)

	// Verify we can get analytics from restored
	retrievedAnalytics, err := restored.GetAnalytics()
	require.NoError(t, err)
	require.NotNil(t, retrievedAnalytics)
}

func TestSnapshot_FullWorkflowWithAnalytics(t *testing.T) {
	// 1. Create aggregate with events
	agg := domain.NewAggregateRoot("order-456", "Order")
	events := []*domain.Event{
		{
			ID:            "evt-1",
			AggregateID:   "order-456",
			AggregateType: "Order",
			EventType:     "OrderPlaced",
			Version:       1,
			Timestamp:     time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
			Data:          []byte(`{}`),
		},
		{
			ID:            "evt-2",
			AggregateID:   "order-456",
			AggregateType: "Order",
			EventType:     "PaymentReceived",
			Version:       2,
			Timestamp:     time.Date(2025, 1, 1, 10, 5, 0, 0, time.UTC),
			Data:          []byte(`{}`),
		},
		{
			ID:            "evt-3",
			AggregateID:   "order-456",
			AggregateType: "Order",
			EventType:     "OrderShipped",
			Version:       3,
			Timestamp:     time.Date(2025, 1, 2, 14, 30, 0, 0, time.UTC),
			Data:          []byte(`{}`),
		},
	}

	err := agg.LoadFromHistory(events)
	require.NoError(t, err)

	// 2. Create snapshot with analytics
	metadata := &store.SnapshotMetadata{
		EventCount:    int64(len(events)),
		SnapshotType:  "protobuf",
		SchemaVersion: "1.0.0",
		CreationTime:  50,
	}

	err = metadata.SetAnalyticsFromAggregate(agg.Analytics())
	require.NoError(t, err)

	snapshot := &store.Snapshot{
		AggregateID:   agg.ID(),
		AggregateType: agg.Type(),
		Version:       agg.Version(),
		Data:          []byte("serialized state"),
		CreatedAt:     time.Now(),
		Metadata:      metadata,
	}

	// 3. Simulate save/load by marshaling metadata
	metadataJSON, err := snapshot.Metadata.MarshalMetadata()
	require.NoError(t, err)

	// 4. Restore metadata
	restoredMetadata, err := store.UnmarshalMetadata(metadataJSON)
	require.NoError(t, err)

	// 5. Get analytics from restored snapshot
	restoredAnalytics, err := restoredMetadata.GetAnalytics()
	require.NoError(t, err)
	require.NotNil(t, restoredAnalytics)

	// 6. Verify analytics matches original
	assert.Equal(t, float64(3), restoredAnalytics["total_events"])

	stats, ok := restoredAnalytics["stats"].(map[string]interface{})
	require.True(t, ok)
	assert.Len(t, stats, 3)

	// Verify each event type
	for _, eventType := range []string{"OrderPlaced", "PaymentReceived", "OrderShipped"} {
		eventStats, ok := stats[eventType].(map[string]interface{})
		require.True(t, ok, "Event type %s not found", eventType)
		assert.Equal(t, eventType, eventStats["event_type"])
		assert.Equal(t, float64(1), eventStats["count"])
	}

	// 7. Create new aggregate and restore analytics
	newAgg := domain.NewAggregateRoot(snapshot.AggregateID, snapshot.AggregateType)

	// Parse analytics back to EventAnalytics struct
	analyticsJSON, err := json.Marshal(restoredAnalytics)
	require.NoError(t, err)

	var parsedAnalytics domain.EventAnalytics
	err = json.Unmarshal(analyticsJSON, &parsedAnalytics)
	require.NoError(t, err)

	newAgg.SetAnalytics(&parsedAnalytics)

	// 8. Verify new aggregate has same analytics
	assert.Equal(t, int64(3), newAgg.Analytics().TotalEvents)
	assert.Equal(t, int64(1), newAgg.Analytics().GetCount("OrderPlaced"))
	assert.Equal(t, int64(1), newAgg.Analytics().GetCount("PaymentReceived"))
	assert.Equal(t, int64(1), newAgg.Analytics().GetCount("OrderShipped"))
}
