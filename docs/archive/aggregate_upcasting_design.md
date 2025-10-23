# Aggregate-Level Upcasting Design

## Philosophy

**Upcasting belongs in the aggregate** - The aggregate is the consistency boundary and should control its own evolution.

### Key Principles:

1. ✅ **Domain owns evolution** - Event/snapshot evolution is domain logic
2. ✅ **Simple by default** - Proto field evolution handles most cases
3. ✅ **Optional complexity** - Upcasters only when needed
4. ✅ **Testable** - Evolution logic tested with aggregate tests

---

## Design

### Optional Interfaces

Aggregates can optionally implement these interfaces:

```go
// EventUpcaster allows aggregate to upgrade old events to current version
type EventUpcaster interface {
    UpcastEvent(event proto.Message) proto.Message
}

// SnapshotUpcaster allows aggregate to upgrade old snapshots to current version
type SnapshotUpcaster interface {
    UpcastSnapshot(state proto.Message) proto.Message
}
```

### Generated Code (in aggregate.pb.go)

The generator will include upcast hooks in the `ApplyEvent` method:

```go
// ApplyEvent applies an event to the Account aggregate
// This method routes events to their specific applier methods
func (a *AccountAggregate) ApplyEvent(event proto.Message) error {
    // UPCAST HOOK: Call if aggregate implements EventUpcaster
    if upcaster, ok := interface{}(a).(EventUpcaster); ok {
        event = upcaster.UpcastEvent(event)
    }

    switch e := event.(type) {
    case *AccountOpenedEvent:
        return a.ApplyAccountOpenedEvent(e)
    case *MoneyDepositedEvent:
        return a.ApplyMoneyDepositedEvent(e)
    default:
        return fmt.Errorf("unknown event type: %T", event)
    }
}
```

And in `UnmarshalSnapshot`:

```go
// UnmarshalSnapshot deserializes the aggregate state from snapshots
func (a *AccountAggregate) UnmarshalSnapshot(data []byte) error {
    a.Account = &Account{}
    if err := proto.Unmarshal(data, a.Account); err != nil {
        return err
    }

    // UPCAST HOOK: Call if aggregate implements SnapshotUpcaster
    if upcaster, ok := interface{}(a).(SnapshotUpcaster); ok {
        a.Account = upcaster.UpcastSnapshot(a.Account).(*Account)
    }

    return nil
}
```

---

## Usage Patterns

### Pattern 1: Proto Field Evolution (No Code Needed)

**Best for**: Field renames, new optional fields

**Proto:**
```protobuf
message AccountOpenedEvent {
  option (eventsourcing.event) = {aggregate_name: "Account"};

  string account_id = 1;
  string owner_name = 2;

  // Field evolution
  string initial_balance = 3 [deprecated = true];  // Old
  string opening_amount = 4;                       // New

  // New optional field
  string currency = 5;  // Defaults to "" for old events

  int64 timestamp = 6;
}
```

**ApplyEvent** (handles both):
```go
func (a *AccountAggregate) ApplyAccountOpenedEvent(e *AccountOpenedEvent) error {
    a.AccountId = e.AccountId
    a.OwnerName = e.OwnerName

    // Handle old vs new field
    if e.OpeningAmount != "" {
        a.Balance = e.OpeningAmount
    } else {
        a.Balance = e.InitialBalance  // Fall back
    }

    // Handle new field with default
    if e.Currency != "" {
        a.Currency = e.Currency
    } else {
        a.Currency = "USD"  // Default for old events
    }

    return nil
}
```

**Benefits:**
- ✅ No upcaster needed
- ✅ Both versions handled in one place
- ✅ Simpler to understand

---

### Pattern 2: Event Upcaster (For Complex Changes)

**Best for**: Type changes, computed fields, complex transformations

**Developer implements** (in separate file, NOT generated):

