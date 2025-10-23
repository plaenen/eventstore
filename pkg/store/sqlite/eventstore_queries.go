package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/plaenen/eventstore/pkg/domain"
	"github.com/plaenen/eventstore/pkg/store/sqlite/sqlcgen"
)

// GetCommandResult retrieves the result of a previously processed command.
func (s *EventStore) GetCommandResult(commandID string) (*domain.CommandResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.getCommandResultNoLock(commandID)
}

// getCommandResultNoLock retrieves command result without locking (internal use).
func (s *EventStore) getCommandResultNoLock(commandID string) (*domain.CommandResult, error) {
	ctx := context.Background()
	row, err := s.queries.GetProcessedCommand(ctx, sqlcgen.GetProcessedCommandParams{
		CommandID: commandID,
		ExpiresAt: domain.Now().Unix(),
	})

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("command not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query command: %w", err)
	}

	processedAt := row.ProcessedAt
	eventIDsJSON := row.EventIds

	var eventIDs []string
	if err := json.Unmarshal([]byte(eventIDsJSON), &eventIDs); err != nil {
		return nil, fmt.Errorf("failed to unmarshal event IDs: %w", err)
	}

	// Load the events
	events := make([]*domain.Event, 0, len(eventIDs))
	for _, eventID := range eventIDs {
		event, err := s.loadEventByID(eventID)
		if err != nil {
			return nil, fmt.Errorf("failed to load event %s: %w", eventID, err)
		}
		events = append(events, event)
	}

	return &domain.CommandResult{
		CommandID:        commandID,
		Events:           events,
		AlreadyProcessed: true,
		ProcessedAt:      time.Unix(processedAt, 0),
	}, nil
}

// loadEventByID loads a single event by its ID.
func (s *EventStore) loadEventByID(eventID string) (*domain.Event, error) {
	ctx := context.Background()
	row, err := s.queries.LoadEventByID(ctx, eventID)
	if err != nil {
		return nil, err
	}

	event := domain.Event{
		ID:            row.EventID,
		AggregateID:   row.AggregateID,
		AggregateType: row.AggregateType,
		EventType:     row.EventType,
		Version:       row.Version,
		Timestamp:     time.Unix(row.Timestamp, 0),
		Data:          row.Data,
	}

	json.Unmarshal([]byte(row.Metadata), &event.Metadata)
	if row.Constraints.Valid && row.Constraints.String != "" {
		json.Unmarshal([]byte(row.Constraints.String), &event.UniqueConstraints)
	}

	return &event, nil
}

