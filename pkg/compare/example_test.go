package compare_test

import (
	"fmt"
	"time"

	"github.com/plaenen/steward-control/pkg/compare"
)

// Example types
type Account struct {
	ID        string
	Email     string
	Name      string
	Balance   int64
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

func Example_basicComparison() {
	account1 := Account{
		ID:    "acc-1",
		Email: "user@example.com",
		Name:  "John Doe",
	}

	account2 := Account{
		ID:    "acc-1",
		Email: "user@example.com",
		Name:  "John Doe",
	}

	result := compare.CompareAggregates(account1, account2, nil)
	fmt.Println(compare.FormatDiff(result))
	// Output: ✓ Aggregates are equal
}

func Example_excludeFields() {
	now := time.Now()

	account1 := Account{
		ID:        "acc-1",
		Email:     "user@example.com",
		CreatedAt: now,
		UpdatedAt: now,
	}

	account2 := Account{
		ID:        "acc-2", // Different ID
		Email:     "user@example.com",
		CreatedAt: now.Add(time.Hour), // Different timestamp
		UpdatedAt: now.Add(time.Hour), // Different timestamp
	}

	// Compare, but ignore ID and timestamps
	result := compare.CompareAggregates(account1, account2, []string{
		"ID",
		"CreatedAt",
		"UpdatedAt",
	})

	fmt.Println(compare.FormatDiff(result))
	// Output: ✓ Aggregates are equal
}

func Example_excludeNestedFields() {
	account1 := Account{
		ID:    "acc-1",
		Email: "user@example.com",
		Profile: Profile{
			Bio:       "Software Engineer",
			AvatarURL: "https://example.com/avatar1.jpg",
			Location:  "San Francisco",
		},
	}

	account2 := Account{
		ID:    "acc-1",
		Email: "user@example.com",
		Profile: Profile{
			Bio:       "Software Engineer",
			AvatarURL: "https://example.com/avatar2.jpg", // Different
			Location:  "San Francisco",
		},
	}

	// Exclude nested field
	result := compare.CompareAggregates(account1, account2, []string{
		"Profile.AvatarURL",
	})

	fmt.Println(compare.FormatDiff(result))
	// Output: ✓ Aggregates are equal
}

func Example_wildcardExclusion() {
	now := time.Now()

	account1 := Account{
		ID:    "acc-1",
		Email: "user@example.com",
		Metadata: Metadata{
			Version:   1,
			Timestamp: now,
			Tags:      []string{"active"},
		},
	}

	account2 := Account{
		ID:    "acc-1",
		Email: "user@example.com",
		Metadata: Metadata{
			Version:   2,                   // Different
			Timestamp: now.Add(time.Hour),  // Different
			Tags:      []string{"premium"}, // Different
		},
	}

	// Exclude all metadata fields using wildcard
	result := compare.CompareAggregates(account1, account2, []string{
		"Metadata.*",
	})

	fmt.Println(compare.FormatDiff(result))
	// Output: ✓ Aggregates are equal
}

func Example_eventSourcingVerification() {
	// Simulate rebuilding an aggregate from events
	original := Account{
		ID:      "acc-1",
		Email:   "user@example.com",
		Name:    "John Doe",
		Balance: 10000,
	}

	// After rebuilding from events
	rebuilt := Account{
		ID:      "acc-1",
		Email:   "user@example.com",
		Name:    "John Doe",
		Balance: 10000,
	}

	// Compare, excluding runtime-generated fields
	result := compare.CompareAggregates(original, rebuilt, []string{
		"CreatedAt",
		"UpdatedAt",
		"Metadata.Timestamp",
	})

	if result.Equal {
		fmt.Println("✓ Aggregate rebuild successful")
	} else {
		fmt.Printf("✗ Rebuild failed:\n%s\n", result.Diff)
	}
	// Output: ✓ Aggregate rebuild successful
}

func Example_withCustomOptions() {
	account1 := Account{
		ID:       "acc-1",
		Email:    "user@example.com",
		Metadata: Metadata{Tags: nil},
	}

	account2 := Account{
		ID:       "acc-1",
		Email:    "user@example.com",
		Metadata: Metadata{Tags: []string{}}, // Empty slice vs nil
	}

	// Without EquateEmpty, these would be different
	result := compare.CompareAggregatesWithOptions(
		account1, account2,
		compare.EquateEmpty(), // Treat nil == empty
	)

	fmt.Println(compare.FormatDiff(result))
	// Output: ✓ Aggregates are equal
}

func Example_multipleOptions() {
	now := time.Now()

	account1 := Account{
		ID:        "acc-1",
		Email:     "user@example.com",
		Balance:   10000,
		CreatedAt: now,
		UpdatedAt: now,
	}

	account2 := Account{
		ID:        "acc-2", // Different
		Email:     "user@example.com",
		Balance:   10000,
		CreatedAt: now.Add(time.Hour), // Different
		UpdatedAt: now.Add(time.Hour), // Different
	}

	// Use multiple options
	result := compare.CompareAggregatesWithOptions(
		account1, account2,
		compare.IgnoreFields(Account{}, "ID", "CreatedAt", "UpdatedAt"),
		compare.EquateEmpty(),
	)

	fmt.Println(compare.FormatDiff(result))
	// Output: ✓ Aggregates are equal
}
