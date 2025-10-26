package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/plaenen/eventstore/pkg/config"
)

func main() {
	fmt.Println("=== Dynamic Runtime Tuning Example ===")
	fmt.Println()

	ctx := context.Background()

	// Example 1: Basic runtime tuning
	fmt.Println("1️⃣  Basic Runtime Tuning Configuration")
	fmt.Println()

	tuning := config.RuntimeTuning{
		MaxConcurrency:    10,
		EventBatchSize:    100,
		ProjectionWorkers: 5,
		CacheTTL:          5 * time.Minute,
		RequestTimeout:    10 * time.Second,
		IdleTimeout:       60 * time.Second,
		BufferSize:        1000,
		Parameters: map[string]interface{}{
			"retry_attempts":     3,
			"backoff_multiplier": 2.0,
			"max_batch_delay":    "100ms",
		},
	}

	// Validate configuration
	if err := tuning.Validate(); err != nil {
		log.Fatalf("Invalid tuning: %v", err)
	}

	provider := config.NewStaticProvider(tuning)
	defer provider.Close()

	current, err := provider.Get(ctx)
	if err != nil {
		log.Fatalf("Failed to get tuning: %v", err)
	}

	fmt.Printf("   ⚙️  Current Runtime Configuration:\n")
	fmt.Printf("      Max Concurrency: %d\n", current.MaxConcurrency)
	fmt.Printf("      Event Batch Size: %d\n", current.EventBatchSize)
	fmt.Printf("      Projection Workers: %d\n", current.ProjectionWorkers)
	fmt.Printf("      Cache TTL: %s\n", current.CacheTTL)
	fmt.Printf("      Request Timeout: %s\n", current.RequestTimeout)
	fmt.Printf("      Buffer Size: %d\n", current.BufferSize)
	fmt.Println()

	// Access custom parameters
	if retries, ok := current.GetParameter("retry_attempts"); ok {
		fmt.Printf("   🔁 Retry Attempts: %v\n", retries)
	}

	if backoff, ok := current.GetParameter("backoff_multiplier"); ok {
		fmt.Printf("   ⏱️  Backoff Multiplier: %v\n", backoff)
	}
	fmt.Println()

	// Example 2: Dynamic tuning with hot-reload
	fmt.Println("2️⃣  Dynamic Tuning with Hot-Reload")
	fmt.Println()

	dynamicTuning := config.RuntimeTuning{
		MaxConcurrency:    5,
		EventBatchSize:    50,
		ProjectionWorkers: 2,
		CacheTTL:          1 * time.Minute,
	}

	dynamicProvider := config.NewStaticProvider(dynamicTuning)
	defer dynamicProvider.Close()

	// Create an application that responds to config changes
	app := NewEventProcessor(dynamicProvider)

	// Start watching for config changes
	updated := make(chan config.RuntimeTuning, 1)
	stop, err := dynamicProvider.Watch(ctx, func(tuning config.RuntimeTuning) {
		fmt.Println("   🔄 Configuration updated!")
		fmt.Printf("      Max Concurrency: %d → %d\n", app.GetMaxConcurrency(), tuning.MaxConcurrency)
		fmt.Printf("      Batch Size: %d → %d\n", app.GetBatchSize(), tuning.EventBatchSize)

		// Update application
		app.UpdateConfig(tuning)
		updated <- tuning
	})
	if err != nil {
		log.Fatalf("Failed to watch: %v", err)
	}
	defer stop()

	// Wait for initial callback
	<-updated
	fmt.Println()

	// Process some events with initial config
	fmt.Println("   📊 Processing events with initial configuration...")
	app.ProcessEvents(100)
	fmt.Println()

	// Simulate load increase - adjust tuning
	fmt.Println("   📈 Simulating load increase - updating tuning...")
	time.Sleep(500 * time.Millisecond)

	highLoadTuning := config.RuntimeTuning{
		MaxConcurrency:    20,  // Increased!
		EventBatchSize:    200, // Increased!
		ProjectionWorkers: 10,  // Increased!
		CacheTTL:          10 * time.Minute,
	}

	dynamicProvider.Update(highLoadTuning)
	<-updated
	fmt.Println()

	// Process with updated config
	fmt.Println("   📊 Processing events with updated configuration...")
	app.ProcessEvents(100)
	fmt.Println()

	// Example 3: Performance profiles
	fmt.Println("3️⃣  Performance Profiles")
	fmt.Println()

	profiles := map[string]config.RuntimeTuning{
		"low-latency": {
			MaxConcurrency:    50,
			EventBatchSize:    10,
			ProjectionWorkers: 20,
			CacheTTL:          30 * time.Second,
			RequestTimeout:    1 * time.Second,
			BufferSize:        100,
		},
		"high-throughput": {
			MaxConcurrency:    10,
			EventBatchSize:    1000,
			ProjectionWorkers: 5,
			CacheTTL:          10 * time.Minute,
			RequestTimeout:    30 * time.Second,
			BufferSize:        10000,
		},
		"balanced": {
			MaxConcurrency:    20,
			EventBatchSize:    100,
			ProjectionWorkers: 10,
			CacheTTL:          5 * time.Minute,
			RequestTimeout:    10 * time.Second,
			BufferSize:        1000,
		},
	}

	for name, profile := range profiles {
		fmt.Printf("   %s:\n", name)
		fmt.Printf("      Concurrency: %d | Batch: %d | Workers: %d | Cache: %s\n",
			profile.MaxConcurrency,
			profile.EventBatchSize,
			profile.ProjectionWorkers,
			profile.CacheTTL)
	}
	fmt.Println()

	// Example 4: Auto-scaling based on metrics
	fmt.Println("4️⃣  Auto-Scaling Pattern")
	fmt.Println()

	autoScaler := NewAutoScaler(dynamicProvider)
	autoScaler.AdjustForLoad(0.8) // 80% load
	fmt.Println()

	// Example 5: A/B testing different configurations
	fmt.Println("5️⃣  A/B Testing Configuration")
	fmt.Println()

	configA := config.RuntimeTuning{
		EventBatchSize: 100,
		CacheTTL:       5 * time.Minute,
	}

	configB := config.RuntimeTuning{
		EventBatchSize: 200,
		CacheTTL:       10 * time.Minute,
	}

	fmt.Println("   📊 Testing Configuration A:")
	fmt.Printf("      Batch Size: %d, Cache TTL: %s\n", configA.EventBatchSize, configA.CacheTTL)

	fmt.Println("   📊 Testing Configuration B:")
	fmt.Printf("      Batch Size: %d, Cache TTL: %s\n", configB.EventBatchSize, configB.CacheTTL)
	fmt.Println()

	fmt.Println("🎉 Dynamic Tuning Demo Complete!")
	fmt.Println()
	fmt.Println("Key Benefits:")
	fmt.Println("   • Hot-reload without restarts")
	fmt.Println("   • Performance optimization in real-time")
	fmt.Println("   • A/B testing configurations")
	fmt.Println("   • Auto-scaling support")
	fmt.Println("   • Environment-specific tuning")
	fmt.Println()

	fmt.Println("Use Cases:")
	fmt.Println("   • Adjust concurrency during peak load")
	fmt.Println("   • Optimize batch sizes for throughput")
	fmt.Println("   • Tune cache TTLs based on hit rates")
	fmt.Println("   • Experiment with different configurations")
	fmt.Println("   • Quick response to performance issues")
	fmt.Println()
}

