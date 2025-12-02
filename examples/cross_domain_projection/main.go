package main

import (
	"context"
	"fmt"
	"log"

	"github.com/plaenen/eventstore/pkg/eventsourcing"
	"google.golang.org/protobuf/proto"

	accountv1 "github.com/plaenen/eventstore/examples/pb/account/domain/v1"
	projectionsv1 "github.com/plaenen/eventstore/examples/pb/projections/v1"
	subscriptionv1 "github.com/plaenen/eventstore/examples/pb/subscription/v1"
)

// UserStatsProjection maintains statistics about users across domains
type UserStatsProjection struct {
	// In-memory state for demonstration
	TotalAccounts      int
	TotalSubscriptions int
}

func main() {
	stats := &UserStatsProjection{}

	// Create the projection builder
	// This was generated from: message MyCustomProjection { option (eventsourcing.projection) = true; ... }
	builder := projectionsv1.NewMyCustomProjectionBuilder("user-stats-projection")

	// Register handler for AccountOpenedEvent (from Account aggregate)
	builder.OnAccountOpened(func(ctx context.Context, event *accountv1.AccountOpenedEvent, envelope *eventsourcing.EventEnvelope) error {
		log.Printf("Handling AccountOpened: ID=%s, Owner=%s", event.AccountId, event.OwnerName)
		stats.TotalAccounts++
		return nil
	})

	// Register handler for SubscriptionCreatedEvent (from Subscription aggregate)
	builder.OnSubscriptionCreated(func(ctx context.Context, event *subscriptionv1.SubscriptionCreatedEvent, envelope *eventsourcing.EventEnvelope) error {
		log.Printf("Handling SubscriptionCreated: ID=%s, Email=%s", event.SubscriptionId, event.AdminEmail)
		stats.TotalSubscriptions++
		return nil
	})

	// Register Reset handler
	builder.OnReset(func(ctx context.Context) error {
		log.Println("Resetting projection state")
		stats.TotalAccounts = 0
		stats.TotalSubscriptions = 0
		return nil
	})

	// Build the projection
	projection := builder.Build()

	// Simulate running the projection with some events
	simulateEvents(projection)
}

func simulateEvents(p eventsourcing.Projection) {
	ctx := context.Background()

	// 1. Simulate Account Opened
	accountEvent := &accountv1.AccountOpenedEvent{
		AccountId: "acc-123",
		OwnerName: "user-456",
	}
	// In a real app, you'd use the EventStore to get envelopes
	// Here we manually create one for demonstration
	fmt.Printf("Projection '%s' is ready to handle events!\n", p.Name())

	// Simulate handling the event
	// We need to wrap it in an envelope
	data, _ := proto.Marshal(accountEvent)
	envelope := &eventsourcing.EventEnvelope{
		Event: eventsourcing.Event{
			EventType: "account.domain.v1.AccountOpenedEvent",
			Data:      data,
		},
	}

	if err := p.Handle(ctx, envelope); err != nil {
		log.Printf("Error handling event: %v", err)
	} else {
		log.Println("Successfully handled AccountOpenedEvent")
	}
}
