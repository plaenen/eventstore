package validation

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Input validation for preventing injection attacks and enforcing data integrity.
//
// This package provides validators for common input types in the Event Sourcing framework:
// - UUIDs (v4 format)
// - Email addresses (RFC 5322)
// - Tenant IDs
// - Aggregate IDs
// - Command IDs
// - String lengths
// - Array sizes
// - Binary data sizes

var (
	// UUIDv4 regex pattern
	// Format: xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx where x is hex and y is 8,9,a,b
	uuidV4Regex = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

	// Email regex (simplified RFC 5322)
	emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

	// Tenant ID regex - alphanumeric, hyphens, underscores only
	tenantIDRegex = regexp.MustCompile(`^[a-zA-Z0-9_\-]{1,128}$`)

	// Principal ID regex - alphanumeric, hyphens, underscores, @, . (for emails or IDs)
	principalIDRegex = regexp.MustCompile(`^[a-zA-Z0-9_\-@.]{1,256}$`)
)

// Default limits
const (
	// String limits
	DefaultMaxStringLength = 1000
	DefaultMaxTextLength   = 10000
	DefaultMaxNameLength   = 256

	// Array limits
	DefaultMaxArraySize = 100

	// Binary limits
	DefaultMaxBinarySize = 10 * 1024 * 1024 // 10 MB
)

// ValidateUUIDv4 validates that a string is a valid UUIDv4.
//
// Rules:
// - Must be in format: xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx
// - All x must be hex digits (0-9, a-f)
// - Version digit must be 4
// - Variant digit must be 8, 9, a, or b
// - Case-insensitive (will be normalized to lowercase)
//
// Example:
//
//	err := ValidateUUIDv4("550e8400-e29b-41d4-a716-446655440000")  // nil
//	err := ValidateUUIDv4("not-a-uuid")                           // error
func ValidateUUIDv4(uuid string) error {
	if uuid == "" {
		return fmt.Errorf("UUID cannot be empty")
	}

	// Normalize to lowercase for validation
	uuid = strings.ToLower(uuid)

	if !uuidV4Regex.MatchString(uuid) {
		return fmt.Errorf("invalid UUIDv4 format: must be xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx")
	}

	return nil
}

// ValidateAggregateID validates an aggregate ID (UUIDv4 format).
func ValidateAggregateID(aggregateID string) error {
	if aggregateID == "" {
		return fmt.Errorf("aggregate_id cannot be empty")
	}

	if err := ValidateUUIDv4(aggregateID); err != nil {
		return fmt.Errorf("invalid aggregate_id: %w", err)
	}

	return nil
}

// ValidateCommandID validates a command ID (UUIDv4 format).
func ValidateCommandID(commandID string) error {
	if commandID == "" {
		return fmt.Errorf("command_id cannot be empty")
	}

	if err := ValidateUUIDv4(commandID); err != nil {
		return fmt.Errorf("invalid command_id: %w", err)
	}

	return nil
}

// ValidateEmail validates an email address.
//
// Rules:
// - Must match simplified RFC 5322 format
// - user@domain.tld
// - Case-insensitive
// - Maximum length: 256 characters
//
// Example:
//
//	err := ValidateEmail("user@example.com")  // nil
//	err := ValidateEmail("invalid.email")     // error
func ValidateEmail(email string) error {
	if email == "" {
		return fmt.Errorf("email cannot be empty")
	}

	if len(email) > 256 {
		return fmt.Errorf("email too long: %d characters (max 256)", len(email))
	}

	// Normalize to lowercase for validation
	email = strings.ToLower(email)

	if !emailRegex.MatchString(email) {
		return fmt.Errorf("invalid email format")
	}

	return nil
}

