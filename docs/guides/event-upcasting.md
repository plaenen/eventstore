# Event Upcasting Guide

Event upcasting allows you to evolve your event schema over time while maintaining compatibility with historical events. This guide shows you how to handle schema changes in your event-sourced aggregates.

## When Do You Need Upcasting?

You need upcasting when you change your event schema in ways that aren't backward compatible:

- **Adding required fields** - New field that old events don't have
- **Renaming fields** - `initial_balance` → `opening_amount`
- **Changing types** - String → structured type
- **Restructuring data** - Combining or splitting fields

## Three Approaches (Simple to Complex)

### 1. Proto Field Evolution (Simplest)

**Use when:** Adding optional fields or deprecating old fields

**How it works:** Use protobuf field deprecation and handle both old and new fields in your code.

**Example:**

```protobuf
message AccountOpenedEvent {
  option (eventsourcing.event) = {aggregate_name: "Account"};
  
  string account_id = 1;
  string owner_name = 2;
  
  // Old field (deprecated but still read)
  string initial_balance = 3 [deprecated = true];
  
  // New field (preferred)
  string opening_amount = 4;
  string currency = 5;  // New optional field
  int64 created_at = 6; // New optional field
}
```

**Handle in your event applier:**

```go
func (a *AccountAggregate) ApplyAccountOpenedEvent(e *AccountOpenedEvent) error {
    a.AccountId = e.AccountId
    a.OwnerName = e.OwnerName
    
    // Handle both old and new fields
    if e.OpeningAmount != "" {
        a.Balance = e.OpeningAmount  // New events
    } else {
        a.Balance = e.InitialBalance // Old events
    }
    
    // New fields with defaults for old events
    if e.Currency != "" {
        a.Currency = e.Currency
    } else {
        a.Currency = "USD"  // Default for old events
    }
    
    a.Status = AccountStatus_ACCOUNT_STATUS_OPEN
    return nil
}
```

**Pros:**
- ✅ No special code needed
- ✅ Protobuf handles compatibility
- ✅ Simple to understand

**Cons:**
- ⚠️ Both fields present in new events (deprecated field stays)
- ⚠️ Limited to simple field additions

---

### 2. Aggregate-Level Upcasting (Recommended)

**Use when:** You need to transform old events into new format

**How it works:** Implement the `EventUpcaster` interface in your aggregate to transform events before applying them.

**Example - Old event structure:**

```protobuf
// Old version (V1)
message AccountOpenedEventV1 {
  string account_id = 1;
  string owner_name = 2;
  string initial_balance = 3;
  int64 timestamp = 4;
}
```

**New event structure:**

```protobuf
// New version (V2)
message AccountOpenedEvent {
  option (eventsourcing.event) = {aggregate_name: "Account"};
  
  string account_id = 1;
  string owner_name = 2;
  string opening_amount = 3;
  string currency = 4;
  int64 created_at = 5;
}
```

**Implement the upcaster:**

```go
package domain

import (
    accountv1 "github.com/your-org/your-app/gen/pb/account/v1"
    "google.golang.org/protobuf/proto"
)

// Wrap the generated aggregate
type Account struct {
    *accountv1.AccountAggregate
}

// Implement EventUpcaster interface
func (a *Account) UpcastEvent(event proto.Message) proto.Message {
    switch oldEvent := event.(type) {
    case *accountv1.AccountOpenedEventV1:
        // Transform V1 → V2
        return &accountv1.AccountOpenedEvent{
            AccountId:     oldEvent.AccountId,
            OwnerName:     oldEvent.OwnerName,
            OpeningAmount: oldEvent.InitialBalance,
            Currency:      "USD",  // Default for old events
            CreatedAt:     oldEvent.Timestamp,
        }
    }
    
    // Return unchanged if already new format
    return event
}
```

**Generated code automatically calls your upcaster:**

```go
// Generated in *_aggregate.es.pb.go
func (a *AccountAggregate) ApplyEvent(event proto.Message) error {
    // UPCAST HOOK: If aggregate implements EventUpcaster, upgrade old events
    if upcaster, ok := interface{}(a).(EventUpcaster); ok {
        event = upcaster.UpcastEvent(event)
    }
    
    switch e := event.(type) {
    case *AccountOpenedEvent:
        return a.ApplyAccountOpenedEvent(e)
    }
}
```

