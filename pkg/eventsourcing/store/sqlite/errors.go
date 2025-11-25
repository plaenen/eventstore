package sqlite

import (
	"database/sql"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/plaenen/eventstore/pkg/errorx"
)

// SQLite/LibSQL error codes
// See: https://www.sqlite.org/rescode.html
const (
	SQLITE_ERROR      = 1
	SQLITE_PERM       = 3
	SQLITE_BUSY       = 5
	SQLITE_LOCKED     = 6
	SQLITE_READONLY   = 8
	SQLITE_IOERR      = 10
	SQLITE_CORRUPT    = 11
	SQLITE_FULL       = 13
	SQLITE_CANTOPEN   = 14
	SQLITE_CONSTRAINT = 19
	SQLITE_AUTH       = 23

	// Extended constraint codes (most common in event sourcing)
	SQLITE_CONSTRAINT_CHECK      = 275
	SQLITE_CONSTRAINT_FOREIGNKEY = 787
	SQLITE_CONSTRAINT_NOTNULL    = 1299
	SQLITE_CONSTRAINT_PRIMARYKEY = 1555
	SQLITE_CONSTRAINT_UNIQUE     = 2067
)

// Regex to extract error code from libsql error messages
// Matches patterns like "error code = 19" or "error code=2067"
var errorCodeRegex = regexp.MustCompile(`error code\s*=\s*(\d+)`)

// Regex to extract field/column name from constraint error messages
// Matches patterns like "UNIQUE constraint failed: events.event_id"
var fieldRegex = regexp.MustCompile(`constraint failed:\s*\w+\.(\w+)`)

// extractErrorCode extracts the numeric error code from a libsql/sqlite error
func extractErrorCode(err error) int {
	if err == nil {
		return 0
	}

	errStr := err.Error()
	matches := errorCodeRegex.FindStringSubmatch(errStr)
	if len(matches) >= 2 {
		code, _ := strconv.Atoi(matches[1])
		return code
	}
	return 0
}

