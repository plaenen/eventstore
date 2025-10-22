# Simplified Proto Options - Final Design

## Summary

Successfully simplified proto options by **removing 90% of configuration** while maintaining full functionality.

## What Changed

### ✅ Removed Options:

1. **`field_mapping`** - Developer handles in `ApplyEvent` implementation
2. **`applies_to_state`** - Developer controls which fields update
3. **`unique_constraints`** - Developer handles in handler code
4. **`produces_events`** - Developer's implementation determines events
5. **All backward compatibility options** - Clean break, no deprecated clutter

### ✅ Final Options:

#### 1. ServiceOptions (Service Level)
```protobuf
service AccountCommandService {
  option (eventsourcing.service) = {
    aggregate_name: "Account"           // Single source of truth
    aggregate_root_message: "Account"   // Explicit root reference
  };

  rpc OpenAccount(OpenAccountCommand) returns (OpenAccountResponse);
}
```

#### 2. AggregateRootOptions (Aggregate State)
```protobuf
message Account {
  option (eventsourcing.aggregate_root) = {
    id_field: "account_id"    // Just the ID field!
  };

  string account_id = 1;
  string balance = 2;
}
```

#### 3. EventOptions (Events)
```protobuf
message AccountOpenedEvent {
  option (eventsourcing.event) = {
    aggregate_name: "Account"   // Just the aggregate!
  };

  string account_id = 1;
  string initial_balance = 2;
  int64 timestamp = 3;
}
```

#### 4. Commands - NO OPTIONS!
```protobuf
message OpenAccountCommand {
  // Everything inherited from service
  // No options needed!

  string account_id = 1;
  string owner_name = 2;
}
```

## Complete Example

```protobuf
syntax = "proto3";
package account.v1;

import "eventsourcing/options.proto";

// ============================================
// SERVICE - Single source of truth
// ============================================
service AccountCommandService {
  option (eventsourcing.service) = {
    aggregate_name: "Account"
    aggregate_root_message: "Account"
  };

  rpc OpenAccount(OpenAccountCommand) returns (OpenAccountResponse);
  rpc Deposit(DepositCommand) returns (DepositResponse);
  rpc Withdraw(WithdrawCommand) returns (WithdrawResponse);
}

// ============================================
// COMMANDS - NO OPTIONS!
// ============================================
message OpenAccountCommand {
  string account_id = 1;
  string owner_name = 2;
  string initial_balance = 3;
}

message DepositCommand {
  string account_id = 1;
  string amount = 2;
}

message WithdrawCommand {
  string account_id = 1;
  string amount = 2;
}

// ============================================
// AGGREGATE ROOT - Just ID field
// ============================================
message Account {
  option (eventsourcing.aggregate_root) = {
    id_field: "account_id"
  };

  string account_id = 1;
  string owner_name = 2;
  string balance = 3;
  AccountStatus status = 4;
}

// ============================================
// EVENTS - Just aggregate name
// ============================================
message AccountOpenedEvent {
  option (eventsourcing.event) = {
    aggregate_name: "Account"
  };

  string account_id = 1;
  string owner_name = 2;
  string initial_balance = 3;
  int64 timestamp = 4;
}

message MoneyDepositedEvent {
  option (eventsourcing.event) = {
    aggregate_name: "Account"
  };

  string account_id = 1;
  string amount = 2;
  string new_balance = 3;
  int64 timestamp = 4;
}

message MoneyWithdrawnEvent {
  option (eventsourcing.event) = {
    aggregate_name: "Account"
  };

  string account_id = 1;
  string amount = 2;
  string new_balance = 3;
  int64 timestamp = 4;
}

enum AccountStatus {
  ACCOUNT_STATUS_UNSPECIFIED = 0;
  ACCOUNT_STATUS_OPEN = 1;
  ACCOUNT_STATUS_CLOSED = 2;
}
```

## Developer Implementation

### Unique Constraints (In Code)

```go
func (h *AccountHandler) OpenAccount(ctx context.Context, cmd *OpenAccountCommand) (*OpenAccountResponse, error) {
    // Developer handles constraints explicitly
    if err := h.constraintStore.Claim(ctx, "account_id", cmd.AccountId); err != nil {
        return nil, eventsourcing.NewAppError(
            "ACCOUNT_ID_TAKEN",
            "Account ID already in use",
            err,
        )
    }

    account := NewAccount(cmd.AccountId)
    if err := account.OpenAccount(ctx, cmd, metadata); err != nil {
        return nil, err
    }

    if _, err := h.repo.SaveWithCommand(account, commandID); err != nil {
        return nil, err
    }

    return &OpenAccountResponse{
        AccountId: cmd.AccountId,
        Version:   account.Version(),
    }, nil
}
```

### Field Mapping (In ApplyEvent)

```go
func (a *AccountAggregate) ApplyAccountOpenedEvent(e *AccountOpenedEvent) error {
    // Developer controls field mapping explicitly
    a.AccountId = e.AccountId
    a.OwnerName = e.OwnerName
    a.Balance = e.InitialBalance  // ← Maps event.InitialBalance to aggregate.Balance
    a.Status = ACCOUNT_STATUS_OPEN
    return nil
}

func (a *AccountAggregate) ApplyMoneyDepositedEvent(e *MoneyDepositedEvent) error {
    // Developer controls which fields update
    a.Balance = e.NewBalance  // Only update balance
    // Don't touch: AccountId, OwnerName, Status
    return nil
}
```

