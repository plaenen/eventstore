# Event Analytics Guide

Event analytics provides automatic tracking of which events have been applied to aggregates, how many times, and when. This is invaluable for debugging, understanding aggregate lifecycles, and gaining insights into event distribution.

## Quick Start

```go
// Load an aggregate
user, err := userRepo.Load("user-123")
if err != nil {
    log.Fatal(err)
}

// Get analytics
analytics := user.Analytics()

fmt.Printf("Total events: %d\n", analytics.TotalEvents)
fmt.Printf("Event types: %v\n", analytics.GetEventTypes())
fmt.Printf("UserCreated count: %d\n", analytics.GetCount("UserCreated"))

// Get detailed stats for an event type
stats := analytics.GetStats("EmailVerified")
if stats != nil {
    fmt.Printf("First applied: %s\n", stats.FirstApplied)
    fmt.Printf("Last applied: %s\n", stats.LastApplied)
    fmt.Printf("Times applied: %d\n", stats.Count)
}
```

## Features

### Automatic Tracking

Analytics are automatically tracked when events are loaded from history:

```go
agg := domain.NewAggregateRoot("order-456", "Order")

events := []*domain.Event{
    // ... events loaded from event store
}

// Analytics are automatically populated during LoadFromHistory
err := agg.LoadFromHistory(events)

// Now you can query analytics
analytics := agg.Analytics()
```

### Persisted in Snapshots

Analytics are automatically included in snapshots, so you don't lose this information:

```go
// When creating a snapshot
snapshot := &store.Snapshot{
    AggregateID:   agg.ID(),
    AggregateType: agg.Type(),
    Version:       agg.Version(),
    Data:          serializedState,
    CreatedAt:     time.Now(),
    Metadata: &store.SnapshotMetadata{
        EventCount: agg.Version(),
    },
}

// Add analytics to snapshot metadata
err := snapshot.Metadata.SetAnalyticsFromAggregate(agg.Analytics())

// When restoring from snapshot
restoredAnalytics, err := snapshot.Metadata.GetAnalytics()
```

## API Reference

### EventAnalytics

#### Basic Queries

```go
// Get total number of events applied
total := analytics.TotalEvents

// Get list of all event types
eventTypes := analytics.GetEventTypes()

// Get count for specific event type
count := analytics.GetCount("OrderPlaced")
```

#### Detailed Statistics

```go
// Get full stats for an event type
stats := analytics.GetStats("UserCreated")
if stats != nil {
    fmt.Printf("Event Type: %s\n", stats.EventType)
    fmt.Printf("Count: %d\n", stats.Count)
    fmt.Printf("First Applied: %s\n", stats.FirstApplied)
    fmt.Printf("Last Applied: %s\n", stats.LastApplied)
}
```

#### Frequency Analysis

```go
// Get most frequently applied event type
mostFrequent := analytics.GetMostFrequent()
fmt.Printf("Most frequent: %s\n", mostFrequent)

// Get least frequently applied event type
leastFrequent := analytics.GetLeastFrequent()
fmt.Printf("Least frequent: %s\n", leastFrequent)
```

#### Distribution Analysis

```go
// Get percentage distribution of event types
distribution := analytics.GetDistribution()

for eventType, percentage := range distribution {
    fmt.Printf("%s: %.2f%%\n", eventType, percentage)
}

// Example output:
// OrderPlaced: 40.00%
// OrderShipped: 35.00%
// OrderDelivered: 25.00%
```

### AggregateRoot Methods

```go
// Get analytics (automatically initialized)
analytics := agg.Analytics()

// Set analytics (used when restoring from snapshot)
agg.SetAnalytics(restoredAnalytics)

// Reset analytics to empty state
agg.ResetAnalytics()
```

## Use Cases

### 1. Debugging Production Issues