// EventProcessor demonstrates runtime tuning in action
type EventProcessor struct {
	tuningProvider   config.Provider[config.RuntimeTuning]
	maxConcurrency   int
	batchSize        int
	projectionWorkers int
}

func NewEventProcessor(provider config.Provider[config.RuntimeTuning]) *EventProcessor {
	initial, _ := provider.Latest()
	return &EventProcessor{
		tuningProvider:    provider,
		maxConcurrency:    initial.MaxConcurrency,
		batchSize:         initial.EventBatchSize,
		projectionWorkers: initial.ProjectionWorkers,
	}
}

func (ep *EventProcessor) UpdateConfig(tuning config.RuntimeTuning) {
	ep.maxConcurrency = tuning.MaxConcurrency
	ep.batchSize = tuning.EventBatchSize
	ep.projectionWorkers = tuning.ProjectionWorkers

	fmt.Printf("   ✅ Configuration applied\n")
}

func (ep *EventProcessor) GetMaxConcurrency() int {
	return ep.maxConcurrency
}

func (ep *EventProcessor) GetBatchSize() int {
	return ep.batchSize
}

func (ep *EventProcessor) ProcessEvents(count int) {
	batches := (count + ep.batchSize - 1) / ep.batchSize

	fmt.Printf("      Processing %d events in %d batches (size: %d)\n",
		count, batches, ep.batchSize)
	fmt.Printf("      Using %d concurrent workers\n", ep.maxConcurrency)
	fmt.Printf("      %d projection workers active\n", ep.projectionWorkers)
}

// AutoScaler adjusts configuration based on load
type AutoScaler struct {
	tuningProvider config.Provider[config.RuntimeTuning]
}

func NewAutoScaler(provider config.Provider[config.RuntimeTuning]) *AutoScaler {
	return &AutoScaler{
		tuningProvider: provider,
	}
}

func (as *AutoScaler) AdjustForLoad(load float64) {
	current, _ := as.tuningProvider.Latest()

	fmt.Printf("   📊 Current Load: %.0f%%\n", load*100)

	if load > 0.8 {
		// High load - scale up
		fmt.Println("   ⬆️  High load detected - scaling up:")
		fmt.Printf("      Concurrency: %d → %d\n", current.MaxConcurrency, current.MaxConcurrency*2)
		fmt.Printf("      Workers: %d → %d\n", current.ProjectionWorkers, current.ProjectionWorkers*2)
	} else if load < 0.3 {
		// Low load - scale down
		fmt.Println("   ⬇️  Low load detected - scaling down:")
		fmt.Printf("      Concurrency: %d → %d\n", current.MaxConcurrency, current.MaxConcurrency/2)
		fmt.Printf("      Workers: %d → %d\n", current.ProjectionWorkers, current.ProjectionWorkers/2)
	} else {
		fmt.Println("   ✅ Load within normal range - no adjustment needed")
	}
}
