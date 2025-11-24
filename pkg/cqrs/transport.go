package cqrs

import (
	"context"

	"google.golang.org/protobuf/proto"
)

// Transport provides RPC-style request/response communication over a message bus.
// This is used for command and query handling in CQRS architectures.
//
// The transport is message-bus agnostic - implementations exist for NATS, HTTP, etc.
type Transport interface {
	// Request sends a request and waits for a response.
	// subject: The topic/subject to send to (e.g., "account.v1.AccountService.OpenAccount")
	// request: The command or query message (proto.Message)
	// Returns the response message directly or an error if the operation failed.
	// This is Go-idiomatic: success returns (message, nil), failure returns (nil, error).
	Request(ctx context.Context, subject string, request proto.Message) (proto.Message, error)

	// Close cleans up resources and connections
	Close() error
}

// Server handles incoming RPC requests and routes them to registered handlers.
// This is the server-side counterpart to Transport.
type Server interface {
	// RegisterHandler registers a handler function for a specific subject.
	// subject: The topic/subject to listen on (e.g., "account.v1.AccountService.OpenAccount")
	// handler: The function that processes requests and returns responses
	RegisterHandler(subject string, handler HandlerFunc) error

	// Start begins listening for and processing requests.
	// This is a blocking call that runs until the context is canceled.
	Start(ctx context.Context) error

	// Close stops the server and cleans up resources
	Close() error
}

// HandlerFunc is a function that handles an incoming request.
// It receives the request message and returns the response message or an error.
// This is Go-idiomatic: success returns (message, nil), failure returns (nil, error).
type HandlerFunc func(ctx context.Context, request proto.Message) (proto.Message, error)

// Interceptor allows intercepting requests on the client or server side.
// Interceptors can modify the context, add logging, handle authentication, etc.
type Interceptor interface {
	// Intercept wraps the handler function with additional logic.
	// next: The next handler in the chain (eventually the actual handler)
	// Returns a new handler that wraps the next handler
	Intercept(next HandlerFunc) HandlerFunc
}

// ClientInterceptor intercepts outbound requests before they're sent.
type ClientInterceptor interface {
	// InterceptRequest is called before sending a request.
	// It can modify the context or request, or abort the request by returning an error.
	InterceptRequest(ctx context.Context, subject string, request proto.Message) (context.Context, error)

	// InterceptResponse is called after receiving a response.
	// It can inspect or modify the response before returning to the caller.
	InterceptResponse(ctx context.Context, subject string, response proto.Message, err error) (proto.Message, error)
}

// ServerInterceptor intercepts inbound requests before they reach the handler.
type ServerInterceptor interface {
	// InterceptHandler wraps a handler function with additional logic.
	InterceptHandler(next HandlerFunc) HandlerFunc
}

// ChainClientInterceptors chains multiple client interceptors together.
func ChainClientInterceptors(interceptors ...ClientInterceptor) ClientInterceptor {
	return &chainedClientInterceptor{interceptors: interceptors}
}

type chainedClientInterceptor struct {
	interceptors []ClientInterceptor
}

func (c *chainedClientInterceptor) InterceptRequest(ctx context.Context, subject string, request proto.Message) (context.Context, error) {
	for _, interceptor := range c.interceptors {
		var err error
		ctx, err = interceptor.InterceptRequest(ctx, subject, request)
		if err != nil {
			return ctx, err
		}
	}
	return ctx, nil
}

func (c *chainedClientInterceptor) InterceptResponse(ctx context.Context, subject string, response proto.Message, err error) (proto.Message, error) {
	// Apply interceptors in reverse order for responses
	for i := len(c.interceptors) - 1; i >= 0; i-- {
		response, err = c.interceptors[i].InterceptResponse(ctx, subject, response, err)
	}
	return response, err
}

// ChainServerInterceptors chains multiple server interceptors together.
func ChainServerInterceptors(interceptors ...ServerInterceptor) ServerInterceptor {
	return &chainedServerInterceptor{interceptors: interceptors}
}

