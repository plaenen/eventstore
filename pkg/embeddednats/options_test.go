package embeddednats

import (
	"net/http"
	"testing"
	"time"
)

func TestEmbeddedServer_Options(t *testing.T) {
	t.Run("WithMonitoring enables HTTP port", func(t *testing.T) {
		// Use a random port for monitoring to avoid conflicts
		monitorPort := 8222

		srv, err := StartEmbeddedServer(
			WithMonitoring(monitorPort),
			WithPort(-1), // Random NATS port
		)
		if err != nil {
			t.Fatalf("failed to start server: %v", err)
		}
		defer srv.Shutdown()

		// Wait a bit for server to start
		time.Sleep(100 * time.Millisecond)

		// Check if monitoring endpoint is reachable
		resp, err := http.Get("http://127.0.0.1:8222/varz")
		if err != nil {
			t.Fatalf("failed to query monitoring endpoint: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}
	})

	t.Run("Health check returns nil when healthy", func(t *testing.T) {
		srv, err := StartEmbeddedServer()
		if err != nil {
			t.Fatalf("failed to start server: %v", err)
		}
		defer srv.Shutdown()

		if err := srv.Health(); err != nil {
			t.Errorf("expected healthy server, got error: %v", err)
		}
	})

	t.Run("Health check returns error when shutdown", func(t *testing.T) {
		srv, err := StartEmbeddedServer()
		if err != nil {
			t.Fatalf("failed to start server: %v", err)
		}
		srv.Shutdown()

		// Give it a moment to fully shutdown
		time.Sleep(100 * time.Millisecond)

		if err := srv.Health(); err == nil {
			t.Error("expected error from Health() after shutdown, got nil")
		}
	})
}
