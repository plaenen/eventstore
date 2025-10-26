// Package tls provides TLS configuration and certificate management for secure transport.
//
// This package implements SEC-002 (TLS/Encryption) from the security roadmap, providing
// comprehensive TLS support for NATS connections and other network services.
//
// Example usage:
//
//	// Basic TLS with certificate files
//	config := &tls.Config{
//	    Enabled:  true,
//	    CertFile: "/path/to/cert.pem",
//	    KeyFile:  "/path/to/key.pem",
//	    CAFile:   "/path/to/ca.pem",
//	}
//
//	tlsConfig, err := config.BuildTLSConfig()
//
//	// Mutual TLS (mTLS)
//	config := &tls.Config{
//	    Enabled:    true,
//	    CertFile:   "/path/to/client-cert.pem",
//	    KeyFile:    "/path/to/client-key.pem",
//	    CAFile:     "/path/to/ca.pem",
//	    ClientAuth: true,
//	}
package tls

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
)

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

// Config represents TLS configuration
type Config struct {
	// Enabled indicates if TLS should be used
	Enabled bool

	// CertFile is the path to the TLS certificate file
	CertFile string

	// KeyFile is the path to the TLS private key file
	KeyFile string

	// CAFile is the path to the CA certificate file for verification
	CAFile string

	// InsecureSkipVerify disables certificate verification
	// WARNING: Only use for development/testing!
	InsecureSkipVerify bool

	// ClientAuth enables mutual TLS (client certificate authentication)
	ClientAuth bool

	// ServerName for certificate verification (SNI)
	ServerName string

	// MinVersion is the minimum TLS version (default: TLS 1.2)
	MinVersion uint16

	// MaxVersion is the maximum TLS version (default: TLS 1.3)
	MaxVersion uint16

	// CipherSuites is the list of enabled cipher suites
	// If empty, Go's default secure cipher suites are used
	CipherSuites []uint16

	// RootCAs is a pool of root certificates (alternative to CAFile)
	RootCAs *x509.CertPool

	// Certificates is a list of certificates (alternative to CertFile/KeyFile)
	Certificates []tls.Certificate
}

// DefaultConfig returns a secure default TLS configuration
func DefaultConfig() *Config {
	return &Config{
		Enabled:    true,
		MinVersion: tls.VersionTLS12,
		MaxVersion: tls.VersionTLS13,
		// Use Go's default secure cipher suites
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_AES_128_GCM_SHA256,
			tls.TLS_AES_256_GCM_SHA384,
			tls.TLS_CHACHA20_POLY1305_SHA256,
		},
	}
}

// DevelopmentConfig returns a TLS config suitable for development
// WARNING: This skips certificate verification - NEVER use in production!
func DevelopmentConfig() *Config {
	return &Config{
		Enabled:            true,
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS12,
	}
}

// ProductionConfig returns a strict TLS config for production
func ProductionConfig(certFile, keyFile, caFile string) *Config {
	return &Config{
		Enabled:            true,
		CertFile:           certFile,
		KeyFile:            keyFile,
		CAFile:             caFile,
		MinVersion:         tls.VersionTLS12,
		MaxVersion:         tls.VersionTLS13,
		InsecureSkipVerify: false,
		ClientAuth:         false,
	}
}

// MutualTLSConfig returns a config with client authentication enabled
func MutualTLSConfig(certFile, keyFile, caFile string) *Config {
	config := ProductionConfig(certFile, keyFile, caFile)
	config.ClientAuth = true
	return config
}

