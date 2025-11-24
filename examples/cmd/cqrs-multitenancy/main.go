package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats-server/v2/server"
	ordersv1 "github.com/plaenen/eventstore/examples/pb/orders/v1"
	"github.com/plaenen/eventstore/pkg/cqrs"
	cqrsnats "github.com/plaenen/eventstore/pkg/cqrs/nats"
	"google.golang.org/protobuf/proto"
)

// =============================================================================
// CQRS-Only Multi-Tenant Example
// =============================================================================
// This example demonstrates:
// 1. Pure CQRS without event sourcing
// 2. Multi-tenant deployments with dynamic subject routing
// 3. Custom SubjectBuilder that extracts tenant ID from context
// 4. Simple in-memory order store (no persistence)
//
// Architecture:
//   Client (Tenant A) --> NATS subjects: "tenant-a.orders.v1.OrderCommandService.CreateOrder"
//   Client (Tenant B) --> NATS subjects: "tenant-b.orders.v1.OrderCommandService.CreateOrder"
//   Server            --> Subscribes to:  "*.orders.v1.OrderCommandService.*"
// =============================================================================

// contextKey for storing tenant ID
type contextKey string

const tenantIDKey contextKey = "tenant_id"

// WithTenantID adds a tenant ID to the context
func WithTenantID(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, tenantIDKey, tenantID)
}

// GetTenantID extracts tenant ID from context
func GetTenantID(ctx context.Context) string {
	if tid, ok := ctx.Value(tenantIDKey).(string); ok {
		return tid
	}
	return "default"
}

// Order store - simple in-memory storage (per-tenant)
type OrderStore struct {
	mu     sync.RWMutex
	orders map[string]map[string]*ordersv1.Order // tenant -> orderID -> Order
}

func NewOrderStore() *OrderStore {
	return &OrderStore{
		orders: make(map[string]map[string]*ordersv1.Order),
	}
}

func (s *OrderStore) CreateOrder(ctx context.Context, tenantID, customerID string, items []*ordersv1.OrderItem, address string) (*ordersv1.Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Initialize tenant storage if needed
	if _, exists := s.orders[tenantID]; !exists {
		s.orders[tenantID] = make(map[string]*ordersv1.Order)
	}

	// Calculate total
	var total float64
	for _, item := range items {
		// Simple price calculation
		total += float64(item.Quantity) * 10.0
	}

	order := &ordersv1.Order{
		OrderId:         uuid.New().String(),
		CustomerId:      customerID,
		Items:           items,
		Status:          ordersv1.OrderStatus_ORDER_STATUS_PENDING,
		TotalAmount:     fmt.Sprintf("%.2f", total),
		ShippingAddress: address,
		CreatedAt:       time.Now().Unix(),
		UpdatedAt:       time.Now().Unix(),
	}

	s.orders[tenantID][order.OrderId] = order
	return order, nil
}

func (s *OrderStore) GetOrder(ctx context.Context, tenantID, orderID string) (*ordersv1.Order, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tenantOrders, exists := s.orders[tenantID]
	if !exists {
		return nil, fmt.Errorf("tenant not found: %s", tenantID)
	}

	order, exists := tenantOrders[orderID]
	if !exists {
		return nil, fmt.Errorf("order not found: %s", orderID)
	}

	return order, nil
}

func (s *OrderStore) ListOrders(ctx context.Context, tenantID, customerID string) ([]*ordersv1.Order, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tenantOrders, exists := s.orders[tenantID]
	if !exists {
		return []*ordersv1.Order{}, nil
	}

	var result []*ordersv1.Order
	for _, order := range tenantOrders {
		if customerID == "" || order.CustomerId == customerID {
			result = append(result, order)
		}
	}

	return result, nil
}

// OrderHandler implements the CQRS handler interfaces
type OrderHandler struct {
	store *OrderStore
}

func NewOrderHandler(store *OrderStore) *OrderHandler {
	return &OrderHandler{store: store}
}

// Command handlers
func (h *OrderHandler) CreateOrder(ctx context.Context, req *ordersv1.CreateOrderRequest) (*ordersv1.CreateOrderResponse, error) {
	tenantID := GetTenantID(ctx)
	fmt.Printf("   [%s] Creating order for customer %s\n", tenantID, req.CustomerId)

	order, err := h.store.CreateOrder(ctx, tenantID, req.CustomerId, req.Items, req.ShippingAddress)
	if err != nil {
		return nil, err
	}

	return &ordersv1.CreateOrderResponse{
		OrderId:     order.OrderId,
		Status:      order.Status.String(),
		TotalAmount: order.TotalAmount,
	}, nil
}

