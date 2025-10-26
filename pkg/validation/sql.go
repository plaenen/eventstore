package validation

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// SQL identifier validation for preventing SQL injection
//
// This package provides validation functions for SQL identifiers (table names,
// column names, etc.) to prevent SQL injection attacks when identifiers must
// be constructed dynamically.
//
// IMPORTANT: Always prefer parameterized queries via SQLC. Only use these
// validators when you absolutely must construct identifiers dynamically.

var (
	// sqlIdentifierRegex matches valid SQL identifiers:
	// - Must start with a letter or underscore
	// - Can contain letters, digits, and underscores
	// - Maximum length: 128 characters (PostgreSQL/SQLite limit)
	sqlIdentifierRegex = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]{0,127}$`)

	// Reserved SQL keywords that should not be used as identifiers
	// This is a subset of commonly reserved words across SQLite, PostgreSQL, MySQL
	reservedKeywords = map[string]bool{
		"SELECT": true, "INSERT": true, "UPDATE": true, "DELETE": true,
		"CREATE": true, "DROP": true, "ALTER": true, "TABLE": true,
		"INDEX": true, "VIEW": true, "TRIGGER": true, "FROM": true,
		"WHERE": true, "JOIN": true, "INNER": true, "OUTER": true,
		"LEFT": true, "RIGHT": true, "ON": true, "AND": true,
		"OR": true, "NOT": true, "NULL": true, "TRUE": true,
		"FALSE": true, "UNION": true, "EXCEPT": true, "INTERSECT": true,
		"ORDER": true, "GROUP": true, "HAVING": true, "LIMIT": true,
		"OFFSET": true, "AS": true, "BY": true, "ASC": true,
		"DESC": true, "DISTINCT": true, "ALL": true, "ANY": true,
		"SOME": true, "EXISTS": true, "IN": true, "LIKE": true,
		"BETWEEN": true, "IS": true, "CASE": true, "WHEN": true,
		"THEN": true, "ELSE": true, "END": true, "PRIMARY": true,
		"FOREIGN": true, "KEY": true, "REFERENCES": true, "UNIQUE": true,
		"CHECK": true, "DEFAULT": true, "CONSTRAINT": true, "CASCADE": true,
		"RESTRICT": true, "NO": true, "ACTION": true, "SET": true,
		"DECLARE": true, "BEGIN": true, "COMMIT": true, "ROLLBACK": true,
		"TRANSACTION": true, "SAVEPOINT": true, "RELEASE": true,
	}
)

// ValidateSQLIdentifier validates that a string is a safe SQL identifier.
//
// Rules:
// - Must start with a letter (a-z, A-Z) or underscore (_)
// - Can only contain letters, digits (0-9), and underscores
// - Length must be between 1 and 128 characters
// - Cannot be a reserved SQL keyword
// - Cannot contain SQL special characters (; , ' " ` etc.)
//
// Returns nil if valid, error describing the issue if invalid.
//
// Example:
//
//	err := ValidateSQLIdentifier("my_table_name")  // nil - valid
//	err := ValidateSQLIdentifier("user-table")     // error - contains hyphen
//	err := ValidateSQLIdentifier("SELECT")         // error - reserved keyword
//	err := ValidateSQLIdentifier("'; DROP TABLE")  // error - SQL injection attempt
func ValidateSQLIdentifier(identifier string) error {
	if identifier == "" {
		return fmt.Errorf("identifier cannot be empty")
	}

	// Check length (max 128 chars for SQLite/PostgreSQL)
	if len(identifier) > 128 {
		return fmt.Errorf("identifier too long: %d characters (max 128)", len(identifier))
	}

	// Check against regex pattern
	if !sqlIdentifierRegex.MatchString(identifier) {
		return fmt.Errorf("invalid identifier format: must start with letter or underscore, contain only letters, digits, and underscores")
	}

	// Check for reserved keywords
	upper := strings.ToUpper(identifier)
	if reservedKeywords[upper] {
		return fmt.Errorf("identifier '%s' is a reserved SQL keyword", identifier)
	}

	// Additional check for common SQL injection patterns
	if containsSQLInjectionPatterns(identifier) {
		return fmt.Errorf("identifier contains potentially malicious SQL patterns")
	}

	return nil
}

