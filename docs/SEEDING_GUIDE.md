# Event Seeding Guide

This guide shows you how to use the event seeding feature for migrations, bootstrapping, and test data setup.

## Table of Contents

1. [Quick Start](#quick-start)
2. [Core Concepts](#core-concepts)
3. [Using SeedAggregate (Recommended)](#using-seedaggregate-recommended)
4. [Using SeedEvents (Low-Level)](#using-seedevents-low-level)
5. [Real-World Examples](#real-world-examples)
6. [Best Practices](#best-practices)
7. [Troubleshooting](#troubleshooting)

## Quick Start

```go
// Bootstrap an admin user
admin := NewUser("admin-001")
admin.Create("admin@example.com", "System Administrator")
admin.AssignRole("super_admin")

// Seed with default options
repo := store.NewRepository(eventStore, "User", userFactory, userApplier)
result, err := repo.SeedAggregate(admin, 0, domain.DefaultSeedOptions())
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Seeded: %d events, Skipped: %d\n", result.Saved, result.Skipped)
```

## Core Concepts

### What is Seeding?

Event seeding is a special way to append events to the event store that differs from normal `Save()` operations:

| Feature | Save() | SeedEvents() |
|---------|--------|--------------|
| Idempotency | No - duplicate calls create duplicates | Yes - skips existing events |
| Event IDs | Must be provided | Auto-generated if missing |
| Version checking | Strict - fails on mismatch | Optional - can be disabled |
| Constraint handling | Fails on violation | Checks ownership, continues |
| Metadata | Standard tracking | Adds lineage metadata |

### When to Use Seeding

**✅ Good use cases:**
- Database migrations (importing historical data)
- Bootstrap data (admin users, system configs)
- Test fixtures (deterministic test data)
- Disaster recovery (rebuilding from backups)

**❌ Bad use cases:**
- Normal application writes (use Save())
- User-initiated actions (use SaveWithCommand())
- Real-time event processing (use Save())

### Seeding Options

```go
opts := &domain.SeedOptions{
    // Skip events with IDs that already exist (default: true)
    SkipExisting: true,

    // Ignore expectedVersion parameter (default: true)
    SkipVersionCheck: true,

    // Check constraint ownership before failing (default: true)
    CheckConstraintOwnership: true,

    // Auto-generate IDs for events without IDs (default: true)
    GenerateDeterministicIDs: true,

    // Custom tags for debugging and lineage tracking
    CustomTags: map[string]string{
        "migration": "v1.0.0",
        "source":    "legacy-db",
        "batch":     "2025-01-15",
    },
}
```

## Using SeedAggregate (Recommended)

The `SeedAggregate()` method is the high-level API that works with aggregate instances.

### Basic Usage

```go
// Create repository
repo := store.NewRepository(
    eventStore,
    "User",
    func(id string) *User { return &User{BaseAggregate: domain.NewBaseAggregate(id, "User")} },
    func(agg *User, evt *domain.Event) error { return agg.ApplyEvent(evt) },
)

// Create aggregate and apply business logic
user := NewUser("user-123")
user.Create("john@example.com", "John Doe")
user.VerifyEmail()

// Seed with defaults
result, err := repo.SeedAggregate(user, 0, domain.DefaultSeedOptions())
if err != nil {
    return err
}

// Check results
if result.HasErrors() {
    for _, seedErr := range result.Errors {
        log.Printf("Failed: %s\n", seedErr.String())
    }
}
```

### With Custom Metadata

```go
opts := domain.DefaultSeedOptions()
opts.CustomTags = map[string]string{
    "migration": "v2.0.0",
    "source":    "production-backup",
    "timestamp": time.Now().Format(time.RFC3339),
}

result, err := repo.SeedAggregate(admin, 0, opts)
```

### Running Repeatedly (Idempotency)

```go
// First run: saves 3 events
result1, _ := repo.SeedAggregate(user, 0, domain.DefaultSeedOptions())
fmt.Println(result1.Saved)   // 3
fmt.Println(result1.Skipped) // 0

// Second run: skips all 3 events
result2, _ := repo.SeedAggregate(user, 0, domain.DefaultSeedOptions())
fmt.Println(result2.Saved)   // 0
fmt.Println(result2.Skipped) // 3
```

## Using SeedEvents (Low-Level)

For more control, you can use the `SeedEvents()` method directly on the event store.

### Basic Usage

```go
events := []*domain.Event{
    {
        ID:            "seed-001",
        AggregateID:   "product-123",
        AggregateType: "Product",
        EventType:     "ProductCreated",
        Version:       1,
        Timestamp:     time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
        Data:          json.RawMessage(`{"name":"Widget","price":9.99}`),
    },
    {
        ID:            "seed-002",
        AggregateID:   "product-123",
        AggregateType: "Product",
        EventType:     "PriceUpdated",
        Version:       2,
        Timestamp:     time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
        Data:          json.RawMessage(`{"price":12.99}`),
    },
}

result, err := eventStore.SeedEvents("product-123", 0, events, domain.DefaultSeedOptions())
```

### Without Predefined IDs (Deterministic Generation)

```go
// Events without IDs - will be auto-generated deterministically
events := []*domain.Event{
    {
        // No ID field - will be generated from content
        AggregateID:   "order-456",
        AggregateType: "Order",
        EventType:     "OrderPlaced",
        Version:       1,
        Timestamp:     time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC),
        Data:          json.RawMessage(`{"total":99.99}`),
    },
}

result, _ := eventStore.SeedEvents("order-456", 0, events, domain.DefaultSeedOptions())

// The generated ID is returned in the result
fmt.Println(result.EventIDs[0]) // "seed-a1b2c3d4..."
```

### With Unique Constraints

```go
events := []*domain.Event{
    {
        ID:            "seed-user-001",
        AggregateID:   "user-123",
        AggregateType: "User",
        EventType:     "UserCreated",
        Version:       1,
        Timestamp:     time.Now(),
        Data:          json.RawMessage(`{"email":"admin@example.com"}`),
        UniqueConstraints: []domain.UniqueConstraint{
            {
                IndexName: "user_email",
                Value:     "admin@example.com",
                Operation: domain.ConstraintClaim,
            },
        },
    },
}

result, err := eventStore.SeedEvents("user-123", 0, events, domain.DefaultSeedOptions())

// If email is already claimed by user-123: succeeds (ownership check)
// If email is claimed by user-456: fails (different owner)
```

## Real-World Examples

### Example 1: Bootstrap Admin User

```go
func BootstrapAdminUser(repo *store.BaseRepository[*User]) error {
    // Create admin aggregate
    admin := NewUser("admin-001")
    admin.Create("admin@example.com", "System Administrator")
    admin.AssignRole("super_admin")
    admin.VerifyEmail()

    // Configure seeding
    opts := domain.DefaultSeedOptions()
    opts.CustomTags = map[string]string{
        "bootstrap": "system-init",
        "version":   "1.0.0",
    }

    // Seed
    result, err := repo.SeedAggregate(admin, 0, opts)
    if err != nil {
        return fmt.Errorf("failed to bootstrap admin: %w", err)
    }

    // Log results
    if result.Saved > 0 {
        log.Printf("Bootstrapped admin user with %d events\n", result.Saved)
    } else {
        log.Println("Admin user already exists, skipping bootstrap")
    }

    return nil
}
```

### Example 2: Historical Data Migration

```go
func MigrateUsersFromLegacyDB(legacyDB *sql.DB, repo *store.BaseRepository[*User]) error {
    // Query legacy data
    rows, err := legacyDB.Query(`
        SELECT id, email, name, created_at, verified_at
        FROM legacy_users
        WHERE created_at < '2025-01-01'
    `)
    if err != nil {
        return err
    }
    defer rows.Close()

    // Migration options
    opts := domain.DefaultSeedOptions()
    opts.CustomTags = map[string]string{
        "migration":  "legacy-users-v1",
        "batch_date": time.Now().Format("2006-01-02"),
        "source":     "legacy_database",
    }

    migrated := 0
    skipped := 0

    for rows.Next() {
        var id, email, name string
        var createdAt, verifiedAt time.Time

        if err := rows.Scan(&id, &email, &name, &createdAt, &verifiedAt); err != nil {
            return err
        }

        // Create aggregate with historical events
        user := NewUser(id)
        user.BaseAggregate.RecordThat(&domain.Event{
            AggregateID:   id,
            AggregateType: "User",
            EventType:     "UserCreated",
            Version:       1,
            Timestamp:     createdAt, // Use historical timestamp
            Data:          json.RawMessage(fmt.Sprintf(`{"email":"%s","name":"%s"}`, email, name)),
            UniqueConstraints: []domain.UniqueConstraint{
                {IndexName: "user_email", Value: email, Operation: domain.ConstraintClaim},
            },
        })

        if !verifiedAt.IsZero() {
            user.BaseAggregate.RecordThat(&domain.Event{
                AggregateID:   id,
                AggregateType: "User",
                EventType:     "EmailVerified",
                Version:       2,
                Timestamp:     verifiedAt,
                Data:          json.RawMessage(`{}`),
            })
        }

        // Seed user
        result, err := repo.SeedAggregate(user, 0, opts)
        if err != nil {
            return fmt.Errorf("failed to migrate user %s: %w", id, err)
        }

        migrated += result.Saved
        skipped += result.Skipped

        if result.HasErrors() {
            for _, seedErr := range result.Errors {
                log.Printf("Migration error for user %s: %s\n", id, seedErr.String())
            }
        }
    }

    log.Printf("Migration complete: %d events migrated, %d skipped\n", migrated, skipped)
    return nil
}
```

### Example 3: Test Fixtures

```go
func SetupTestFixtures(t *testing.T, repo *store.BaseRepository[*User]) {
    fixtures := []struct {
        id    string
        email string
        name  string
        role  string
    }{
        {"test-user-1", "alice@test.com", "Alice", "user"},
        {"test-user-2", "bob@test.com", "Bob", "user"},
        {"test-admin-1", "admin@test.com", "Admin", "admin"},
    }

    opts := domain.DefaultSeedOptions()
    opts.CustomTags = map[string]string{
        "test_fixture": "user_base_set",
        "test_run":     t.Name(),
    }

    for _, fixture := range fixtures {
        user := NewUser(fixture.id)
        user.Create(fixture.email, fixture.name)
        user.AssignRole(fixture.role)

        result, err := repo.SeedAggregate(user, 0, opts)
        require.NoError(t, err)

        if result.HasErrors() {
            t.Fatalf("Failed to seed fixture %s: %v", fixture.id, result.Errors)
        }
    }
}
```

### Example 4: Integration with Credentials Provider

```go
func SeedProductionBackup() error {
    // Load database URL from credentials provider
    provider := credentials.NewEnvTokenProvider("TURSO_DATABASE_URL", 5*time.Minute)
    dbURL, err := provider.GetToken(context.Background())
    if err != nil {
        return fmt.Errorf("failed to get database URL: %w", err)
    }

    authProvider := credentials.NewEnvTokenProvider("TURSO_AUTH_TOKEN", 5*time.Minute)
    authToken, err := authProvider.GetToken(context.Background())
    if err != nil {
        return fmt.Errorf("failed to get auth token: %w", err)
    }

    // Create event store with remote database
    eventStore, err := sqlite.NewEventStore(
        sqlite.WithLibSQLRemote(dbURL, authToken),
    )
    if err != nil {
        return err
    }
    defer eventStore.Close()

    // Create repository
    repo := store.NewRepository(eventStore, "Order", orderFactory, orderApplier)

    // Load backup data
    backupData, err := os.ReadFile("backup-orders.json")
    if err != nil {
        return err
    }

    var orders []BackupOrder
    if err := json.Unmarshal(backupData, &orders); err != nil {
        return err
    }

    // Seed orders
    opts := domain.DefaultSeedOptions()
    opts.CustomTags = map[string]string{
        "restore":    "production-backup",
        "backup_at":  "2025-01-15T10:00:00Z",
        "restore_at": time.Now().Format(time.RFC3339),
    }

    for _, backupOrder := range orders {
        order := reconstructOrderFromBackup(backupOrder)
        result, err := repo.SeedAggregate(order, 0, opts)
        if err != nil {
            log.Printf("Failed to restore order %s: %v\n", order.ID(), err)
            continue
        }

        log.Printf("Restored order %s: %d events saved, %d skipped\n",
            order.ID(), result.Saved, result.Skipped)
    }

    return nil
}
```

## Best Practices

### 1. Use Fixed Timestamps for Determinism

```go
// ✅ Good - deterministic seeding
event := &domain.Event{
    Timestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
    // ... other fields
}

// ❌ Bad - non-deterministic seeding
event := &domain.Event{
    Timestamp: time.Now(), // Different every run!
    // ... other fields
}
```

### 2. Always Check Results

```go
result, err := repo.SeedAggregate(user, 0, domain.DefaultSeedOptions())
if err != nil {
    return err
}

if result.HasErrors() {
    for _, seedErr := range result.Errors {
        // Log or handle individual failures
        log.Printf("Seed error: %s\n", seedErr.String())
    }

    // Decide: continue or abort?
    if result.Failed > result.Saved {
        return fmt.Errorf("too many seed failures: %d failed, %d saved",
            result.Failed, result.Saved)
    }
}
```

### 3. Tag Your Seeded Data

```go
opts := domain.DefaultSeedOptions()
opts.CustomTags = map[string]string{
    "migration":    "v1.0.0",
    "source":       "legacy-system",
    "batch":        "2025-01-15",
    "operator":     "admin@example.com",
    "backup_file":  "backup-20250115.sql",
}
```

### 4. Handle Unique Constraints Carefully

```go
// Seeding with constraints checks ownership
result, err := repo.SeedAggregate(user, 0, domain.DefaultSeedOptions())

// If constraint is owned by same aggregate: succeeds
// If constraint is owned by different aggregate: fails

// To override and force seed (dangerous!):
opts := domain.DefaultSeedOptions()
opts.CheckConstraintOwnership = false // Skip constraint checks
result, err := repo.SeedAggregate(user, 0, opts)
```

### 5. Batch Large Migrations

```go
func MigrateLargeDataset(legacyDB *sql.DB, repo *store.BaseRepository[*Order]) error {
    const batchSize = 1000

    for offset := 0; ; offset += batchSize {
        rows, err := legacyDB.Query(`
            SELECT * FROM orders
            ORDER BY id
            LIMIT ? OFFSET ?
        `, batchSize, offset)
        if err != nil {
            return err
        }

        count := 0
        for rows.Next() {
            // Process row...
            count++
        }
        rows.Close()

        if count == 0 {
            break // No more data
        }

        log.Printf("Processed batch: offset=%d, count=%d\n", offset, count)
    }

    return nil
}
```

### 6. Verify Seeded Data

```go
// After seeding, verify
result, err := repo.SeedAggregate(admin, 0, opts)
if err != nil {
    return err
}

// Load and verify
loaded, err := repo.Load(admin.ID())
if err != nil {
    return fmt.Errorf("failed to verify seeded aggregate: %w", err)
}

if loaded.Version() != int64(result.Saved) {
    return fmt.Errorf("version mismatch: expected %d, got %d",
        result.Saved, loaded.Version())
}
```

## Troubleshooting

### Issue: "Event already exists"

**Cause**: Event ID collision with SkipExisting=false

**Solution**:
```go
opts := domain.DefaultSeedOptions()
opts.SkipExisting = true // Enable idempotency
```

### Issue: "Constraint violation"

**Cause**: Unique constraint owned by different aggregate

**Solution**:
```go
// Option 1: Check what owns the constraint
owner, err := eventStore.GetConstraintOwner("user_email", "admin@example.com")
if err != nil {
    return err
}
log.Printf("Constraint owned by: %s\n", owner)

// Option 2: Override (dangerous!)
opts := domain.DefaultSeedOptions()
opts.CheckConstraintOwnership = false
```

### Issue: "Version mismatch"

**Cause**: SkipVersionCheck=false and aggregate already exists

**Solution**:
```go
// Load current version first
version, err := eventStore.GetAggregateVersion("user-123")
if err != nil {
    return err
}

// Seed from that version
result, err := repo.SeedAggregate(user, version, domain.DefaultSeedOptions())
```

### Issue: Non-Deterministic IDs

**Cause**: Using time.Now() for timestamps

**Solution**:
```go
// ✅ Use fixed timestamps from source data
event.Timestamp = historicalTimestamp

// ✅ Or use a deterministic base time
baseTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
event.Timestamp = baseTime.Add(time.Duration(index) * time.Second)
```

### Issue: Performance Degradation

**Cause**: Seeding millions of events in single transaction

**Solution**:
```go
// Batch by aggregate
for _, aggregateData := range largeBatch {
    // Each aggregate gets its own transaction
    result, err := repo.SeedAggregate(aggregate, 0, opts)
    if err != nil {
        log.Printf("Failed: %v\n", err)
        continue
    }
}
```

## See Also

- [LIBSQL_USAGE.md](LIBSQL_USAGE.md) - Database configuration and connection
- [seed-revised.md](seed-revised.md) - Technical design and implementation details
- [pkg/domain/seed.go](/Users/pascallaenen/Documents/github/eventsourcing/pkg/domain/seed.go) - Core seeding types
- [pkg/store/repository.go:151](/Users/pascallaenen/Documents/github/eventsourcing/pkg/store/repository.go:151) - SeedAggregate implementation
