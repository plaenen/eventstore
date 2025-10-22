# Proto Options Simplification Proposal

## Executive Summary

Simplify proto annotations by **80%** while enabling better tooling (projection SDK generation). Key changes:

1. ✅ **Remove unique constraints from proto** - developers handle in code
2. ✅ **Add `aggregate_root_message` to service** - enables cross-file organization
3. ✅ **Keep explicit event marking** - enables projection SDK generation
4. ✅ **Remove redundant aggregate references** - single source of truth

---

## Before (Current Design)

### Problems:
- ❌ Redundant aggregate name (5 places!)
- ❌ Unique constraints in proto (should be in code)
- ❌ `produces_events` duplicates handler implementation
- ❌ Events and commands both need aggregate options
- ❌ No clear aggregate root reference from service

### Example:

```protobuf
// Service
service AccountCommandService {
  option (eventsourcing.aggregate_name) = "Account";  // ← Aggregate name #1
  rpc OpenAccount(OpenAccountCommand) returns (OpenAccountResponse);
}

// Command
message OpenAccountCommand {
  option (eventsourcing.aggregate_options) = {
    aggregate: "Account"                              // ← Aggregate name #2 (redundant!)
    produces_events: "AccountOpenedEvent"             // ← Duplicates handler implementation
    unique_constraints: {                             // ← Should be in handler code!
      index_name: "account_id"
      field: "account_id"
      operation: CONSTRAINT_OPERATION_CLAIM
    }
  };

  string account_id = 1;
  string owner_name = 2;
}

// Aggregate
message Account {
  option (eventsourcing.aggregate_root) = {
    id_field: "account_id"
    type_name: "Account"                              // ← Aggregate name #3 (redundant!)
  };

  string account_id = 1;
  string balance = 2;
}

// Event
message AccountOpenedEvent {
  option (eventsourcing.event_options) = {
    aggregate: "Account"                              // ← Aggregate name #4 (redundant!)
    applies_to_state: ["account_id", "owner_name", "balance"]
    field_mapping: {
      key: "initial_balance"
      value: "balance"
    }
  };

  string account_id = 1;
  string owner_name = 2;
  string initial_balance = 3;
  int64 timestamp = 4;
}
```

**Character count: ~1,450 characters of options**

---

## After (Proposed Design)

### Benefits:
- ✅ Single source of truth (service declares aggregate)
- ✅ Unique constraints in handler code (more flexible)
- ✅ Explicit aggregate root reference (cross-file support)
- ✅ Explicit event marking (enables projection SDK)
- ✅ Commands have NO options (inherit from service)

### Example:

```protobuf
// Service - SINGLE source of truth
service AccountCommandService {
  option (eventsourcing.service) = {
    aggregate_name: "Account"           // ← Only aggregate declaration
    aggregate_root_message: "Account"   // ← Explicit root reference
  };

  rpc OpenAccount(OpenAccountCommand) returns (OpenAccountResponse);
}

// Command - NO OPTIONS!
message OpenAccountCommand {
  // Everything inherited from service
  // Unique constraints handled in handler code

  string account_id = 1;
  string owner_name = 2;
}

// Aggregate - Just ID field
message Account {
  option (eventsourcing.aggregate_root) = {
    id_field: "account_id"
  };

  string account_id = 1;
  string balance = 2;
}

// Event - Minimal marking for projection SDK
message AccountOpenedEvent {
  option (eventsourcing.event) = {
    aggregate_name: "Account"
    // Optional: field_mapping if needed
  };

  string account_id = 1;
  string owner_name = 2;
  string initial_balance = 3;
  int64 timestamp = 4;
}
```

**Character count: ~280 characters of options** (80% reduction!)

---

## Handler Code - Where Constraints Live

### Before (Proto-Driven):
```go
// Generator creates constraint enforcement from proto
func (h *AccountHandler) OpenAccount(ctx context.Context, cmd *OpenAccountCommand) (*OpenAccountResponse, error) {
    // Generator automatically enforces unique_constraints from proto
    // Less control, less flexibility
}
```

### After (Developer-Driven):
```go
// Developer explicitly handles constraints
func (h *AccountHandler) OpenAccount(ctx context.Context, cmd *OpenAccountCommand) (*OpenAccountResponse, error) {
    // Developer controls constraint logic
    if err := h.constraintStore.Claim(ctx, "account_id", cmd.AccountId); err != nil {
        return nil, eventsourcing.NewAppError(
            "ACCOUNT_ID_ALREADY_EXISTS",
            "Account ID already in use",
            err,
        )
    }

    account := NewAccount(cmd.AccountId)
    if err := account.OpenAccount(ctx, cmd, metadata); err != nil {
        return nil, err
    }

    if _, err := h.repo.SaveWithCommand(account, commandID); err != nil {
        // Constraint automatically rolled back on error
        return nil, err
    }

    return &OpenAccountResponse{
        AccountId: cmd.AccountId,
        Version:   account.Version(),
    }, nil
}
```

**Benefits:**
- ✅ Explicit constraint management (easier to understand)
- ✅ Can add complex business logic (e.g., "email must be unique per tenant")
- ✅ Better error messages (developer controls)
- ✅ Testable constraint logic
- ✅ No magic - clear what's happening

---

## Projection SDK Generation

### Why Events Need Explicit Marking

