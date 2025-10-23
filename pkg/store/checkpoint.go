package store

import "time"

// ProjectionCheckpoint tracks the progress of a projection.
type ProjectionCheckpoint struct {
	ProjectionName string
	Position       int64
	LastEventID    string
	UpdatedAt      time.Time
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
