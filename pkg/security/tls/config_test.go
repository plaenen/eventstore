package tls

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if !cfg.Enabled {
		t.Error("DefaultConfig should be enabled")
	}

	if cfg.MinVersion != tls.VersionTLS12 {
		t.Errorf("Expected MinVersion to be TLS 1.2, got %v", cfg.MinVersion)
	}

	if cfg.MaxVersion != tls.VersionTLS13 {
		t.Errorf("Expected MaxVersion to be TLS 1.3, got %v", cfg.MaxVersion)
	}

	if len(cfg.CipherSuites) == 0 {
		t.Error("DefaultConfig should have cipher suites configured")
	}

	if cfg.InsecureSkipVerify {
		t.Error("DefaultConfig should not skip verification")
	}
}

func TestDevelopmentConfig(t *testing.T) {
	cfg := DevelopmentConfig()

	if !cfg.Enabled {
		t.Error("DevelopmentConfig should be enabled")
	}

	if !cfg.InsecureSkipVerify {
		t.Error("DevelopmentConfig should skip verification")
	}

	if cfg.MinVersion != tls.VersionTLS12 {
		t.Errorf("Expected MinVersion to be TLS 1.2, got %v", cfg.MinVersion)
	}
}

func TestProductionConfig(t *testing.T) {
	cfg := ProductionConfig("cert.pem", "key.pem", "ca.pem")

	if !cfg.Enabled {
		t.Error("ProductionConfig should be enabled")
	}

	if cfg.InsecureSkipVerify {
		t.Error("ProductionConfig should not skip verification")
	}

	if cfg.CertFile != "cert.pem" {
		t.Errorf("Expected CertFile to be 'cert.pem', got %s", cfg.CertFile)
	}

	if cfg.KeyFile != "key.pem" {
		t.Errorf("Expected KeyFile to be 'key.pem', got %s", cfg.KeyFile)
	}

	if cfg.CAFile != "ca.pem" {
		t.Errorf("Expected CAFile to be 'ca.pem', got %s", cfg.CAFile)
	}

	if cfg.MinVersion != tls.VersionTLS12 {
		t.Errorf("Expected MinVersion to be TLS 1.2, got %v", cfg.MinVersion)
	}

	if cfg.MaxVersion != tls.VersionTLS13 {
		t.Errorf("Expected MaxVersion to be TLS 1.3, got %v", cfg.MaxVersion)
	}
}

func TestMutualTLSConfig(t *testing.T) {
	cfg := MutualTLSConfig("cert.pem", "key.pem", "ca.pem")

	if !cfg.ClientAuth {
		t.Error("MutualTLSConfig should have ClientAuth enabled")
	}

	if !cfg.IsMutualTLS() {
		t.Error("MutualTLSConfig should be identified as mutual TLS")
	}
}