```go
// When investigating a bug, check what events were applied
user, _ := userRepo.Load("problematic-user-id")
analytics := user.Analytics()

fmt.Println("Event history summary:")
for _, eventType := range analytics.GetEventTypes() {
    stats := analytics.GetStats(eventType)
    fmt.Printf("  %s: %d times (first: %s, last: %s)\n",
        eventType,
        stats.Count,
        stats.FirstApplied.Format("2006-01-02 15:04"),
        stats.LastApplied.Format("2006-01-02 15:04"))
}

// Example output:
// Event history summary:
//   UserCreated: 1 times (first: 2025-01-01 10:00, last: 2025-01-01 10:00)
//   EmailVerified: 1 times (first: 2025-01-01 10:15, last: 2025-01-01 10:15)
//   PasswordChanged: 3 times (first: 2025-01-05 14:30, last: 2025-01-20 09:45)
```

### 2. Understanding Aggregate Lifecycle

```go
// Analyze the typical lifecycle of an order
order, _ := orderRepo.Load("order-789")
analytics := order.Analytics()

// Check if order followed expected flow
expectedFlow := []string{"OrderPlaced", "PaymentReceived", "OrderShipped", "OrderDelivered"}
for _, eventType := range expectedFlow {
    if analytics.GetCount(eventType) == 0 {
        fmt.Printf("WARNING: Expected event %s was never applied!\n", eventType)
    }
}

// Check for unusual patterns
if analytics.GetCount("PaymentFailed") > 2 {
    fmt.Println("WARNING: Multiple payment failures detected")
}
```

### 3. Performance Optimization

```go
// Identify aggregates with excessive events
func findHeavyAggregates(repo *store.BaseRepository[*Order], ids []string) {
    for _, id := range ids {
        order, err := repo.Load(id)
        if err != nil {
            continue
        }

        analytics := order.Analytics()
        if analytics.TotalEvents > 1000 {
            fmt.Printf("Order %s has %d events - consider snapshotting\n",
                id, analytics.TotalEvents)

            // Show distribution to understand what's causing the high count
            distribution := analytics.GetDistribution()
            for eventType, pct := range distribution {
                if pct > 50.0 {
                    fmt.Printf("  Dominated by %s: %.1f%%\n", eventType, pct)
                }
            }
        }
    }
}
```

### 4. Analytics Dashboard

```go
// Build analytics for reporting
type AggregateAnalytics struct {
    AggregateID   string
    TotalEvents   int64
    EventTypes    []string
    MostFrequent  string
    FirstActivity time.Time
    LastActivity  time.Time
}

func getAggregateAnalytics(repo *store.BaseRepository[*User], id string) (*AggregateAnalytics, error) {
    user, err := repo.Load(id)
    if err != nil {
        return nil, err
    }

    analytics := user.Analytics()
    eventTypes := analytics.GetEventTypes()

    var firstActivity, lastActivity time.Time
    for _, eventType := range eventTypes {
        stats := analytics.GetStats(eventType)
        if firstActivity.IsZero() || stats.FirstApplied.Before(firstActivity) {
            firstActivity = stats.FirstApplied
        }
        if stats.LastApplied.After(lastActivity) {
            lastActivity = stats.LastApplied
        }
    }

    return &AggregateAnalytics{
        AggregateID:   id,
        TotalEvents:   analytics.TotalEvents,
        EventTypes:    eventTypes,
        MostFrequent:  analytics.GetMostFrequent(),
        FirstActivity: firstActivity,
        LastActivity:  lastActivity,
    }, nil
}
```

### 5. Testing Assertions

```go
func TestUserRegistrationFlow(t *testing.T) {
    user := NewUser("test-user")
    user.Register("test@example.com", "password")
    user.VerifyEmail()

    // Simulate saving and loading
    events := user.UncommittedEvents()
    user.ClearUncommittedEvents()

    reloaded := NewUser("test-user")
    reloaded.LoadFromHistory(events)

    // Assert expected events were applied
    analytics := reloaded.Analytics()
    assert.Equal(t, int64(2), analytics.TotalEvents)
    assert.Equal(t, int64(1), analytics.GetCount("UserRegistered"))
    assert.Equal(t, int64(1), analytics.GetCount("EmailVerified"))

    // Assert event order
    regStats := analytics.GetStats("UserRegistered")
    verifyStats := analytics.GetStats("EmailVerified")
    assert.True(t, regStats.FirstApplied.Before(verifyStats.FirstApplied),
        "Registration should happen before email verification")
}
```

