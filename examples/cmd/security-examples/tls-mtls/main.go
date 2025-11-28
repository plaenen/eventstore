package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"log"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"

	natsserver "github.com/plaenen/eventstore/pkg/embeddednats"
	"github.com/plaenen/eventstore/pkg/security/tls"

	"github.com/nats-io/nats.go"
)

func main() {
	fmt.Println("=== TLS/mTLS Security Example ===")
	fmt.Println()

	// Create temporary directory for certificates
	tmpDir, err := os.MkdirTemp("", "tls-example-*")
	if err != nil {
		log.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	fmt.Printf("📁 Created temporary directory: %s\n", tmpDir)
	fmt.Println()

	// Example 1: Generate self-signed certificates for testing
	fmt.Println("1️⃣  Generating Self-Signed Certificates (for testing)")
	fmt.Println()

	caCertFile, caKeyFile, err := generateCACertificate(tmpDir)
	if err != nil {
		log.Fatalf("Failed to generate CA certificate: %v", err)
	}
	fmt.Printf("   ✅ Generated CA certificate: %s\n", caCertFile)

	serverCertFile, serverKeyFile, err := generateServerCertificate(tmpDir, caCertFile, caKeyFile)
	if err != nil {
		log.Fatalf("Failed to generate server certificate: %v", err)
	}
	fmt.Printf("   ✅ Generated server certificate: %s\n", serverCertFile)

	clientCertFile, clientKeyFile, err := generateClientCertificate(tmpDir, caCertFile, caKeyFile)
	if err != nil {
		log.Fatalf("Failed to generate client certificate: %v", err)
	}
	fmt.Printf("   ✅ Generated client certificate: %s\n", clientCertFile)
	fmt.Println()

	// Example 2: Basic TLS (Server Authentication Only)
	fmt.Println("2️⃣  Basic TLS - Server Authentication Only")
	fmt.Println()

	tlsConfig := &tls.Config{
		Enabled:  true,
		CertFile: serverCertFile,
		KeyFile:  serverKeyFile,
		CAFile:   caCertFile,
	}

	// Start embedded NATS server with TLS
	fmt.Println("   🚀 Starting NATS server with TLS...")
	srv, err := natsserver.StartEmbeddedServer(
		natsserver.WithPort(14222),
		natsserver.WithTLSConfig(tlsConfig),
	)
	if err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
	defer srv.Shutdown()

	fmt.Printf("   ✅ NATS server running with TLS on %s\n", srv.URL())
	fmt.Println()

	// Connect client with TLS
	fmt.Println("   🔌 Connecting client with TLS...")
	clientTLSConfig := &tls.Config{
		Enabled:  true,
		CAFile:   caCertFile,
		// InsecureSkipVerify: true, // Only for testing! Never in production!
	}

	clientTLS, err := clientTLSConfig.BuildClientTLSConfig()
	if err != nil {
		log.Fatalf("Failed to build client TLS config: %v", err)
	}

	nc, err := nats.Connect(srv.URL(), nats.Secure(clientTLS))
	if err != nil {
		log.Fatalf("Failed to connect with TLS: %v", err)
	}
	defer nc.Close()

	fmt.Println("   ✅ Client connected with TLS")
	fmt.Printf("   🔒 Connection is encrypted: %v\n", nc.TLSRequired())
	fmt.Println()

	// Test basic pub/sub over TLS
	fmt.Println("   📤 Testing encrypted messaging...")

	// Subscribe
	msgCh := make(chan string, 1)
	_, err = nc.Subscribe("test.tls", func(msg *nats.Msg) {
		msgCh <- string(msg.Data)
	})
	if err != nil {
		log.Fatalf("Failed to subscribe: %v", err)
	}

	// Publish
	err = nc.Publish("test.tls", []byte("Hello TLS!"))
	if err != nil {
		log.Fatalf("Failed to publish: %v", err)
	}

	// Wait for message
	select {
	case msg := <-msgCh:
		fmt.Printf("   ✅ Received encrypted message: %s\n", msg)
	case <-time.After(2 * time.Second):
		log.Fatal("Timeout waiting for message")
	}
	fmt.Println()

	// Close connection for next example
	nc.Close()
	srv.Shutdown()

	// Example 3: Mutual TLS (mTLS) - Both Server and Client Authentication
	fmt.Println("3️⃣  Mutual TLS (mTLS) - Server and Client Authentication")
	fmt.Println()

	// Start embedded NATS server with mTLS
	fmt.Println("   🚀 Starting NATS server with mTLS...")
	mtlsSrv, err := natsserver.StartEmbeddedServer(
		natsserver.WithPort(14223),
		natsserver.WithMutualTLS(serverCertFile, serverKeyFile, caCertFile),
	)
	if err != nil {
		log.Fatalf("Failed to start mTLS server: %v", err)
	}
	defer mtlsSrv.Shutdown()

	fmt.Printf("   ✅ NATS server running with mTLS on %s\n", mtlsSrv.URL())
	fmt.Println()

	// Connect client with client certificate
	fmt.Println("   🔌 Connecting client with client certificate...")
	mtlsClientConfig := &tls.Config{
		Enabled:  true,
		CertFile: clientCertFile,
		KeyFile:  clientKeyFile,
		CAFile:   caCertFile,
	}

	mtlsClientTLS, err := mtlsClientConfig.BuildClientTLSConfig()
	if err != nil {
		log.Fatalf("Failed to build mTLS client config: %v", err)
	}

	mtlsNC, err := nats.Connect(mtlsSrv.URL(), nats.Secure(mtlsClientTLS))
	if err != nil {
		log.Fatalf("Failed to connect with mTLS: %v", err)
	}
	defer mtlsNC.Close()

	fmt.Println("   ✅ Client authenticated with certificate")
	fmt.Printf("   🔒 Mutual TLS established: %v\n", mtlsNC.TLSRequired())
	fmt.Println()

	// Test pub/sub over mTLS
	fmt.Println("   📤 Testing mutually authenticated messaging...")

	// Subscribe
	mtlsMsgCh := make(chan string, 1)
	_, err = mtlsNC.Subscribe("test.mtls", func(msg *nats.Msg) {
		mtlsMsgCh <- string(msg.Data)
	})
	if err != nil {
		log.Fatalf("Failed to subscribe: %v", err)
	}

	// Publish
	err = mtlsNC.Publish("test.mtls", []byte("Hello mTLS!"))
	if err != nil {
		log.Fatalf("Failed to publish: %v", err)
	}

	// Wait for message
	select {
	case msg := <-mtlsMsgCh:
		fmt.Printf("   ✅ Received mTLS message: %s\n", msg)
	case <-time.After(2 * time.Second):
		log.Fatal("Timeout waiting for message")
	}
	fmt.Println()

	// Example 4: TLS Configuration Validation
	fmt.Println("4️⃣  TLS Configuration Validation")
	fmt.Println()

	// Production config
	prodConfig := tls.ProductionConfig(serverCertFile, serverKeyFile, caCertFile)
	fmt.Printf("   🏭 Production config is secure: %v\n", prodConfig.IsSecure())
	fmt.Printf("      - Min TLS version: %s\n", tls.GetTLSVersion(prodConfig.MinVersion))
	fmt.Printf("      - Max TLS version: %s\n", tls.GetTLSVersion(prodConfig.MaxVersion))
	fmt.Printf("      - Skip verify: %v (should be false)\n", prodConfig.InsecureSkipVerify)
	fmt.Println()

	// Development config (insecure)
	devConfig := tls.DevelopmentConfig()
	fmt.Printf("   🔧 Development config is secure: %v\n", devConfig.IsSecure())
	fmt.Printf("      - Skip verify: %v (OK for development only!)\n", devConfig.InsecureSkipVerify)
	fmt.Println()

	// Mutual TLS config
	mtlsValidationConfig := tls.MutualTLSConfig(serverCertFile, serverKeyFile, caCertFile)
	fmt.Printf("   🔐 Mutual TLS config:\n")
	fmt.Printf("      - Is mutual TLS: %v\n", mtlsValidationConfig.IsMutualTLS())
	fmt.Printf("      - Is secure: %v\n", mtlsValidationConfig.IsSecure())
	fmt.Println()

	// Example 5: Configuration Types
	fmt.Println("5️⃣  Configuration Helper Functions")
	fmt.Println()

	// Default config
	defaultConfig := tls.DefaultConfig()
	fmt.Println("   📋 Default Config:")
	fmt.Printf("      - Min TLS: %s\n", tls.GetTLSVersion(defaultConfig.MinVersion))
	fmt.Printf("      - Max TLS: %s\n", tls.GetTLSVersion(defaultConfig.MaxVersion))
	fmt.Printf("      - Cipher Suites: %d configured\n", len(defaultConfig.CipherSuites))
	for i, suite := range defaultConfig.CipherSuites {
		if i < 3 { // Show first 3
			fmt.Printf("        • %s\n", tls.GetCipherSuiteName(suite))
		}
	}
	if len(defaultConfig.CipherSuites) > 3 {
		fmt.Printf("        ... and %d more\n", len(defaultConfig.CipherSuites)-3)
	}
	fmt.Println()

	fmt.Println("🎉 TLS/mTLS Example Complete!")
	fmt.Println()
	fmt.Println("Key Takeaways:")
	fmt.Println("   • TLS encrypts all traffic between client and server")
	fmt.Println("   • mTLS provides mutual authentication (both parties verified)")
	fmt.Println("   • Use ProductionConfig for production deployments")
	fmt.Println("   • DevelopmentConfig is INSECURE - only for testing")
	fmt.Println("   • Always use proper CA-signed certificates in production")
	fmt.Println("   • Minimum TLS version should be 1.2 or higher")
	fmt.Println()

	// Security recommendations
	fmt.Println("🔒 Security Recommendations:")
	fmt.Println("   ✓ Use mutual TLS (mTLS) for sensitive services")
	fmt.Println("   ✓ Use CA-signed certificates (not self-signed) in production")
	fmt.Println("   ✓ Regularly rotate certificates")
	fmt.Println("   ✓ Monitor certificate expiration")
	fmt.Println("   ✓ Use strong cipher suites (avoid deprecated ones)")
	fmt.Println("   ✗ NEVER use InsecureSkipVerify in production")
	fmt.Println("   ✗ NEVER use TLS 1.0 or 1.1 (deprecated and insecure)")
	fmt.Println()
}

// generateCACertificate creates a self-signed CA certificate for testing
func generateCACertificate(dir string) (certFile, keyFile string, err error) {
	// Generate private key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate private key: %w", err)
	}

	// Create certificate template
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"EventSourcing Test CA"},
			CommonName:   "Test CA",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour), // 1 year
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	// Create certificate
	certBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return "", "", fmt.Errorf("failed to create certificate: %w", err)
	}

	// Write certificate to file
	certFile = filepath.Join(dir, "ca.crt")
	certOut, err := os.Create(certFile)
	if err != nil {
		return "", "", fmt.Errorf("failed to create cert file: %w", err)
	}
	defer certOut.Close()

	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: certBytes}); err != nil {
		return "", "", fmt.Errorf("failed to encode certificate: %w", err)
	}

	// Write private key to file
	keyFile = filepath.Join(dir, "ca.key")
	keyOut, err := os.Create(keyFile)
	if err != nil {
		return "", "", fmt.Errorf("failed to create key file: %w", err)
	}
	defer keyOut.Close()

	if err := pem.Encode(keyOut, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}); err != nil {
		return "", "", fmt.Errorf("failed to encode private key: %w", err)
	}

	return certFile, keyFile, nil
}