**Pros:**
- ✅ Clean separation (old events → new format)
- ✅ Event appliers only handle new format
- ✅ Type-safe transformations
- ✅ Testable

**Cons:**
- ⚠️ Must keep old event definitions in proto
- ⚠️ Need to implement upcaster interface

---

### 3. Snapshot Upcasting

**Use when:** Your aggregate state structure changes

**Example:**

```go
// Implement SnapshotUpcaster interface
func (a *Account) UpcastSnapshot(snapshot proto.Message) proto.Message {
    switch oldSnapshot := snapshot.(type) {
    case *accountv1.AccountV1:
        // Transform old snapshot structure to new
        return &accountv1.Account{
            AccountId: oldSnapshot.Id,  // Field renamed
            OwnerName: oldSnapshot.Owner,
            Balance:   oldSnapshot.CurrentBalance,
            Status:    accountv1.AccountStatus_ACCOUNT_STATUS_OPEN,
        }
    }
    
    return snapshot
}
```

**Generated code automatically calls your upcaster:**

```go
// Generated in *_aggregate.es.pb.go
func (a *AccountAggregate) UnmarshalSnapshot(data []byte) error {
    a.Account = &Account{}
    if err := proto.Unmarshal(data, a.Account); err != nil {
        return err
    }
    
    // UPCAST HOOK: If aggregate implements SnapshotUpcaster, upgrade old snapshots
    if upcaster, ok := interface{}(a).(SnapshotUpcaster); ok {
        a.Account = upcaster.UpcastSnapshot(a.Account).(*Account)
    }
    
    return nil
}
```

---

## Complete Example

Here's a complete example showing event evolution:

### 1. Define Old and New Event Types

```protobuf
// proto/account/v1/account.proto

// Old version (keep for backward compatibility)
message AccountOpenedEventV1 {
  string account_id = 1;
  string owner_name = 2;
  string initial_balance = 3;
  int64 timestamp = 4;
}

// New version
message AccountOpenedEvent {
  option (eventsourcing.event) = {aggregate_name: "Account"};
  
  string account_id = 1;
  string owner_name = 2;
  string opening_amount = 3;  // Renamed from initial_balance
  string currency = 4;          // New field
  int64 created_at = 5;         // Renamed from timestamp
}
```

### 2. Implement Upcaster

```go
// domain/account.go
package domain

import (
    accountv1 "github.com/your-org/your-app/gen/pb/account/v1"
    "google.golang.org/protobuf/proto"
)

type Account struct {
    *accountv1.AccountAggregate
}

// UpcastEvent transforms old events to new format
func (a *Account) UpcastEvent(event proto.Message) proto.Message {
    switch old := event.(type) {
    case *accountv1.AccountOpenedEventV1:
        return &accountv1.AccountOpenedEvent{
            AccountId:     old.AccountId,
            OwnerName:     old.OwnerName,
            OpeningAmount: old.InitialBalance,
            Currency:      "USD",  // Default
            CreatedAt:     old.Timestamp,
        }
    }
    return event
}
```

### 3. Use in Repository

```go
// main.go
package main

import (
    "github.com/plaenen/eventstore/pkg/store/sqlite"
    accountv1 "github.com/your-org/your-app/gen/pb/account/v1"
    "github.com/your-org/your-app/domain"
)

func main() {
    eventStore, _ := sqlite.NewEventStore(sqlite.WithDSN("events.db"))
    
    // Create repository with factory function
    repo := sqlite.NewRepository(
        eventStore,
        func(id string) *domain.Account {
            return &domain.Account{
                AccountAggregate: accountv1.NewAccount(id),
            }
        },
    )
    
    // Load aggregate - old events automatically upcasted!
    account, err := repo.Load(ctx, "acc-123")
    if err != nil {
        log.Fatal(err)
    }
    
    // All events are now in new format
    // Event appliers only handle AccountOpenedEvent (V2)
}
```

---

## Testing Upcasting

Always test your upcasters with old event data:

