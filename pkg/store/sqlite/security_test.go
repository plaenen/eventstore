package sqlite

import (
	"testing"

	"github.com/plaenen/eventstore/pkg/store/sqlite/migrate"
	"github.com/plaenen/eventstore/pkg/validation"
)

// TestSQL_Injection_Protection tests that the codebase is protected against SQL injection attacks.
// This test verifies that malicious table names and identifiers are rejected.
func TestSQL_Injection_Protection(t *testing.T) {
	// Test cases with known SQL injection patterns
	injectionAttempts := []string{
		"users; DROP TABLE events--",
		"users'; DROP TABLE events--",
		`users"; DROP TABLE events--`,
		"users/*comment*/",
		"users--comment",
		"users OR 1=1",
		"users' OR '1'='1",
		"users UNION SELECT * FROM passwords",
		"`users`",
		"users()",
		"../../etc/passwd",
		"<script>alert('xss')</script>",
		"../../../root",
		"user;",
		"user\x00admin",
	}

	t.Run("projection names are sanitized", func(t *testing.T) {
		for _, attempt := range injectionAttempts {
			// Sanitization should always produce a safe result
			sanitized := validation.SanitizeIdentifier(attempt)

			// The sanitized version should be valid
			if err := validation.ValidateSQLIdentifier(sanitized); err != nil {
				t.Errorf("SanitizeIdentifier(%q) = %q which is invalid: %v",
					attempt, sanitized, err)
			}

			// The sanitized version should NOT contain dangerous patterns
			if containsDangerousPattern(sanitized) {
				t.Errorf("SanitizeIdentifier(%q) = %q which still contains dangerous patterns",
					attempt, sanitized)
			}
		}
	})

	t.Run("migration table names are validated", func(t *testing.T) {
		for _, attempt := range injectionAttempts {
			// Should panic or error when creating migrator with invalid table name
			shouldPanic := func() {
				defer func() {
					if r := recover(); r == nil {
						t.Errorf("migrate.New(%q) did not panic, but should have", attempt)
					}
				}()
				migrate.New(nil, attempt)
			}
			shouldPanic()
		}
	})

	t.Run("valid identifiers are accepted", func(t *testing.T) {
		validNames := []string{
			"users",
			"user_accounts",
			"_private",
			"table123",
			"Account_Balance_2024",
			"projection_account_balance_schema_migrations",
		}

		for _, name := range validNames {
			if err := validation.ValidateSQLIdentifier(name); err != nil {
				t.Errorf("ValidateSQLIdentifier(%q) should be valid but got error: %v", name, err)
			}
		}
	})
}

// TestProjectionBuilder_SQL_Injection tests that projection names are safe
func TestProjectionBuilder_SQL_Injection(t *testing.T) {
	// Test that sanitizeTableName properly handles injection attempts
	tests := []struct {
		input    string
		shouldBeValid bool
	}{
		// Valid inputs
		{"account-balance", true},
		{"user-accounts", true},
		{"my_projection", true},

		// Injection attempts - should be sanitized to safe values
		{"users; DROP TABLE", true}, // Will be sanitized
		{"users'--", true},           // Will be sanitized
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := sanitizeTableName(tt.input)

			// Result should always be valid
			if err := validation.ValidateSQLIdentifier(result); err != nil {
				if tt.shouldBeValid {
					t.Errorf("sanitizeTableName(%q) = %q is invalid: %v", tt.input, result, err)
				}
			}

			// Result should not contain dangerous patterns
			if containsDangerousPattern(result) {
				t.Errorf("sanitizeTableName(%q) = %q contains dangerous patterns", tt.input, result)
			}
		})
	}
}

// containsDangerousPattern checks if a string contains SQL injection SYNTAX patterns.
// Note: Keywords like "DROP", "SELECT" etc. are safe if they're part of a valid identifier.
// What's dangerous is SQL syntax characters that could break out of the identifier context.
func containsDangerousPattern(s string) bool {
	dangerousSyntax := []string{
		";",  // Statement separator
		"--", // SQL comment
		"/*", // Block comment start
		"*/", // Block comment end
		"'",  // String delimiter
		"\"", // String delimiter
		"`",  // Identifier delimiter (MySQL/SQLite)
		"=",  // Comparison operator
		"(",  // Function call / subquery
		")",  // Function call / subquery
		" ",  // Whitespace can separate SQL tokens
		"\t", // Tab
		"\n", // Newline
	}

	for _, pattern := range dangerousSyntax {
		if contains(s, pattern) {
			return true
		}
	}
	return false
}

// contains is a simple case-sensitive substring check
func contains(s, substr string) bool {
	return len(s) >= len(substr) && indexOf(s, substr) >= 0
}

// indexOf returns the index of substr in s, or -1 if not found
func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
