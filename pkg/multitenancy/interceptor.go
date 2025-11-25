package multitenancy

import (
	"context"
	"fmt"

	"github.com/plaenen/eventstore/pkg/cqrs"
	"google.golang.org/protobuf/proto"
)

// TenantIsolationInterceptor ensures tenant isolation for all RPC calls.
// It validates that a tenant ID is present in the context.
//
// The tenant ID should be set in context by:
// 1. Client-side: Added to NATS headers before sending
// 2. Server-side: Extracted from NATS headers into context (done by nats.Server)
type TenantIsolationInterceptor struct{}

// NewTenantIsolationInterceptor creates a new tenant isolation interceptor.
func NewTenantIsolationInterceptor() *TenantIsolationInterceptor {
	return &TenantIsolationInterceptor{}
}

// InterceptHandler implements cqrs.ServerInterceptor.
func (t *TenantIsolationInterceptor) InterceptHandler(next cqrs.HandlerFunc) cqrs.HandlerFunc {
	return func(ctx context.Context, request proto.Message) (proto.Message, error) {
		// Get tenant ID from context (populated from NATS headers)
		tenantID, err := GetTenantID(ctx)
		if err != nil {
			return nil, fmt.Errorf("tenant isolation: %w", err)
		}

		// Validate tenant ID is not empty
		if tenantID == "" {
			return nil, fmt.Errorf("tenant isolation: tenant ID is empty")
		}

		// Execute handler with validated tenant context
		return next(ctx, request)
	}
}

// TenantExtractionInterceptor extracts tenant ID from different sources and adds it to context.
// Priority: 1. Context (already set), 2. Custom extractor function
//
// Note: In the Transport model, tenant ID typically comes from NATS headers
// which are automatically extracted into context by the server. This interceptor
// provides a fallback mechanism for custom extraction logic.
type TenantExtractionInterceptor struct {
	// Extractor is a custom function to extract tenant ID from the request
	// Used when tenant ID is not in context (e.g., embedded in the request message)
	Extractor func(ctx context.Context, request proto.Message) (string, error)
}

// NewTenantExtractionInterceptor creates a new tenant extraction interceptor.
func NewTenantExtractionInterceptor(extractor func(ctx context.Context, request proto.Message) (string, error)) *TenantExtractionInterceptor {
	return &TenantExtractionInterceptor{
		Extractor: extractor,
	}
}

// InterceptHandler implements cqrs.ServerInterceptor.
func (t *TenantExtractionInterceptor) InterceptHandler(next cqrs.HandlerFunc) cqrs.HandlerFunc {
	return func(ctx context.Context, request proto.Message) (proto.Message, error) {
		// Check if tenant ID already in context
		if HasTenantID(ctx) {
			return next(ctx, request)
		}

		// Use custom extractor if provided
		if t.Extractor != nil {
			tenantID, err := t.Extractor(ctx, request)
			if err != nil {
				return nil, fmt.Errorf("tenant extraction failed: %w", err)
			}
			ctx = WithTenantID(ctx, tenantID)
			return next(ctx, request)
		}

		// No tenant ID found
		return nil, fmt.Errorf("tenant ID not found in context and no extractor provided")
	}
}

// TenantAuthorizer validates that a principal has access to a tenant.
type TenantAuthorizer interface {
	// Authorize checks if a principal can access a tenant.
	Authorize(ctx context.Context, principalID, tenantID string) error
}

// TenantAuthorizationInterceptor ensures principal has access to tenant.
type TenantAuthorizationInterceptor struct {
	Authorizer TenantAuthorizer

	// PrincipalExtractor extracts the principal ID from context or request.
	// If nil, defaults to extracting from context using "principal_id" key.
	PrincipalExtractor func(ctx context.Context, request proto.Message) (string, error)
}

// NewTenantAuthorizationInterceptor creates a new tenant authorization interceptor.
func NewTenantAuthorizationInterceptor(authorizer TenantAuthorizer) *TenantAuthorizationInterceptor {
	return &TenantAuthorizationInterceptor{
		Authorizer: authorizer,
		PrincipalExtractor: func(ctx context.Context, request proto.Message) (string, error) {
			// Default: extract from context (set from NATS headers)
			principalID, ok := ctx.Value("principal_id").(string)
			if !ok || principalID == "" {
				return "", fmt.Errorf("principal ID not found in context")
			}
			return principalID, nil
		},
	}
}

// InterceptHandler implements cqrs.ServerInterceptor.
func (t *TenantAuthorizationInterceptor) InterceptHandler(next cqrs.HandlerFunc) cqrs.HandlerFunc {
	return func(ctx context.Context, request proto.Message) (proto.Message, error) {
		// Get tenant ID from context
		tenantID, err := GetTenantID(ctx)
		if err != nil {
			return nil, fmt.Errorf("tenant authorization: %w", err)
		}

		// Get principal ID
		principalID, err := t.PrincipalExtractor(ctx, request)
		if err != nil {
			return nil, fmt.Errorf("tenant authorization: %w", err)
		}

		// Check if principal can access this tenant
		if err := t.Authorizer.Authorize(ctx, principalID, tenantID); err != nil {
			return nil, fmt.Errorf("tenant authorization failed: %w", err)
		}

		return next(ctx, request)
	}
}

// ClientTenantInterceptor adds tenant ID to outgoing requests.
// This interceptor runs on the client side to inject tenant ID into NATS headers.
type ClientTenantInterceptor struct{}

// NewClientTenantInterceptor creates a new client-side tenant interceptor.
func NewClientTenantInterceptor() *ClientTenantInterceptor {
	return &ClientTenantInterceptor{}
}

// InterceptRequest implements cqrs.ClientInterceptor.
func (c *ClientTenantInterceptor) InterceptRequest(ctx context.Context, subject string, request proto.Message) (context.Context, error) {
	// Get tenant ID from context
	tenantID, err := GetTenantID(ctx)
	if err != nil {
		return ctx, fmt.Errorf("client tenant interceptor: %w", err)
	}

	// Tenant ID will be automatically added to NATS headers by the transport layer
	// if it exists in context with key "tenant_id"
	// No additional work needed here - just validate it exists

	if tenantID == "" {
		return ctx, fmt.Errorf("client tenant interceptor: tenant ID is empty")
	}

	return ctx, nil
}

// InterceptResponse implements cqrs.ClientInterceptor.
func (c *ClientTenantInterceptor) InterceptResponse(ctx context.Context, subject string, response proto.Message, err error) (proto.Message, error) {
	// No-op for responses
	return response, err
}
