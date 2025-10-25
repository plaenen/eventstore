package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/plaenen/eventstore/pkg/cqrs"
	natscqrs "github.com/plaenen/eventstore/pkg/cqrs/nats"
	"github.com/plaenen/eventstore/pkg/infrastructure/nats"
	"github.com/plaenen/eventstore/pkg/runner"
	"github.com/plaenen/eventstore/pkg/runtime/embeddednats"
	"github.com/plaenen/eventstore/pkg/security/credentials"
	_ "gocloud.dev/secrets/localsecrets" // Local encryption for development
)

func main() {
	fmt.Println("=== Secure Credentials Example ===")
	fmt.Println()
	fmt.Println("This example demonstrates secure credential management:")
	fmt.Println("  • Local encryption for development (NaCl secretbox)")
	fmt.Println("  • Environment variables for CI/CD")
	fmt.Println("  • Production-ready patterns")
	fmt.Println()

	ctx := context.Background()

	// ===================================================================
	// EXAMPLE 1: Static Token (DEVELOPMENT ONLY)
	// ===================================================================
	fmt.Println("1️⃣  Static Token Provider (Development Only)")
	fmt.Println("   ⚠️  WARNING: Use only for local development!")
	fmt.Println()

	staticProvider := credentials.NewStaticTokenProvider("dev-token-12345", 24*time.Hour)
	defer staticProvider.Close()

	creds, err := staticProvider.GetCredentials(ctx)
	if err != nil {
		log.Fatalf("Failed to get static credentials: %v", err)
	}

	fmt.Printf("   ✅ Got credentials: type=%s\n", creds.Type)
	fmt.Printf("      Metadata: %v\n", creds.Metadata)
	fmt.Println()

	// ===================================================================
	// EXAMPLE 2: Environment Variables (CI/CD)
	// ===================================================================
	fmt.Println("2️⃣  Environment Variable Provider (CI/CD)")
	fmt.Println("   Better than static - credentials injected at runtime")
	fmt.Println()

	// Set example env vars
	os.Setenv("NATS_TOKEN", "ci-token-67890")

	envProvider := credentials.NewEnvTokenProvider("NATS_TOKEN", 5*time.Minute)
	defer envProvider.Close()

	envCreds, err := envProvider.GetCredentials(ctx)
	if err != nil {
		log.Fatalf("Failed to get env credentials: %v", err)
	}

	fmt.Printf("   ✅ Got credentials from ENV: type=%s\n", envCreds.Type)
	fmt.Printf("      Environment variable: NATS_TOKEN\n")
	fmt.Println()

	// ===================================================================
	// EXAMPLE 3: Local Encrypted Secrets (Recommended for Development)
	// ===================================================================
	fmt.Println("3️⃣  Local Encrypted Secrets (Recommended)")
	fmt.Println("   Uses NaCl secretbox for encryption")
	fmt.Println()

	// Create credentials to store
	tokenCreds := &credentials.Credentials{
		Type:  credentials.CredentialTypeToken,
		Token: "super-secret-production-token",
		Metadata: map[string]string{
			"environment": "development",
			"created_at":  time.Now().Format(time.RFC3339),
		},
	}

	// Option A: Random key (new key every run)
	fmt.Println("   📝 Storing credentials with random key...")
	randomKeyURL := "base64key://" // Empty = random key
	if err := credentials.StoreCredentials(ctx, randomKeyURL, tokenCreds); err != nil {
		log.Printf("   Note: Store not fully implemented for random keys, but retrieval works: %v", err)
	}

	// Option B: Fixed key (recommended for dev - store key securely)
	// This key should be stored in your .env file or keychain, NOT in code!
	fixedKey := "smGbjm71Nxd1Ig5FS0wj9SlbzAIrnolCz9bQQ6uAhl4=" // Example base64 key
	fixedKeyURL := fmt.Sprintf("base64key://%s", fixedKey)

	fmt.Printf("   📝 Using fixed key for persistent encryption\n")
	fmt.Printf("      Key: %s... (truncated)\n", fixedKey[:10])
	fmt.Println()

	// Create a secret provider using local encryption
	// Note: For production, you'd use AWS/GCP/Azure URLs here
	localProvider, err := credentials.NewSecretProvider(ctx, fixedKeyURL)
	if err != nil {
		log.Printf("   ⚠️  Note: Full implementation requires secret storage setup\n")
		log.Printf("      For now, using fallback provider\n")
		// Fall back to static for this demo
		localProvider = nil
	} else {
		defer localProvider.Close()

		localCreds, err := localProvider.GetCredentials(ctx)
		if err != nil {
			log.Fatalf("Failed to get local credentials: %v", err)
		}

		fmt.Printf("   ✅ Got encrypted credentials: type=%s\n", localCreds.Type)
		fmt.Println()
	}

	// ===================================================================
	// EXAMPLE 4: Chain Provider (Fallback Pattern)
	// ===================================================================
	fmt.Println("4️⃣  Chain Provider (Production Pattern)")
	fmt.Println("   Try secrets manager → env vars → fail")
	fmt.Println()

	// Create chain: try cloud, fall back to env, finally static
	chainProvider := credentials.NewChainProvider(
		// First choice: environment (would be AWS Secrets Manager in prod)
		credentials.NewEnvTokenProvider("NATS_TOKEN", 5*time.Minute),
		// Fallback: static (for local dev only)
		credentials.NewStaticTokenProvider("fallback-token", 24*time.Hour),
	)
	defer chainProvider.Close()

	chainCreds, err := chainProvider.GetCredentials(ctx)
	if err != nil {
		log.Fatalf("Chain provider failed: %v", err)
	}

	fmt.Printf("   ✅ Got credentials from chain: type=%s\n", chainCreds.Type)
	fmt.Println()

	// ===================================================================
	// EXAMPLE 5: Using Credentials with NATS Transport
	// ===================================================================
	fmt.Println("5️⃣  Using Secure Credentials with NATS")
	fmt.Println()

	// Start embedded NATS server
	fmt.Println("   Starting embedded NATS server...")
	natsService := embeddednats.New(
		embeddednats.WithLogger(runner.NewNoopLogger()),
		embeddednats.WithNATSOptions(
			nats.WithPort(14222), // Different port to avoid conflicts
		),
	)

	runnerInstance := runner.New(
		[]runner.Service{natsService},
		runner.WithLogger(runner.NewNoopLogger()),
	)

	errCh := make(chan error, 1)
	go func() {
		errCh <- runnerInstance.Run(ctx)
	}()

	// Wait for service to be ready
	time.Sleep(1 * time.Second)

	fmt.Println("   ✅ NATS server started")
	fmt.Println()

	// Create NATS transport with secure credentials
	fmt.Println("   Creating NATS transport with credentials...")

	// Use static provider for this local demo (no auth required for embedded)
	provider := credentials.NewStaticTokenProvider("demo-token", 24*time.Hour)
	defer provider.Close()

	transport, err := natscqrs.NewTransport(&natscqrs.TransportConfig{
		TransportConfig: &cqrs.TransportConfig{
			Timeout:              5 * time.Second,
			MaxReconnectAttempts: 3,
			ReconnectWait:        1 * time.Second,
		},
		URL:                natsService.URL(),
		Name:               "secure-credentials-demo",
		CredentialProvider: provider, // ✅ SECURE!
		// Token: "plaintext",  // ❌ DEPRECATED - don't use!
	})
	if err != nil {
		log.Fatalf("Failed to create transport: %v", err)
	}
	defer transport.Close()

	fmt.Println("   ✅ Transport created with secure credentials")
	fmt.Printf("   📡 Connected to: %s\n", transport.ConnectedURL())
	fmt.Println()

	// ===================================================================
	// COMPARISON: Old vs New
	// ===================================================================
	fmt.Println("📊 Migration Guide:")
	fmt.Println()
	fmt.Println("   ❌ OLD (INSECURE):")
	fmt.Println("   transport, _ := nats.NewTransport(&nats.TransportConfig{")
	fmt.Println("       Token: \"my-secret-token\",  // Plaintext!")
	fmt.Println("   })")
	fmt.Println()
	fmt.Println("   ✅ NEW (SECURE):")
	fmt.Println("   provider := credentials.NewEnvTokenProvider(\"NATS_TOKEN\", ttl)")
	fmt.Println("   transport, _ := nats.NewTransport(&nats.TransportConfig{")
	fmt.Println("       CredentialProvider: provider,")
	fmt.Println("   })")
	fmt.Println()

	// ===================================================================
	// PRODUCTION EXAMPLES
	// ===================================================================
	fmt.Println("🚀 Production Examples:")
	fmt.Println()

	fmt.Println("   AWS Secrets Manager:")
	fmt.Println("   provider, _ := credentials.NewSecretProvider(ctx,")
	fmt.Println("       \"awskms://arn:aws:secretsmanager:us-east-1:123:secret:nats-creds\")")
	fmt.Println()

	fmt.Println("   GCP Secret Manager:")
	fmt.Println("   provider, _ := credentials.NewSecretProvider(ctx,")
	fmt.Println("       \"gcpkms://projects/PROJECT/secrets/nats-creds/versions/latest\")")
	fmt.Println()

	fmt.Println("   Azure Key Vault:")
	fmt.Println("   provider, _ := credentials.NewSecretProvider(ctx,")
	fmt.Println("       \"azurekeyvault://vault-name.vault.azure.net/secrets/nats-creds\")")
	fmt.Println()

	fmt.Println("   HashiCorp Vault:")
	fmt.Println("   provider, _ := credentials.NewSecretProvider(ctx,")
	fmt.Println("       \"hashivault://vault.example.com:8200/secret/data/nats\")")
	fmt.Println()

	// ===================================================================
	// KEY BENEFITS
	// ===================================================================
	fmt.Println("✅ Key Benefits:")
	fmt.Println("   • No plaintext credentials in code or config")
	fmt.Println("   • Automatic credential rotation")
	fmt.Println("   • Caching for performance")
	fmt.Println("   • Vendor-agnostic (easy to migrate)")
	fmt.Println("   • Works with all major cloud providers")
	fmt.Println("   • Backward compatible (deprecated fields still work)")
	fmt.Println()

	// ===================================================================
	// BEST PRACTICES
	// ===================================================================
	fmt.Println("📚 Best Practices:")
	fmt.Println("   1. Use environment variables for CI/CD")
	fmt.Println("   2. Use cloud secret managers for production")
	fmt.Println("   3. Use local encryption for development")
	fmt.Println("   4. Never commit credentials to version control")
	fmt.Println("   5. Rotate credentials regularly")
	fmt.Println("   6. Use chain providers for fallback")
	fmt.Println("   7. Set appropriate cache TTLs")
	fmt.Println()

	fmt.Println("🎉 Demo Complete!")
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("   1. Set up environment variables: export NATS_TOKEN=...")
	fmt.Println("   2. For production, configure cloud secret manager")
	fmt.Println("   3. Migrate existing code from Token/Pass to CredentialProvider")
	fmt.Println("   4. See docs/security/IMMEDIATE_ACTIONS.md for details")
	fmt.Println()
}
