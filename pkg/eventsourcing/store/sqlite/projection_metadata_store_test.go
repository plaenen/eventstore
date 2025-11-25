package sqlite

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestProjectionMetadataStore(t *testing.T) {
	// Create in-memory database for testing
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Create metadata store
	store, err := NewProjectionMetadataStore(db)
	if err != nil {
		t.Fatalf("Failed to create metadata store: %v", err)
	}

	t.Run("Get_NonExistentKey_ReturnsEmptyString", func(t *testing.T) {
		value, err := store.Get("test-projection", "non-existent")
		if err != nil {
			t.Errorf("Get failed: %v", err)
		}
		if value != "" {
			t.Errorf("Expected empty string, got %q", value)
		}
	})

	t.Run("Set_Get_Success", func(t *testing.T) {
		err := store.Set("test-projection", "schema_version", "3")
		if err != nil {
			t.Fatalf("Set failed: %v", err)
		}

		value, err := store.Get("test-projection", "schema_version")
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}

		if value != "3" {
			t.Errorf("Expected '3', got %q", value)
		}
	})

	t.Run("Set_UpdatesExistingValue", func(t *testing.T) {
		err := store.Set("test-projection", "schema_version", "3")
		if err != nil {
			t.Fatalf("Initial set failed: %v", err)
		}

		err = store.Set("test-projection", "schema_version", "4")
		if err != nil {
			t.Fatalf("Update failed: %v", err)
		}

		value, err := store.Get("test-projection", "schema_version")
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}

		if value != "4" {
			t.Errorf("Expected '4', got %q", value)
		}
	})

	t.Run("GetAll_ReturnsAllMetadata", func(t *testing.T) {
		// Clear any existing metadata first
		_ = store.DeleteAll("multi-projection")

		// Set multiple values
		err := store.Set("multi-projection", "schema_version", "2")
		if err != nil {
			t.Fatalf("Set schema_version failed: %v", err)
		}

		err = store.Set("multi-projection", "batch_size", "1000")
		if err != nil {
			t.Fatalf("Set batch_size failed: %v", err)
		}

		err = store.Set("multi-projection", "rebuild_requested", "true")
		if err != nil {
			t.Fatalf("Set rebuild_requested failed: %v", err)
		}

		// Get all metadata
		metadata, err := store.GetAll("multi-projection")
		if err != nil {
			t.Fatalf("GetAll failed: %v", err)
		}

		// Verify all values
		if len(metadata) != 3 {
			t.Errorf("Expected 3 metadata entries, got %d", len(metadata))
		}

		if metadata["schema_version"] != "2" {
			t.Errorf("Expected schema_version='2', got %q", metadata["schema_version"])
		}

		if metadata["batch_size"] != "1000" {
			t.Errorf("Expected batch_size='1000', got %q", metadata["batch_size"])
		}

		if metadata["rebuild_requested"] != "true" {
			t.Errorf("Expected rebuild_requested='true', got %q", metadata["rebuild_requested"])
		}
	})

	t.Run("GetAll_EmptyProjection_ReturnsEmptyMap", func(t *testing.T) {
		metadata, err := store.GetAll("non-existent-projection")
		if err != nil {
			t.Fatalf("GetAll failed: %v", err)
		}

		if len(metadata) != 0 {
			t.Errorf("Expected empty map, got %d entries", len(metadata))
		}
	})

	t.Run("Delete_RemovesKey", func(t *testing.T) {
		// Set a value
		err := store.Set("delete-test", "temp_key", "temp_value")
		if err != nil {
			t.Fatalf("Set failed: %v", err)
		}

		// Delete it
		err = store.Delete("delete-test", "temp_key")
		if err != nil {
			t.Fatalf("Delete failed: %v", err)
		}

		// Verify it's gone
		value, err := store.Get("delete-test", "temp_key")
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}

		if value != "" {
			t.Errorf("Expected empty string after delete, got %q", value)
		}
	})

	t.Run("Delete_NonExistentKey_NoError", func(t *testing.T) {
		err := store.Delete("non-existent", "non-existent")
		if err != nil {
			t.Errorf("Delete should not error on non-existent key: %v", err)
		}
	})

	t.Run("DeleteAll_RemovesAllMetadata", func(t *testing.T) {
		// Set multiple values
		_ = store.Set("cleanup-projection", "key1", "value1")
		_ = store.Set("cleanup-projection", "key2", "value2")
		_ = store.Set("cleanup-projection", "key3", "value3")

		// Delete all
		err := store.DeleteAll("cleanup-projection")
		if err != nil {
			t.Fatalf("DeleteAll failed: %v", err)
		}

		// Verify all are gone
		metadata, err := store.GetAll("cleanup-projection")
		if err != nil {
			t.Fatalf("GetAll failed: %v", err)
		}

		if len(metadata) != 0 {
			t.Errorf("Expected empty map after DeleteAll, got %d entries", len(metadata))
		}
	})

	t.Run("ListProjections_ReturnsProjectionsWithMetadata", func(t *testing.T) {
		// Create a fresh database for this test to avoid interference
		testDB, err := sql.Open("sqlite", ":memory:")
		if err != nil {
			t.Fatalf("Failed to open test database: %v", err)
		}
		defer testDB.Close()

		testStore, err := NewProjectionMetadataStore(testDB)
		if err != nil {
			t.Fatalf("Failed to create test metadata store: %v", err)
		}

		// Add metadata for multiple projections
		_ = testStore.Set("projection-a", "key1", "value1")
		_ = testStore.Set("projection-b", "key2", "value2")
		_ = testStore.Set("projection-c", "key3", "value3")
		_ = testStore.Set("projection-a", "key4", "value4") // duplicate projection

		// List projections
		projections, err := testStore.ListProjections()
		if err != nil {
			t.Fatalf("ListProjections failed: %v", err)
		}

		// Should have 3 unique projections
		if len(projections) != 3 {
			t.Errorf("Expected 3 projections, got %d: %v", len(projections), projections)
		}

		// Verify projection names (should be sorted)
		expectedProjections := []string{"projection-a", "projection-b", "projection-c"}
		for i, expected := range expectedProjections {
			if projections[i] != expected {
				t.Errorf("Expected projection[%d]=%q, got %q", i, expected, projections[i])
			}
		}
	})

	t.Run("MultipleProjections_Isolated", func(t *testing.T) {
		// Set metadata for two different projections
		_ = store.Set("proj-1", "key", "value-1")
		_ = store.Set("proj-2", "key", "value-2")

		// Verify they don't interfere with each other
		value1, err := store.Get("proj-1", "key")
		if err != nil {
			t.Fatalf("Get proj-1 failed: %v", err)
		}
		if value1 != "value-1" {
			t.Errorf("Expected 'value-1', got %q", value1)
		}

		value2, err := store.Get("proj-2", "key")
		if err != nil {
			t.Fatalf("Get proj-2 failed: %v", err)
		}
		if value2 != "value-2" {
			t.Errorf("Expected 'value-2', got %q", value2)
		}

		// Delete one shouldn't affect the other
		_ = store.Delete("proj-1", "key")

		value1After, _ := store.Get("proj-1", "key")
		if value1After != "" {
			t.Errorf("Expected empty string for proj-1, got %q", value1After)
		}

		value2After, _ := store.Get("proj-2", "key")
		if value2After != "value-2" {
			t.Errorf("Expected 'value-2' for proj-2, got %q", value2After)
		}
	})
}

