package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/plaenen/eventstore/pkg/eventsourcing"
)

// ProjectionStatusStore implements eventsourcing.ProjectionStatusStore for SQLite.
type ProjectionStatusStore struct {
	db *sql.DB
}

// NewProjectionStatusStore creates a new SQLite-based projection status store.
func NewProjectionStatusStore(db *sql.DB) (*ProjectionStatusStore, error) {
	store := &ProjectionStatusStore{db: db}

	// Create status table
	if err := store.ensureTable(); err != nil {
		return nil, err
	}

	return store, nil
}

// ensureTable creates the projection status table if it doesn't exist.
func (s *ProjectionStatusStore) ensureTable() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS projection_status (
			projection_name TEXT PRIMARY KEY,
			status TEXT NOT NULL,
			message TEXT,
			updated_at INTEGER NOT NULL,
			progress_json TEXT
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create projection_status table: %w", err)
	}
	return nil
}

// Save saves the projection status.
func (s *ProjectionStatusStore) Save(state *eventsourcing.ProjectionState) error {
	ctx := context.Background()

	// Serialize progress to JSON
	var progressJSON *string
	if state.Progress != nil {
		data, err := json.Marshal(state.Progress)
		if err != nil {
			return fmt.Errorf("failed to marshal progress: %w", err)
		}
		str := string(data)
		progressJSON = &str
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO projection_status (projection_name, status, message, updated_at, progress_json)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(projection_name) DO UPDATE SET
			status = excluded.status,
			message = excluded.message,
			updated_at = excluded.updated_at,
			progress_json = excluded.progress_json
	`, state.ProjectionName, state.Status, state.Message, state.UpdatedAt.Unix(), progressJSON)

	if err != nil {
		return fmt.Errorf("failed to save projection status: %w", err)
	}

	return nil
}

// Load loads the projection status.
func (s *ProjectionStatusStore) Load(projectionName string) (*eventsourcing.ProjectionState, error) {
	ctx := context.Background()

	var status string
	var message sql.NullString
	var updatedAt int64
	var progressJSON sql.NullString

	err := s.db.QueryRowContext(ctx, `
		SELECT status, message, updated_at, progress_json
		FROM projection_status
		WHERE projection_name = ?
	`, projectionName).Scan(&status, &message, &updatedAt, &progressJSON)

	if err == sql.ErrNoRows {
		// No status found - assume ready
		return &eventsourcing.ProjectionState{
			ProjectionName: projectionName,
			Status:         eventsourcing.ProjectionStatusReady,
			UpdatedAt:      eventsourcing.Now(),
		}, nil
	}

	if err != nil {
		return nil, fmt.Errorf("failed to load projection status: %w", err)
	}

	state := &eventsourcing.ProjectionState{
		ProjectionName: projectionName,
		Status:         eventsourcing.ProjectionStatus(status),
		UpdatedAt:      eventsourcing.TimeFromUnix(updatedAt),
	}

	if message.Valid {
		state.Message = message.String
	}

	if progressJSON.Valid {
		var progress eventsourcing.RebuildProgress
		if err := json.Unmarshal([]byte(progressJSON.String), &progress); err != nil {
			return nil, fmt.Errorf("failed to unmarshal progress: %w", err)
		}
		state.Progress = &progress
	}

	return state, nil
}

// UpdateProgress updates rebuild progress.
func (s *ProjectionStatusStore) UpdateProgress(projectionName string, progress *eventsourcing.RebuildProgress) error {
	ctx := context.Background()

	// Serialize progress to JSON
	data, err := json.Marshal(progress)
	if err != nil {
		return fmt.Errorf("failed to marshal progress: %w", err)
	}

	_, err = s.db.ExecContext(ctx, `
		UPDATE projection_status
		SET progress_json = ?, updated_at = ?
		WHERE projection_name = ?
	`, string(data), eventsourcing.Now().Unix(), projectionName)

	if err != nil {
		return fmt.Errorf("failed to update progress: %w", err)
	}

	return nil
}