```go
func TestEventUpcasting(t *testing.T) {
    account := &domain.Account{
        AccountAggregate: accountv1.NewAccount("test-123"),
    }
    
    // Create old V1 event
    oldEvent := &accountv1.AccountOpenedEventV1{
        AccountId:      "test-123",
        OwnerName:      "Alice",
        InitialBalance: "1000.00",
        Timestamp:      1234567890,
    }
    
    // Upcast it
    newEvent := account.UpcastEvent(oldEvent)
    
    // Verify transformation
    v2Event, ok := newEvent.(*accountv1.AccountOpenedEvent)
    if !ok {
        t.Fatal("Expected AccountOpenedEvent")
    }
    
    assert.Equal(t, "test-123", v2Event.AccountId)
    assert.Equal(t, "Alice", v2Event.OwnerName)
    assert.Equal(t, "1000.00", v2Event.OpeningAmount)
    assert.Equal(t, "USD", v2Event.Currency)
    assert.Equal(t, int64(1234567890), v2Event.CreatedAt)
}
```

---

## Best Practices

### 1. Keep Old Event Definitions

Don't delete old event types from your proto files:

```protobuf
// ✅ Good: Keep V1 for backward compatibility
message AccountOpenedEventV1 { ... }
message AccountOpenedEvent { ... }

// ❌ Bad: Deleting V1 breaks existing events
// message AccountOpenedEvent { ... }  // Changed fields directly
```

### 2. Version Your Events Explicitly

Use clear naming for versions:

```protobuf
// ✅ Good: Explicit versions
message AccountOpenedEventV1 { ... }
message AccountOpenedEvent { ... }  // Current version

// ⚠️ Acceptable: Implicit current version
message AccountOpenedEventDeprecated { ... }
message AccountOpenedEvent { ... }
```

### 3. Test with Real Event Data

Export old events and test upcasting:

```go
func TestUpcastingWithRealData(t *testing.T) {
    // Load real V1 event from JSON/protobuf
    oldEventData := loadTestData("old_event_v1.json")
    
    oldEvent := &accountv1.AccountOpenedEventV1{}
    json.Unmarshal(oldEventData, oldEvent)
    
    // Test upcasting
    account := &domain.Account{...}
    newEvent := account.UpcastEvent(oldEvent)
    
    // Verify
    assert.NotNil(t, newEvent)
}
```

### 4. Document Changes

Add comments explaining the evolution:

```protobuf
message AccountOpenedEvent {
  option (eventsourcing.event) = {aggregate_name: "Account"};
  
  // V2 changes (2025-10-23):
  // - Renamed initial_balance → opening_amount
  // - Added currency field (defaults to USD for V1 events)
  // - Renamed timestamp → created_at
  
  string account_id = 1;
  string opening_amount = 3;
  string currency = 4;
  int64 created_at = 5;
}
```

---

## When NOT to Use Upcasting

**Don't use upcasting for:**

- ❌ **Business rule changes** - Create new events instead
- ❌ **Data corrections** - Use compensating events
- ❌ **Deleting sensitive data** - Use event encryption/deletion strategies

**Use new event types instead:**

```protobuf
// ✅ Good: New business rule = new event
message AccountOpenedEvent { ... }           // Old rule
message AccountOpenedWithLimitsEvent { ... } // New rule

// ❌ Bad: Changing business logic via upcasting
message AccountOpenedEvent {
  // Don't add business rule changes here
}
```

---

## Summary

| Approach | Use When | Complexity | Pros |
|----------|----------|------------|------|
| **Proto Evolution** | Adding optional fields | Low | Simple, no code changes |
| **Event Upcasting** | Renaming/restructuring | Medium | Clean, type-safe |
| **Snapshot Upcasting** | State structure changes | Medium | Handles snapshots |

**Start simple:**
1. Try proto field evolution first
2. Use event upcasting for renames/restructures
3. Add snapshot upcasting if state changes

**Remember:**
- Event upcasting runs during event replay
- Upcasters should be pure functions (no side effects)
- Test with real historical events
- Keep old event definitions in proto

For more details, see the [design document](../archive/aggregate_upcasting_design.md) in the archive.
