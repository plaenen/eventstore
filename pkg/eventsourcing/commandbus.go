package eventsourcing

import (
	"context"
	"fmt"
	"sync"
)

// DefaultCommandBus is a simple in-memory implementation of CommandBus.
type DefaultCommandBus struct {
	handlers   map[string]CommandHandler
	middleware []CommandMiddleware
	eventBus   EventBus // Optional: for publishing events after command execution
	mu         sync.RWMutex
}

// NewCommandBus creates a new command bus instance.
func NewCommandBus() *DefaultCommandBus {
	return &DefaultCommandBus{
		handlers:   make(map[string]CommandHandler),
		middleware: make([]CommandMiddleware, 0),
	}
}

// NewCommandBusWithEventBus creates a command bus that automatically publishes events.
func NewCommandBusWithEventBus(eventBus EventBus) *DefaultCommandBus {
	return &DefaultCommandBus{
		handlers:   make(map[string]CommandHandler),
		middleware: make([]CommandMiddleware, 0),
		eventBus:   eventBus,
	}
}

// Register registers a handler for a specific command type.
func (b *DefaultCommandBus) Register(commandType string, handler CommandHandler) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, exists := b.handlers[commandType]; exists {
		panic(fmt.Sprintf("handler already registered for command type: %s", commandType))
	}

	b.handlers[commandType] = handler
}

// Use adds middleware to the command processing pipeline.
// Middleware is executed in the order it was added (first added = outermost).
func (b *DefaultCommandBus) Use(middleware CommandMiddleware) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.middleware = append(b.middleware, middleware)
}

// Send sends a command to its registered handler.
func (b *DefaultCommandBus) Send(ctx context.Context, cmd *CommandEnvelope) error {
	if cmd == nil {
		return ErrInvalidCommand
	}

	// Resolve command type from envelope or metadata
	commandType := cmd.Metadata.Custom["command_type"]
	if commandType == "" {
		return fmt.Errorf("%w: command_type not specified in metadata", ErrInvalidCommand)
	}

	// Get handler
	b.mu.RLock()
	handler, exists := b.handlers[commandType]
	middleware := b.middleware
	b.mu.RUnlock()

	if !exists {
		return fmt.Errorf("%w: %s", ErrCommandNotFound, commandType)
	}

	// Build middleware chain (reverse order so first added is outermost)
	finalHandler := handler
	for i := len(middleware) - 1; i >= 0; i-- {
		finalHandler = middleware[i](finalHandler)
	}

	// Execute handler
	events, err := finalHandler.Handle(ctx, cmd)
	if err != nil {
		return fmt.Errorf("command handler failed: %w", err)
	}

	// Publish events if event bus is configured
	if b.eventBus != nil && len(events) > 0 {
		if err := b.eventBus.Publish(events); err != nil {
			return fmt.Errorf("failed to publish events: %w", err)
		}
	}

	return nil
}

// GetRegisteredHandlers returns the list of registered command types (for debugging).
func (b *DefaultCommandBus) GetRegisteredHandlers() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	types := make([]string, 0, len(b.handlers))
	for t := range b.handlers {
		types = append(types, t)
	}
	return types
}
