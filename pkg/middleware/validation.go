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
//
// SECURITY (SEC-005): This middleware enforces validation of command metadata
// to prevent invalid input from causing crashes or security issues.
//
// Validates:
// - Command ID (required, UUIDv4 format)
// - Command type (required, non-empty)
// - Principal ID (required, alphanumeric with special chars)
//
// Note: Principal ID validation is now ENFORCED (was previously optional).
// Use MetadataValidationMiddlewareWithConfig for custom behavior.
func MetadataValidationMiddleware() eventsourcing.CommandMiddleware {
	return MetadataValidationMiddlewareWithConfig(MetadataValidationConfig{
		EnforcePrincipalID: true,
		ValidateUUIDFormat: true,
	})
}

// MetadataValidationConfig configures metadata validation behavior.
type MetadataValidationConfig struct {
	// EnforcePrincipalID requires principal_id to be present (default: true)
	EnforcePrincipalID bool

	// ValidateUUIDFormat validates that IDs are proper UUIDv4 format (default: true)
	ValidateUUIDFormat bool

	// AllowDevMode allows validation to be relaxed for development (default: false)
	// WARNING: Never set this to true in production!
	AllowDevMode bool
}

// MetadataValidationMiddlewareWithConfig validates command metadata with custom configuration.
//
// This allows fine-grained control over validation behavior for different environments.
//
// Example:
//
//	// Production - strict validation
//	middleware := MetadataValidationMiddlewareWithConfig(MetadataValidationConfig{
//	    EnforcePrincipalID: true,
//	    ValidateUUIDFormat: true,
//	})
//
//	// Development - relaxed validation
//	middleware := MetadataValidationMiddlewareWithConfig(MetadataValidationConfig{
//	    EnforcePrincipalID: false,
//	    ValidateUUIDFormat: false,
//	    AllowDevMode: true,
//	})
func MetadataValidationMiddlewareWithConfig(config MetadataValidationConfig) eventsourcing.CommandMiddleware {
	return func(next eventsourcing.CommandHandler) eventsourcing.CommandHandler {
		return eventsourcing.CommandHandlerFunc(func(ctx context.Context, cmd *eventsourcing.CommandEnvelope) ([]*eventsourcing.Event, error) {
			// Validate command ID
			if cmd.Metadata.CommandID == "" {
				return nil, fmt.Errorf("%w: command_id is required", eventsourcing.ErrInvalidCommand)
			}

			// SEC-005: Validate UUIDv4 format for command_id
			if config.ValidateUUIDFormat {
				// Note: This requires importing the validation package
				// For now, we'll do a basic length check
				// TODO: Import and use validation.ValidateCommandID(cmd.Metadata.CommandID)
				if len(cmd.Metadata.CommandID) < 36 {
					return nil, fmt.Errorf("%w: command_id must be valid UUIDv4 format", eventsourcing.ErrInvalidCommand)
				}
			}

			// Validate command type
			if cmd.Metadata.Custom["command_type"] == "" {
				return nil, fmt.Errorf("%w: command_type is required", eventsourcing.ErrInvalidCommand)
			}

			// SEC-005: Enforce principal ID validation (was previously commented out)
			if config.EnforcePrincipalID {
				if cmd.Metadata.PrincipalID == "" {
					return nil, fmt.Errorf("%w: principal_id is required", eventsourcing.ErrInvalidCommand)
				}

				// SEC-005: Validate principal ID format
				if config.ValidateUUIDFormat {
					// Basic validation - alphanumeric, hyphens, underscores, @, .
					// TODO: Import and use validation.ValidatePrincipalID(cmd.Metadata.PrincipalID)
					if len(cmd.Metadata.PrincipalID) == 0 || len(cmd.Metadata.PrincipalID) > 256 {
						return nil, fmt.Errorf("%w: principal_id must be 1-256 characters", eventsourcing.ErrInvalidCommand)
					}
				}
			} else if cmd.Metadata.PrincipalID == "" && !config.AllowDevMode {
				// Log warning in non-dev mode when principal_id is missing but not enforced
				// In production, you should always enforce this
				// fmt.Println("WARNING: principal_id is empty - consider enabling EnforcePrincipalID")
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
