package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/plaenen/eventstore/pkg/cqrs"
	"github.com/plaenen/eventstore/pkg/domain"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// CommandBus is a NATS-based implementation of cqrs.CommandBus.
// Enables distributed command processing across multiple services.
type CommandBus struct {
	nc         *nats.Conn
	handlers   map[string]cqrs.CommandHandler
	middleware []cqrs.CommandMiddleware
	subs       map[string]*nats.Subscription
	timeout    time.Duration
	mu         sync.RWMutex
}

// CommandBusConfig holds configuration for the NATS command bus.
type CommandBusConfig struct {
	// URL is the NATS server URL
	URL string

	// Timeout is the maximum time to wait for a command response
	Timeout time.Duration

	// QueueGroup is the queue group name for load balancing handlers
	QueueGroup string
}

// DefaultCommandBusConfig returns sensible defaults.
func DefaultCommandBusConfig() CommandBusConfig {
	return CommandBusConfig{
		URL:        nats.DefaultURL,
		Timeout:    30 * time.Second,
		QueueGroup: "command-handlers",
	}
}

// NewCommandBus creates a new NATS-based command bus.
func NewCommandBus(config CommandBusConfig) (*CommandBus, error) {
	// Connect to NATS
	nc, err := nats.Connect(config.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	return &CommandBus{
		nc:         nc,
		handlers:   make(map[string]cqrs.CommandHandler),
		middleware: make([]cqrs.CommandMiddleware, 0),
		subs:       make(map[string]*nats.Subscription),
		timeout:    config.Timeout,
	}, nil
}

// Register registers a handler for a command type and subscribes to NATS.
func (b *CommandBus) Register(commandType string, handler cqrs.CommandHandler) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, exists := b.handlers[commandType]; exists {
		panic(fmt.Sprintf("handler already registered for command type: %s", commandType))
	}

	b.handlers[commandType] = handler

	// Subscribe to NATS subject for this command type
	// Subject pattern: commands.{AggregateType}.{CommandName}
	// For simplicity, we use the full command type as the subject
	subject := fmt.Sprintf("commands.%s", commandType)

	// Use queue group for load balancing across multiple instances
	sub, err := b.nc.QueueSubscribe(subject, "command-handlers", func(msg *nats.Msg) {
		b.handleMessage(msg, commandType)
	})

	if err != nil {
		panic(fmt.Sprintf("failed to subscribe to %s: %v", subject, err))
	}

	b.subs[commandType] = sub
}

// Use adds middleware to the command processing pipeline.
func (b *CommandBus) Use(middleware cqrs.CommandMiddleware) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.middleware = append(b.middleware, middleware)
}

// Send publishes a command to NATS and waits for the response (request-reply pattern).
func (b *CommandBus) Send(ctx context.Context, cmd *domain.CommandEnvelope) error {
	if cmd == nil {
		return domain.ErrInvalidCommand
	}

	// Get command type
	commandType := cmd.Metadata.Custom["command_type"]
	if commandType == "" {
		return fmt.Errorf("%w: command_type not specified in metadata", domain.ErrInvalidCommand)
	}

	// Serialize command envelope
	data, err := b.serializeCommandEnvelope(cmd)
	if err != nil {
		return fmt.Errorf("failed to serialize command: %w", err)
	}

	// Build NATS subject
	subject := fmt.Sprintf("commands.%s", commandType)

	// Send command using request-reply pattern
	msg, err := b.nc.RequestWithContext(ctx, subject, data)
	if err != nil {
		return fmt.Errorf("failed to send command: %w", err)
	}

	// Deserialize response
	var response CommandResponse
	if err := json.Unmarshal(msg.Data, &response); err != nil {
		return fmt.Errorf("failed to deserialize response: %w", err)
	}

	if response.Error != "" {
		return fmt.Errorf("command failed: %s", response.Error)
	}

	return nil
}

// handleMessage processes an incoming command message from NATS.
func (b *CommandBus) handleMessage(msg *nats.Msg, commandType string) {
	ctx := context.Background()

	// Deserialize command envelope
	envelope, err := b.deserializeCommandEnvelope(msg.Data)
	if err != nil {
		b.sendErrorResponse(msg, fmt.Errorf("failed to deserialize command: %w", err))
		return
	}

	// Get handler
	b.mu.RLock()
	handler, exists := b.handlers[commandType]
	middleware := b.middleware
	b.mu.RUnlock()

	if !exists {
		b.sendErrorResponse(msg, fmt.Errorf("no handler registered for command type: %s", commandType))
		return
	}

	// Build middleware chain
	finalHandler := handler
	for i := len(middleware) - 1; i >= 0; i-- {
		finalHandler = middleware[i](finalHandler)
	}

	// Execute handler
	events, err := finalHandler.Handle(ctx, envelope)
	if err != nil {
		b.sendErrorResponse(msg, err)
		return
	}

	// Send success response
	b.sendSuccessResponse(msg, events)
}

// serializeCommandEnvelope serializes a command envelope to JSON.
func (b *CommandBus) serializeCommandEnvelope(cmd *domain.CommandEnvelope) ([]byte, error) {
	// Serialize the protobuf command to bytes
	commandData, err := proto.Marshal(cmd.Command)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal command: %w", err)
	}

	// Create wire format
	wire := &CommandEnvelopeWire{
		CommandData: commandData,
		Metadata:    cmd.Metadata,
	}

	return json.Marshal(wire)
}

// deserializeCommandEnvelope deserializes a command envelope from JSON.
func (b *CommandBus) deserializeCommandEnvelope(data []byte) (*domain.CommandEnvelope, error) {
	var wire CommandEnvelopeWire
	if err := json.Unmarshal(data, &wire); err != nil {
		return nil, err
	}

	// Note: We can't deserialize the actual proto message here without knowing the type
	// The handler will need to deserialize it based on the command_type in metadata
	// For now, we'll store the raw bytes in a wrapper
	return &domain.CommandEnvelope{
		Command:  &RawCommand{Data: wire.CommandData},
		Metadata: wire.Metadata,
	}, nil
}

// sendSuccessResponse sends a success response back to the command sender.
func (b *CommandBus) sendSuccessResponse(msg *nats.Msg, events []*domain.Event) {
	response := CommandResponse{
		Success: true,
		Events:  events,
	}

	data, _ := json.Marshal(response)
	msg.Respond(data)
}

// sendErrorResponse sends an error response back to the command sender.
func (b *CommandBus) sendErrorResponse(msg *nats.Msg, err error) {
	response := CommandResponse{
		Success: false,
		Error:   err.Error(),
	}

	data, _ := json.Marshal(response)
	msg.Respond(data)
}

// Close closes the command bus and all subscriptions.
func (b *CommandBus) Close() error {
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

// CommandEnvelopeWire is the wire format for command envelopes.
type CommandEnvelopeWire struct {
	CommandData []byte                        `json:"command_data"`
	Metadata    domain.CommandMetadata `json:"metadata"`
}

// CommandResponse is sent back to command senders.
type CommandResponse struct {
	Success bool                   `json:"success"`
	Error   string                 `json:"error,omitempty"`
	Events  []*domain.Event `json:"events,omitempty"`
}

// RawCommand is a placeholder for commands that haven't been deserialized yet.
type RawCommand struct {
	Data []byte
}

func (r *RawCommand) ProtoReflect() protoreflect.Message {
	return nil
}

func (r *RawCommand) Reset() {}

func (r *RawCommand) String() string {
	return fmt.Sprintf("RawCommand{%d bytes}", len(r.Data))
}
