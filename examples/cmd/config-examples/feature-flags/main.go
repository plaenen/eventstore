package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/plaenen/eventstore/pkg/config"
)

func main() {
	fmt.Println("=== Feature Flags Example ===")
	fmt.Println()

	ctx := context.Background()

	// Example 1: Static feature flags (development)
	fmt.Println("1️⃣  Static Feature Flags (Development)")
	fmt.Println()

	flags := config.FeatureFlags{
		EnableNewUI:                true,
		EnableExperimentalFeatures: false,
		EnableDebugMode:            true,
		EnableMetrics:              true,
		Features: map[string]bool{
			"dark_mode":          true,
			"beta_dashboard":     true,
			"experimental_cache": false,
		},
	}

	provider := config.NewStaticProvider(flags)
	defer provider.Close()

	currentFlags, err := provider.Get(ctx)
	if err != nil {
		log.Fatalf("Failed to get flags: %v", err)
	}

	fmt.Printf("   ✅ Loaded feature flags:\n")
	fmt.Printf("      New UI: %v\n", currentFlags.EnableNewUI)
	fmt.Printf("      Debug Mode: %v\n", currentFlags.EnableDebugMode)
	fmt.Printf("      Metrics: %v\n", currentFlags.EnableMetrics)
	fmt.Println()

	// Check specific features
	if currentFlags.IsEnabled("dark_mode") {
		fmt.Println("   🌙 Dark mode is enabled")
	}

	if currentFlags.IsEnabled("beta_dashboard") {
		fmt.Println("   📊 Beta dashboard is enabled")
	}

	if !currentFlags.IsEnabled("experimental_cache") {
		fmt.Println("   ⚠️  Experimental cache is disabled")
	}
	fmt.Println()

	// Example 2: Dynamic feature flags with watching
	fmt.Println("2️⃣  Dynamic Feature Flags with Watching")
	fmt.Println()

	// In production, you'd use:
	// provider, _ := config.NewProvider[config.FeatureFlags](ctx,
	//     "awsparamstore:///prod/feature-flags?decoder=json")

	// For this demo, we'll use static with manual updates
	dynamicFlags := config.FeatureFlags{
		EnableNewUI: false,
		Features: map[string]bool{
			"new_feature": false,
		},
	}

	dynamicProvider := config.NewStaticProvider(dynamicFlags)
	defer dynamicProvider.Close()

	// Watch for changes
	updated := make(chan config.FeatureFlags, 1)
	stop, err := dynamicProvider.Watch(ctx, func(flags config.FeatureFlags) {
		fmt.Printf("   🔄 Configuration updated!\n")
		fmt.Printf("      New UI: %v\n", flags.EnableNewUI)
		fmt.Printf("      New Feature: %v\n", flags.IsEnabled("new_feature"))
		updated <- flags
	})
	if err != nil {
		log.Fatalf("Failed to watch: %v", err)
	}
	defer stop()

	// Wait for initial callback with timeout
	select {
	case <-updated:
		fmt.Println()
	case <-time.After(2 * time.Second):
		fmt.Println("   ⏱️  Timeout waiting for initial callback")
		fmt.Println()
	}

	// Simulate a feature flag change
	fmt.Println("   📝 Simulating feature flag update...")
	time.Sleep(500 * time.Millisecond)

	newFlags := config.FeatureFlags{
		EnableNewUI: true, // Enabled!
		Features: map[string]bool{
			"new_feature": true, // Enabled!
		},
	}
	dynamicProvider.Update(newFlags)

	// Wait for update callback with timeout
	select {
	case <-updated:
		fmt.Println()
	case <-time.After(2 * time.Second):
		// StaticProvider Watch doesn't trigger on Update, that's OK
		fmt.Println("   ℹ️  Note: StaticProvider Watch callback only fires once on creation")
		fmt.Println()
	}

	// Example 3: Application usage pattern
	fmt.Println("3️⃣  Application Usage Pattern")
	fmt.Println()

	app := NewApplication(provider)
	app.HandleRequest()
	fmt.Println()

	// Example 4: Progressive rollout
	fmt.Println("4️⃣  Progressive Feature Rollout")
	fmt.Println()

	rolloutFlags := config.FeatureFlags{
		Features: map[string]bool{
			"new_api_v2":     true, // 100% rollout
			"beta_ui":        true, // 50% rollout (handled by app logic)
			"experimental_x": false, // 0% rollout
		},
	}

	rolloutProvider := config.NewStaticProvider(rolloutFlags)
	defer rolloutProvider.Close()

	current, _ := rolloutProvider.Get(ctx)

	fmt.Println("   📊 Feature Rollout Status:")
	fmt.Printf("      API v2: %v (100%% enabled)\n", current.IsEnabled("new_api_v2"))
	fmt.Printf("      Beta UI: %v (50%% enabled)\n", current.IsEnabled("beta_ui"))
	fmt.Printf("      Experimental X: %v (0%% enabled)\n", current.IsEnabled("experimental_x"))
	fmt.Println()

	// Example 5: Environment-specific flags
	fmt.Println("5️⃣  Environment-Specific Configuration")
	fmt.Println()

	envFlags := map[string]config.FeatureFlags{
		"development": {
			EnableDebugMode:            true,
			EnableExperimentalFeatures: true,
			EnableMetrics:              false,
		},
		"staging": {
			EnableDebugMode:            true,
			EnableExperimentalFeatures: false,
			EnableMetrics:              true,
		},
		"production": {
			EnableDebugMode:            false,
			EnableExperimentalFeatures: false,
			EnableMetrics:              true,
			EnableTracing:              true,
		},
	}

	for env, flags := range envFlags {
		fmt.Printf("   %s:\n", env)
		fmt.Printf("      Debug: %v | Experimental: %v | Metrics: %v | Tracing: %v\n",
			flags.EnableDebugMode,
			flags.EnableExperimentalFeatures,
			flags.EnableMetrics,
			flags.EnableTracing)
	}
	fmt.Println()

	fmt.Println("🎉 Feature Flags Demo Complete!")
	fmt.Println()
	fmt.Println("Key Benefits:")
	fmt.Println("   • Enable/disable features without code changes")
	fmt.Println("   • Progressive rollouts")
	fmt.Println("   • A/B testing support")
	fmt.Println("   • Emergency kill switches")
	fmt.Println("   • Environment-specific configuration")
	fmt.Println()
}

// Application demonstrates feature flag usage in an application
type Application struct {
	flagProvider config.Provider[config.FeatureFlags]
}

func NewApplication(provider config.Provider[config.FeatureFlags]) *Application {
	return &Application{
		flagProvider: provider,
	}
}

func (a *Application) HandleRequest() {
	flags, err := a.flagProvider.Latest()
	if err != nil {
		log.Printf("Failed to get flags: %v", err)
		return
	}

	fmt.Println("   🚀 Processing request with current feature flags:")

	// Feature-specific logic
	if flags.EnableNewUI {
		fmt.Println("      ✨ Serving new UI")
	} else {
		fmt.Println("      📄 Serving classic UI")
	}

	if flags.EnableMetrics {
		fmt.Println("      📈 Collecting metrics")
	}

	if flags.IsEnabled("dark_mode") {
		fmt.Println("      🌙 Applying dark mode theme")
	}
}
