package nats

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/plaenen/eventstore/pkg/domain"
	"github.com/plaenen/eventstore/pkg/messaging"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

// EventBus is a NATS-based implementation of domain.EventBus.
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
func (b *EventBus) Publish(events []*domain.Event) error {
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

// Subscribe subscribes to events matching the filter with optional configuration.
func (b *EventBus) Subscribe(
	filter messaging.EventFilter,
	handler messaging.EventHandler,
	opts ...messaging.SubscribeOption,
) (messaging.Subscription, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Build configuration from options
	config := &messaging.SubscribeConfig{
		DeliverAll: true, // Default: deliver all messages
	}
	for _, opt := range opts {
		opt(config)
	}

	// Generate consumer name if not provided
	consumerName := config.ConsumerName
	if consumerName == "" {
		consumerName = fmt.Sprintf("consumer_%s", domain.GenerateID()[:8])
	}

	// Build NATS subject from filter
	subject := b.buildSubject(filter)

	// Create consumer configuration
	consumerConfig := &nats.ConsumerConfig{
		Durable:           consumerName,
		DeliverSubject:    fmt.Sprintf("_INBOX.%s.%s", b.streamName, consumerName),
		AckPolicy:         nats.AckExplicitPolicy,
		AckWait:           30 * time.Second,
		MaxDeliver:        10,
		MaxAckPending:     1, // Only 1 unacked message at a time across ALL instances (maintains ordering)
		InactiveThreshold: 24 * time.Hour, // Prevent auto-deletion
		FilterSubject:     subject,
	}

	// Configure delivery policy based on options
	if config.DeliverAll {
		consumerConfig.DeliverPolicy = nats.DeliverAllPolicy
	} else if config.StartSequence > 0 {
		consumerConfig.DeliverPolicy = nats.DeliverByStartSequencePolicy
		consumerConfig.OptStartSeq = config.StartSequence

		// Validate sequence is within stream bounds
		if err := b.validateSequence(config.StartSequence); err != nil {
			return nil, fmt.Errorf("invalid start sequence: %w", err)
		}
	} else {
		// StartSequence == 0 means deliver new messages only
		consumerConfig.DeliverPolicy = nats.DeliverNewPolicy
	}

	// Try to create consumer, update if it already exists
	_, err := b.js.AddConsumer(b.streamName, consumerConfig)
	if err != nil {
		// Check if consumer already exists (check error message since there's no specific error type)
		errMsg := err.Error()
		if strings.Contains(errMsg, "already exists") || strings.Contains(errMsg, "already in use") {
			// Consumer exists, update it
			_, err = b.js.UpdateConsumer(b.streamName, consumerConfig)
			if err != nil {
				return nil, fmt.Errorf("failed to update existing consumer %s: %w", consumerName, err)
			}
		} else {
			return nil, fmt.Errorf("failed to create consumer %s: %w", consumerName, err)
		}
	}

	// Subscribe using durable consumer name
	sub, err := b.js.Subscribe(
		subject,
		func(msg *nats.Msg) {
		// Deserialize event
		event, err := b.deserializeEvent(msg.Data)
		if err != nil {
			// Log error and nack
			msg.Nak()
			return
		}

		// Get NATS metadata
		meta, err := msg.Metadata()
		if err != nil {
			// Metadata unavailable (shouldn't happen with JetStream)
			msg.Nak()
			return
		}

		// Create event envelope with NATS metadata
		envelope := &domain.EventEnvelope{
			Event: *event,
			NATSMetadata: &domain.NATSMetadata{
				StreamSequence:   meta.Sequence.Stream,
				ConsumerSequence: meta.Sequence.Consumer,
				Timestamp:        meta.Timestamp,
				NumDelivered:     meta.NumDelivered,
			},
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
		nats.Bind(b.streamName, consumerName),
		nats.ManualAck(),
	)

	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to consumer: %w", err)
	}

	// Store subscription
	b.subs[consumerName] = sub

	return &subscription{
		bus:          b,
		sub:          sub,
		consumerName: consumerName,
	}, nil
}

// validateSequence validates that a sequence number is within stream bounds.
func (b *EventBus) validateSequence(sequence uint64) error {
	streamInfo, err := b.js.StreamInfo(b.streamName)
	if err != nil {
		return fmt.Errorf("failed to get stream info: %w", err)
	}

	// Check if sequence is too old (stream purged)
	if sequence < streamInfo.State.FirstSeq && streamInfo.State.FirstSeq > 1 {
		return fmt.Errorf(
			"sequence %d is before stream first sequence %d (stream may have been purged)",
			sequence, streamInfo.State.FirstSeq,
		)
	}

	// Check if sequence is ahead of stream (caught up - this is OK)
	if sequence > streamInfo.State.LastSeq {
		// Log for visibility but don't error
		fmt.Printf("[NATS] Start sequence %d is ahead of stream last sequence %d (caught up)\n",
			sequence, streamInfo.State.LastSeq)
	}

	return nil
}

// buildSubject builds a NATS subject from an event filter.
func (b *EventBus) buildSubject(filter messaging.EventFilter) string {
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
func (b *EventBus) serializeEvent(event *domain.Event) ([]byte, error) {
	return json.Marshal(event)
}

// deserializeEvent deserializes an event from JSON.
func (b *EventBus) deserializeEvent(data []byte) (*domain.Event, error) {
	var event domain.Event
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

// subscription implements messaging.Subscription.
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
func DeserializeEventPayload(event *domain.Event, msg proto.Message) error {
	return proto.Unmarshal(event.Data, msg)
}

// DeserializeEventPayloadDynamic dynamically deserializes an event payload based on event type.
func DeserializeEventPayloadDynamic(event *domain.Event) (proto.Message, error) {
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