type chainedServerInterceptor struct {
	interceptors []ServerInterceptor
}

func (c *chainedServerInterceptor) InterceptHandler(next HandlerFunc) HandlerFunc {
	// Apply interceptors in reverse order so the first interceptor is the outermost
	handler := next
	for i := len(c.interceptors) - 1; i >= 0; i-- {
		handler = c.interceptors[i].InterceptHandler(handler)
	}
	return handler
}

// SubjectBuilder builds NATS subjects for RPC calls.
// This allows customization of subject naming conventions.
type SubjectBuilder interface {
	// BuildSubject constructs a subject string for the given service and method.
	// ctx: Context for extracting tenant, environment, or other routing information
	// packageName: Proto package name (e.g., "account.v1")
	// serviceName: Service name (e.g., "AccountService")
	// methodName: Method name (e.g., "OpenAccount")
	// Returns: Complete subject string (e.g., "myapp.prod.account.v1.AccountService.OpenAccount")
	BuildSubject(ctx context.Context, packageName, serviceName, methodName string) string
}

// DefaultSubjectBuilder creates subjects in the format: package.service.method
type DefaultSubjectBuilder struct{}

func (d *DefaultSubjectBuilder) BuildSubject(ctx context.Context, packageName, serviceName, methodName string) string {
	return packageName + "." + serviceName + "." + methodName
}

// PrefixedSubjectBuilder adds a configurable prefix to subjects.
// Useful for multi-tenant, multi-environment deployments.
type PrefixedSubjectBuilder struct {
	// Prefix is a static prefix (e.g., "myapp", "production")
	Prefix string

	// PrefixFunc dynamically computes prefix from context (e.g., extract tenant ID)
	// If both Prefix and PrefixFunc are set, PrefixFunc takes precedence
	PrefixFunc func(ctx context.Context) string
}

func (p *PrefixedSubjectBuilder) BuildSubject(ctx context.Context, packageName, serviceName, methodName string) string {
	prefix := p.Prefix
	if p.PrefixFunc != nil {
		prefix = p.PrefixFunc(ctx)
	}

	if prefix == "" {
		return packageName + "." + serviceName + "." + methodName
	}

	return prefix + "." + packageName + "." + serviceName + "." + methodName
}

// ContextKey types for storing values in context
type contextKey string

const (
	// SubjectBuilderKey is the context key for storing a SubjectBuilder
	SubjectBuilderKey contextKey = "cqrs.subject_builder"
)

// SubjectBuilderFromContext extracts the SubjectBuilder from context.
// Returns DefaultSubjectBuilder if none is set.
func SubjectBuilderFromContext(ctx context.Context) SubjectBuilder {
	if builder, ok := ctx.Value(SubjectBuilderKey).(SubjectBuilder); ok {
		return builder
	}
	return &DefaultSubjectBuilder{}
}

// WithSubjectBuilder adds a SubjectBuilder to the context.
func WithSubjectBuilder(ctx context.Context, builder SubjectBuilder) context.Context {
	return context.WithValue(ctx, SubjectBuilderKey, builder)
}

// ClientOption configures a CQRS client.
// This is a shared option type used by all generated clients.
type ClientOption func(client interface{})

// WithClientSubjectBuilder returns a ClientOption that sets a custom SubjectBuilder.
// Use this for multi-tenant deployments, environment-specific routing, etc.
//
// Example:
//
//	tenantBuilder := &cqrs.PrefixedSubjectBuilder{
//	    PrefixFunc: func(ctx context.Context) string {
//	        return ctx.Value("tenant_id").(string)
//	    },
//	}
//	client := NewOrderCommandServiceClient(transport, cqrs.WithClientSubjectBuilder(tenantBuilder))
func WithClientSubjectBuilder(builder SubjectBuilder) ClientOption {
	return func(client interface{}) {
		type subjectBuilderSetter interface {
			setSubjectBuilder(SubjectBuilder)
		}
		if c, ok := client.(subjectBuilderSetter); ok {
			c.setSubjectBuilder(builder)
		}
	}
}