// generateServerCertificate creates a server certificate signed by the CA
func generateServerCertificate(dir, caCertFile, caKeyFile string) (certFile, keyFile string, err error) {
	// Load CA certificate
	caCertPEM, err := os.ReadFile(caCertFile)
	if err != nil {
		return "", "", fmt.Errorf("failed to read CA cert: %w", err)
	}
	caCertBlock, _ := pem.Decode(caCertPEM)
	caCert, err := x509.ParseCertificate(caCertBlock.Bytes)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse CA cert: %w", err)
	}

	// Load CA private key
	caKeyPEM, err := os.ReadFile(caKeyFile)
	if err != nil {
		return "", "", fmt.Errorf("failed to read CA key: %w", err)
	}
	caKeyBlock, _ := pem.Decode(caKeyPEM)
	caKey, err := x509.ParsePKCS1PrivateKey(caKeyBlock.Bytes)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse CA key: %w", err)
	}

	// Generate server private key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate private key: %w", err)
	}

	// Create certificate template
	template := x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject: pkix.Name{
			Organization: []string{"EventSourcing Test"},
			CommonName:   "localhost",
		},
		DNSNames:    []string{"localhost"},
		IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	// Create certificate
	certBytes, err := x509.CreateCertificate(rand.Reader, &template, caCert, &privateKey.PublicKey, caKey)
	if err != nil {
		return "", "", fmt.Errorf("failed to create certificate: %w", err)
	}

	// Write certificate
	certFile = filepath.Join(dir, "server.crt")
	certOut, err := os.Create(certFile)
	if err != nil {
		return "", "", fmt.Errorf("failed to create cert file: %w", err)
	}
	defer certOut.Close()

	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: certBytes}); err != nil {
		return "", "", fmt.Errorf("failed to encode certificate: %w", err)
	}

	// Write private key
	keyFile = filepath.Join(dir, "server.key")
	keyOut, err := os.Create(keyFile)
	if err != nil {
		return "", "", fmt.Errorf("failed to create key file: %w", err)
	}
	defer keyOut.Close()

	if err := pem.Encode(keyOut, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}); err != nil {
		return "", "", fmt.Errorf("failed to encode private key: %w", err)
	}

	return certFile, keyFile, nil
}

