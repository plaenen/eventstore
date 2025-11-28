# TLS/Encryption Package

Comprehensive TLS configuration and certificate management for secure transport in the EventSourcing framework.

**Security Implementation**: This package implements **SEC-002 (TLS/Encryption)** from the security roadmap, providing enterprise-grade TLS support for NATS connections and other network services.

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Configuration Types](#configuration-types)
- [Usage Examples](#usage-examples)
- [NATS Integration](#nats-integration)
- [Certificate Management](#certificate-management)
- [Production Deployment](#production-deployment)
- [Security Best Practices](#security-best-practices)
- [Troubleshooting](#troubleshooting)
- [API Reference](#api-reference)

## Overview

The TLS package provides:

✅ **Transport Layer Security (TLS)** - Encrypt all network traffic
✅ **Mutual TLS (mTLS)** - Bidirectional authentication with client certificates
✅ **Certificate Management** - Load certificates from files or in-memory
✅ **Validation** - Automatic configuration validation
✅ **Flexible Configuration** - Support for various deployment scenarios
✅ **Security Defaults** - Secure-by-default with modern TLS versions and cipher suites
✅ **NATS Integration** - Seamless integration with NATS transport and embedded server

### Why TLS/mTLS?

| Security Level | Description | Use Case |
|---------------|-------------|----------|
| **No TLS** | ❌ Unencrypted, plaintext traffic | Development only (NEVER in production!) |
| **Basic TLS** | 🔒 Encrypted traffic, server authenticated | Most production deployments |
| **Mutual TLS** | 🔐 Encrypted traffic, both sides authenticated | High-security environments, zero-trust networks |

## Quick Start

### 1. Basic TLS (Server Authentication)

Encrypt traffic and verify server identity:

```go
package main

import (
    "log"

    "github.com/plaenen/eventstore/pkg/security/tls"
    natscqrs "github.com/plaenen/eventstore/pkg/cqrs/nats"
)

func main() {
    // Create TLS configuration
    tlsConfig := &tls.Config{
        Enabled:  true,
        CAFile:   "/path/to/ca.pem",
    }

    // Create NATS transport with TLS
    transport, err := natscqrs.NewTransport(&natscqrs.TransportConfig{
        URL:       "nats://production-server:4222",
        TLSConfig: tlsConfig,
    })
    if err != nil {
        log.Fatal(err)
    }
    defer transport.Close()

    // Use transport for secure communication
}
```

### 2. Mutual TLS (Client + Server Authentication)

Maximum security with bidirectional authentication:

```go
// Create mTLS configuration
tlsConfig := tls.MutualTLSConfig(
    "/path/to/client-cert.pem",
    "/path/to/client-key.pem",
    "/path/to/ca.pem",
)

// Create NATS transport with mTLS
transport, err := natscqrs.NewTransport(&natscqrs.TransportConfig{
    URL:       "nats://production-server:4222",
    TLSConfig: tlsConfig,
})
```

### 3. Development Mode (Testing Only)

**⚠️ WARNING: INSECURE - Only for development/testing!**

```go
// Development config - skips certificate verification
tlsConfig := tls.DevelopmentConfig()

// ⚠️ NEVER use this in production!
```

## Configuration Types

### Config Structure

```go
type Config struct {
    // Core Settings
    Enabled  bool   // Enable TLS (default: false)
    CertFile string // Path to TLS certificate file
    KeyFile  string // Path to TLS private key file
    CAFile   string // Path to CA certificate file

    // Security Options
    InsecureSkipVerify bool   // Skip certificate verification (⚠️ INSECURE!)
    ClientAuth         bool   // Enable mutual TLS (mTLS)
    ServerName         string // Server name for SNI

    // TLS Protocol
    MinVersion   uint16   // Minimum TLS version (default: TLS 1.2)
    MaxVersion   uint16   // Maximum TLS version (default: TLS 1.3)
    CipherSuites []uint16 // Allowed cipher suites

    // Advanced Options
    RootCAs      *x509.CertPool     // Root CA pool (alternative to CAFile)
    Certificates []tls.Certificate  // Certificates (alternative to CertFile/KeyFile)
}
```

### Helper Functions

```go
// DefaultConfig - Secure defaults for production
cfg := tls.DefaultConfig()
// • Enabled: true
// • MinVersion: TLS 1.2
// • MaxVersion: TLS 1.3
// • Secure cipher suites

// ProductionConfig - Production-ready configuration
cfg := tls.ProductionConfig("cert.pem", "key.pem", "ca.pem")
// • Everything from DefaultConfig
// • Certificate files configured
// • Verification enabled

// MutualTLSConfig - mTLS with client authentication
cfg := tls.MutualTLSConfig("client-cert.pem", "client-key.pem", "ca.pem")
// • Everything from ProductionConfig
// • ClientAuth enabled

// DevelopmentConfig - ⚠️ INSECURE - Development only!
cfg := tls.DevelopmentConfig()
// • InsecureSkipVerify: true (⚠️ DANGEROUS!)
// • Only use in local development
```

## Usage Examples

### Example 1: NATS Client with TLS

```go
package main

import (
    "log"

    natscqrs "github.com/plaenen/eventstore/pkg/cqrs/nats"
    "github.com/plaenen/eventstore/pkg/security/tls"
)

func main() {
    // Configure TLS for NATS client
    tlsConfig := &tls.Config{
        Enabled: true,
        CAFile:  "/etc/certs/ca.pem",
    }

    // Create transport
    transport, err := natscqrs.NewTransport(&natscqrs.TransportConfig{
        URL:       "nats://prod.example.com:4222",
        Name:      "my-service",
        TLSConfig: tlsConfig,
    })
    if err != nil {
        log.Fatalf("Failed to create transport: %v", err)
    }
    defer transport.Close()

    // Verify connection is encrypted
    if !transport.IsConnected() {
        log.Fatal("Not connected")
    }

    log.Println("Connected with TLS encryption")
}
```

### Example 2: Embedded NATS Server with TLS

```go
package main

import (
    "log"

    natsserver "github.com/plaenen/eventstore/pkg/embeddednats"
    "github.com/plaenen/eventstore/pkg/security/tls"
)

func main() {
    // Start embedded NATS server with TLS
    srv, err := natsserver.StartEmbeddedServer(
        natsserver.WithPort(4222),
        natsserver.WithTLS(
            "/etc/certs/server-cert.pem",
            "/etc/certs/server-key.pem",
            "/etc/certs/ca.pem",
        ),
    )
    if err != nil {
        log.Fatalf("Failed to start server: %v", err)
    }
    defer srv.Shutdown()

    log.Printf("NATS server with TLS running on %s", srv.URL())

    // Server will now only accept TLS connections
    select {} // Keep running
}
```

### Example 3: Mutual TLS (mTLS) for High Security

```go
package main

import (
    "log"

    natscqrs "github.com/plaenen/eventstore/pkg/cqrs/nats"
    natsserver "github.com/plaenen/eventstore/pkg/embeddednats"
    "github.com/plaenen/eventstore/pkg/security/tls"
)

func main() {
    // 1. Start server with mTLS
    srv, err := natsserver.StartEmbeddedServer(
        natsserver.WithMutualTLS(
            "/etc/certs/server-cert.pem",
            "/etc/certs/server-key.pem",
            "/etc/certs/ca.pem",
        ),
    )
    if err != nil {
        log.Fatal(err)
    }
    defer srv.Shutdown()

    // 2. Connect client with client certificate
    clientTLS := tls.MutualTLSConfig(
        "/etc/certs/client-cert.pem",
        "/etc/certs/client-key.pem",
        "/etc/certs/ca.pem",
    )

    transport, err := natscqrs.NewTransport(&natscqrs.TransportConfig{
        URL:       srv.URL(),
        TLSConfig: clientTLS,
    })
    if err != nil {
        log.Fatal(err)
    }
    defer transport.Close()

    log.Println("Mutual TLS connection established")
    // Both server and client are now mutually authenticated
}
```

### Example 4: Custom TLS Configuration

```go
// Fine-grained control over TLS settings
tlsConfig := &tls.Config{
    Enabled:    true,
    CertFile:   "/etc/certs/cert.pem",
    KeyFile:    "/etc/certs/key.pem",
    CAFile:     "/etc/certs/ca.pem",

    // Strict TLS version requirements
    MinVersion: tls.VersionTLS13, // Require TLS 1.3
    MaxVersion: tls.VersionTLS13,

    // Custom cipher suites
    CipherSuites: []uint16{
        tls.TLS_AES_256_GCM_SHA384,
        tls.TLS_CHACHA20_POLY1305_SHA256,
    },

    // SNI (Server Name Indication)
    ServerName: "api.example.com",
}

// Validate configuration before use
if err := tlsConfig.Validate(); err != nil {
    log.Fatalf("Invalid TLS config: %v", err)
}

// Check if configuration is production-ready
if !tlsConfig.IsSecure() {
    log.Fatal("TLS configuration is not secure for production")
}

// Build standard library tls.Config
stdConfig, err := tlsConfig.BuildTLSConfig()
if err != nil {
    log.Fatal(err)
}
```

### Example 5: Configuration Validation

```go
// Validate TLS configuration
tlsConfig := tls.ProductionConfig("cert.pem", "key.pem", "ca.pem")

// Check if configuration is valid
if err := tlsConfig.Validate(); err != nil {
    log.Fatalf("Invalid TLS config: %v", err)
}

// Check if configuration is secure for production
if !tlsConfig.IsSecure() {
    log.Fatal("Configuration is not secure")
}

// Check if mutual TLS is enabled
if tlsConfig.IsMutualTLS() {
    log.Println("Mutual TLS is enabled")
}

// Get human-readable TLS version
log.Printf("Min TLS version: %s", tls.GetTLSVersion(tlsConfig.MinVersion))
log.Printf("Max TLS version: %s", tls.GetTLSVersion(tlsConfig.MaxVersion))

// List configured cipher suites
for _, suite := range tlsConfig.CipherSuites {
    log.Printf("Cipher suite: %s", tls.GetCipherSuiteName(suite))
}
```

## NATS Integration

### NATS Transport with TLS

```go
import (
    natscqrs "github.com/plaenen/eventstore/pkg/cqrs/nats"
    "github.com/plaenen/eventstore/pkg/security/tls"
)

// Create TLS-enabled NATS transport
tlsConfig := &tls.Config{
    Enabled: true,
    CAFile:  "/etc/certs/ca.pem",
}

transport, err := natscqrs.NewTransport(&natscqrs.TransportConfig{
    URL:       "nats://secure-nats:4222",
    TLSConfig: tlsConfig,
})
```

### Embedded NATS Server with TLS

```go
import (
    natsserver "github.com/plaenen/eventstore/pkg/embeddednats"
)

// Method 1: Using WithTLSConfig
tlsConfig := tls.ProductionConfig("server.crt", "server.key", "ca.crt")
srv, err := natsserver.StartEmbeddedServer(
    natsserver.WithTLSConfig(tlsConfig),
)

// Method 2: Using WithTLS (convenience)
srv, err := natsserver.StartEmbeddedServer(
    natsserver.WithTLS("server.crt", "server.key", "ca.crt"),
)

// Method 3: Using WithMutualTLS
srv, err := natsserver.StartEmbeddedServer(
    natsserver.WithMutualTLS("server.crt", "server.key", "ca.crt"),
)
```

## Certificate Management

### Certificate Requirements

#### Server Certificates

Must include:
- Server authentication extended key usage (`ExtKeyUsageServerAuth`)
- Subject Alternative Names (SANs) for hostnames and IPs
- Valid from trusted CA

Example certificate generation:
```bash
# Generate server certificate
openssl req -new -x509 -days 365 \
  -key server.key \
  -out server.crt \
  -subj "/CN=myserver.example.com" \
  -addext "subjectAltName=DNS:myserver.example.com,IP:10.0.0.1"
```

#### Client Certificates (for mTLS)

Must include:
- Client authentication extended key usage (`ExtKeyUsageClientAuth`)
- Valid from trusted CA

```bash
# Generate client certificate
openssl req -new -x509 -days 365 \
  -key client.key \
  -out client.crt \
  -subj "/CN=client-service"
```

### Certificate File Formats

The package supports PEM-encoded certificates:

```
# Certificate file (cert.pem)
-----BEGIN CERTIFICATE-----
MIIDXTCCAkWgAwIBAgIJAKL0UG+mRkSvMA0GCSqGSIb3DQEBCwUAMEUxCzAJBgNV
...
-----END CERTIFICATE-----

# Private key file (key.pem)
-----BEGIN RSA PRIVATE KEY-----
MIIEpAIBAAKCAQEA2Z3qX9Cy7UbHPRpOueKLvmfqCAGO+3e9MWDgNq4u0qQr6zNq
...
-----END RSA PRIVATE KEY-----
```

### Certificate Rotation

For zero-downtime certificate rotation:

1. Generate new certificates
2. Update configuration files
3. Reload configuration (if using dynamic config)
4. Old connections continue with old certificates
5. New connections use new certificates

```go
// Example with configuration management
import "github.com/plaenen/eventstore/pkg/config"

// Watch for TLS config updates
type TLSConfig struct {
    CertFile string `json:"cert_file"`
    KeyFile  string `json:"key_file"`
    CAFile   string `json:"ca_file"`
}

provider, _ := config.NewProvider[TLSConfig](ctx, "awsparamstore:///prod/tls")

// Auto-reload on certificate rotation
stop, _ := provider.Watch(ctx, func(cfg TLSConfig) {
    // Recreate transport with new certificates
    newTLS := &tls.Config{
        Enabled:  true,
        CertFile: cfg.CertFile,
        KeyFile:  cfg.KeyFile,
        CAFile:   cfg.CAFile,
    }
    // Update transport...
})
defer stop()
```

## Production Deployment

### Deployment Checklist

- [ ] Use production-signed certificates (not self-signed)
- [ ] Enable certificate verification (`InsecureSkipVerify: false`)
- [ ] Use minimum TLS 1.2 or higher
- [ ] Configure appropriate cipher suites
- [ ] Set certificate expiration monitoring
- [ ] Plan certificate rotation strategy
- [ ] Use mutual TLS for internal services
- [ ] Store private keys securely (encrypted at rest)
- [ ] Limit private key permissions (chmod 600)
- [ ] Use separate certificates for dev/staging/production

### Environment-Specific Configuration

#### Development

```go
// Development - Local testing with self-signed certs
tlsConfig := &tls.Config{
    Enabled:            true,
    CertFile:           "dev-cert.pem",
    KeyFile:            "dev-key.pem",
    InsecureSkipVerify: true, // ⚠️ OK for development only
}
```

#### Staging

```go
// Staging - Real certificates, but relaxed settings
tlsConfig := &tls.Config{
    Enabled:    true,
    CertFile:   "/etc/certs/staging-cert.pem",
    KeyFile:    "/etc/certs/staging-key.pem",
    CAFile:     "/etc/certs/staging-ca.pem",
    MinVersion: tls.VersionTLS12,
}
```

#### Production

```go
// Production - Strict security
tlsConfig := &tls.Config{
    Enabled:    true,
    CertFile:   "/etc/certs/prod-cert.pem",
    KeyFile:    "/etc/certs/prod-key.pem",
    CAFile:     "/etc/certs/prod-ca.pem",
    ClientAuth: true, // mTLS for internal services
    MinVersion: tls.VersionTLS12,
    MaxVersion: tls.VersionTLS13,
}

// Validate before deployment
if err := tlsConfig.Validate(); err != nil {
    panic(fmt.Sprintf("Invalid TLS config: %v", err))
}

if !tlsConfig.IsSecure() {
    panic("TLS configuration is not secure for production")
}
```

### Kubernetes Deployment

#### Using Kubernetes Secrets

```yaml
# Create secret with certificates
apiVersion: v1
kind: Secret
metadata:
  name: nats-tls-certs
type: kubernetes.io/tls
data:
  tls.crt: <base64-encoded-cert>
  tls.key: <base64-encoded-key>
  ca.crt:  <base64-encoded-ca>
---
# Mount secrets in pod
apiVersion: v1
kind: Pod
metadata:
  name: eventsourcing-service
spec:
  containers:
  - name: service
    image: myservice:latest
    volumeMounts:
    - name: tls-certs
      mountPath: /etc/certs
      readOnly: true
  volumes:
  - name: tls-certs
    secret:
      secretName: nats-tls-certs
      defaultMode: 0400
```

#### Application Configuration

```go
// Read certificates from Kubernetes-mounted volume
tlsConfig := &tls.Config{
    Enabled:  true,
    CertFile: "/etc/certs/tls.crt",
    KeyFile:  "/etc/certs/tls.key",
    CAFile:   "/etc/certs/ca.crt",
}
```

### Docker Deployment

```dockerfile
# Dockerfile
FROM golang:1.21 AS builder
WORKDIR /app
COPY . .
RUN go build -o service .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/

# Copy application
COPY --from=builder /app/service .

# Certificates will be mounted at runtime
VOLUME /etc/certs

CMD ["./service"]
```

```bash
# Run with mounted certificates
docker run -d \
  -v /path/to/certs:/etc/certs:ro \
  -e TLS_CERT_FILE=/etc/certs/cert.pem \
  -e TLS_KEY_FILE=/etc/certs/key.pem \
  -e TLS_CA_FILE=/etc/certs/ca.pem \
  myservice:latest
```

## Security Best Practices

### ✅ DO's

1. **Use TLS 1.2 or Higher**
   ```go
   tlsConfig.MinVersion = tls.VersionTLS12 // Minimum
   tlsConfig.MaxVersion = tls.VersionTLS13 // Recommended
   ```

2. **Use Mutual TLS for Internal Services**
   ```go
   tlsConfig := tls.MutualTLSConfig("cert.pem", "key.pem", "ca.pem")
   ```

3. **Verify Certificates**
   ```go
   tlsConfig.InsecureSkipVerify = false // Always in production
   ```

4. **Use Strong Cipher Suites**
   ```go
   tlsConfig.CipherSuites = []uint16{
       tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
       tls.TLS_AES_256_GCM_SHA384,
       tls.TLS_CHACHA20_POLY1305_SHA256,
   }
   ```

5. **Protect Private Keys**
   ```bash
   chmod 600 /etc/certs/*.key
   chown service:service /etc/certs/*.key
   ```

6. **Monitor Certificate Expiration**
   ```go
   // Check certificate expiration
   cert, _ := tls.LoadX509KeyPair(certFile, keyFile)
   leaf, _ := x509.ParseCertificate(cert.Certificate[0])
   if time.Until(leaf.NotAfter) < 30*24*time.Hour {
       log.Warn("Certificate expires in less than 30 days")
   }
   ```

7. **Rotate Certificates Regularly**
   - Set up automated certificate rotation
   - Use short-lived certificates (90 days or less)
   - Test rotation in staging before production

8. **Use Production CAs**
   - Let's Encrypt (free, automated)
   - DigiCert, Sectigo (commercial)
   - Internal CA for internal services

### ❌ DON'Ts

1. **NEVER Use InsecureSkipVerify in Production**
   ```go
   // ❌ DANGEROUS - Opens up to MITM attacks
   tlsConfig.InsecureSkipVerify = true
   ```

2. **NEVER Use Self-Signed Certificates in Production**
   - Use proper CA-signed certificates
   - Self-signed certs acceptable only for development

3. **NEVER Use TLS 1.0 or 1.1**
   ```go
   // ❌ INSECURE - Deprecated protocols
   tlsConfig.MinVersion = tls.VersionTLS10 // Don't do this!
   tlsConfig.MinVersion = tls.VersionTLS11 // Or this!
   ```

4. **NEVER Commit Private Keys to Git**
   ```bash
   # Add to .gitignore
   *.key
   *.pem
   /certs/
   ```

5. **NEVER Share Private Keys**
   - Each service should have unique certificates
   - Use separate certs for different environments

6. **NEVER Ignore Certificate Errors**
   ```go
   // ❌ BAD - Silently ignoring errors
   _, err := tlsConfig.BuildTLSConfig()
   // Don't ignore this error!
   ```

7. **NEVER Use Weak Cipher Suites**
   ```go
   // ❌ INSECURE - Deprecated cipher suites
   tls.TLS_RSA_WITH_RC4_128_SHA           // Don't use RC4
   tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA      // Don't use 3DES
   ```

## Troubleshooting

### Common Issues

#### 1. Certificate Verification Failed

**Error**: `x509: certificate signed by unknown authority`

**Solution**:
```go
// Ensure CA file is specified
tlsConfig.CAFile = "/path/to/ca.pem"

// OR for development only
tlsConfig.InsecureSkipVerify = true // ⚠️ Development only!
```

#### 2. Certificate Hostname Mismatch

**Error**: `x509: certificate is valid for server.local, not 192.168.1.100`

**Solutions**:
```go
// Option 1: Use the correct hostname
transport, err := natscqrs.NewTransport(&natscqrs.TransportConfig{
    URL: "nats://server.local:4222", // Use DNS name
})

// Option 2: Set ServerName for SNI
tlsConfig.ServerName = "server.local"

// Option 3: Add IP SAN to certificate
// Regenerate certificate with IP in Subject Alternative Names
```

#### 3. Missing Client Certificate (mTLS)

**Error**: `tls: client didn't provide a certificate`

**Solution**:
```go
// Client must provide certificate for mTLS
clientTLS := &tls.Config{
    Enabled:  true,
    CertFile: "client-cert.pem", // Required for mTLS
    KeyFile:  "client-key.pem",  // Required for mTLS
    CAFile:   "ca.pem",
}
```

#### 4. Protocol Version Mismatch

**Error**: `tls: protocol version not supported`

**Solution**:
```go
// Ensure compatible TLS versions
// Server minimum ≤ Client maximum
// Client minimum ≤ Server maximum

// Server
serverTLS.MinVersion = tls.VersionTLS12
serverTLS.MaxVersion = tls.VersionTLS13

// Client
clientTLS.MinVersion = tls.VersionTLS12
clientTLS.MaxVersion = tls.VersionTLS13
```

#### 5. File Permission Errors

**Error**: `permission denied` when reading certificate files

**Solution**:
```bash
# Set correct permissions
chmod 644 cert.pem    # Certificate (world-readable)
chmod 600 key.pem     # Private key (owner only)
chown myservice:myservice *.pem
```

#### 6. Certificate Expired

**Error**: `x509: certificate has expired or is not yet valid`

**Solution**:
```bash
# Check certificate expiration
openssl x509 -in cert.pem -text -noout | grep "Not After"

# Renew certificate before expiration
# Set up monitoring for expiration dates
```

### Debugging TLS Issues

#### Enable Debug Logging

```go
// Enable NATS debug logging
srv, err := natsserver.StartEmbeddedServer(
    natsserver.WithDebug(true),
    natsserver.WithTrace(true),
    natsserver.WithTLSConfig(tlsConfig),
)
```

#### Verify Certificate Chain

```bash
# Check certificate
openssl x509 -in cert.pem -text -noout

# Verify certificate chain
openssl verify -CAfile ca.pem cert.pem

# Test TLS connection
openssl s_client -connect server:4222 -CAfile ca.pem

# Test with client certificate
openssl s_client -connect server:4222 \
  -cert client-cert.pem \
  -key client-key.pem \
  -CAfile ca.pem
```

#### Check Certificate Details

```go
// Load and inspect certificate
cert, err := tls.LoadX509KeyPair(certFile, keyFile)
if err != nil {
    log.Fatal(err)
}

leaf, err := x509.ParseCertificate(cert.Certificate[0])
if err != nil {
    log.Fatal(err)
}

// Print certificate details
log.Printf("Subject: %s", leaf.Subject)
log.Printf("Issuer: %s", leaf.Issuer)
log.Printf("Valid from: %s", leaf.NotBefore)
log.Printf("Valid until: %s", leaf.NotAfter)
log.Printf("DNS Names: %v", leaf.DNSNames)
log.Printf("IP Addresses: %v", leaf.IPAddresses)
```

## API Reference

### Config

```go
type Config struct {
    Enabled            bool
    CertFile           string
    KeyFile            string
    CAFile             string
    InsecureSkipVerify bool
    ClientAuth         bool
    ServerName         string
    MinVersion         uint16
    MaxVersion         uint16
    CipherSuites       []uint16
    RootCAs            *x509.CertPool
    Certificates       []tls.Certificate
}
```

### Methods

#### Validation

```go
// Validate checks if the TLS configuration is valid
func (c *Config) Validate() error

// IsSecure returns true if TLS is properly configured for production
func (c *Config) IsSecure() bool

// IsMutualTLS returns true if mutual TLS is enabled
func (c *Config) IsMutualTLS() bool
```

#### Building TLS Config

```go
// BuildTLSConfig creates a standard library tls.Config
func (c *Config) BuildTLSConfig() (*tls.Config, error)

// BuildClientTLSConfig creates a client TLS config
func (c *Config) BuildClientTLSConfig() (*tls.Config, error)

// BuildServerTLSConfig creates a server TLS config
func (c *Config) BuildServerTLSConfig() (*tls.Config, error)
```

### Helper Functions

```go
// DefaultConfig returns a secure default TLS configuration
func DefaultConfig() *Config

// DevelopmentConfig returns a TLS config suitable for development
// ⚠️ WARNING: This skips certificate verification - NEVER use in production!
func DevelopmentConfig() *Config

// ProductionConfig returns a strict TLS config for production
func ProductionConfig(certFile, keyFile, caFile string) *Config

// MutualTLSConfig returns a config with client authentication enabled
func MutualTLSConfig(certFile, keyFile, caFile string) *Config

// GetTLSVersion returns a human-readable TLS version string
func GetTLSVersion(version uint16) string

// GetCipherSuiteName returns a human-readable cipher suite name
func GetCipherSuiteName(cipherSuite uint16) string
```

### Errors

```go
var (
    // ErrTLSNotEnabled is returned when TLS is required but not enabled
    ErrTLSNotEnabled = errors.New("TLS is not enabled")

    // ErrInvalidCertificate is returned when certificate validation fails
    ErrInvalidCertificate = errors.New("invalid certificate")

    // ErrMissingCAFile is returned when CA file is required but not provided
    ErrMissingCAFile = errors.New("CA file is required for certificate verification")

    // ErrMissingCertOrKey is returned when cert or key file is missing
    ErrMissingCertOrKey = errors.New("both certificate and key files are required")
)
```

## Examples

See the [TLS/mTLS example](../../../examples/cmd/security-examples/tls-mtls) for a comprehensive demonstration including:

- Self-signed certificate generation
- Basic TLS setup
- Mutual TLS (mTLS) setup
- Configuration validation
- Security best practices

Run the example:

```bash
cd examples/cmd/security-examples/tls-mtls
go run main.go
```

## Related Packages

- **[pkg/security/credentials](../credentials)** - Secure credential management (SEC-001)
- **[pkg/cqrs/nats](../../cqrs/nats)** - NATS transport with TLS support
- **[pkg/infrastructure/nats](../../infrastructure/nats)** - Embedded NATS server with TLS

## Security Roadmap

This package implements:

- ✅ **SEC-002: TLS/Encryption** - Complete implementation
  - Transport Layer Security (TLS)
  - Mutual TLS (mTLS)
  - Certificate management
  - NATS integration

Related security features:

- ✅ **SEC-001: Authentication & Credentials** - Secure credential management
- 🔲 **SEC-003: RBAC** - Role-based access control (planned)
- 🔲 **SEC-004: Audit Logging** - Security audit trails (planned)
- 🔲 **SEC-005: Encryption at Rest** - Data encryption (planned)

## License

Copyright © 2024 EventSourcing Framework
