package embeddednats

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/plaenen/eventstore/pkg/runner"
	"go.opentelemetry.io/otel/trace"
)

func TestService_Lifecycle(t *testing.T) {
	t.Run("successful start and stop", func(t *testing.T) {
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

		if err := service.Stop(ctx); err != nil {
			t.Fatalf("failed to stop service: %v", err)
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

		if service.URL() == "" {
			t.Error("expected non-empty URL")
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

		if service.URL() == "" {
			t.Error("expected non-empty URL")
		}
	})

	t.Run("name returns embedded-nats", func(t *testing.T) {
		service := New()
		if service.Name() != "embedded-nats" {
			t.Errorf("expected name 'embedded-nats', got %s", service.Name())
		}
	})

	t.Run("stop is safe without start", func(t *testing.T) {
		service := New()
		ctx := context.Background()

		if err := service.Stop(ctx); err != nil {
			t.Errorf("stop should not fail without start: %v", err)
		}
	})

	t.Run("url returns empty before start", func(t *testing.T) {
		service := New()
		if service.URL() != "" {
			t.Error("expected empty URL before start")
		}
	})

	t.Run("server returns nil before start", func(t *testing.T) {
		service := New()
		if service.Server() != nil {
			t.Error("expected nil server before start")
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

func TestService_Connection(t *testing.T) {
	t.Run("can connect to started service", func(t *testing.T) {
		service := New()
		ctx := context.Background()

		if err := service.Start(ctx); err != nil {
			t.Fatalf("failed to start service: %v", err)
		}
		defer service.Stop(ctx)

		nc, err := nats.Connect(service.URL())
		if err != nil {
			t.Fatalf("failed to connect to service: %v", err)
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

		if service.URL() == "" {
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
