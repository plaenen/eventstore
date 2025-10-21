package main

import (
	"context"
	"fmt"
	"log"

	"github.com/plaenen/eventsourcing/pkg/eventsourcing"
	"github.com/plaenen/eventsourcing/pkg/multitenancy"
	accountv1 "github.com/plaenen/eventsourcing/examples/pb/account/v1"
)

// This example demonstrates multi-tenancy patterns in event sourcing

func main() {
	fmt.Println("=== Multi-Tenant Event Sourcing Example ===")
	fmt.Println()

	// Demo both strategies
	demoSharedDatabase()
	fmt.Println()
	demoDatabasePerTenant()
}

func demoSharedDatabase() {
	fmt.Println("📊 Strategy 1: Shared Database with Tenant-Prefixed Aggregates")
	fmt.Println("=" + string(make([]byte, 60)))

	// Create multi-tenant event store (shared database)
	multiStore, err := multitenancy.NewMultiTenantEventStore(multitenancy.MultiTenantConfig{
		Strategy:  multitenancy.SharedDatabase,
		SharedDSN: ":memory:",
		WALMode:   true,
	})
	if err != nil {
		log.Fatalf("Failed to create multi-tenant store: %v", err)
	}
	defer multiStore.Close()

	// Tenant A: Create account
	tenantACtx := multitenancy.WithTenantID(context.Background(), "tenant-a")

	// Compose tenant-scoped aggregate ID
	accountID := multitenancy.ComposeAggregateID("tenant-a", "acc-001")
	fmt.Printf("✅ Tenant A: Creating account with ID: %s\n", accountID)

	// Get store for tenant A (returns shared store)
	storeA, _ := multiStore.GetStore(tenantACtx)
	repoA := accountv1.NewAccountRepository(storeA)

	accountA := accountv1.NewAccount(accountID)

	commandIDA := eventsourcing.GenerateID()
	accountA.SetCommandID(commandIDA)

	err = accountA.OpenAccount(tenantACtx, &accountv1.OpenAccountCommand{
		AccountId:      accountID,
		OwnerName:      "Alice (Tenant A)",
		InitialBalance: "1000.00",
	}, eventsourcing.EventMetadata{
		CausationID:   commandIDA,
		CorrelationID: eventsourcing.GenerateID(),
		PrincipalID:   "user-alice",
		TenantID:      "tenant-a",
	})
	if err != nil {
		log.Fatalf("Failed to open account: %v", err)
	}

	result, err := repoA.SaveWithCommand(accountA, commandIDA)
	if err != nil {
		log.Fatalf("Failed to save: %v", err)
	}
	fmt.Printf("   Version: %d, Events: %d\n", accountA.Version(), len(result.Events))

	// Tenant B: Create account with same local ID
	tenantBCtx := multitenancy.WithTenantID(context.Background(), "tenant-b")

	accountIDB := multitenancy.ComposeAggregateID("tenant-b", "acc-001") // Same local ID!
	fmt.Printf("✅ Tenant B: Creating account with ID: %s\n", accountIDB)

	storeB, _ := multiStore.GetStore(tenantBCtx)
	repoB := accountv1.NewAccountRepository(storeB)

	accountB := accountv1.NewAccount(accountIDB)

	commandIDB := eventsourcing.GenerateID()
	accountB.SetCommandID(commandIDB)

	err = accountB.OpenAccount(tenantBCtx, &accountv1.OpenAccountCommand{
		AccountId:      accountIDB,
		OwnerName:      "Bob (Tenant B)",
		InitialBalance: "2000.00",
	}, eventsourcing.EventMetadata{
		CausationID:   commandIDB,
		CorrelationID: eventsourcing.GenerateID(),
		PrincipalID:   "user-bob",
		TenantID:      "tenant-b",
	})
	if err != nil {
		log.Fatalf("Failed to open account: %v", err)
	}

	result, err = repoB.SaveWithCommand(accountB, commandIDB)
	if err != nil {
		log.Fatalf("Failed to save: %v", err)
	}
	fmt.Printf("   Version: %d, Events: %d\n", accountB.Version(), len(result.Events))

	// Verify isolation: Load accounts
	fmt.Println()
	fmt.Println("🔒 Verifying Tenant Isolation:")

	loadedA, err := repoA.Load(accountID)
	if err != nil {
		log.Fatalf("Failed to load tenant A account: %v", err)
	}
	fmt.Printf("   Tenant A - Account: %s, Owner: %s, Balance: %s\n",
		loadedA.AccountId, loadedA.OwnerName, loadedA.Balance)

	loadedB, err := repoB.Load(accountIDB)
	if err != nil {
		log.Fatalf("Failed to load tenant B account: %v", err)
	}
	fmt.Printf("   Tenant B - Account: %s, Owner: %s, Balance: %s\n",
		loadedB.AccountId, loadedB.OwnerName, loadedB.Balance)

	// Decompose aggregate IDs to show structure
	fmt.Println()
	fmt.Println("🔍 Aggregate ID Structure:")
	tenantID, localID, _ := multitenancy.DecomposeAggregateID(accountID)
	fmt.Printf("   Full ID: %s → Tenant: %s, Local: %s\n", accountID, tenantID, localID)

	tenantID, localID, _ = multitenancy.DecomposeAggregateID(accountIDB)
	fmt.Printf("   Full ID: %s → Tenant: %s, Local: %s\n", accountIDB, tenantID, localID)

	fmt.Println()
	fmt.Println("✨ Benefits of Shared Database:")
	fmt.Println("   • Single database file - simple deployment")
	fmt.Println("   • Easy to query across tenants for analytics")
	fmt.Println("   • Works with all existing features (snapshots, projections)")
	fmt.Println("   • Natural partitioning via tenant-prefixed IDs")
}

