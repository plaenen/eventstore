# Local-First Event Sourcing with SQLite/LibSQL

This package supports a "local-first" deployment model using [LibSQL](https://github.com/tursodatabase/go-libsql) embedded replicas. This architecture combines the low latency of a local SQLite database with the durability and synchronization capabilities of a cloud database.

## Architecture

In a local-first setup, your application reads from and writes to a local SQLite file. A background process (managed by the LibSQL driver) automatically synchronizes this local file with a primary database in the cloud (e.g., Turso).

```mermaid
graph TD
    App[Application] <-->|Read/Write (Fast)| LocalDB[(Local SQLite File)]
    LocalDB <-->|Async Sync| CloudDB[(Cloud Primary)]
    CloudDB <-->|Sync| OtherReplica[(Other Replicas)]
```

### Benefits
*   **Zero Latency Reads:** Reads are served immediately from the local disk.
*   **Offline Capability:** The application can continue to operate even if the network is down. Writes are queued locally and synced when connectivity is restored.
*   **Edge Ready:** Ideal for deploying to edge locations (like Fly.io regions) close to users.

### Trade-offs
*   **Eventual Consistency:** While writes are immediately visible to the local instance, other replicas will see them only after synchronization.
*   **Conflict Resolution:** If multiple replicas modify the same data offline, conflicts can occur. Event Sourcing mitigates this naturally:
    *   **Append-Only:** We only append events, we don't update existing rows.
    *   **Optimistic Concurrency:** If two replicas append an event with `Version: 5` for the same aggregate, the cloud primary will reject the second one during sync. The local replica will then receive an error and must handle it (e.g., by retrying or notifying the user).

## Configuration

To enable embedded replicas, use the `WithLibSQLEmbeddedReplica` option when creating the event store.

```go
import (
    "time"
    "github.com/plaenen/eventstore/pkg/eventsourcing/store/sqlite"
)

func main() {
    store, err := sqlite.NewEventStore(
        // Enable embedded replica
        sqlite.WithLibSQLEmbeddedReplica(
            "./data/local.db",          // Local file path
            "libsql://mydb.turso.io",   // Remote URL
            os.Getenv("TURSO_TOKEN"),   // Auth token
        ),
        // Optional: Configure sync interval (default is usually 1 minute)
        sqlite.WithSyncInterval(5 * time.Second),
        // Optional: Enable encryption at rest
        sqlite.WithEncryption("my-secret-key"),
    )
    if err != nil {
        panic(err)
    }
    defer store.Close()
}
```

## Handling Sync Conflicts

When using embedded replicas, `AppendEvents` might succeed locally but fail later during sync if another replica has already modified the aggregate.

Currently, the LibSQL driver handles sync in the background. If a sync conflict occurs (e.g., primary rejects a write), the local database state may need to be reconciled.

**Best Practice:**
For critical write operations where immediate consistency is required, consider connecting directly to the remote primary using `WithLibSQLRemote` instead of using an embedded replica. Use embedded replicas primarily for read-heavy workloads or where offline tolerance is more important than immediate global consistency.