func (h *OrderHandler) CancelOrder(ctx context.Context, req *ordersv1.CancelOrderRequest) (*ordersv1.CancelOrderResponse, error) {
	tenantID := GetTenantID(ctx)
	fmt.Printf("   [%s] Cancelling order %s\n", tenantID, req.OrderId)

	return &ordersv1.CancelOrderResponse{
		Success: true,
		Message: fmt.Sprintf("Order %s cancelled: %s", req.OrderId, req.Reason),
	}, nil
}

func (h *OrderHandler) UpdateOrderStatus(ctx context.Context, req *ordersv1.UpdateOrderStatusRequest) (*ordersv1.UpdateOrderStatusResponse, error) {
	tenantID := GetTenantID(ctx)
	fmt.Printf("   [%s] Updating order %s status to %s\n", tenantID, req.OrderId, req.NewStatus)

	return &ordersv1.UpdateOrderStatusResponse{
		Success:       true,
		CurrentStatus: req.NewStatus,
	}, nil
}

// Query handlers
func (h *OrderHandler) GetOrder(ctx context.Context, req *ordersv1.GetOrderRequest) (*ordersv1.Order, error) {
	tenantID := GetTenantID(ctx)
	fmt.Printf("   [%s] Getting order %s\n", tenantID, req.OrderId)

	return h.store.GetOrder(ctx, tenantID, req.OrderId)
}

func (h *OrderHandler) ListOrders(ctx context.Context, req *ordersv1.ListOrdersRequest) (*ordersv1.ListOrdersResponse, error) {
	tenantID := GetTenantID(ctx)
	fmt.Printf("   [%s] Listing orders for customer %s\n", tenantID, req.CustomerId)

	orders, err := h.store.ListOrders(ctx, tenantID, req.CustomerId)
	if err != nil {
		return nil, err
	}

	return &ordersv1.ListOrdersResponse{
		Orders:     orders,
		TotalCount: int32(len(orders)),
	}, nil
}

func (h *OrderHandler) GetOrdersByStatus(ctx context.Context, req *ordersv1.GetOrdersByStatusRequest) (*ordersv1.ListOrdersResponse, error) {
	tenantID := GetTenantID(ctx)
	fmt.Printf("   [%s] Getting orders by status %s\n", tenantID, req.Status)

	// Simplified - return empty for now
	return &ordersv1.ListOrdersResponse{
		Orders:     []*ordersv1.Order{},
		TotalCount: 0,
	}, nil
}

// TenantSubjectBuilder builds NATS subjects with tenant prefix
type TenantSubjectBuilder struct {
	mu sync.RWMutex
}

func (t *TenantSubjectBuilder) BuildSubject(ctx context.Context, packageName, serviceName, methodName string) string {
	tenantID := GetTenantID(ctx)
	// Format: tenant-{id}.package.service.method
	// Example: "tenant-acme.orders.v1.OrderCommandService.CreateOrder"
	return fmt.Sprintf("tenant-%s.%s.%s.%s", tenantID, packageName, serviceName, methodName)
}

// TenantExtractorInterceptor extracts tenant from NATS subject and adds to context
type TenantExtractorInterceptor struct{}

func (i *TenantExtractorInterceptor) InterceptHandler(next cqrs.HandlerFunc) cqrs.HandlerFunc {
	return func(ctx context.Context, request proto.Message) (proto.Message, error) {
		// In a real implementation, you'd extract the tenant from the NATS message subject
		// For this example, we'll extract it from the subject if available in context
		// The NATS server would need to pass this via context

		// For demo purposes, we'll use a default tenant in the server
		// In production, parse from msg.Subject in the NATS handler
		ctx = WithTenantID(ctx, "server-extracted")

		return next(ctx, request)
	}
}