```go
// account_aggregate_upcaster.go

package accountv1

import "google.golang.org/protobuf/proto"

// UpcastEvent upgrades old event versions to current schema
func (a *AccountAggregate) UpcastEvent(event proto.Message) proto.Message {
    switch old := event.(type) {
    case *AccountOpenedEventV1:
        // V1 → V2: Add currency, change balance type
        return &AccountOpenedEvent{
            AccountId:     old.AccountId,
            OwnerName:     old.OwnerName,
            OpeningAmount: convertToDecimal(old.InitialBalance),  // Convert
            Currency:      inferCurrency(old.AccountId),          // Infer from ID
            Timestamp:     old.Timestamp,
        }

    case *MoneyDepositedEventV1:
        // V1 → V2: Different structure
        return &MoneyDepositedEvent{
            AccountId:  old.AccountId,
            Amount:     old.DepositAmount,  // Field rename
            NewBalance: calculateNewBalance(old),  // Computed
            Timestamp:  old.Timestamp,
        }
    }

    return event  // Already current version
}

func convertToDecimal(old string) string {
    // Business logic for conversion
    return decimal.NewFromString(old).String()
}

func inferCurrency(accountId string) string {
    // Business logic to infer currency from account ID pattern
    if strings.HasPrefix(accountId, "EUR") {
        return "EUR"
    }
    return "USD"  // Default
}
```

**Benefits:**
- ✅ Clean transformation in one place
- ✅ Old events automatically upgraded
- ✅ ApplyEvent only sees current version
- ✅ Testable independently

---

### Pattern 3: Snapshot Upcaster

**Best for**: Snapshot format changes

**Developer implements:**

```go
// UpcastSnapshot upgrades old snapshot versions to current schema
func (a *AccountAggregate) UpcastSnapshot(state proto.Message) proto.Message {
    account := state.(*Account)

    // Handle old format with string status
    if account.Status == ACCOUNT_STATUS_UNSPECIFIED && account.StatusString != "" {
        account.Status = parseStatus(account.StatusString)
        account.StatusString = ""  // Clear old field
    }

    // Handle missing fields with defaults
    if account.Currency == "" {
        account.Currency = "USD"
    }

    if account.CreatedAt == 0 {
        account.CreatedAt = time.Now().Unix()  // Best guess
    }

    return account
}

func parseStatus(s string) AccountStatus {
    switch s {
    case "open":
        return ACCOUNT_STATUS_OPEN
    case "closed":
        return ACCOUNT_STATUS_CLOSED
    default:
        return ACCOUNT_STATUS_UNSPECIFIED
    }
}
```

**Benefits:**
- ✅ Old snapshots work seamlessly
- ✅ Gradual migration (snapshots update on next save)
- ✅ No upfront data migration needed

---

## Testing

### Test Event Evolution

```go
func TestAccountEvolution_V1ToV2(t *testing.T) {
    agg := NewAccount("acc-123")

    // Create old V1 event
    oldEvent := &AccountOpenedEventV1{
        AccountId:      "acc-123",
        OwnerName:      "Alice",
        InitialBalance: "1000.00",  // Old field name
        Timestamp:      time.Now().Unix(),
    }

    // Apply old event through upcast
    err := agg.ApplyEvent(oldEvent)
    require.NoError(t, err)

    // Verify state is correct after upcast
    assert.Equal(t, "acc-123", agg.AccountId)
    assert.Equal(t, "Alice", agg.OwnerName)
    assert.Equal(t, "1000.00", agg.Balance)  // Mapped correctly
    assert.Equal(t, "USD", agg.Currency)     // Default added
}
```

### Test Snapshot Evolution

```go
func TestSnapshotEvolution_OldFormat(t *testing.T) {
    agg := NewAccount("acc-123")

    // Create old snapshot format
    oldSnapshot := &Account{
        AccountId:    "acc-123",
        Balance:      "1000.00",
        StatusString: "open",  // Old string field
        // Currency missing
    }

    data, _ := proto.Marshal(oldSnapshot)

    // Unmarshal (will trigger upcast)
    err := agg.UnmarshalSnapshot(data)
    require.NoError(t, err)

    // Verify upcasted correctly
    assert.Equal(t, ACCOUNT_STATUS_OPEN, agg.Status)  // Converted from string
    assert.Equal(t, "USD", agg.Currency)               // Default added
}
```