// ValidateTenantID validates a tenant ID.
//
// Rules:
// - Alphanumeric, hyphens, and underscores only
// - 1-128 characters
// - No spaces or special characters
//
// Example:
//
//	err := ValidateTenantID("tenant-123")    // nil
//	err := ValidateTenantID("tenant 123")    // error - contains space
func ValidateTenantID(tenantID string) error {
	if tenantID == "" {
		return fmt.Errorf("tenant_id cannot be empty")
	}

	if !tenantIDRegex.MatchString(tenantID) {
		return fmt.Errorf("invalid tenant_id: must be alphanumeric with hyphens/underscores, 1-128 characters")
	}

	return nil
}

// ValidatePrincipalID validates a principal ID (user/service identifier).
//
// Rules:
// - Can be email format (user@domain.com)
// - Or alphanumeric with hyphens, underscores, @, .
// - 1-256 characters
//
// Example:
//
//	err := ValidatePrincipalID("user@example.com")    // nil
//	err := ValidatePrincipalID("service-account-123") // nil
//	err := ValidatePrincipalID("user@domain")         // nil
func ValidatePrincipalID(principalID string) error {
	if principalID == "" {
		return fmt.Errorf("principal_id cannot be empty")
	}

	if !principalIDRegex.MatchString(principalID) {
		return fmt.Errorf("invalid principal_id: must be alphanumeric with hyphens/underscores/@/., 1-256 characters")
	}

	return nil
}

// ValidateStringLength validates string length constraints.
//
// Rules:
// - minLength: minimum length (inclusive), 0 means no minimum
// - maxLength: maximum length (inclusive), must be > 0
// - Counts UTF-8 runes, not bytes
//
// Example:
//
//	err := ValidateStringLength("hello", "name", 1, 100)   // nil
//	err := ValidateStringLength("", "name", 1, 100)        // error - too short
//	err := ValidateStringLength("x"*101, "name", 1, 100)   // error - too long
func ValidateStringLength(value, fieldName string, minLength, maxLength int) error {
	if maxLength <= 0 {
		return fmt.Errorf("maxLength must be > 0")
	}

	length := utf8.RuneCountInString(value)

	if minLength > 0 && length < minLength {
		return fmt.Errorf("%s too short: %d characters (min %d)", fieldName, length, minLength)
	}

	if length > maxLength {
		return fmt.Errorf("%s too long: %d characters (max %d)", fieldName, length, maxLength)
	}

	return nil
}

// ValidateStringNotEmpty validates that a string is not empty.
//
// Rules:
// - Must have at least 1 character
// - Whitespace-only strings are considered empty
//
// Example:
//
//	err := ValidateStringNotEmpty("hello", "name")   // nil
//	err := ValidateStringNotEmpty("", "name")        // error
//	err := ValidateStringNotEmpty("   ", "name")     // error
func ValidateStringNotEmpty(value, fieldName string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s cannot be empty", fieldName)
	}
	return nil
}

// ValidateArraySize validates the size of an array/slice.
//
// Rules:
// - size: actual array size
// - maxSize: maximum allowed size, must be > 0
//
// Example:
//
//	err := ValidateArraySize(10, "items", 100)   // nil
//	err := ValidateArraySize(101, "items", 100)  // error
func ValidateArraySize(size int, fieldName string, maxSize int) error {
	if maxSize <= 0 {
		return fmt.Errorf("maxSize must be > 0")
	}

	if size > maxSize {
		return fmt.Errorf("%s too large: %d items (max %d)", fieldName, size, maxSize)
	}

	return nil
}

// ValidateBinarySize validates the size of binary data.
//
// Rules:
// - size: actual data size in bytes
// - maxSize: maximum allowed size in bytes, must be > 0
//
// Example:
//
//	err := ValidateBinarySize(1024, "file", 10*1024*1024)  // nil - 1KB < 10MB
//	err := ValidateBinarySize(11*1024*1024, "file", 10*1024*1024)  // error - 11MB > 10MB
func ValidateBinarySize(size int64, fieldName string, maxSize int64) error {
	if maxSize <= 0 {
		return fmt.Errorf("maxSize must be > 0")
	}

	if size > maxSize {
		return fmt.Errorf("%s too large: %d bytes (max %d)", fieldName, size, maxSize)
	}

	return nil
}