// containsSQLInjectionPatterns checks for common SQL injection patterns
func containsSQLInjectionPatterns(s string) bool {
	// Check for SQL comment markers
	if strings.Contains(s, "--") || strings.Contains(s, "/*") || strings.Contains(s, "*/") {
		return true
	}

	// Check for quote characters
	if strings.ContainsAny(s, "'\"`") {
		return true
	}

	// Check for semicolons (statement separator)
	if strings.Contains(s, ";") {
		return true
	}

	// Check for parentheses (could indicate function calls)
	if strings.ContainsAny(s, "()") {
		return true
	}

	// Check for operators that shouldn't be in identifiers
	if strings.ContainsAny(s, "=<>!+-*/|&^%~") {
		return true
	}

	// Check for whitespace (should only be in delimited identifiers)
	for _, r := range s {
		if unicode.IsSpace(r) {
			return true
		}
	}

	return false
}

// SanitizeIdentifier converts a string into a valid SQL identifier.
//
// This function:
// - Replaces hyphens with underscores
// - Removes or replaces invalid characters
// - Ensures the result starts with a letter or underscore
// - Truncates to 128 characters
// - Prepends 'id_' if the result would be a reserved keyword
//
// WARNING: This function may modify the identifier significantly.
// It's better to validate and reject invalid identifiers than to sanitize them.
// Use ValidateSQLIdentifier() when possible.
//
// Example:
//
//	SanitizeIdentifier("my-table")      // "my_table"
//	SanitizeIdentifier("123_table")     // "t_123_table"
//	SanitizeIdentifier("SELECT")        // "id_SELECT"
//	SanitizeIdentifier("user@domain")   // "user_domain"
func SanitizeIdentifier(identifier string) string {
	if identifier == "" {
		return "identifier"
	}

	// Replace hyphens with underscores
	result := strings.ReplaceAll(identifier, "-", "_")

	// Remove or replace other invalid characters
	var clean strings.Builder
	clean.Grow(len(result))

	for i, r := range result {
		switch {
		case unicode.IsLetter(r) || r == '_':
			clean.WriteRune(r)
		case unicode.IsDigit(r):
			// Digits are ok but not as first character
			clean.WriteRune(r)
		case unicode.IsSpace(r):
			// Replace whitespace with underscore
			clean.WriteRune('_')
		default:
			// Remove other characters
			continue
		}

		// Stop at 128 characters
		if i >= 127 {
			break
		}
	}

	result = clean.String()

	// Ensure it starts with a letter or underscore
	if len(result) == 0 || unicode.IsDigit(rune(result[0])) {
		result = "t_" + result
	}

	// Truncate to 128 characters
	if len(result) > 128 {
		result = result[:128]
	}

	// Check if it's a reserved keyword and prepend if needed
	if reservedKeywords[strings.ToUpper(result)] {
		result = "id_" + result
		if len(result) > 128 {
			result = result[:128]
		}
	}

	return result
}

// ValidateTableName is an alias for ValidateSQLIdentifier for clarity.
func ValidateTableName(tableName string) error {
	return ValidateSQLIdentifier(tableName)
}

// ValidateColumnName is an alias for ValidateSQLIdentifier for clarity.
func ValidateColumnName(columnName string) error {
	return ValidateSQLIdentifier(columnName)
}

// QuoteIdentifier safely quotes a SQL identifier using double quotes.
//
// This function:
// - Validates the identifier first
// - Wraps it in double quotes (SQL standard)
// - Escapes any embedded double quotes
//
// Use this when you must use an identifier that might conflict with keywords.
//
// Example:
//
//	quoted, err := QuoteIdentifier("user")  // "\"user\""
//	quoted, err := QuoteIdentifier("my_table")  // "\"my_table\""
func QuoteIdentifier(identifier string) (string, error) {
	// First validate
	if err := ValidateSQLIdentifier(identifier); err != nil {
		return "", err
	}

	// Escape any embedded double quotes (by doubling them)
	escaped := strings.ReplaceAll(identifier, "\"", "\"\"")

	// Wrap in double quotes
	return fmt.Sprintf(`"%s"`, escaped), nil
}
