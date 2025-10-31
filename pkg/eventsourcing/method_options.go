package eventsourcing

import "context"

// MethodOptions holds contextual data for aggregate method invocations
// This includes authentication, tracing, and other cross-cutting concerns
type MethodOptions struct {
	Principal     *Principal
	Headers       map[string]string
	Metadata      map[string]interface{}
	TraceID       string
	TenantID      string
	CorrelationID string
}

// MethodOption configures method execution context
type MethodOption func(*MethodOptions)

// WithPrincipal sets the authenticated principal (user/service identity)
// Use this to pass authentication context to aggregate methods
func WithPrincipal(principal *Principal) MethodOption {
	return func(opts *MethodOptions) {
		opts.Principal = principal
	}
}

// WithHeader sets a single header value
func WithHeader(key, value string) MethodOption {
	return func(opts *MethodOptions) {
		if opts.Headers == nil {
			opts.Headers = make(map[string]string)
		}
		opts.Headers[key] = value
	}
}

// WithHeaders sets multiple headers at once
func WithHeaders(headers map[string]string) MethodOption {
	return func(opts *MethodOptions) {
		if opts.Headers == nil {
			opts.Headers = make(map[string]string)
		}
		for k, v := range headers {
			opts.Headers[k] = v
		}
	}
}

// WithTraceID sets the distributed tracing ID
func WithTraceID(traceID string) MethodOption {
	return func(opts *MethodOptions) {
		opts.TraceID = traceID
	}
}

// WithTenantID sets the tenant ID for multi-tenant systems
func WithTenantID(tenantID string) MethodOption {
	return func(opts *MethodOptions) {
		opts.TenantID = tenantID
	}
}

// WithCorrelationID sets the correlation ID for request tracking
func WithCorrelationID(correlationID string) MethodOption {
	return func(opts *MethodOptions) {
		opts.CorrelationID = correlationID
	}
}

// WithMetadata sets custom metadata
func WithMetadata(key string, value interface{}) MethodOption {
	return func(opts *MethodOptions) {
		if opts.Metadata == nil {
			opts.Metadata = make(map[string]interface{})
		}
		opts.Metadata[key] = value
	}
}

// ApplyMethodOptions applies all options to create final MethodOptions
func ApplyMethodOptions(opts ...MethodOption) *MethodOptions {
	options := &MethodOptions{
		Headers:  make(map[string]string),
		Metadata: make(map[string]interface{}),
	}
	for _, opt := range opts {
		opt(options)
	}
	return options
}

// Principal represents an authenticated user or service
type Principal struct {
	ID          string
	Username    string
	Email       string
	Roles       []string
	Permissions []string
	Claims      map[string]interface{}
}

// HasRole checks if principal has a specific role
func (p *Principal) HasRole(role string) bool {
	for _, r := range p.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// HasPermission checks if principal has a specific permission
func (p *Principal) HasPermission(permission string) bool {
	for _, perm := range p.Permissions {
		if perm == permission {
			return true
		}
	}
	return false
}

// HasAnyPermission checks if principal has any of the specified permissions
func (p *Principal) HasAnyPermission(permissions ...string) bool {
	for _, perm := range permissions {
		if p.HasPermission(perm) {
			return true
		}
	}
	return false
}

// HasAllPermissions checks if principal has all specified permissions
func (p *Principal) HasAllPermissions(permissions ...string) bool {
	for _, perm := range permissions {
		if !p.HasPermission(perm) {
			return false
		}
	}
	return true
}

// Context keys for extracting method options
type contextKey string

const (
	PrincipalContextKey     contextKey = "eventsourcing.principal"
	TraceIDContextKey       contextKey = "eventsourcing.traceID"
	TenantIDContextKey      contextKey = "eventsourcing.tenantID"
	CorrelationIDContextKey contextKey = "eventsourcing.correlationID"
	HeadersContextKey       contextKey = "eventsourcing.headers"
)

// WithPrincipalContext adds principal to context
func WithPrincipalContext(ctx context.Context, principal *Principal) context.Context {
	return context.WithValue(ctx, PrincipalContextKey, principal)
}

// GetPrincipalFromContext extracts principal from context
func GetPrincipalFromContext(ctx context.Context) (*Principal, bool) {
	principal, ok := ctx.Value(PrincipalContextKey).(*Principal)
	return principal, ok
}

// WithTraceIDContext adds trace ID to context
func WithTraceIDContext(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, TraceIDContextKey, traceID)
}

// GetTraceIDFromContext extracts trace ID from context
func GetTraceIDFromContext(ctx context.Context) (string, bool) {
	traceID, ok := ctx.Value(TraceIDContextKey).(string)
	return traceID, ok
}

// WithTenantIDContext adds tenant ID to context
func WithTenantIDContext(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, TenantIDContextKey, tenantID)
}

// GetTenantIDFromContext extracts tenant ID from context
func GetTenantIDFromContext(ctx context.Context) (string, bool) {
	tenantID, ok := ctx.Value(TenantIDContextKey).(string)
	return tenantID, ok
}

// WithCorrelationIDContext adds correlation ID to context
func WithCorrelationIDContext(ctx context.Context, correlationID string) context.Context {
	return context.WithValue(ctx, CorrelationIDContextKey, correlationID)
}

// GetCorrelationIDFromContext extracts correlation ID from context
func GetCorrelationIDFromContext(ctx context.Context) (string, bool) {
	correlationID, ok := ctx.Value(CorrelationIDContextKey).(string)
	return correlationID, ok
}

// WithHeadersContext adds headers to context
func WithHeadersContext(ctx context.Context, headers map[string]string) context.Context {
	return context.WithValue(ctx, HeadersContextKey, headers)
}

// GetHeadersFromContext extracts headers from context
func GetHeadersFromContext(ctx context.Context) (map[string]string, bool) {
	headers, ok := ctx.Value(HeadersContextKey).(map[string]string)
	return headers, ok
}

// ExtractMethodOptionsFromContext extracts all method options from context
// This is typically called by generated server code
func ExtractMethodOptionsFromContext(ctx context.Context) []MethodOption {
	var opts []MethodOption

	if principal, ok := GetPrincipalFromContext(ctx); ok {
		opts = append(opts, WithPrincipal(principal))
	}

	if traceID, ok := GetTraceIDFromContext(ctx); ok {
		opts = append(opts, WithTraceID(traceID))
	}

	if tenantID, ok := GetTenantIDFromContext(ctx); ok {
		opts = append(opts, WithTenantID(tenantID))
	}

	if correlationID, ok := GetCorrelationIDFromContext(ctx); ok {
		opts = append(opts, WithCorrelationID(correlationID))
	}

	if headers, ok := GetHeadersFromContext(ctx); ok {
		opts = append(opts, WithHeaders(headers))
	}

	return opts
}
