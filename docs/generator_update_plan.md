# Generator Update Plan

## Current State

The generator (`cmd/protoc-gen-eventsourcing/main.go`) currently:

1. ✅ Uses naming heuristics to find aggregates (lines 350-397)
   - Looks for messages with `_id` fields
   - Excludes messages ending in Command/Event/Request/Response/View
   - Fallback to service-based detection

2. ❌ Checks for old `E_EventOptions` extension (line 440)
   - Uses deprecated `eventsourcing.E_EventOptions`
   - Checks `opts.GetAggregate()` field

3. ❌ Doesn't read `aggregate_root` option
   - Just mentioned in comments (line 13)
   - Never actually used in code

4. ❌ Doesn't read new `ServiceOptions`
   - No usage of `aggregate_root_message`
   - No reading of service-level aggregate declaration

## Required Changes

### 1. Update Extension References

**Old:**
```go
eventsourcing.E_EventOptions
eventsourcing.E_AggregateOptions
```

**New:**
```go
eventsourcing.E_Service        // Service options
eventsourcing.E_AggregateRoot  // Aggregate root options
eventsourcing.E_Event          // Event options
```

### 2. Read ServiceOptions

Add function to extract service options:

```go
func getServiceOptions(svc *protogen.Service) *eventsourcing.ServiceOptions {
    if proto.HasExtension(svc.Desc.Options(), eventsourcing.E_Service) {
        return proto.GetExtension(
            svc.Desc.Options(),
            eventsourcing.E_Service,
        ).(*eventsourcing.ServiceOptions)
    }
    return nil
}
```

### 3. Read AggregateRootOptions

Add function to read aggregate root options:

```go
func getAggregateRootOptions(msg *protogen.Message) *eventsourcing.AggregateRootOptions {
    if proto.HasExtension(msg.Desc.Options(), eventsourcing.E_AggregateRoot) {
        return proto.GetExtension(
            msg.Desc.Options(),
            eventsourcing.E_AggregateRoot,
        ).(*eventsourcing.AggregateRootOptions)
    }
    return nil
}
```

### 4. Update findAggregates()

**Current approach:** Heuristic-based (lines 350-397)
**New approach:** Use ServiceOptions + AggregateRootOptions

```go
func findAggregates(file *protogen.File) []*AggregateInfo {
    var aggregates []*AggregateInfo

    // Step 1: Get aggregate names from services
    aggregateNames := make(map[string]*eventsourcing.ServiceOptions)
    for _, svc := range file.Services {
        if opts := getServiceOptions(svc); opts != nil {
            aggregateNames[opts.GetAggregateName()] = opts
        }
    }

    // Step 2: Find aggregate root messages
    for _, msg := range file.Messages {
        opts := getAggregateRootOptions(msg)
        if opts == nil {
            continue // Not an aggregate root
        }

        messageName := string(msg.Desc.Name())
        typeName := opts.GetTypeName()
        if typeName == "" {
            typeName = messageName // Default to message name
        }

        idField := opts.GetIdField()
        if idField == "" {
            return nil, fmt.Errorf("aggregate_root option must specify id_field")
        }

        // Find the Go field name
        idFieldGo := ""
        for _, field := range msg.Fields {
            if string(field.Desc.Name()) == idField {
                idFieldGo = field.GoName
                break
            }
        }

        if idFieldGo == "" {
            return nil, fmt.Errorf("id_field '%s' not found in message %s", idField, messageName)
        }

        aggregates = append(aggregates, &AggregateInfo{
            Message:     msg,
            MessageName: messageName,
            TypeName:    typeName,
            IDField:     idField,
            IDFieldGo:   idFieldGo,
        })
    }

    return aggregates
}
```

### 5. Update findEventsForAggregate()

**Current:** Checks old `E_EventOptions` extension (line 440)
**New:** Check new `E_Event` extension

```go
func findEventsForAggregate(file *protogen.File, aggregateName string) []*EventInfo {
    var events []*EventInfo

    for _, msg := range file.Messages {
        if !strings.HasSuffix(string(msg.Desc.Name()), "Event") {
            continue
        }

        // Check for event option
        if !proto.HasExtension(msg.Desc.Options(), eventsourcing.E_Event) {
            continue // Skip events without explicit marking
        }

        opts := proto.GetExtension(
            msg.Desc.Options(),
            eventsourcing.E_Event,
        ).(*eventsourcing.EventOptions)

        // Only include events for this aggregate
        if opts.GetAggregateName() != aggregateName {
            continue
        }

        events = append(events, &EventInfo{
            MessageName:   string(msg.Desc.Name()),
            AggregateName: aggregateName,
        })
    }

    return events
}
```

### 6. Remove Unique Constraint Generation

Since unique constraints are now handled in code, remove:
- Lines 189-199: UniqueConstraint code generation
- Lines 333-348: ConstraintInfo struct

### 7. Remove field_mapping and applies_to_state

Since these are removed from EventOptions:
- Remove from EventInfo struct (lines 338-341)
- Remove extractStateFieldsFromEvent() function (lines 468-477)

## Migration Strategy

### Phase 1: Support Both Old and New Options
- Keep fallback to naming heuristics
- Check for new options first, fall back to old
- Emit warnings when old options detected

### Phase 2: New Options Only
- Remove all heuristic-based detection
- Require explicit options on all entities
- Fail generation if options missing

## Files to Update

1. ✅ `proto/eventsourcing/options.proto` - Already done
2. ⏳ `cmd/protoc-gen-eventsourcing/main.go` - Update generator
3. ⏳ `examples/proto/account/v1/account.proto` - Migrate to new format
4. ⏳ `pkg/eventsourcing/repository.go` - Add snapshot upcaster support

## Testing Plan

1. Update generator code
2. Regenerate existing examples with warnings
3. Migrate one example to new format
4. Test generation with new format
5. Verify generated code compiles and works
