# Security Package

**Secure credential management for the Event Sourcing Framework**

[![Security Badge](https://img.shields.io/badge/security-SEC--001-blue)](../../docs/security/SEC-001-secure-credentials.md)

This package provides enterprise-grade credential management using [Go Cloud Development Kit](https://gocloud.dev/howto/secrets/), enabling secure storage and retrieval of sensitive data across multiple cloud providers and local environments.

## Table of Contents

- [Quick Start](#quick-start)
- [Provider Types](#provider-types)
- [Usage Examples](#usage-examples)
- [Production Deployment](#production-deployment)
- [Security Best Practices](#security-best-practices)
- [Migration Guide](#migration-guide)
- [API Reference](#api-reference)
- [Troubleshooting](#troubleshooting)

## Quick Start

### Installation

The credentials package is part of the eventsourcing framework. Import the providers you need:

```go
import (
    "github.com/plaenen/eventstore/pkg/security/credentials"

    // Cloud provider imports (choose what you need):
    _ "gocloud.dev/secrets/awskms"        // AWS Secrets Manager
    _ "gocloud.dev/secrets/gcpkms"        // GCP Secret Manager
    _ "gocloud.dev/secrets/azurekeyvault" // Azure Key Vault
    _ "gocloud.dev/secrets/hashivault"    // HashiCorp Vault
    _ "gocloud.dev/secrets/localsecrets"  // Local development
)
```

### Basic Usage

```go
ctx := context.Background()

// Development: Environment variables
provider := credentials.NewEnvTokenProvider("NATS_TOKEN", 5*time.Minute)
defer provider.Close()

// Get credentials
creds, err := provider.GetCredentials(ctx)
if err != nil {
    log.Fatal(err)
}

// Use with transport
transport, err := natscqrs.NewTransport(&natscqrs.TransportConfig{
    URL:                "nats://localhost:4222",
    CredentialProvider: provider, // ✅ Secure
})
```

## Provider Types

### Overview

| Provider | Use Case | Security Level | Rotation Support |
|----------|----------|----------------|------------------|
| `SecretProvider` | Production (Cloud) | ⭐⭐⭐⭐⭐ High | ✅ Yes (automatic) |
| `EnvProvider` | CI/CD, Containers | ⭐⭐⭐⭐ Good | ✅ Yes (manual) |
| `ChainProvider` | Fallback scenarios | ⭐⭐⭐⭐ Good | ✅ Yes |
| `StaticProvider` | Development only | ⭐ Low | ❌ No |

### 1. SecretProvider (Recommended for Production)

Uses Go Cloud Development Kit for vendor-agnostic secret management.

**Supported Backends:**
- AWS Secrets Manager
- GCP Secret Manager
- Azure Key Vault
- HashiCorp Vault
- Local file encryption (development)

**Features:**
- Automatic credential rotation
- Built-in caching with configurable TTL
- Background refresh
- Thread-safe

**Example:**

```go
// AWS Secrets Manager
provider, err := credentials.NewSecretProvider(ctx,
    "awskms://arn:aws:secretsmanager:us-east-1:123456789:secret:nats-credentials")

// GCP Secret Manager
provider, err := credentials.NewSecretProvider(ctx,
    "gcpkms://projects/my-project/secrets/nats-creds/versions/latest")

// Azure Key Vault
provider, err := credentials.NewSecretProvider(ctx,
    "azurekeyvault://my-vault.vault.azure.net/secrets/nats-creds")

// Local (development)
provider, err := credentials.NewSecretProvider(ctx,
    "base64key://smGbjm71Nxd1Ig5FS0wj9SlbzAIrnolCz9bQQ6uAhl4=")
```

### 2. EnvProvider (Recommended for CI/CD)

Reads credentials from environment variables at runtime.

**Best for:**
- Kubernetes secrets
- Docker containers
- CI/CD pipelines
- 12-factor apps

**Example:**

```go
// Token from environment
provider := credentials.NewEnvTokenProvider("NATS_TOKEN", 5*time.Minute)

// Username/password from environment
provider := credentials.NewEnvUserPasswordProvider("NATS_USER", "NATS_PASS")
```

### 3. ChainProvider (Recommended for Flexibility)

Tries multiple providers in order, falling back on failure.

**Example:**

```go
provider := credentials.NewChainProvider(
    // Try cloud secret manager first
    credentials.NewSecretProvider(ctx, "awskms://..."),
    // Fall back to environment variables
    credentials.NewEnvTokenProvider("NATS_TOKEN", 5*time.Minute),
    // Last resort: static (dev only)
    credentials.NewStaticTokenProvider("dev-token", 24*time.Hour),
)
defer provider.Close()
```

### 4. StaticProvider (Development Only)

⚠️ **WARNING: Never use in production!**

Provides credentials from static values. Useful only for local development.

**Example:**

```go
// ⚠️ DEVELOPMENT ONLY
provider := credentials.NewStaticTokenProvider("dev-token-12345", 24*time.Hour)
provider := credentials.NewStaticUserPasswordProvider("user", "pass")
```

## Usage Examples

### Example 1: NATS Transport with Secure Credentials

```go
package main

import (
    "context"
    "log"
    "time"

    natscqrs "github.com/plaenen/eventstore/pkg/cqrs/nats"
    "github.com/plaenen/eventstore/pkg/security/credentials"
    _ "gocloud.dev/secrets/awskms"
)

func main() {
    ctx := context.Background()

    // Load credentials from AWS Secrets Manager
    provider, err := credentials.NewSecretProvider(ctx,
        "awskms://arn:aws:secretsmanager:us-east-1:123456:secret:nats-prod-creds")
    if err != nil {
        log.Fatalf("Failed to create provider: %v", err)
    }
    defer provider.Close()

    // Create transport with secure credentials
    transport, err := natscqrs.NewTransport(&natscqrs.TransportConfig{
        URL:                "nats://production.example.com:4222",
        CredentialProvider: provider,
        Name:               "my-service",
    })
    if err != nil {
        log.Fatalf("Failed to create transport: %v", err)
    }
    defer transport.Close()

    log.Printf("Connected to NATS: %s", transport.ConnectedURL())
}
```

### Example 2: Multiple Credential Types

```go
// Token authentication
tokenCreds := &credentials.Credentials{
    Type:  credentials.CredentialTypeToken,
    Token: "your-secret-token",
    ExpiresAt: &expiryTime,
}

// Username/Password authentication
userPassCreds := &credentials.Credentials{
    Type:     credentials.CredentialTypeUserPassword,
    User:     "admin",
    Password: "secure-password",
}

// NKey authentication (NATS)
nkeyCreds := &credentials.Credentials{
    Type:      credentials.CredentialTypeNKey,
    PublicKey: "UABC...",
    Seed:      "SUABC...",
}

// JWT authentication
jwtCreds := &credentials.Credentials{
    Type:     credentials.CredentialTypeJWT,
    JWTToken: "eyJhbGciOiJIUzI1NiIs...",
}

// mTLS authentication
mtlsCreds := &credentials.Credentials{
    Type:    credentials.CredentialTypeMTLS,
    CertPEM: "-----BEGIN CERTIFICATE-----\n...",
    KeyPEM:  "-----BEGIN PRIVATE KEY-----\n...",
}
```

### Example 3: Credential Rotation

```go
provider, err := credentials.NewSecretProvider(ctx, secretURL)
if err != nil {
    log.Fatal(err)
}
defer provider.Close()

// Manual rotation
if err := provider.Rotate(ctx); err != nil {
    log.Printf("Rotation failed: %v", err)
}

// Automatic rotation is enabled by default with:
config := credentials.DefaultConfig()
// config.AutoRefresh = true
// config.RefreshInterval = 2.5 * time.Minute
provider, err := credentials.NewSecretProviderWithConfig(ctx, secretURL, config)
```

### Example 4: Custom Configuration

```go
config := credentials.ProviderConfig{
    URL:             "awskms://...",
    CacheTTL:        10 * time.Minute,      // Cache for 10 minutes
    AutoRefresh:     true,                   // Enable auto-refresh
    RefreshInterval: 5 * time.Minute,        // Refresh every 5 minutes
}

provider, err := credentials.NewSecretProviderWithConfig(ctx, config.URL, config)
```

### Example 5: Storing Credentials

```go
// Create credentials
creds := &credentials.Credentials{
    Type:  credentials.CredentialTypeToken,
    Token: "production-secret-token",
    Metadata: map[string]string{
        "environment": "production",
        "service":     "nats",
    },
}

// Validate
if err := creds.Validate(); err != nil {
    log.Fatalf("Invalid credentials: %v", err)
}

// Store in secret backend
err := credentials.StoreCredentials(ctx,
    "awskms://arn:aws:secretsmanager:us-east-1:123:secret:my-secret",
    creds)
if err != nil {
    log.Fatalf("Failed to store: %v", err)
}
```

## Production Deployment

### AWS Secrets Manager

```go
package main

import (
    "context"
    "log"

    "github.com/plaenen/eventstore/pkg/security/credentials"
    _ "gocloud.dev/secrets/awskms"
)

func main() {
    ctx := context.Background()

    // ARN format: arn:aws:secretsmanager:REGION:ACCOUNT:secret:NAME
    provider, err := credentials.NewSecretProvider(ctx,
        "awskms://arn:aws:secretsmanager:us-east-1:123456789012:secret:prod/nats-credentials-AbCdEf")
    if err != nil {
        log.Fatal(err)
    }
    defer provider.Close()

    creds, err := provider.GetCredentials(ctx)
    if err != nil {
        log.Fatal(err)
    }

    log.Printf("Loaded credentials: type=%s", creds.Type)
}
```

**Setup:**

1. **Create secret in AWS:**
```bash
aws secretsmanager create-secret \
    --name prod/nats-credentials \
    --description "NATS production credentials" \
    --secret-string file://secret.json
```

2. **Grant IAM permissions:**
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "secretsmanager:GetSecretValue",
        "secretsmanager:DescribeSecret"
      ],
      "Resource": "arn:aws:secretsmanager:*:*:secret:prod/nats-credentials-*"
    }
  ]
}
```

3. **Secret format (secret.json):**
```json
{
  "credentials": {
    "type": "token",
    "token": "your-secret-token",
    "metadata": {
      "environment": "production"
    }
  },
  "version": 1,
  "created_at": "2024-01-15T10:00:00Z"
}
```

### GCP Secret Manager

```go
provider, err := credentials.NewSecretProvider(ctx,
    "gcpkms://projects/my-project-123/secrets/nats-credentials/versions/latest")
```

**Setup:**

```bash
# Create secret
gcloud secrets create nats-credentials \
    --data-file=secret.json \
    --replication-policy=automatic

# Grant access
gcloud secrets add-iam-policy-binding nats-credentials \
    --member="serviceAccount:my-service@my-project.iam.gserviceaccount.com" \
    --role="roles/secretmanager.secretAccessor"
```

### Azure Key Vault

```go
provider, err := credentials.NewSecretProvider(ctx,
    "azurekeyvault://my-vault-name.vault.azure.net/secrets/nats-credentials")
```

**Setup:**

```bash
# Create secret
az keyvault secret set \
    --vault-name my-vault-name \
    --name nats-credentials \
    --file secret.json

# Grant access
az keyvault set-policy \
    --name my-vault-name \
    --object-id <service-principal-id> \
    --secret-permissions get list
```

### Kubernetes

Use environment variables with Kubernetes secrets:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: nats-credentials
type: Opaque
stringData:
  NATS_TOKEN: "your-secret-token"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-service
spec:
  template:
    spec:
      containers:
      - name: app
        env:
        - name: NATS_TOKEN
          valueFrom:
            secretKeyRef:
              name: nats-credentials
              key: NATS_TOKEN
```

**Application code:**

```go
provider := credentials.NewEnvTokenProvider("NATS_TOKEN", 5*time.Minute)
```

## Security Best Practices

### ✅ DO

1. **Use cloud secret managers in production**
   ```go
   provider, err := credentials.NewSecretProvider(ctx, "awskms://...")
   ```

2. **Use environment variables for CI/CD**
   ```go
   provider := credentials.NewEnvTokenProvider("SECRET_TOKEN", ttl)
   ```

3. **Enable automatic rotation**
   ```go
   config := credentials.DefaultConfig()
   config.AutoRefresh = true
   provider, _ := credentials.NewSecretProviderWithConfig(ctx, url, config)
   ```

4. **Set appropriate cache TTLs**
   ```go
   config.CacheTTL = 5 * time.Minute // Balance security vs performance
   ```

5. **Close providers when done**
   ```go
   defer provider.Close() // Always clean up
   ```

6. **Validate credentials before use**
   ```go
   if err := creds.Validate(); err != nil {
       log.Fatal(err)
   }
   ```

### ❌ DON'T

1. **Never hardcode credentials**
   ```go
   // ❌ BAD
   Token: "my-secret-token"

   // ✅ GOOD
   CredentialProvider: provider
   ```

2. **Never commit secrets to version control**
   ```bash
   # Add to .gitignore
   .env
   *.pem
   *.key
   secrets/
   ```

3. **Never use static providers in production**
   ```go
   // ❌ BAD - Development only!
   provider := credentials.NewStaticTokenProvider(token, ttl)
   ```

4. **Never log sensitive credentials**
   ```go
   // ✅ GOOD - Credentials.MarshalJSON() redacts sensitive fields
   log.Printf("Credentials: %+v", creds) // Safe - passwords redacted
   ```

5. **Never share credentials between environments**
   - Dev, staging, and production must use separate credentials
   - Rotate immediately if credentials are exposed

6. **Never set long cache TTLs**
   ```go
   // ❌ BAD
   CacheTTL: 24 * time.Hour

   // ✅ GOOD
   CacheTTL: 5 * time.Minute
   ```

## Migration Guide

### From Plaintext to Secure Credentials

#### Before (Insecure)

```go
transport, err := natscqrs.NewTransport(&natscqrs.TransportConfig{
    URL:   "nats://production:4222",
    Token: "my-secret-token", // ❌ Plaintext in code!
})
```

#### After (Secure)

```go
// Option 1: Environment variables
provider := credentials.NewEnvTokenProvider("NATS_TOKEN", 5*time.Minute)

transport, err := natscqrs.NewTransport(&natscqrs.TransportConfig{
    URL:                "nats://production:4222",
    CredentialProvider: provider, // ✅ Secure!
})
```

```go
// Option 2: Cloud secret manager
provider, _ := credentials.NewSecretProvider(ctx, "awskms://...")

transport, err := natscqrs.NewTransport(&natscqrs.TransportConfig{
    URL:                "nats://production:4222",
    CredentialProvider: provider, // ✅ Even better!
})
```

### Migration Steps

1. **Identify hardcoded credentials**
   ```bash
   grep -r "Token:" . --include="*.go"
   grep -r "Password:" . --include="*.go"
   ```

2. **Store credentials securely**
   ```bash
   # AWS example
   aws secretsmanager create-secret \
       --name my-service/credentials \
       --secret-string file://secret.json
   ```

3. **Update application code**
   - Replace `Token` with `CredentialProvider`
   - Add provider initialization

4. **Test in development**
   ```go
   provider := credentials.NewEnvTokenProvider("DEV_TOKEN", 5*time.Minute)
   ```

5. **Deploy to production**
   ```go
   provider, _ := credentials.NewSecretProvider(ctx, "awskms://...")
   ```

6. **Verify and rotate**
   - Test connectivity
   - Rotate old credentials
   - Remove old plaintext configs

## API Reference

### Provider Interface

```go
type Provider interface {
    // GetCredentials retrieves the current credentials
    GetCredentials(ctx context.Context) (*Credentials, error)

    // Rotate triggers credential rotation (if supported)
    Rotate(ctx context.Context) error

    // Type returns the credential type this provider manages
    Type() CredentialType

    // Close releases any resources held by the provider
    Close() error
}
```

### Credentials Types

```go
const (
    CredentialTypeToken        CredentialType = "token"
    CredentialTypeNKey         CredentialType = "nkey"
    CredentialTypeJWT          CredentialType = "jwt"
    CredentialTypeUserPassword CredentialType = "user_password"
    CredentialTypeMTLS         CredentialType = "mtls"
)
```

### Credentials Struct

```go
type Credentials struct {
    Type      CredentialType
    Token     string
    User      string
    Password  string
    PublicKey string
    Seed      string
    JWTToken  string
    CertPEM   string
    KeyPEM    string
    ExpiresAt *time.Time
    Metadata  map[string]string
}
```

**Methods:**

- `IsExpired() bool` - Check if credentials have expired
- `Validate() error` - Validate credentials for their type
- `MarshalJSON() ([]byte, error)` - Safely marshal (redacts sensitive fields)

### Configuration

```go
type ProviderConfig struct {
    URL             string
    CacheTTL        time.Duration
    AutoRefresh     bool
    RefreshInterval time.Duration
}
```

**Defaults:**

- `CacheTTL`: 5 minutes
- `AutoRefresh`: true
- `RefreshInterval`: 2.5 minutes (50% of CacheTTL)

### Errors

```go
var (
    ErrCredentialsExpired = errors.New("credentials expired")
    ErrInvalidCredentials = errors.New("invalid credentials")
    ErrProviderClosed     = errors.New("provider is closed")
)
```

## Troubleshooting

### Common Issues

#### 1. "Provider is closed"

**Problem:** Attempting to use a provider after calling `Close()`.

**Solution:**
```go
// Ensure Close() is only called once using defer
defer provider.Close()

// Or check before use
creds, err := provider.GetCredentials(ctx)
if errors.Is(err, credentials.ErrProviderClosed) {
    // Recreate provider
}
```

#### 2. "Credentials expired"

**Problem:** Credentials have passed their expiration time.

**Solution:**
```go
creds, err := provider.GetCredentials(ctx)
if errors.Is(err, credentials.ErrCredentialsExpired) {
    // Trigger rotation
    if err := provider.Rotate(ctx); err != nil {
        log.Fatal(err)
    }
    creds, err = provider.GetCredentials(ctx)
}
```

#### 3. "Failed to open secret keeper"

**Problem:** Invalid secret URL or missing cloud provider import.

**Solution:**
```go
// Ensure correct import
import _ "gocloud.dev/secrets/awskms"

// Verify URL format
url := "awskms://arn:aws:secretsmanager:us-east-1:123:secret:name"
//       ^^^^^^^ Scheme must match imported provider
```

#### 4. "Failed to decrypt secret"

**Problem:** Insufficient permissions or incorrect secret format.

**Solution:**
```bash
# AWS: Check IAM permissions
aws secretsmanager get-secret-value --secret-id my-secret

# Verify secret format
{
  "credentials": {
    "type": "token",
    "token": "..."
  },
  "version": 1,
  "created_at": "2024-01-15T10:00:00Z"
}
```

#### 5. Environment variable not set

**Problem:** `EnvProvider` can't find required environment variable.

**Solution:**
```bash
# Set environment variable
export NATS_TOKEN="your-token-here"

# Or in code, check before creating provider
if os.Getenv("NATS_TOKEN") == "" {
    log.Fatal("NATS_TOKEN environment variable not set")
}
```

### Debug Logging

Enable debug logging to troubleshoot issues:

```go
// Add logging around provider operations
log.Printf("Creating provider with URL: %s", url)
provider, err := credentials.NewSecretProvider(ctx, url)
if err != nil {
    log.Printf("Provider creation failed: %v", err)
}

log.Printf("Getting credentials...")
creds, err := provider.GetCredentials(ctx)
if err != nil {
    log.Printf("Failed to get credentials: %v", err)
}

log.Printf("Credentials loaded: type=%s", creds.Type)
```

### Testing

#### Unit Tests

```go
func TestMyService(t *testing.T) {
    // Use static provider for tests
    provider := credentials.NewStaticTokenProvider("test-token", 1*time.Hour)
    defer provider.Close()

    service := NewService(provider)
    // Test service...
}
```

#### Integration Tests

```go
func TestIntegration(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test")
    }

    // Use real provider
    ctx := context.Background()
    provider, err := credentials.NewSecretProvider(ctx,
        os.Getenv("TEST_SECRET_URL"))
    if err != nil {
        t.Fatalf("Failed to create provider: %v", err)
    }
    defer provider.Close()

    // Run integration test...
}
```

## Additional Resources

- [SEC-001 Security Specification](../../docs/security/SEC-001-secure-credentials.md)
- [Go Cloud Development Kit Documentation](https://gocloud.dev/howto/secrets/)
- [Example Application](../../examples/cmd/secure-credentials/main.go)
- [Unit Tests](./credentials/provider_test.go)

## Support

For issues or questions:

1. Check [Troubleshooting](#troubleshooting) section
2. Review [examples](../../examples/cmd/secure-credentials/)
3. Open an issue on GitHub

## License

See the main project LICENSE file.
