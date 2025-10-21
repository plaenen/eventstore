package multitenancy

import (
	"context"
	"fmt"

	"github.com/plaenen/eventsourcing/pkg/eventsourcing"
)

// TenantIsolationMiddleware ensures tenant isolation for all commands
// It validates that:
// 1. Tenant ID is present in context
// 2. Aggregate IDs match the tenant context
// 3. Commands cannot cross tenant boundaries
func TenantIsolationMiddleware() eventsourcing.CommandMiddleware {
	return func(next eventsourcing.CommandHandler) eventsourcing.CommandHandler {
		return eventsourcing.CommandHandlerFunc(func(ctx context.Context, envelope *eventsourcing.CommandEnvelope) ([]*eventsourcing.Event, error) {
			// Get tenant ID from context
			tenantID, err := GetTenantID(ctx)
			if err != nil {
				return nil, fmt.Errorf("tenant isolation: %w", err)
			}

			// Validate tenant ID in metadata (if set by client)
			if envelope.Metadata.TenantID != "" && envelope.Metadata.TenantID != tenantID {
				return nil, fmt.Errorf("tenant isolation: metadata tenant (%s) doesn't match context tenant (%s)",
					envelope.Metadata.TenantID, tenantID)
			}

			// Set tenant ID in metadata for downstream handlers
			envelope.Metadata.TenantID = tenantID

			// Execute command
			events, err := next.Handle(ctx, envelope)
			if err != nil {
				return nil, err
			}

			// Validate all emitted events belong to the correct tenant
			for _, event := range events {
				if err := ValidateTenantID(event.AggregateID, tenantID); err != nil {
					return nil, fmt.Errorf("tenant isolation: event validation failed: %w", err)
				}

				// Ensure tenant ID is set in event metadata
				event.Metadata.TenantID = tenantID
			}

			return events, nil
		})
	}
}

// TenantExtractionMiddleware extracts tenant ID from different sources
// Priority: 1. Context, 2. Metadata, 3. Custom extractor function
func TenantExtractionMiddleware(extractor func(*eventsourcing.CommandEnvelope) (string, error)) eventsourcing.CommandMiddleware {
	return func(next eventsourcing.CommandHandler) eventsourcing.CommandHandler {
		return eventsourcing.CommandHandlerFunc(func(ctx context.Context, envelope *eventsourcing.CommandEnvelope) ([]*eventsourcing.Event, error) {
			// Check if tenant ID already in context
			if HasTenantID(ctx) {
				return next.Handle(ctx, envelope)
			}

			// Try metadata
			if envelope.Metadata.TenantID != "" {
				ctx = WithTenantID(ctx, envelope.Metadata.TenantID)
				return next.Handle(ctx, envelope)
			}

			// Use custom extractor
			if extractor != nil {
				tenantID, err := extractor(envelope)
				if err != nil {
					return nil, fmt.Errorf("tenant extraction failed: %w", err)
				}
				ctx = WithTenantID(ctx, tenantID)
				return next.Handle(ctx, envelope)
			}

			return nil, fmt.Errorf("tenant ID not found and no extractor provided")
		})
	}
}

// TenantAuthorizationMiddleware ensures principal has access to tenant
type TenantAuthorizer interface {
	// Authorize checks if a principal can access a tenant
	Authorize(ctx context.Context, principalID, tenantID string) error
}

func TenantAuthorizationMiddleware(authorizer TenantAuthorizer) eventsourcing.CommandMiddleware {
	return func(next eventsourcing.CommandHandler) eventsourcing.CommandHandler {
		return eventsourcing.CommandHandlerFunc(func(ctx context.Context, envelope *eventsourcing.CommandEnvelope) ([]*eventsourcing.Event, error) {
			tenantID, err := GetTenantID(ctx)
			if err != nil {
				return nil, err
			}

			// Check if principal can access this tenant
			if err := authorizer.Authorize(ctx, envelope.Metadata.PrincipalID, tenantID); err != nil {
				return nil, fmt.Errorf("tenant authorization failed: %w", err)
			}

			return next.Handle(ctx, envelope)
		})
	}
}
