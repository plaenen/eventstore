package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/plaenen/eventstore/pkg/cqrs"
	"github.com/plaenen/eventstore/pkg/multitenancy"
	"github.com/plaenen/eventstore/pkg/observability"
	"github.com/plaenen/eventstore/pkg/protocol"
	"github.com/plaenen/eventstore/pkg/security/credentials"
	"github.com/plaenen/eventstore/pkg/security/tls"
	"go.opentelemetry.io/otel/propagation"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

// Transport implements cqrs.Transport using NATS request/reply
type Transport struct {
	nc        *nats.Conn
	config    *cqrs.TransportConfig
	telemetry *observability.Telemetry
}

// TransportConfig extends the base transport config with NATS-specific options
type TransportConfig struct {
	*cqrs.TransportConfig

	// URL is the NATS server URL (e.g., "nats://localhost:4222")
	URL string

	// Name is the client name for connection identification
	Name string

	// CredentialProvider provides secure credential management
	// Use this instead of Token/User/Pass for production deployments
	CredentialProvider credentials.Provider

	// TLSConfig provides TLS/mTLS configuration for secure connections
	// When enabled, connections will use TLS encryption
	// Set ClientAuth to true for mutual TLS (mTLS)
	TLSConfig *tls.Config

	// Telemetry for observability (optional)
	Telemetry *observability.Telemetry
}

