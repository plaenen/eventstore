package store

import (
	"context"
	"time"

	"github.com/plaenen/eventstore/pkg/domain"
)

// Projection defines the interface for building read models from events.
// Projections consume events and can be rebuilt from EventStore.
type Projection interface {
	// Name returns the unique name of this projection.
	Name() string

	// Handle processes an event and updates the read model.
	Handle(ctx context.Context, event *domain.EventEnvelope) error

	// Reset resets the projection state (useful for rebuilding).
	Reset(ctx context.Context) error
}

// ProjectionStatus represents the current operational status of a projection.
type ProjectionStatus string

const (
	// ProjectionStatusReady indicates the projection is up-to-date and ready to serve queries
	ProjectionStatusReady ProjectionStatus = "READY"

	// ProjectionStatusRebuilding indicates the projection is being rebuilt from scratch
	ProjectionStatusRebuilding ProjectionStatus = "REBUILDING"

	// ProjectionStatusFailed indicates the projection encountered an error
	ProjectionStatusFailed ProjectionStatus = "FAILED"

	// ProjectionStatusPaused indicates the projection is paused (not processing events)
	ProjectionStatusPaused ProjectionStatus = "PAUSED"
)

// ProjectionState tracks the operational state of a projection.
type ProjectionState struct {
	ProjectionName string
	Status         ProjectionStatus
	Message        string // Optional status message (e.g., error details)
	UpdatedAt      time.Time
	Progress       *RebuildProgress // Optional progress info during rebuild
}

// RebuildProgress tracks progress during a projection rebuild.
type RebuildProgress struct {
	EventsProcessed int64
	TotalEvents     int64 // 0 if unknown
	StartedAt       time.Time
	EstimatedETA    *time.Time // nil if can't estimate
}

// ProjectionStatusStore persists projection status for monitoring.
type ProjectionStatusStore interface {
	// Save saves the projection status
	Save(state *ProjectionState) error

	// Load loads the projection status
	Load(projectionName string) (*ProjectionState, error)

	// UpdateProgress updates rebuild progress
	UpdateProgress(projectionName string, progress *RebuildProgress) error
}

// EventHandlerRegistration represents a typed event handler registration.
// This is the key to enabling cross-domain projections.
type EventHandlerRegistration struct {
	EventType string
	Handler   func(context.Context, *domain.EventEnvelope) error
}
