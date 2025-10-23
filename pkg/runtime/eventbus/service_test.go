package eventbus

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	natsclient "github.com/nats-io/nats.go"
	natseventbus "github.com/plaenen/eventstore/pkg/messaging/nats"
	"github.com/plaenen/eventstore/pkg/runner"
	"go.opentelemetry.io/otel/trace"
)

func TestService_Lifecycle(t *testing.T) {
	t.Run("successful start and stop with defaults", func(t *testing.T) {
		service := New()
		ctx := context.Background()

		if err := service.Start(ctx); err != nil {
			t.Fatalf("failed to start service: %v", err)
		}

		if service.URL() == "" {
			t.Error("expected non-empty URL after start")
		}

		if service.Server() == nil {
			t.Error("expected non-nil server after start")
		}

		if service.EventBus() == nil {
			t.Error("expected non-nil event bus after start")
		}

		if err := service.Stop(ctx); err != nil {
			t.Fatalf("failed to stop service: %v", err)
		}
	})

	t.Run("with custom config", func(t *testing.T) {
		config := natseventbus.DefaultConfig()
		config.StreamName = "TEST_EVENTS_CUSTOM"
		config.StreamSubjects = []string{"test.custom.>"}

		service := New(WithConfig(config))
		ctx := context.Background()

		if err := service.Start(ctx); err != nil {
			t.Fatalf("failed to start service: %v", err)
		}
		defer service.Stop(ctx)

		if service.EventBus() == nil {
			t.Error("expected non-nil event bus")
		}
	})

	t.Run("with logger", func(t *testing.T) {
		logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}))

		service := New(WithLogger(logger))
		ctx := context.Background()

		if err := service.Start(ctx); err != nil {
			t.Fatalf("failed to start service: %v", err)
		}
		defer service.Stop(ctx)

		if service.EventBus() == nil {
			t.Error("expected non-nil event bus")
		}
	})

	t.Run("with tracer", func(t *testing.T) {
		tracer := trace.NewNoopTracerProvider().Tracer("test")
		service := New(WithTracer(tracer))
		ctx := context.Background()

		if err := service.Start(ctx); err != nil {
			t.Fatalf("failed to start service: %v", err)
		}
		defer service.Stop(ctx)

		if service.EventBus() == nil {
			t.Error("expected non-nil event bus")
		}
	})

	t.Run("name returns eventbus", func(t *testing.T) {
		service := New()
		if service.Name() != "eventbus" {
			t.Errorf("expected name 'eventbus', got %s", service.Name())
		}
	})

	t.Run("stop is safe without start", func(t *testing.T) {
		service := New()
		ctx := context.Background()

		if err := service.Stop(ctx); err != nil {
			t.Errorf("stop should not fail without start: %v", err)
		}
	})

	t.Run("accessors return nil/empty before start", func(t *testing.T) {
		service := New()

		if service.URL() != "" {
			t.Error("expected empty URL before start")
		}

		if service.Server() != nil {
			t.Error("expected nil server before start")
		}

		if service.EventBus() != nil {
			t.Error("expected nil event bus before start")
		}
	})
}

func TestService_HealthCheck(t *testing.T) {
	t.Run("healthy after start", func(t *testing.T) {
		service := New()
		ctx := context.Background()

		if err := service.Start(ctx); err != nil {
			t.Fatalf("failed to start service: %v", err)
		}
		defer service.Stop(ctx)

		if err := service.HealthCheck(ctx); err != nil {
			t.Errorf("expected healthy service, got error: %v", err)
		}
	})

	t.Run("unhealthy before start", func(t *testing.T) {
		service := New()
		ctx := context.Background()

		if err := service.HealthCheck(ctx); err == nil {
			t.Error("expected health check to fail before start")
		}
	})

	t.Run("unhealthy after stop", func(t *testing.T) {
		service := New()
		ctx := context.Background()

		if err := service.Start(ctx); err != nil {
			t.Fatalf("failed to start service: %v", err)
		}

		if err := service.Stop(ctx); err != nil {
			t.Fatalf("failed to stop service: %v", err)
		}

		time.Sleep(100 * time.Millisecond)

		if err := service.HealthCheck(ctx); err == nil {
			t.Error("expected health check to fail after stop")
		}
	})
}

func TestService_Integration(t *testing.T) {
	t.Run("event bus is functional", func(t *testing.T) {
		service := New()
		ctx := context.Background()

		if err := service.Start(ctx); err != nil {
			t.Fatalf("failed to start service: %v", err)
		}
		defer service.Stop(ctx)

		bus := service.EventBus()
		if bus == nil {
			t.Fatal("expected non-nil event bus")
		}

		nc, err := natsclient.Connect(service.URL())
		if err != nil {
			t.Fatalf("failed to connect to NATS: %v", err)
		}
		defer nc.Close()

		if !nc.IsConnected() {
			t.Error("expected connection to be established")
		}
	})
}

func TestService_WithRunner(t *testing.T) {
	t.Run("works with runner", func(t *testing.T) {
		service := New()

		r := runner.New([]runner.Service{service})

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		errCh := make(chan error, 1)
		go func() {
			errCh <- r.Run(ctx)
		}()

		time.Sleep(500 * time.Millisecond)

		if service.EventBus() == nil {
			t.Error("expected service to be started by runner")
		}

		if err := r.HealthCheck(context.Background()); err != nil {
			t.Errorf("health check failed: %v", err)
		}

		cancel()

		select {
		case err := <-errCh:
			if err != nil && err != context.Canceled {
				t.Errorf("runner failed: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Error("runner did not shutdown within timeout")
		}
	})
}

func TestService_InterfaceCompliance(t *testing.T) {
	var _ runner.Service = (*Service)(nil)
	var _ runner.HealthChecker = (*Service)(nil)
}
