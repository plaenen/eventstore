package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// SeedOptions configures how events are seeded into the event store.
// Seeding is used for migrations, bootstrapping, and test data setup.
type SeedOptions struct {
	// SkipExisting makes seeding idempotent by skipping events with IDs that already exist.
	// When true, events with existing IDs are counted as "skipped" rather than causing errors.
	// Default: true
	SkipExisting bool

	// SkipVersionCheck disables optimistic concurrency checking during seeding.
	// When true, the expectedVersion parameter is ignored.
	// This allows seeding into aggregates with unknown current versions.
	// Default: true
	SkipVersionCheck bool

	// CheckConstraintOwnership determines how unique constraints are handled.
	// When true, constraints are checked but won't fail if the owner matches the aggregateID.
	// When false, all constraint checking is skipped (USE WITH CAUTION).
	// Default: true (check ownership for safety)
	CheckConstraintOwnership bool

	// GenerateDeterministicIDs automatically generates IDs for events without IDs.
	// IDs are generated as SHA-256 hash of: aggregateID + eventType + version + timestamp + data
	// This ensures the same event content always produces the same ID.
	// Default: true
	GenerateDeterministicIDs bool

	// CustomTags allows injecting custom metadata tags for debugging and data lineage.
	// These are added to the event metadata along with standard seeding markers.
	// Example: {"migration": "v1.0", "source": "legacy-db", "batch": "2025-01-15"}
	// Default: nil (no custom tags)
	CustomTags map[string]string
}

// DefaultSeedOptions returns recommended defaults for safe seeding.
func DefaultSeedOptions() *SeedOptions {
	return &SeedOptions{
		SkipExisting:             true,  // Idempotent by default
		SkipVersionCheck:         true,  // Don't enforce version during seeding
		CheckConstraintOwnership: true,  // Safe: check ownership
		GenerateDeterministicIDs: true,  // Auto-generate missing IDs
		CustomTags:               nil,   // No custom tags by default
	}
}

// SeedResult reports the outcome of a seed operation.
type SeedResult struct {
	// Saved is the count of events successfully persisted
	Saved int

	// Skipped is the count of events that already existed and were skipped
	Skipped int

	// Failed is the count of events that could not be saved
	Failed int

	// Errors contains detailed information about failures
	Errors []SeedError

	// EventIDs contains all event IDs (both generated and provided)
	EventIDs []string
}

// Success returns true if all events were either saved or skipped (no failures).
func (r *SeedResult) Success() bool {
	return r.Failed == 0
}

// HasErrors returns true if any events failed to save.
func (r *SeedResult) HasErrors() bool {
	return len(r.Errors) > 0
}

// SeedError provides detailed information about a single event that failed to seed.
type SeedError struct {
	// EventID is the ID of the event that failed (if available)
	EventID string

	// EventType is the type of the event that failed
	EventType string

	// Version is the aggregate version of the failed event
	Version int64

	// Reason is a human-readable explanation of the failure
	Reason string

	// Error is the underlying error that caused the failure
	Error error
}

// String returns a formatted error message.
func (e SeedError) String() string {
	return fmt.Sprintf("event %s (type=%s, version=%d): %s - %v",
		e.EventID, e.EventType, e.Version, e.Reason, e.Error)
}

// GenerateDeterministicSeedID generates a deterministic event ID for seeding.
// The ID is based on a SHA-256 hash of the event's content, ensuring idempotency.
//
// IMPORTANT: The timestamp is included in the hash, so events must have
// timestamps set BEFORE calling this function for true determinism.
//
// Hash includes:
//   - aggregate_id: ensures IDs are unique across aggregates
//   - event_type: distinguishes different event types
//   - version: allows same event type at different versions
//   - timestamp: ensures temporal ordering is preserved
//   - data: ensures different event data produces different IDs
//
// Returns: "seed-" prefix + first 32 hex chars of SHA-256 hash (128 bits)
func GenerateDeterministicSeedID(event *Event) string {
	h := sha256.New()

	// Include all relevant event fields in deterministic order
	h.Write([]byte(event.AggregateID))
	h.Write([]byte("|"))
	h.Write([]byte(event.AggregateType))
	h.Write([]byte("|"))
	h.Write([]byte(event.EventType))
	h.Write([]byte("|"))
	h.Write([]byte(fmt.Sprintf("%d", event.Version)))
	h.Write([]byte("|"))

	// Include timestamp for temporal uniqueness
	// Format: Unix nanoseconds as string for precision
	h.Write([]byte(fmt.Sprintf("%d", event.Timestamp.UnixNano())))
	h.Write([]byte("|"))

	// Include event data for content uniqueness
	h.Write(event.Data)

	// Generate hex string from hash
	hashBytes := h.Sum(nil)
	hashHex := hex.EncodeToString(hashBytes)

	// Use first 32 chars (128 bits) with "seed-" prefix for clarity
	return "seed-" + hashHex[:32]
}

// AugmentSeedMetadata adds seeding metadata to an event.
// This is used for data lineage tracking and debugging.
//
// Standard metadata added:
//   - _seeded: "true" (marks event as seeded)
//   - _seeded_at: RFC3339 timestamp of when seeding occurred
//
// Custom tags from SeedOptions.CustomTags are also added.
func AugmentSeedMetadata(event *Event, opts *SeedOptions) {
	if event.Metadata.Custom == nil {
		event.Metadata.Custom = make(map[string]string)
	}

	// Add standard seeding markers
	event.Metadata.Custom["_seeded"] = "true"
	event.Metadata.Custom["_seeded_at"] = event.Timestamp.Format("2006-01-02T15:04:05.999Z07:00")

	// Add custom tags for debugging and lineage
	if opts.CustomTags != nil {
		for key, value := range opts.CustomTags {
			event.Metadata.Custom[key] = value
		}
	}
}
