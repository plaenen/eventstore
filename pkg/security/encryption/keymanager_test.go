package encryption

import (
	"strings"
	"testing"
)

func TestKeyManager_AddKey(t *testing.T) {
	km := NewKeyManager(DefaultConfig())

	key, _ := GenerateKey(32)

	err := km.AddKey("test-key", key, true)
	if err != nil {
		t.Fatalf("AddKey() error = %v", err)
	}

	// Verify key was added
	retrievedKey, err := km.GetKey("test-key")
	if err != nil {
		t.Fatalf("GetKey() error = %v", err)
	}

	if retrievedKey.Info.ID != "test-key" {
		t.Errorf("Key ID = %s, want test-key", retrievedKey.Info.ID)
	}

	if !retrievedKey.Info.Active {
		t.Error("Key should be active")
	}
}

func TestKeyManager_GetActiveKey(t *testing.T) {
	km := NewKeyManager(DefaultConfig())

	// Should error when no keys
	_, err := km.GetActiveKey()
	if err != ErrNoActiveKey {
		t.Errorf("GetActiveKey() error = %v, want %v", err, ErrNoActiveKey)
	}

	// Add a key
	key, _ := GenerateKey(32)
	_ = km.AddKey("key1", key, true)

	// Should return the active key
	activeKey, err := km.GetActiveKey()
	if err != nil {
		t.Fatalf("GetActiveKey() error = %v", err)
	}

	if activeKey.Info.ID != "key1" {
		t.Errorf("Active key ID = %s, want key1", activeKey.Info.ID)
	}
}

func TestKeyManager_SetActiveKey(t *testing.T) {
	km := NewKeyManager(DefaultConfig())

	// Add two keys
	key1, _ := GenerateKey(32)
	key2, _ := GenerateKey(32)

	_ = km.AddKey("key1", key1, true)
	_ = km.AddKey("key2", key2, false)

	// Set key2 as active
	err := km.SetActiveKey("key2")
	if err != nil {
		t.Fatalf("SetActiveKey() error = %v", err)
	}

	// Verify key2 is now active
	activeKey, _ := km.GetActiveKey()
	if activeKey.Info.ID != "key2" {
		t.Errorf("Active key ID = %s, want key2", activeKey.Info.ID)
	}

	// Verify key1 is no longer active
	key1Info, _ := km.GetKey("key1")
	if key1Info.Info.Active {
		t.Error("key1 should not be active")
	}
}

func TestKeyManager_RotateKey(t *testing.T) {
	km := NewKeyManager(DefaultConfig())

	// Add initial key
	key1, _ := GenerateKey(32)
	_ = km.AddKey("key1", key1, true)

	// Get initial active key
	activeKey1, _ := km.GetActiveKey()
	initialVersion := activeKey1.Info.Version

	// Rotate
	newKeyID, err := km.RotateKey()
	if err != nil {
		t.Fatalf("RotateKey() error = %v", err)
	}

	// Verify new key is active
	activeKey2, _ := km.GetActiveKey()
	if activeKey2.Info.ID != newKeyID {
		t.Errorf("Active key ID = %s, want %s", activeKey2.Info.ID, newKeyID)
	}

	// Verify version incremented
	if activeKey2.Info.Version <= initialVersion {
		t.Errorf("Version = %d, want > %d", activeKey2.Info.Version, initialVersion)
	}

	// Verify old key still exists but is not active
	oldKey, _ := km.GetKey("key1")
	if oldKey.Info.Active {
		t.Error("Old key should not be active after rotation")
	}
}

func TestKeyManager_ListKeys(t *testing.T) {
	km := NewKeyManager(DefaultConfig())

	// Add multiple keys
	for i := 1; i <= 3; i++ {
		key, _ := GenerateKey(32)
		_ = km.AddKey(string(rune('a'+i)), key, i == 1)
	}

	// List keys
	keys := km.ListKeys()

	if len(keys) != 3 {
		t.Errorf("ListKeys() returned %d keys, want 3", len(keys))
	}
}

func TestKeyManager_RemoveKey(t *testing.T) {
	km := NewKeyManager(DefaultConfig())

	// Add keys
	key1, _ := GenerateKey(32)
	key2, _ := GenerateKey(32)
	_ = km.AddKey("key1", key1, true)
	_ = km.AddKey("key2", key2, false)

	// Try to remove active key (should fail)
	err := km.RemoveKey("key1")
	if err == nil {
		t.Error("RemoveKey() should fail for active key")
	}

	// Remove inactive key
	err = km.RemoveKey("key2")
	if err != nil {
		t.Errorf("RemoveKey() error = %v", err)
	}

	// Verify key was removed
	_, err = km.GetKey("key2")
	if err != ErrKeyNotFound {
		t.Errorf("GetKey() error = %v, want %v", err, ErrKeyNotFound)
	}
}

func TestKeyManager_ExportKeys(t *testing.T) {
	km := NewKeyManager(DefaultConfig())

	// Add a key
	key, _ := GenerateKey(32)
	_ = km.AddKey("test-key", key, true)

	// Export
	exported, err := km.ExportKeys()
	if err != nil {
		t.Fatalf("ExportKeys() error = %v", err)
	}

	// Verify it's valid JSON containing key info
	if !strings.Contains(exported, "test-key") {
		t.Error("Exported keys should contain key ID")
	}

	if !strings.Contains(exported, "AES-256-GCM") {
		t.Error("Exported keys should contain algorithm")
	}
}