// ValidateEventType validates an event type string.
//
// Rules:
// - Must not be empty
// - Typically in format: "domain.EventName" or "EventName"
// - Alphanumeric, dots, and underscores only
// - 1-256 characters
//
// Example:
//
//	err := ValidateEventType("account.Created")      // nil
//	err := ValidateEventType("user.ProfileUpdated")  // nil
//	err := ValidateEventType("")                     // error
func ValidateEventType(eventType string) error {
	if eventType == "" {
		return fmt.Errorf("event_type cannot be empty")
	}

	if len(eventType) > 256 {
		return fmt.Errorf("event_type too long: %d characters (max 256)", len(eventType))
	}

	// Allow alphanumeric, dots, and underscores
	eventTypeRegex := regexp.MustCompile(`^[a-zA-Z0-9._]+$`)
	if !eventTypeRegex.MatchString(eventType) {
		return fmt.Errorf("invalid event_type: must contain only alphanumeric characters, dots, and underscores")
	}

	return nil
}

// ValidateAggregateType validates an aggregate type string.
//
// Rules:
// - Must not be empty
// - Typically PascalCase or snake_case
// - Alphanumeric and underscores only
// - 1-128 characters
//
// Example:
//
//	err := ValidateAggregateType("BankAccount")    // nil
//	err := ValidateAggregateType("user_profile")   // nil
//	err := ValidateAggregateType("")               // error
func ValidateAggregateType(aggregateType string) error {
	if aggregateType == "" {
		return fmt.Errorf("aggregate_type cannot be empty")
	}

	if len(aggregateType) > 128 {
		return fmt.Errorf("aggregate_type too long: %d characters (max 128)", len(aggregateType))
	}

	// Allow alphanumeric and underscores
	aggregateTypeRegex := regexp.MustCompile(`^[a-zA-Z0-9_]+$`)
	if !aggregateTypeRegex.MatchString(aggregateType) {
		return fmt.Errorf("invalid aggregate_type: must contain only alphanumeric characters and underscores")
	}

	return nil
}

// ValidateVersion validates an aggregate version number.
//
// Rules:
// - Must be >= 0
// - Typically sequential starting from 0 or 1
//
// Example:
//
//	err := ValidateVersion(0)   // nil
//	err := ValidateVersion(1)   // nil
//	err := ValidateVersion(-1)  // error
func ValidateVersion(version int64) error {
	if version < 0 {
		return fmt.Errorf("version must be >= 0, got %d", version)
	}
	return nil
}

// InputValidators provides a collection of validators for command inputs.
//
// Use this to validate command envelope metadata and payload data.
type InputValidators struct {
	// ValidateAggregateID validates aggregate IDs (UUIDv4)
	ValidateAggregateID func(string) error

	// ValidateCommandID validates command IDs (UUIDv4)
	ValidateCommandID func(string) error

	// ValidateEmail validates email addresses
	ValidateEmail func(string) error

	// ValidateTenantID validates tenant IDs
	ValidateTenantID func(string) error

	// ValidatePrincipalID validates principal IDs
	ValidatePrincipalID func(string) error

	// ValidateEventType validates event type strings
	ValidateEventType func(string) error

	// ValidateAggregateType validates aggregate type strings
	ValidateAggregateType func(string) error
}

// DefaultInputValidators returns validators with default implementations.
func DefaultInputValidators() *InputValidators {
	return &InputValidators{
		ValidateAggregateID:   ValidateAggregateID,
		ValidateCommandID:     ValidateCommandID,
		ValidateEmail:         ValidateEmail,
		ValidateTenantID:      ValidateTenantID,
		ValidatePrincipalID:   ValidatePrincipalID,
		ValidateEventType:     ValidateEventType,
		ValidateAggregateType: ValidateAggregateType,
	}
}
