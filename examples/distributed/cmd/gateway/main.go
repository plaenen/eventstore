package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/plaenen/eventsourcing/pkg/sdk"
	"github.com/plaenen/eventsourcing/pkg/unifiedsdk"
	accountv1 "github.com/plaenen/eventsourcing/examples/pb/account/v1"
)

// API Gateway - Thin Client (NO DATABASE!)
// This service only sends commands to NATS
// Command handlers run in a separate service with the database

func main() {
	fmt.Println("🌐 API Gateway Starting...")
	fmt.Println()

	// Get configuration from environment
	natsURL := getEnv("NATS_URL", "nats://localhost:4222")
	port := getEnv("PORT", "8080")

	// Create THIN CLIENT - No database!
	client, err := unifiedsdk.New(
		unifiedsdk.WithMode(sdk.ProductionMode),
		unifiedsdk.WithRole(sdk.RoleCommandSender), // ← THIN CLIENT!
		unifiedsdk.WithNATSURL(natsURL),
		// NO WithSQLiteDSN - not needed!
	)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	fmt.Printf("✅ Connected to NATS at %s\n", natsURL)
	fmt.Println("✅ Thin client (no database) - Role: CommandSender")
	fmt.Println()

	// Create HTTP handlers
	handler := &HTTPHandler{
		sdk: client,
	}

	// Register HTTP routes
	http.HandleFunc("/accounts", handler.CreateAccount)
	http.HandleFunc("/accounts/", handler.HandleAccount)

	// Start HTTP server
	fmt.Printf("🚀 HTTP Server listening on :%s\n", port)
	fmt.Println()
	fmt.Println("Example requests:")
	fmt.Println("  POST http://localhost:8080/accounts")
	fmt.Println("  POST http://localhost:8080/accounts/acc-001/deposit")
	fmt.Println("  POST http://localhost:8080/accounts/acc-001/withdraw")
	fmt.Println()

	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

type HTTPHandler struct {
	sdk *unifiedsdk.SDK
}

type CreateAccountRequest struct {
	AccountID      string `json:"account_id"`
	OwnerName      string `json:"owner_name"`
	InitialBalance string `json:"initial_balance"`
}

type DepositRequest struct {
	Amount string `json:"amount"`
}

type WithdrawRequest struct {
	Amount string `json:"amount"`
}

func (h *HTTPHandler) CreateAccount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req CreateAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	ctx := context.Background()

	// Send command via NATS (thin client!)
	fmt.Printf("📤 Sending OpenAccount command for %s\n", req.AccountID)

	err := h.sdk.Account.OpenAccount(ctx, &accountv1.OpenAccountCommand{
		AccountId:      req.AccountID,
		OwnerName:      req.OwnerName,
		InitialBalance: req.InitialBalance,
	}, "api-gateway")

	if err != nil {
		fmt.Printf("❌ Command failed: %v\n", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	fmt.Printf("✅ Command sent successfully\n")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"account_id": req.AccountID,
		"status":     "created",
	})
}

func (h *HTTPHandler) HandleAccount(w http.ResponseWriter, r *http.Request) {
	// Parse URL: /accounts/{id}/{action}
	// Example: /accounts/acc-001/deposit

	path := r.URL.Path[len("/accounts/"):]
	if path == "" {
		http.Error(w, "Account ID required", http.StatusBadRequest)
		return
	}

	// Split into account ID and action
	var accountID, action string
	if idx := len(path); idx > 0 {
		parts := splitPath(path)
		if len(parts) >= 2 {
			accountID = parts[0]
			action = parts[1]
		} else {
			accountID = parts[0]
		}
	}

	ctx := context.Background()

	switch action {
	case "deposit":
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req DepositRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		fmt.Printf("📤 Sending Deposit command for %s\n", accountID)

		err := h.sdk.Account.Deposit(ctx, &accountv1.DepositCommand{
			AccountId: accountID,
			Amount:    req.Amount,
		}, "api-gateway")

		if err != nil {
			fmt.Printf("❌ Command failed: %v\n", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		fmt.Printf("✅ Command sent successfully\n")

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"account_id": accountID,
			"status":     "deposited",
			"amount":     req.Amount,
		})

	case "withdraw":
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req WithdrawRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		fmt.Printf("📤 Sending Withdraw command for %s\n", accountID)

		err := h.sdk.Account.Withdraw(ctx, &accountv1.WithdrawCommand{
			AccountId: accountID,
			Amount:    req.Amount,
		}, "api-gateway")

		if err != nil {
			fmt.Printf("❌ Command failed: %v\n", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		fmt.Printf("✅ Command sent successfully\n")

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"account_id": accountID,
			"status":     "withdrawn",
			"amount":     req.Amount,
		})

	default:
		http.Error(w, "Unknown action", http.StatusNotFound)
	}
}

func splitPath(path string) []string {
	var parts []string
	current := ""
	for _, c := range path {
		if c == '/' {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
