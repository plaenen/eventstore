# Production Readiness Review: pkg/multitenancy

## Overview
This document summarizes the findings from the production readiness review of the `pkg/multitenancy` package and its integration with the CQRS transport layer.

## Critical Issues

### 1. Broken Tenant ID Propagation (Context Keys)
**Severity**: **Critical**
**Location**: `pkg/cqrs/nats/transport.go` and `pkg/cqrs/nats/server.go`
**Description**: The NATS transport and server implementations use a string literal `"tenant_id"` as the context key when getting/setting the tenant ID. However, `pkg/multitenancy/context.go` uses a private type `contextKey` for the key.
**Impact**: `multitenancy.GetTenantID(ctx)` will always fail to find the ID set by the transport layer, breaking tenant isolation.
**Fix**: Update `transport.go` and `server.go` to use `multitenancy.GetTenantID` and `multitenancy.WithTenantID`.

## Major Issues

### 2. Context Propagation in Encryption
**Severity**: **High**
**Location**: `pkg/multitenancy/encrypted_store.go`
**Description**: The `encryptEvent` and `decryptEvent` methods use `context.Background()` when calling `keyProvider.GetCurrentKey` and `keyProvider.GetKey`.
**Impact**: Context cancellation, timeouts, and trace spans from the request are not propagated to the key provider (which might make network calls).
**Fix**: Update `EventStore` interface to accept context in `AppendEvents` (breaking change) or accept the limitation. For now, we should at least document this limitation.

### 3. Race Condition in Store Eviction
**Severity**: **Medium**
**Location**: `pkg/multitenancy/store.go`
**Description**: When `MaxOpenTenants` is reached, the LRU cache evicts and **closes** the oldest store. If that store is currently in use by another goroutine (which obtained it via `GetStore` just before eviction), operations on it will fail.
**Impact**: Intermittent failures under high load with many active tenants.
**Fix**: Implement reference counting or a "close on idle" mechanism instead of immediate close on eviction. Alternatively, rely on `database/sql` connection pooling and don't close the DB handle until the entire service shuts down (if file descriptors allow).

## Minor Issues

### 4. Hardcoded NATS Header Keys
**Severity**: **Low**
**Location**: `pkg/cqrs/nats/transport.go`
**Description**: The header key `"Tenant-ID"` is hardcoded string literal.
**Fix**: Define a constant for this header key.

## Recommendations

1.  **Immediate Fix**: Fix the context key mismatch in `pkg/cqrs/nats`.
2.  **Refactor**: Update `EventStore` interface to include `context.Context` in all methods.
3.  **Enhancement**: Implement reference counting for `MultiTenantEventStore` to safely handle evictions.
