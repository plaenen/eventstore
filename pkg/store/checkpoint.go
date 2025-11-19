package store

import "time"

// ProjectionCheckpoint tracks the progress of a projection.
type ProjectionCheckpoint struct {
	ProjectionName string

	// Position in EventStore (incremented during rebuild and normal processing)
	Position int64

	// NATSSequence is the last processed NATS stream sequence number.
	// This is used to resume subscriptions from the correct position.
	// Nullable to support legacy checkpoints (nil = no NATS checkpoint yet).
	NATSSequence *int64

	LastEventID string
	UpdatedAt   time.Time

	// IsRebuilding tracks if projection is currently rebuilding.
	// Used to detect interrupted rebuilds on restart.
	IsRebuilding bool
}

// CheckpointStore persists projection checkpoints.
type CheckpointStore interface {
	// Save saves a checkpoint.
	Save(checkpoint *ProjectionCheckpoint) error

	// Load loads a checkpoint for a projection.
	Load(projectionName string) (*ProjectionCheckpoint, error)

	// Delete deletes a checkpoint (for rebuilding).
	Delete(projectionName string) error
}