// generateClientCertificate creates a client certificate signed by the CA
func generateClientCertificate(dir, caCertFile, caKeyFile string) (certFile, keyFile string, err error) {
	// Load CA certificate
	caCertPEM, err := os.ReadFile(caCertFile)
	if err != nil {
		return "", "", fmt.Errorf("failed to read CA cert: %w", err)
	}
	caCertBlock, _ := pem.Decode(caCertPEM)
	caCert, err := x509.ParseCertificate(caCertBlock.Bytes)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse CA cert: %w", err)
	}

	// Load CA private key
	caKeyPEM, err := os.ReadFile(caKeyFile)
	if err != nil {
		return "", "", fmt.Errorf("failed to read CA key: %w", err)
	}
	caKeyBlock, _ := pem.Decode(caKeyPEM)
	caKey, err := x509.ParsePKCS1PrivateKey(caKeyBlock.Bytes)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse CA key: %w", err)
	}

	// Generate client private key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate private key: %w", err)
	}

	// Create certificate template
	template := x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject: pkix.Name{
			Organization: []string{"EventSourcing Test"},
			CommonName:   "test-client",
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	// Create certificate
	certBytes, err := x509.CreateCertificate(rand.Reader, &template, caCert, &privateKey.PublicKey, caKey)
	if err != nil {
		return "", "", fmt.Errorf("failed to create certificate: %w", err)
	}

	// Write certificate
	certFile = filepath.Join(dir, "client.crt")
	certOut, err := os.Create(certFile)
	if err != nil {
		return "", "", fmt.Errorf("failed to create cert file: %w", err)
	}
	defer certOut.Close()

	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: certBytes}); err != nil {
		return "", "", fmt.Errorf("failed to encode certificate: %w", err)
	}

	// Write private key
	keyFile = filepath.Join(dir, "client.key")
	keyOut, err := os.Create(keyFile)
	if err != nil {
		return "", "", fmt.Errorf("failed to create key file: %w", err)
	}
	defer keyOut.Close()

	if err := pem.Encode(keyOut, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}); err != nil {
		return "", "", fmt.Errorf("failed to encode private key: %w", err)
	}

	return certFile, keyFile, nil
}
