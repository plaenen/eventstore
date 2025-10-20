package sqlite

import "database/sql"

// RunMigrations runs all pending migrations on the event store.
func (s *EventStore) RunMigrations() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return runMigrations(s.db)
}

// GetMigrationVersion returns the current migration version by querying the database directly.
// Returns (version, dirty, error).
// dirty indicates if a migration is currently in progress or failed.
// Note: This implementation queries the schema_migrations table directly to avoid issues
// with the golang-migrate sqlite driver and modernc.org/sqlite compatibility.
func (s *EventStore) GetMigrationVersion() (uint, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var version int64
	var dirty bool
	err := s.db.QueryRow("SELECT version, dirty FROM schema_migrations LIMIT 1").Scan(&version, &dirty)
	if err == sql.ErrNoRows {
		return 0, false, nil // No migrations run yet
	}
	if err != nil {
		return 0, false, err
	}

	return uint(version), dirty, nil
}
