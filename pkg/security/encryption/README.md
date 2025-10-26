# Encryption at Rest Package

Comprehensive data encryption at rest for the EventSourcing framework.

**Security Implementation**: This package implements **SEC-103 (Data Encryption at Rest)** from the security roadmap, providing enterprise-grade encryption for events, snapshots, and sensitive data.

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Features](#features)
- [Usage Examples](#usage-examples)
- [Key Management](#key-management)
- [Event Encryption](#event-encryption)
- [Production Deployment](#production-deployment)
- [Security Best Practices](#security-best-practices)
- [API Reference](#api-reference)
- [Troubleshooting](#troubleshooting)

## Overview

The encryption package provides:

✅ **AES-256-GCM Encryption** - Authenticated encryption with associated data
✅ **Key Management** - Rotation, versioning, and lifecycle management
✅ **Field-Level Encryption** - Encrypt only sensitive fields
✅ **Full Data Encryption** - Encrypt entire event payloads
✅ **Password-Based Encryption** - Argon2id/PBKDF2 key derivation
✅ **Key Rotation** - Zero-downtime key rotation support
✅ **Event Integration** - Seamless event encryption/decryption

### Why Encrypt at Rest?

| Threat | Without Encryption | With Encryption |
|--------|-------------------|-----------------|
| **Database Breach** | ❌ All data exposed | ✅ Data protected |
| **Backup Theft** | ❌ Plaintext data | ✅ Encrypted backups |
| **Insider Threat** | ❌ Full access | ✅ Key-based access control |
| **Compliance** | ❌ GDPR/HIPAA violations | ✅ Compliance met |

## Quick Start

### 1. Basic Encryption

```go
package main

import (
    "github.com/plaenen/eventstore/pkg/security/encryption"
)

func main() {
    // Generate encryption key
    masterKey, _ := encryption.GenerateKey(32)

    // Create encryption service
    service, _ := encryption.NewService(masterKey)

    // Encrypt data
    ciphertext, _ := service.EncryptString("sensitive data")

    // Decrypt data
    plaintext, _ := service.DecryptString(ciphertext)
}
```

### 2. Password-Based Encryption

```go
// Generate salt
salt, _ := encryption.GenerateSalt()

// Create service with password
password := "my-secure-password"
service, _ := encryption.NewServiceWithPassword(password, salt)

// Use normally
ciphertext, _ := service.EncryptString("secret")
```

### 3. Event Encryption

```go
// Create event encryptor
eventEnc := encryption.NewEventEncryptor(service, nil)

// Encrypt event
encryptedEvent, _ := eventEnc.EncryptEvent(event)

// Decrypt event
decryptedEvent, _ := eventEnc.DecryptEvent(encryptedEvent)
```

## Features

### AES-256-GCM Encryption

- **Algorithm**: AES-256 in Galois/Counter Mode
- **Authentication**: Built-in authentication (AEAD)
- **Nonce**: Unique nonce per encryption
- **Key Size**: 256-bit (32 bytes) or 128-bit (16 bytes)

```go
config := &encryption.Config{
    Algorithm:     encryption.AlgorithmAES256GCM,
    KeySize:       32,
    KeyDerivation: encryption.KeyDerivationArgon2,
}
```

### Key Derivation Functions

**Argon2id (Recommended)**
- Memory-hard function
- Resistant to GPU/ASIC attacks
- OWASP recommended

```go
config := encryption.DefaultConfig()
// Argon2Time: 3, Argon2Memory: 64MB, Argon2Threads: 4
```

**PBKDF2-SHA256 (Legacy Support)**
- Compatible with older systems
- 600,000 iterations (OWASP 2023)

```go
config := &encryption.Config{
    KeyDerivation:    encryption.KeyDerivationPBKDF2,
    PBKDF2Iterations: 600000,
}
```

### Key Management

**Features:**
- Key versioning
- Key rotation
- Multiple active keys
- Key metadata export
- Key lifecycle management

```go
km := service.KeyManager()

// Rotate key
newKeyID, _ := km.RotateKey()

// List all keys
keys := km.ListKeys()

// Get active key
activeKey, _ := km.GetActiveKey()
```

### Event Encryption Modes

**Full Encryption:**
```go
config := &encryption.EventEncryptionConfig{
    EncryptData: true,
    FieldEncryption: false,
}
```

**Field-Level Encryption:**
```go
config := &encryption.EventEncryptionConfig{
    EncryptData: true,
    FieldEncryption: true,
    EncryptedFields: []string{"ssn", "credit_card", "password"},
}
```

## Usage Examples

### Example 1: Basic Encryption/Decryption

```go
// Generate key
key, _ := encryption.GenerateKey(32)

// Create cipher
cipher, _ := encryption.NewCipher(key, encryption.DefaultConfig())

// Encrypt
plaintext := []byte("sensitive data")
ciphertext, _ := cipher.Encrypt(plaintext)

// Decrypt
decrypted, _ := cipher.Decrypt(ciphertext)
```

### Example 2: Service with Key Rotation

```go
// Create service
masterKey, _ := encryption.GenerateKey(32)
service, _ := encryption.NewService(masterKey)

// Encrypt with original key
data1 := "old data"
cipher1, _ := service.EncryptString(data1)

// Rotate key
newKeyID, _ := service.RotateKey()

// Encrypt with new key
data2 := "new data"
cipher2, _ := service.EncryptString(data2)

// Both can still be decrypted
old, _ := service.DecryptString(cipher1)  // Works!
new, _ := service.DecryptString(cipher2)  // Works!

// Re-encrypt old data with new key
reEncrypted, _ := service.ReEncrypt(cipher1)
```

### Example 3: Full Event Encryption

```go
// Create encryptor
eventEnc := encryption.NewEventEncryptor(service, &encryption.EventEncryptionConfig{
    EncryptData: true,
    FieldEncryption: false,
})

// Create event
eventData := map[string]interface{}{
    "account_id":   "ACC-123",
    "amount":       1000.0,
    "ssn":          "123-45-6789",
    "credit_card":  "4532-1111-2222-3333",
}
eventDataBytes, _ := json.Marshal(eventData)

event := &domain.Event{
    ID:            "evt-001",
    AggregateID:   "account-123",
    AggregateType: "Account",
    EventType:     "AccountCredited",
    Version:       1,
    Timestamp:     time.Now(),
    Data:          eventDataBytes,
}

// Encrypt (entire payload encrypted)
encrypted, _ := eventEnc.EncryptEvent(event)

// Store encrypted event
// ...

// Decrypt when loading
decrypted, _ := eventEnc.DecryptEvent(encrypted)
```

### Example 4: Field-Level Event Encryption

```go
// Create field-level encryptor
fieldEnc := encryption.NewEventEncryptor(service, &encryption.EventEncryptionConfig{
    EncryptData:     true,
    FieldEncryption: true,
    EncryptedFields: []string{"ssn", "credit_card"},
})

// Create event with mixed sensitive/non-sensitive data
eventData := map[string]interface{}{
    "account_id":   "ACC-123",    // Not encrypted (searchable)
    "amount":       1000.0,       // Not encrypted (searchable)
    "customer_name": "John Doe",  // Not encrypted (searchable)
    "ssn":          "123-45-6789", // ENCRYPTED
    "credit_card":  "4532-1111-2222-3333", // ENCRYPTED
}

// Encrypt (only ssn and credit_card encrypted)
encrypted, _ := fieldEnc.EncryptEvent(event)

// Result: Can still search by account_id, amount, customer_name
// But ssn and credit_card are protected
```

### Example 5: Password-Based Encryption

```go
// Generate salt (store this!)
salt, _ := encryption.GenerateSalt()

// Derive key from password
password := "user-password-123"
key, _ := encryption.DeriveKey(password, salt, encryption.DefaultConfig())

// Create cipher with derived key
cipher, _ := encryption.NewCipher(key, encryption.DefaultConfig())

// Or use service directly
service, _ := encryption.NewServiceWithPassword(password, salt)
```

## Key Management

### Key Generation

```go
// Generate AES-256 key (32 bytes)
key256, _ := encryption.GenerateKey(32)

// Generate AES-128 key (16 bytes)
key128, _ := encryption.GenerateKey(16)

// Generate salt for key derivation
salt, _ := encryption.GenerateSalt()
```

### Key Rotation

```go
service, _ := encryption.NewService(masterKey)

// Rotate to new key
newKeyID, _ := service.RotateKey()

// Old data still decrypts (uses old key)
// New data uses new key automatically

// Re-encrypt old data with new key
reEncrypted, _ := service.ReEncrypt(oldCiphertext)
```

### Key Storage

**DO:**
- ✅ Store keys in AWS KMS, HashiCorp Vault, or Azure Key Vault
- ✅ Use environment variables for temporary storage
- ✅ Encrypt keys at rest
- ✅ Use separate keys per environment

**DON'T:**
- ❌ Store keys in source code
- ❌ Commit keys to version control
- ❌ Share keys between environments
- ❌ Store keys in plaintext files

### Integration with Credential Provider

```go
import (
    "github.com/plaenen/eventstore/pkg/security/credentials"
    "github.com/plaenen/eventstore/pkg/security/encryption"
)

// Store encryption key securely
credProvider, _ := credentials.NewSecretProvider(ctx,
    "awskms://arn:aws:secretsmanager:us-east-1:123:secret:encryption-key")

creds, _ := credProvider.GetCredentials(ctx)

// Use the key
service, _ := encryption.NewService([]byte(creds.Token))
```

## Event Encryption

### When to Use Full vs Field-Level

| Scenario | Recommendation |
|----------|---------------|
| Highly sensitive data (medical records) | Full encryption |
| Mixed sensitive/non-sensitive fields | Field-level |
| Need to search/query events | Field-level (encrypt only PII) |
| Regulatory compliance (HIPAA) | Full encryption |
| Performance-sensitive | Field-level |

### Full Encryption

**Pros:**
- Maximum security
- All data protected
- Simple configuration

**Cons:**
- Cannot search encrypted fields
- Slightly slower
- Must decrypt to query

**Use When:**
- All data is equally sensitive
- Compliance requires full encryption
- Search not needed

### Field-Level Encryption

**Pros:**
- Granular control
- Searchable non-sensitive fields
- Better performance
- Flexible queries

**Cons:**
- More configuration
- Need to identify sensitive fields
- Potential for mistakes

**Use When:**
- Only some fields are sensitive
- Need to search/filter events
- Performance matters

## Production Deployment

### Deployment Checklist

- [ ] Generate strong master keys (32 bytes)
- [ ] Store keys in secure key management system
- [ ] Use Argon2id for password-based encryption
- [ ] Enable key rotation schedule
- [ ] Configure field-level encryption for searchable data
- [ ] Test encryption/decryption performance
- [ ] Set up key backup and recovery
- [ ] Monitor encryption operations
- [ ] Document key management procedures
- [ ] Train team on security practices

### AWS Deployment

```go
// 1. Store master key in AWS Secrets Manager
aws secretsmanager create-secret \
  --name prod/encryption/master-key \
  --secret-binary fileb://master-key.bin

// 2. Use in application
credProvider, _ := credentials.NewSecretProvider(ctx,
    "awsparamstore:///prod/encryption/master-key")

creds, _ := credProvider.GetCredentials(ctx)
service, _ := encryption.NewService([]byte(creds.Token))
```

### Kubernetes Deployment

```yaml
# Create secret with encryption key
apiVersion: v1
kind: Secret
metadata:
  name: encryption-key
type: Opaque
data:
  master-key: <base64-encoded-key>
---
# Mount in pod
apiVersion: v1
kind: Pod
spec:
  containers:
  - name: app
    env:
    - name: ENCRYPTION_KEY
      valueFrom:
        secretKeyRef:
          name: encryption-key
          key: master-key
```

```go
// Use in application
keyBase64 := os.Getenv("ENCRYPTION_KEY")
key, _ := base64.StdEncoding.DecodeString(keyBase64)
service, _ := encryption.NewService(key)
```

### Performance Tuning

**For Development:**
```go
config := encryption.FastConfig()
// Faster key derivation, less secure
// Only use in development!
```

**For Production:**
```go
config := encryption.DefaultConfig()
// Secure defaults, slower but safer
```

**Custom Tuning:**
```go
config := &encryption.Config{
    Algorithm:     encryption.AlgorithmAES256GCM,
    KeyDerivation: encryption.KeyDerivationArgon2,
    KeySize:       32,
    Argon2Time:    3,    // Iterations
    Argon2Memory:  64 * 1024, // 64 MiB
    Argon2Threads: 4,    // Parallel threads
}
```

## Security Best Practices

### ✅ DO's

1. **Use Strong Keys**
   ```go
   key, _ := encryption.GenerateKey(32) // AES-256
   ```

2. **Rotate Keys Regularly**
   ```go
   // Rotate every 90 days
   service.RotateKey()
   ```

3. **Use Argon2id**
   ```go
   config.KeyDerivation = encryption.KeyDerivationArgon2
   ```

4. **Store Keys Securely**
   - AWS KMS, HashiCorp Vault, Azure Key Vault
   - Never in source code or config files

5. **Encrypt Sensitive Fields**
   ```go
   config.EncryptedFields = []string{"ssn", "credit_card", "password"}
   ```

6. **Test Recovery Procedures**
   - Test key backup/restore
   - Document recovery steps

### ❌ DON'TS

1. **NEVER Store Keys in Code**
   ```go
   // ❌ BAD
   masterKey := []byte("hardcoded-key-12345")

   // ✅ GOOD
   masterKey, _ := loadKeyFromVault()
   ```

2. **NEVER Skip Key Derivation**
   ```go
   // ❌ BAD - using password directly
   service, _ := encryption.NewService([]byte("password"))

   // ✅ GOOD - derive key from password
   key, _ := encryption.DeriveKey("password", salt, config)
   ```

3. **NEVER Share Keys Between Environments**
   - Dev, staging, production must have separate keys

4. **NEVER Use Fast Config in Production**
   ```go
   // ❌ BAD in production
   config := encryption.FastConfig()

   // ✅ GOOD
   config := encryption.DefaultConfig()
   ```

5. **NEVER Ignore Encryption Errors**
   ```go
   // ❌ BAD
   ciphertext, _ := service.Encrypt(data)

   // ✅ GOOD
   ciphertext, err := service.Encrypt(data)
   if err != nil {
       log.Fatalf("Encryption failed: %v", err)
   }
   ```

## API Reference

### Core Functions

```go
// Key generation
GenerateKey(size int) ([]byte, error)
GenerateSalt() ([]byte, error)
DeriveKey(password string, salt []byte, config *Config) ([]byte, error)

// Cipher creation
NewCipher(key []byte, config *Config) (*Cipher, error)
NewCipherWithPassword(password string, salt []byte, config *Config) (*Cipher, error)

// Service creation
NewService(masterKey []byte) (*Service, error)
NewServiceWithConfig(masterKey []byte, config *Config) (*Service, error)
NewServiceWithPassword(password string, salt []byte) (*Service, error)
```

### Cipher Methods

```go
// Encryption/Decryption
Encrypt(plaintext []byte) ([]byte, error)
Decrypt(ciphertext []byte) ([]byte, error)
EncryptString(plaintext string) (string, error)
DecryptString(ciphertext string) (string, error)

// Key ID management
SetKeyID(keyID string)
KeyID() string
```

### Service Methods

```go
// Encryption/Decryption
Encrypt(plaintext []byte) (string, error)
Decrypt(ciphertext string) ([]byte, error)
EncryptString(plaintext string) (string, error)
DecryptString(ciphertext string) (string, error)

// Key management
RotateKey() (string, error)
ReEncrypt(oldCiphertext string) (string, error)
KeyManager() *KeyManager
```

### EventEncryptor Methods

```go
// Event encryption
EncryptEvent(event *domain.Event) (*domain.Event, error)
DecryptEvent(event *domain.Event) (*domain.Event, error)
EncryptEvents(events []*domain.Event) ([]*domain.Event, error)
DecryptEvents(events []*domain.Event) ([]*domain.Event, error)
```

## Troubleshooting

### Common Issues

#### 1. Decryption Failed

**Error**: `decryption failed: message authentication failed`

**Causes:**
- Wrong encryption key
- Corrupted ciphertext
- Modified data

**Solution:**
```go
// Verify using correct key
key, err := loadCorrectKey()
if err != nil {
    log.Fatal("Failed to load key")
}

// Verify ciphertext format
if !strings.Contains(ciphertext, ":") {
    log.Fatal("Invalid ciphertext format")
}
```

#### 2. Key Not Found

**Error**: `key {id} not found`

**Cause:** Trying to decrypt with a key that no longer exists

**Solution:**
```go
// Keep old keys for decryption
// Only remove keys after re-encrypting all data
km.ListKeys() // Verify key exists before removing
```

#### 3. Performance Issues

**Problem:** Slow encryption/decryption

**Solutions:**
```go
// 1. Use fast config for development
config := encryption.FastConfig()

// 2. Reduce Argon2 parameters
config.Argon2Time = 1
config.Argon2Memory = 16 * 1024

// 3. Use field-level encryption instead of full
config.FieldEncryption = true
```

#### 4. Out of Memory

**Problem:** High memory usage with Argon2

**Solution:**
```go
// Reduce memory parameter
config := &encryption.Config{
    KeyDerivation: encryption.KeyDerivationArgon2,
    Argon2Memory:  32 * 1024, // Reduce from 64 MiB to 32 MiB
}
```

## Examples

See the [encryption-at-rest example](../../../examples/cmd/security-examples/encryption-at-rest) for comprehensive demonstrations including:

- Basic encryption/decryption
- Password-based encryption
- Key rotation
- Full event encryption
- Field-level event encryption
- Key management
- Performance comparison

Run the example:

```bash
cd examples/cmd/security-examples/encryption-at-rest
go run main.go
```

## Related Packages

- **[pkg/security/credentials](../credentials)** - Secure credential management (SEC-001)
- **[pkg/security/tls](../tls)** - TLS/mTLS encryption in transit (SEC-002)
- **[pkg/domain](../../domain)** - Event and aggregate types

## Security Roadmap

This package implements:

- ✅ **SEC-103: Data Encryption at Rest** - Complete implementation
  - AES-256-GCM encryption
  - Key management and rotation
  - Field-level encryption
  - Event encryption integration

Related security features:

- ✅ **SEC-001: Authentication & Credentials** - Secure credential management
- ✅ **SEC-002: TLS/Encryption** - Encryption in transit
- 🔲 **SEC-003: RBAC** - Role-based access control (planned)
- 🔲 **SEC-004: Audit Logging** - Security audit trails (planned)

## License

Copyright © 2024 EventSourcing Framework

## Integration with Credentials Package

The encryption package integrates seamlessly with the credentials package (SEC-001) for secure key storage.

### Quick Integration

```go
import (
    "context"
    "github.com/plaenen/eventstore/pkg/security/credentials"
    "github.com/plaenen/eventstore/pkg/security/encryption"
)

// AWS Secrets Manager
provider, _ := credentials.NewSecretProvider(ctx,
    "awsparamstore:///prod/encryption/master-key")

// Create encryption service from credential provider
service, _ := encryption.NewServiceFromCredentialProvider(ctx, provider)

// Use normally
encrypted, _ := service.EncryptString("sensitive data")
```

### Key Storage Best Practices

**1. Production: AWS Secrets Manager**
```bash
# Generate key
openssl rand -base64 32 > master-key.txt

# Store in AWS
aws secretsmanager create-secret \
  --name prod/encryption/master-key \
  --secret-string file://master-key.txt

# Delete local copy
shred -u master-key.txt
```

```go
// Application code
provider, _ := credentials.NewSecretProvider(ctx,
    "awsparamstore:///prod/encryption/master-key")
service, _ := encryption.NewServiceFromCredentialProvider(ctx, provider)
```

**2. Development: Environment Variables**
```bash
# Generate and export key
export ENCRYPTION_MASTER_KEY=$(openssl rand -base64 32)
```

```go
// Application code
provider := credentials.NewEnvTokenProvider("ENCRYPTION_MASTER_KEY", 0)
service, _ := encryption.NewServiceFromCredentialProvider(ctx, provider)
```

**3. Flexible: Chain Provider**
```go
// Try production first, fall back to development
provider := credentials.NewChainProvider(
    credentials.NewSecretProvider(ctx, "awsparamstore:///prod/key"),
    credentials.NewEnvTokenProvider("ENCRYPTION_KEY", 0),
)

service, _ := encryption.NewServiceFromCredentialProvider(ctx, provider)
```

### Key Rotation with Credentials

```go
// 1. Load current key
currentProvider, _ := credentials.NewSecretProvider(ctx,
    "awsparamstore:///prod/encryption/master-key")
service, _ := encryption.NewServiceFromCredentialProvider(ctx, currentProvider)

// 2. Load new key
newProvider, _ := credentials.NewSecretProvider(ctx,
    "awsparamstore:///prod/encryption/master-key-v2")
newCreds, _ := newProvider.GetCredentials(ctx)
newKey, _ := encryption.DecodeKey(newCreds.Token)

// 3. Add new key to key manager
km := service.KeyManager()
km.AddKey("master-v2", newKey, true) // Set as active

// 4. Old data still decrypts, new encryptions use new key
```

### Multi-Tenant Key Management

```go
// Get tenant-specific encryption key
func getTenantEncryptionService(ctx context.Context, tenantID string) (*encryption.Service, error) {
    // Tenant-specific key path
    keyPath := fmt.Sprintf("/prod/encryption/tenant/%s/key", tenantID)
    
    provider, err := credentials.NewSecretProvider(ctx,
        "awsparamstore://" + keyPath)
    if err != nil {
        return nil, err
    }
    
    return encryption.NewServiceFromCredentialProvider(ctx, provider)
}

// Use tenant-specific encryption
tenantService, _ := getTenantEncryptionService(ctx, "tenant-123")
encrypted, _ := tenantService.EncryptString("tenant data")
```

### Helper Functions

```go
// Generate and encode key for storage
encoded, _ := encryption.GenerateAndEncodeKey(32)
fmt.Println(encoded) // Store this in AWS Secrets Manager

// Decode key from storage
key, _ := encryption.DecodeKey(encoded)

// Create service with decoded key
service, _ := encryption.NewService(key)
```

See [secure-creds example](../../../examples/cmd/security-examples/secure-creds) for complete integration demonstration.

