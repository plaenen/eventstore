# Immediate Security Actions - Implementation Guide

**Priority:** P0 - CRITICAL
**Timeline:** 1-2 weeks
**Status:** 🔴 Not started

This document provides detailed implementation guidance for the 5 critical security issues identified in the security roadmap. These MUST be addressed before any production deployment.

---

## SEC-001: Authentication & Credentials Management

### Problem Statement

```go
// CURRENT - INSECURE ❌
type TransportConfig struct {
    URL   string
    Token string  // Stored as plaintext
    User  string  // Transmitted as plaintext
    Pass  string  // CRITICAL: Password in memory/logs/config
}
```

### Solution Architecture

```go
// NEW - SECURE ✅
type TransportConfig struct {
    URL               string
    CredentialProvider CredentialProvider  // Interface-based
    TLSConfig         *TLSConfig          // Required for auth
}

type CredentialProvider interface {
    // GetCredentials returns current credentials
    GetCredentials(ctx context.Context) (*Credentials, error)

    // Rotate triggers credential rotation
    Rotate(ctx context.Context) error

    // Type returns the credential type
    Type() CredentialType
}

type Credentials struct {
    Type       CredentialType
    Token      string  // Short-lived, never logged
    ExpiresAt  time.Time
}

type CredentialType string

const (
    CredentialTypeToken    CredentialType = "token"
    CredentialTypeNKey     CredentialType = "nkey"
    CredentialTypeJWT      CredentialType = "jwt"
    CredentialTypeMTLS     CredentialType = "mtls"
)
```

### Implementation Steps

#### Step 1: Create Credential Provider Interface

**File:** `pkg/security/credentials/provider.go`

```go
package credentials

import (
    "context"
    "crypto/aes"
    "crypto/cipher"
    "crypto/rand"
    "encoding/base64"
    "errors"
    "time"
    "golang.org/x/crypto/argon2"
)

var (
    ErrCredentialsExpired = errors.New("credentials expired")
    ErrInvalidCredentials = errors.New("invalid credentials")
)

// Provider implementations
type TokenProvider struct {
    token     string
    expiresAt time.Time
    encrypted bool
}

func NewTokenProvider(token string, ttl time.Duration) *TokenProvider {
    return &TokenProvider{
        token:     token,
        expiresAt: time.Now().Add(ttl),
        encrypted: false,
    }
}

func (p *TokenProvider) GetCredentials(ctx context.Context) (*Credentials, error) {
    if time.Now().After(p.expiresAt) {
        return nil, ErrCredentialsExpired
    }

    return &Credentials{
        Type:      CredentialTypeToken,
        Token:     p.token,
        ExpiresAt: p.expiresAt,
    }, nil
}

func (p *TokenProvider) Rotate(ctx context.Context) error {
    // Implement rotation logic
    return errors.New("manual rotation required")
}

func (p *TokenProvider) Type() CredentialType {
    return CredentialTypeToken
}

// Vault-based provider for production
type VaultProvider struct {
    client    *VaultClient
    path      string
    role      string
    ttl       time.Duration
    cached    *Credentials
    cacheUntil time.Time
}

// Environment-based provider for development
type EnvProvider struct {
    tokenEnvVar string
}

// File-based encrypted provider
type EncryptedFileProvider struct {
    filepath   string
    masterKey  []byte
}
```

#### Step 2: Implement Encryption for Local Storage

**File:** `pkg/security/credentials/encryption.go`

```go
package credentials

import (
    "crypto/aes"
    "crypto/cipher"
    "crypto/rand"
    "encoding/base64"
    "fmt"
    "golang.org/x/crypto/argon2"
    "io"
    "os"
)

type EncryptedStorage struct {
    filepath string
    key      []byte
}

func NewEncryptedStorage(filepath string, password string) (*EncryptedStorage, error) {
    // Derive encryption key from password using Argon2
    salt := make([]byte, 16)
    if _, err := io.ReadFull(rand.Reader, salt); err != nil {
        return nil, err
    }

    key := argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32)

    return &EncryptedStorage{
        filepath: filepath,
        key:      key,
    }, nil
}

func (s *EncryptedStorage) Store(credentials string) error {
    block, err := aes.NewCipher(s.key)
    if err != nil {
        return err
    }

    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return err
    }

    nonce := make([]byte, gcm.NonceSize())
    if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
        return err
    }

    ciphertext := gcm.Seal(nonce, nonce, []byte(credentials), nil)
    encoded := base64.StdEncoding.EncodeToString(ciphertext)

    return os.WriteFile(s.filepath, []byte(encoded), 0600)
}

func (s *EncryptedStorage) Load() (string, error) {
    data, err := os.ReadFile(s.filepath)
    if err != nil {
        return "", err
    }

    ciphertext, err := base64.StdEncoding.DecodeString(string(data))
    if err != nil {
        return "", err
    }

    block, err := aes.NewCipher(s.key)
    if err != nil {
        return "", err
    }

    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return "", err
    }

    nonceSize := gcm.NonceSize()
    if len(ciphertext) < nonceSize {
        return "", fmt.Errorf("ciphertext too short")
    }

    nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
    plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
    if err != nil {
        return "", err
    }

    return string(plaintext), nil
}
```

