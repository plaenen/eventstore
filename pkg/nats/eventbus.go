package nats

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/plaenen/eventstore/pkg/eventsourcing"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

// EventBus is a NATS-based implementation of eventsourcing.EventBus.
// Uses JetStream for durable event streaming with at-least-once delivery.
type EventBus struct {
	nc         *nats.Conn
	js         nats.JetStreamContext
	streamName string
	mu         sync.RWMutex
	subs       map[string]*nats.Subscription
}

// Config holds configuration for the NATS event bus.
type Config struct {
	// URL is the NATS server URL
	URL string

	// StreamName is the JetStream stream name for events
	StreamName string

	// StreamSubjects are the subjects to publish events to (default: "events.*")
	StreamSubjects []string

	// MaxAge is how long to retain events in the stream
	MaxAge time.Duration

	// MaxBytes is the maximum bytes the stream can store
	MaxBytes int64
}

// DefaultConfig returns sensible defaults for NATS event bus.
func DefaultConfig() Config {
	return Config{
		URL:            nats.DefaultURL,
		StreamName:     "EVENTS",
		StreamSubjects: []string{"events.>"},
		MaxAge:         7 * 24 * time.Hour, // 7 days
		MaxBytes:       1024 * 1024 * 1024, // 1 GB
	}
}

// NewEventBus creates a new NATS-based event bus.
func NewEventBus(config Config) (*EventBus, error) {
	// Connect to NATS
	nc, err := nats.Connect(config.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	// Create JetStream context
	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("failed to create JetStream context: %w", err)
	}

	bus := &EventBus{
		nc:         nc,
		js:         js,
		streamName: config.StreamName,
		subs:       make(map[string]*nats.Subscription),
	}

	// Create or update stream
	if err := bus.ensureStream(config); err != nil {
		nc.Close()
		return nil, fmt.Errorf("failed to ensure stream: %w", err)
	}

	return bus, nil
}

// ensureStream creates or updates the JetStream stream.
func (b *EventBus) ensureStream(config Config) error {
	streamConfig := &nats.StreamConfig{
		Name:      config.StreamName,
		Subjects:  config.StreamSubjects,
		Retention: nats.InterestPolicy, // Messages deleted when all consumers have processed them
		MaxAge:    config.MaxAge,
		MaxBytes:  config.MaxBytes,
		Storage:   nats.FileStorage,
		Replicas:  1,
	}

	// Try to get existing stream
	stream, err := b.js.StreamInfo(config.StreamName)
	if err != nil {
		// Stream doesn't exist, create it
		_, err = b.js.AddStream(streamConfig)
		if err != nil {
			return fmt.Errorf("failed to create stream: %w", err)
		}
		return nil
	}

	// Update existing stream if needed
	if stream.Config.MaxAge != config.MaxAge || stream.Config.MaxBytes != config.MaxBytes {
		_, err = b.js.UpdateStream(streamConfig)
		if err != nil {
			return fmt.Errorf("failed to update stream: %w", err)
		}
	}

	return nil
}