---

## Migration Workflow

### Scenario: Rename event field

1. **Add new field to proto, deprecate old:**
   ```protobuf
   string initial_balance = 2 [deprecated = true];
   string opening_amount = 3;
   ```

2. **Update ApplyEvent to handle both:**
   ```go
   if e.OpeningAmount != "" {
       a.Balance = e.OpeningAmount
   } else {
       a.Balance = e.InitialBalance
   }
   ```

3. **Deploy** - Old events work, new events use new field

4. **Later: Add upcaster to clean up (optional):**
   ```go
   func (a *AccountAggregate) UpcastEvent(event proto.Message) proto.Message {
       if old, ok := event.(*AccountOpenedEvent); ok {
           if old.InitialBalance != "" && old.OpeningAmount == "" {
               old.OpeningAmount = old.InitialBalance
               old.InitialBalance = ""
           }
       }
       return event
   }
   ```

5. **Much later: Remove deprecated field from proto**

---

## Snapshot Schema Versioning

Use `SnapshotMetadata.SchemaVersion` to track format:

```go
// When saving snapshot
metadata := &SnapshotMetadata{
    SchemaVersion: "v2",  // Current schema version
    SnapshotType:  "protobuf",
    EventCount:    agg.Version(),
}

snapshot := &Snapshot{
    AggregateID:   agg.ID(),
    AggregateType: agg.Type(),
    Version:       agg.Version(),
    Data:          data,
    Metadata:      metadata,
    CreatedAt:     time.Now(),
}
```

```go
// When loading snapshot
func (a *AccountAggregate) UnmarshalSnapshot(data []byte) error {
    // Unmarshal
    a.Account = &Account{}
    proto.Unmarshal(data, a.Account)

    // Upcast if old version
    if upcaster, ok := interface{}(a).(SnapshotUpcaster); ok {
        a.Account = upcaster.UpcastSnapshot(a.Account).(*Account)
    }

    return nil
}
```

---

## Advanced: Optional Repository-Level Upcaster

For complex cases needing external data:

```go
// Optional interface for repository-level upcasting
type RepositoryEventUpcaster interface {
    UpcastEvent(event *Event) (proto.Message, error)
}

// Example: Need database lookup during upcast
type AccountRepositoryUpcaster struct {
    db *sql.DB
}

func (u *AccountRepositoryUpcaster) UpcastEvent(event *Event) (proto.Message, error) {
    if event.EventType == "AccountOpenedEventV1" {
        var v1 AccountOpenedEventV1
        proto.Unmarshal(event.Data, &v1)

        // Look up missing data from external source
        currency, err := u.db.QueryCurrency(v1.AccountId)
        if err != nil {
            return nil, err
        }

        return &AccountOpenedEvent{
            AccountId:     v1.AccountId,
            OwnerName:     v1.OwnerName,
            OpeningAmount: v1.InitialBalance,
            Currency:      currency,  // From database
        }, nil
    }

    // Deserialize normally
    return deserializeEvent(event)
}

// Use in repository
repo := NewAccountRepository(eventStore)
repo.WithEventUpcaster(&AccountRepositoryUpcaster{db: db})
```

**Only use when:**
- ❌ Simple transformations - use aggregate-level
- ✅ Need external data (database, API calls)
- ✅ Shared transformation across multiple aggregates
- ✅ Infrastructure-level concerns

---

## Summary

### Recommendation: Aggregate-Level by Default

**Simple cases (90%):**
- Proto field evolution
- Handle in ApplyEvent

**Medium cases (9%):**
- Implement `EventUpcaster`
- Implement `SnapshotUpcaster`
- Business logic in aggregate

**Complex cases (1%):**
- Repository-level upcaster
- External data needed
- Shared transformations

### Generated Code Changes

Generator will add upcast hooks to:
1. ✅ `ApplyEvent()` method
2. ✅ `UnmarshalSnapshot()` method

Developer implements upcasters only when needed.
