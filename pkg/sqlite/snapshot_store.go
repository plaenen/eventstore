package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/plaenen/eventstore/pkg/eventsourcing"
	"github.com/plaenen/eventstore/pkg/sqlite/sqlcgen"
)

// SnapshotStore implements eventsourcing.SnapshotStore using SQLite.
type SnapshotStore struct {
	db      *sql.DB
	queries *sqlcgen.Queries
}

// NewSnapshotStore creates a new SQLite-backed snapshot store.
func NewSnapshotStore(db *sql.DB) *SnapshotStore {
	return &SnapshotStore{
		db:      db,
		queries: sqlcgen.New(db),
	}
}

// SaveSnapshot persists a snapshot for an aggregate.
func (s *SnapshotStore) SaveSnapshot(snapshot *eventsourcing.Snapshot) error {
	ctx := context.Background()

	metadata := ""
	if snapshot.Metadata != nil {
		m, err := snapshot.Metadata.MarshalMetadata()
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
		metadata = m
	}

	err := s.queries.SaveSnapshot(ctx, sqlcgen.SaveSnapshotParams{
		AggregateID:   snapshot.AggregateID,
		AggregateType: snapshot.AggregateType,
		Version:       snapshot.Version,
		Data:          snapshot.Data,
		CreatedAt:     snapshot.CreatedAt.Unix(),
		Metadata: sql.NullString{
			String: metadata,
			Valid:  metadata != "",
		},
	})

	if err != nil {
		return fmt.Errorf("failed to save snapshot: %w", err)
	}

	return nil
}

// GetLatestSnapshot retrieves the most recent snapshot for an aggregate.
func (s *SnapshotStore) GetLatestSnapshot(aggregateID string) (*eventsourcing.Snapshot, error) {
	ctx := context.Background()

	row, err := s.queries.GetLatestSnapshot(ctx, aggregateID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, eventsourcing.ErrSnapshotNotFound
		}
		return nil, fmt.Errorf("failed to get latest snapshot: %w", err)
	}

	return rowToSnapshot(row)
}

// GetSnapshotBeforeVersion retrieves the latest snapshot at or before a specific version.
func (s *SnapshotStore) GetSnapshotBeforeVersion(aggregateID string, version int64) (*eventsourcing.Snapshot, error) {
	ctx := context.Background()

	row, err := s.queries.GetLatestSnapshotBeforeVersion(ctx, sqlcgen.GetLatestSnapshotBeforeVersionParams{
		AggregateID: aggregateID,
		Version:     version,
	})
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, eventsourcing.ErrSnapshotNotFound
		}
		return nil, fmt.Errorf("failed to get snapshot before version: %w", err)
	}

	return rowToSnapshot(row)
}

// DeleteOldSnapshots removes snapshots older than the specified version for an aggregate.
func (s *SnapshotStore) DeleteOldSnapshots(aggregateID string, olderThanVersion int64) error {
	ctx := context.Background()

	err := s.queries.DeleteOldSnapshots(ctx, sqlcgen.DeleteOldSnapshotsParams{
		AggregateID: aggregateID,
		Version:     olderThanVersion,
	})

	if err != nil {
		return fmt.Errorf("failed to delete old snapshots: %w", err)
	}

	return nil
}

// GetSnapshotStats returns statistics about snapshots in the store.
func (s *SnapshotStore) GetSnapshotStats() (*eventsourcing.SnapshotStats, error) {
	ctx := context.Background()

	stats, err := s.queries.GetSnapshotStats(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get snapshot stats: %w", err)
	}

	totalSize := int64(0)
	if stats.TotalSizeBytes.Valid {
		totalSize = int64(stats.TotalSizeBytes.Float64)
	}

	avgSize := int64(0)
	if stats.AvgSizeBytes.Valid {
		avgSize = int64(stats.AvgSizeBytes.Float64)
	}

	oldestSnap := int64(0)
	if oldest, ok := stats.OldestSnapshot.(int64); ok {
		oldestSnap = oldest
	}

	newestSnap := int64(0)
	if newest, ok := stats.NewestSnapshot.(int64); ok {
		newestSnap = newest
	}

	return &eventsourcing.SnapshotStats{
		TotalSnapshots:   stats.TotalSnapshots,
		UniqueAggregates: stats.UniqueAggregates,
		TotalSizeBytes:   totalSize,
		AvgSizeBytes:     avgSize,
		OldestSnapshot:   time.Unix(oldestSnap, 0),
		NewestSnapshot:   time.Unix(newestSnap, 0),
	}, nil
}

// Helper function to convert sqlc snapshot to eventsourcing.Snapshot
func rowToSnapshot(row sqlcgen.Snapshot) (*eventsourcing.Snapshot, error) {
	var metadata *eventsourcing.SnapshotMetadata
	if row.Metadata.Valid && row.Metadata.String != "" {
		m, err := eventsourcing.UnmarshalMetadata(row.Metadata.String)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
		metadata = m
	}

	return &eventsourcing.Snapshot{
		AggregateID:   row.AggregateID,
		AggregateType: row.AggregateType,
		Version:       row.Version,
		Data:          row.Data,
		CreatedAt:     time.Unix(row.CreatedAt, 0),
		Metadata:      metadata,
	}, nil
}
