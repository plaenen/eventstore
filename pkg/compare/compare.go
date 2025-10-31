package compare

import (
	"fmt"
	"strings"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

// ComparisonResult contains the result of comparing two values
type ComparisonResult struct {
	Equal bool
	Diff  string
}

// CompareAggregates deeply compares two aggregate states, excluding specified paths
//
// This function uses Google's go-cmp library for robust, battle-tested comparison.
//
// Parameters:
//   - expected: The expected aggregate state
//   - actual: The actual aggregate state to compare against
//   - excludePaths: List of paths to exclude from comparison (e.g., "Metadata.UpdatedAt", "ID")
//
// Returns:
//   - ComparisonResult with Equal=true if states match, false otherwise
//   - Diff contains a human-readable diff if not equal
//
// Example:
//
//	result := CompareAggregates(agg1.State(), agg2.State(), []string{"Metadata.Timestamp", "Version"})
//	if !result.Equal {
//	    fmt.Println(result.Diff)
//	}
//
// Path Format:
//   - Use dot notation for nested fields: "User.Profile.Email"
//   - Use wildcards for all fields in a struct: "Metadata.*"
//   - Use array notation for slices: "Items[0].Name"
func CompareAggregates(expected, actual any, excludePaths []string) *ComparisonResult {
	// Build comparison options
	opts := buildCompareOptions(excludePaths)

	// Perform comparison
	diff := cmp.Diff(expected, actual, opts...)

	return &ComparisonResult{
		Equal: diff == "",
		Diff:  diff,
	}
}

// CompareAggregatesWithOptions compares two aggregates with custom cmp.Options
//
// This allows full control over the comparison behavior using go-cmp options.
//
// Example:
//
//	opts := []cmp.Option{
//	    cmpopts.IgnoreFields(User{}, "ID", "CreatedAt"),
//	    cmpopts.EquateApproxTime(time.Second),
//	}
//	result := CompareAggregatesWithOptions(expected, actual, opts...)
func CompareAggregatesWithOptions(expected, actual any, opts ...cmp.Option) *ComparisonResult {
	diff := cmp.Diff(expected, actual, opts...)

	return &ComparisonResult{
		Equal: diff == "",
		Diff:  diff,
	}
}

// buildCompareOptions creates cmp.Options from exclude paths
func buildCompareOptions(excludePaths []string) []cmp.Option {
	if len(excludePaths) == 0 {
		return nil
	}

	// Group paths by struct type for more efficient filtering
	// For now, use a simple approach with FilterPath
	opts := make([]cmp.Option, 0, len(excludePaths))

	for _, path := range excludePaths {
		// Handle wildcard patterns
		if strings.HasSuffix(path, ".*") {
			// Ignore all fields under this path
			prefix := strings.TrimSuffix(path, ".*")
			opts = append(opts, cmp.FilterPath(func(p cmp.Path) bool {
				return pathStartsWith(p.String(), prefix)
			}, cmp.Ignore()))
		} else {
			// Exact path match
			opts = append(opts, cmp.FilterPath(func(p cmp.Path) bool {
				return p.String() == path
			}, cmp.Ignore()))
		}
	}

	return opts
}

// pathStartsWith checks if a cmp.Path string starts with a prefix
func pathStartsWith(path, prefix string) bool {
	if path == prefix {
		return true
	}
	return strings.HasPrefix(path, prefix+".")
}

// FormatDiff returns a formatted comparison result
func FormatDiff(result *ComparisonResult) string {
	if result.Equal {
		return "✓ Aggregates are equal"
	}
	return fmt.Sprintf("✗ Aggregates differ:\n%s", result.Diff)
}

// Common comparison options that can be reused

// IgnoreUnexportedFields returns an option that ignores all unexported fields
// in the given struct types.
//
// Example:
//
//	result := CompareAggregatesWithOptions(
//	    expected, actual,
//	    IgnoreUnexportedFields(User{}, Profile{}),
//	)
func IgnoreUnexportedFields(types ...any) cmp.Option {
	return cmpopts.IgnoreUnexported(types...)
}

// IgnoreFields returns an option that ignores specific fields in a struct type.
//
// Example:
//
//	result := CompareAggregatesWithOptions(
//	    expected, actual,
//	    IgnoreFields(User{}, "ID", "CreatedAt", "UpdatedAt"),
//	)
func IgnoreFields(structType any, fields ...string) cmp.Option {
	return cmpopts.IgnoreFields(structType, fields...)
}

// IgnoreTypes ignores all values of the specified types during comparison.
//
// Example:
//
//	result := CompareAggregatesWithOptions(
//	    expected, actual,
//	    IgnoreTypes(time.Time{}), // Ignore all time.Time fields
//	)
func IgnoreTypes(types ...any) cmp.Option {
	return cmpopts.IgnoreTypes(types...)
}

// SortSlices returns an option that sorts slices before comparison.
// Useful when slice order doesn't matter.
//
// Example:
//
//	result := CompareAggregatesWithOptions(
//	    expected, actual,
//	    SortSlices(func(a, b string) bool { return a < b }),
//	)
func SortSlices(less any) cmp.Option {
	return cmpopts.SortSlices(less)
}

// EquateEmpty returns an option that considers nil and empty containers as equal.
//
// Example:
//
//	result := CompareAggregatesWithOptions(
//	    expected, actual,
//	    EquateEmpty(), // nil slice == empty slice
//	)
func EquateEmpty() cmp.Option {
	return cmpopts.EquateEmpty()
}
