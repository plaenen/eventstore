package nats

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/micro"
	"github.com/plaenen/eventstore/pkg/cqrs"
	"github.com/plaenen/eventstore/pkg/eventsourcing"
	"github.com/plaenen/eventstore/pkg/observability"
	"go.opentelemetry.io/otel/propagation"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

// Server implements cqrs.Server using NATS microservices
type Server struct {
	nc       *nats.Conn
	config   *cqrs.ServerConfig
	handlers map[string]cqrs.HandlerFunc
	services []micro.Service
	mu       sync.RWMutex
	ctx      context.Context
	cancel   context.CancelFunc

	// Service metadata
	serviceName    string
	serviceVersion string

	// Observability (optional)
	telemetry *observability.Telemetry
}

// ServerConfig extends the base server config with NATS-specific options
type ServerConfig struct {
	*cqrs.ServerConfig

	// URL is the NATS server URL (e.g., "nats://localhost:4222")
	URL string

	// Name is the service name (e.g., "AccountService")
	Name string

	// Version is the service version (e.g., "1.0.0")
	Version string

	// Description is a human-readable service description
	Description string

	// Credentials for authentication (optional)
	Token string
	User  string
	Pass  string

	// Telemetry for observability (optional)
	Telemetry *observability.Telemetry
}

// NewServer creates a new NATS server for handling requests
func NewServer(config *ServerConfig) (*Server, error) {
	if config == nil {
		config = &ServerConfig{
			ServerConfig: cqrs.DefaultServerConfig(),
			URL:          "nats://localhost:4222",
			Name:         "eventsourcing-server",
			Version:      "1.0.0",
			Description:  "Event sourcing service",
		}
	}

	if config.Version == "" {
		config.Version = "1.0.0"
	}

	// Build NATS options
	opts := []nats.Option{
		nats.Name(config.Name),
		nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
			if err != nil {
				fmt.Printf("NATS server disconnected: %v\n", err)
			}
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			fmt.Printf("NATS server reconnected to %s\n", nc.ConnectedUrl())
		}),
	}

	// Add authentication if provided
	if config.Token != "" {
		opts = append(opts, nats.Token(config.Token))
	} else if config.User != "" && config.Pass != "" {
		opts = append(opts, nats.UserInfo(config.User, config.Pass))
	}

	// Connect to NATS
	nc, err := nats.Connect(config.URL, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Server{
		nc:             nc,
		config:         config.ServerConfig,
		handlers:       make(map[string]cqrs.HandlerFunc),
		services:       make([]micro.Service, 0),
		ctx:            ctx,
		cancel:         cancel,
		serviceName:    config.Name,
		serviceVersion: config.Version,
		telemetry:      config.Telemetry,
	}, nil
}

// RegisterHandler registers a handler for a specific subject
func (s *Server) RegisterHandler(subject string, handler cqrs.HandlerFunc) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.handlers[subject]; exists {
		return fmt.Errorf("handler already registered for subject: %s", subject)
	}

	// Wrap handler with observability middleware if telemetry is configured
	if s.telemetry != nil {
		middleware := observability.HandlerMiddleware(s.telemetry, subject)
		handler = middleware(handler)
	}

	s.handlers[subject] = handler
	return nil
}

// Start begins listening for requests on all registered subjects
func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.handlers) == 0 {
		return fmt.Errorf("no handlers registered")
	}

	// Create a single NATS microservice with multiple endpoints
	config := micro.Config{
		Name:        s.serviceName,
		Version:     s.serviceVersion,
		Description: fmt.Sprintf("Event sourcing service with %d endpoints", len(s.handlers)),
		QueueGroup:  s.config.QueueGroup,
	}

	svc, err := micro.AddService(s.nc, config)
	if err != nil {
		return fmt.Errorf("failed to add service: %w", err)
	}

	// Add all handlers as endpoints
	for subject, handler := range s.handlers {
		// Capture handler in closure
		h := handler

		// Create endpoint name by replacing dots with dashes (endpoint names can't have dots)
		endpointName := strings.ReplaceAll(subject, ".", "-")

		// Add endpoint with the subject
		err = svc.AddEndpoint(endpointName, micro.HandlerFunc(func(req micro.Request) {
			s.handleMicroRequest(req, h)
		}), micro.WithEndpointSubject(subject))
		if err != nil {
			return fmt.Errorf("failed to add endpoint %s: %w", subject, err)
		}
	}

	s.services = append(s.services, svc)
	fmt.Printf("NATS service started: %s v%s with %d endpoints\n", s.serviceName, s.serviceVersion, len(s.handlers))
	return nil
}

