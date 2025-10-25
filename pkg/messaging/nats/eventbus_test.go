package nats_test

import (
	"testing"
	"time"

	"github.com/plaenen/eventstore/pkg/domain"
	natsserver "github.com/plaenen/eventstore/pkg/infrastructure/nats"
	"github.com/plaenen/eventstore/pkg/messaging"
	natspkg "github.com/plaenen/eventstore/pkg/messaging/nats"
)

func TestEmbeddedNATSEventBus(t *testing.T) {
	// Start embedded NATS server
	srv, err := natsserver.StartEmbeddedServer()
	if err != nil {
		t.Fatalf("failed to start embedded server: %v", err)
	}
	defer srv.Shutdown()

	// Create EventBus
	config := natspkg.DefaultConfig()
	config.URL = srv.URL()
	bus, err := natspkg.NewEventBus(config)
	if err != nil {
		t.Fatalf("failed to create event bus: %v", err)
	}
	defer bus.Close()

	t.Run("PublishAndSubscribe", func(t *testing.T) {
		received := make(chan *domain.Event, 1)

		// Subscribe to events
		sub, err := bus.Subscribe(messaging.EventFilter{
			AggregateTypes: []string{"TestAggregate"},
		}, func(envelope *domain.EventEnvelope) error {
			received <- &envelope.Event
			return nil
		})
		if err != nil {
			t.Fatalf("failed to subscribe: %v", err)
		}
		defer sub.Unsubscribe()

		// Give subscription time to be ready
		time.Sleep(100 * time.Millisecond)

		// Publish event
		event := &domain.Event{
			ID:            "test-event-1",
			AggregateID:   "agg-1",
			AggregateType: "TestAggregate",
			EventType:     "test.Created",
			Version:       1,
			Timestamp:     time.Now(),
			Data:          []byte("test data"),
			Metadata: domain.EventMetadata{
				PrincipalID: "test-user",
			},
		}

		err = bus.Publish([]*domain.Event{event})
		if err != nil {
			t.Fatalf("failed to publish event: %v", err)
		}

		// Wait for event
		select {
		case evt := <-received:
			if evt.ID != "test-event-1" {
				t.Errorf("expected event ID 'test-event-1', got '%s'", evt.ID)
			}
			if evt.AggregateID != "agg-1" {
				t.Errorf("expected aggregate ID 'agg-1', got '%s'", evt.AggregateID)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timeout waiting for event")
		}
	})

	t.Run("EventIdempotency", func(t *testing.T) {
		received := make(chan *domain.Event, 10)

		// Subscribe to events
		sub, err := bus.Subscribe(messaging.EventFilter{
			AggregateTypes: []string{"IdempotentAggregate"},
		}, func(envelope *domain.EventEnvelope) error {
			received <- &envelope.Event
			return nil
		})
		if err != nil {
			t.Fatalf("failed to subscribe: %v", err)
		}
		defer sub.Unsubscribe()

		time.Sleep(100 * time.Millisecond)

		// Publish same event twice (same ID = deduplication)
		event := &domain.Event{
			ID:            "idempotent-event-1",
			AggregateID:   "agg-2",
			AggregateType: "IdempotentAggregate",
			EventType:     "test.Created",
			Version:       1,
			Timestamp:     time.Now(),
			Data:          []byte("test"),
			Metadata:      domain.EventMetadata{},
		}

		// Publish twice
		err = bus.Publish([]*domain.Event{event})
		if err != nil {
			t.Fatalf("first publish failed: %v", err)
		}

		err = bus.Publish([]*domain.Event{event})
		if err != nil {
			t.Fatalf("second publish failed: %v", err)
		}

		// Should only receive one event due to deduplication
		select {
		case <-received:
			// First event received (expected)
		case <-time.After(2 * time.Second):
			t.Fatal("timeout waiting for first event")
		}

		// Check no duplicate
		select {
		case <-received:
			t.Error("received duplicate event (deduplication failed)")
		case <-time.After(500 * time.Millisecond):
			// Good, no duplicate
		}
	})

	t.Run("MultipleSubscribers", func(t *testing.T) {
		received1 := make(chan *domain.Event, 1)
		received2 := make(chan *domain.Event, 1)

		// First subscriber
		sub1, err := bus.Subscribe(messaging.EventFilter{
			AggregateTypes: []string{"MultiSubAggregate"},
		}, func(envelope *domain.EventEnvelope) error {
			received1 <- &envelope.Event
			return nil
		})
		if err != nil {
			t.Fatalf("failed to create sub1: %v", err)
		}
		defer sub1.Unsubscribe()

		// Second subscriber
		sub2, err := bus.Subscribe(messaging.EventFilter{
			AggregateTypes: []string{"MultiSubAggregate"},
		}, func(envelope *domain.EventEnvelope) error {
			received2 <- &envelope.Event
			return nil
		})
		if err != nil {
			t.Fatalf("failed to create sub2: %v", err)
		}
		defer sub2.Unsubscribe()

		time.Sleep(100 * time.Millisecond)

		// Publish event
		event := &domain.Event{
			ID:            "multi-sub-event-1",
			AggregateID:   "agg-3",
			AggregateType: "MultiSubAggregate",
			EventType:     "test.Created",
			Version:       1,
			Timestamp:     time.Now(),
			Data:          []byte("test"),
			Metadata:      domain.EventMetadata{},
		}

		err = bus.Publish([]*domain.Event{event})
		if err != nil {
			t.Fatalf("failed to publish: %v", err)
		}

		// Both subscribers should receive the event
		timeout := time.After(2 * time.Second)
		receivedCount := 0

		for receivedCount < 2 {
			select {
			case <-received1:
				receivedCount++
			case <-received2:
				receivedCount++
			case <-timeout:
				t.Fatalf("timeout: only received %d/2 events", receivedCount)
			}
		}
	})
}
