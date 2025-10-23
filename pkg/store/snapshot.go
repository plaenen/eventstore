package store

import (
	"encoding/json"
	"time"
)

// Snapshot represents a serialized aggregate state at a specific version.
type Snapshot struct {
	AggregateID   string
	AggregateType string
	Version       int64
	Data          []byte
	CreatedAt     time.Time
	Metadata      *SnapshotMetadata
}

// SnapshotMetadata contains information about the snapshot.
type SnapshotMetadata struct {
	Size          int64  `json:"size"`           // Size of the snapshot in bytes
	EventCount    int64  `json:"event_count"`    // Number of events included
	CreationTime  int64  `json:"creation_time"`  // Time taken to create snapshot (ms)
	SnapshotType  string `json:"snapshot_type"`  // Type of serialization used
	SchemaVersion string `json:"schema_version"` // Version of the aggregate schema
}

// MarshalMetadata serializes the snapshot metadata to JSON.
func (m *SnapshotMetadata) MarshalMetadata() (string, error) {
	if m == nil {
		return "", nil
	}
	data, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// UnmarshalMetadata deserializes the snapshot metadata from JSON.
func UnmarshalMetadata(data string) (*SnapshotMetadata, error) {
	if data == "" {
		return nil, nil
	}
	var m SnapshotMetadata
	if err := json.Unmarshal([]byte(data), &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// SnapshotStore defines the interface for snapshot persistence.
type SnapshotStore interface {
	// SaveSnapshot persists a snapshot for an aggregate.
	SaveSnapshot(snapshot *Snapshot) error

	// GetLatestSnapshot retrieves the most recent snapshot for an aggregate.
	GetLatestSnapshot(aggregateID string) (*Snapshot, error)

	// GetSnapshotBeforeVersion retrieves the latest snapshot at or before a specific version.
	GetSnapshotBeforeVersion(aggregateID string, version int64) (*Snapshot, error)

	// DeleteOldSnapshots removes snapshots older than the specified version for an aggregate.
	DeleteOldSnapshots(aggregateID string, olderThanVersion int64) error

	// GetSnapshotStats returns statistics about snapshots in the store.
	GetSnapshotStats() (*SnapshotStats, error)
}

// SnapshotStats contains statistics about snapshots.
type SnapshotStats struct {
	TotalSnapshots   int64
	UniqueAggregates int64
	TotalSizeBytes   int64
	AvgSizeBytes     int64
	OldestSnapshot   time.Time
	NewestSnapshot   time.Time
}

// SnapshotStrategy defines when snapshots should be created.
type SnapshotStrategy interface {
	// ShouldCreateSnapshot determines if a snapshot should be created
	// based on the aggregate's current state.
	ShouldCreateSnapshot(currentVersion int64, eventsSinceLastSnapshot int64) bool
}

// IntervalSnapshotStrategy creates snapshots every N events.
type IntervalSnapshotStrategy struct {
	Interval int64 // Create snapshot every N events
}

// NewIntervalSnapshotStrategy creates a strategy that snapshots every N events.
func NewIntervalSnapshotStrategy(interval int64) *IntervalSnapshotStrategy {
	return &IntervalSnapshotStrategy{Interval: interval}
}

// ShouldCreateSnapshot checks if we've passed the interval threshold.
func (s *IntervalSnapshotStrategy) ShouldCreateSnapshot(currentVersion int64, eventsSinceLastSnapshot int64) bool {
	if s.Interval <= 0 {
		return false
	}
	// Create snapshot if we've accumulated enough events since last snapshot
	return eventsSinceLastSnapshot >= s.Interval
}

// Snapshotable is an interface for aggregates that can be snapshotted.
type Snapshotable interface {
	// MarshalSnapshot serializes the aggregate state to bytes.
	MarshalSnapshot() ([]byte, error)

	// UnmarshalSnapshot deserializes the aggregate state from bytes.
	UnmarshalSnapshot(data []byte) error
}