// LoadEvents loads all events for an aggregate starting from afterVersion.
func (s *EventStore) LoadEvents(aggregateID string, afterVersion int64) ([]*domain.Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ctx := context.Background()
	rows, err := s.queries.LoadEvents(ctx, sqlcgen.LoadEventsParams{
		AggregateID: aggregateID,
		Version:     afterVersion,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query events: %w", err)
	}

	events := make([]*domain.Event, 0, len(rows))
	for _, row := range rows {
		event := domain.Event{
			ID:            row.EventID,
			AggregateID:   row.AggregateID,
			AggregateType: row.AggregateType,
			EventType:     row.EventType,
			Version:       row.Version,
			Timestamp:     time.Unix(row.Timestamp, 0),
			Data:          row.Data,
		}

		json.Unmarshal([]byte(row.Metadata), &event.Metadata)
		if row.Constraints.Valid && row.Constraints.String != "" {
			json.Unmarshal([]byte(row.Constraints.String), &event.UniqueConstraints)
		}

		events = append(events, &event)
	}

	return events, nil
}

// LoadAllEvents loads all events from all aggregates for projection building.
func (s *EventStore) LoadAllEvents(fromPosition int64, limit int) ([]*domain.Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ctx := context.Background()
	rows, err := s.queries.LoadAllEvents(ctx, sqlcgen.LoadAllEventsParams{
		Position: sql.NullInt64{Int64: fromPosition, Valid: true},
		Limit:    int64(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query all events: %w", err)
	}

	events := make([]*domain.Event, 0, len(rows))
	for _, row := range rows {
		event := domain.Event{
			ID:            row.EventID,
			AggregateID:   row.AggregateID,
			AggregateType: row.AggregateType,
			EventType:     row.EventType,
			Version:       row.Version,
			Timestamp:     time.Unix(row.Timestamp, 0),
			Data:          row.Data,
		}

		json.Unmarshal([]byte(row.Metadata), &event.Metadata)
		if row.Constraints.Valid && row.Constraints.String != "" {
			json.Unmarshal([]byte(row.Constraints.String), &event.UniqueConstraints)
		}

		events = append(events, &event)
	}

	return events, nil
}

// GetAggregateVersion returns the current version of an aggregate.
func (s *EventStore) GetAggregateVersion(aggregateID string) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ctx := context.Background()
	versionRaw, err := s.queries.GetAggregateVersion(ctx, aggregateID)
	if err != nil {
		return 0, fmt.Errorf("failed to get aggregate version: %w", err)
	}

	return versionRaw.(int64), nil
}

// CheckUniqueness checks if a value is available for claiming.
func (s *EventStore) CheckUniqueness(indexName, value string) (available bool, ownerID string, error error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ctx := context.Background()
	ownerID, err := s.queries.GetConstraintOwner(ctx, sqlcgen.GetConstraintOwnerParams{
		IndexName: indexName,
		Value:     value,
	})

	if err == sql.ErrNoRows {
		return true, "", nil // Available
	}
	if err != nil {
		return false, "", fmt.Errorf("failed to check uniqueness: %w", err)
	}

	return false, ownerID, nil // Not available, already claimed
}

// GetConstraintOwner returns the aggregate ID that owns a unique value.
func (s *EventStore) GetConstraintOwner(indexName, value string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ctx := context.Background()
	ownerID, err := s.queries.GetConstraintOwner(ctx, sqlcgen.GetConstraintOwnerParams{
		IndexName: indexName,
		Value:     value,
	})

	if err == sql.ErrNoRows {
		return "", nil // Not claimed
	}
	if err != nil {
		return "", fmt.Errorf("failed to get constraint owner: %w", err)
	}

	return ownerID, nil
}

// RebuildConstraints rebuilds the unique constraint index from the event stream.
func (s *EventStore) RebuildConstraints() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	ctx := context.Background()
	queries := sqlcgen.New(tx)

	// Clear existing constraints
	err = queries.DeleteAllConstraints(ctx)
	if err != nil {
		return fmt.Errorf("failed to clear constraints: %w", err)
	}

	// Replay all events in order
	rows, err := queries.GetAllConstraints(ctx)
	if err != nil {
		return fmt.Errorf("failed to query events: %w", err)
	}

	for _, row := range rows {
		if !row.Constraints.Valid || row.Constraints.String == "" {
			continue
		}

		var constraints []domain.UniqueConstraint
		if err := json.Unmarshal([]byte(row.Constraints.String), &constraints); err != nil {
			return fmt.Errorf("failed to unmarshal constraints: %w", err)
		}

		// Apply constraints
		for _, constraint := range constraints {
			if constraint.Operation == domain.ConstraintClaim {
				err = queries.ClaimConstraint(ctx, sqlcgen.ClaimConstraintParams{
					IndexName:   constraint.IndexName,
					Value:       constraint.Value,
					AggregateID: row.AggregateID,
					CreatedAt:   domain.Now().Unix(),
				})
				if err != nil {
					return fmt.Errorf("failed to rebuild constraint: %w", err)
				}
			} else if constraint.Operation == domain.ConstraintRelease {
				err = queries.ReleaseConstraint(ctx, sqlcgen.ReleaseConstraintParams{
					IndexName:   constraint.IndexName,
					Value:       constraint.Value,
					AggregateID: row.AggregateID,
				})
				if err != nil {
					return fmt.Errorf("failed to rebuild constraint release: %w", err)
				}
			}
		}
	}

	return tx.Commit()
}

// CleanExpiredCommands removes expired command records (maintenance operation).
func (s *EventStore) CleanExpiredCommands() (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ctx := context.Background()
	rowsAffected, err := s.queries.CleanExpiredCommands(ctx, domain.Now().Unix())
	if err != nil {
		return 0, fmt.Errorf("failed to clean expired commands: %w", err)
	}

	return rowsAffected, nil
}

// DB returns the underlying database connection for direct SQL queries (e.g., projections).
func (s *EventStore) DB() *sql.DB {
	return s.db
}

// Close closes the event store and releases resources.
func (s *EventStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.db.Close()
}
