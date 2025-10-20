package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/plaenen/eventsourcing/pkg/sdk"
	accountv1 "github.com/plaenen/eventsourcing/examples/pb/account/v1"
)

// This example demonstrates using the GENERATED AccountClient
// instead of manually constructing commands

func main() {
	fmt.Println("=== Generated SDK Client Example ===")
	fmt.Println("Using type-safe AccountClient generated from proto")
	fmt.Println()

	// 1. Create SDK client
	sdkClient, err := sdk.NewBuilder().
		WithMode(sdk.DevelopmentMode).
		WithSQLiteDSN(":memory:").
		Build()
	if err != nil {
		log.Fatalf("Failed to create SDK client: %v", err)
	}
	defer sdkClient.Close()

	// 2. Create the GENERATED AccountClient
	accountClient := accountv1.NewAccountClient(sdkClient)

	// 3. Use type-safe command methods!
	ctx := context.Background()

	fmt.Println("📤 Opening account using generated client...")
	_, err = accountClient.OpenAccount(ctx, &accountv1.OpenAccountCommand{
		AccountId:      "acc-generated-123",
		OwnerName:      "Alice Johnson",
		InitialBalance: "5000.00",
	}, "user-alice")
	if err != nil {
		log.Fatalf("Failed to open account: %v", err)
	}
	fmt.Println("✅ Account opened!")

	// Give time for async processing
	time.Sleep(100 * time.Millisecond)

	fmt.Println("\n📤 Depositing money using generated client...")
	_, err = accountClient.Deposit(ctx, &accountv1.DepositCommand{
		AccountId: "acc-generated-123",
		Amount:    "1500.00",
	}, "user-alice")
	if err != nil {
		log.Fatalf("Failed to deposit: %v", err)
	}
	fmt.Println("✅ Money deposited!")

	time.Sleep(100 * time.Millisecond)

	fmt.Println("\n📤 Withdrawing money using generated client...")
	_, err = accountClient.Withdraw(ctx, &accountv1.WithdrawCommand{
		AccountId: "acc-generated-123",
		Amount:    "500.00",
	}, "user-alice")
	if err != nil {
		log.Fatalf("Failed to withdraw: %v", err)
	}
	fmt.Println("✅ Money withdrawn!")

	time.Sleep(100 * time.Millisecond)

	fmt.Println("\n🎉 All operations completed successfully!")
	fmt.Println("\nKey Benefits:")
	fmt.Println("  ✅ Type-safe API - compiler checks for errors")
	fmt.Println("  ✅ Auto-generated from proto - no manual coding")
	fmt.Println("  ✅ Consistent interface across all aggregates")
	fmt.Println("  ✅ Automatic metadata handling (command ID, correlation, etc.)")
	fmt.Println("  ✅ Works with both Development and Production modes")

	// Query methods are also generated (placeholders for now)
	fmt.Println("\n📊 Query methods are also available:")
	fmt.Println("  - accountClient.GetAccount()")
	fmt.Println("  - accountClient.ListAccounts()")
	fmt.Println("  - accountClient.GetAccountBalance()")
	fmt.Println("  - accountClient.GetAccountHistory()")
}
