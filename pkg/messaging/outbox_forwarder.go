package messaging

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/plaenen/eventstore/pkg/domain"
	"github.com/plaenen/eventstore/pkg/store"
)

// OutboxForwarder polls the event store for unpublished events and forwards them to the event bus.
// This implements the transactional outbox pattern for reliable event publishing.
type OutboxForwarder struct {
	store      store.EventStore
	bus        EventBus
	pollRate   time.Duration
	batchSize  int
	maxRetries int
	logger     Logger
	stopCh     chan struct{}
	doneCh     chan struct{}
}

// Logger defines the logging interface used by OutboxForwarder.
type Logger interface {
	Info(msg string, args ...interface{})
	Error(msg string, args ...interface{})
	Warn(msg string, args ...interface{})
}

// defaultLogger provides a simple log-based implementation.
type defaultLogger struct{}

func (l *defaultLogger) Info(msg string, args ...interface{}) {
	log.Printf("[INFO] "+msg, args...)
}

func (l *defaultLogger) Error(msg string, args ...interface{}) {
	log.Printf("[ERROR] "+msg, args...)
}

func (l *defaultLogger) Warn(msg string, args ...interface{}) {
	log.Printf("[WARN] "+msg, args...)
}

// OutboxForwarderConfig configures the outbox forwarder behavior.
type OutboxForwarderConfig struct {
	// PollRate is how often to poll for unpublished events (default: 1 second)
	PollRate time.Duration

	// BatchSize is the maximum number of events to process per poll (default: 100)
	BatchSize int

	// MaxRetries is the maximum number of publish attempts before giving up (default: 10)
	MaxRetries int

	// Logger is the logger to use (default: logs to stdout)
	Logger Logger
}

// DefaultOutboxForwarderConfig returns the default configuration.
func DefaultOutboxForwarderConfig() OutboxForwarderConfig {
	return OutboxForwarderConfig{
		PollRate:   1 * time.Second,
		BatchSize:  100,
		MaxRetries: 10,
		Logger:     &defaultLogger{},
	}
}

// NewOutboxForwarder creates a new outbox forwarder with the given configuration.
func NewOutboxForwarder(store store.EventStore, bus EventBus, cfg OutboxForwarderConfig) *OutboxForwarder {
	if cfg.PollRate == 0 {
		cfg.PollRate = 1 * time.Second
	}
	if cfg.BatchSize == 0 {
		cfg.BatchSize = 100
	}
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = 10
	}
	if cfg.Logger == nil {
		cfg.Logger = &defaultLogger{}
	}

	return &OutboxForwarder{
		store:      store,
		bus:        bus,
		pollRate:   cfg.PollRate,
		batchSize:  cfg.BatchSize,
		maxRetries: cfg.MaxRetries,
		logger:     cfg.Logger,
		stopCh:     make(chan struct{}),
		doneCh:     make(chan struct{}),
	}
}

// Start begins polling for unpublished events in the background.
// This is non-blocking and returns immediately.
func (f *OutboxForwarder) Start(ctx context.Context) {
	go f.run(ctx)
}

// Stop gracefully stops the forwarder and waits for the current batch to complete.
func (f *OutboxForwarder) Stop() {
	close(f.stopCh)
	<-f.doneCh
}

// run is the main event loop that polls and publishes events.
func (f *OutboxForwarder) run(ctx context.Context) {
	defer close(f.doneCh)

	ticker := time.NewTicker(f.pollRate)
	defer ticker.Stop()

	f.logger.Info("OutboxForwarder started (pollRate=%v, batchSize=%d, maxRetries=%d)",
		f.pollRate, f.batchSize, f.maxRetries)

	for {
		select {
		case <-ctx.Done():
			f.logger.Info("OutboxForwarder stopped due to context cancellation")
			return
		case <-f.stopCh:
			f.logger.Info("OutboxForwarder stopped")
			return
		case <-ticker.C:
			if err := f.processBatch(ctx); err != nil {
				f.logger.Error("Failed to process batch: %v", err)
			}
		}
	}
}

// processBatch loads and publishes a batch of unpublished events.
func (f *OutboxForwarder) processBatch(ctx context.Context) error {
	// Load unpublished events
	envelopes, err := f.store.LoadUnpublishedEvents(f.batchSize)
	if err != nil {
		return fmt.Errorf("failed to load unpublished events: %w", err)
	}

	if len(envelopes) == 0 {
		return nil // No events to process
	}

	f.logger.Info("Processing batch of %d unpublished events", len(envelopes))

	// Extract events from envelopes (EventBus.Publish expects []*domain.Event)
	events := make([]*domain.Event, len(envelopes))
	eventIDs := make([]string, len(envelopes))
	for i, envelope := range envelopes {
		// Make a copy of the event to avoid pointer issues
		event := envelope.Event
		events[i] = &event
		eventIDs[i] = event.ID
	}

	// Publish to event bus
	if err := f.bus.Publish(events); err != nil {
		// Record failure for all events in the batch
		f.logger.Error("Failed to publish batch: %v", err)
		for _, eventID := range eventIDs {
			if recErr := f.store.RecordPublishFailure(eventID, err); recErr != nil {
				f.logger.Error("Failed to record publish failure for event %s: %v", eventID, recErr)
			}
		}
		return fmt.Errorf("failed to publish events: %w", err)
	}

	// Mark events as published
	if err := f.store.MarkEventsPublished(eventIDs); err != nil {
		f.logger.Error("Failed to mark events as published: %v", err)
		return fmt.Errorf("failed to mark events as published: %w", err)
	}

	f.logger.Info("Successfully published batch of %d events", len(events))
	return nil
}