#### Step 3: Update NATS Transport

**File:** `pkg/cqrs/nats/transport.go`

```diff
  type TransportConfig struct {
      *cqrs.TransportConfig

      URL string
      Name string

-     // Credentials for authentication (optional)
-     Token string
-     User  string
-     Pass  string

+     // CredentialProvider for secure authentication
+     CredentialProvider credentials.CredentialProvider

+     // TLSConfig for secure connections (required in production)
+     TLSConfig *TLSConfig
+
      Telemetry *observability.Telemetry
  }

  func NewTransport(config *TransportConfig) (*Transport, error) {
+     // Validate security configuration
+     if config.CredentialProvider != nil && config.TLSConfig == nil {
+         return nil, errors.New("TLS required when using authentication")
+     }

      opts := []nats.Option{
          nats.Name(config.Name),
          nats.MaxReconnects(config.MaxReconnectAttempts),
          nats.ReconnectWait(config.ReconnectWait),
      }

-     // Add authentication if provided
-     if config.Token != "" {
-         opts = append(opts, nats.Token(config.Token))
-     } else if config.User != "" && config.Pass != "" {
-         opts = append(opts, nats.UserInfo(config.User, config.Pass))
-     }

+     // Get credentials from provider
+     if config.CredentialProvider != nil {
+         creds, err := config.CredentialProvider.GetCredentials(context.Background())
+         if err != nil {
+             return nil, fmt.Errorf("failed to get credentials: %w", err)
+         }
+
+         switch creds.Type {
+         case credentials.CredentialTypeToken:
+             opts = append(opts, nats.Token(creds.Token))
+         case credentials.CredentialTypeNKey:
+             opts = append(opts, nats.Nkey(creds.PublicKey, creds.SignCallback))
+         case credentials.CredentialTypeJWT:
+             opts = append(opts, nats.UserJWT(creds.JWTCallback, creds.SignCallback))
+         }
+     }
+
+     // Add TLS configuration
+     if config.TLSConfig != nil {
+         tlsConfig, err := config.TLSConfig.ToNativeTLSConfig()
+         if err != nil {
+             return nil, fmt.Errorf("failed to create TLS config: %w", err)
+         }
+         opts = append(opts, nats.Secure(tlsConfig))
+     }

      nc, err := nats.Connect(config.URL, opts...)
      // ...
  }
```

### Testing

```go
// pkg/security/credentials/provider_test.go
func TestTokenProvider(t *testing.T) {
    provider := NewTokenProvider("test-token", 1*time.Hour)

    creds, err := provider.GetCredentials(context.Background())
    assert.NoError(t, err)
    assert.Equal(t, "test-token", creds.Token)
    assert.Equal(t, CredentialTypeToken, creds.Type)
}

func TestEncryptedStorage(t *testing.T) {
    tmpfile := t.TempDir() + "/creds.enc"
    storage, err := NewEncryptedStorage(tmpfile, "test-password")
    assert.NoError(t, err)

    err = storage.Store("secret-token")
    assert.NoError(t, err)

    loaded, err := storage.Load()
    assert.NoError(t, err)
    assert.Equal(t, "secret-token", loaded)
}
```

### Migration Guide

