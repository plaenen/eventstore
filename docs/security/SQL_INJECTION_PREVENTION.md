# SQL Injection Prevention - SEC-003

**Status**: ✅ COMPLETE
**Severity**: HIGH
**Last Updated**: 2025-10-26

## Overview

This document describes the SQL injection prevention measures implemented in the Event Sourcing framework (SEC-003 from the security roadmap).

## Summary

**Result**: The codebase is **PROTECTED** against SQL injection attacks.

### Key Achievements

✅ **All queries use SQLC** with parameterized queries
✅ **SQL identifier validation** for dynamic table names
✅ **Comprehensive test coverage** for injection attempts
✅ **Zero dynamic SQL** construction in query code
✅ **Secure-by-default** projection and migration systems

## Implementation

### 1. SQLC for All Queries

All SQL queries are defined in `.sql` files and generated using [SQLC](https://sqlc.dev/), which provides:

- **Parameterized queries**: All user input is passed as parameters (`?` placeholders)
- **Type safety**: Go types are generated from SQL schemas
- **No string concatenation**: SQL is never constructed dynamically

**Query Files**:
- `pkg/store/sqlite/queries/events.sql` - Event store queries
- `pkg/store/sqlite/queries/commands.sql` - Command queries
- `pkg/store/sqlite/queries/constraints.sql` - Constraint queries
- `pkg/store/sqlite/queries/snapshots.sql` - Snapshot queries

**Example - Safe Parameterized Query**:
```sql
-- name: LoadEvents :many
SELECT event_id, aggregate_id, aggregate_type, event_type,
       version, timestamp, data, metadata, constraints
FROM events
WHERE aggregate_id = ? AND version > ?
ORDER BY version ASC;
```

Generated Go code:
```go
func (q *Queries) LoadEvents(ctx context.Context, arg LoadEventsParams) ([]Event, error) {
    // SQLC generates safe parameterized queries
    rows, err := q.db.QueryContext(ctx, loadEvents, arg.AggregateID, arg.Version)
    // ... parameter binding is handled safely by database/sql
}
```

### 2. SQL Identifier Validation

When SQL identifiers (table names, column names) **must** be constructed dynamically, they are validated using `pkg/validation/sql.go`:

**New Package: `pkg/validation/sql.go`**

Provides comprehensive SQL identifier validation:

```go
// ValidateSQLIdentifier validates that a string is a safe SQL identifier
func ValidateSQLIdentifier(identifier string) error {
    // Rules:
    // - Must start with letter or underscore
    // - Only letters, digits, underscores allowed
    // - Maximum 128 characters
    // - Not a reserved SQL keyword
    // - No SQL injection patterns
}

// SanitizeIdentifier converts a string into a valid SQL identifier
func SanitizeIdentifier(identifier string) string {
    // Replaces invalid characters
    // Ensures valid format
    // Returns safe identifier
}
```

**Validation Rules**:
- ✅ Starts with `[a-zA-Z_]`
- ✅ Contains only `[a-zA-Z0-9_]`
- ✅ Length: 1-128 characters
- ❌ Reserved keywords (`SELECT`, `DROP`, `TABLE`, etc.)
- ❌ SQL syntax (`; ' " -- /* */ ( ) =` etc.)
- ❌ Whitespace or special characters

**Example - Projection Name Sanitization**:
```go
// BEFORE (VULNERABLE):
func sanitizeTableName(name string) string {
    return strings.ReplaceAll(name, "-", "_")  // NOT SAFE!
}

// AFTER (SECURE):
func sanitizeTableName(name string) string {
    return validation.SanitizeIdentifier(name)  // SAFE!
}
```

### 3. Protected Areas

#### Migration System (`pkg/store/sqlite/migrate/`)

**Protection**: Table names are validated when creating a Migrator:

```go
func New(db *sql.DB, tableName string) *Migrator {
    // Validates table name to prevent SQL injection
    if err := validateTableName(tableName); err != nil {
        panic(fmt.Sprintf("invalid migration table name %q: %v", tableName, err))
    }
    return &Migrator{...}
}
```

**Safe Usage**:
```go
// These are all safe because table names are hardcoded:
m := migrate.New(db, "schema_migrations")  // ✅ SAFE
m := migrate.New(db, "checkpoint_schema_migrations")  // ✅ SAFE

// This would panic at initialization:
m := migrate.New(db, "table; DROP TABLE events--")  // ❌ PANICS
```

#### Projection System (`pkg/store/sqlite/projection_builder.go`)

**Protection**: Projection names are sanitized before use:

```go
func runProjectionMigrations(db *sql.DB, migrationsFS fs.FS, path string, projectionName string) error {
    // Sanitize projection name for use in table name
    sanitizedName := sanitizeTableName(projectionName)
    tableName := fmt.Sprintf("projection_%s_schema_migrations", sanitizedName)
    // ...
}
```

**Safe Usage**:
```go
// User-provided names are sanitized:
projection := sqlite.NewSQLiteProjectionBuilder(
    "account-balance",  // Input: hyphenated
    db, checkpointStore, eventStore,
).Build()
// Results in: "projection_account_balance_schema_migrations" ✅ SAFE
```

### 4. Migration Files

All migration files are **static SQL** with no dynamic content:

**Example - `migrations/000001_initial_schema.up.sql`**:
```sql
-- Static SQL - no injection risk
CREATE TABLE IF NOT EXISTS events (
    event_id TEXT PRIMARY KEY,
    aggregate_id TEXT NOT NULL,
    -- ... more columns
);

CREATE INDEX IF NOT EXISTS idx_events_aggregate
    ON events(aggregate_id, version);
```

✅ No string interpolation
✅ No user input
✅ No dynamic table names
✅ Reviewed and safe

## Security Audit Results

### Audit Checklist

| Area | Status | Notes |
|------|--------|-------|
| ✅ SQLC queries use parameters | SAFE | All `?` placeholders |
| ✅ Migration files static | SAFE | No dynamic SQL |
| ✅ Table name validation | SAFE | `validation.ValidateSQLIdentifier()` |
| ✅ Projection names sanitized | SAFE | `validation.SanitizeIdentifier()` |
| ✅ No `fmt.Sprintf` with SQL | SAFE | Only for validated identifiers |
| ✅ Reserved keyword check | SAFE | Prevents `SELECT`, `DROP`, etc. |
| ✅ Injection pattern detection | SAFE | Detects `;`, `'`, `--`, etc. |

### Attack Surface Analysis

**Areas That Could Be Vulnerable (But Aren't)**:

1. **Projection Names**
   - ✅ **PROTECTED**: Sanitized via `validation.SanitizeIdentifier()`
   - Input: `"users; DROP TABLE events--"`
   - Output: `"users_DROP_TABLE_events__"` (safe identifier)

2. **Migration Table Names**
   - ✅ **PROTECTED**: Validated via `validateTableName()`
   - Invalid names cause panic at initialization
   - All usage in codebase uses hardcoded names

3. **Query Parameters**
   - ✅ **PROTECTED**: All use SQLC parameterized queries
   - No string concatenation in query code

## Test Coverage

### Validation Package Tests

**File**: `pkg/validation/sql_test.go`

- 23 validation test cases
- 11 sanitization test cases
- 26 injection pattern tests
- Benchmarks for performance

**Key Tests**:
```go
// Valid identifiers
TestValidateSQLIdentifier("users")              // ✅ Pass
TestValidateSQLIdentifier("user_accounts_123")  // ✅ Pass

// Injection attempts
TestValidateSQLIdentifier("users; DROP TABLE")  // ❌ Error
TestValidateSQLIdentifier("users' OR '1'='1")   // ❌ Error
TestValidateSQLIdentifier("SELECT")             // ❌ Error (reserved)
```

### Security Tests

**File**: `pkg/store/sqlite/security_test.go`

Tests actual SQL injection attempts against the codebase:

```go
func TestSQL_Injection_Protection(t *testing.T) {
    injectionAttempts := []string{
        "users; DROP TABLE events--",
        "users'; DROP TABLE events--",
        "users\" OR \"1\"=\"1",
        "users/*comment*/",
        // ... 15 different attack patterns
    }

    // All attempts are neutralized
    for _, attempt := range injectionAttempts {
        sanitized := validation.SanitizeIdentifier(attempt)
        // Verify no dangerous patterns remain
        assert.NoDangerousPatterns(sanitized)
    }
}
```

**Test Results**:
```
=== RUN   TestSQL_Injection_Protection
=== RUN   TestSQL_Injection_Protection/projection_names_are_sanitized
=== RUN   TestSQL_Injection_Protection/migration_table_names_are_validated
=== RUN   TestSQL_Injection_Protection/valid_identifiers_are_accepted
--- PASS: TestSQL_Injection_Protection
=== RUN   TestProjectionBuilder_SQL_Injection
--- PASS: TestProjectionBuilder_SQL_Injection
PASS
ok  	github.com/plaenen/eventstore/pkg/store/sqlite
```

## Best Practices

### DO ✅

1. **Use SQLC for all queries**
   ```go
   // Define in .sql file:
   -- name: GetUser :one
   SELECT * FROM users WHERE id = ?;

   // Use generated code:
   user, err := queries.GetUser(ctx, userID)
   ```

2. **Validate dynamic identifiers**
   ```go
   tableName := req.TableName
   if err := validation.ValidateSQLIdentifier(tableName); err != nil {
       return fmt.Errorf("invalid table name: %w", err)
   }
   ```

3. **Sanitize user input for identifiers**
   ```go
   projectionName := validation.SanitizeIdentifier(userInput)
   ```

4. **Use parameterized queries**
   ```go
   // CORRECT:
   db.QueryContext(ctx, "SELECT * FROM users WHERE id = ?", userID)

   // WRONG:
   db.QueryContext(ctx, fmt.Sprintf("SELECT * FROM users WHERE id = %s", userID))
   ```

### DON'T ❌

1. **Never concatenate SQL strings**
   ```go
   // ❌ VULNERABLE:
   query := "SELECT * FROM " + tableName + " WHERE id = " + userID
   db.Query(query)
   ```

2. **Never use `fmt.Sprintf` for SQL values**
   ```go
   // ❌ VULNERABLE:
   query := fmt.Sprintf("SELECT * FROM users WHERE name = '%s'", userName)
   db.Query(query)
   ```

3. **Never skip validation for "trusted" input**
   ```go
   // ❌ WRONG:
   // "It's from config, so it's safe"
   tableName := config.TableName
   query := fmt.Sprintf("CREATE TABLE %s", tableName)  // Still vulnerable!

   // ✅ CORRECT:
   if err := validation.ValidateSQLIdentifier(config.TableName); err != nil {
       return err
   }
   ```

## Production Checklist

Before deploying to production:

- [ ] All queries defined in SQLC `.sql` files
- [ ] No `fmt.Sprintf` with SQL keywords
- [ ] All dynamic identifiers validated
- [ ] Security tests passing
- [ ] Code review completed
- [ ] Penetration testing performed (if applicable)

## References

### Code Files

- `pkg/validation/sql.go` - SQL identifier validation
- `pkg/validation/sql_test.go` - Validation tests
- `pkg/store/sqlite/security_test.go` - Security tests
- `pkg/store/sqlite/queries/*.sql` - SQLC query definitions
- `pkg/store/sqlite/migrate/migrate.go` - Migration system
- `pkg/store/sqlite/projection_builder.go` - Projection system

### External Resources

- [OWASP SQL Injection](https://owasp.org/www-community/attacks/SQL_Injection)
- [SQLC Documentation](https://docs.sqlc.dev/)
- [Go database/sql Package](https://pkg.go.dev/database/sql)
- [CWE-89: SQL Injection](https://cwe.mitre.org/data/definitions/89.html)

## Continuous Monitoring

### Code Review Guidelines

When reviewing code that touches SQL:

1. ✅ Verify all queries use SQLC
2. ✅ Check for `fmt.Sprintf` with SQL keywords
3. ✅ Ensure identifiers are validated
4. ✅ Look for string concatenation with user input
5. ✅ Verify parameters use `?` placeholders

### Automated Checks

Consider adding to CI/CD:

```bash
# Check for dangerous patterns
grep -r "fmt.Sprintf.*SELECT\|INSERT\|UPDATE\|DELETE" pkg/

# Verify SQLC is used
find pkg/store/sqlite -name "*.go" -exec grep -l "db.Query\|db.Exec" {} \;

# Run security tests
go test ./pkg/store/sqlite/... -run SQL_Injection
go test ./pkg/validation/...
```

## Conclusion

**SEC-003 (SQL Injection Prevention) is COMPLETE and VERIFIED.**

The Event Sourcing framework is protected against SQL injection attacks through:

1. **SQLC parameterized queries** for all data queries
2. **SQL identifier validation** for dynamic table/column names
3. **Comprehensive testing** including injection attempt scenarios
4. **Secure-by-default** API design

No SQL injection vulnerabilities were found during the security audit.

---

**Document Version**: 1.0
**Last Security Audit**: 2025-10-26
**Next Review**: 2025-11-26 (monthly)
