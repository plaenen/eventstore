// Package messaging provides backward compatibility aliases for types
// that have been moved to the eventsourcing package.
//
// Deprecated: Import types directly from github.com/plaenen/eventstore/pkg/eventsourcing instead.
package messaging

import "github.com/plaenen/eventstore/pkg/eventsourcing"

// Type aliases for backward compatibility.
// Deprecated: Use eventsourcing.* types directly.
type (
	EventBus        = eventsourcing.EventBus
	EventFilter     = eventsourcing.EventFilter
	EventHandler    = eventsourcing.EventHandler
	Subscription    = eventsourcing.Subscription
	SubscribeOption = eventsourcing.SubscribeOption
	SubscribeConfig = eventsourcing.SubscribeConfig

	// OutboxForwarder aliases - deprecated, use eventsourcing.OutboxForwarder
	OutboxForwarder       = eventsourcing.OutboxForwarder
	OutboxForwarderConfig = eventsourcing.OutboxForwarderConfig
	Logger                = eventsourcing.OutboxLogger
)

// Function aliases for backward compatibility.
// Deprecated: Use eventsourcing.* functions directly.
var (
	WithConsumerName  = eventsourcing.WithConsumerName
	WithStartSequence = eventsourcing.WithStartSequence
	WithDeliverAll    = eventsourcing.WithDeliverAll

	// OutboxForwarder functions - deprecated
	NewOutboxForwarder          = eventsourcing.NewOutboxForwarder
	DefaultOutboxForwarderConfig = eventsourcing.DefaultOutboxForwarderConfig
)
