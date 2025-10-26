package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/plaenen/eventstore/pkg/config"
)

func main() {
	fmt.Println("=== Service Discovery Example ===")
	fmt.Println()

	ctx := context.Background()

	// Example 1: Static service endpoints (development)
	fmt.Println("1️⃣  Static Service Endpoints (Development)")
	fmt.Println()

	endpoints := config.ServiceEndpoints{
		NATSURL:     "nats://localhost:4222",
		DatabaseURL: "postgres://localhost:5432/dev_db",
		CacheURL:    "redis://localhost:6379",
		APIURL:      "http://localhost:8080",
		MetricsURL:  "http://localhost:9090",
		TracingURL:  "http://localhost:14268",
		Endpoints: map[string]string{
			"auth_service":    "http://localhost:8081",
			"user_service":    "http://localhost:8082",
			"payment_service": "http://localhost:8083",
		},
	}

	// Validate configuration
	if err := endpoints.Validate(); err != nil {
		log.Fatalf("Invalid endpoints: %v", err)
	}

	provider := config.NewStaticProvider(endpoints)
	defer provider.Close()

	current, err := provider.Get(ctx)
	if err != nil {
		log.Fatalf("Failed to get endpoints: %v", err)
	}

	fmt.Printf("   ✅ Service Endpoints Loaded:\n")
	fmt.Printf("      NATS: %s\n", current.NATSURL)
	fmt.Printf("      Database: %s\n", current.DatabaseURL)
	fmt.Printf("      Cache: %s\n", current.CacheURL)
	fmt.Printf("      API: %s\n", current.APIURL)
	fmt.Println()

	// Access custom endpoints
	if authURL, ok := current.GetEndpoint("auth_service"); ok {
		fmt.Printf("   🔐 Auth Service: %s\n", authURL)
	}

	if paymentURL, ok := current.GetEndpoint("payment_service"); ok {
		fmt.Printf("   💳 Payment Service: %s\n", paymentURL)
	}
	fmt.Println()

	// Example 2: Dynamic service discovery
	fmt.Println("2️⃣  Dynamic Service Discovery")
	fmt.Println()

	// In production, endpoints might change due to:
	// - Auto-scaling
	// - Blue-green deployments
	// - Failover
	// - Load balancer updates

	// Create a dynamic provider
	// In prod: config.NewProvider[config.ServiceEndpoints](ctx,
	//           "awsparamstore:///prod/endpoints?decoder=json")

	dynamicEndpoints := config.ServiceEndpoints{
		NATSURL:     "nats://nats-cluster-1:4222",
		DatabaseURL: "postgres://db-primary:5432/prod",
		CacheURL:    "redis://redis-cluster:6379",
	}

	dynamicProvider := config.NewStaticProvider(dynamicEndpoints)
	defer dynamicProvider.Close()

	// Watch for endpoint changes
	updated := make(chan config.ServiceEndpoints, 1)
	stop, err := dynamicProvider.Watch(ctx, func(ep config.ServiceEndpoints) {
		fmt.Println("   🔄 Endpoints updated!")
		fmt.Printf("      NATS: %s\n", ep.NATSURL)
		fmt.Printf("      Database: %s\n", ep.DatabaseURL)
		updated <- ep
	})
	if err != nil {
		log.Fatalf("Failed to watch: %v", err)
	}
	defer stop()

	// Wait for initial callback
	<-updated
	fmt.Println()

	// Simulate a failover scenario
	fmt.Println("   ⚠️  Simulating database failover...")
	time.Sleep(500 * time.Millisecond)

	failoverEndpoints := config.ServiceEndpoints{
		NATSURL:     "nats://nats-cluster-1:4222",
		DatabaseURL: "postgres://db-secondary:5432/prod", // Failover!
		CacheURL:    "redis://redis-cluster:6379",
	}

	dynamicProvider.Update(failoverEndpoints)
	<-updated
	fmt.Println()

	// Example 3: Multi-environment configuration
	fmt.Println("3️⃣  Multi-Environment Configuration")
	fmt.Println()

	envConfigs := map[string]config.ServiceEndpoints{
		"development": {
			NATSURL:     "nats://localhost:4222",
			DatabaseURL: "postgres://localhost:5432/dev",
			CacheURL:    "redis://localhost:6379",
		},
		"staging": {
			NATSURL:     "nats://staging-nats:4222",
			DatabaseURL: "postgres://staging-db:5432/staging",
			CacheURL:    "redis://staging-redis:6379",
		},
		"production": {
			NATSURL:     "nats://prod-nats-cluster:4222",
			DatabaseURL: "postgres://prod-db-cluster:5432/prod",
			CacheURL:    "redis://prod-redis-cluster:6379",
		},
	}

	for env, cfg := range envConfigs {
		fmt.Printf("   %s:\n", env)
		fmt.Printf("      NATS: %s\n", cfg.NATSURL)
		fmt.Printf("      DB: %s\n", cfg.DatabaseURL)
		fmt.Printf("      Cache: %s\n", cfg.CacheURL)
		fmt.Println()
	}

	// Example 4: Application integration
	fmt.Println("4️⃣  Application Integration Pattern")
	fmt.Println()

	app := NewApplication(provider)
	if err := app.Connect(); err != nil {
		log.Printf("Connection error: %v", err)
	}
	fmt.Println()

	// Example 5: Health check pattern
	fmt.Println("5️⃣  Service Health Checks")
	fmt.Println()

	healthChecker := NewHealthChecker(provider)
	if err := healthChecker.CheckAll(ctx); err != nil {
		log.Printf("Health check failed: %v", err)
	}
	fmt.Println()

	// Example 6: Circuit breaker pattern
	fmt.Println("6️⃣  Circuit Breaker with Service Discovery")
	fmt.Println()

	circuitBreaker := NewCircuitBreaker(provider)
	circuitBreaker.CallService("payment_service")
	fmt.Println()

	fmt.Println("🎉 Service Discovery Demo Complete!")
	fmt.Println()
	fmt.Println("Key Benefits:")
	fmt.Println("   • Dynamic endpoint updates without restarts")
	fmt.Println("   • Automatic failover handling")
	fmt.Println("   • Multi-environment support")
	fmt.Println("   • Service mesh integration")
	fmt.Println("   • Zero-downtime deployments")
	fmt.Println()

	fmt.Println("Production Usage:")
	fmt.Println("   AWS: awsparamstore:///prod/endpoints")
	fmt.Println("   GCP: gcpruntimeconfig://projects/PROJECT/configs/endpoints")
	fmt.Println("   Azure: azureappconfig://connection?key=endpoints")
	fmt.Println("   etcd: etcd://localhost:2379/endpoints")
	fmt.Println()
}

