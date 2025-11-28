package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"time"

	"github.com/plaenen/eventstore/pkg/errorx"
	"github.com/plaenen/eventstore/pkg/identity/store/sqlite/sqlcgen"
	"github.com/plaenen/eventstore/pkg/security/encryption"
	"github.com/plaenen/eventstore/pkg/sqlite/migrate"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// SQLiteKeyStore is a SQLite-based implementation of KeyStore.
// It stores seeds in a table, encrypted at rest.
type SQLiteKeyStore struct {
	db         *sql.DB
	queries    *sqlcgen.Queries
	encryption *encryption.Service
}

// NewSQLiteKeyStore creates a new SQLiteKeyStore.
// It runs migrations to ensure the schema is up to date.
func NewSQLiteKeyStore(ctx context.Context, db *sql.DB, encService *encryption.Service) (*SQLiteKeyStore, error) {
	// Run migrations
	migrator := migrate.New(db, "identity_schema_migrations")
	if err := migrator.LoadFromFS(migrationsFS, "migrations"); err != nil {
		return nil, fmt.Errorf("failed to load migrations: %w", err)
	}
	if err := migrator.Up(); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return &SQLiteKeyStore{
		db:         db,
		queries:    sqlcgen.New(db),
		encryption: encService,
	}, nil
}

// SaveSeed stores a seed in the database, encrypted.
func (s *SQLiteKeyStore) SaveSeed(ctx context.Context, id string, seed []byte) error {
	// Encrypt the seed
	encryptedSeed, err := s.encryption.Encrypt(seed)
	if err != nil {
		return errorx.Wrap(err, "failed to encrypt seed")
	}

	err = s.queries.SaveSeed(ctx, sqlcgen.SaveSeedParams{
		ID:        id,
		Seed:      encryptedSeed,
		CreatedAt: time.Now(),
	})
	if err != nil {
		return fmt.Errorf("failed to save seed to database: %w", err)
	}

	return nil
}

// GetSeed retrieves a seed from the database and decrypts it.
func (s *SQLiteKeyStore) GetSeed(ctx context.Context, id string) ([]byte, error) {
	encryptedSeed, err := s.queries.GetSeed(ctx, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errorx.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get seed from database: %w", err)
	}

	// Decrypt the seed
	seed, err := s.encryption.Decrypt(encryptedSeed)
	if err != nil {
		return nil, errorx.Wrap(err, "failed to decrypt seed")
	}

	return seed, nil
}