// Validate checks if the TLS configuration is valid
func (c *Config) Validate() error {
	if !c.Enabled {
		return nil // No validation needed if TLS is disabled
	}

	// Check for certificate and key files
	if c.CertFile != "" || c.KeyFile != "" {
		if c.CertFile == "" || c.KeyFile == "" {
			return ErrMissingCertOrKey
		}

		// Verify files exist
		if _, err := os.Stat(c.CertFile); err != nil {
			return fmt.Errorf("certificate file not found: %w", err)
		}
		if _, err := os.Stat(c.KeyFile); err != nil {
			return fmt.Errorf("key file not found: %w", err)
		}
	}

	// Check CA file if provided
	if c.CAFile != "" {
		if _, err := os.Stat(c.CAFile); err != nil {
			return fmt.Errorf("CA file not found: %w", err)
		}
	}

	// Warn about insecure skip verify
	if c.InsecureSkipVerify && !isDevelopmentMode() {
		return fmt.Errorf("InsecureSkipVerify is enabled in non-development mode - this is unsafe")
	}

	// Validate TLS version
	if c.MinVersion > c.MaxVersion && c.MaxVersion != 0 {
		return fmt.Errorf("MinVersion (%d) cannot be greater than MaxVersion (%d)", c.MinVersion, c.MaxVersion)
	}

	return nil
}

// BuildTLSConfig creates a standard library tls.Config from our Config
func (c *Config) BuildTLSConfig() (*tls.Config, error) {
	if !c.Enabled {
		return nil, ErrTLSNotEnabled
	}

	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("invalid TLS config: %w", err)
	}

	tlsConfig := &tls.Config{
		MinVersion:         c.MinVersion,
		MaxVersion:         c.MaxVersion,
		CipherSuites:       c.CipherSuites,
		InsecureSkipVerify: c.InsecureSkipVerify,
		ServerName:         c.ServerName,
	}

	// Load certificates
	if len(c.Certificates) > 0 {
		tlsConfig.Certificates = c.Certificates
	} else if c.CertFile != "" && c.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(c.CertFile, c.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	// Load CA certificates
	if c.RootCAs != nil {
		tlsConfig.RootCAs = c.RootCAs
	} else if c.CAFile != "" {
		caCert, err := os.ReadFile(c.CAFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA file: %w", err)
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}

		tlsConfig.RootCAs = caCertPool

		// For client authentication, also set ClientCAs
		if c.ClientAuth {
			tlsConfig.ClientCAs = caCertPool
			tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
		}
	}

	return tlsConfig, nil
}

// BuildClientTLSConfig creates a client TLS config
func (c *Config) BuildClientTLSConfig() (*tls.Config, error) {
	return c.BuildTLSConfig()
}

// BuildServerTLSConfig creates a server TLS config
func (c *Config) BuildServerTLSConfig() (*tls.Config, error) {
	tlsConfig, err := c.BuildTLSConfig()
	if err != nil {
		return nil, err
	}

	// Server-specific settings
	if c.ClientAuth && tlsConfig.ClientCAs != nil {
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	}

	return tlsConfig, nil
}

// IsMutualTLS returns true if mutual TLS is enabled
func (c *Config) IsMutualTLS() bool {
	return c.Enabled && c.ClientAuth
}

// IsSecure returns true if TLS is properly configured for production
func (c *Config) IsSecure() bool {
	if !c.Enabled {
		return false
	}

	if c.InsecureSkipVerify {
		return false
	}

	if c.MinVersion < tls.VersionTLS12 {
		return false
	}

	// Should have certificates
	if c.CertFile == "" && len(c.Certificates) == 0 {
		return false
	}

	// Should have CA for verification
	if c.CAFile == "" && c.RootCAs == nil && !c.InsecureSkipVerify {
		return false
	}

	return true
}

// isDevelopmentMode checks if we're running in development mode
// This is a simple heuristic - in production, use proper environment detection
func isDevelopmentMode() bool {
	env := os.Getenv("ENVIRONMENT")
	if env == "" {
		env = os.Getenv("ENV")
	}

	return env == "development" || env == "dev" || env == "local" || env == ""
}

// GetTLSVersion returns a human-readable TLS version string
func GetTLSVersion(version uint16) string {
	switch version {
	case tls.VersionTLS10:
		return "TLS 1.0"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS13:
		return "TLS 1.3"
	default:
		return fmt.Sprintf("Unknown (0x%04x)", version)
	}
}

// GetCipherSuiteName returns a human-readable cipher suite name
func GetCipherSuiteName(cipherSuite uint16) string {
	suite := tls.CipherSuiteName(cipherSuite)
	if suite != "" {
		return suite
	}
	return fmt.Sprintf("Unknown (0x%04x)", cipherSuite)
}