// handleMicroRequest processes an incoming micro request
func (s *Server) handleMicroRequest(req micro.Request, handler cqrs.HandlerFunc) {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(s.ctx, s.config.HandlerTimeout)
	defer cancel()

	// Extract trace context from NATS headers for distributed tracing
	if s.telemetry != nil {
		propagator := propagation.TraceContext{}
		ctx = propagator.Extract(ctx, &natsMicroHeaderCarrier{headers: req.Headers()})
	}

	// Extract metadata from headers
	if tenantID := req.Headers().Get("Tenant-ID"); tenantID != "" {
		ctx = context.WithValue(ctx, "tenant_id", tenantID)
	}
	if traceID := req.Headers().Get("Trace-ID"); traceID != "" {
		ctx = context.WithValue(ctx, "trace_id", traceID)
	}

	// Deserialize request based on Message-Type header
	messageType := req.Headers().Get("Message-Type")
	if messageType == "" {
		s.respondMicroWithError(req, "INVALID_REQUEST", "Missing Message-Type header")
		return
	}

	// Create request message instance
	request, err := s.createMessageInstance(messageType)
	if err != nil {
		s.respondMicroWithError(req, "INVALID_MESSAGE_TYPE", fmt.Sprintf("Unknown message type: %s", messageType))
		return
	}

	// Unmarshal request
	if err := proto.Unmarshal(req.Data(), request); err != nil {
		s.respondMicroWithError(req, "INVALID_REQUEST", fmt.Sprintf("Failed to unmarshal request: %v", err))
		return
	}

	// Call handler
	response, err := handler(ctx, request)
	if err != nil {
		s.respondMicroWithError(req, "HANDLER_ERROR", err.Error())
		return
	}

	// If handler returned nil, create error response
	if response == nil {
		response = eventsourcing.NewSimpleErrorResponse("HANDLER_ERROR", "Handler returned nil response")
	}

	// Marshal response
	responseData, err := proto.Marshal(response)
	if err != nil {
		s.respondMicroWithError(req, "INTERNAL_ERROR", fmt.Sprintf("Failed to marshal response: %v", err))
		return
	}

	// Send response
	if err := req.Respond(responseData); err != nil {
		fmt.Printf("Failed to send response: %v\n", err)
	}
}

// createMessageInstance creates a proto message instance from a type name
func (s *Server) createMessageInstance(messageType string) (proto.Message, error) {
	// Look up message type in proto registry
	msgType, err := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName(messageType))
	if err != nil {
		return nil, fmt.Errorf("message type not found: %w", err)
	}

	// Create new instance
	return msgType.New().Interface(), nil
}

// respondMicroWithError sends an error response for micro requests
func (s *Server) respondMicroWithError(req micro.Request, code, message string) {
	response := eventsourcing.NewSimpleErrorResponse(code, message)
	responseData, err := proto.Marshal(response)
	if err != nil {
		fmt.Printf("Failed to marshal error response: %v\n", err)
		return
	}

	if err := req.Respond(responseData); err != nil {
		fmt.Printf("Failed to send error response: %v\n", err)
	}
}

// Close stops the server and closes all services
func (s *Server) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Cancel context
	s.cancel()

	// Stop all services
	for _, svc := range s.services {
		if err := svc.Stop(); err != nil {
			fmt.Printf("Error stopping service: %v\n", err)
		}
	}

	// Close NATS connection
	if s.nc != nil {
		s.nc.Close()
	}

	fmt.Println("NATS services stopped")
	return nil
}

// IsConnected returns true if connected to NATS
func (s *Server) IsConnected() bool {
	return s.nc != nil && s.nc.IsConnected()
}

// natsMicroHeaderCarrier adapts NATS micro.Headers to propagation.TextMapCarrier
type natsMicroHeaderCarrier struct {
	headers micro.Headers
}

func (c *natsMicroHeaderCarrier) Get(key string) string {
	return c.headers.Get(key)
}

func (c *natsMicroHeaderCarrier) Set(key, value string) {
	// micro.Headers is a map[string][]string, so we need to set it directly
	c.headers[key] = []string{value}
}

func (c *natsMicroHeaderCarrier) Keys() []string {
	keys := make([]string, 0, len(c.headers))
	for k := range c.headers {
		keys = append(keys, k)
	}
	return keys
}