// Application demonstrates service discovery in an application
type Application struct {
	endpointProvider config.Provider[config.ServiceEndpoints]
}

func NewApplication(provider config.Provider[config.ServiceEndpoints]) *Application {
	return &Application{
		endpointProvider: provider,
	}
}

func (a *Application) Connect() error {
	endpoints, err := a.endpointProvider.Latest()
	if err != nil {
		return fmt.Errorf("failed to get endpoints: %w", err)
	}

	fmt.Println("   🔌 Connecting to services:")
	fmt.Printf("      ✅ NATS: %s\n", endpoints.NATSURL)
	fmt.Printf("      ✅ Database: %s\n", endpoints.DatabaseURL)

	if endpoints.CacheURL != "" {
		fmt.Printf("      ✅ Cache: %s\n", endpoints.CacheURL)
	}

	return nil
}

// HealthChecker checks health of all configured services
type HealthChecker struct {
	endpointProvider config.Provider[config.ServiceEndpoints]
}

func NewHealthChecker(provider config.Provider[config.ServiceEndpoints]) *HealthChecker {
	return &HealthChecker{
		endpointProvider: provider,
	}
}

func (h *HealthChecker) CheckAll(ctx context.Context) error {
	endpoints, err := h.endpointProvider.Get(ctx)
	if err != nil {
		return err
	}

	fmt.Println("   🏥 Running health checks:")

	// In real implementation, you'd make actual HTTP calls
	services := []struct {
		name string
		url  string
	}{
		{"NATS", endpoints.NATSURL},
		{"Database", endpoints.DatabaseURL},
		{"Cache", endpoints.CacheURL},
	}

	for _, svc := range services {
		if svc.url == "" {
			continue
		}
		fmt.Printf("      ✅ %s: healthy\n", svc.name)
	}

	return nil
}

// CircuitBreaker demonstrates circuit breaker pattern with dynamic endpoints
type CircuitBreaker struct {
	endpointProvider config.Provider[config.ServiceEndpoints]
	failures         map[string]int
}

func NewCircuitBreaker(provider config.Provider[config.ServiceEndpoints]) *CircuitBreaker {
	return &CircuitBreaker{
		endpointProvider: provider,
		failures:         make(map[string]int),
	}
}

func (cb *CircuitBreaker) CallService(serviceName string) error {
	endpoints, err := cb.endpointProvider.Latest()
	if err != nil {
		return err
	}

	url, ok := endpoints.GetEndpoint(serviceName)
	if !ok {
		return fmt.Errorf("service %s not found", serviceName)
	}

	fmt.Printf("   🔌 Calling %s at %s\n", serviceName, url)

	// Check circuit breaker state
	if cb.failures[serviceName] >= 3 {
		fmt.Printf("   ⚠️  Circuit breaker OPEN for %s\n", serviceName)
		return fmt.Errorf("circuit breaker open")
	}

	// In real implementation, make actual call
	fmt.Printf("   ✅ Call successful\n")
	cb.failures[serviceName] = 0

	return nil
}