func TestProjectionMetadataStore_Transactions(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	store, err := NewProjectionMetadataStore(db)
	if err != nil {
		t.Fatalf("Failed to create metadata store: %v", err)
	}

	t.Run("SetInTx_Commit_Success", func(t *testing.T) {
		tx, err := db.Begin()
		if err != nil {
			t.Fatalf("Failed to begin transaction: %v", err)
		}

		err = store.SetInTx(tx, "tx-proj", "tx-key", "tx-value")
		if err != nil {
			tx.Rollback()
			t.Fatalf("SetInTx failed: %v", err)
		}

		err = tx.Commit()
		if err != nil {
			t.Fatalf("Commit failed: %v", err)
		}

		// Verify the value was committed
		value, err := store.Get("tx-proj", "tx-key")
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if value != "tx-value" {
			t.Errorf("Expected 'tx-value', got %q", value)
		}
	})

	t.Run("SetInTx_Rollback_NotPersisted", func(t *testing.T) {
		tx, err := db.Begin()
		if err != nil {
			t.Fatalf("Failed to begin transaction: %v", err)
		}

		err = store.SetInTx(tx, "rollback-proj", "rollback-key", "rollback-value")
		if err != nil {
			tx.Rollback()
			t.Fatalf("SetInTx failed: %v", err)
		}

		tx.Rollback()

		// Verify the value was NOT persisted
		value, err := store.Get("rollback-proj", "rollback-key")
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if value != "" {
			t.Errorf("Expected empty string (rollback), got %q", value)
		}
	})

	t.Run("DeleteInTx_Commit_Success", func(t *testing.T) {
		// Set a value first
		_ = store.Set("delete-tx-proj", "delete-tx-key", "delete-tx-value")

		tx, err := db.Begin()
		if err != nil {
			t.Fatalf("Failed to begin transaction: %v", err)
		}

		err = store.DeleteInTx(tx, "delete-tx-proj", "delete-tx-key")
		if err != nil {
			tx.Rollback()
			t.Fatalf("DeleteInTx failed: %v", err)
		}

		err = tx.Commit()
		if err != nil {
			t.Fatalf("Commit failed: %v", err)
		}

		// Verify the value was deleted
		value, err := store.Get("delete-tx-proj", "delete-tx-key")
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if value != "" {
			t.Errorf("Expected empty string (deleted), got %q", value)
		}
	})
}

