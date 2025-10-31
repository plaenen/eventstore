package compare

import (
	"testing"
	"time"
)

// Test structures
type User struct {
	ID        string
	Email     string
	Name      string
	Profile   Profile
	CreatedAt time.Time
	UpdatedAt time.Time
	Metadata  Metadata
}

type Profile struct {
	Bio       string
	AvatarURL string
	Location  string
}

type Metadata struct {
	Version   int
	Timestamp time.Time
	Tags      []string
}

func TestCompareAggregates_Equal(t *testing.T) {
	user1 := User{
		ID:    "user-1",
		Email: "test@example.com",
		Name:  "Test User",
		Profile: Profile{
			Bio:       "Software Engineer",
			AvatarURL: "https://example.com/avatar.jpg",
			Location:  "San Francisco",
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	user2 := user1

	result := CompareAggregates(user1, user2, nil)
	if !result.Equal {
		t.Errorf("Expected users to be equal, got diff:\n%s", result.Diff)
	}
}

func TestCompareAggregates_Different(t *testing.T) {
	user1 := User{
		ID:    "user-1",
		Email: "test@example.com",
		Name:  "Test User",
	}

	user2 := User{
		ID:    "user-1",
		Email: "different@example.com",
		Name:  "Test User",
	}

	result := CompareAggregates(user1, user2, nil)
	if result.Equal {
		t.Error("Expected users to be different")
	}

	if result.Diff == "" {
		t.Error("Expected diff to be populated")
	}
}

func TestCompareAggregates_ExcludeFields(t *testing.T) {
	now := time.Now()

	user1 := User{
		ID:        "user-1",
		Email:     "test@example.com",
		Name:      "Test User",
		CreatedAt: now,
		UpdatedAt: now,
	}

	user2 := User{
		ID:        "user-2", // Different ID
		Email:     "test@example.com",
		Name:      "Test User",
		CreatedAt: now.Add(time.Hour),    // Different timestamp
		UpdatedAt: now.Add(2 * time.Hour), // Different timestamp
	}

	// Exclude ID and timestamp fields
	result := CompareAggregates(user1, user2, []string{"ID", "CreatedAt", "UpdatedAt"})
	if !result.Equal {
		t.Errorf("Expected users to be equal after excluding fields, got diff:\n%s", result.Diff)
	}
}

func TestCompareAggregates_ExcludeNestedFields(t *testing.T) {
	user1 := User{
		ID:    "user-1",
		Email: "test@example.com",
		Name:  "Test User",
		Profile: Profile{
			Bio:       "Software Engineer",
			AvatarURL: "https://example.com/avatar1.jpg",
			Location:  "San Francisco",
		},
	}

	user2 := User{
		ID:    "user-1",
		Email: "test@example.com",
		Name:  "Test User",
		Profile: Profile{
			Bio:       "Software Engineer",
			AvatarURL: "https://example.com/avatar2.jpg", // Different avatar
			Location:  "San Francisco",
		},
	}

	// Exclude nested field
	result := CompareAggregates(user1, user2, []string{"Profile.AvatarURL"})
	if !result.Equal {
		t.Errorf("Expected users to be equal after excluding Profile.AvatarURL, got diff:\n%s", result.Diff)
	}
}

func TestCompareAggregates_ExcludeWildcard(t *testing.T) {
	now := time.Now()

	user1 := User{
		ID:    "user-1",
		Email: "test@example.com",
		Name:  "Test User",
		Metadata: Metadata{
			Version:   1,
			Timestamp: now,
			Tags:      []string{"active", "verified"},
		},
	}

	user2 := User{
		ID:    "user-1",
		Email: "test@example.com",
		Name:  "Test User",
		Metadata: Metadata{
			Version:   2,                   // Different version
			Timestamp: now.Add(time.Hour),  // Different timestamp
			Tags:      []string{"premium"}, // Different tags
		},
	}

	// Exclude all metadata fields using wildcard
	result := CompareAggregates(user1, user2, []string{"Metadata.*"})
	if !result.Equal {
		t.Errorf("Expected users to be equal after excluding Metadata.*, got diff:\n%s", result.Diff)
	}
}

func TestCompareAggregatesWithOptions_IgnoreFields(t *testing.T) {
	now := time.Now()

	user1 := User{
		ID:        "user-1",
		Email:     "test@example.com",
		Name:      "Test User",
		CreatedAt: now,
		UpdatedAt: now,
	}

	user2 := User{
		ID:        "user-2",
		Email:     "test@example.com",
		Name:      "Test User",
		CreatedAt: now.Add(time.Hour),
		UpdatedAt: now.Add(2 * time.Hour),
	}

	// Use IgnoreFields helper
	result := CompareAggregatesWithOptions(
		user1, user2,
		IgnoreFields(User{}, "ID", "CreatedAt", "UpdatedAt"),
	)

	if !result.Equal {
		t.Errorf("Expected users to be equal after ignoring fields, got diff:\n%s", result.Diff)
	}
}

func TestCompareAggregatesWithOptions_EquateEmpty(t *testing.T) {
	user1 := User{
		ID:    "user-1",
		Email: "test@example.com",
		Metadata: Metadata{
			Tags: nil, // nil slice
		},
	}

	user2 := User{
		ID:    "user-1",
		Email: "test@example.com",
		Metadata: Metadata{
			Tags: []string{}, // empty slice
		},
	}

	// Without EquateEmpty, these are different
	result := CompareAggregates(user1, user2, nil)
	if result.Equal {
		t.Error("Expected users to be different (nil vs empty slice)")
	}

	// With EquateEmpty, they are equal
	result = CompareAggregatesWithOptions(user1, user2, EquateEmpty())
	if !result.Equal {
		t.Errorf("Expected users to be equal with EquateEmpty, got diff:\n%s", result.Diff)
	}
}

func TestFormatDiff(t *testing.T) {
	user1 := User{ID: "1", Email: "test1@example.com"}
	user2 := User{ID: "1", Email: "test2@example.com"}

	result := CompareAggregates(user1, user2, nil)

	formatted := FormatDiff(result)
	if formatted == "" {
		t.Error("Expected formatted diff to be non-empty")
	}

	// Check for equal case
	result = CompareAggregates(user1, user1, nil)
	formatted = FormatDiff(result)
	if formatted != "✓ Aggregates are equal" {
		t.Errorf("Expected equal message, got: %s", formatted)
	}
}

func TestCompareAggregates_NestedStructs(t *testing.T) {
	user1 := User{
		ID:    "user-1",
		Email: "test@example.com",
		Profile: Profile{
			Bio:      "Engineer",
			Location: "SF",
		},
		Metadata: Metadata{
			Version: 1,
			Tags:    []string{"tag1", "tag2"},
		},
	}

	user2 := User{
		ID:    "user-1",
		Email: "test@example.com",
		Profile: Profile{
			Bio:      "Engineer",
			Location: "SF",
		},
		Metadata: Metadata{
			Version: 2, // Different
			Tags:    []string{"tag1", "tag2"},
		},
	}

	// Exclude just the version field
	result := CompareAggregates(user1, user2, []string{"Metadata.Version"})
	if !result.Equal {
		t.Errorf("Expected users to be equal after excluding Metadata.Version, got diff:\n%s", result.Diff)
	}
}

// Benchmark tests
func BenchmarkCompareAggregates_Simple(b *testing.B) {
	user1 := User{ID: "user-1", Email: "test@example.com", Name: "Test"}
	user2 := User{ID: "user-1", Email: "test@example.com", Name: "Test"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CompareAggregates(user1, user2, nil)
	}
}

func BenchmarkCompareAggregates_WithExclusions(b *testing.B) {
	now := time.Now()
	user1 := User{
		ID:        "user-1",
		Email:     "test@example.com",
		CreatedAt: now,
		UpdatedAt: now,
	}
	user2 := user1

	excludePaths := []string{"ID", "CreatedAt", "UpdatedAt"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CompareAggregates(user1, user2, excludePaths)
	}
}

func BenchmarkCompareAggregates_Nested(b *testing.B) {
	user1 := User{
		ID:    "user-1",
		Email: "test@example.com",
		Profile: Profile{
			Bio:       "Engineer",
			AvatarURL: "https://example.com/avatar.jpg",
			Location:  "SF",
		},
		Metadata: Metadata{
			Version: 1,
			Tags:    []string{"tag1", "tag2", "tag3"},
		},
	}
	user2 := user1

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CompareAggregates(user1, user2, nil)
	}
}
