package nats_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	natsserver "github.com/plaenen/eventstore/pkg/embeddednats"
)

// TestNATSDeduplicationRaw tests NATS JetStream deduplication directly
// without going through our EventBus abstraction
func TestNATSDeduplicationRaw(t *testing.T) {
	// Start embedded NATS server
	srv, err := natsserver.StartEmbeddedServer()
	if err != nil {
		t.Fatalf("failed to start embedded server: %v", err)
	}
	defer srv.Shutdown()

	// Connect to NATS
	nc, err := nats.Connect(srv.URL())
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer nc.Close()

	// Create JetStream context
	js, err := nc.JetStream()
	if err != nil {
		t.Fatalf("failed to create JetStream context: %v", err)
	}

	// Create stream with deduplication
	streamName := "TEST_DEDUP"
	streamConfig := &nats.StreamConfig{
		Name:       streamName,
		Subjects:   []string{"test.dedup.>"},
		Storage:    nats.FileStorage,
		Duplicates: 2 * time.Minute,
	}

	_, err = js.AddStream(streamConfig)
	if err != nil {
		t.Fatalf("failed to create stream: %v", err)
	}

	// Verify stream has Duplicates set
	info, err := js.StreamInfo(streamName)
	if err != nil {
		t.Fatalf("failed to get stream info: %v", err)
	}
	t.Logf("Stream Duplicates window: %v", info.Config.Duplicates)
	if info.Config.Duplicates != 2*time.Minute {
		t.Fatalf("Duplicates not set correctly: got %v, want %v", info.Config.Duplicates, 2*time.Minute)
	}

	// Publish message with ID twice
	msgID := fmt.Sprintf("test-msg-%d", time.Now().UnixNano())
	subject := "test.dedup.event"
	data := []byte("test data")

	// First publish
	ack1, err := js.Publish(subject, data, nats.MsgId(msgID))
	if err != nil {
		t.Fatalf("first publish failed: %v", err)
	}
	t.Logf("First publish ACK - Stream: %s, Sequence: %d, Duplicate: %v",
		ack1.Stream, ack1.Sequence, ack1.Duplicate)

	time.Sleep(50 * time.Millisecond)

	// Second publish with same ID
	ack2, err := js.Publish(subject, data, nats.MsgId(msgID))
	if err != nil {
		t.Fatalf("second publish failed: %v", err)
	}
	t.Logf("Second publish ACK - Stream: %s, Sequence: %d, Duplicate: %v",
		ack2.Stream, ack2.Sequence, ack2.Duplicate)

	// Check if second was marked as duplicate
	if !ack2.Duplicate {
		t.Error("Second publish was NOT marked as duplicate by NATS!")
	}

	// Check stream message count
	info, err = js.StreamInfo(streamName)
	if err != nil {
		t.Fatalf("failed to get stream info: %v", err)
	}
	t.Logf("Stream message count: %d", info.State.Msgs)

	// Note: In a test environment with stream reuse, there might be messages from previous runs
	// The important thing is that the subscriber only receives 1 message with our ID

	// Create consumer and verify only one message
	consumerConfig := &nats.ConsumerConfig{
		Durable:        "test-consumer",
		AckPolicy:      nats.AckExplicitPolicy,
		DeliverSubject: fmt.Sprintf("_INBOX.%s.test", streamName),
	}

	_, err = js.AddConsumer(streamName, consumerConfig)
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}

	received := 0
	sub, err := js.Subscribe("test.dedup.>", func(msg *nats.Msg) {
		received++
		t.Logf("Received message %d", received)
		msg.Ack()
	}, nats.Bind(streamName, "test-consumer"))
	if err != nil {
		t.Fatalf("failed to subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	time.Sleep(500 * time.Millisecond)

	if received != 1 {
		t.Errorf("Expected to receive 1 message, got %d", received)
	} else {
		t.Log("✓ Deduplication working correctly!")
	}
}
