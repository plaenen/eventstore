package nats

import (
	"context"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/plaenen/eventsourcing/pkg/eventsourcing"
	"google.golang.org/protobuf/proto"
)

// Transport implements eventsourcing.Transport using NATS request/reply
type Transport struct {
	nc     *nats.Conn
	config *eventsourcing.TransportConfig
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
		nc:     nc,
		config: config.TransportConfig,
	}, nil
}

// Request sends a request and waits for a response
func (t *Transport) Request(ctx context.Context, subject string, request proto.Message) (*eventsourcing.Response, error) {
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
