package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"github.com/plaenen/eventstore/pkg/errorx"
)

func TestTranslateError(t *testing.T) {
	tests := []struct {
		name         string
		inputErr     error
		resourceType string
		resourceID   string
		wantError    error
		checkType    bool // If true, only check error type/interface
	}{
		{
			name:      "nil error",
			inputErr:  nil,
			wantError: nil,
		},
		{
			name:         "sql.ErrNoRows",
			inputErr:     sql.ErrNoRows,
			resourceType: "Aggregate",
			resourceID:   "123",
			wantError:    errorx.NewNotFoundError("Aggregate", "123"),
		},
		{
			name:         "Primary Key Violation (numeric code)",
			inputErr:     fmt.Errorf("error code = 1555"), // SQLITE_CONSTRAINT_PRIMARYKEY
			resourceType: "Event",
			resourceID:   "evt-1",
			wantError:    errorx.NewConflictError("evt-1", -1, -1),
		},
		{
			name:         "Unique Constraint Violation (version)",
			inputErr:     fmt.Errorf("error code = 2067: UNIQUE constraint failed: events.aggregate_id, events.version"),
			resourceType: "Aggregate",
			resourceID:   "agg-1",
			wantError:    errorx.NewConflictError("agg-1", -1, -1),
		},
		{
			name:         "Unique Constraint Violation (other)",
			inputErr:     fmt.Errorf("error code = 2067: UNIQUE constraint failed: unique_constraints.value"),
			resourceType: "Constraint",
			resourceID:   "user@example.com",
			checkType:    true, // Check if it's a UniqueConstraintError
		},
		{
			name:         "Foreign Key Violation",
			inputErr:     fmt.Errorf("error code = 787"), // SQLITE_CONSTRAINT_FOREIGNKEY
			resourceType: "Event",
			resourceID:   "evt-1",
			checkType:    true, // Check if it wraps ErrInvalidArgument
		},
		{
			name:         "Database Busy",
			inputErr:     fmt.Errorf("error code = 5"), // SQLITE_BUSY
			resourceType: "EventStore",
			resourceID:   "db",
			checkType:    true, // Check if it wraps ErrTimeout
		},
		{
			name:         "Generic Constraint (string match)",
			inputErr:     errors.New("UNIQUE constraint failed: events.event_id"),
			resourceType: "Event",
			resourceID:   "evt-1",
			checkType:    true, // Should be treated as conflict/unique error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := translateError(tt.inputErr, tt.resourceType, tt.resourceID)

			if tt.checkType {
				if got == nil {
					t.Errorf("translateError() = nil, want error")
				}
				return
			}

			if tt.wantError == nil {
				if got != nil {
					t.Errorf("translateError() = %v, want nil", got)
				}
				return
			}

			if got == nil {
				t.Errorf("translateError() = nil, want %v", tt.wantError)
				return
			}

			if got.Error() != tt.wantError.Error() {
				t.Errorf("translateError() = %v, want %v", got, tt.wantError)
			}
		})
	}
}
