package eventsourcing_test

import (
	"context"
	"testing"
	"time"

	"github.com/plaenen/eventstore/pkg/eventsourcing"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestCommandBus(t *testing.T) {
	bus := eventsourcing.NewCommandBus()

	t.Run("RegisterAndSend", func(t *testing.T) {
		executed := false

		// Register handler
		bus.Register("test.Command", eventsourcing.CommandHandlerFunc(
			func(ctx context.Context, cmd *eventsourcing.CommandEnvelope) ([]*eventsourcing.Event, error) {
				executed = true
				return []*eventsourcing.Event{
					{
						ID:            "event-1",
						AggregateID:   "agg-1",
						AggregateType: "Test",
						EventType:     "test.Created",
						Version:       1,
						Timestamp:     time.Now(),
						Data:          []byte("test"),
						Metadata: eventsourcing.EventMetadata{
							CausationID: cmd.Metadata.CommandID,
						},
					},
				}, nil
			},
		))

		// Send command
		err := bus.Send(context.Background(), &eventsourcing.CommandEnvelope{
			Command: &emptypb.Empty{},
			Metadata: eventsourcing.CommandMetadata{
				CommandID:   "cmd-1",
				PrincipalID: "user-1",
				Custom: map[string]string{
					"command_type": "test.Command",
				},
			},
		})

		if err != nil {
			t.Fatalf("failed to send command: %v", err)
		}

		if !executed {
			t.Error("command handler was not executed")
		}
	})

	t.Run("CommandNotFound", func(t *testing.T) {
		err := bus.Send(context.Background(), &eventsourcing.CommandEnvelope{
			Command: &emptypb.Empty{},
			Metadata: eventsourcing.CommandMetadata{
				CommandID: "cmd-2",
				Custom: map[string]string{
					"command_type": "nonexistent.Command",
				},
			},
		})

		if err == nil {
			t.Error("expected error for nonexistent command")
		}
	})

	t.Run("Middleware", func(t *testing.T) {
		bus := eventsourcing.NewCommandBus()
		middlewareCalled := false

		// Add middleware
		bus.Use(func(next eventsourcing.CommandHandler) eventsourcing.CommandHandler {
			return eventsourcing.CommandHandlerFunc(func(ctx context.Context, cmd *eventsourcing.CommandEnvelope) ([]*eventsourcing.Event, error) {
				middlewareCalled = true
				return next.Handle(ctx, cmd)
			})
		})

		// Register handler
		bus.Register("test.MiddlewareCommand", eventsourcing.CommandHandlerFunc(
			func(ctx context.Context, cmd *eventsourcing.CommandEnvelope) ([]*eventsourcing.Event, error) {
				return nil, nil
			},
		))

		// Send command
		err := bus.Send(context.Background(), &eventsourcing.CommandEnvelope{
			Command: &emptypb.Empty{},
			Metadata: eventsourcing.CommandMetadata{
				CommandID: "cmd-3",
				Custom: map[string]string{
					"command_type": "test.MiddlewareCommand",
				},
			},
		})

		if err != nil {
			t.Fatalf("failed to send command: %v", err)
		}

		if !middlewareCalled {
			t.Error("middleware was not called")
		}
	})

	t.Run("MultipleMiddleware", func(t *testing.T) {
		bus := eventsourcing.NewCommandBus()
		order := []int{}

		// Add middleware in order
		bus.Use(func(next eventsourcing.CommandHandler) eventsourcing.CommandHandler {
			return eventsourcing.CommandHandlerFunc(func(ctx context.Context, cmd *eventsourcing.CommandEnvelope) ([]*eventsourcing.Event, error) {
				order = append(order, 1)
				events, err := next.Handle(ctx, cmd)
				order = append(order, 4)
				return events, err
			})
		})

		bus.Use(func(next eventsourcing.CommandHandler) eventsourcing.CommandHandler {
			return eventsourcing.CommandHandlerFunc(func(ctx context.Context, cmd *eventsourcing.CommandEnvelope) ([]*eventsourcing.Event, error) {
				order = append(order, 2)
				events, err := next.Handle(ctx, cmd)
				order = append(order, 3)
				return events, err
			})
		})

		// Register handler
		bus.Register("test.OrderCommand", eventsourcing.CommandHandlerFunc(
			func(ctx context.Context, cmd *eventsourcing.CommandEnvelope) ([]*eventsourcing.Event, error) {
				return nil, nil
			},
		))

		// Send command
		err := bus.Send(context.Background(), &eventsourcing.CommandEnvelope{
			Command: &emptypb.Empty{},
			Metadata: eventsourcing.CommandMetadata{
				CommandID: "cmd-4",
				Custom: map[string]string{
					"command_type": "test.OrderCommand",
				},
			},
		})

		if err != nil {
			t.Fatalf("failed to send command: %v", err)
		}

		// Verify middleware execution order: 1 -> 2 -> handler -> 3 -> 4
		expected := []int{1, 2, 3, 4}
		if len(order) != len(expected) {
			t.Fatalf("expected %d middleware calls, got %d", len(expected), len(order))
		}

		for i, v := range expected {
			if order[i] != v {
				t.Errorf("expected order[%d] = %d, got %d", i, v, order[i])
			}
		}
	})
}
