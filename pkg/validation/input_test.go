package validation

import (
	"strings"
	"testing"
)

func TestValidateUUIDv4(t *testing.T) {
	tests := []struct {
		name    string
		uuid    string
		wantErr bool
	}{
		// Valid UUIDs
		{
			name:    "valid UUIDv4",
			uuid:    "550e8400-e29b-41d4-a716-446655440000",
			wantErr: false,
		},
		{
			name:    "valid UUIDv4 uppercase",
			uuid:    "550E8400-E29B-41D4-A716-446655440000",
			wantErr: false,
		},
		{
			name:    "valid UUIDv4 mixed case",
			uuid:    "550e8400-E29B-41d4-A716-446655440000",
			wantErr: false,
		},
		{
			name:    "valid UUIDv4 with variant 8",
			uuid:    "123e4567-e89b-42d3-8456-426614174000",
			wantErr: false,
		},
		{
			name:    "valid UUIDv4 with variant 9",
			uuid:    "123e4567-e89b-42d3-9456-426614174000",
			wantErr: false,
		},
		{
			name:    "valid UUIDv4 with variant a",
			uuid:    "123e4567-e89b-42d3-a456-426614174000",
			wantErr: false,
		},
		{
			name:    "valid UUIDv4 with variant b",
			uuid:    "123e4567-e89b-42d3-b456-426614174000",
			wantErr: false,
		},

		// Invalid UUIDs
		{
			name:    "empty string",
			uuid:    "",
			wantErr: true,
		},
		{
			name:    "not a UUID",
			uuid:    "not-a-uuid",
			wantErr: true,
		},
		{
			name:    "UUIDv1 (version 1)",
			uuid:    "550e8400-e29b-11d4-a716-446655440000",
			wantErr: true,
		},
		{
			name:    "wrong format - no hyphens",
			uuid:    "550e8400e29b41d4a716446655440000",
			wantErr: true,
		},
		{
			name:    "wrong format - too short",
			uuid:    "550e8400-e29b-41d4-a716",
			wantErr: true,
		},
		{
			name:    "wrong variant (c)",
			uuid:    "123e4567-e89b-42d3-c456-426614174000",
			wantErr: true,
		},
		{
			name:    "invalid characters",
			uuid:    "550e8400-e29b-41d4-a716-44665544000g",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateUUIDv4(tt.uuid)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateUUIDv4() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateAggregateID(t *testing.T) {
	tests := []struct {
		name        string
		aggregateID string
		wantErr     bool
	}{
		{"valid", "550e8400-e29b-41d4-a716-446655440000", false},
		{"empty", "", true},
		{"invalid format", "not-a-uuid", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAggregateID(tt.aggregateID)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateAggregateID() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateCommandID(t *testing.T) {
	tests := []struct {
		name      string
		commandID string
		wantErr   bool
	}{
		{"valid", "550e8400-e29b-41d4-a716-446655440000", false},
		{"empty", "", true},
		{"invalid format", "not-a-uuid", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCommandID(tt.commandID)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCommandID() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateEmail(t *testing.T) {
	tests := []struct {
		name    string
		email   string
		wantErr bool
	}{
		// Valid emails
		{"simple email", "user@example.com", false},
		{"with subdomain", "user@mail.example.com", false},
		{"with plus", "user+tag@example.com", false},
		{"with dot", "first.last@example.com", false},
		{"with hyphen in domain", "user@my-site.com", false},
		{"with numbers", "user123@example456.com", false},

		// Invalid emails
		{"empty", "", true},
		{"no @", "userexample.com", true},
		{"no domain", "user@", true},
		{"no user", "@example.com", true},
		{"no TLD", "user@example", true},
		{"too long", strings.Repeat("a", 250) + "@example.com", true},
		{"spaces", "user @example.com", true},
		{"multiple @", "user@@example.com", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEmail(tt.email)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateEmail() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateTenantID(t *testing.T) {
	tests := []struct {
		name     string
		tenantID string
		wantErr  bool
	}{
		// Valid tenant IDs
		{"simple", "tenant123", false},
		{"with hyphen", "tenant-123", false},
		{"with underscore", "tenant_123", false},
		{"mixed", "my-tenant_123", false},
		{"all caps", "TENANT123", false},

		// Invalid tenant IDs
		{"empty", "", true},
		{"with space", "tenant 123", true},
		{"with @", "tenant@123", true},
		{"with dot", "tenant.123", true},
		{"too long", strings.Repeat("a", 129), true},
		{"special chars", "tenant!123", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTenantID(tt.tenantID)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTenantID() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidatePrincipalID(t *testing.T) {
	tests := []struct {
		name        string
		principalID string
		wantErr     bool
	}{
		// Valid principal IDs
		{"email", "user@example.com", false},
		{"service account", "service-account-123", false},
		{"with underscore", "user_123", false},
		{"with @", "user@domain", false},
		{"with dot", "user.name", false},
		{"mixed", "user-123@service.local", false},

		// Invalid principal IDs
		{"empty", "", true},
		{"with space", "user 123", true},
		{"too long", strings.Repeat("a", 257), true},
		{"special chars", "user!123", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePrincipalID(tt.principalID)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePrincipalID() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateStringLength(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		fieldName string
		minLength int
		maxLength int
		wantErr   bool
	}{
		{"valid within range", "hello", "name", 1, 10, false},
		{"exact min", "h", "name", 1, 10, false},
		{"exact max", "helloworld", "name", 1, 10, false},
		{"empty with min 0", "", "name", 0, 10, false},
		{"too short", "", "name", 1, 10, true},
		{"too long", "hello world!!", "name", 1, 10, true},
		{"UTF-8 multi-byte", "こんにちは", "name", 1, 10, false}, // 5 characters
		{"invalid max", "hello", "name", 1, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateStringLength(tt.value, tt.fieldName, tt.minLength, tt.maxLength)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateStringLength() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateStringNotEmpty(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		fieldName string
		wantErr   bool
	}{
		{"valid", "hello", "name", false},
		{"empty", "", "name", true},
		{"whitespace only", "   ", "name", true},
		{"tab", "\t", "name", true},
		{"newline", "\n", "name", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateStringNotEmpty(tt.value, tt.fieldName)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateStringNotEmpty() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateArraySize(t *testing.T) {
	tests := []struct {
		name      string
		size      int
		fieldName string
		maxSize   int
		wantErr   bool
	}{
		{"valid", 10, "items", 100, false},
		{"exact max", 100, "items", 100, false},
		{"zero size", 0, "items", 100, false},
		{"too large", 101, "items", 100, true},
		{"invalid max", 10, "items", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateArraySize(tt.size, tt.fieldName, tt.maxSize)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateArraySize() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateBinarySize(t *testing.T) {
	tests := []struct {
		name      string
		size      int64
		fieldName string
		maxSize   int64
		wantErr   bool
	}{
		{"valid 1KB", 1024, "file", 10 * 1024 * 1024, false},
		{"exact max", 10 * 1024 * 1024, "file", 10 * 1024 * 1024, false},
		{"zero size", 0, "file", 10 * 1024 * 1024, false},
		{"too large 11MB", 11 * 1024 * 1024, "file", 10 * 1024 * 1024, true},
		{"invalid max", 1024, "file", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBinarySize(tt.size, tt.fieldName, tt.maxSize)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateBinarySize() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateEventType(t *testing.T) {
	tests := []struct {
		name      string
		eventType string
		wantErr   bool
	}{
		{"simple", "Created", false},
		{"with domain", "account.Created", false},
		{"with subdomain", "bank.account.Created", false},
		{"with underscore", "user_profile.Updated", false},
		{"empty", "", true},
		{"too long", strings.Repeat("a", 257), true},
		{"with space", "account Created", true},
		{"with hyphen", "account-Created", true},
		{"with special chars", "account.Created!", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEventType(tt.eventType)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateEventType() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateAggregateType(t *testing.T) {
	tests := []struct {
		name          string
		aggregateType string
		wantErr       bool
	}{
		{"PascalCase", "BankAccount", false},
		{"snake_case", "bank_account", false},
		{"with numbers", "Account123", false},
		{"empty", "", true},
		{"too long", strings.Repeat("a", 129), true},
		{"with dot", "bank.account", true},
		{"with hyphen", "bank-account", true},
		{"with space", "Bank Account", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAggregateType(tt.aggregateType)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateAggregateType() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateVersion(t *testing.T) {
	tests := []struct {
		name    string
		version int64
		wantErr bool
	}{
		{"zero", 0, false},
		{"one", 1, false},
		{"large", 1000000, false},
		{"negative", -1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateVersion(tt.version)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateVersion() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDefaultInputValidators(t *testing.T) {
	validators := DefaultInputValidators()

	// Test that all validators are set
	if validators.ValidateAggregateID == nil {
		t.Error("ValidateAggregateID is nil")
	}
	if validators.ValidateCommandID == nil {
		t.Error("ValidateCommandID is nil")
	}
	if validators.ValidateEmail == nil {
		t.Error("ValidateEmail is nil")
	}
	if validators.ValidateTenantID == nil {
		t.Error("ValidateTenantID is nil")
	}
	if validators.ValidatePrincipalID == nil {
		t.Error("ValidatePrincipalID is nil")
	}
	if validators.ValidateEventType == nil {
		t.Error("ValidateEventType is nil")
	}
	if validators.ValidateAggregateType == nil {
		t.Error("ValidateAggregateType is nil")
	}

	// Test that validators work
	if err := validators.ValidateAggregateID("550e8400-e29b-41d4-a716-446655440000"); err != nil {
		t.Errorf("ValidateAggregateID failed: %v", err)
	}

	if err := validators.ValidateEmail("user@example.com"); err != nil {
		t.Errorf("ValidateEmail failed: %v", err)
	}
}

// Benchmark tests
func BenchmarkValidateUUIDv4(b *testing.B) {
	uuid := "550e8400-e29b-41d4-a716-446655440000"
	for i := 0; i < b.N; i++ {
		ValidateUUIDv4(uuid)
	}
}

func BenchmarkValidateEmail(b *testing.B) {
	email := "user@example.com"
	for i := 0; i < b.N; i++ {
		ValidateEmail(email)
	}
}

func BenchmarkValidateStringLength(b *testing.B) {
	value := "hello world"
	for i := 0; i < b.N; i++ {
		ValidateStringLength(value, "test", 1, 100)
	}
}
