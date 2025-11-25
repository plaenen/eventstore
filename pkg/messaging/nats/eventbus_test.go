package nats_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/plaenen/eventstore/pkg/eventsourcing"
	natsserver "github.com/plaenen/eventstore/pkg/infrastructure/nats"
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
		received := make(chan *eventsourcing.Event, 1)

		// Subscribe to events
		sub, err := bus.Subscribe(eventsourcing.EventFilter{
			AggregateTypes: []string{"TestAggregate"},
		}, func(envelope *eventsourcing.EventEnvelope) error {
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
		event := &eventsourcing.Event{
			ID:            "test-event-1",
			AggregateID:   "agg-1",
			AggregateType: "TestAggregate",
			EventType:     "test.Created",
			Version:       1,
			Timestamp:     time.Now(),
			Data:          []byte("test data"),
			Metadata: eventsourcing.EventMetadata{
				PrincipalID: "test-user",
			},
		}

		err = bus.Publish([]*eventsourcing.Event{event})
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
		// First, verify the stream has Duplicates set
		streamInfo, err := bus.StreamInfo()
		if err != nil {
			t.Fatalf("failed to get stream info: %v", err)
		}
		t.Logf("Stream Duplicates window: %v", streamInfo.Config.Duplicates)
		if streamInfo.Config.Duplicates == 0 {
			t.Fatal("Stream does not have Duplicates window configured!")
		}

		received := make(chan *eventsourcing.Event, 10)

		// Use a unique aggregate type and event ID to avoid collision with other tests
		uniqueID := fmt.Sprintf("idempotent-event-%d", time.Now().UnixNano())

		// Subscribe - ONLY accept events with our unique ID
		sub, err := bus.Subscribe(eventsourcing.EventFilter{
			AggregateTypes: []string{"IdempotentAggregate"},
		}, func(envelope *eventsourcing.EventEnvelope) error {
			t.Logf("Received event ID: %s", envelope.Event.ID)
			// Only forward events with our unique ID
			if envelope.Event.ID == uniqueID {
				received <- &envelope.Event
			}
			return nil
		})
		if err != nil {
			t.Fatalf("failed to subscribe: %v", err)
		}
		defer sub.Unsubscribe()

		time.Sleep(200 * time.Millisecond)

		// Publish same event twice (same ID = deduplication)
		event := &eventsourcing.Event{
			ID:            uniqueID,
			AggregateID:   "agg-2",
			AggregateType: "IdempotentAggregate",
			EventType:     "test.Created",
			Version:       1,
			Timestamp:     time.Now(),
			Data:          []byte("test"),
			Metadata:      eventsourcing.EventMetadata{},
		}

		// Publish twice
		err = bus.Publish([]*eventsourcing.Event{event})
		if err != nil {
			t.Fatalf("first publish failed: %v", err)
		}

		t.Logf("Published first event: %s", uniqueID)

		time.Sleep(50 * time.Millisecond) // Small delay between publishes

		err = bus.Publish([]*eventsourcing.Event{event})
		if err != nil {
			t.Fatalf("second publish failed: %v", err)
		}

		t.Logf("Published second event (should be deduplicated): %s", uniqueID)

		time.Sleep(200 * time.Millisecond) // Give time for messages to arrive

		// Check stream message count
		streamInfo2, err2 := bus.StreamInfo()
		if err2 != nil {
			t.Fatalf("failed to get stream info: %v", err2)
		}
		t.Logf("Stream total messages: %d", streamInfo2.State.Msgs)

		// Should only receive one event due to deduplication
		select {
		case evt := <-received:
			t.Logf("Received first event: %s", evt.ID)
		case <-time.After(2 * time.Second):
			t.Fatal("timeout waiting for first event")
		}

		// Check no duplicate
		select {
		case evt := <-received:
			t.Errorf("received duplicate event (deduplication failed): %s", evt.ID)
		case <-time.After(500 * time.Millisecond):
			t.Log("✓ No duplicate received - deduplication working!")
		}
	})

	t.Run("MultipleSubscribers", func(t *testing.T) {
		received1 := make(chan *eventsourcing.Event, 1)
		received2 := make(chan *eventsourcing.Event, 1)

		// First subscriber
		sub1, err := bus.Subscribe(eventsourcing.EventFilter{
			AggregateTypes: []string{"MultiSubAggregate"},
		}, func(envelope *eventsourcing.EventEnvelope) error {
			received1 <- &envelope.Event
			return nil
		})
		if err != nil {
			t.Fatalf("failed to create sub1: %v", err)
		}
		defer sub1.Unsubscribe()

		// Second subscriber
		sub2, err := bus.Subscribe(eventsourcing.EventFilter{
			AggregateTypes: []string{"MultiSubAggregate"},
		}, func(envelope *eventsourcing.EventEnvelope) error {
			received2 <- &envelope.Event
			return nil
		})
		if err != nil {
			t.Fatalf("failed to create sub2: %v", err)
		}
		defer sub2.Unsubscribe()

		time.Sleep(100 * time.Millisecond)

		// Publish event
		event := &eventsourcing.Event{
			ID:            "multi-sub-event-1",
			AggregateID:   "agg-3",
			AggregateType: "MultiSubAggregate",
			EventType:     "test.Created",
			Version:       1,
			Timestamp:     time.Now(),
			Data:          []byte("test"),
			Metadata:      eventsourcing.EventMetadata{},
		}

		err = bus.Publish([]*eventsourcing.Event{event})
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

	// Regression test for consumer type mismatch bug
	// https://github.com/plaenen/eventstore/issues/XXX
	t.Run("DurableConsumerIsPushBased", func(t *testing.T) {
		// This test verifies that durable consumers created with WithConsumerName
		// are push-based (have DeliverSubject set), preventing the error:
		// "nats: must use pull subscribe to bind to pull based consumer"

		received := make(chan *eventsourcing.Event, 1)

		// Subscribe with a deterministic consumer name (like projections do)
		sub, err := bus.Subscribe(
			eventsourcing.EventFilter{
				AggregateTypes: []string{"DurableTestAggregate"},
			},
			func(envelope *eventsourcing.EventEnvelope) error {
				received <- &envelope.Event
				return nil
			},
			eventsourcing.WithConsumerName("test_projection_consumer"),
		)
		if err != nil {
			t.Fatalf("failed to create durable consumer: %v", err)
		}
		defer sub.Unsubscribe()

		time.Sleep(100 * time.Millisecond)

		// Publish an event
		event := &eventsourcing.Event{
			ID:            "durable-test-1",
			AggregateID:   "agg-durable",
			AggregateType: "DurableTestAggregate",
			EventType:     "test.Created",
			Version:       1,
			Timestamp:     time.Now(),
			Data:          []byte("test"),
			Metadata:      eventsourcing.EventMetadata{},
		}

		err = bus.Publish([]*eventsourcing.Event{event})
		if err != nil {
			t.Fatalf("failed to publish: %v", err)
		}

		// Verify event was received (proves push-based consumer works)
		select {
		case evt := <-received:
			if evt.ID != "durable-test-1" {
				t.Errorf("expected event ID 'durable-test-1', got '%s'", evt.ID)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timeout waiting for event - consumer may be pull-based instead of push-based")
		}
	})

	// Regression test for DeliverPolicy update issue
	// https://github.com/plaenen/eventstore/issues/XXX
	t.Run("ConsumerRecreatedOnDeliverPolicyChange", func(t *testing.T) {
		// This test simulates what happens when a projection restarts:
		// 1. First start: No checkpoint → DeliverAll policy
		// 2. Second start: Has checkpoint with nats_sequence → DeliverByStartSequence policy
		// The consumer must be recreated since DeliverPolicy is immutable

		received := make(chan *eventsourcing.Event, 10)

		// First subscription: DeliverAll (like first projection start)
		sub1, err := bus.Subscribe(
			eventsourcing.EventFilter{
				AggregateTypes: []string{"PolicyChangeAggregate"},
			},
			func(envelope *eventsourcing.EventEnvelope) error {
				received <- &envelope.Event
				return nil
			},
			eventsourcing.WithConsumerName("test_policy_change_consumer"),
			eventsourcing.WithDeliverAll(),
		)
		if err != nil {
			t.Fatalf("failed to create first subscription: %v", err)
		}

		time.Sleep(100 * time.Millisecond)

		// Publish some events
		for i := 1; i <= 3; i++ {
			event := &eventsourcing.Event{
				ID:            fmt.Sprintf("policy-change-%d", i),
				AggregateID:   "agg-policy",
				AggregateType: "PolicyChangeAggregate",
				EventType:     "test.Event",
				Version:       int64(i),
				Timestamp:     time.Now(),
				Data:          []byte(fmt.Sprintf("event %d", i)),
				Metadata:      eventsourcing.EventMetadata{},
			}
			if err := bus.Publish([]*eventsourcing.Event{event}); err != nil {
				t.Fatalf("failed to publish event %d: %v", i, err)
			}
		}

		// Receive events
		for i := 0; i < 3; i++ {
			select {
			case <-received:
				// Good
			case <-time.After(2 * time.Second):
				t.Fatalf("timeout waiting for event %d", i+1)
			}
		}

		// Unsubscribe (simulates app restart)
		sub1.Unsubscribe()
		time.Sleep(100 * time.Millisecond)

		// Second subscription: DeliverNew (like projection restart with checkpoint)
		// This should recreate the consumer with new DeliverPolicy (DeliverAll → DeliverNew)
		// In real scenario, this would be DeliverByStartSequence, but that requires knowing
		// the actual stream sequence, so we use DeliverNew to just test policy change
		sub2, err := bus.Subscribe(
			eventsourcing.EventFilter{
				AggregateTypes: []string{"PolicyChangeAggregate"},
			},
			func(envelope *eventsourcing.EventEnvelope) error {
				received <- &envelope.Event
				return nil
			},
			eventsourcing.WithConsumerName("test_policy_change_consumer"), // Same name!
			// No options = DeliverNew policy (different from DeliverAll)
		)
		if err != nil {
			// If we get "deliver policy can not be updated", the fix failed
			if strings.Contains(err.Error(), "deliver policy can not be updated") {
				t.Fatalf("consumer recreation failed - got policy update error: %v", err)
			}
			t.Fatalf("failed to recreate subscription with different policy: %v", err)
		}
		defer sub2.Unsubscribe()

		time.Sleep(100 * time.Millisecond)

		// Publish another event
		event := &eventsourcing.Event{
			ID:            "policy-change-4",
			AggregateID:   "agg-policy",
			AggregateType: "PolicyChangeAggregate",
			EventType:     "test.Event",
			Version:       4,
			Timestamp:     time.Now(),
			Data:          []byte("event 4"),
			Metadata:      eventsourcing.EventMetadata{},
		}
		if err := bus.Publish([]*eventsourcing.Event{event}); err != nil {
			t.Fatalf("failed to publish event after recreation: %v", err)
		}

		// Should receive the new event (proves consumer was recreated successfully)
		select {
		case evt := <-received:
			if evt.ID != "policy-change-4" {
				t.Errorf("expected event 'policy-change-4', got '%s'", evt.ID)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timeout waiting for event after consumer recreation")
		}

		t.Log("✓ Consumer successfully recreated with new DeliverPolicy")
	})
}
