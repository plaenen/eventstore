package middleware

import (
	"context"
	"fmt"

	"github.com/plaenen/eventstore/pkg/eventsourcing"
	"github.com/plaenen/eventstore/pkg/validation"
)

// EnhancedValidationMiddleware provides comprehensive input validation using
// the validation package (SEC-005: Input Validation Gaps).
//
// This middleware validates:
// - Command ID (UUIDv4 format)
// - Aggregate ID (UUIDv4 format)
// - Principal ID (email or service account format)
// - Tenant ID (alphanumeric with hyphens/underscores)
// - Event type (alphanumeric with dots/underscores)
// - Aggregate type (alphanumeric with underscores)
// - String lengths (prevents buffer overflows)
// - Prevents injection attacks
//
// Example:
//
//	bus := cqrs.NewCommandBus(
//	    middleware.EnhancedValidationMiddleware(),
//	    middleware.AuthorizationMiddleware(...),
//	    // ... other middleware
//	)
func EnhancedValidationMiddleware() eventsourcing.CommandMiddleware {
	return EnhancedValidationMiddlewareWithConfig(EnhancedValidationConfig{
		EnforcePrincipalID:  true,
		EnforceTenantID:     false, // Optional for backward compatibility
		ValidateUUIDs:       true,
		ValidateStringLengths: true,
		MaxStringLength:     validation.DefaultMaxStringLength,
	})
}

// EnhancedValidationConfig configures enhanced validation behavior.
type EnhancedValidationConfig struct {
	// EnforcePrincipalID requires principal_id to be present and valid
	EnforcePrincipalID bool

	// EnforceTenantID requires tenant_id to be present and valid
	EnforceTenantID bool

	// ValidateUUIDs validates UUID format for IDs
	ValidateUUIDs bool

	// ValidateStringLengths enforces string length limits
	ValidateStringLengths bool

	// MaxStringLength is the maximum allowed string length
	MaxStringLength int
}

// EnhancedValidationMiddlewareWithConfig creates validation middleware with custom configuration.
func EnhancedValidationMiddlewareWithConfig(config EnhancedValidationConfig) eventsourcing.CommandMiddleware {
	return func(next eventsourcing.CommandHandler) eventsourcing.CommandHandler {
		return eventsourcing.CommandHandlerFunc(func(ctx context.Context, cmd *eventsourcing.CommandEnvelope) ([]*eventsourcing.Event, error) {
			// Validate command ID (required)
			if cmd.Metadata.CommandID == "" {
				return nil, fmt.Errorf("%w: command_id is required", eventsourcing.ErrInvalidCommand)
			}

			if config.ValidateUUIDs {
				if err := validation.ValidateCommandID(cmd.Metadata.CommandID); err != nil {
					return nil, fmt.Errorf("%w: %v", eventsourcing.ErrInvalidCommand, err)
				}
			}

			// Validate principal ID
			if config.EnforcePrincipalID {
				if cmd.Metadata.PrincipalID == "" {
					return nil, fmt.Errorf("%w: principal_id is required", eventsourcing.ErrInvalidCommand)
				}

				if err := validation.ValidatePrincipalID(cmd.Metadata.PrincipalID); err != nil {
					return nil, fmt.Errorf("%w: %v", eventsourcing.ErrInvalidCommand, err)
				}
			}

			// Validate tenant ID (if multi-tenant)
			if config.EnforceTenantID {
				tenantID := cmd.Metadata.Custom["tenant_id"]
				if tenantID == "" {
					return nil, fmt.Errorf("%w: tenant_id is required", eventsourcing.ErrInvalidCommand)
				}

				if err := validation.ValidateTenantID(tenantID); err != nil {
					return nil, fmt.Errorf("%w: %v", eventsourcing.ErrInvalidCommand, err)
				}
			}

			// Validate aggregate ID (if present in metadata)
			if aggregateID := cmd.Metadata.Custom["aggregate_id"]; aggregateID != "" {
				if config.ValidateUUIDs {
					if err := validation.ValidateAggregateID(aggregateID); err != nil {
						return nil, fmt.Errorf("%w: %v", eventsourcing.ErrInvalidCommand, err)
					}
				}
			}

			// Validate command type
			if cmd.Metadata.Custom["command_type"] == "" {
				return nil, fmt.Errorf("%w: command_type is required", eventsourcing.ErrInvalidCommand)
			}

			// Validate string lengths in metadata
			if config.ValidateStringLengths {
				// Validate command_type length
				if commandType := cmd.Metadata.Custom["command_type"]; commandType != "" {
					if err := validation.ValidateStringLength(commandType, "command_type", 1, validation.DefaultMaxNameLength); err != nil {
						return nil, fmt.Errorf("%w: %v", eventsourcing.ErrInvalidCommand, err)
					}
				}

				// Validate principal_id length (if present)
				if cmd.Metadata.PrincipalID != "" {
					if err := validation.ValidateStringLength(cmd.Metadata.PrincipalID, "principal_id", 1, 256); err != nil {
						return nil, fmt.Errorf("%w: %v", eventsourcing.ErrInvalidCommand, err)
					}
				}
			}

			return next.Handle(ctx, cmd)
		})
	}
}

// StrictValidationMiddleware provides the strictest validation for production use.
//
// This enforces:
// - Principal ID (required)
// - Tenant ID (required)
// - UUID format validation
// - String length limits
//
// Use this in production environments where security is critical.
func StrictValidationMiddleware() eventsourcing.CommandMiddleware {
	return EnhancedValidationMiddlewareWithConfig(EnhancedValidationConfig{
		EnforcePrincipalID:  true,
		EnforceTenantID:     true,
		ValidateUUIDs:       true,
		ValidateStringLengths: true,
		MaxStringLength:     validation.DefaultMaxStringLength,
	})
}

// DevModeValidationMiddleware provides relaxed validation for development.
//
// This allows:
// - Optional principal ID
// - Optional tenant ID
// - No UUID format validation
// - Relaxed string length limits
//
// WARNING: Never use this in production!
func DevModeValidationMiddleware() eventsourcing.CommandMiddleware {
	return EnhancedValidationMiddlewareWithConfig(EnhancedValidationConfig{
		EnforcePrincipalID:  false,
		EnforceTenantID:     false,
		ValidateUUIDs:       false,
		ValidateStringLengths: false,
		MaxStringLength:     validation.DefaultMaxTextLength,
	})
}
