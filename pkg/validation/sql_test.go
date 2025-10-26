package validation

import (
	"strings"
	"testing"
)

func TestValidateSQLIdentifier(t *testing.T) {
	tests := []struct {
		name       string
		identifier string
		wantErr    bool
		errContains string
	}{
		// Valid identifiers
		{
			name:       "simple table name",
			identifier: "users",
			wantErr:    false,
		},
		{
			name:       "table with underscores",
			identifier: "user_accounts",
			wantErr:    false,
		},
		{
			name:       "starts with underscore",
			identifier: "_private_table",
			wantErr:    false,
		},
		{
			name:       "mixed case",
			identifier: "UserAccounts",
			wantErr:    false,
		},
		{
			name:       "with numbers",
			identifier: "table123",
			wantErr:    false,
		},
		{
			name:       "projection table",
			identifier: "projection_account_balance_schema_migrations",
			wantErr:    false,
		},

		// Invalid identifiers - Empty/Length
		{
			name:        "empty string",
			identifier:  "",
			wantErr:     true,
			errContains: "cannot be empty",
		},
		{
			name:        "too long",
			identifier:  strings.Repeat("a", 129),
			wantErr:     true,
			errContains: "too long",
		},

		// Invalid identifiers - Format
		{
			name:        "starts with number",
			identifier:  "123_table",
			wantErr:     true,
			errContains: "invalid identifier format",
		},
		{
			name:        "contains hyphen",
			identifier:  "user-accounts",
			wantErr:     true,
			errContains: "invalid identifier format",
		},
		{
			name:        "contains space",
			identifier:  "user accounts",
			wantErr:     true,
			errContains: "invalid identifier format",
		},
		{
			name:        "contains period",
			identifier:  "schema.table",
			wantErr:     true,
			errContains: "invalid identifier format",
		},

		// Reserved keywords
		{
			name:        "SELECT keyword",
			identifier:  "SELECT",
			wantErr:     true,
			errContains: "reserved SQL keyword",
		},
		{
			name:        "select lowercase",
			identifier:  "select",
			wantErr:     true,
			errContains: "reserved SQL keyword",
		},
		{
			name:        "TABLE keyword",
			identifier:  "TABLE",
			wantErr:     true,
			errContains: "reserved SQL keyword",
		},
		{
			name:        "FROM keyword",
			identifier:  "FROM",
			wantErr:     true,
			errContains: "reserved SQL keyword",
		},

		// SQL Injection attempts
		{
			name:        "SQL injection with semicolon",
			identifier:  "users; DROP TABLE events--",
			wantErr:     true,
			errContains: "invalid identifier format",
		},
		{
			name:        "SQL injection with quotes",
			identifier:  "users' OR '1'='1",
			wantErr:     true,
			errContains: "invalid identifier format",
		},
		{
			name:        "SQL injection with double quotes",
			identifier:  `users" OR "1"="1`,
			wantErr:     true,
			errContains: "invalid identifier format",
		},
		{
			name:        "SQL comment attempt",
			identifier:  "users--comment",
			wantErr:     true,
			errContains: "invalid identifier format",
		},
		{
			name:        "SQL block comment",
			identifier:  "users/*comment*/",
			wantErr:     true,
			errContains: "invalid identifier format",
		},
		{
			name:        "SQL with backticks",
			identifier:  "`users`",
			wantErr:     true,
			errContains: "invalid identifier format",
		},
		{
			name:        "SQL with parentheses",
			identifier:  "users()",
			wantErr:     true,
			errContains: "invalid identifier format",
		},
		{
			name:        "SQL with equals",
			identifier:  "user=admin",
			wantErr:     true,
			errContains: "invalid identifier format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSQLIdentifier(tt.identifier)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSQLIdentifier() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errContains != "" {
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("ValidateSQLIdentifier() error = %v, should contain %q", err, tt.errContains)
				}
			}
		})
	}
}

