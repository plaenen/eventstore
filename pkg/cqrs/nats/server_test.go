package nats_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/plaenen/eventstore/pkg/cqrs"
	cqrsnats "github.com/plaenen/eventstore/pkg/cqrs/nats"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func startNATSServer(t *testing.T) *server.Server {
	opts := &server.Options{
		Host: "127.0.0.1",
		Port: -1, // Random port
	}
	ns, err := server.NewServer(opts)
	require.NoError(t, err)

	go ns.Start()

	if !ns.ReadyForConnections(10 * time.Second) {
		t.Fatal("NATS server failed to start")
	}

	t.Cleanup(func() {
		ns.Shutdown()
	})

	return ns
}

func TestServer_StartAndHandle(t *testing.T) {
	ns := startNATSServer(t)
	url := fmt.Sprintf("nats://%s", ns.Addr().String())

	// Create server
	svcConfig := &cqrsnats.ServerConfig{
		ServerConfig: cqrs.DefaultServerConfig(),
		URL:          url,
		Name:         "test-service",
	}

	srv, err := cqrsnats.NewServer(svcConfig)
	require.NoError(t, err)
	defer srv.Close()

	// Register handler
	handlerCalled := false
	err = srv.RegisterHandler("test.command", func(ctx context.Context, req proto.Message) (proto.Message, error) {
		handlerCalled = true
		input := req.(*wrapperspb.StringValue)
		return &wrapperspb.StringValue{Value: "echo: " + input.Value}, nil
	})
	require.NoError(t, err)

	// Start server
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = srv.Start(ctx)
	require.NoError(t, err)

	// Connect client to send request
	nc, err := nats.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	// Create request
	req := &wrapperspb.StringValue{Value: "hello"}
	reqData, err := proto.Marshal(req)
	require.NoError(t, err)

	msg := nats.NewMsg("test.command")
	msg.Data = reqData
	msg.Header.Set("Request-Type", string(req.ProtoReflect().Descriptor().FullName()))

	// Send request
	respMsg, err := nc.RequestMsg(msg, 2*time.Second)
	require.NoError(t, err)

	// Verify response
	assert.True(t, handlerCalled)

	// Check headers
	respType := respMsg.Header.Get("Response-Type")
	assert.Equal(t, "google.protobuf.StringValue", respType)

	// Check body
	resp := &wrapperspb.StringValue{}
	err = proto.Unmarshal(respMsg.Data, resp)
	require.NoError(t, err)
	assert.Equal(t, "echo: hello", resp.Value)
}

func TestServer_ErrorHandling(t *testing.T) {
	ns := startNATSServer(t)
	url := fmt.Sprintf("nats://%s", ns.Addr().String())

	srv, err := cqrsnats.NewServer(&cqrsnats.ServerConfig{
		ServerConfig: cqrs.DefaultServerConfig(),
		URL:          url,
		Name:         "error-service",
	})
	require.NoError(t, err)
	defer srv.Close()

	// Register handler that returns error
	err = srv.RegisterHandler("test.error", func(ctx context.Context, req proto.Message) (proto.Message, error) {
		return nil, fmt.Errorf("something went wrong")
	})
	require.NoError(t, err)

	err = srv.Start(context.Background())
	require.NoError(t, err)

	// Connect client
	nc, err := nats.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	// Send request
	req := &emptypb.Empty{}
	reqData, _ := proto.Marshal(req)

	msg := nats.NewMsg("test.error")
	msg.Data = reqData
	msg.Header.Set("Request-Type", string(req.ProtoReflect().Descriptor().FullName()))

	respMsg, err := nc.RequestMsg(msg, 2*time.Second)
	require.NoError(t, err)

	// Verify error response
	assert.Equal(t, "true", respMsg.Header.Get("Error"))
	assert.Contains(t, string(respMsg.Data), "something went wrong")
}