// Publish publishes events to NATS JetStream.
func (b *EventBus) Publish(events []*eventsourcing.Event) error {
	if len(events) == 0 {
		return nil
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, event := range events {
		// Serialize event to JSON
		eventJSON, err := b.serializeEvent(event)
		if err != nil {
			return fmt.Errorf("failed to serialize event %s: %w", event.ID, err)
		}

		// Determine subject based on aggregate type and event type
		subject := fmt.Sprintf("events.%s.%s", event.AggregateType, event.EventType)

		// Publish to JetStream with event ID as message ID (deduplication)
		_, err = b.js.Publish(subject, eventJSON, nats.MsgId(event.ID))
		if err != nil {
			return fmt.Errorf("failed to publish event %s: %w", event.ID, err)
		}
	}

	return nil
}

// Subscribe subscribes to events matching the filter.
func (b *EventBus) Subscribe(filter eventsourcing.EventFilter, handler eventsourcing.EventHandler) (eventsourcing.Subscription, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Build NATS subject from filter
	subject := b.buildSubject(filter)

	// Create consumer name based on filter
	consumerName := fmt.Sprintf("consumer_%s", eventsourcing.GenerateID()[:8])

	// Create durable consumer
	sub, err := b.js.QueueSubscribe(
		subject,
		consumerName,
		func(msg *nats.Msg) {
			// Deserialize event
			event, err := b.deserializeEvent(msg.Data)
			if err != nil {
				// Log error and nack
				msg.Nak()
				return
			}

			// Create event envelope (payload will be deserialized by handler if needed)
			envelope := &eventsourcing.EventEnvelope{
				Event: *event,
			}

			// Call handler
			if err := handler(envelope); err != nil {
				// Handler failed, nack for retry
				msg.Nak()
				return
			}

			// Handler succeeded, ack
			msg.Ack()
		},
		nats.Durable(consumerName),
		nats.ManualAck(),
		nats.AckExplicit(),
	)

	if err != nil {
		return nil, fmt.Errorf("failed to subscribe: %w", err)
	}

	// Store subscription
	b.subs[consumerName] = sub

	return &subscription{
		bus:          b,
		sub:          sub,
		consumerName: consumerName,
	}, nil
}

// buildSubject builds a NATS subject from an event filter.
func (b *EventBus) buildSubject(filter eventsourcing.EventFilter) string {
	if len(filter.AggregateTypes) == 0 && len(filter.EventTypes) == 0 {
		return "events.>" // All events
	}

	if len(filter.AggregateTypes) == 1 && len(filter.EventTypes) == 0 {
		return fmt.Sprintf("events.%s.>", filter.AggregateTypes[0])
	}

	if len(filter.AggregateTypes) == 1 && len(filter.EventTypes) == 1 {
		return fmt.Sprintf("events.%s.%s", filter.AggregateTypes[0], filter.EventTypes[0])
	}

	// For complex filters, subscribe to all and filter in handler
	return "events.>"
}

// serializeEvent serializes an event to JSON.
func (b *EventBus) serializeEvent(event *eventsourcing.Event) ([]byte, error) {
	return json.Marshal(event)
}

// deserializeEvent deserializes an event from JSON.
func (b *EventBus) deserializeEvent(data []byte) (*eventsourcing.Event, error) {
	var event eventsourcing.Event
	if err := json.Unmarshal(data, &event); err != nil {
		return nil, err
	}
	return &event, nil
}

// Close closes the event bus and all subscriptions.
func (b *EventBus) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Unsubscribe all
	for _, sub := range b.subs {
		sub.Unsubscribe()
	}

	// Close NATS connection
	b.nc.Close()

	return nil
}

// subscription implements eventsourcing.Subscription.
type subscription struct {
	bus          *EventBus
	sub          *nats.Subscription
	consumerName string
}

func (s *subscription) Unsubscribe() error {
	s.bus.mu.Lock()
	defer s.bus.mu.Unlock()

	delete(s.bus.subs, s.consumerName)
	return s.sub.Unsubscribe()
}

// DeserializeEventPayload is a helper to deserialize event payloads.
// Users can call this to get the typed protobuf message from an event.
func DeserializeEventPayload(event *eventsourcing.Event, msg proto.Message) error {
	return proto.Unmarshal(event.Data, msg)
}

// DeserializeEventPayloadDynamic dynamically deserializes an event payload based on event type.
func DeserializeEventPayloadDynamic(event *eventsourcing.Event) (proto.Message, error) {
	// Look up message type in proto registry
	messageType, err := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName(event.EventType))
	if err != nil {
		return nil, fmt.Errorf("message type %s not found in registry: %w", event.EventType, err)
	}

	// Create new instance
	msg := messageType.New().Interface()

	// Unmarshal
	if err := proto.Unmarshal(event.Data, msg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal event: %w", err)
	}

	return msg, nil
}
