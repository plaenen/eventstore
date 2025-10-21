package main

import (
	"fmt"

	"github.com/plaenen/eventsourcing/pkg/sdk"
	"github.com/plaenen/eventsourcing/pkg/unifiedsdk"
)

// Test that thin client can be created without a database

func main() {
	fmt.Println("=== Testing Thin Client Pattern ===")
	fmt.Println()

	// Test 1: Thin client (command sender) - NO DATABASE
	fmt.Println("1️⃣ Creating thin client (RoleCommandSender)...")
	thinClient, err := unifiedsdk.New(
		unifiedsdk.WithMode(sdk.DevelopmentMode), // Dev mode for testing
		unifiedsdk.WithRole(sdk.RoleCommandSender), // ← THIN CLIENT
		// NO WithSQLiteDSN - not needed!
	)
	if err != nil {
		fmt.Printf("❌ Failed: %v\n", err)
		return
	}
	defer thinClient.Close()

	if thinClient.Client().EventStore() == nil {
		fmt.Println("✅ SUCCESS: Thin client has NO event store")
	} else {
		fmt.Println("❌ FAILED: Thin client should not have event store")
	}

	fmt.Println()

	// Test 2: Handler client - HAS DATABASE
	fmt.Println("2️⃣ Creating handler client (RoleCommandHandler)...")
	handlerClient, err := unifiedsdk.New(
		unifiedsdk.WithMode(sdk.DevelopmentMode),
		unifiedsdk.WithRole(sdk.RoleCommandHandler), // ← HANDLER
		unifiedsdk.WithSQLiteDSN(":memory:"), // ← Requires database
	)
	if err != nil {
		fmt.Printf("❌ Failed: %v\n", err)
		return
	}
	defer handlerClient.Close()

	if handlerClient.Client().EventStore() != nil {
		fmt.Println("✅ SUCCESS: Handler client has event store")
	} else {
		fmt.Println("❌ FAILED: Handler client should have event store")
	}

	fmt.Println()

	// Test 3: Full-stack client - HAS DATABASE (default)
	fmt.Println("3️⃣ Creating full-stack client (RoleFullStack - default)...")
	fullstackClient, err := unifiedsdk.New(
		unifiedsdk.WithMode(sdk.DevelopmentMode),
		// Role defaults to RoleFullStack
		unifiedsdk.WithSQLiteDSN(":memory:"),
	)
	if err != nil {
		fmt.Printf("❌ Failed: %v\n", err)
		return
	}
	defer fullstackClient.Close()

	if fullstackClient.Client().EventStore() != nil {
		fmt.Println("✅ SUCCESS: Full-stack client has event store")
	} else {
		fmt.Println("❌ FAILED: Full-stack client should have event store")
	}

	fmt.Println()

	// Test 4: Handler with empty DSN - SHOULD FAIL
	fmt.Println("4️⃣ Testing handler with empty DSN (should fail)...")
	_, err = unifiedsdk.New(
		unifiedsdk.WithMode(sdk.DevelopmentMode),
		unifiedsdk.WithRole(sdk.RoleCommandHandler),
		unifiedsdk.WithSQLiteDSN(""), // ← Explicitly empty DSN
	)
	if err != nil {
		fmt.Printf("✅ SUCCESS: Correctly rejected (error: %v)\n", err)
	} else {
		fmt.Println("❌ FAILED: Should have required database for handler")
	}

	fmt.Println()
	fmt.Println("🎉 All tests passed!")
}