### 6. Snapshot Strategy Decisions

```go
// Decide whether to snapshot based on event distribution
func shouldSnapshot(agg domain.Aggregate) bool {
    analytics := agg.Analytics()

    // Snapshot if aggregate has many events
    if analytics.TotalEvents > 500 {
        return true
    }

    // Snapshot if dominated by one event type (sign of repeated updates)
    distribution := analytics.GetDistribution()
    for _, pct := range distribution {
        if pct > 70.0 {
            // One event type accounts for >70% of events
            return true
        }
    }

    return false
}
```

### 7. Event Pattern Detection

```go
// Detect suspicious patterns
func detectAnomalies(analytics *domain.EventAnalytics) []string {
    var anomalies []string

    // Check for password changes too frequently
    if count := analytics.GetCount("PasswordChanged"); count > 10 {
        anomalies = append(anomalies, fmt.Sprintf(
            "Excessive password changes: %d times", count))
    }

    // Check for failed login attempts
    if count := analytics.GetCount("LoginFailed"); count > 5 {
        stats := analytics.GetStats("LoginFailed")
        anomalies = append(anomalies, fmt.Sprintf(
            "Multiple login failures: %d times (last: %s)",
            count, stats.LastApplied))
    }

    // Check for account locked without subsequent unlock
    if analytics.GetCount("AccountLocked") > 0 &&
       analytics.GetCount("AccountUnlocked") == 0 {
        anomalies = append(anomalies, "Account is locked")
    }

    return anomalies
}
```

## Advanced Usage

### Merging Analytics

```go
// Combine analytics from multiple sources
func combineAnalytics(aggregates []domain.Aggregate) *domain.EventAnalytics {
    combined := domain.NewEventAnalytics()

    for _, agg := range aggregates {
        combined.Merge(agg.Analytics())
    }

    return combined
}

// Example: Analyze all orders from a customer
func analyzeCustomerOrders(orderRepo *store.BaseRepository[*Order], customerID string) {
    orders := loadOrdersForCustomer(orderRepo, customerID)
    combined := combineAnalytics(orders)

    fmt.Printf("Customer %s order analytics:\n", customerID)
    fmt.Printf("Total events across all orders: %d\n", combined.TotalEvents)
    fmt.Printf("Event distribution:\n")

    for eventType, pct := range combined.GetDistribution() {
        fmt.Printf("  %s: %.1f%%\n", eventType, pct)
    }
}
```

### Cloning Analytics

```go
// Clone analytics for comparison
func compareAnalyticsBeforeAfter(agg domain.Aggregate, operation func(domain.Aggregate)) {
    before := agg.Analytics().Clone()

    operation(agg)

    after := agg.Analytics()

    // Compare
    fmt.Printf("Events added: %d\n", after.TotalEvents - before.TotalEvents)

    for _, eventType := range after.GetEventTypes() {
        beforeCount := before.GetCount(eventType)
        afterCount := after.GetCount(eventType)
        if afterCount > beforeCount {
            fmt.Printf("  %s: +%d\n", eventType, afterCount-beforeCount)
        }
    }
}
```

### Custom Analytics Serialization

```go
// Export analytics to JSON for external systems
func exportAnalytics(agg domain.Aggregate) ([]byte, error) {
    analytics := agg.Analytics()
    return json.MarshalIndent(analytics, "", "  ")
}

// Import analytics from JSON
func importAnalytics(data []byte) (*domain.EventAnalytics, error) {
    var analytics domain.EventAnalytics
    err := json.Unmarshal(data, &analytics)
    if err != nil {
        return nil, err
    }
    return &analytics, nil
}
```