With explicit `option (eventsourcing.event)`, the generator can create type-safe projection SDKs:

```go
// Generated from proto
type AccountProjection interface {
    // All events discovered via (eventsourcing.event) option
    ApplyAccountOpenedEvent(e *AccountOpenedEvent) error
    ApplyMoneyDepositedEvent(e *MoneyDepositedEvent) error
    ApplyMoneyWithdrawnEvent(e *MoneyWithdrawnEvent) error
    ApplyAccountClosedEvent(e *AccountClosedEvent) error
}

// Generated event router
func RouteAccountEvent(projection AccountProjection, event proto.Message) error {
    switch e := event.(type) {
    case *AccountOpenedEvent:
        return projection.ApplyAccountOpenedEvent(e)
    case *MoneyDepositedEvent:
        return projection.ApplyMoneyDepositedEvent(e)
    case *MoneyWithdrawnEvent:
        return projection.ApplyMoneyWithdrawnEvent(e)
    case *AccountClosedEvent:
        return projection.ApplyAccountClosedEvent(e)
    default:
        return fmt.Errorf("unknown event type for Account: %T", event)
    }
}

// Developer implements projection
type AccountViewProjection struct {
    db *sql.DB
}

func (p *AccountViewProjection) ApplyAccountOpenedEvent(e *AccountOpenedEvent) error {
    _, err := p.db.Exec(`
        INSERT INTO account_view (account_id, owner_name, balance, status)
        VALUES (?, ?, ?, ?)
    `, e.AccountId, e.OwnerName, e.InitialBalance, "OPEN")
    return err
}

// ProjectionManager uses generated router
pm := eventsourcing.NewProjectionManager(checkpointStore, eventStore, eventBus)
pm.RegisterWithRouter("account_view", accountView, RouteAccountEvent)
```

---

## Cross-File Organization

### Why `aggregate_root_message` is Needed

**Scenario**: Large aggregates with multiple proto files

```
account/
  v1/
    account_service.proto   (service + commands + queries)
    account_state.proto     (Account aggregate root)
    account_events.proto    (events)
```

**account_service.proto:**
```protobuf
service AccountCommandService {
  option (eventsourcing.service) = {
    aggregate_name: "Account"
    aggregate_root_message: "Account"  // ← Points to message in account_state.proto
  };

  rpc OpenAccount(OpenAccountCommand) returns (OpenAccountResponse);
}
```

**account_state.proto:**
```protobuf
message Account {
  option (eventsourcing.aggregate_root) = {
    id_field: "account_id"
  };

  string account_id = 1;
  string balance = 2;
}
```

**account_events.proto:**
```protobuf
message AccountOpenedEvent {
  option (eventsourcing.event) = {
    aggregate_name: "Account"  // ← Links back to service
  };

  string account_id = 1;
  int64 timestamp = 2;
}
```

**Generator can:**
1. Read service → find `aggregate_root_message: "Account"`
2. Scan all files for `message Account` with `aggregate_root` option
3. Scan all files for events with `event.aggregate_name: "Account"`
4. Generate complete aggregate + repository + projection SDK

---

## Migration Path

### Phase 1: Add New Options (Non-Breaking)
- Add `aggregate_root_message` to service options
- Add simplified `event` option
- Keep old options working

### Phase 2: Update Examples
- Update all example proto files to new format
- Update documentation
- Show side-by-side comparison

### Phase 3: Deprecation Warning
- Generator emits warnings for old options
- Documentation marks old options as deprecated

### Phase 4: Remove Old Options
- Remove deprecated options
- Clean up generator code
- Major version bump

---

## Decision Points

### 1. Unique Constraints
**Proposal**: Remove from proto, handle in code
**Rationale**: More flexible, easier to test, no magic
**Status**: ✅ Agreed

### 2. `aggregate_root_message`
**Proposal**: Add to service options
**Rationale**: Enables cross-file organization
**Status**: ✅ Agreed

### 3. Explicit Event Marking
**Proposal**: Keep `option (eventsourcing.event)` required
**Rationale**: Enables projection SDK generation
**Status**: ✅ Agreed

### 4. Command Options
**Proposal**: Remove all command-level options
**Rationale**: Everything inherited from service
**Status**: ✅ Agreed

### 5. Event Field Mapping
**Proposal**: Keep as optional override
**Rationale**: Needed for initial_balance → balance cases
**Status**: ✅ Agreed

---

## Summary

| Aspect | Before | After | Benefit |
|--------|--------|-------|---------|
| **Service** | `aggregate_name` only | `aggregate_name` + `aggregate_root_message` | Cross-file support |
| **Commands** | Full `aggregate_options` | NO OPTIONS | 100% reduction |
| **Aggregate** | `id_field` + `type_name` | `id_field` only | Simpler |
| **Events** | Full `event_options` | Minimal `event` | Enables projection SDK |
| **Constraints** | Proto declarations | Handler code | More flexible |
| **Lines of proto** | ~30 lines options | ~6 lines options | 80% reduction |

**Next Steps:**
1. Update `options.proto` with new design
2. Update generator to support new options
3. Migrate examples to new format
4. Generate projection SDK code
5. Update documentation

---

## Questions?

1. Should event `aggregate_name` be optional if event is in same file as aggregate?
2. Should we auto-generate constraint helpers (e.g., `ClaimAccountId()` methods)?
3. Should projection SDK be opt-in or generated by default?