## Event Evolution Strategy

### Simple Case: Proto Field Evolution

```protobuf
message AccountOpenedEvent {
  option (eventsourcing.event) = {aggregate_name: "Account"};

  string account_id = 1;
  string owner_name = 2;

  // Field renamed - keep both for backward compatibility
  string initial_balance = 3 [deprecated = true];  // Old field
  string opening_amount = 4;                       // New field

  // New field with default
  string currency = 5;  // Defaults to "" for old events

  int64 timestamp = 6;
}
```

**Handle in ApplyEvent:**
```go
func (a *AccountAggregate) ApplyAccountOpenedEvent(e *AccountOpenedEvent) error {
    a.AccountId = e.AccountId
    a.OwnerName = e.OwnerName

    // Handle old vs new field
    if e.OpeningAmount != "" {
        a.Balance = e.OpeningAmount
    } else {
        a.Balance = e.InitialBalance  // Fall back to old field
    }

    // Handle new field with default
    if e.Currency == "" {
        a.Currency = "USD"  // Default for old events
    } else {
        a.Currency = e.Currency
    }

    return nil
}
```

### Complex Case: Custom Upcaster

For complex transformations, developer can provide optional upcaster:

```go
type EventUpcaster interface {
    Upcast(event proto.Message) (proto.Message, error)
}

type AccountEventUpcaster struct{}

func (u *AccountEventUpcaster) Upcast(old proto.Message) (proto.Message, error) {
    switch e := old.(type) {
    case *AccountOpenedEventV1:
        // Complex transformation logic
        return &AccountOpenedEventV2{
            AccountId:     e.AccountId,
            OwnerName:     e.OwnerName,
            OpeningAmount: convertCurrency(e.InitialBalance, "USD"),
            Currency:      "USD",
        }, nil
    }
    return old, nil // Already current version
}

// Use in repository
repo := NewAccountRepository(eventStore)
repo.WithUpcaster(&AccountEventUpcaster{})
```

## Benefits

| Aspect | Before | After | Improvement |
|--------|--------|-------|-------------|
| **Service** | `aggregate_name` | `aggregate_name` + `aggregate_root_message` | +clarity |
| **Commands** | ~15 lines options | 0 lines | **100% reduction** |
| **Aggregate** | `id_field` + `type_name` | `id_field` | Simpler |
| **Events** | ~10 lines options | 1 line | **90% reduction** |
| **Constraints** | Proto + code | Code only | More flexible |
| **Field mapping** | Proto + code | Code only | More flexible |
| **Total proto** | ~40 lines options | ~4 lines options | **90% reduction** |

## Design Principles

1. ✅ **Configuration in proto, implementation in code**
   - Proto: Structure and relationships
   - Code: Business logic and behavior

2. ✅ **Single source of truth**
   - Service declares aggregate once
   - Everything else inherits or references

3. ✅ **Convention over configuration**
   - Sensible defaults
   - Override only when needed

4. ✅ **Developer control**
   - Explicit over implicit
   - Testable over magic
   - Flexible over constrained

## Migration from Old Format

### Old Format:
```protobuf
message OpenAccountCommand {
  option (eventsourcing.aggregate_options) = {
    aggregate: "Account"
    produces_events: "AccountOpenedEvent"
    unique_constraints: {
      index_name: "account_id"
      field: "account_id"
      operation: CONSTRAINT_OPERATION_CLAIM
    }
  };
  string account_id = 1;
}

message AccountOpenedEvent {
  option (eventsourcing.event_options) = {
    aggregate: "Account"
    applies_to_state: ["account_id", "owner_name", "balance"]
    field_mapping: {
      key: "initial_balance"
      value: "balance"
    }
  };
  string account_id = 1;
  string initial_balance = 2;
}
```

### New Format:
```protobuf
// NO OPTIONS on command!
message OpenAccountCommand {
  string account_id = 1;
}

// Minimal option on event
message AccountOpenedEvent {
  option (eventsourcing.event) = {aggregate_name: "Account"};
  string account_id = 1;
  string initial_balance = 2;
}

// Constraint in handler code
func (h *Handler) OpenAccount(ctx, cmd) (*Response, error) {
    h.constraintStore.Claim(ctx, "account_id", cmd.AccountId)
    // ...
}

// Field mapping in ApplyEvent
func (a *Aggregate) ApplyAccountOpenedEvent(e *Event) error {
    a.Balance = e.InitialBalance  // Explicit mapping
    // ...
}
```

## Next Steps

1. ✅ **Options.proto updated** - Clean, minimal design
2. ⏳ **Update generator** - Support new options
3. ⏳ **Migrate examples** - Update bankaccount to new format
4. ⏳ **Test generation** - Verify everything works
5. ⏳ **Projection SDK** - Generate projection interfaces from events