func TestProjectionMetadataStore_UseCases(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	store, err := NewProjectionMetadataStore(db)
	if err != nil {
		t.Fatalf("Failed to create metadata store: %v", err)
	}

	t.Run("UseCase_SchemaVersionTracking", func(t *testing.T) {
		projectionName := "user-projection"
		currentSchemaVersion := "3"

		// Save schema version
		err := store.Set(projectionName, "schema_version", currentSchemaVersion)
		if err != nil {
			t.Fatalf("Failed to save schema version: %v", err)
		}

		// Later: Check if rebuild needed
		savedVersion, err := store.Get(projectionName, "schema_version")
		if err != nil {
			t.Fatalf("Failed to get schema version: %v", err)
		}

		if savedVersion != currentSchemaVersion {
			t.Errorf("Schema version mismatch: expected %q, got %q", currentSchemaVersion, savedVersion)
		}
	})

	t.Run("UseCase_ManualRebuildRequest", func(t *testing.T) {
		projectionName := "analytics-projection"

		// Operator requests rebuild
		err := store.Set(projectionName, "rebuild_requested", "true")
		if err != nil {
			t.Fatalf("Failed to set rebuild flag: %v", err)
		}

		// On startup: Check rebuild flag
		rebuildRequested, err := store.Get(projectionName, "rebuild_requested")
		if err != nil {
			t.Fatalf("Failed to get rebuild flag: %v", err)
		}

		if rebuildRequested == "true" {
			// Rebuild would happen here
			// After rebuild, clear the flag
			err = store.Delete(projectionName, "rebuild_requested")
			if err != nil {
				t.Fatalf("Failed to clear rebuild flag: %v", err)
			}
		}

		// Verify flag is cleared
		cleared, _ := store.Get(projectionName, "rebuild_requested")
		if cleared != "" {
			t.Errorf("Expected empty rebuild flag, got %q", cleared)
		}
	})

	t.Run("UseCase_CustomProjectionConfiguration", func(t *testing.T) {
		projectionName := "recommendation-projection"

		// Store projection-specific settings
		config := map[string]string{
			"batch_size":       "1000",
			"algorithm":        "collaborative-filtering-v2",
			"retention_days":   "90",
			"filter_tenant_id": "tenant-123",
		}

		for key, value := range config {
			err := store.Set(projectionName, key, value)
			if err != nil {
				t.Fatalf("Failed to set %s: %v", key, err)
			}
		}

		// Read configuration at runtime
		savedConfig, err := store.GetAll(projectionName)
		if err != nil {
			t.Fatalf("Failed to get all config: %v", err)
		}

		// Verify all config values
		for key, expected := range config {
			if savedConfig[key] != expected {
				t.Errorf("Config mismatch for %s: expected %q, got %q", key, expected, savedConfig[key])
			}
		}
	})

	t.Run("UseCase_ProjectionVersioning", func(t *testing.T) {
		projectionName := "payment-projection"

		// Track projection logic version
		logicVersion := "2.1.0"
		err := store.Set(projectionName, "logic_version", logicVersion)
		if err != nil {
			t.Fatalf("Failed to set logic version: %v", err)
		}

		// Later: Check if projection logic changed
		deployedVersion, err := store.Get(projectionName, "logic_version")
		if err != nil {
			t.Fatalf("Failed to get logic version: %v", err)
		}

		if deployedVersion != logicVersion {
			// Rebuild would be triggered here
			t.Logf("Logic version changed: %s -> %s (would trigger rebuild)", deployedVersion, logicVersion)
		}
	})
}