// extractFieldName extracts the field/column name from a constraint error message
func extractFieldName(err error) string {
	if err == nil {
		return ""
	}

	errStr := err.Error()
	matches := fieldRegex.FindStringSubmatch(errStr)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// translateError converts SQLite/LibSQL errors to domain errors from pkg/errorx.
//
// This function maps database-specific errors to domain errors that are meaningful
// in an event sourcing context:
//
// - sql.ErrNoRows → ErrNotFound (aggregate/event stream doesn't exist)
// - UNIQUE/PRIMARY KEY violations → ErrConflict (version conflict in event sourcing)
// - FOREIGN KEY violations → ErrInvalidArgument (invalid reference)
// - BUSY/LOCKED → ErrTimeout (database temporarily unavailable)
// - Other errors → ErrInternal (system errors)
//
// Usage:
//
//	events, err := eventsourcing.queryEvents(aggregateID)
//	if err != nil {
//	    return translateError(err, "Aggregate", aggregateID)
//	}
func translateError(err error, resourceType, resourceID string) error {
	if err == nil {
		return nil
	}

	// Handle sql.ErrNoRows (from QueryRow when no results)
	if err == sql.ErrNoRows {
		return errorx.NewNotFoundError(resourceType, resourceID)
	}

	errStr := strings.ToLower(err.Error())
	code := extractErrorCode(err)

	// Check extended constraint codes first (more specific)
	switch code {
	case SQLITE_CONSTRAINT_PRIMARYKEY:
		// In event sourcing, primary key violations typically mean:
		// 1. Duplicate event_id (idempotency - event already appended)
		// 2. Version conflict (optimistic locking)
		return errorx.NewConflictError(resourceID, -1, -1)

	case SQLITE_CONSTRAINT_UNIQUE:
		// Unique constraint violations in event sourcing:
		// 1. Duplicate aggregate_id + version (version conflict)
		// 2. Unique index violations (e.g., account_id already claimed)
		field := extractFieldName(err)
		if field == "version" || strings.Contains(errStr, "version") {
			return errorx.NewConflictError(resourceID, -1, -1)
		}
		// Other unique violations (e.g., unique business constraints)
		return errorx.NewUniqueConstraintError(field, resourceID, "")

	case SQLITE_CONSTRAINT_FOREIGNKEY:
		// Foreign key violations indicate invalid references
		return fmt.Errorf("%s %s: %w: referenced record does not exist",
			resourceType, resourceID, errorx.ErrInvalidArgument)

	case SQLITE_CONSTRAINT_NOTNULL:
		// NOT NULL violations indicate missing required fields
		field := extractFieldName(err)
		if field != "" {
			return fmt.Errorf("%s %s: %w: field '%s' is required",
				resourceType, resourceID, errorx.ErrInvalidArgument, field)
		}
		return fmt.Errorf("%s %s: %w: required field missing",
			resourceType, resourceID, errorx.ErrInvalidArgument)

	case SQLITE_CONSTRAINT_CHECK:
		// CHECK constraint violations indicate business rule violations
		return fmt.Errorf("%s %s: %w: check constraint failed",
			resourceType, resourceID, errorx.ErrInvalidArgument)
	}

	// Check primary error codes
	switch code {
	case SQLITE_CONSTRAINT:
		// Generic constraint - parse message to determine type
		return translateConstraintByMessage(err, errStr, resourceType, resourceID)

	case SQLITE_BUSY, SQLITE_LOCKED:
		// Database is locked by another connection
		return fmt.Errorf("%s %s: %w: database is busy, retry recommended",
			resourceType, resourceID, errorx.ErrTimeout)

	case SQLITE_FULL:
		// Disk is full
		return fmt.Errorf("%w: database is full", errorx.ErrUnavailable)

	case SQLITE_READONLY:
		// Database is read-only
		return fmt.Errorf("%w: database is read-only", errorx.ErrUnavailable)

	case SQLITE_PERM, SQLITE_AUTH:
		// Permission denied
		return fmt.Errorf("%w: database permission denied", errorx.ErrUnavailable)

	case SQLITE_CORRUPT:
		// Database corruption detected
		return fmt.Errorf("%w: database corruption detected", errorx.ErrDataCorruption)

	case SQLITE_IOERR:
		// I/O error
		return fmt.Errorf("%w: database I/O error", errorx.ErrInternal)

	case SQLITE_CANTOPEN:
		// Can't open database file
		return fmt.Errorf("%w: cannot open database", errorx.ErrUnavailable)
	}

	// Fallback: check by string patterns (for drivers without numeric codes)
	return translateByMessagePattern(err, errStr, resourceType, resourceID)
}

// translateConstraintByMessage handles generic SQLITE_CONSTRAINT by parsing message
func translateConstraintByMessage(err error, errStr, resourceType, resourceID string) error {
	switch {
	case strings.Contains(errStr, "unique"):
		field := extractFieldName(err)
		if field == "version" || strings.Contains(errStr, "version") {
			return errorx.NewConflictError(resourceID, -1, -1)
		}
		return errorx.NewUniqueConstraintError(field, resourceID, "")

	case strings.Contains(errStr, "primary key"):
		return errorx.NewConflictError(resourceID, -1, -1)

	case strings.Contains(errStr, "foreign key"):
		return fmt.Errorf("%s %s: %w: foreign key constraint",
			resourceType, resourceID, errorx.ErrInvalidArgument)

	case strings.Contains(errStr, "not null"):
		field := extractFieldName(err)
		return fmt.Errorf("%s %s: %w: field '%s' required",
			resourceType, resourceID, errorx.ErrInvalidArgument, field)

	case strings.Contains(errStr, "check"):
		return fmt.Errorf("%s %s: %w: check constraint",
			resourceType, resourceID, errorx.ErrInvalidArgument)

	default:
		return fmt.Errorf("%s %s: constraint violation: %w",
			resourceType, resourceID, err)
	}
}

// translateByMessagePattern handles errors without numeric codes
func translateByMessagePattern(err error, errStr, resourceType, resourceID string) error {
	switch {
	case strings.Contains(errStr, "no rows"):
		return errorx.NewNotFoundError(resourceType, resourceID)

	case strings.Contains(errStr, "unique constraint failed"):
		if strings.Contains(errStr, "version") {
			return errorx.NewConflictError(resourceID, -1, -1)
		}
		field := extractFieldName(err)
		return errorx.NewUniqueConstraintError(field, resourceID, "")

	case strings.Contains(errStr, "primary key constraint failed"):
		return errorx.NewConflictError(resourceID, -1, -1)

	case strings.Contains(errStr, "foreign key constraint failed"):
		return fmt.Errorf("%s %s: %w: foreign key violation",
			resourceType, resourceID, errorx.ErrInvalidArgument)

	case strings.Contains(errStr, "not null constraint failed"):
		field := extractFieldName(err)
		return fmt.Errorf("%s %s: %w: field '%s' required",
			resourceType, resourceID, errorx.ErrInvalidArgument, field)

	case strings.Contains(errStr, "check constraint failed"):
		return fmt.Errorf("%s %s: %w: validation failed",
			resourceType, resourceID, errorx.ErrInvalidArgument)

	case strings.Contains(errStr, "database is locked"):
		return fmt.Errorf("%s %s: %w: database busy",
			resourceType, resourceID, errorx.ErrTimeout)

	default:
		// Unknown error - wrap as internal error
		return errorx.Wrap(errorx.ErrInternal,
			fmt.Sprintf("database error for %s %s", resourceType, resourceID))
	}
}

// isRetryable checks if a database error should be retried.
//
// Returns true for:
// - SQLITE_BUSY (database locked)
// - SQLITE_LOCKED (table locked)
//
// These are transient errors that may succeed on retry.
func isRetryable(err error) bool {
	if err == nil {
		return false
	}

	code := extractErrorCode(err)
	if code == SQLITE_BUSY || code == SQLITE_LOCKED {
		return true
	}

	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "database is locked") ||
		strings.Contains(errStr, "database is busy")
}

// isConstraintViolation checks if error is any type of constraint violation
func isConstraintViolation(err error) bool {
	if err == nil {
		return false
	}

	code := extractErrorCode(err)
	// Extended constraint codes are 275-2579, primary code is 19
	if code == SQLITE_CONSTRAINT || (code >= 275 && code <= 2579) {
		return true
	}

	return strings.Contains(strings.ToLower(err.Error()), "constraint")
}

// isUniqueViolation checks if error is specifically a unique constraint violation
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}

	code := extractErrorCode(err)
	if code == SQLITE_CONSTRAINT_UNIQUE || code == SQLITE_CONSTRAINT_PRIMARYKEY {
		return true
	}

	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "unique constraint failed") ||
		strings.Contains(errStr, "primary key constraint failed")
}