```go
// Before (INSECURE)
transport, _ := nats.NewTransport(&nats.TransportConfig{
    URL:  "nats://localhost:4222",
    User: "admin",
    Pass: "password123",  // ❌ Plaintext
})

// After (SECURE)
provider := credentials.NewTokenProvider(
    os.Getenv("NATS_TOKEN"),
    24*time.Hour,
)

transport, _ := nats.NewTransport(&nats.TransportConfig{
    URL:                "nats://localhost:4222",
    CredentialProvider: provider,
    TLSConfig: &nats.TLSConfig{
        Enabled:  true,
        CAFile:   "/path/to/ca.crt",
        CertFile: "/path/to/client.crt",
        KeyFile:  "/path/to/client.key",
    },
})
```

---

## SEC-002: TLS/Encryption for NATS

### Problem Statement

All NATS connections are currently unencrypted, allowing:
- Man-in-the-middle attacks
- Credential interception
- Data exfiltration
- Command/event tampering

### Solution Architecture

```go
type TLSConfig struct {
    // Enabled controls TLS (false only for local development)
    Enabled bool

    // Certificate files
    CertFile string
    KeyFile  string
    CAFile   string

    // Options
    InsecureSkipVerify bool  // Only for development
    ServerName         string
    MinVersion         uint16  // Default: TLS 1.3

    // Client authentication (mTLS)
    ClientAuth bool
}
```

### Implementation Steps

#### Step 1: Create TLS Configuration

**File:** `pkg/security/tls/config.go`

```go
package tls

import (
    "crypto/tls"
    "crypto/x509"
    "fmt"
    "os"
)

type Config struct {
    Enabled            bool
    CertFile           string
    KeyFile            string
    CAFile             string
    InsecureSkipVerify bool
    ServerName         string
    MinVersion         uint16
    ClientAuth         bool
}

func (c *Config) ToNativeTLSConfig() (*tls.Config, error) {
    if !c.Enabled {
        return nil, nil
    }

    tlsConfig := &tls.Config{
        MinVersion: c.MinVersion,
        ServerName: c.ServerName,
    }

    // Set minimum TLS version (default to 1.3)
    if tlsConfig.MinVersion == 0 {
        tlsConfig.MinVersion = tls.VersionTLS13
    }

    // Load CA certificate
    if c.CAFile != "" {
        caCert, err := os.ReadFile(c.CAFile)
        if err != nil {
            return nil, fmt.Errorf("failed to read CA file: %w", err)
        }

        caCertPool := x509.NewCertPool()
        if !caCertPool.AppendCertsFromPEM(caCert) {
            return nil, fmt.Errorf("failed to parse CA certificate")
        }
        tlsConfig.RootCAs = caCertPool
    }

    // Load client certificate (for mTLS)
    if c.CertFile != "" && c.KeyFile != "" {
        cert, err := tls.LoadX509KeyPair(c.CertFile, c.KeyFile)
        if err != nil {
            return nil, fmt.Errorf("failed to load client certificate: %w", err)
        }
        tlsConfig.Certificates = []tls.Certificate{cert}
    }

    // ONLY for development
    if c.InsecureSkipVerify {
        tlsConfig.InsecureSkipVerify = true
    }

    return tlsConfig, nil
}

// Validate ensures configuration is secure
func (c *Config) Validate(env string) error {
    if !c.Enabled && env == "production" {
        return fmt.Errorf("TLS must be enabled in production")
    }

    if c.InsecureSkipVerify && env == "production" {
        return fmt.Errorf("InsecureSkipVerify cannot be used in production")
    }

    if c.ClientAuth && (c.CertFile == "" || c.KeyFile == "") {
        return fmt.Errorf("client certificate required for mTLS")
    }

    return nil
}
```

#### Step 2: Update Embedded NATS

**File:** `pkg/infrastructure/nats/embedded.go`

```diff
  type Option func(*server.Options)

+ func WithTLS(certFile, keyFile string) Option {
+     return func(opts *server.Options) {
+         opts.TLSConfig = &tls.Config{
+             MinVersion: tls.VersionTLS13,
+         }
+         opts.TLS = true
+         opts.TLSCert = certFile
+         opts.TLSKey = keyFile
+     }
+ }
+
+ func WithTLSVerify(caFile string) Option {
+     return func(opts *server.Options) {
+         opts.TLSVerify = true
+         opts.TLSCaCert = caFile
+     }
+ }
```

### Example Usage

