# Multi-Tenancy Guide

This guide explains how to build multi-tenant applications using the event sourcing framework.

## Table of Contents

- [Overview](#overview)
- [Isolation Strategies](#isolation-strategies)
- [Quick Start](#quick-start)
- [Tenant Context](#tenant-context)
- [Middleware Integration](#middleware-integration)
- [SDK Integration](#sdk-integration)
- [Best Practices](#best-practices)
- [Examples](#examples)

## Overview

Multi-tenancy allows a single application instance to serve multiple customers (tenants) while keeping their data isolated and secure. This framework provides built-in support for multi-tenancy at all layers.

### Key Features

- ✅ **Multiple isolation strategies** - Choose between shared database or database-per-tenant
- ✅ **Tenant context propagation** - Automatic tenant tracking through commands and events
- ✅ **Middleware enforcement** - Validate tenant boundaries at runtime
- ✅ **Type-safe APIs** - Compiler-enforced tenant isolation
- ✅ **Works with all features** - Snapshots, projections, unique constraints, etc.

## Isolation Strategies

### Strategy 1: Shared Database with Tenant-Prefixed Aggregates

**How it works:**
- All tenants share the same database
- Aggregate IDs are prefixed with tenant ID: `{tenantID}::{aggregateID}`
- Example: `tenant-abc::acc-123`, `tenant-xyz::acc-456`

**Configuration:**

```go
import "github.com/plaenen/eventsourcing/pkg/multitenancy"

multiStore, err := multitenancy.NewMultiTenantEventStore(multitenancy.MultiTenantConfig{
    Strategy:  multitenancy.SharedDatabase,
    SharedDSN: "./events.db",
    WALMode:   true,
})
```

**Pros:**
- ✅ Simple deployment - single database file
- ✅ Easy cross-tenant analytics and reporting
- ✅ Lower infrastructure overhead
- ✅ All existing features work out of the box

**Cons:**
- ❌ Data is commingled in same tables
- ❌ Harder to export/delete single tenant data
- ❌ Need to filter queries by tenant

**When to use:**
- Many small tenants (hundreds to thousands)
- Need cross-tenant analytics
- Lower infrastructure complexity preferred
- Moderate security requirements

### Strategy 2: Database-Per-Tenant

**How it works:**
- Each tenant gets their own SQLite database file
- Aggregate IDs can be simple local IDs (no prefix needed)
- Example: Tenant A has `./data/tenant-a.db`, Tenant B has `./data/tenant-b.db`

**Configuration:**

```go
multiStore, err := multitenancy.NewMultiTenantEventStore(multitenancy.MultiTenantConfig{
    Strategy:             multitenancy.DatabasePerTenant,
    DatabasePathTemplate: "./data/tenant_%s.db",
    WALMode:              true,
})
```

**Pros:**
- ✅ Complete data isolation at filesystem level
- ✅ Easy to backup/restore single tenant
- ✅ Easy to delete tenant data (just remove file)
- ✅ Simple aggregate IDs
- ✅ Natural security boundary

**Cons:**
- ❌ More complex connection management
- ❌ Harder to do cross-tenant analytics
- ❌ More file handles needed
- ❌ Each tenant needs schema migrations

**When to use:**
- Large enterprise tenants (dozens to hundreds)
- Strong data isolation requirements
- Need to export/delete tenant data easily
- Compliance requirements (GDPR, etc.)

## Quick Start

### 1. Set Up Multi-Tenant Store

```go
package main

import (
    "context"
    "github.com/plaenen/eventsourcing/pkg/multitenancy"
    "github.com/plaenen/eventsourcing/pkg/unifiedsdk"
)

func main() {
    // Create multi-tenant store
    multiStore, err := multitenancy.NewMultiTenantEventStore(multitenancy.MultiTenantConfig{
        Strategy:  multitenancy.SharedDatabase,
        SharedDSN: "./events.db",
        WALMode:   true,
    })
    if err != nil {
        panic(err)
    }
    defer multiStore.Close()

    // Wrap with tenant-aware interface
    eventStore := multitenancy.NewTenantAwareEventStore(multiStore)

    // Use with SDK (context must have tenant ID!)
    // ... see below
}
```

### 2. Add Tenant Context to Requests

```go
// Extract tenant from request (HTTP header, JWT claim, subdomain, etc.)
func extractTenantID(r *http.Request) string {
    // Option 1: From subdomain
    // tenant-a.myapp.com → "tenant-a"

    // Option 2: From header
    return r.Header.Get("X-Tenant-ID")

    // Option 3: From JWT claim
    // claims := r.Context().Value("claims")
    // return claims["tenant_id"]
}

// HTTP middleware to inject tenant into context
func TenantMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        tenantID := extractTenantID(r)
        if tenantID == "" {
            http.Error(w, "Missing tenant ID", http.StatusBadRequest)
            return
        }

        // Add tenant to context
        ctx := multitenancy.WithTenantID(r.Context(), tenantID)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}
```

### 3. Use Tenant-Scoped Aggregate IDs

**Strategy 1 (Shared Database):**

```go
import "github.com/plaenen/eventsourcing/pkg/multitenancy"

// Client provides local ID
localAccountID := "acc-123"

// Get tenant from context
tenantID := multitenancy.MustGetTenantID(ctx)

// Compose tenant-scoped ID
aggregateID := multitenancy.ComposeAggregateID(tenantID, localAccountID)
// Result: "tenant-abc::acc-123"

// Use with SDK
account := accountv1.NewAccount(aggregateID)
```

**Strategy 2 (Database-Per-Tenant):**

```go
// With separate databases, use simple local IDs
localAccountID := "acc-123"

// Tenant context determines which database to use
account := accountv1.NewAccount(localAccountID)
```

### 4. Configure Command Bus with Tenant Middleware

```go
import (
    "github.com/plaenen/eventsourcing/pkg/eventsourcing"
    "github.com/plaenen/eventsourcing/pkg/multitenancy"
)

commandBus := eventsourcing.NewCommandBus()

// Add tenant isolation middleware (IMPORTANT!)
commandBus.Use(multitenancy.TenantIsolationMiddleware())

// Optional: Tenant extraction from different sources
commandBus.Use(multitenancy.TenantExtractionMiddleware(
    func(envelope *eventsourcing.CommandEnvelope) (string, error) {
        // Custom logic to extract tenant from command
        return envelope.Metadata.TenantID, nil
    },
))

// Optional: Tenant authorization
authorizer := &MyTenantAuthorizer{} // Implement TenantAuthorizer interface
commandBus.Use(multitenancy.TenantAuthorizationMiddleware(authorizer))
```

## Tenant Context

### Adding Tenant to Context

```go
import "github.com/plaenen/eventsourcing/pkg/multitenancy"

// Add tenant ID to context
ctx := multitenancy.WithTenantID(context.Background(), "tenant-abc")

// Retrieve tenant ID
tenantID, err := multitenancy.GetTenantID(ctx)
if err != nil {
    // Tenant ID not found
}

// Panic if tenant ID not found (use when required)
tenantID := multitenancy.MustGetTenantID(ctx)

// Check if context has tenant ID
if multitenancy.HasTenantID(ctx) {
    // Process
}
```

### Tenant in Metadata

Tenant ID is automatically propagated through command and event metadata:

```go
// Commands
envelope := &eventsourcing.CommandEnvelope{
    Command: cmd,
    Metadata: eventsourcing.CommandMetadata{
        TenantID: "tenant-abc", // Set by middleware
        // ... other fields
    },
}

// Events
event := &eventsourcing.Event{
    Metadata: eventsourcing.EventMetadata{
        TenantID: "tenant-abc", // Inherited from command
        // ... other fields
    },
}
```

## Middleware Integration

### Tenant Isolation Middleware

Enforces tenant boundaries for all commands:

```go
commandBus.Use(multitenancy.TenantIsolationMiddleware())
```

**What it does:**
1. Validates tenant ID is present in context
2. Ensures aggregate IDs match tenant context
3. Prevents commands from crossing tenant boundaries
4. Adds tenant ID to all emitted events

### Tenant Extraction Middleware

Extracts tenant ID from various sources:

```go
extractor := func(envelope *eventsourcing.CommandEnvelope) (string, error) {
    // Option 1: From command metadata
    if envelope.Metadata.TenantID != "" {
        return envelope.Metadata.TenantID, nil
    }

    // Option 2: From principal ID (e.g., "tenant-abc:user-123")
    parts := strings.Split(envelope.Metadata.PrincipalID, ":")
    if len(parts) == 2 {
        return parts[0], nil
    }

    return "", fmt.Errorf("cannot extract tenant ID")
}

commandBus.Use(multitenancy.TenantExtractionMiddleware(extractor))
```

### Tenant Authorization Middleware

Validates principal has access to tenant:

```go
type MyTenantAuthorizer struct {
    db *sql.DB
}

func (a *MyTenantAuthorizer) Authorize(ctx context.Context, principalID, tenantID string) error {
    // Check database for user-tenant relationship
    var count int
    err := a.db.QueryRow(`
        SELECT COUNT(*) FROM tenant_users
        WHERE tenant_id = ? AND user_id = ?
    `, tenantID, principalID).Scan(&count)

    if err != nil {
        return err
    }

    if count == 0 {
        return fmt.Errorf("principal %s not authorized for tenant %s", principalID, tenantID)
    }

    return nil
}

authorizer := &MyTenantAuthorizer{db: db}
commandBus.Use(multitenancy.TenantAuthorizationMiddleware(authorizer))
```

## SDK Integration

### With Unified SDK

```go
import (
    "github.com/plaenen/eventsourcing/pkg/unifiedsdk"
    "github.com/plaenen/eventsourcing/pkg/multitenancy"
)

// Create multi-tenant store
multiStore, _ := multitenancy.NewMultiTenantEventStore(multitenancy.MultiTenantConfig{
    Strategy:  multitenancy.SharedDatabase,
    SharedDSN: "./events.db",
})

eventStore := multitenancy.NewTenantAwareEventStore(multiStore)

// Create SDK with tenant-aware store
s, err := unifiedsdk.New(
    unifiedsdk.WithMode(sdk.DevelopmentMode),
    // Note: SDK config needs to be extended to accept custom event store
    // For now, you'd need to manually wire the command bus
)

// Use with tenant context
ctx := multitenancy.WithTenantID(context.Background(), "tenant-abc")

// Compose tenant-scoped aggregate ID
aggregateID := multitenancy.ComposeAggregateID("tenant-abc", "acc-123")

s.Account.OpenAccount(ctx, &accountv1.OpenAccountCommand{
    AccountId:      aggregateID,
    OwnerName:      "Alice",
    InitialBalance: "1000.00",
}, "user-alice")
```

## Best Practices

### 1. Always Use Tenant Context

```go
// ✅ Good - Tenant in context
ctx := multitenancy.WithTenantID(r.Context(), tenantID)
s.Account.OpenAccount(ctx, cmd, principalID)

// ❌ Bad - Missing tenant context
s.Account.OpenAccount(context.Background(), cmd, principalID)
```

### 2. Validate Tenant Access Early

```go
// At API gateway/ingress
func (h *Handler) OpenAccount(w http.ResponseWriter, r *http.Request) {
    tenantID := extractTenantID(r)
    principalID := extractPrincipalID(r)

    // Validate access BEFORE processing
    if !h.authorizer.CanAccess(principalID, tenantID) {
        http.Error(w, "Unauthorized", http.StatusForbidden)
        return
    }

    ctx := multitenancy.WithTenantID(r.Context(), tenantID)
    // ... process command
}
```

### 3. Use Middleware Stack

Recommended order:

```go
commandBus.Use(middleware.RecoveryMiddleware(logger))              // 1. Panic recovery
commandBus.Use(middleware.LoggingMiddleware(logger))               // 2. Logging
commandBus.Use(multitenancy.TenantExtractionMiddleware(extractor)) // 3. Extract tenant
commandBus.Use(multitenancy.TenantIsolationMiddleware())           // 4. Enforce isolation
commandBus.Use(multitenancy.TenantAuthorizationMiddleware(auth))   // 5. Authorize access
commandBus.Use(middleware.ValidationMiddleware())                  // 6. Validate command
```

### 4. Index Projections by Tenant

**For Shared Database:**

```sql
CREATE TABLE account_views (
    tenant_id TEXT NOT NULL,
    account_id TEXT NOT NULL,
    owner_name TEXT NOT NULL,
    balance TEXT NOT NULL,
    version INTEGER NOT NULL,
    PRIMARY KEY (tenant_id, account_id)
);

CREATE INDEX idx_tenant_accounts ON account_views(tenant_id);
```

**Projection handler:**

```go
func (p *AccountViewProjection) Handle(ctx context.Context, event *eventsourcing.Event) error {
    tenantID, localID, _ := multitenancy.DecomposeAggregateID(event.AggregateID)

    _, err := p.db.Exec(`
        INSERT INTO account_views (tenant_id, account_id, owner_name, balance, version)
        VALUES (?, ?, ?, ?, ?)
        ON CONFLICT(tenant_id, account_id) DO UPDATE SET ...
    `, tenantID, localID, ...)

    return err
}
```

### 5. Monitor Tenant Usage

```go
// Track tenant metrics
type TenantMetrics struct {
    TenantID       string
    CommandCount   int64
    EventCount     int64
    StorageBytes   int64
    LastActivityAt time.Time
}

// Middleware to track usage
func TenantMetricsMiddleware(metrics *TenantMetrics) eventsourcing.Middleware {
    return func(next eventsourcing.CommandHandler) eventsourcing.CommandHandler {
        return eventsourcing.CommandHandlerFunc(func(ctx context.Context, cmd *eventsourcing.CommandEnvelope) ([]*eventsourcing.Event, error) {
            tenantID, _ := multitenancy.GetTenantID(ctx)

            // Track command
            atomic.AddInt64(&metrics.CommandCount, 1)
            metrics.LastActivityAt = time.Now()

            events, err := next.Handle(ctx, cmd)

            if err == nil {
                atomic.AddInt64(&metrics.EventCount, int64(len(events)))
            }

            return events, err
        })
    }
}
```

### 6. Handle Tenant Lifecycle

```go
// Provision new tenant
func ProvisionTenant(tenantID string) error {
    // For DatabasePerTenant strategy
    multiStore, _ := getMultiStore()

    ctx := multitenancy.WithTenantID(context.Background(), tenantID)
    store, err := multiStore.GetStore(ctx)
    if err != nil {
        return err
    }

    // Database is auto-created
    // Run tenant-specific setup (projections, read models, etc.)

    return nil
}

// Delete tenant
func DeleteTenant(tenantID string) error {
    // For DatabasePerTenant strategy - just remove file
    dbPath := fmt.Sprintf("./data/tenant_%s.db", tenantID)
    return os.Remove(dbPath)

    // For SharedDatabase strategy - mark for deletion and clean up async
    // (Don't actually delete to maintain audit trail)
}
```

## Examples

### Complete HTTP Handler with Multi-Tenancy

```go
func (h *AccountHandler) OpenAccount(w http.ResponseWriter, r *http.Request) {
    // 1. Extract tenant from request
    tenantID := r.Header.Get("X-Tenant-ID")
    if tenantID == "" {
        http.Error(w, "Missing X-Tenant-ID header", http.StatusBadRequest)
        return
    }

    // 2. Extract principal (from JWT, session, etc.)
    principalID := extractPrincipalID(r)

    // 3. Authorize access
    if err := h.authorizer.Authorize(r.Context(), principalID, tenantID); err != nil {
        http.Error(w, "Unauthorized", http.StatusForbidden)
        return
    }

    // 4. Parse request
    var req OpenAccountRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    // 5. Create tenant context
    ctx := multitenancy.WithTenantID(r.Context(), tenantID)

    // 6. Compose tenant-scoped aggregate ID
    aggregateID := multitenancy.ComposeAggregateID(tenantID, req.AccountID)

    // 7. Execute command via SDK
    _, err := h.sdk.Account.OpenAccount(ctx, &accountv1.OpenAccountCommand{
        AccountId:      aggregateID,
        OwnerName:      req.OwnerName,
        InitialBalance: req.InitialBalance,
    }, principalID)

    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    w.WriteHeader(http.StatusCreated)
    json.NewEncoder(w).Encode(map[string]string{
        "account_id": aggregateID,
        "tenant_id":  tenantID,
    })
}
```

### Query with Tenant Filtering

```go
func (h *AccountHandler) ListAccounts(w http.ResponseWriter, r *http.Request) {
    // Extract tenant
    tenantID := r.Header.Get("X-Tenant-ID")

    // Query projection with tenant filter
    rows, err := h.db.Query(`
        SELECT account_id, owner_name, balance
        FROM account_views
        WHERE tenant_id = ?
        ORDER BY owner_name
    `, tenantID)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    defer rows.Close()

    var accounts []AccountView
    for rows.Next() {
        var account AccountView
        if err := rows.Scan(&account.AccountID, &account.OwnerName, &account.Balance); err != nil {
            http.Error(w, err.Error(), http.StatusInternalServerError)
            return
        }
        accounts = append(accounts, account)
    }

    json.NewEncoder(w).Encode(accounts)
}
```

### Testing Multi-Tenant Logic

```go
func TestMultiTenantIsolation(t *testing.T) {
    // Setup
    multiStore, _ := multitenancy.NewMultiTenantEventStore(multitenancy.MultiTenantConfig{
        Strategy:  multitenancy.SharedDatabase,
        SharedDSN: ":memory:",
    })
    defer multiStore.Close()

    // Tenant A creates account
    tenantACtx := multitenancy.WithTenantID(context.Background(), "tenant-a")
    storeA, _ := multiStore.GetStore(tenantACtx)
    repoA := accountv1.NewAccountRepository(storeA)

    accountA := accountv1.NewAccount(multitenancy.ComposeAggregateID("tenant-a", "acc-001"))
    // ... create account

    // Tenant B creates account with same local ID
    tenantBCtx := multitenancy.WithTenantID(context.Background(), "tenant-b")
    storeB, _ := multiStore.GetStore(tenantBCtx)
    repoB := accountv1.NewAccountRepository(storeB)

    accountB := accountv1.NewAccount(multitenancy.ComposeAggregateID("tenant-b", "acc-001"))
    // ... create account

    // Verify isolation - both should exist independently
    loadedA, err := repoA.Load(multitenancy.ComposeAggregateID("tenant-a", "acc-001"))
    assert.NoError(t, err)
    assert.Equal(t, "Tenant A Owner", loadedA.OwnerName)

    loadedB, err := repoB.Load(multitenancy.ComposeAggregateID("tenant-b", "acc-001"))
    assert.NoError(t, err)
    assert.Equal(t, "Tenant B Owner", loadedB.OwnerName)

    // Verify Tenant A cannot access Tenant B's aggregate
    _, err = repoA.Load(multitenancy.ComposeAggregateID("tenant-b", "acc-001"))
    assert.Error(t, err) // Should fail or return different data
}
```

## Migration Strategy

### Adding Multi-Tenancy to Existing System

**Step 1: Add tenant field to existing data**

```sql
-- Add tenant column to events table
ALTER TABLE events ADD COLUMN tenant_id TEXT;

-- Add tenant column to projections
ALTER TABLE account_views ADD COLUMN tenant_id TEXT;

-- Backfill with default tenant (for migration)
UPDATE events SET tenant_id = 'default' WHERE tenant_id IS NULL;
UPDATE account_views SET tenant_id = 'default' WHERE tenant_id IS NULL;

-- Make tenant_id NOT NULL
ALTER TABLE events ALTER COLUMN tenant_id SET NOT NULL;
```

**Step 2: Update aggregate IDs**

```go
// Migration script
func MigrateToTenantPrefixedIDs(db *sql.DB, defaultTenant string) error {
    rows, err := db.Query("SELECT id, aggregate_id FROM events ORDER BY version")
    if err != nil {
        return err
    }
    defer rows.Close()

    for rows.Next() {
        var id, aggregateID string
        rows.Scan(&id, &aggregateID)

        // Add tenant prefix if not already present
        if !strings.Contains(aggregateID, "::") {
            newAggregateID := multitenancy.ComposeAggregateID(defaultTenant, aggregateID)
            _, err := db.Exec("UPDATE events SET aggregate_id = ? WHERE id = ?", newAggregateID, id)
            if err != nil {
                return err
            }
        }
    }

    return nil
}
```

**Step 3: Deploy with tenant middleware**

Deploy code with tenant extraction middleware that defaults to "default" tenant for legacy clients.

**Step 4: Migrate clients**

Update clients to send `X-Tenant-ID` header or use tenant-specific subdomains.

## Summary

Multi-tenancy in event sourcing requires careful consideration of:

1. **Isolation strategy** - Choose based on tenant count and security requirements
2. **Context propagation** - Always pass tenant through the stack
3. **Middleware enforcement** - Use isolation and authorization middleware
4. **Projection design** - Index and filter by tenant
5. **Testing** - Verify tenant boundaries are enforced

The framework provides building blocks for both strategies, allowing you to choose the right approach for your needs.
