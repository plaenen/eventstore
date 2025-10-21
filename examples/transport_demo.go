package main

import (
	"context"
	"fmt"
	"log"
	"time"

	accountv1 "github.com/plaenen/eventstore/examples/pb/account/v1"
	"github.com/plaenen/eventstore/pkg/eventsourcing"
	natspkg "github.com/plaenen/eventstore/pkg/nats"
	"google.golang.org/protobuf/proto"
)

// This example demonstrates the new transport layer
// Server receives requests via NATS and responds
// Client sends requests via NATS transport

func main() {
	fmt.Println("=== Transport Layer Demo ===")
	fmt.Println()

	// 1. Start Server
	fmt.Println("1️⃣  Starting NATS server...")
	server, err := natspkg.NewServer(&natspkg.ServerConfig{
		ServerConfig: &eventsourcing.ServerConfig{
			QueueGroup:     "demo-handlers",
			MaxConcurrent:  10,
			HandlerTimeout: 5 * time.Second,
		},
		URL:         "nats://localhost:4222",
		Name:        "AccountService",
		Version:     "1.0.0",
		Description: "Demo account service",
	})
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}
	defer server.Close()

	// 2. Register a handler
	fmt.Println("2️⃣  Registering OpenAccount handler...")
	server.RegisterHandler("account.v1.AccountCommandService.OpenAccount",
		func(ctx context.Context, request proto.Message) (*eventsourcing.Response, error) {
			cmd := request.(*accountv1.OpenAccountCommand)
			fmt.Printf("   📥 Server received: OpenAccount for %s (%s)\n", cmd.AccountId, cmd.OwnerName)

			// Simulate business logic
			if cmd.OwnerName == "" {
				return eventsourcing.NewErrorResponse(
					"INVALID_OWNER",
					"Owner name is required",
					"Provide a valid owner name",
					nil,
				), nil
			}

			// Return success response
			response := &accountv1.OpenAccountResponse{
				AccountId: cmd.AccountId,
				Version:   1,
			}

			return eventsourcing.NewSuccessResponse(response)
		})

	// 3. Start server
	fmt.Println("3️⃣  Starting server...")
	if err := server.Start(context.Background()); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
	fmt.Println("   ✅ Server listening on NATS\n")

	// Give server time to start
	time.Sleep(500 * time.Millisecond)

	// 4. Create client transport
	fmt.Println("4️⃣  Creating client transport...")
	transport, err := natspkg.NewTransport(&natspkg.TransportConfig{
		TransportConfig: &eventsourcing.TransportConfig{
			Timeout:              5 * time.Second,
			MaxReconnectAttempts: 3,
			ReconnectWait:        1 * time.Second,
		},
		URL:  "nats://localhost:4222",
		Name: "demo-client",
	})
	if err != nil {
		log.Fatalf("Failed to create transport: %v", err)
	}
	defer transport.Close()
	fmt.Println("   ✅ Transport connected\n")

	// 5. Send request
	fmt.Println("5️⃣  Sending OpenAccount command...")
	ctx := context.Background()

	cmd := &accountv1.OpenAccountCommand{
		AccountId:      "acc-123",
		OwnerName:      "Alice",
		InitialBalance: "1000.00",
	}

	resp, err := transport.Request(ctx, "account.v1.AccountCommandService.OpenAccount", cmd)
	if err != nil {
		log.Fatalf("Request failed: %v", err)
	}

	// 6. Handle response
	fmt.Println("6️⃣  Processing response...")
	if !resp.Success {
		fmt.Printf("   ❌ Error: %s (code: %s)\n", resp.GetError().Message, resp.GetError().Code)
		if resp.GetError().Solution != "" {
			fmt.Printf("   💡 Solution: %s\n", resp.GetError().Solution)
		}
		return
	}

	// Unpack response data
	openResp := &accountv1.OpenAccountResponse{}
	if err := resp.UnpackData(openResp); err != nil {
		log.Fatalf("Failed to unpack response: %v", err)
	}

	fmt.Printf("   ✅ Success! Account %s created (version %d)\n", openResp.AccountId, openResp.Version)
	fmt.Println()

	// 7. Test error case
	fmt.Println("7️⃣  Testing error case (empty owner name)...")
	badCmd := &accountv1.OpenAccountCommand{
		AccountId:      "acc-456",
		OwnerName:      "", // Empty!
		InitialBalance: "500.00",
	}

	resp, err = transport.Request(ctx, "account.v1.AccountCommandService.OpenAccount", badCmd)
	if err != nil {
		log.Fatalf("Request failed: %v", err)
	}

	if !resp.Success {
		fmt.Printf("   ✅ Expected error received:\n")
		fmt.Printf("      Code: %s\n", resp.GetError().Code)
		fmt.Printf("      Message: %s\n", resp.GetError().Message)
		fmt.Printf("      Solution: %s\n", resp.GetError().Solution)
	} else {
		fmt.Println("   ❌ Should have failed but didn't")
	}

	fmt.Println()
	fmt.Println("🎉 Transport layer demo complete!")
}