```go
// Development (localhost only)
service := embeddednats.New(
    embeddednats.WithNATSOptions(
        nats.WithPort(4222),
        // No TLS for local development
    ),
)

// Production
service := embeddednats.New(
    embeddednats.WithNATSOptions(
        nats.WithPort(4222),
        nats.WithTLS("/etc/certs/server.crt", "/etc/certs/server.key"),
        nats.WithTLSVerify("/etc/certs/ca.crt"),
    ),
)
```

---

## SEC-003: SQL Injection Prevention

### Audit Checklist

- [x] Review all migration files in `pkg/store/sqlite/migrations/`
- [ ] Ensure no string concatenation in SQL
- [ ] Validate all table/column name inputs
- [ ] Add SQL identifier validation

### Implementation

**File:** `pkg/security/sql/validation.go`

```go
package sql

import (
    "fmt"
    "regexp"
)

var (
    // SQL identifiers must be alphanumeric + underscore
    sqlIdentifierRegex = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

    ErrInvalidIdentifier = fmt.Errorf("invalid SQL identifier")
)

// ValidateIdentifier ensures a string is a safe SQL identifier
func ValidateIdentifier(name string) error {
    if len(name) == 0 || len(name) > 64 {
        return fmt.Errorf("%w: length must be 1-64", ErrInvalidIdentifier)
    }

    if !sqlIdentifierRegex.MatchString(name) {
        return fmt.Errorf("%w: %s", ErrInvalidIdentifier, name)
    }

    // Check reserved words
    if isReservedWord(name) {
        return fmt.Errorf("%w: %s is a reserved word", ErrInvalidIdentifier, name)
    }

    return nil
}

func isReservedWord(word string) bool {
    reserved := map[string]bool{
        "SELECT": true, "INSERT": true, "UPDATE": true, "DELETE": true,
        "FROM": true, "WHERE": true, "JOIN": true, "DROP": true,
        // ... add all SQL reserved words
    }
    return reserved[strings.ToUpper(word)]
}
```

---

## SEC-004: Error Information Disclosure

### Implementation

**File:** `pkg/security/errors/sanitize.go`

```go
package errors

import (
    "errors"
    "fmt"
    "github.com/plaenen/eventstore/pkg/eventsourcing"
)

type ErrorSanitizer struct {
    mode string  // "development" or "production"
}

func NewErrorSanitizer(mode string) *ErrorSanitizer {
    return &ErrorSanitizer{mode: mode}
}

func (s *ErrorSanitizer) Sanitize(err error) error {
    if s.mode == "development" {
        return err  // Return full error in development
    }

    // Production: Return safe error
    if errors.Is(err, eventsourcing.ErrConcurrencyConflict) {
        return errors.New("version conflict")
    }

    if errors.Is(err, eventsourcing.ErrAggregateNotFound) {
        return errors.New("resource not found")
    }

    // Default: generic error
    return errors.New("internal server error")
}

func (s *ErrorSanitizer) SanitizeAppError(appErr *eventsourcing.AppError) *eventsourcing.AppError {
    if s.mode == "development" {
        return appErr
    }

    // Map internal codes to safe codes
    safeCode := s.mapToSafeCode(appErr.Code)
    safeMessage := s.getSafeMessage(safeCode)

    return &eventsourcing.AppError{
        Code:    safeCode,
        Message: safeMessage,
    }
}

func (s *ErrorSanitizer) mapToSafeCode(code string) string {
    mapping := map[string]string{
        "DATABASE_ERROR":     "INTERNAL_ERROR",
        "SQL_ERROR":          "INTERNAL_ERROR",
        "CONSTRAINT_VIOLATION": "INVALID_INPUT",
    }

    if safe, ok := mapping[code]; ok {
        return safe
    }
    return code
}
```

---

## SEC-005: Input Validation Gaps

### Implementation

**File:** `pkg/validation/validators.go`

