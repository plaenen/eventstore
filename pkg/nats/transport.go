package nats

import (
	"context"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/plaenen/eventstore/pkg/eventsourcing"
	"github.com/plaenen/eventstore/pkg/observability"
	"go.opentelemetry.io/otel/propagation"
	"google.golang.org/protobuf/proto"
)

// Transport implements eventsourcing.Transport using NATS request/reply
type Transport struct {
	nc        *nats.Conn
	config    *eventsourcing.TransportConfig
	telemetry *observability.Telemetry
}

// TransportConfig extends the base transport config with NATS-specific options
type TransportConfig struct {
	*eventsourcing.TransportConfig

	// URL is the NATS server URL (e.g., "nats://localhost:4222")
	URL string

	// Name is the client name for connection identification
	Name string

	// Credentials for authentication (optional)
	Token string
	User  string
	Pass  string

	// Telemetry for observability (optional)
	Telemetry *observability.Telemetry
}

// NewTransport creates a new NATS transport for client-side request/reply
func NewTransport(config *TransportConfig) (*Transport, error) {
	if config == nil {
		config = &TransportConfig{
			TransportConfig: eventsourcing.DefaultTransportConfig(),
			URL:             "nats://localhost:4222",
			Name:            "eventsourcing-client",
		}
	}

	// Build NATS options
	opts := []nats.Option{
		nats.Name(config.Name),
		nats.MaxReconnects(config.MaxReconnectAttempts),
		nats.ReconnectWait(config.ReconnectWait),
		nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
			if err != nil {
				fmt.Printf("NATS disconnected: %v\n", err)
			}
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			fmt.Printf("NATS reconnected to %s\n", nc.ConnectedUrl())
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

	return &Transport{
		nc:        nc,
		config:    config.TransportConfig,
		telemetry: config.Telemetry,
	}, nil
}

// Request sends a request and waits for a response with automatic retry on version conflicts
func (t *Transport) Request(ctx context.Context, subject string, request proto.Message) (*eventsourcing.Response, error) {
	// Use transport middleware if telemetry is available
	if t.telemetry != nil {
		middleware := observability.NewTransportMiddleware(t.telemetry)
		return middleware.WrapRequest(ctx, subject, request, func(ctx context.Context) (*eventsourcing.Response, error) {
			return t.doRequestWithRetry(ctx, subject, request)
		})
	}
	return t.doRequestWithRetry(ctx, subject, request)
}

// doRequestWithRetry wraps doRequest with retry logic for handling version conflicts
func (t *Transport) doRequestWithRetry(ctx context.Context, subject string, request proto.Message) (*eventsourcing.Response, error) {
	maxRetries := t.config.MaxRetries
	var lastResponse *eventsourcing.Response
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		// Check if context is still valid
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		// Attempt the request
		resp, err := t.doRequest(ctx, subject, request)

		// If transport error, return immediately (don't retry network failures)
		if err != nil {
			return nil, err
		}

		lastResponse = resp

		// If no application error, success!
		if resp.Error == nil {
			return resp, nil
		}

		// Check if this is a retryable error (concurrency conflict)
		if !t.isRetryableError(resp.Error) {
			return resp, nil
		}

		// Don't retry on last attempt
		if attempt == maxRetries {
			break
		}

		// Exponential backoff: 10ms, 20ms, 40ms
		backoff := time.Duration(10*(1<<uint(attempt))) * time.Millisecond
		select {
		case <-time.After(backoff):
			// Continue to next retry
		case <-ctx.Done():
			return nil, ctx.Err()
		}

		lastErr = fmt.Errorf("retrying after %v (attempt %d/%d)", backoff, attempt+1, maxRetries)
	}

	// All retries exhausted
	if lastErr != nil {
		// Could log here if needed: fmt.Printf("All retries exhausted: %v\n", lastErr)
	}
	return lastResponse, nil
}

// isRetryableError determines if an application error should trigger a retry
func (t *Transport) isRetryableError(appErr *eventsourcing.AppError) bool {
	// Retry on concurrency conflicts (optimistic locking failures)
	if appErr.Code == "SAVE_FAILED" {
		// Check if the message indicates a version mismatch
		if len(appErr.Message) > 0 &&
			(containsString(appErr.Message, "concurrency conflict") ||
				containsString(appErr.Message, "version mismatch") ||
				containsString(appErr.Message, "optimistic lock")) {
			return true
		}
	}
	return false
}

// containsString checks if a string contains a substring (case-insensitive helper)
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		len(s) > len(substr) && (s[:len(substr)] == substr ||
			s[len(s)-len(substr):] == substr ||
			findSubstring(s, substr)))
}

// findSubstring performs a simple substring search
func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// doRequest performs the actual NATS request
func (t *Transport) doRequest(ctx context.Context, subject string, request proto.Message) (*eventsourcing.Response, error) {
	// Serialize request
	requestData, err := proto.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create NATS message with metadata
	msg := nats.NewMsg(subject)
	msg.Data = requestData

	// Add metadata from context (tenant, trace IDs, etc.)
	if tenantID, ok := ctx.Value("tenant_id").(string); ok {
		msg.Header.Set("Tenant-ID", tenantID)
	}
	if traceID, ok := ctx.Value("trace_id").(string); ok {
		msg.Header.Set("Trace-ID", traceID)
	}

	// Inject trace context into NATS headers for distributed tracing
	if t.telemetry != nil {
		propagator := propagation.TraceContext{}
		propagator.Inject(ctx, &natsHeaderCarrier{header: msg.Header})
	}

	// Set message type for server-side routing
	msg.Header.Set("Message-Type", string(request.ProtoReflect().Descriptor().FullName()))

	// Determine timeout from context or use default
	timeout := t.config.Timeout
	if deadline, ok := ctx.Deadline(); ok {
		timeout = time.Until(deadline)
	}

	// Send request and wait for response
	respMsg, err := t.nc.RequestMsg(msg, timeout)
	if err != nil {
		if err == nats.ErrTimeout {
			return eventsourcing.NewSimpleErrorResponse("TIMEOUT", "Request timed out"), nil
		}
		return nil, fmt.Errorf("request failed: %w", err)
	}

	// Deserialize response
	response := &eventsourcing.Response{}
	if err := proto.Unmarshal(respMsg.Data, response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return response, nil
}

// natsHeaderCarrier adapts NATS headers to propagation.TextMapCarrier
type natsHeaderCarrier struct {
	header nats.Header
}

func (c *natsHeaderCarrier) Get(key string) string {
	return c.header.Get(key)
}

func (c *natsHeaderCarrier) Set(key, value string) {
	c.header.Set(key, value)
}

func (c *natsHeaderCarrier) Keys() []string {
	keys := make([]string, 0, len(c.header))
	for k := range c.header {
		keys = append(keys, k)
	}
	return keys
}

// Close closes the NATS connection
func (t *Transport) Close() error {
	if t.nc != nil {
		t.nc.Close()
	}
	return nil
}

// IsConnected returns true if connected to NATS
func (t *Transport) IsConnected() bool {
	return t.nc != nil && t.nc.IsConnected()
}

// ConnectedURL returns the URL of the connected NATS server
func (t *Transport) ConnectedURL() string {
	if t.nc != nil {
		return t.nc.ConnectedUrl()
	}
	return ""
}
