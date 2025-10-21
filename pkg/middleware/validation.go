package middleware

import (
	"context"
	"fmt"

	"github.com/plaenen/eventstore/pkg/eventsourcing"
)

// Validator defines the interface for validating commands.
type Validator interface {
	// Validate validates a command and returns an error if invalid.
	Validate(cmd interface{}) error
}

// ValidationMiddleware validates commands before they are handled.
func ValidationMiddleware(validator Validator) eventsourcing.CommandMiddleware {
	return func(next eventsourcing.CommandHandler) eventsourcing.CommandHandler {
		return eventsourcing.CommandHandlerFunc(func(ctx context.Context, cmd *eventsourcing.CommandEnvelope) ([]*eventsourcing.Event, error) {
			// Validate the command payload
			if err := validator.Validate(cmd.Command); err != nil {
				return nil, fmt.Errorf("command validation failed: %w", err)
			}

			// Proceed to next handler
			return next.Handle(ctx, cmd)
		})
	}
}

// MetadataValidationMiddleware validates command metadata.
func MetadataValidationMiddleware() eventsourcing.CommandMiddleware {
	return func(next eventsourcing.CommandHandler) eventsourcing.CommandHandler {
		return eventsourcing.CommandHandlerFunc(func(ctx context.Context, cmd *eventsourcing.CommandEnvelope) ([]*eventsourcing.Event, error) {
			// Validate command ID
			if cmd.Metadata.CommandID == "" {
				return nil, fmt.Errorf("%w: command_id is required", eventsourcing.ErrInvalidCommand)
			}

			// Validate command type
			if cmd.Metadata.Custom["command_type"] == "" {
				return nil, fmt.Errorf("%w: command_type is required", eventsourcing.ErrInvalidCommand)
			}

			// Validate principal ID (optional but recommended)
			if cmd.Metadata.PrincipalID == "" {
				// Log warning but don't fail
				// In production, you might want to enforce this
			}

			return next.Handle(ctx, cmd)
		})
	}
}

// ProtobufValidator can be used with protobuf generated validation.
// Example with protoc-gen-validate:
type ProtobufValidator struct{}

func (v *ProtobufValidator) Validate(cmd interface{}) error {
	// Check if command implements Validate() method
	type validatable interface {
		Validate() error
	}

	if validator, ok := cmd.(validatable); ok {
		return validator.Validate()
	}

	// No validation available, pass through
	return nil
}
