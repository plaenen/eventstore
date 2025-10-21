package bankaccount

import (
	"fmt"
	"log"
	"time"

	"github.com/plaenen/eventsourcing/pkg/eventsourcing"
	accountv1 "github.com/plaenen/eventsourcing/examples/pb/account/v1"
	"google.golang.org/protobuf/proto"
)

// AccountRepositoryWithSnapshots extends the base repository with snapshot support
type AccountRepositoryWithSnapshots struct {
	*accountv1.AccountRepository
	eventStore          eventsourcing.EventStore
	snapshotStore       eventsourcing.SnapshotStore
	snapshotStrategy    eventsourcing.SnapshotStrategy
	lastSnapshotVersion map[string]int64 // Track last snapshot version per aggregate
}

// NewAccountRepositoryWithSnapshots creates a repository with snapshot support
func NewAccountRepositoryWithSnapshots(
	eventStore eventsourcing.EventStore,
	snapshotStore eventsourcing.SnapshotStore,
	snapshotStrategy eventsourcing.SnapshotStrategy,
) *AccountRepositoryWithSnapshots {
	return &AccountRepositoryWithSnapshots{
		AccountRepository:   accountv1.NewAccountRepository(eventStore),
		eventStore:          eventStore,
		snapshotStore:       snapshotStore,
		snapshotStrategy:    snapshotStrategy,
		lastSnapshotVersion: make(map[string]int64),
	}
}

// Load loads an account using snapshots for optimization
func (r *AccountRepositoryWithSnapshots) Load(aggregateID string) (*accountv1.AccountAggregate, error) {
	// Try to load the latest snapshot
	snapshot, err := r.snapshotStore.GetLatestSnapshot(aggregateID)

	account := accountv1.NewAccount(aggregateID)
	fromVersion := int64(0)

	if err == nil {
		// Restore from snapshot
		if err := account.UnmarshalSnapshot(snapshot.Data); err != nil {
			log.Printf("Failed to unmarshal snapshot, falling back to full event replay: %v", err)
		} else {
			// Create a synthetic event to set the version correctly
			snapshotEvent := &eventsourcing.Event{
				Version: snapshot.Version,
			}
			account.LoadFromHistory([]*eventsourcing.Event{snapshotEvent})
			// LoadEvents uses afterVersion (exclusive), so we pass snapshot.Version to get events AFTER it
			fromVersion = snapshot.Version
			r.lastSnapshotVersion[aggregateID] = snapshot.Version
		}
	} else if err != eventsourcing.ErrSnapshotNotFound {
		// Log error but continue with full replay
		log.Printf("Failed to load snapshot, using full event replay: %v", err)
	}

	// Load events since snapshot (or from beginning if no snapshot)
	events, err := r.eventStore.LoadEvents(aggregateID, fromVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to load events: %w", err)
	}

	// No events found and no snapshot
	if len(events) == 0 && fromVersion == 0 {
		return nil, eventsourcing.ErrAggregateNotFound
	}

	// Apply remaining events
	for _, event := range events {
		msg, err := deserializeEvent(event)
		if err != nil {
			return nil, err
		}
		if err := account.ApplyEvent(msg); err != nil {
			return nil, err
		}
		account.LoadFromHistory([]*eventsourcing.Event{event})
	}

	return account, nil
}

// SaveWithCommand saves the aggregate and creates snapshots according to strategy
func (r *AccountRepositoryWithSnapshots) SaveWithCommand(aggregate *accountv1.AccountAggregate, commandID string) (*eventsourcing.CommandResult, error) {
	// Save events first
	result, err := r.AccountRepository.SaveWithCommand(aggregate, commandID)
	if err != nil {
		return nil, err
	}

	// Don't snapshot if this was an idempotent replay
	if result.AlreadyProcessed {
		return result, nil
	}

	// Check if we should create a snapshot
	lastVersion := r.lastSnapshotVersion[aggregate.ID()]
	eventsSinceSnapshot := aggregate.Version() - lastVersion

	if r.snapshotStrategy.ShouldCreateSnapshot(aggregate.Version(), eventsSinceSnapshot) {
		// Reload the aggregate to ensure we have the latest state including all applied events
		// This is necessary because ApplyChange doesn't immediately update aggregate state
		freshAggregate, err := r.Load(aggregate.ID())
		if err != nil {
			log.Printf("Failed to reload aggregate for snapshot: %v", err)
		} else if err := r.createSnapshot(freshAggregate); err != nil {
			// Log error but don't fail the command
			log.Printf("Failed to create snapshot for %s: %v", aggregate.ID(), err)
		} else {
			r.lastSnapshotVersion[aggregate.ID()] = freshAggregate.Version()

			// Cleanup old snapshots (keep last 3 versions)
			// Delete snapshots older than current version - (3 * strategy interval)
			if strategy, ok := r.snapshotStrategy.(*eventsourcing.IntervalSnapshotStrategy); ok {
				retentionVersion := freshAggregate.Version() - (3 * strategy.Interval)
				if retentionVersion > 0 {
					if err := r.snapshotStore.DeleteOldSnapshots(freshAggregate.ID(), retentionVersion); err != nil {
						log.Printf("Failed to cleanup old snapshots for %s: %v", freshAggregate.ID(), err)
					}
				}
			}
		}
	}

	return result, nil
}

// createSnapshot creates a snapshot for the given aggregate
func (r *AccountRepositoryWithSnapshots) createSnapshot(aggregate *accountv1.AccountAggregate) error {
	startTime := time.Now()

	data, err := aggregate.MarshalSnapshot()
	if err != nil {
		return fmt.Errorf("failed to marshal snapshot: %w", err)
	}

	creationTime := time.Since(startTime).Milliseconds()

	snapshot := &eventsourcing.Snapshot{
		AggregateID:   aggregate.ID(),
		AggregateType: aggregate.Type(),
		Version:       aggregate.Version(),
		Data:          data,
		CreatedAt:     time.Now(),
		Metadata: &eventsourcing.SnapshotMetadata{
			Size:          int64(len(data)),
			EventCount:    aggregate.Version(),
			CreationTime:  creationTime,
			SnapshotType:  "protobuf",
			SchemaVersion: "1.0",
		},
	}

	return r.snapshotStore.SaveSnapshot(snapshot)
}

// Helper function for event deserialization
func deserializeEvent(event *eventsourcing.Event) (proto.Message, error) {
	switch event.EventType {
	case "accountv1.AccountOpenedEvent":
		msg := &accountv1.AccountOpenedEvent{}
		if err := proto.Unmarshal(event.Data, msg); err != nil {
			return nil, err
		}
		return msg, nil
	case "accountv1.MoneyDepositedEvent":
		msg := &accountv1.MoneyDepositedEvent{}
		if err := proto.Unmarshal(event.Data, msg); err != nil {
			return nil, err
		}
		return msg, nil
	case "accountv1.MoneyWithdrawnEvent":
		msg := &accountv1.MoneyWithdrawnEvent{}
		if err := proto.Unmarshal(event.Data, msg); err != nil {
			return nil, err
		}
		return msg, nil
	case "accountv1.AccountClosedEvent":
		msg := &accountv1.AccountClosedEvent{}
		if err := proto.Unmarshal(event.Data, msg); err != nil {
			return nil, err
		}
		return msg, nil
	default:
		return nil, fmt.Errorf("unknown event type: %s", event.EventType)
	}
}
