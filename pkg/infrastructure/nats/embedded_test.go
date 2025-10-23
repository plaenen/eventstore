package nats

import (
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
)

func TestEmbeddedServer_StartAndShutdown(t *testing.T) {
	t.Run("normal startup and shutdown", func(t *testing.T) {
		srv, err := StartEmbeddedServer()
		if err != nil {
			t.Fatalf("failed to start embedded server: %v", err)
		}

		// Verify server is running
		if srv.URL() == "" {
			t.Error("expected non-empty URL")
		}

		// Test connection
		nc, err := nats.Connect(srv.URL())
		if err != nil {
			t.Fatalf("failed to connect to embedded server: %v", err)
		}

		// Close connection before shutdown
		nc.Close()

		// Shutdown should complete quickly
		done := make(chan struct{})
		go func() {
			srv.Shutdown()
			close(done)
		}()

		select {
		case <-done:
			// Success - shutdown completed
		case <-time.After(10 * time.Second):
			t.Fatal("shutdown timed out after 10 seconds")
		}
	})

	t.Run("shutdown with active connections", func(t *testing.T) {
		srv, err := StartEmbeddedServer()
		if err != nil {
			t.Fatalf("failed to start embedded server: %v", err)
		}

		// Create multiple connections
		var conns []*nats.Conn
		for i := 0; i < 5; i++ {
			nc, err := nats.Connect(srv.URL())
			if err != nil {
				t.Fatalf("failed to connect: %v", err)
			}
			conns = append(conns, nc)
		}

		// Shutdown even with active connections
		done := make(chan struct{})
		go func() {
			srv.Shutdown()
			close(done)
		}()

		// Should complete within timeout (5 seconds + buffer)
		select {
		case <-done:
			// Success - shutdown completed despite active connections
		case <-time.After(10 * time.Second):
			t.Fatal("shutdown timed out with active connections")
		}

		// Clean up connections
		for _, nc := range conns {
			nc.Close()
		}
	})

	t.Run("multiple shutdown calls are safe", func(t *testing.T) {
		srv, err := StartEmbeddedServer()
		if err != nil {
			t.Fatalf("failed to start embedded server: %v", err)
		}

		// First shutdown
		srv.Shutdown()

		// Second shutdown should not panic or hang
		done := make(chan struct{})
		go func() {
			srv.Shutdown()
			close(done)
		}()

		select {
		case <-done:
			// Success - multiple shutdowns are safe
		case <-time.After(2 * time.Second):
			t.Fatal("second shutdown call hung")
		}
	})

	t.Run("shutdown with nil server is safe", func(t *testing.T) {
		srv := &EmbeddedServer{
			server: nil,
			url:    "",
		}

		// Should not panic
		done := make(chan struct{})
		go func() {
			srv.Shutdown()
			close(done)
		}()

		select {
		case <-done:
			// Success
		case <-time.After(1 * time.Second):
			t.Fatal("shutdown with nil server hung")
		}
	})
}


func TestConcurrentShutdowns(t *testing.T) {
	t.Run("concurrent shutdown calls don't panic", func(t *testing.T) {
		srv, err := StartEmbeddedServer()
		if err != nil {
			t.Fatalf("failed to start embedded server: %v", err)
		}

		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				srv.Shutdown()
			}()
		}

		// Wait for all shutdowns with timeout
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			// Success - all concurrent shutdowns completed
		case <-time.After(15 * time.Second):
			t.Fatal("concurrent shutdowns timed out")
		}
	})
}

func TestEmbeddedServer_URL(t *testing.T) {
	srv, err := StartEmbeddedServer()
	if err != nil {
		t.Fatalf("failed to start embedded server: %v", err)
	}
	defer srv.Shutdown()

	url := srv.URL()
	if url == "" {
		t.Error("expected non-empty URL")
	}

	// URL should be a valid NATS URL
	if url[:7] != "nats://" {
		t.Errorf("expected URL to start with 'nats://', got: %s", url)
	}

	// Should be able to connect using the URL
	nc, err := nats.Connect(url)
	if err != nil {
		t.Errorf("failed to connect using URL %s: %v", url, err)
	}
	nc.Close()
}

func TestConnectToEmbedded(t *testing.T) {
	srv, err := StartEmbeddedServer()
	if err != nil {
		t.Fatalf("failed to start embedded server: %v", err)
	}
	defer srv.Shutdown()

	nc, err := ConnectToEmbedded(srv)
	if err != nil {
		t.Fatalf("failed to connect to embedded server: %v", err)
	}
	defer nc.Close()

	if !nc.IsConnected() {
		t.Error("expected connection to be established")
	}
}

// Benchmark shutdown performance
func BenchmarkEmbeddedServer_Shutdown(b *testing.B) {
	for i := 0; i < b.N; i++ {
		srv, err := StartEmbeddedServer()
		if err != nil {
			b.Fatalf("failed to start server: %v", err)
		}

		// Create a connection
		nc, _ := nats.Connect(srv.URL())
		nc.Close()

		// Measure shutdown time
		srv.Shutdown()
	}
}