func demoDatabasePerTenant() {
	fmt.Println("🗄️  Strategy 2: Database-Per-Tenant Isolation")
	fmt.Println("=" + string(make([]byte, 60)))

	// Create multi-tenant event store (separate databases)
	multiStore, err := multitenancy.NewMultiTenantEventStore(multitenancy.MultiTenantConfig{
		Strategy:             multitenancy.DatabasePerTenant,
		DatabasePathTemplate: "/tmp/tenant_%s.db", // Each tenant gets their own file
		WALMode:              true,
	})
	if err != nil {
		log.Fatalf("Failed to create multi-tenant store: %v", err)
	}
	defer multiStore.Close()

	// Tenant C: Uses local aggregate IDs (no tenant prefix needed!)
	tenantCCtx := multitenancy.WithTenantID(context.Background(), "tenant-c")

	// With separate databases, we can use simple local IDs
	accountIDC := "acc-001" // No tenant prefix!
	fmt.Printf("✅ Tenant C: Creating account with local ID: %s\n", accountIDC)
	fmt.Printf("   Database: /tmp/tenant_tenant-c.db\n")

	storeC, _ := multiStore.GetStore(tenantCCtx)
	repoC := accountv1.NewAccountRepository(storeC)

	accountC := accountv1.NewAccount(accountIDC)

	commandIDC := eventsourcing.GenerateID()
	accountC.SetCommandID(commandIDC)

	err = accountC.OpenAccount(tenantCCtx, &accountv1.OpenAccountCommand{
		AccountId:      accountIDC,
		OwnerName:      "Charlie (Tenant C)",
		InitialBalance: "3000.00",
	}, eventsourcing.EventMetadata{
		CausationID:   commandIDC,
		CorrelationID: eventsourcing.GenerateID(),
		PrincipalID:   "user-charlie",
		TenantID:      "tenant-c",
	})
	if err != nil {
		log.Fatalf("Failed to open account: %v", err)
	}

	result, err := repoC.SaveWithCommand(accountC, commandIDC)
	if err != nil {
		log.Fatalf("Failed to save: %v", err)
	}
	fmt.Printf("   Version: %d, Events: %d\n", accountC.Version(), len(result.Events))

	// Tenant D: Can also use "acc-001" - completely isolated!
	tenantDCtx := multitenancy.WithTenantID(context.Background(), "tenant-d")

	accountIDD := "acc-001" // Same ID as Tenant C, different database!
	fmt.Printf("✅ Tenant D: Creating account with local ID: %s\n", accountIDD)
	fmt.Printf("   Database: /tmp/tenant_tenant-d.db\n")

	storeD, _ := multiStore.GetStore(tenantDCtx)
	repoD := accountv1.NewAccountRepository(storeD)

	accountD := accountv1.NewAccount(accountIDD)

	commandIDD := eventsourcing.GenerateID()
	accountD.SetCommandID(commandIDD)

	err = accountD.OpenAccount(tenantDCtx, &accountv1.OpenAccountCommand{
		AccountId:      accountIDD,
		OwnerName:      "Diana (Tenant D)",
		InitialBalance: "4000.00",
	}, eventsourcing.EventMetadata{
		CausationID:   commandIDD,
		CorrelationID: eventsourcing.GenerateID(),
		PrincipalID:   "user-diana",
		TenantID:      "tenant-d",
	})
	if err != nil {
		log.Fatalf("Failed to open account: %v", err)
	}

	result, err = repoD.SaveWithCommand(accountD, commandIDD)
	if err != nil {
		log.Fatalf("Failed to save: %v", err)
	}
	fmt.Printf("   Version: %d, Events: %d\n", accountD.Version(), len(result.Events))

	// Verify isolation
	fmt.Println()
	fmt.Println("🔒 Verifying Database Isolation:")

	loadedC, err := repoC.Load(accountIDC)
	if err != nil {
		log.Fatalf("Failed to load tenant C account: %v", err)
	}
	fmt.Printf("   Tenant C (DB: tenant_c) - Owner: %s, Balance: %s\n",
		loadedC.OwnerName, loadedC.Balance)

	loadedD, err := repoD.Load(accountIDD)
	if err != nil {
		log.Fatalf("Failed to load tenant D account: %v", err)
	}
	fmt.Printf("   Tenant D (DB: tenant_d) - Owner: %s, Balance: %s\n",
		loadedD.OwnerName, loadedD.Balance)

	fmt.Println()
	fmt.Println("✨ Benefits of Database-Per-Tenant:")
	fmt.Println("   • Complete data isolation - separate files")
	fmt.Println("   • Easy to backup/restore single tenant")
	fmt.Println("   • Easy to delete tenant data (just remove file)")
	fmt.Println("   • Simple aggregate IDs (no tenant prefix needed)")
	fmt.Println("   • Natural security boundary")
}
