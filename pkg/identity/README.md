# Identity Package

The `identity` package provides services for managing NATS Decentralized Authentication (JWTs) and issuing credentials to clients.

## Overview

This package implements an `IdentityService` that allows clients to exchange a third-party token (e.g., OIDC, custom token) for NATS credentials. This enables a pattern where clients authenticate with your backend, and your backend issues limited-scope NATS credentials for the client to connect directly to the NATS cluster.

## Components

### Service

The `Service` is a ConnectRPC handler that implements the `IdentityService` definition. It exposes:

- `ExchangeToken`: Validates a request and issues a NATS User JWT and Credentials file.

### AccountManager

The `AccountManager` handles the low-level NATS JWT operations:

- **CreateAccount**: Creates a new NATS Account, signs it with an Operator Key, and publishes it to the NATS Resolver.
- **CreateUser**: Creates a new NATS User within an Account, signs it with the Account Key, and generates a Credentials file.

### KeyStore

The `KeyStore` interface defines how private keys (seeds) are stored.

- **FileKeyStore**: A file-based implementation suitable for development or simple deployments. It stores seeds in a secure directory with restricted permissions (0600) and encrypts them at rest.
- **SQLiteKeyStore**: A SQLite-based implementation suitable for production (e.g., using Turso). It stores seeds in a `identity_seeds` table, encrypted at rest.
- **MemoryKeyStore**: An in-memory implementation for testing.

## Usage

### Initialization

```go
import (
    "database/sql"
    "os"

    "github.com/plaenen/eventstore/pkg/identity"
    "github.com/plaenen/eventstore/pkg/identity/nats"
    "github.com/plaenen/eventstore/pkg/identity/store/sqlite"
    "github.com/plaenen/eventstore/pkg/runner"
    "github.com/plaenen/eventstore/pkg/security/encryption"
    _ "modernc.org/sqlite"
)

func main() {
    logger := runner.NewLogger() // Your logger implementation
    nc, _ := nats.Connect("nats://localhost:4222")
    ctx := context.Background()

    // Initialize Encryption Service (for KeyStore)
    // In production, load key from a secret manager
    encKey, _ := encryption.GenerateKey(32)
    encService, _ := encryption.NewService(encKey)

    // Option 1: FileKeyStore
    // keyStore, _ := nats.NewFileKeyStore("/path/to/keystore", encService)

    // Option 2: SQLiteKeyStore (e.g., with Turso)
    db, _ := sql.Open("sqlite", "file:data.db")
    keyStore, _ := sqlite.NewSQLiteKeyStore(ctx, db, encService)

    // Initialize AccountManager
    // operatorSeed should be loaded from a secure location (env var, secret manager)
    operatorSeed := os.Getenv("NATS_OPERATOR_SEED")
    manager := nats.NewAccountManager(operatorSeed, keyStore, nc, logger)

    // Initialize Service
    svc := identity.NewService(manager, logger)

    // Register with ConnectRPC...
}
```

### Security Considerations

- **Operator Seed**: The Operator Seed is highly sensitive. It allows creating new Accounts. Ensure it is stored securely.
- **KeyStore**: The `KeyStore` stores Account private keys. These keys allow creating Users and signing JWTs. Access to the `KeyStore` storage must be restricted.
- **Permissions**: The `AccountManager` currently grants broad permissions to created users. In a production environment, you should refine the `pubAllow` and `subAllow` lists in `CreateUser` to follow the Principle of Least Privilege.