## Performance Considerations

### Memory Usage

Analytics tracking adds minimal overhead:
- ~40 bytes per unique event type
- ~16 bytes per event tracked (8 bytes count + 8 bytes timestamp)
- For an aggregate with 1000 events of 10 different types: ~500 bytes

### CPU Overhead

- Recording an event: O(1) map lookup and update
- Negligible impact on event replay performance

### Snapshot Size

Analytics add ~100-500 bytes to snapshot metadata depending on number of unique event types.

## Best Practices

### 1. Use Analytics for Insights, Not Business Logic

❌ **Bad** - Using analytics in business logic:
```go
// Don't do this!
if agg.Analytics().GetCount("PasswordChanged") > 5 {
    return errors.New("too many password changes")
}
```

✅ **Good** - Use analytics for observability:
```go
// Use analytics for monitoring/alerting
if agg.Analytics().GetCount("PasswordChanged") > 10 {
    logger.Warn("Unusual number of password changes",
        "aggregateID", agg.ID(),
        "count", agg.Analytics().GetCount("PasswordChanged"))
}
```

### 2. Include Analytics in Snapshots

Always include analytics when creating snapshots:

```go
metadata := &store.SnapshotMetadata{
    EventCount: agg.Version(),
}

// Always include analytics
err := metadata.SetAnalyticsFromAggregate(agg.Analytics())
if err != nil {
    return err
}
```

### 3. Use Distribution for Optimization Decisions

```go
// Good pattern for identifying optimization opportunities
distribution := agg.Analytics().GetDistribution()
for eventType, pct := range distribution {
    if pct > 50.0 {
        logger.Info("Consider optimizing event type",
            "eventType", eventType,
            "percentage", pct)
    }
}
```

### 4. Log Analytics for Debugging

Include analytics in error logs:

```go
if err != nil {
    analytics := agg.Analytics()
    logger.Error("Operation failed",
        "error", err,
        "aggregateID", agg.ID(),
        "totalEvents", analytics.TotalEvents,
        "eventTypes", analytics.GetEventTypes(),
        "mostFrequent", analytics.GetMostFrequent())
    return err
}
```

## Limitations

1. **Analytics are not part of the event stream** - They are derived data computed during replay
2. **Clock skew** - FirstApplied/LastApplied depend on event timestamps being accurate
3. **No cross-aggregate analytics** - Each aggregate tracks its own events independently
4. **Reset on aggregate recreation** - Analytics start fresh if you create a new aggregate instance

## Snapshots and Analytics

Event analytics are automatically preserved in snapshots! See [SNAPSHOT_GUIDE.md](SNAPSHOT_GUIDE.md) for complete details on:

- How analytics are persisted in snapshot metadata
- How analytics are restored when loading from snapshots
- How analytics are updated when applying events after a snapshot
- Performance optimization with snapshots

**Quick example:**
```go
// Save snapshot with analytics
repo.SaveSnapshot(order)

// Load later - analytics are restored!
order, _ := repo.Load("order-123")
analytics := order.Analytics() // Complete analytics across all events
```

## See Also

- [SNAPSHOT_GUIDE.md](SNAPSHOT_GUIDE.md) - Snapshot + analytics integration
- [pkg/domain/analytics.go](/Users/pascallaenen/Documents/github/eventsourcing/pkg/domain/analytics.go) - Core analytics implementation
- [pkg/domain/aggregate.go:202](/Users/pascallaenen/Documents/github/eventsourcing/pkg/domain/aggregate.go:202) - Analytics methods on AggregateRoot
- [pkg/store/snapshot.go:115](/Users/pascallaenen/Documents/github/eventsourcing/pkg/store/snapshot.go:115) - Snapshot integration
- [pkg/store/repository.go:286](/Users/pascallaenen/Documents/github/eventsourcing/pkg/store/repository.go:286) - SaveSnapshot with analytics
