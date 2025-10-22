package eventsourcing

import (
	"context"
)

// EventHandlerRegistration represents a typed event handler registration.
// This is the key to enabling cross-domain projections.
type EventHandlerRegistration struct {
	EventType string
	Handler   func(context.Context, *EventEnvelope) error
}

// GenericProjectionBuilder provides a fluent API for building projections
// that can handle events from multiple aggregates/domains.
type GenericProjectionBuilder struct {
	name      string
	handlers  map[string]func(context.Context, *EventEnvelope) error
	resetFunc func(context.Context) error
}

// NewProjectionBuilder creates a new generic projection builder.
// This builder can accept event handlers from any aggregate/domain.
//
// Example:
//
//	projection := eventsourcing.NewProjectionBuilder("customer-overview").
//	    On(accountv1.OnAccountOpened(func(ctx, event, envelope) { ... })).
//	    On(orderv1.OnOrderPlaced(func(ctx, event, envelope) { ... })).
//	    Build()
func NewProjectionBuilder(name string) *GenericProjectionBuilder {
	return &GenericProjectionBuilder{
		name:     name,
		handlers: make(map[string]func(context.Context, *EventEnvelope) error),
	}
}

// On registers an event handler. The handler must be created by a generated
// On{EventName} function from any aggregate package.
//
// Example:
//
//	builder.On(accountv1.OnAccountOpened(func(ctx context.Context, event *accountv1.AccountOpenedEvent, envelope *eventsourcing.EventEnvelope) error {
//	    // Handle account opened event
//	    return nil
//	}))
func (b *GenericProjectionBuilder) On(registration EventHandlerRegistration) *GenericProjectionBuilder {
	b.handlers[registration.EventType] = registration.Handler
	return b
}

// OnReset registers a function to reset the projection state.
func (b *GenericProjectionBuilder) OnReset(resetFunc func(context.Context) error) *GenericProjectionBuilder {
	b.resetFunc = resetFunc
	return b
}

// Build creates the final Projection implementation.
func (b *GenericProjectionBuilder) Build() Projection {
	return &GenericProjection{
		name:      b.name,
		handlers:  b.handlers,
		resetFunc: b.resetFunc,
	}
}

// GenericProjection implements Projection with support for multiple domains.
type GenericProjection struct {
	name      string
	handlers  map[string]func(context.Context, *EventEnvelope) error
	resetFunc func(context.Context) error
}

// Name returns the projection name.
func (p *GenericProjection) Name() string {
	return p.name
}

// Handle dispatches events to registered typed handlers.
func (p *GenericProjection) Handle(ctx context.Context, envelope *EventEnvelope) error {
	handler, exists := p.handlers[envelope.EventType]
	if !exists {
		// No handler registered for this event type - skip it
		return nil
	}
	return handler(ctx, envelope)
}

// Reset resets the projection state.
func (p *GenericProjection) Reset(ctx context.Context) error {
	if p.resetFunc == nil {
		return nil // No reset function registered
	}
	return p.resetFunc(ctx)
}