func TestIsMutualTLS(t *testing.T) {
	tests := []struct {
		name     string
		config   *Config
		expected bool
	}{
		{
			name: "mTLS enabled",
			config: &Config{
				Enabled:    true,
				ClientAuth: true,
			},
			expected: true,
		},
		{
			name: "TLS enabled but not mTLS",
			config: &Config{
				Enabled:    true,
				ClientAuth: false,
			},
			expected: false,
		},
		{
			name: "TLS disabled",
			config: &Config{
				Enabled:    false,
				ClientAuth: true,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.config.IsMutualTLS(); got != tt.expected {
				t.Errorf("IsMutualTLS() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestIsSecure(t *testing.T) {
	// Create temporary directory for test certificates
	tmpDir, err := os.MkdirTemp("", "tls-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Generate test certificates
	caCertFile, caKeyFile := generateTestCA(t, tmpDir)
	certFile, keyFile := generateTestCert(t, tmpDir, caCertFile, caKeyFile)

	tests := []struct {
		name     string
		config   *Config
		expected bool
	}{
		{
			name: "secure production config",
			config: &Config{
				Enabled:    true,
				CertFile:   certFile,
				KeyFile:    keyFile,
				CAFile:     caCertFile,
				MinVersion: tls.VersionTLS12,
			},
			expected: true,
		},
		{
			name: "disabled TLS",
			config: &Config{
				Enabled: false,
			},
			expected: false,
		},
		{
			name: "skip verify enabled",
			config: &Config{
				Enabled:            true,
				CertFile:           certFile,
				InsecureSkipVerify: true,
				MinVersion:         tls.VersionTLS12,
			},
			expected: false,
		},
		{
			name: "TLS version too old",
			config: &Config{
				Enabled:    true,
				CertFile:   certFile,
				CAFile:     caCertFile,
				MinVersion: tls.VersionTLS11,
			},
			expected: false,
		},
		{
			name: "no certificates",
			config: &Config{
				Enabled:    true,
				CAFile:     caCertFile,
				MinVersion: tls.VersionTLS12,
			},
			expected: false,
		},
		{
			name: "no CA file and no skip verify",
			config: &Config{
				Enabled:    true,
				CertFile:   certFile,
				MinVersion: tls.VersionTLS12,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.config.IsSecure(); got != tt.expected {
				t.Errorf("IsSecure() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	// Create temporary directory for test certificates
	tmpDir, err := os.MkdirTemp("", "tls-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Generate test certificates
	caCertFile, caKeyFile := generateTestCA(t, tmpDir)
	certFile, keyFile := generateTestCert(t, tmpDir, caCertFile, caKeyFile)

	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: &Config{
				Enabled:  true,
				CertFile: certFile,
				KeyFile:  keyFile,
				CAFile:   caCertFile,
			},
			wantErr: false,
		},
		{
			name: "disabled TLS",
			config: &Config{
				Enabled: false,
			},
			wantErr: false,
		},
		{
			name: "missing key file",
			config: &Config{
				Enabled:  true,
				CertFile: certFile,
			},
			wantErr: true,
		},
		{
			name: "missing cert file",
			config: &Config{
				Enabled: true,
				KeyFile: keyFile,
			},
			wantErr: true,
		},
		{
			name: "non-existent cert file",
			config: &Config{
				Enabled:  true,
				CertFile: "/non/existent/cert.pem",
				KeyFile:  keyFile,
			},
			wantErr: true,
		},
		{
			name: "non-existent key file",
			config: &Config{
				Enabled:  true,
				CertFile: certFile,
				KeyFile:  "/non/existent/key.pem",
			},
			wantErr: true,
		},
		{
			name: "non-existent CA file",
			config: &Config{
				Enabled:  true,
				CertFile: certFile,
				KeyFile:  keyFile,
				CAFile:   "/non/existent/ca.pem",
			},
			wantErr: true,
		},
		{
			name: "min version greater than max version",
			config: &Config{
				Enabled:    true,
				MinVersion: tls.VersionTLS13,
				MaxVersion: tls.VersionTLS12,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestBuildTLSConfig(t *testing.T) {
	// Create temporary directory for test certificates
	tmpDir, err := os.MkdirTemp("", "tls-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Generate test certificates
	caCertFile, caKeyFile := generateTestCA(t, tmpDir)
	certFile, keyFile := generateTestCert(t, tmpDir, caCertFile, caKeyFile)

	t.Run("build client TLS config", func(t *testing.T) {
		cfg := &Config{
			Enabled:    true,
			CertFile:   certFile,
			KeyFile:    keyFile,
			CAFile:     caCertFile,
			MinVersion: tls.VersionTLS12,
			MaxVersion: tls.VersionTLS13,
		}

		tlsConfig, err := cfg.BuildClientTLSConfig()
		if err != nil {
			t.Fatalf("BuildClientTLSConfig() error = %v", err)
		}

		if tlsConfig.MinVersion != tls.VersionTLS12 {
			t.Errorf("Expected MinVersion to be TLS 1.2, got %v", tlsConfig.MinVersion)
		}

		if tlsConfig.MaxVersion != tls.VersionTLS13 {
			t.Errorf("Expected MaxVersion to be TLS 1.3, got %v", tlsConfig.MaxVersion)
		}

		if len(tlsConfig.Certificates) == 0 {
			t.Error("Expected certificates to be loaded")
		}

		if tlsConfig.RootCAs == nil {
			t.Error("Expected RootCAs to be set")
		}
	})

	t.Run("build server TLS config", func(t *testing.T) {
		cfg := &Config{
			Enabled:    true,
			CertFile:   certFile,
			KeyFile:    keyFile,
			CAFile:     caCertFile,
			ClientAuth: true,
		}

		tlsConfig, err := cfg.BuildServerTLSConfig()
		if err != nil {
			t.Fatalf("BuildServerTLSConfig() error = %v", err)
		}

		if tlsConfig.ClientAuth != tls.RequireAndVerifyClientCert {
			t.Errorf("Expected ClientAuth to be RequireAndVerifyClientCert, got %v", tlsConfig.ClientAuth)
		}

		if tlsConfig.ClientCAs == nil {
			t.Error("Expected ClientCAs to be set for mTLS")
		}
	})

	t.Run("disabled TLS", func(t *testing.T) {
		cfg := &Config{
			Enabled: false,
		}

		_, err := cfg.BuildTLSConfig()
		if err != ErrTLSNotEnabled {
			t.Errorf("Expected ErrTLSNotEnabled, got %v", err)
		}
	})
}

func TestGetTLSVersion(t *testing.T) {
	tests := []struct {
		version  uint16
		expected string
	}{
		{tls.VersionTLS10, "TLS 1.0"},
		{tls.VersionTLS11, "TLS 1.1"},
		{tls.VersionTLS12, "TLS 1.2"},
		{tls.VersionTLS13, "TLS 1.3"},
		{0x9999, "Unknown (0x9999)"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := GetTLSVersion(tt.version)
			if got != tt.expected {
				t.Errorf("GetTLSVersion(%v) = %v, want %v", tt.version, got, tt.expected)
			}
		})
	}
}

func TestGetCipherSuiteName(t *testing.T) {
	// Test a known cipher suite
	name := GetCipherSuiteName(tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256)
	if name == "" {
		t.Error("Expected non-empty cipher suite name")
	}

	// Test unknown cipher suite - tls.CipherSuiteName returns the hex representation
	name = GetCipherSuiteName(0x9999)
	// The standard library returns "0x9999" for unknown suites
	if name == "" {
		t.Error("Expected non-empty cipher suite name")
	}
}

// Helper functions for testing

func generateTestCA(t *testing.T, dir string) (certFile, keyFile string) {
	t.Helper()

	// Generate private key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate private key: %v", err)
	}

	// Create certificate template
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test CA"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	// Create certificate
	certBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatalf("Failed to create certificate: %v", err)
	}

	// Write certificate
	certFile = filepath.Join(dir, "ca.crt")
	certOut, err := os.Create(certFile)
	if err != nil {
		t.Fatalf("Failed to create cert file: %v", err)
	}
	defer certOut.Close()

	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: certBytes}); err != nil {
		t.Fatalf("Failed to encode certificate: %v", err)
	}

	// Write private key
	keyFile = filepath.Join(dir, "ca.key")
	keyOut, err := os.Create(keyFile)
	if err != nil {
		t.Fatalf("Failed to create key file: %v", err)
	}
	defer keyOut.Close()

	if err := pem.Encode(keyOut, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}); err != nil {
		t.Fatalf("Failed to encode private key: %v", err)
	}

	return certFile, keyFile
}

func generateTestCert(t *testing.T, dir, caCertFile, caKeyFile string) (certFile, keyFile string) {
	t.Helper()

	// Load CA certificate
	caCertPEM, err := os.ReadFile(caCertFile)
	if err != nil {
		t.Fatalf("Failed to read CA cert: %v", err)
	}
	caCertBlock, _ := pem.Decode(caCertPEM)
	caCert, err := x509.ParseCertificate(caCertBlock.Bytes)
	if err != nil {
		t.Fatalf("Failed to parse CA cert: %v", err)
	}

	// Load CA private key
	caKeyPEM, err := os.ReadFile(caKeyFile)
	if err != nil {
		t.Fatalf("Failed to read CA key: %v", err)
	}
	caKeyBlock, _ := pem.Decode(caKeyPEM)
	caKey, err := x509.ParsePKCS1PrivateKey(caKeyBlock.Bytes)
	if err != nil {
		t.Fatalf("Failed to parse CA key: %v", err)
	}

	// Generate certificate private key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate private key: %v", err)
	}

	// Create certificate template
	template := x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject: pkix.Name{
			Organization: []string{"Test"},
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().Add(24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}

	// Create certificate
	certBytes, err := x509.CreateCertificate(rand.Reader, &template, caCert, &privateKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("Failed to create certificate: %v", err)
	}

	// Write certificate
	certFile = filepath.Join(dir, "cert.crt")
	certOut, err := os.Create(certFile)
	if err != nil {
		t.Fatalf("Failed to create cert file: %v", err)
	}
	defer certOut.Close()

	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: certBytes}); err != nil {
		t.Fatalf("Failed to encode certificate: %v", err)
	}

	// Write private key
	keyFile = filepath.Join(dir, "cert.key")
	keyOut, err := os.Create(keyFile)
	if err != nil {
		t.Fatalf("Failed to create key file: %v", err)
	}
	defer keyOut.Close()

	if err := pem.Encode(keyOut, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}); err != nil {
		t.Fatalf("Failed to encode private key: %v", err)
	}

	return certFile, keyFile
}