// NewTransport creates a new NATS transport for client-side request/reply
func NewTransport(config *TransportConfig) (*Transport, error) {
	if config == nil {
		config = &TransportConfig{
			TransportConfig: cqrs.DefaultTransportConfig(),
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

	// Add authentication - prefer CredentialProvider over deprecated fields
	if config.CredentialProvider != nil {
		// Use secure credential provider
		ctx := context.Background()
		creds, err := config.CredentialProvider.GetCredentials(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get credentials: %w", err)
		}

		// Apply credentials based on type
		switch creds.Type {
		case credentials.CredentialTypeToken:
			if creds.Token == "" {
				return nil, fmt.Errorf("token credential is empty")
			}
			opts = append(opts, nats.Token(creds.Token))

		case credentials.CredentialTypeUserPassword:
			if creds.User == "" || creds.Password == "" {
				return nil, fmt.Errorf("user/password credentials are incomplete")
			}
			opts = append(opts, nats.UserInfo(creds.User, creds.Password))

		case credentials.CredentialTypeNKey:
			if creds.Seed == "" {
				return nil, fmt.Errorf("nkey seed is empty")
			}
			// Parse NKey seed
			kp, err := nats.NkeyOptionFromSeed(creds.Seed)
			if err != nil {
				return nil, fmt.Errorf("invalid nkey seed: %w", err)
			}
			opts = append(opts, kp)

		case credentials.CredentialTypeJWT:
			// JWT authentication would typically use UserJWT option
			// This requires a JWT and a signature callback
			return nil, fmt.Errorf("JWT authentication not yet implemented")

		case credentials.CredentialTypeMTLS:
			// mTLS would be configured via TLS config, not here
			return nil, fmt.Errorf("mTLS should be configured via TLS config, not credential provider")

		default:
			return nil, fmt.Errorf("unsupported credential type: %s", creds.Type)
		}
	}
	// NOTE: Deprecated Token/User/Pass fields have been removed.
	// Use CredentialProvider for authentication.

	// Add TLS configuration if provided
	if config.TLSConfig != nil && config.TLSConfig.Enabled {
		tlsConfig, err := config.TLSConfig.BuildClientTLSConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to build TLS config: %w", err)
		}
		opts = append(opts, nats.Secure(tlsConfig))
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

// Request sends a request and waits for a response with automatic retry on version conflicts.
// Returns (message, nil) on success or (nil, error) on failure.
// This is Go-idiomatic error handling.
func (t *Transport) Request(ctx context.Context, subject string, request proto.Message) (proto.Message, error) {
	// TODO: Update observability middleware to work with new signature
	// For now, bypass middleware
	return t.doRequestWithRetry(ctx, subject, request)
}

// doRequestWithRetry wraps doRequest with retry logic for handling version conflicts
func (t *Transport) doRequestWithRetry(ctx context.Context, subject string, request proto.Message) (proto.Message, error) {
	maxRetries := t.config.MaxRetries
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		// Check if context is still valid
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		// Attempt the request
		resp, err := t.doRequest(ctx, subject, request)

		// If error, check if it's retryable
		if err != nil {
			if t.isRetryableError(err) {
				lastErr = err
				// Don't retry on last attempt
				if attempt == maxRetries {
					break
				}

				// Exponential backoff: 10ms, 20ms, 40ms
				backoff := time.Duration(10*(1<<uint(attempt))) * time.Millisecond
				select {
				case <-time.After(backoff):
					// Continue to next retry
					continue
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
			// Not retryable or transport error
			return nil, err
		}

		// Success! Return the response
		if resp != nil {
			return resp, nil
		}

		// Response is nil but no error? Should not happen
		return nil, protocol.ErrInternal("received nil response with no error")
	}

	// All retries exhausted
	if lastErr != nil {
		return nil, protocol.FormatError(lastErr, fmt.Sprintf("all retries exhausted after %d attempts", maxRetries+1))
	}
	return nil, protocol.ErrInternal("request failed with no error details")
}

// isRetryableError determines if an application error should trigger a retry
func (t *Transport) isRetryableError(err error) bool {
	// Check if it's an AppError
	appErr, ok := err.(*protocol.AppError)
	if !ok {
		return false
	}

	// Retry on concurrency conflicts (optimistic locking failures)
	if appErr.Code == "SAVE_FAILED" || appErr.Code == protocol.ErrCodeConflict {
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

// doRequest performs the actual NATS request.
// Returns (message, nil) on success or (nil, error) on failure.
func (t *Transport) doRequest(ctx context.Context, subject string, request proto.Message) (proto.Message, error) {
	// Serialize request
	requestData, err := proto.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create NATS message with metadata
	msg := nats.NewMsg(subject)
	msg.Data = requestData

	// Add metadata from context (tenant, trace IDs, etc.)
	if tenantID, err := multitenancy.GetTenantID(ctx); err == nil {
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

	// Set request message type for server-side routing
	msg.Header.Set("Request-Type", string(request.ProtoReflect().Descriptor().FullName()))

	// Determine timeout from context or use default
	timeout := t.config.Timeout
	if deadline, ok := ctx.Deadline(); ok {
		timeout = time.Until(deadline)
	}

	// Send request and wait for response
	respMsg, err := t.nc.RequestMsg(msg, timeout)
	if err != nil {
		if err == nats.ErrTimeout {
			return nil, protocol.ErrTimeout("Request timed out")
		}
		return nil, fmt.Errorf("request failed: %w", err)
	}

	// Check if response is an error (indicated by Error header)
	if errHeader := respMsg.Header.Get("Error"); errHeader == "true" {
		// Deserialize error from response body (JSON encoded AppError)
		var appErr protocol.AppError
		if err := json.Unmarshal(respMsg.Data, &appErr); err != nil {
			return nil, fmt.Errorf("failed to unmarshal error response: %w", err)
		}
		return nil, &appErr
	}

	// Get expected response type from header
	responseTypeName := respMsg.Header.Get("Response-Type")
	if responseTypeName == "" {
		return nil, protocol.ErrInternal("response missing Response-Type header")
	}

	// Look up the message type in the proto registry
	msgType, err := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName(responseTypeName))
	if err != nil {
		return nil, fmt.Errorf("unknown response type %s: %w", responseTypeName, err)
	}

	// Create a new instance of the response message type
	responseMsg := msgType.New().Interface()

	// Deserialize response
	if err := proto.Unmarshal(respMsg.Data, responseMsg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return responseMsg, nil
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
