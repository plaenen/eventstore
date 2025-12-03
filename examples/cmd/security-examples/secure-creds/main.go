package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"os"

	"github.com/plaenen/eventstore/pkg/security/credentials"
	"github.com/plaenen/eventstore/pkg/security/encryption"
)

func main() {
	fmt.Println("=== Secure Credential Integration Example ===")
	fmt.Println()
	fmt.Println("This example demonstrates how to integrate the credentials")
	fmt.Println("package (SEC-001) with the encryption package (SEC-103)")
	fmt.Println()

	ctx := context.Background()

	// Example 1: Store Encryption Key in Environment (Development)
	fmt.Println("1️⃣  Development: Store Key in Environment Variable")
	fmt.Println()

	// Generate a master key
	masterKey, err := encryption.GenerateKey(32)
	if err != nil {
		log.Fatalf("Failed to generate key: %v", err)
	}

	// Encode key as base64 for storage
	keyBase64 := base64.StdEncoding.EncodeToString(masterKey)

	// In development, store in environment variable
	os.Setenv("ENCRYPTION_MASTER_KEY", keyBase64)
	fmt.Printf("   🔑 Generated key (base64): %s...\n", keyBase64[:40])
	fmt.Println()

	// Create environment-based credential provider
	envProvider := credentials.NewEnvTokenProvider("ENCRYPTION_MASTER_KEY", 0)

	// Get key from environment
	creds, err := envProvider.GetCredentials(ctx)
	if err != nil {
		log.Fatalf("Failed to get credentials: %v", err)
	}

	// Decode the key
	decodedKey, err := base64.StdEncoding.DecodeString(creds.Token)
	if err != nil {
		log.Fatalf("Failed to decode key: %v", err)
	}

	// Create encryption service with retrieved key
	service, err := encryption.NewService(decodedKey)
	if err != nil {
		log.Fatalf("Failed to create service: %v", err)
	}

	// Test encryption
	testData := "Sensitive customer data"
	encrypted, err := service.EncryptString(testData)
	if err != nil {
		log.Fatalf("Failed to encrypt: %v", err)
	}

	fmt.Printf("   📝 Original: %s\n", testData)
	fmt.Printf("   🔒 Encrypted: %s...\n", encrypted[:50])

	decrypted, err := service.DecryptString(encrypted)
	if err != nil {
		log.Fatalf("Failed to decrypt: %v", err)
	}

	fmt.Printf("   🔓 Decrypted: %s\n", decrypted)
	fmt.Printf("   ✅ Match: %v\n", testData == decrypted)
	fmt.Println()

	// Example 2: Production Pattern (AWS Secrets Manager)
	fmt.Println("2️⃣  Production: AWS Secrets Manager Integration")
	fmt.Println()

	fmt.Println("   Production Setup:")
	fmt.Println("   -----------------")
	fmt.Println()
	fmt.Println("   Step 1: Generate and store encryption key")
	fmt.Println("   ```bash")
	fmt.Println("   # Generate 32-byte key")
	fmt.Println("   openssl rand -base64 32 > master-key.txt")
	fmt.Println()
	fmt.Println("   # Store in AWS Secrets Manager")
	fmt.Println("   aws secretsmanager create-secret \\")
	fmt.Println("     --name prod/encryption/master-key \\")
	fmt.Println("     --secret-string file://master-key.txt")
	fmt.Println()
	fmt.Println("   # Securely delete local copy")
	fmt.Println("   shred -u master-key.txt")
	fmt.Println("   ```")
	fmt.Println()

	fmt.Println("   Step 2: Application retrieves key at startup")
	fmt.Println("   ```go")
	fmt.Println("   // Create credential provider for encryption key")
	fmt.Println("   keyProvider, err := credentials.NewSecretProvider(ctx,")
	fmt.Println("       \"awsparamstore:///prod/encryption/master-key\")")
	fmt.Println()
	fmt.Println("   // Get the encryption key")
	fmt.Println("   creds, err := keyProvider.GetCredentials(ctx)")
	fmt.Println()
	fmt.Println("   // Decode from base64")
	fmt.Println("   key, err := base64.StdEncoding.DecodeString(creds.Token)")
	fmt.Println()
	fmt.Println("   // Create encryption service")
	fmt.Println("   encService, err := encryption.NewService(key)")
	fmt.Println("   ```")
	fmt.Println()

	// Example 3: Key Rotation with Credentials Store
	fmt.Println("3️⃣  Key Rotation with Credential Store")
	fmt.Println()

	fmt.Println("   Production Key Rotation Process:")
	fmt.Println("   --------------------------------")
	fmt.Println()
	fmt.Println("   Step 1: Generate new key")
	fmt.Println("   ```bash")
	fmt.Println("   openssl rand -base64 32 > new-key.txt")
	fmt.Println()
	fmt.Println("   # Store as new version")
	fmt.Println("   aws secretsmanager create-secret \\")
	fmt.Println("     --name prod/encryption/master-key-v2 \\")
	fmt.Println("     --secret-string file://new-key.txt")
	fmt.Println("   ```")
	fmt.Println()

	fmt.Println("   Step 2: Application loads both keys")
	fmt.Println("   ```go")
	fmt.Println("   // Load old key")
	fmt.Println("   oldKeyProvider, _ := credentials.NewSecretProvider(ctx,")
	fmt.Println("       \"awsparamstore:///prod/encryption/master-key\")")
	fmt.Println("   oldCreds, _ := oldKeyProvider.GetCredentials(ctx)")
	fmt.Println("   oldKey, _ := base64.StdEncoding.DecodeString(oldCreds.Token)")
	fmt.Println()
	fmt.Println("   // Load new key")
	fmt.Println("   newKeyProvider, _ := credentials.NewSecretProvider(ctx,")
	fmt.Println("       \"awsparamstore:///prod/encryption/master-key-v2\")")
	fmt.Println("   newCreds, _ := newKeyProvider.GetCredentials(ctx)")
	fmt.Println("   newKey, _ := base64.StdEncoding.DecodeString(newCreds.Token)")
	fmt.Println()
	fmt.Println("   // Create service with old key")
	fmt.Println("   service, _ := encryption.NewService(oldKey)")
	fmt.Println()
	fmt.Println("   // Add new key and set as active")
	fmt.Println("   km := service.KeyManager()")
	fmt.Println("   km.AddKey(\"master-v2\", newKey, true)")
	fmt.Println()
	fmt.Println("   // Old data still decrypts, new data uses new key")
	fmt.Println("   ```")
	fmt.Println()

	// Example 4: Chain Provider for Fallback
	fmt.Println("4️⃣  Chain Provider for Development/Production")
	fmt.Println()

	fmt.Println("   Use ChainProvider for flexible key management:")
	fmt.Println("   ```go")
	fmt.Println("   // Try AWS first, fall back to environment")
	fmt.Println("   provider := credentials.NewChainProvider(")
	fmt.Println("       // Production: AWS Secrets Manager")
	fmt.Println("       credentials.NewSecretProvider(ctx,")
	fmt.Println("           \"awsparamstore:///prod/encryption/master-key\"),")
	fmt.Println()
	fmt.Println("       // Development: Environment variable")
	fmt.Println("       credentials.NewEnvTokenProvider(\"ENCRYPTION_MASTER_KEY\", 0),")
	fmt.Println()
	fmt.Println("       // Last resort: Static (testing only!)")
	fmt.Println("       credentials.NewStaticTokenProvider(\"base64-encoded-test-key\", 0),")
	fmt.Println("   )")
	fmt.Println()
	fmt.Println("   creds, _ := provider.GetCredentials(ctx)")
	fmt.Println("   key, _ := base64.StdEncoding.DecodeString(creds.Token)")
	fmt.Println("   service, _ := encryption.NewService(key)")
	fmt.Println("   ```")
	fmt.Println()

	// Example 5: Multi-Tenant Key Management
	fmt.Println("5️⃣  Multi-Tenant Key Management")
	fmt.Println()

	fmt.Println("   Store per-tenant encryption keys:")
	fmt.Println("   ```go")
	fmt.Println("   // Get tenant-specific key")
	fmt.Println("   tenantID := \"tenant-abc-123\"")
	fmt.Printf("%s\n", "   keyPath := fmt.Sprintf(\"/prod/encryption/tenant/%s/key\", tenantID)")
	fmt.Println()
	fmt.Println("   provider, _ := credentials.NewSecretProvider(ctx,")
	fmt.Println("       \"awsparamstore://\" + keyPath)")
	fmt.Println()
	fmt.Println("   creds, _ := provider.GetCredentials(ctx)")
	fmt.Println("   key, _ := base64.StdEncoding.DecodeString(creds.Token)")
	fmt.Println()
	fmt.Println("   // Create tenant-specific encryption service")
	fmt.Println("   tenantService, _ := encryption.NewService(key)")
	fmt.Println()
	fmt.Println("   // Encrypt tenant data")
	fmt.Println("   encrypted, _ := tenantService.EncryptString(tenantData)")
	fmt.Println("   ```")
	fmt.Println()

	// Example 6: Complete Production Setup
	fmt.Println("6️⃣  Complete Production Setup")
	fmt.Println()

	fmt.Println("   ```go")
	fmt.Println("   package main")
	fmt.Println()
	fmt.Println("   import (")
	fmt.Println("       \"context\"")
	fmt.Println("       \"encoding/base64\"")
	fmt.Println("       \"log\"")
	fmt.Println()
	fmt.Println("       \"github.com/plaenen/eventstore/pkg/security/credentials\"")
	fmt.Println("       \"github.com/plaenen/eventstore/pkg/security/encryption\"")
	fmt.Println("   )")
	fmt.Println()
	fmt.Println("   func main() {")
	fmt.Println("       ctx := context.Background()")
	fmt.Println()
	fmt.Println("       // 1. Load encryption key from AWS Secrets Manager")
	fmt.Println("       keyProvider, err := credentials.NewSecretProvider(ctx,")
	fmt.Println("           \"awsparamstore:///prod/encryption/master-key\")")
	fmt.Println("       if err != nil {")
	fmt.Printf("%s\n", "           log.Fatalf(\"Failed to create key provider: %v\", err)")
	fmt.Println("       }")
	fmt.Println()
	fmt.Println("       creds, err := keyProvider.GetCredentials(ctx)")
	fmt.Println("       if err != nil {")
	fmt.Printf("%s\n", "           log.Fatalf(\"Failed to get encryption key: %v\", err)")
	fmt.Println("       }")
	fmt.Println()
	fmt.Println("       // 2. Decode key from base64")
	fmt.Println("       key, err := base64.StdEncoding.DecodeString(creds.Token)")
	fmt.Println("       if err != nil {")
	fmt.Printf("%s\n", "           log.Fatalf(\"Failed to decode key: %v\", err)")
	fmt.Println("       }")
	fmt.Println()
	fmt.Println("       // 3. Create encryption service")
	fmt.Println("       encService, err := encryption.NewService(key)")
	fmt.Println("       if err != nil {")
	fmt.Printf("%s\n", "           log.Fatalf(\"Failed to create encryption service: %v\", err)")
	fmt.Println("       }")
	fmt.Println()
	fmt.Println("       // 4. Use encryption service")
	fmt.Println("       sensitiveData := \"customer SSN: 123-45-6789\"")
	fmt.Println("       encrypted, err := encService.EncryptString(sensitiveData)")
	fmt.Println("       if err != nil {")
	fmt.Printf("%s\n", "           log.Fatalf(\"Encryption failed: %v\", err)")
	fmt.Println("       }")
	fmt.Println()
	fmt.Println("       // 5. Store encrypted data")
	fmt.Println("       // ...")
	fmt.Println("   }")
	fmt.Println("   ```")
	fmt.Println()

	fmt.Println("🎉 Integration Example Complete!")
	fmt.Println()
	fmt.Println("Key Takeaways:")
	fmt.Println("   • NEVER store encryption keys in source code")
	fmt.Println("   • Use credentials.NewSecretProvider for production")
	fmt.Println("   • Use credentials.NewEnvProvider for development")
	fmt.Println("   • Use credentials.NewChainProvider for flexibility")
	fmt.Println("   • Store keys as base64-encoded strings")
	fmt.Println("   • Rotate keys regularly using key management")
	fmt.Println("   • Use per-tenant keys for multi-tenant systems")
	fmt.Println()

	fmt.Println("🔒 Security Best Practices:")
	fmt.Println("   ✓ Store encryption keys in AWS Secrets Manager/KMS")
	fmt.Println("   ✓ Use IAM roles for key access (no hardcoded credentials)")
	fmt.Println("   ✓ Enable CloudTrail for key access auditing")
	fmt.Println("   ✓ Rotate keys every 90 days")
	fmt.Println("   ✓ Use separate keys per environment")
	fmt.Println("   ✓ Implement least-privilege access")
	fmt.Println("   ✗ NEVER commit keys to version control")
	fmt.Println("   ✗ NEVER log encryption keys")
	fmt.Println("   ✗ NEVER share keys between tenants")
	fmt.Println()

	fmt.Println("📚 References:")
	fmt.Println("   • Credentials Package: pkg/security/credentials/README.md")
	fmt.Println("   • Encryption Package: pkg/security/encryption/README.md")
	fmt.Println("   • AWS Secrets Manager: https://docs.aws.amazon.com/secretsmanager/")
	fmt.Println("   • HashiCorp Vault: https://www.vaultproject.io/")
	fmt.Println()
}
