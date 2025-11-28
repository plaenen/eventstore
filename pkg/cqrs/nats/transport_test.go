package nats_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/micro"
	"github.com/plaenen/eventstore/pkg/cqrs"
	cqrsnats "github.com/plaenen/eventstore/pkg/cqrs/nats"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestTransport_Request(t *testing.T) {
	ns := startNATSServer(t)
	url := fmt.Sprintf("nats://%s", ns.Addr().String())

	// Setup server-side responder using raw NATS micro
	nc, err := nats.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	svc, err := micro.AddService(nc, micro.Config{
		Name:    "responder",
		Version: "1.0.0",
	})
	require.NoError(t, err)

	err = svc.AddEndpoint("echo", micro.HandlerFunc(func(req micro.Request) {
		// Unmarshal request
		msg := &wrapperspb.StringValue{}
		_ = proto.Unmarshal(req.Data(), msg)

		// Create response
		resp := &wrapperspb.StringValue{Value: "echo: " + msg.Value}
		respData, _ := proto.Marshal(resp)

		headers := micro.Headers{}
		headers["Response-Type"] = []string{string(resp.ProtoReflect().Descriptor().FullName())}
		req.Respond(respData, micro.WithHeaders(headers))
	}), micro.WithEndpointSubject("test.echo"))
	require.NoError(t, err)

	// Create Transport
	transport, err := cqrsnats.NewTransport(&cqrsnats.TransportConfig{
		TransportConfig: cqrs.DefaultTransportConfig(),
		URL:             url,
		Name:            "test-client",
	})
	require.NoError(t, err)
	defer transport.Close()

	// Send request
	req := &wrapperspb.StringValue{Value: "world"}
	resp, err := transport.Request(context.Background(), "test.echo", req)
	require.NoError(t, err)

	// Verify response
	result, ok := resp.(*wrapperspb.StringValue)
	require.True(t, ok)
	assert.Equal(t, "echo: world", result.Value)
}

func TestTransport_Retry(t *testing.T) {
	ns := startNATSServer(t)
	url := fmt.Sprintf("nats://%s", ns.Addr().String())

	// Setup responder that fails with version conflict once, then succeeds
	nc, err := nats.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	attempts := 0
	svc, err := micro.AddService(nc, micro.Config{
		Name:    "retry-service",
		Version: "1.0.0",
	})
	require.NoError(t, err)

	err = svc.AddEndpoint("retry", micro.HandlerFunc(func(req micro.Request) {
		attempts++
		if attempts == 1 {
			// Fail with version conflict
			headers := micro.Headers{}
			headers["Error"] = []string{"true"}
			// Simulate AppError JSON
			errJSON := `{"code":"CONFLICT","message":"optimistic lock failure: version mismatch"}`
			req.Respond([]byte(errJSON), micro.WithHeaders(headers))
			return
		}

		// Succeed
		resp := &wrapperspb.StringValue{Value: "success"}
		respData, _ := proto.Marshal(resp)
		headers := micro.Headers{}
		headers["Response-Type"] = []string{string(resp.ProtoReflect().Descriptor().FullName())}
		req.Respond(respData, micro.WithHeaders(headers))
	}), micro.WithEndpointSubject("test.retry"))
	require.NoError(t, err)

	// Create Transport with retries
	config := cqrs.DefaultTransportConfig()
	config.MaxRetries = 3
	config.ReconnectWait = 10 * time.Millisecond // Fast retry

	transport, err := cqrsnats.NewTransport(&cqrsnats.TransportConfig{
		TransportConfig: config,
		URL:             url,
		Name:            "retry-client",
	})
	require.NoError(t, err)
	defer transport.Close()

	// Send request
	req := &wrapperspb.StringValue{Value: "retry-me"}
	resp, err := transport.Request(context.Background(), "test.retry", req)
	require.NoError(t, err)

	// Verify success
	result, ok := resp.(*wrapperspb.StringValue)
	require.True(t, ok)
	assert.Equal(t, "success", result.Value)
	assert.Equal(t, 2, attempts)
}
