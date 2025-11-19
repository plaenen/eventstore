package messaging

import "github.com/plaenen/eventstore/pkg/domain"

// EventBus defines the interface for publishing and subscribing to events.
type EventBus interface {
	// Publish publishes events to all subscribers.
	Publish(events []*domain.Event) error

	// Subscribe subscribes to events matching the filter with optional configuration.
	// The handler is called for each event.
	// Variadic opts parameter is backward compatible (existing code can pass zero options).
	Subscribe(filter EventFilter, handler EventHandler, opts ...SubscribeOption) (Subscription, error)

	// Close closes the event bus and releases resources.
	Close() error
}

// EventFilter defines criteria for filtering events.
type EventFilter struct {
	// AggregateTypes filters by aggregate type (empty = all types)
	AggregateTypes []string

	// EventTypes filters by event type (empty = all types)
	EventTypes []string
}

// EventHandler processes an event.
// Return an error to nack the event (it will be retried based on bus configuration).
type EventHandler func(event *domain.EventEnvelope) error

// Subscription represents an active event subscription.
type Subscription interface {
	// Unsubscribe stops receiving events and cleans up resources.
	Unsubscribe() error
}

// SubscribeOption configures subscription behavior.
type SubscribeOption func(*SubscribeConfig)

// SubscribeConfig holds subscription configuration.
type SubscribeConfig struct {
	// ConsumerName is the durable consumer name (if empty, generates random name)
	ConsumerName string

	// StartSequence is the NATS stream sequence to start from (0 = deliver all from beginning)
	// This is INCLUSIVE (starts at this sequence, not after)
	StartSequence uint64

	// DeliverAll overrides StartSequence and delivers all messages from beginning
	DeliverAll bool
}

// WithConsumerName sets a deterministic consumer name for durable subscriptions.
// This is critical for maintaining subscription state across restarts.
func WithConsumerName(name string) SubscribeOption {
	return func(c *SubscribeConfig) {
		c.ConsumerName = name
	}
}

// WithStartSequence sets the stream sequence to start from (inclusive).
// Use this when resuming from a checkpoint.
// The sequence is INCLUSIVE, meaning it will start AT this sequence, not after it.
func WithStartSequence(sequence uint64) SubscribeOption {
	return func(c *SubscribeConfig) {
		c.StartSequence = sequence
		c.DeliverAll = false
	}
}

// WithDeliverAll sets the subscription to deliver all messages from the beginning,
// ignoring any start sequence. This is the default behavior when no options are provided.
func WithDeliverAll() SubscribeOption {
	return func(c *SubscribeConfig) {
		c.DeliverAll = true
	}
}
