package store

// ProjectionMetadataStore manages projection metadata for lifecycle operations.
// Metadata includes schema versions, rebuild flags, and custom properties.
//
// This completes the projection management triad:
// - ProjectionCheckpoint: tracks event position and NATS sequence
// - ProjectionStatus: monitors projection health (READY/REBUILDING/FAILED/PAUSED)
// - ProjectionMetadata: manages lifecycle (schema versions, rebuild triggers, config)
//
// Common use cases:
//
// 1. Schema Version Tracking (automatic rebuilds):
//
//	metadataStore.Set("user-projection", "schema_version", "3")
//	// On startup, check if rebuild needed:
//	version, _ := metadataStore.Get("user-projection", "schema_version")
//	if currentVersion > version { Rebuild() }
//
// 2. Manual Rebuild Requests:
//
//	metadataStore.Set("user-projection", "rebuild_requested", "true")
//	// On startup:
//	if metadataStore.Get("user-projection", "rebuild_requested") == "true" {
//	    Rebuild()
//	    metadataStore.Delete("user-projection", "rebuild_requested")
//	}
//
// 3. Custom Projection Configuration:
//
//	metadataStore.Set("analytics-projection", "batch_size", "1000")
//	metadataStore.Set("analytics-projection", "filter_tenant_id", "tenant-123")
//
// 4. Projection Versioning:
//
//	metadataStore.Set("recommendation-projection", "logic_version", "2.1.0")
//	metadataStore.Set("recommendation-projection", "algorithm", "collaborative-filtering-v2")
type ProjectionMetadataStore interface {
	// Get retrieves a metadata value by key.
	// Returns empty string if key doesn't exist.
	Get(projectionName, key string) (string, error)

	// Set saves a metadata key-value pair.
	// Creates or updates the value atomically.
	Set(projectionName, key, value string) error

	// Delete removes a metadata key.
	// No-op if key doesn't exist.
	Delete(projectionName, key string) error

	// GetAll retrieves all metadata for a projection as a map.
	// Returns empty map if projection has no metadata.
	GetAll(projectionName string) (map[string]string, error)

	// DeleteAll removes all metadata for a projection.
	// Useful during projection cleanup/removal.
	DeleteAll(projectionName string) error

	// ListProjections returns a list of all projection names that have metadata.
	// Useful for discovering which projections are configured.
	ListProjections() ([]string, error)
}
