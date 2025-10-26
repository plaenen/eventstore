package main

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/plaenen/eventstore/pkg/domain"
	"github.com/plaenen/eventstore/pkg/security/encryption"
)

func main() {
	fmt.Println("=== Encryption at Rest Example ===")
	fmt.Println()

	// Example 1: Basic Encryption/Decryption
	fmt.Println("1️⃣  Basic Encryption/Decryption")
	fmt.Println()

	// Generate a master key
	masterKey, err := encryption.GenerateKey(32)
	if err != nil {
		log.Fatalf("Failed to generate key: %v", err)
	}

	// Create encryption service
	service, err := encryption.NewService(masterKey)
	if err != nil {
		log.Fatalf("Failed to create service: %v", err)
	}

	// Encrypt data
	plaintext := "Sensitive customer data"
	ciphertext, err := service.EncryptString(plaintext)
	if err != nil {
		log.Fatalf("Failed to encrypt: %v", err)
	}

	fmt.Printf("   📝 Plaintext: %s\n", plaintext)
	fmt.Printf("   🔒 Ciphertext: %s\n", ciphertext)
	fmt.Println()

	// Decrypt data
	decrypted, err := service.DecryptString(ciphertext)
	if err != nil {
		log.Fatalf("Failed to decrypt: %v", err)
	}

	fmt.Printf("   🔓 Decrypted: %s\n", decrypted)
	fmt.Printf("   ✅ Match: %v\n", plaintext == decrypted)
	fmt.Println()

	// Example 2: Password-Based Encryption
	fmt.Println("2️⃣  Password-Based Encryption (Key Derivation)")
	fmt.Println()

	// Generate salt for key derivation
	salt, err := encryption.GenerateSalt()
	if err != nil {
		log.Fatalf("Failed to generate salt: %v", err)
	}

	// Create service with password
	password := "my-secure-password-123"
	pwdService, err := encryption.NewServiceWithPassword(password, salt)
	if err != nil {
		log.Fatalf("Failed to create password-based service: %v", err)
	}

	// Encrypt with derived key
	secretData := "This data is protected by a password"
	encrypted, err := pwdService.EncryptString(secretData)
	if err != nil {
		log.Fatalf("Failed to encrypt: %v", err)
	}

	fmt.Printf("   🔑 Password: %s\n", password)
	fmt.Printf("   🧂 Salt (hex): %x\n", salt[:8])
	fmt.Printf("   🔒 Encrypted: %s...\n", encrypted[:50])
	fmt.Println()

	// Decrypt with same password
	decrypted2, err := pwdService.DecryptString(encrypted)
	if err != nil {
		log.Fatalf("Failed to decrypt: %v", err)
	}

	fmt.Printf("   🔓 Decrypted: %s\n", decrypted2)
	fmt.Printf("   ✅ Match: %v\n", secretData == decrypted2)
	fmt.Println()

	// Example 3: Key Rotation
	fmt.Println("3️⃣  Key Rotation")
	fmt.Println()

	// Encrypt with original key
	data1 := "Data encrypted with original key"
	cipher1, err := service.EncryptString(data1)
	if err != nil {
		log.Fatalf("Failed to encrypt: %v", err)
	}

	fmt.Printf("   📝 Original data: %s\n", data1)
	fmt.Printf("   🔒 Ciphertext 1: %s...\n", cipher1[:50])
	fmt.Println()

	// Rotate the key
	newKeyID, err := service.RotateKey()
	if err != nil {
		log.Fatalf("Failed to rotate key: %v", err)
	}

	fmt.Printf("   🔄 Rotated to new key: %s\n", newKeyID)
	fmt.Println()

	// Encrypt with new key
	data2 := "Data encrypted with new key"
	cipher2, err := service.EncryptString(data2)
	if err != nil {
		log.Fatalf("Failed to encrypt: %v", err)
	}

	fmt.Printf("   📝 New data: %s\n", data2)
	fmt.Printf("   🔒 Ciphertext 2: %s...\n", cipher2[:50])
	fmt.Println()

	// Verify old data can still be decrypted
	decryptedOld, err := service.DecryptString(cipher1)
	if err != nil {
		log.Fatalf("Failed to decrypt old data: %v", err)
	}

	fmt.Printf("   ✅ Old data still decrypts: %v\n", decryptedOld == data1)

	// Verify new data can be decrypted
	decryptedNew, err := service.DecryptString(cipher2)
	if err != nil {
		log.Fatalf("Failed to decrypt new data: %v", err)
	}

	fmt.Printf("   ✅ New data decrypts: %v\n", decryptedNew == data2)
	fmt.Println()

	// Re-encrypt old data with new key
	reEncrypted, err := service.ReEncrypt(cipher1)
	if err != nil {
		log.Fatalf("Failed to re-encrypt: %v", err)
	}

	fmt.Printf("   🔄 Re-encrypted old data with new key\n")
	fmt.Printf("   🔒 New ciphertext: %s...\n", reEncrypted[:50])
	fmt.Println()

	// Example 4: Event Encryption (Full)
	fmt.Println("4️⃣  Event Encryption - Full Data Encryption")
	fmt.Println()

	// Create event encryptor
	eventEncryptor := encryption.NewEventEncryptor(service, &encryption.EventEncryptionConfig{
		EncryptData:     true,
		FieldEncryption: false, // Full encryption
	})

	// Create a sample event
	eventData := map[string]interface{}{
		"account_id":      "ACC-12345",
		"amount":          1000.50,
		"transaction_id":  "TXN-98765",
		"customer_name":   "John Doe",
		"ssn":             "123-45-6789", // Sensitive!
		"credit_card":     "4532-1111-2222-3333", // Sensitive!
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
		Metadata: domain.EventMetadata{
			PrincipalID: "user-456",
		},
	}

	fmt.Printf("   📝 Original event data:\n")
	fmt.Printf("      %s\n", string(eventDataBytes))
	fmt.Println()

	// Encrypt event
	encryptedEvent, err := eventEncryptor.EncryptEvent(event)
	if err != nil {
		log.Fatalf("Failed to encrypt event: %v", err)
	}

	fmt.Printf("   🔒 Encrypted event data:\n")
	fmt.Printf("      %s...\n", string(encryptedEvent.Data[:min(100, len(encryptedEvent.Data))]))
	fmt.Println()

	// Decrypt event
	decryptedEvent, err := eventEncryptor.DecryptEvent(encryptedEvent)
	if err != nil {
		log.Fatalf("Failed to decrypt event: %v", err)
	}

	fmt.Printf("   🔓 Decrypted event data:\n")
	fmt.Printf("      %s\n", string(decryptedEvent.Data))
	fmt.Printf("   ✅ Match: %v\n", string(event.Data) == string(decryptedEvent.Data))
	fmt.Println()

	// Example 5: Event Encryption (Field-Level)
	fmt.Println("5️⃣  Event Encryption - Field-Level Encryption")
	fmt.Println()

	// Create field-level encryptor
	fieldEncryptor := encryption.NewEventEncryptor(service, &encryption.EventEncryptionConfig{
		EncryptData:     true,
		FieldEncryption: true,
		EncryptedFields: []string{"ssn", "credit_card"}, // Only encrypt sensitive fields
	})

	// Create another event
	event2Data := map[string]interface{}{
		"account_id":     "ACC-67890",
		"amount":         2500.75,
		"customer_name":  "Jane Smith",
		"email":          "jane@example.com", // Not encrypted
		"ssn":            "987-65-4321", // Encrypted
		"credit_card":    "5432-6666-7777-8888", // Encrypted
	}

	event2DataBytes, _ := json.Marshal(event2Data)

	event2 := &domain.Event{
		ID:            "evt-002",
		AggregateID:   "account-456",
		AggregateType: "Account",
		EventType:     "AccountCredited",
		Version:       1,
		Timestamp:     time.Now(),
		Data:          event2DataBytes,
		Metadata: domain.EventMetadata{
			PrincipalID: "user-789",
		},
	}

	fmt.Printf("   📝 Original event (field-level):\n")
	fmt.Printf("      %s\n", string(event2DataBytes))
	fmt.Println()

	// Encrypt event (only specified fields)
	fieldEncryptedEvent, err := fieldEncryptor.EncryptEvent(event2)
	if err != nil {
		log.Fatalf("Failed to encrypt event: %v", err)
	}

	fmt.Printf("   🔒 Field-encrypted event:\n")
	// Parse to show which fields are encrypted
	var encryptedObj map[string]interface{}
	json.Unmarshal(fieldEncryptedEvent.Data, &encryptedObj)
	encBytes, _ := json.MarshalIndent(encryptedObj, "      ", "  ")
	fmt.Printf("      %s\n", string(encBytes))
	fmt.Println()

	fmt.Printf("   ℹ️  Notice: Only 'ssn' and 'credit_card' are encrypted\n")
	fmt.Printf("   ℹ️  Other fields like 'email' and 'customer_name' remain searchable\n")
	fmt.Println()

	// Decrypt event
	fieldDecryptedEvent, err := fieldEncryptor.DecryptEvent(fieldEncryptedEvent)
	if err != nil {
		log.Fatalf("Failed to decrypt event: %v", err)
	}

	fmt.Printf("   🔓 Decrypted event:\n")
	fmt.Printf("      %s\n", string(fieldDecryptedEvent.Data))
	fmt.Println()

	// Example 6: Key Management
	fmt.Println("6️⃣  Key Management")
	fmt.Println()

	km := service.KeyManager()

	// List all keys
	keys := km.ListKeys()
	fmt.Printf("   📋 Total keys: %d\n", len(keys))
	for _, key := range keys {
		activeStatus := ""
		if key.Active {
			activeStatus = " (ACTIVE)"
		}
		fmt.Printf("      • %s - Version %d - %s%s\n", key.ID, key.Version, key.Algorithm, activeStatus)
	}
	fmt.Println()

	// Export key metadata
	exported, err := km.ExportKeys()
	if err != nil {
		log.Fatalf("Failed to export keys: %v", err)
	}

	fmt.Printf("   📤 Exported key metadata:\n")
	fmt.Printf("      %s\n", exported)
	fmt.Println()

	// Example 7: Performance Comparison
	fmt.Println("7️⃣  Performance Comparison")
	fmt.Println()

	// Default config (Argon2)
	defaultConfig := encryption.DefaultConfig()
	fmt.Printf("   🐌 Default Config (High Security):\n")
	fmt.Printf("      Algorithm: %s\n", defaultConfig.Algorithm)
	fmt.Printf("      Key Derivation: %s\n", defaultConfig.KeyDerivation)
	fmt.Printf("      Argon2 Memory: %d KiB\n", defaultConfig.Argon2Memory)
	fmt.Printf("      Argon2 Iterations: %d\n", defaultConfig.Argon2Time)
	fmt.Println()

	// Fast config (for development)
	fastConfig := encryption.FastConfig()
	fmt.Printf("   🚀 Fast Config (Development Only):\n")
	fmt.Printf("      Algorithm: %s\n", fastConfig.Algorithm)
	fmt.Printf("      Key Derivation: %s\n", fastConfig.KeyDerivation)
	fmt.Printf("      Argon2 Memory: %d KiB\n", fastConfig.Argon2Memory)
	fmt.Printf("      Argon2 Iterations: %d\n", fastConfig.Argon2Time)
	fmt.Println()

	fmt.Println("🎉 Encryption at Rest Example Complete!")
	fmt.Println()
	fmt.Println("Key Takeaways:")
	fmt.Println("   • Data encrypted at rest with AES-256-GCM")
	fmt.Println("   • Key rotation supported without data loss")
	fmt.Println("   • Field-level encryption for partial encryption")
	fmt.Println("   • Password-based key derivation with Argon2/PBKDF2")
	fmt.Println("   • Full encryption for maximum security")
	fmt.Println("   • Key management with version tracking")
	fmt.Println()

	fmt.Println("🔒 Security Recommendations:")
	fmt.Println("   ✓ Use Argon2id for password-based encryption")
	fmt.Println("   ✓ Rotate keys regularly")
	fmt.Println("   ✓ Use field-level encryption for searchable data")
	fmt.Println("   ✓ Store master keys securely (AWS KMS, HashiCorp Vault)")
	fmt.Println("   ✓ Use full data encryption for highly sensitive data")
	fmt.Println("   ✗ NEVER store encryption keys in plaintext")
	fmt.Println("   ✗ NEVER commit encryption keys to version control")
	fmt.Println()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