func main() {
	fmt.Println("=== CQRS Multi-Tenant Example ===")
	fmt.Println("Pure CQRS with dynamic subject routing")
	fmt.Println()

	ctx := context.Background()

	// 1. Start Embedded NATS Server
	fmt.Println("1️⃣  Starting embedded NATS server...")
	opts := &server.Options{
		Host: "127.0.0.1",
		Port: 14222,
	}
	ns, err := server.NewServer(opts)
	if err != nil {
		log.Fatalf("Failed to create NATS server: %v", err)
	}

	go ns.Start()

	if !ns.ReadyForConnections(4 * time.Second) {
		log.Fatal("NATS server not ready")
	}
	defer ns.Shutdown()
	fmt.Println("   ✅ NATS server ready")
	fmt.Println()

	// 2. Create Order Store and Handler
	fmt.Println("2️⃣  Creating order handler...")
	store := NewOrderStore()
	handler := NewOrderHandler(store)
	fmt.Println("   ✅ Handler ready")
	fmt.Println()

	// 3. Start CQRS Server (listens on wildcard subjects)
	fmt.Println("3️⃣  Starting CQRS servers...")

	natsServer, err := cqrsnats.NewServer(&cqrsnats.ServerConfig{
		ServerConfig: &cqrs.ServerConfig{
			QueueGroup:     "order-handlers",
			MaxConcurrent:  10,
			HandlerTimeout: 5 * time.Second,
		},
		URL:  "nats://localhost:14222",
		Name: "OrderService",
	})
	if err != nil {
		log.Fatalf("Failed to create NATS server: %v", err)
	}
	defer natsServer.Close()

	// Register handlers
	commandServer := ordersv1.NewCqrsOrderCommandServiceServer(natsServer, handler)
	if err := commandServer.Start(ctx); err != nil {
		log.Fatalf("Failed to start command server: %v", err)
	}

	queryServer := ordersv1.NewCqrsOrderQueryServiceServer(natsServer, handler)
	if err := queryServer.Start(ctx); err != nil {
		log.Fatalf("Failed to start query server: %v", err)
	}

	fmt.Println("   ✅ Servers started and listening on wildcard subjects")
	fmt.Println()

	time.Sleep(500 * time.Millisecond)

	// 4. Create Clients with Tenant-Specific Subject Builders
	fmt.Println("4️⃣  Creating tenant-specific clients...")

	transport, err := cqrsnats.NewTransport(&cqrsnats.TransportConfig{
		TransportConfig: &cqrs.TransportConfig{
			Timeout: 5 * time.Second,
		},
		URL:  "nats://localhost:14222",
		Name: "order-client",
	})
	if err != nil {
		log.Fatalf("Failed to create transport: %v", err)
	}
	defer transport.Close()

	// Tenant A client with custom subject builder
	tenantBuilder := &TenantSubjectBuilder{}

	commandClientA := ordersv1.NewCqrsOrderCommandServiceClient(
		transport,
		cqrs.WithClientSubjectBuilder(tenantBuilder),
	)

	queryClientA := ordersv1.NewCqrsOrderQueryServiceClient(
		transport,
		cqrs.WithClientSubjectBuilder(tenantBuilder),
	)

	fmt.Println("   ✅ Clients ready with custom SubjectBuilder")
	fmt.Println()

	// 5. Execute Commands for Different Tenants
	fmt.Println("5️⃣  Executing multi-tenant commands...")
	fmt.Println()

	// Tenant ACME creates an order
	ctxACME := WithTenantID(ctx, "acme")
	fmt.Println("   📦 [ACME] Creating order...")
	resp1, err := commandClientA.CreateOrder(ctxACME, &ordersv1.CreateOrderRequest{
		CustomerId: "customer-001",
		Items: []*ordersv1.OrderItem{
			{ProductId: "prod-1", ProductName: "Widget", Quantity: 5, Price: "10.00"},
		},
		ShippingAddress: "123 Main St, ACME City",
	})
	if err != nil {
		fmt.Printf("   ❌ Error: %v\n", err)
	} else {
		fmt.Printf("   ✅ Order created: %s (Total: $%s)\n", resp1.OrderId, resp1.TotalAmount)
	}
	fmt.Println()

	// Tenant GLOBEX creates an order
	ctxGLOBEX := WithTenantID(ctx, "globex")
	fmt.Println("   📦 [GLOBEX] Creating order...")
	resp2, err := commandClientA.CreateOrder(ctxGLOBEX, &ordersv1.CreateOrderRequest{
		CustomerId: "customer-002",
		Items: []*ordersv1.OrderItem{
			{ProductId: "prod-2", ProductName: "Gadget", Quantity: 3, Price: "10.00"},
		},
		ShippingAddress: "456 Oak Ave, GLOBEX Town",
	})
	if err != nil {
		fmt.Printf("   ❌ Error: %v\n", err)
	} else {
		fmt.Printf("   ✅ Order created: %s (Total: $%s)\n", resp2.OrderId, resp2.TotalAmount)
	}
	fmt.Println()

	// 6. Query Orders (tenant-isolated)
	fmt.Println("6️⃣  Querying tenant-specific orders...")
	fmt.Println()

	fmt.Println("   🔍 [ACME] Listing orders...")
	listResp, err := queryClientA.ListOrders(ctxACME, &ordersv1.ListOrdersRequest{
		CustomerId: "customer-001",
	})
	if err != nil {
		fmt.Printf("   ❌ Error: %v\n", err)
	} else {
		fmt.Printf("   ✅ Found %d orders for ACME\n", listResp.TotalCount)
		for _, order := range listResp.Orders {
			fmt.Printf("      - Order %s: $%s (%s)\n", order.OrderId, order.TotalAmount, order.Status)
		}
	}
	fmt.Println()

	fmt.Println("✅ Demo complete!")
	fmt.Println()
	fmt.Println("Key Takeaways:")
	fmt.Println("  • Custom SubjectBuilder enables multi-tenancy")
	fmt.Println("  • Each tenant gets isolated NATS subjects")
	fmt.Println("  • Pure CQRS - no event sourcing required")
	fmt.Println("  • Shared cqrs.ClientOption for all clients")
}
