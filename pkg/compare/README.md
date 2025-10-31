# Compare Package

A battle-tested aggregate state comparison utility built on Google's `go-cmp` library.

## Features

- Deep comparison of nested structs
- Path-based field exclusion with wildcard support
- Human-readable diff output
- Comprehensive comparison options (ignore fields, types, sort slices, etc.)
- High performance with minimal allocations

## Installation

```bash
go get github.com/google/go-cmp/cmp
```

## Usage

### Basic Comparison

```go
import "github.com/plaenen/steward-control/pkg/compare"

user1 := User{ID: "1", Email: "test@example.com"}
user2 := User{ID: "1", Email: "test@example.com"}

result := compare.CompareAggregates(user1, user2, nil)
if result.Equal {
    fmt.Println("Users are equal")
} else {
    fmt.Println(result.Diff)
}
```

### Excluding Fields

```go
// Exclude specific fields from comparison
result := compare.CompareAggregates(user1, user2, []string{
    "ID",           // Exact field
    "CreatedAt",    // Timestamp field
    "UpdatedAt",    // Another timestamp
})
```

### Excluding Nested Fields

```go
// Exclude nested fields using dot notation
result := compare.CompareAggregates(user1, user2, []string{
    "Profile.AvatarURL",  // Nested field
    "Metadata.Timestamp", // Another nested field
})
```

### Wildcard Exclusions

```go
// Exclude all fields under a path
result := compare.CompareAggregates(user1, user2, []string{
    "Metadata.*", // Exclude all metadata fields
})
```

### Advanced Options

```go
// Use custom comparison options
result := compare.CompareAggregatesWithOptions(
    user1, user2,
    compare.IgnoreFields(User{}, "ID", "CreatedAt"),
    compare.EquateEmpty(), // nil == empty slice
)
```

### Available Options

- **`IgnoreFields(type, fields...)`** - Ignore specific fields in a struct
- **`IgnoreUnexportedFields(types...)`** - Ignore all unexported fields
- **`IgnoreTypes(types...)`** - Ignore all values of specific types
- **`SortSlices(lessFunc)`** - Sort slices before comparison
- **`EquateEmpty()`** - Treat nil and empty containers as equal

### Formatting Output

```go
result := compare.CompareAggregates(expected, actual, nil)
fmt.Println(compare.FormatDiff(result))
// Output:
// ✓ Aggregates are equal
// or
// ✗ Aggregates differ:
// <detailed diff>
```

## Performance

Benchmarks on Apple M1 Max:

```
BenchmarkCompareAggregates_Simple-10         226003    5340 ns/op    2045 B/op    32 allocs/op
BenchmarkCompareAggregates_WithExclusions-10 138656    9218 ns/op    4218 B/op   161 allocs/op
BenchmarkCompareAggregates_Nested-10         156056    7312 ns/op    3392 B/op    38 allocs/op
```

- Simple comparison: ~5.3μs per operation
- With exclusions: ~9.2μs per operation
- Nested structs: ~7.3μs per operation

## Use Cases

### Event Sourcing Verification

Verify that rebuilding an aggregate from events produces the same state:

```go
// Rebuild aggregate from events
rebuilt := RebuildFromEvents(events)

// Compare with expected state, excluding timestamps
result := compare.CompareAggregates(
    expected, rebuilt,
    []string{"Metadata.Timestamp", "Version"},
)

if !result.Equal {
    log.Error("Aggregate rebuild failed", "diff", result.Diff)
}
```

### Testing

Compare expected vs actual states in tests:

```go
func TestUserCreation(t *testing.T) {
    user := CreateUser("test@example.com")

    expected := User{
        Email: "test@example.com",
        Status: "active",
    }

    result := compare.CompareAggregates(
        expected, user,
        []string{"ID", "CreatedAt"}, // Ignore generated fields
    )

    if !result.Equal {
        t.Errorf("User creation failed:\n%s", result.Diff)
    }
}
```

### Migration Validation

Verify data migrations preserve important state:

```go
original := LoadFromOldDB(id)
migrated := LoadFromNewDB(id)

result := compare.CompareAggregates(
    original, migrated,
    []string{"Metadata.*"}, // Ignore metadata changes
)

if !result.Equal {
    return fmt.Errorf("migration validation failed: %s", result.Diff)
}
```

## Path Format

- **Exact field**: `"FieldName"`
- **Nested field**: `"Parent.Child.Field"`
- **Wildcard**: `"Parent.*"` (matches all fields under Parent)
- **Array element**: Automatically handled by go-cmp

## Comparison with reflect.DeepEqual

| Feature | `compare.CompareAggregates` | `reflect.DeepEqual` |
|---------|---------------------------|-------------------|
| Exclude fields | ✅ Yes | ❌ No |
| Detailed diff | ✅ Yes | ❌ No |
| Wildcard paths | ✅ Yes | ❌ No |
| Custom options | ✅ Yes | ❌ No |
| Performance | ✅ Fast | ✅ Fast |
| Battle-tested | ✅ Google's go-cmp | ✅ Go stdlib |

## Why go-cmp?

This package uses Google's `go-cmp` library because it:

- Is widely used and battle-tested across the Go ecosystem
- Provides detailed, human-readable diffs
- Supports extensive customization options
- Has excellent performance characteristics
- Is actively maintained by Google
- Supports protocol buffers and other complex types

## Contributing

When adding new comparison features, ensure:

1. Tests cover the new functionality
2. Benchmarks demonstrate performance impact
3. Documentation includes usage examples
4. The API remains simple and intuitive

## License

MIT
