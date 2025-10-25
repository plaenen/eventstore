# Secure Credentials Example

This example demonstrates **SEC-001: Secure Credential Management** - one of the critical security fixes identified in the security roadmap.

## What This Example Shows

✅ **Secure credential storage** using Go Cloud Development Kit
✅ **Multiple provider types** (static, environment, encrypted, chain)
✅ **Production-ready patterns** for AWS, GCP, Azure, Vault
✅ **Backward compatibility** with deprecated fields
✅ **Migration path** from plaintext to secure credentials

## Running the Example

```bash
go run ./examples/cmd/secure-credentials/main.go
```

## Providers Demonstrated

### 1. Static Token Provider (Development Only)
```go
provider := credentials.NewStaticTokenProvider("token", 24*time.Hour)
```
⚠️ **WARNING:** Use only for local development!

### 2. Environment Variable Provider (CI/CD)
```go
os.Setenv("NATS_TOKEN", "secret")
provider := credentials.NewEnvTokenProvider("NATS_TOKEN", 5*time.Minute)
```
✅ Better than static - credentials injected at runtime

### 3. Local Encrypted Secrets (Development)
```go
// Random key (new each run)
provider, _ := credentials.NewSecretProvider(ctx, "base64key://")

// Fixed key (persistent encryption)
provider, _ := credentials.NewSecretProvider(ctx,
    "base64key://smGbjm71Nxd1Ig5FS0wj9SlbzAIrnolCz9bQQ6uAhl4=")
```
✅ Recommended for development - uses NaCl secretbox encryption

### 4. Chain Provider (Production Pattern)
```go
provider := credentials.NewChainProvider(
    envProvider,        // Try environment first
    staticProvider,     // Fall back to static
)
```
✅ Production pattern - graceful fallback

### 5. Cloud Providers (Production)

#### AWS Secrets Manager
```go
provider, _ := credentials.NewSecretProvider(ctx,
    "awskms://arn:aws:secretsmanager:us-east-1:123456:secret:nats-creds")
```

#### GCP Secret Manager
```go
provider, _ := credentials.NewSecretProvider(ctx,
    "gcpkms://projects/PROJECT/secrets/nats-creds/versions/latest")
```

#### Azure Key Vault
```go
provider, _ := credentials.NewSecretProvider(ctx,
    "azurekeyvault://vault-name.vault.azure.net/secrets/nats-creds")
```

#### HashiCorp Vault
```go
provider, _ := credentials.NewSecretProvider(ctx,
    "hashivault://vault.example.com:8200/secret/data/nats")
```

## Migration Guide

### Before (INSECURE ❌)
```go
transport, _ := nats.NewTransport(&nats.TransportConfig{
    URL:   "nats://localhost:4222",
    Token: "my-secret-token",  // Plaintext!
})
```

### After (SECURE ✅)
```go
provider := credentials.NewEnvTokenProvider("NATS_TOKEN", 5*time.Minute)
defer provider.Close()

transport, _ := nats.NewTransport(&nats.TransportConfig{
    URL:                "nats://localhost:4222",
    CredentialProvider: provider,  // Secure!
})
defer transport.Close()
```

## Local Development Setup

### Option 1: Environment Variables (Recommended)
```bash
# Set environment variable
export NATS_TOKEN="your-development-token"

# Run application
go run ./examples/cmd/secure-credentials/main.go
```

### Option 2: Local Encrypted Storage
```bash
# Generate a random base64 key (32 bytes)
openssl rand -base64 32

# Store in .env file (DO NOT COMMIT!)
echo "ENCRYPTION_KEY=<generated-key>" >> .env

# Use in your code
provider, _ := credentials.NewSecretProvider(ctx,
    fmt.Sprintf("base64key://%s", os.Getenv("ENCRYPTION_KEY")))
```

## Production Setup

### AWS Secrets Manager

1. **Create Secret:**
```bash
aws secretsmanager create-secret \
    --name nats-credentials \
    --description "NATS authentication token" \
    --secret-string '{"type":"token","token":"production-token"}'
```

2. **Grant Access:**
```bash
aws iam attach-role-policy \
    --role-name your-app-role \
    --policy-arn arn:aws:iam::aws:policy/SecretsManagerReadWrite
```

3. **Use in Code:**
```go
import _ "gocloud.dev/secrets/awskms"  // Enable AWS provider

provider, _ := credentials.NewSecretProvider(ctx,
    "awskms://arn:aws:secretsmanager:us-east-1:123456:secret:nats-credentials")
```

### GCP Secret Manager

1. **Create Secret:**
```bash
echo -n '{"type":"token","token":"production-token"}' | \
    gcloud secrets create nats-credentials --data-file=-
```