func TestService_EncryptDecrypt(t *testing.T) {
	masterKey, _ := GenerateKey(32)
	service, err := NewService(masterKey)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	plaintext := "sensitive data"

	// Encrypt
	ciphertext, err := service.EncryptString(plaintext)
	if err != nil {
		t.Fatalf("EncryptString() error = %v", err)
	}

	// Verify ciphertext contains key ID
	if !strings.Contains(ciphertext, ":") {
		t.Error("Ciphertext should contain key ID")
	}

	// Decrypt
	decrypted, err := service.DecryptString(ciphertext)
	if err != nil {
		t.Fatalf("DecryptString() error = %v", err)
	}

	if decrypted != plaintext {
		t.Errorf("Decrypted = %s, want %s", decrypted, plaintext)
	}
}

func TestService_WithPassword(t *testing.T) {
	password := "my-secure-password"
	salt, _ := GenerateSalt()

	service, err := NewServiceWithPassword(password, salt)
	if err != nil {
		t.Fatalf("NewServiceWithPassword() error = %v", err)
	}

	// Test encryption/decryption
	plaintext := []byte("test data")
	ciphertext, err := service.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}

	decrypted, err := service.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}

	if string(decrypted) != string(plaintext) {
		t.Error("Decrypted doesn't match plaintext")
	}
}

func TestService_KeyRotation(t *testing.T) {
	masterKey, _ := GenerateKey(32)
	service, err := NewService(masterKey)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	// Encrypt with original key
	plaintext := "data before rotation"
	ciphertext1, err := service.EncryptString(plaintext)
	if err != nil {
		t.Fatalf("EncryptString() error = %v", err)
	}

	// Rotate key
	newKeyID, err := service.RotateKey()
	if err != nil {
		t.Fatalf("RotateKey() error = %v", err)
	}

	// Encrypt with new key
	ciphertext2, err := service.EncryptString(plaintext)
	if err != nil {
		t.Fatalf("EncryptString() error = %v", err)
	}

	// Verify ciphertexts are different (different keys)
	if ciphertext1 == ciphertext2 {
		t.Error("Ciphertexts should be different after rotation")
	}

	// Verify new ciphertext uses new key
	if !strings.Contains(ciphertext2, newKeyID) {
		t.Error("New ciphertext should use new key ID")
	}

	// Verify old ciphertext can still be decrypted
	decrypted1, err := service.DecryptString(ciphertext1)
	if err != nil {
		t.Fatalf("DecryptString() old ciphertext error = %v", err)
	}

	if decrypted1 != plaintext {
		t.Error("Old ciphertext should still decrypt correctly")
	}

	// Verify new ciphertext can be decrypted
	decrypted2, err := service.DecryptString(ciphertext2)
	if err != nil {
		t.Fatalf("DecryptString() new ciphertext error = %v", err)
	}

	if decrypted2 != plaintext {
		t.Error("New ciphertext should decrypt correctly")
	}
}

func TestService_ReEncrypt(t *testing.T) {
	masterKey, _ := GenerateKey(32)
	service, err := NewService(masterKey)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	plaintext := "data to re-encrypt"

	// Encrypt with original key
	oldCiphertext, err := service.EncryptString(plaintext)
	if err != nil {
		t.Fatalf("EncryptString() error = %v", err)
	}

	// Rotate key
	newKeyID, err := service.RotateKey()
	if err != nil {
		t.Fatalf("RotateKey() error = %v", err)
	}

	// Re-encrypt
	newCiphertext, err := service.ReEncrypt(oldCiphertext)
	if err != nil {
		t.Fatalf("ReEncrypt() error = %v", err)
	}

	// Verify new ciphertext uses new key
	if !strings.Contains(newCiphertext, newKeyID) {
		t.Error("Re-encrypted data should use new key")
	}

	// Verify decryption still works
	decrypted, err := service.DecryptString(newCiphertext)
	if err != nil {
		t.Fatalf("DecryptString() error = %v", err)
	}

	if decrypted != plaintext {
		t.Errorf("Decrypted = %s, want %s", decrypted, plaintext)
	}
}

func TestService_DecryptWithMissingKey(t *testing.T) {
	masterKey, _ := GenerateKey(32)
	service, _ := NewService(masterKey)

	// Create ciphertext with non-existent key ID
	invalidCiphertext := "nonexistent-key:VGVzdCBkYXRh"

	// Attempt to decrypt
	_, err := service.DecryptString(invalidCiphertext)
	if err == nil {
		t.Error("DecryptString() should fail with non-existent key")
	}

	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Error should mention key not found, got: %v", err)
	}
}

func BenchmarkService_Encrypt(b *testing.B) {
	masterKey, _ := GenerateKey(32)
	service, _ := NewService(masterKey)
	plaintext := []byte("benchmark data for encryption performance testing")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = service.Encrypt(plaintext)
	}
}

func BenchmarkService_Decrypt(b *testing.B) {
	masterKey, _ := GenerateKey(32)
	service, _ := NewService(masterKey)
	plaintext := []byte("benchmark data for decryption performance testing")
	ciphertext, _ := service.Encrypt(plaintext)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = service.Decrypt(ciphertext)
	}
}

func BenchmarkService_RotateKey(b *testing.B) {
	masterKey, _ := GenerateKey(32)
	service, _ := NewService(masterKey)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = service.RotateKey()
	}
}
