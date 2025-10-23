package messaging

import "github.com/plaenen/eventstore/pkg/domain"

// EventBus defines the interface for publishing and subscribing to events.
type EventBus interface {
	// Publish publishes events to all subscribers.
	Publish(events []*domain.Event) error

	// Subscribe subscribes to events matching the filter.
	// The handler is called for each event.
	Subscribe(filter EventFilter, handler EventHandler) (Subscription, error)

	// Close closes the event bus and releases resources.
	Close() error
}

// EventFilter defines criteria for filtering events.
type EventFilter struct {
	// AggregateTypes filters by aggregate type (empty = all types)
	AggregateTypes []string

	// EventTypes filters by event type (empty = all types)
	EventTypes []string

	// FromPosition starts consuming from this position (0 = from beginning)
	FromPosition int64
}

// EventHandler processes an event.
// Return an error to nack the event (it will be retried based on bus configuration).
type EventHandler func(event *domain.EventEnvelope) error

// Subscription represents an active event subscription.
type Subscription interface {
	// Unsubscribe stops receiving events and cleans up resources.
	Unsubscribe() error
}