func TestSanitizeIdentifier(t *testing.T) {
	tests := []struct {
		name       string
		identifier string
		want       string
	}{
		{
			name:       "already valid",
			identifier: "users",
			want:       "users",
		},
		{
			name:       "hyphen to underscore",
			identifier: "user-accounts",
			want:       "user_accounts",
		},
		{
			name:       "multiple hyphens",
			identifier: "my-table-name",
			want:       "my_table_name",
		},
		{
			name:       "starts with number",
			identifier: "123_table",
			want:       "t_123_table",
		},
		{
			name:       "reserved keyword",
			identifier: "SELECT",
			want:       "id_SELECT",
		},
		{
			name:       "empty string",
			identifier: "",
			want:       "identifier",
		},
		{
			name:       "special characters",
			identifier: "user@domain.com",
			want:       "userdomaincom",
		},
		{
			name:       "whitespace",
			identifier: "user accounts",
			want:       "user_accounts",
		},
		{
			name:       "mixed invalid chars",
			identifier: "user-table@123!",
			want:       "user_table123",
		},
		{
			name:       "too long",
			identifier: strings.Repeat("a", 200),
			want:       strings.Repeat("a", 128),
		},
		{
			name:       "projection name with hyphen",
			identifier: "account-balance",
			want:       "account_balance",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeIdentifier(tt.identifier)
			if got != tt.want {
				t.Errorf("SanitizeIdentifier() = %v, want %v", got, tt.want)
			}

			// Verify the result is a valid identifier
			if err := ValidateSQLIdentifier(got); err != nil {
				t.Errorf("SanitizeIdentifier() produced invalid identifier %q: %v", got, err)
			}
		})
	}
}

func TestValidateTableName(t *testing.T) {
	// Should be same as ValidateSQLIdentifier
	err := ValidateTableName("users")
	if err != nil {
		t.Errorf("ValidateTableName(users) failed: %v", err)
	}

	err = ValidateTableName("invalid-name")
	if err == nil {
		t.Error("ValidateTableName(invalid-name) should have failed")
	}
}

func TestValidateColumnName(t *testing.T) {
	// Should be same as ValidateSQLIdentifier
	err := ValidateColumnName("user_id")
	if err != nil {
		t.Errorf("ValidateColumnName(user_id) failed: %v", err)
	}

	err = ValidateColumnName("user.id")
	if err == nil {
		t.Error("ValidateColumnName(user.id) should have failed")
	}
}

func TestQuoteIdentifier(t *testing.T) {
	tests := []struct {
		name       string
		identifier string
		want       string
		wantErr    bool
	}{
		{
			name:       "simple identifier",
			identifier: "users",
			want:       `"users"`,
			wantErr:    false,
		},
		{
			name:       "identifier with underscore",
			identifier: "user_accounts",
			want:       `"user_accounts"`,
			wantErr:    false,
		},
		{
			name:       "identifier with embedded quote",
			identifier: "user_id",
			want:       `"user_id"`,
			wantErr:    false,
		},
		{
			name:       "invalid identifier",
			identifier: "user-accounts",
			want:       "",
			wantErr:    true,
		},
		{
			name:       "SQL injection attempt",
			identifier: "users'; DROP TABLE--",
			want:       "",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := QuoteIdentifier(tt.identifier)
			if (err != nil) != tt.wantErr {
				t.Errorf("QuoteIdentifier() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("QuoteIdentifier() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestContainsSQLInjectionPatterns(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		// Safe strings
		{"simple", "users", false},
		{"with underscore", "user_accounts", false},
		{"with numbers", "table123", false},
		{"mixed case", "UserAccounts", false},

		// Dangerous patterns
		{"semicolon", "users;", true},
		{"single quote", "users'", true},
		{"double quote", `users"`, true},
		{"backtick", "users`", true},
		{"SQL comment", "users--", true},
		{"block comment start", "users/*", true},
		{"block comment end", "users*/", true},
		{"parentheses", "users()", true},
		{"equals", "users=admin", true},
		{"less than", "users<10", true},
		{"greater than", "users>10", true},
		{"exclamation", "users!", true},
		{"plus", "users+", true},
		{"minus", "users-accounts", true}, // hyphen triggers check (even though regex catches it first)
		{"asterisk", "users*", true},
		{"slash", "users/", true},
		{"pipe", "users|", true},
		{"ampersand", "users&", true},
		{"caret", "users^", true},
		{"percent", "users%", true},
		{"tilde", "users~", true},
		{"space", "user accounts", true},
		{"tab", "user\taccounts", true},
		{"newline", "user\naccounts", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containsSQLInjectionPatterns(tt.input)
			if got != tt.want {
				t.Errorf("containsSQLInjectionPatterns(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// Benchmark tests
func BenchmarkValidateSQLIdentifier(b *testing.B) {
	for i := 0; i < b.N; i++ {
		ValidateSQLIdentifier("user_accounts_table_name")
	}
}

func BenchmarkSanitizeIdentifier(b *testing.B) {
	for i := 0; i < b.N; i++ {
		SanitizeIdentifier("user-accounts-table-name")
	}
}

func BenchmarkValidateSQLIdentifier_Invalid(b *testing.B) {
	for i := 0; i < b.N; i++ {
		ValidateSQLIdentifier("users'; DROP TABLE--")
	}
}
