package sqlite

import (
	"strings"
	"testing"
	"time"

	"github.com/plaenen/eventstore/pkg/eventsourcing"
)

// TestValidateAppendEventsInput tests the input validation for AppendEvents
func TestValidateAppendEventsInput(t *testing.T) {
	tests := []struct {
		name        string
		aggregateID string
		events      []*eventsourcing.Event
		wantErr     bool
		errContains string
	}{
		{
			name:        "valid event",
			aggregateID: "test-aggregate-123",
			events: []*eventsourcing.Event{
				{
					ID:            "event-1",
					AggregateID:   "test-aggregate-123",
					AggregateType: "TestAggregate",
					EventType:     "TestEvent",
					Version:       1,
					Timestamp:     time.Now(),
					Data:          []byte("{}"),
				},
			},
			wantErr: false,
		},
		{
			name:        "empty aggregate_id",
			aggregateID: "",
			events: []*eventsourcing.Event{
				{
					ID:            "event-1",
					AggregateID:   "test-aggregate-123",
					AggregateType: "TestAggregate",
					EventType:     "TestEvent",
					Version:       1,
				},
			},
			wantErr:     true,
			errContains: "value cannot be empty",
		},
		{
			name:        "whitespace-only aggregate_id",
			aggregateID: "   ",
			events: []*eventsourcing.Event{
				{
					ID:            "event-1",
					AggregateID:   "test-aggregate-123",
					AggregateType: "TestAggregate",
					EventType:     "TestEvent",
					Version:       1,
				},
			},
			wantErr:     true,
			errContains: "value cannot be empty",
		},
		{
			name:        "nil event",
			aggregateID: "test-aggregate-123",
			events: []*eventsourcing.Event{
				nil,
			},
			wantErr:     true,
			errContains: "event at index 0 is nil",
		},
		{
			name:        "empty event_id",
			aggregateID: "test-aggregate-123",
			events: []*eventsourcing.Event{
				{
					ID:            "",
					AggregateID:   "test-aggregate-123",
					AggregateType: "TestAggregate",
					EventType:     "TestEvent",
					Version:       1,
				},
			},
			wantErr:     true,
			errContains: "value cannot be empty",
		},
		{
			name:        "empty event aggregate_id",
			aggregateID: "test-aggregate-123",
			events: []*eventsourcing.Event{
				{
					ID:            "event-1",
					AggregateID:   "",
					AggregateType: "TestAggregate",
					EventType:     "TestEvent",
					Version:       1,
				},
			},
			wantErr:     true,
			errContains: "value cannot be empty",
		},
		{
			name:        "mismatched aggregate_id",
			aggregateID: "test-aggregate-123",
			events: []*eventsourcing.Event{
				{
					ID:            "event-1",
					AggregateID:   "different-aggregate-456",
					AggregateType: "TestAggregate",
					EventType:     "TestEvent",
					Version:       1,
				},
			},
			wantErr:     true,
			errContains: "aggregate_id mismatch",
		},
		{
			name:        "empty aggregate_type",
			aggregateID: "test-aggregate-123",
			events: []*eventsourcing.Event{
				{
					ID:            "event-1",
					AggregateID:   "test-aggregate-123",
					AggregateType: "",
					EventType:     "TestEvent",
					Version:       1,
				},
			},
			wantErr:     true,
			errContains: "aggregate_type cannot be empty",
		},
		{
			name:        "whitespace-only aggregate_type",
			aggregateID: "test-aggregate-123",
			events: []*eventsourcing.Event{
				{
					ID:            "event-1",
					AggregateID:   "test-aggregate-123",
					AggregateType: "   ",
					EventType:     "TestEvent",
					Version:       1,
				},
			},
			wantErr:     true,
			errContains: "aggregate_type cannot be empty",
		},
		{
			name:        "empty event_type",
			aggregateID: "test-aggregate-123",
			events: []*eventsourcing.Event{
				{
					ID:            "event-1",
					AggregateID:   "test-aggregate-123",
					AggregateType: "TestAggregate",
					EventType:     "",
					Version:       1,
				},
			},
			wantErr:     true,
			errContains: "event_type cannot be empty",
		},
		{
			name:        "negative version",
			aggregateID: "test-aggregate-123",
			events: []*eventsourcing.Event{
				{
					ID:            "event-1",
					AggregateID:   "test-aggregate-123",
					AggregateType: "TestAggregate",
					EventType:     "TestEvent",
					Version:       -1,
				},
			},
			wantErr:     true,
			errContains: "must be >= 0",
		},
		{
			name:        "empty constraint index_name",
			aggregateID: "test-aggregate-123",
			events: []*eventsourcing.Event{
				{
					ID:            "event-1",
					AggregateID:   "test-aggregate-123",
					AggregateType: "TestAggregate",
					EventType:     "TestEvent",
					Version:       1,
					UniqueConstraints: []eventsourcing.UniqueConstraint{
						{
							IndexName: "",
							Value:     "test-value",
							Operation: eventsourcing.ConstraintClaim,
						},
					},
				},
			},
			wantErr:     true,
			errContains: "value cannot be empty",
		},
		{
			name:        "empty constraint value",
			aggregateID: "test-aggregate-123",
			events: []*eventsourcing.Event{
				{
					ID:            "event-1",
					AggregateID:   "test-aggregate-123",
					AggregateType: "TestAggregate",
					EventType:     "TestEvent",
					Version:       1,
					UniqueConstraints: []eventsourcing.UniqueConstraint{
						{
							IndexName: "test-index",
							Value:     "",
							Operation: eventsourcing.ConstraintClaim,
						},
					},
				},
			},
			wantErr:     true,
			errContains: "value cannot be empty",
		},
		{
			name:        "valid constraint",
			aggregateID: "test-aggregate-123",
			events: []*eventsourcing.Event{
				{
					ID:            "event-1",
					AggregateID:   "test-aggregate-123",
					AggregateType: "TestAggregate",
					EventType:     "TestEvent",
					Version:       1,
					UniqueConstraints: []eventsourcing.UniqueConstraint{
						{
							IndexName: "test-index",
							Value:     "test-value",
							Operation: eventsourcing.ConstraintClaim,
						},
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAppendEventsInput(tt.aggregateID, tt.events)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateAppendEventsInput() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errContains != "" {
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("validateAppendEventsInput() error = %v, want error containing %q", err, tt.errContains)
				}
			}
		})
	}
}