```go
package validation

import (
    "fmt"
    "regexp"
    "github.com/google/uuid"
)

var (
    emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)

    ErrInvalidUUID   = fmt.Errorf("invalid UUID format")
    ErrInvalidEmail  = fmt.Errorf("invalid email format")
    ErrTooLong       = fmt.Errorf("value too long")
    ErrTooShort      = fmt.Errorf("value too short")
)

// ValidateUUID ensures string is a valid UUID v4
func ValidateUUID(id string) error {
    parsed, err := uuid.Parse(id)
    if err != nil {
        return fmt.Errorf("%w: %s", ErrInvalidUUID, id)
    }

    if parsed.Version() != 4 {
        return fmt.Errorf("%w: must be UUID v4", ErrInvalidUUID)
    }

    return nil
}

// ValidateEmail ensures string is a valid email
func ValidateEmail(email string) error {
    if len(email) > 254 {  // RFC 5321
        return fmt.Errorf("%w: max 254 characters", ErrTooLong)
    }

    if !emailRegex.MatchString(email) {
        return ErrInvalidEmail
    }

    return nil
}

// ValidateStringLength ensures string is within bounds
func ValidateStringLength(s string, min, max int) error {
    if len(s) < min {
        return fmt.Errorf("%w: minimum %d characters", ErrTooShort, min)
    }
    if len(s) > max {
        return fmt.Errorf("%w: maximum %d characters", ErrTooLong, max)
    }
    return nil
}

// ValidateTenantID ensures tenant ID format
func ValidateTenantID(tenantID string) error {
    // Example: "tenant_<uuid>"
    if !strings.HasPrefix(tenantID, "tenant_") {
        return fmt.Errorf("tenant ID must start with 'tenant_'")
    }

    uuidPart := strings.TrimPrefix(tenantID, "tenant_")
    return ValidateUUID(uuidPart)
}
```

**Update:** `pkg/middleware/validation.go`

```diff
  func MetadataValidationMiddleware() eventsourcing.CommandMiddleware {
      return func(next eventsourcing.CommandHandler) eventsourcing.CommandHandler {
          return eventsourcing.CommandHandlerFunc(func(ctx context.Context, cmd *eventsourcing.CommandEnvelope) ([]*eventsourcing.Event, error) {
              // Validate command ID
              if cmd.Metadata.CommandID == "" {
                  return nil, fmt.Errorf("%w: command_id is required", eventsourcing.ErrInvalidCommand)
              }
+             if err := validation.ValidateUUID(cmd.Metadata.CommandID); err != nil {
+                 return nil, fmt.Errorf("%w: invalid command_id", eventsourcing.ErrInvalidCommand)
+             }

-             // Validate principal ID (optional but recommended)
-             if cmd.Metadata.PrincipalID == "" {
-                 // Log warning but don't fail
-                 // In production, you might want to enforce this
-             }

+             // Validate principal ID (REQUIRED)
+             if cmd.Metadata.PrincipalID == "" {
+                 return nil, fmt.Errorf("%w: principal_id is required", eventsourcing.ErrInvalidCommand)
+             }
+             if err := validation.ValidateUUID(cmd.Metadata.PrincipalID); err != nil {
+                 return nil, fmt.Errorf("%w: invalid principal_id", eventsourcing.ErrInvalidCommand)
+             }

              return next.Handle(ctx, cmd)
          })
      }
  }
```

---

## Implementation Timeline

| Week | Tasks | Owner | Status |
|------|-------|-------|--------|
| 1 | SEC-001: Credential provider interface | Backend Team | 🔴 Not started |
| 1 | SEC-002: TLS configuration | DevOps Team | 🔴 Not started |
| 1 | SEC-003: SQL injection audit | Security Team | 🔴 Not started |
| 2 | SEC-004: Error sanitization | Backend Team | 🔴 Not started |
| 2 | SEC-005: Input validation | Backend Team | 🔴 Not started |
| 2 | Testing & Documentation | All Teams | 🔴 Not started |

---

## Testing Requirements

### Security Test Suite

```go
// pkg/security/tests/critical_test.go
func TestCredentialEncryption(t *testing.T) {
    // Test encrypted storage
}

func TestTLSEnforcement(t *testing.T) {
    // Test that production mode requires TLS
}

func TestSQLInjectionPrevention(t *testing.T) {
    // Test SQL injection attempts
}

func TestErrorSanitization(t *testing.T) {
    // Test no sensitive data in errors
}

func TestInputValidation(t *testing.T) {
    // Test all validators
}
```

---

## Deployment Checklist

Before deploying to production:

- [ ] All 5 critical security issues resolved
- [ ] Security tests passing (100%)
- [ ] Penetration testing completed
- [ ] Security documentation updated
- [ ] Incident response plan in place
- [ ] Monitoring and alerting configured
- [ ] Backup and recovery tested

---

## Support

For questions or assistance:
- Security: security@[domain].com
- Architecture: architecture@[domain].com
- DevOps: devops@[domain].com