2. **Grant Access:**
```bash
gcloud secrets add-iam-policy-binding nats-credentials \
    --member="serviceAccount:your-app@project.iam.gserviceaccount.com" \
    --role="roles/secretmanager.secretAccessor"
```

3. **Use in Code:**
```go
import _ "gocloud.dev/secrets/gcpkms"  // Enable GCP provider

provider, _ := credentials.NewSecretProvider(ctx,
    "gcpkms://projects/PROJECT/secrets/nats-credentials/versions/latest")
```

## Secret Format

Secrets should be JSON with this structure:

```json
{
  "credentials": {
    "type": "token",
    "token": "your-secret-token",
    "expires_at": "2025-12-31T23:59:59Z",
    "metadata": {
      "environment": "production",
      "created_by": "admin"
    }
  },
  "version": 1,
  "created_at": "2025-10-25T10:00:00Z"
}
```

### Credential Types

**Token:**
```json
{"type": "token", "token": "bearer-token"}
```

**Username/Password:**
```json
{"type": "user_password", "user": "admin", "password": "secret"}
```

**NATS NKey:**
```json
{"type": "nkey", "public_key": "...", "seed": "..."}
```

## Configuration

### Cache TTL
```go
config := credentials.ProviderConfig{
    CacheTTL:        5 * time.Minute,
    AutoRefresh:     true,
    RefreshInterval: 2*time.Minute + 30*time.Second,
}

provider, _ := credentials.NewSecretProviderWithConfig(ctx, url, config)
```

### Rotation
```go
// Automatic rotation (recommended)
config.AutoRefresh = true
provider, _ := credentials.NewSecretProviderWithConfig(ctx, url, config)

// Manual rotation
provider.Rotate(ctx)
```

## Best Practices

1. ✅ **Use environment variables for CI/CD**
   - Easy to inject in pipelines
   - No files to manage
   - Works everywhere

2. ✅ **Use cloud secret managers for production**
   - Built-in rotation
   - Audit logs
   - IAM integration

3. ✅ **Use local encryption for development**
   - Better than plaintext
   - No cloud costs
   - Works offline

4. ❌ **Never commit credentials to version control**
   - Use `.env` files (add to `.gitignore`)
   - Use secret managers
   - Rotate if exposed

5. ✅ **Rotate credentials regularly**
   - Automate with secret manager
   - Set short TTLs
   - Monitor expiration

6. ✅ **Use chain providers for fallback**
   - Graceful degradation
   - Development/production parity
   - Easy testing

7. ✅ **Set appropriate cache TTLs**
   - Balance performance vs security
   - Shorter TTL = more API calls
   - Longer TTL = stale credentials risk

## Testing

### Unit Tests
```go
func TestCredentialProvider(t *testing.T) {
    provider := credentials.NewStaticTokenProvider("test-token", 1*time.Hour)
    defer provider.Close()

    creds, err := provider.GetCredentials(context.Background())
    assert.NoError(t, err)
    assert.Equal(t, "test-token", creds.Token)
}
```

### Integration Tests
```bash
# Set test environment
export NATS_TOKEN="test-token"

# Run tests
go test ./pkg/security/credentials/...
```

## Troubleshooting

### Error: "failed to get credentials"
- Check environment variables are set
- Verify secret manager permissions
- Check network connectivity

### Error: "credentials expired"
- Set longer TTL
- Enable auto-refresh
- Check system clock

### Error: "unsupported credential type"
- Verify secret format
- Check type field matches provider
- See credential types above

## Security Considerations

⚠️ **DO:**
- Use strong encryption keys (32 bytes minimum)
- Rotate credentials regularly
- Use IAM roles (don't use long-lived credentials)
- Enable audit logging
- Monitor for suspicious activity

❌ **DON'T:**
- Store credentials in code
- Commit `.env` files
- Use production credentials in development
- Share credentials in chat/email
- Log credentials (even encrypted)

## Next Steps

1. ✅ Set up environment variables for local development
2. ✅ Configure cloud secret manager for production
3. ✅ Migrate existing code from Token/Pass to CredentialProvider
4. ✅ Add credential rotation policies
5. ✅ Enable audit logging
6. ✅ Test failover scenarios

## Related Documentation

- [Security Roadmap](../../../docs/SECURITY_ROADMAP.md) - Complete security plan
- [Immediate Actions](../../../docs/security/IMMEDIATE_ACTIONS.md) - Critical fixes
- [Go Cloud Secrets](https://gocloud.dev/howto/secrets/) - Official documentation

## Status

✅ **SEC-001: IMPLEMENTED**

This example demonstrates the solution to SEC-001 (Plaintext Credentials) from the security roadmap.

**Before:** Credentials stored as plaintext strings
**After:** Secure credential management with Go Cloud CDK
**Impact:** Eliminates credential exposure risk

---

**Questions?** See the [Security Review Summary](../../../docs/REVIEW_SUMMARY.md) for context.
