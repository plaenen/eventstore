package nats_test

import (
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	natsserver "github.com/plaenen/eventstore/pkg/embeddednats"
	"github.com/plaenen/eventstore/pkg/eventsourcing"
	natspkg "github.com/plaenen/eventstore/pkg/messaging/nats"
)

func TestMultiTenantAuthIsolation(t *testing.T) {
	// Define users with restricted permissions
	users := []*server.User{
		{
			Username: "tenant-a",
			Password: "password-a",
			Permissions: &server.Permissions{
				Publish: &server.SubjectPermission{
					Allow: []string{
						"events.tenant-a.>",
						"$JS.API.CONSUMER.*.EVENTS_tenant-a.>", // Allow consumer ops ONLY on own stream
						"_INBOX.>",                             // Allow replies
						"$JS.ACK.EVENTS_tenant-a.>",            // Allow ACKs for own stream
						"$JS.API.STREAM.INFO.EVENTS_tenant-a",  // Allow stream info for own stream
					},
				},
				Subscribe: &server.SubjectPermission{
					Allow: []string{
						"events.tenant-a.>",
						"_INBOX.>",
					},
				},
			},
		},
		{
			Username: "tenant-b",
			Password: "password-b",
			Permissions: &server.Permissions{
				Publish: &server.SubjectPermission{
					Allow: []string{
						"events.tenant-b.>",
						"$JS.API.CONSUMER.*.EVENTS_tenant-b.>",
						"_INBOX.>",
						"$JS.ACK.EVENTS_tenant-b.>",
						"$JS.API.STREAM.INFO.EVENTS_tenant-b",
					},
				},
				Subscribe: &server.SubjectPermission{
					Allow: []string{
						"events.tenant-b.>",
						"_INBOX.>",
					},
				},
			},
		},
		{
			Username: "admin",
			Password: "password-admin",
			Permissions: &server.Permissions{
				Publish: &server.SubjectPermission{
					Allow: []string{">"},
				},
				Subscribe: &server.SubjectPermission{
					Allow: []string{">"},
				},
			},
		},
	}

	// Start embedded NATS server with auth and explicit store dir
	srv, err := natsserver.StartEmbeddedServer(
		natsserver.WithUsers(users),
		natsserver.WithStoreDir(t.TempDir()),
	)
	if err != nil {
		t.Fatalf("failed to start embedded server: %v", err)
	}
	defer srv.Shutdown()

	// Helper to create bus for a tenant
	createBus := func(user, password, streamName, tenantID string, skipStream bool) *natspkg.EventBus {
		config := natspkg.DefaultConfig()
		config.URL = srv.URL()
		config.User = user
		config.Password = password
		config.StreamName = streamName
		config.StreamSubjects = []string{"events." + tenantID + ".>"}
		config.SkipStreamCreation = skipStream

		// Debug: List streams before creating
		if !skipStream {
			nc, _ := nats.Connect(srv.URL(), nats.UserInfo("admin", "password-admin"))
			js, _ := nc.JetStream()
			names := js.StreamNames()
			for name := range names {
				info, _ := js.StreamInfo(name)
				t.Logf("Existing stream: %s, Subjects: %v", name, info.Config.Subjects)
			}
			nc.Close()
		}

		bus, err := natspkg.NewEventBus(config)
		if err != nil {
			t.Fatalf("failed to create event bus for %s: %v", user, err)
		}
		return bus
	}

	// 1. Admin creates the streams first
	adminBusA := createBus("admin", "password-admin", "EVENTS_tenant-a", "tenant-a", false)
	defer adminBusA.Close()

	adminBusB := createBus("admin", "password-admin", "EVENTS_tenant-b", "tenant-b", false)
	defer adminBusB.Close()

	// 2. Create Tenant A and Tenant B buses (skip stream creation)
	busA := createBus("tenant-a", "password-a", "EVENTS_tenant-a", "tenant-a", true)
	defer busA.Close()

	busB := createBus("tenant-b", "password-b", "EVENTS_tenant-b", "tenant-b", true)
	defer busB.Close()

	// 3. Verify Tenant A can publish to Tenant A
	eventA := &eventsourcing.Event{
		ID:            "event-a-1",
		AggregateID:   "agg-a",
		AggregateType: "TestAgg",
		EventType:     "test.Event",
		Version:       1,
		Timestamp:     time.Now(),
		Data:          []byte("data-a"),
		Metadata: eventsourcing.EventMetadata{
			TenantID: "tenant-a",
		},
	}

	if err := busA.Publish([]*eventsourcing.Event{eventA}); err != nil {
		t.Errorf("Tenant A failed to publish to own tenant: %v", err)
	}

	// 4. Verify Tenant A CANNOT publish to Tenant B (Subject permission check)
	eventAtoB := &eventsourcing.Event{
		ID:            "event-a-to-b",
		AggregateID:   "agg-b",
		AggregateType: "TestAgg",
		EventType:     "test.Event",
		Version:       1,
		Timestamp:     time.Now(),
		Data:          []byte("data-a-to-b"),
		Metadata: eventsourcing.EventMetadata{
			TenantID: "tenant-b",
		},
	}

	if err := busA.Publish([]*eventsourcing.Event{eventAtoB}); err == nil {
		t.Error("Tenant A should NOT be able to publish to Tenant B")
	}

	// 5. Verify Tenant A can subscribe to Tenant A
	subA, err := busA.Subscribe(eventsourcing.EventFilter{
		TenantIDs: []string{"tenant-a"},
	}, func(e *eventsourcing.EventEnvelope) error {
		return nil
	})
	if err != nil {
		t.Errorf("Tenant A failed to subscribe to own tenant: %v", err)
	}
	defer subA.Unsubscribe()

	// 6. Verify Tenant A CANNOT receive Tenant B events
	// Even if Subscribe succeeds (creating a consumer with filter 'events.tenant-b.>'),
	// the stream 'EVENTS_tenant-a' does not contain these events.
	// So we verify that NO events are received.

	received := make(chan *eventsourcing.EventEnvelope, 1)
	subB, err := busA.Subscribe(eventsourcing.EventFilter{
		TenantIDs: []string{"tenant-b"},
	}, func(e *eventsourcing.EventEnvelope) error {
		received <- e
		return nil
	})
	if err != nil {
		// If it errors, that's also fine (and better), but if it succeeds, we check delivery.
		t.Logf("Subscribe to tenant-b returned error (good): %v", err)
	} else {
		defer subB.Unsubscribe()

		// Publish event to Tenant B (using Tenant B's bus)
		eventB := &eventsourcing.Event{
			ID:            "event-b-1",
			AggregateID:   "agg-b",
			AggregateType: "TestAgg",
			EventType:     "test.Event",
			Version:       1,
			Timestamp:     time.Now(),
			Data:          []byte("data-b"),
			Metadata: eventsourcing.EventMetadata{
				TenantID: "tenant-b",
			},
		}
		if err := busB.Publish([]*eventsourcing.Event{eventB}); err != nil {
			t.Fatalf("Tenant B failed to publish: %v", err)
		}

		// Check if Tenant A received it
		select {
		case <-received:
			t.Error("Tenant A received Tenant B's event! Isolation failed.")
		case <-time.After(500 * time.Millisecond):
			t.Log("Correct: Tenant A did not receive Tenant B's event")
		}
	}

	// 7. Verify Tenant A cannot access Tenant B's stream directly
	// Try to create a bus for Tenant A but pointing to Tenant B's stream
	configAtoB := natspkg.DefaultConfig()
	configAtoB.URL = srv.URL()
	configAtoB.User = "tenant-a"
	configAtoB.Password = "password-a"
	configAtoB.StreamName = "EVENTS_tenant-b" // Accessing B's stream
	configAtoB.SkipStreamCreation = true      // We assume it exists

	busAtoB, err := natspkg.NewEventBus(configAtoB)
	if err != nil {
		t.Logf("Correct: Tenant A failed to connect to Tenant B's stream: %v", err)
	} else {
		// If connection succeeds, try to subscribe.
		// NewEventBus doesn't check stream access if SkipStreamCreation is true.
		// So we must try to use it.
		_, err := busAtoB.Subscribe(eventsourcing.EventFilter{
			TenantIDs: []string{"tenant-b"},
		}, func(e *eventsourcing.EventEnvelope) error { return nil })

		if err == nil {
			t.Error("Tenant A was able to subscribe to Tenant B's stream!")
			busAtoB.Close()
		} else {
			t.Logf("Correct: Tenant A failed to subscribe to Tenant B's stream: %v", err)
		}
	}
}
