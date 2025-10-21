package adaptive

import (
	"testing"
	"time"

	paperClient "github.com/danenherdi/paper-client-go"
)

// MockPaperClient implements the necessary methods for testing
// Note: Can't fully mock paperClient.PaperClient since it's a struct, not an interface
// So test it with a real client or create integration tests separately

func TestIntelligentOrchestrator_NewOrchestrator(t *testing.T) {
	// Skip if no real PaperCache available
	t.Skip("Requires real PaperCache instance - run integration tests separately")

	client, _ := paperClient.ClientConnect("paper://localhost:3145")
	config := DefaultOrchestratorConfig()

	orchestrator := NewIntelligentOrchestrator(client, config)

	if orchestrator == nil {
		t.Fatal("Expected non-nil orchestrator")
	}

	if orchestrator.metricsAggregator == nil {
		t.Error("MetricsAggregator not initialized")
	}

	if orchestrator.patternDetector == nil {
		t.Error("PatternDetector not initialized")
	}

	if orchestrator.trendAnalyzer == nil {
		t.Error("TrendAnalyzer not initialized")
	}

	if orchestrator.costBenefitAnalyzer == nil {
		t.Error("CostBenefitAnalyzer not initialized")
	}

	t.Log("All components initialized correctly")
}

func TestIntelligentOrchestrator_GetCurrentPolicy(t *testing.T) {
	// This test doesn't require PaperCache
	client, _ := paperClient.ClientConnect("paper://localhost:3145") // Won't be called
	orchestrator := NewIntelligentOrchestrator(client, nil)

	// Set a policy directly
	orchestrator.mu.Lock()
	orchestrator.currentPolicy = "lfu"
	orchestrator.mu.Unlock()

	policy := orchestrator.GetCurrentPolicy()
	if policy != "lfu" {
		t.Errorf("Expected policy 'lfu', got '%s'", policy)
	}

	t.Logf("Current policy correctly returned: %s", policy)
}

func TestIntelligentOrchestrator_RecordAccess(t *testing.T) {
	// This test doesn't require PaperCache
	client, _ := paperClient.ClientConnect("paper://localhost:3145") // Won't be called
	orchestrator := NewIntelligentOrchestrator(client, nil)

	// Record some accesses
	orchestrator.RecordAccess("key1", true)
	orchestrator.RecordAccess("key2", false)
	orchestrator.RecordAccess("key1", true)

	// Verify pattern detector recorded them
	size := orchestrator.patternDetector.GetAccessLogSize()
	if size != 3 {
		t.Errorf("Expected 3 recorded accesses, got %d", size)
	}

	t.Logf("Recorded %d accesses successfully", size)
}

func TestIntelligentOrchestrator_GetStatus(t *testing.T) {
	client, _ := paperClient.ClientConnect("paper://localhost:3145")
	config := &OrchestratorConfig{
		EvaluationInterval: 10 * time.Second,
		StabilityPeriod:    30 * time.Second,
		SwitchThreshold:    0.05,
		MaxMemory:          8 * 1024 * 1024 * 1024,
	}

	orchestrator := NewIntelligentOrchestrator(client, config)
	orchestrator.mu.Lock()
	orchestrator.currentPolicy = "lru"
	orchestrator.mu.Unlock()

	status := orchestrator.GetStatus()

	if status["current_policy"] != "lru" {
		t.Errorf("Expected current_policy 'lru', got %v", status["current_policy"])
	}

	if status["evaluation_interval"] != "10s" {
		t.Errorf("Expected evaluation_interval '10s', got %v", status["evaluation_interval"])
	}

	t.Logf("Status: %+v", status)
}

func TestIntelligentOrchestrator_StabilityPeriod(t *testing.T) {
	client, _ := paperClient.ClientConnect("paper://localhost:3145")
	config := &OrchestratorConfig{
		EvaluationInterval: 1 * time.Second,
		StabilityPeriod:    5 * time.Second, // Short for testing
		SwitchThreshold:    0.05,
		MaxMemory:          512 * 1024 * 1024,
	}

	orchestrator := NewIntelligentOrchestrator(client, config)
	orchestrator.mu.Lock()
	orchestrator.currentPolicy = "lru"
	orchestrator.lastSwitchTime = time.Now()
	orchestrator.mu.Unlock()

	// Immediately after switch, should be in stability period
	orchestrator.mu.RLock()
	timeSinceSwitch := time.Since(orchestrator.lastSwitchTime)
	stabilityPeriod := orchestrator.config.StabilityPeriod
	orchestrator.mu.RUnlock()

	if timeSinceSwitch >= stabilityPeriod {
		t.Error("Should be in stability period immediately after switch")
	}

	t.Logf("Time since switch: %v (stability period: %v)",
		timeSinceSwitch, stabilityPeriod)
}

func TestIntelligentOrchestrator_DefaultConfig(t *testing.T) {
	config := DefaultOrchestratorConfig()

	if config.EvaluationInterval != 10*time.Second {
		t.Errorf("Expected evaluation interval 10s, got %v", config.EvaluationInterval)
	}

	if config.StabilityPeriod != 30*time.Second {
		t.Errorf("Expected stability period 30s, got %v", config.StabilityPeriod)
	}

	if config.SwitchThreshold != 0.05 {
		t.Errorf("Expected switch threshold 0.05, got %.3f", config.SwitchThreshold)
	}

	if config.MaxMemory != 8*1024*1024*1024 {
		t.Errorf("Expected max memory 8GB, got %d", config.MaxMemory)
	}

	t.Logf("Default config: %+v", config)
}

func TestIntelligentOrchestrator_PrintStatus(t *testing.T) {
	client, _ := paperClient.ClientConnect("paper://localhost:3145")
	orchestrator := NewIntelligentOrchestrator(client, nil)

	orchestrator.mu.Lock()
	orchestrator.currentPolicy = "lfu"
	orchestrator.mu.Unlock()

	// Should log status without errors
	orchestrator.PrintStatus()

	// Just verify it doesn't crash
	status := orchestrator.GetStatus()
	if status == nil {
		t.Error("Expected non-nil status")
	}

	t.Log("PrintStatus executed without errors")
}

// ============================================================================
// INTEGRATION TESTS (Require real PaperCache instance)
// Run with: go test -tags=integration
// ============================================================================

// TestIntelligentOrchestrator_Initialize_Integration tests full initialization
// Requires PaperCache running on localhost:8191
func TestIntelligentOrchestrator_Initialize_Integration(t *testing.T) {
	t.Skip("Integration test - requires PaperCache on localhost:3145")

	client, _ := paperClient.ClientConnect("paper://localhost:3145")
	config := DefaultOrchestratorConfig()

	orchestrator := NewIntelligentOrchestrator(client, config)

	// Initialize should succeed
	err := orchestrator.Initialize()
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Should have discovered policies
	policies := orchestrator.metricsAggregator.GetConfiguredPolicies()
	if len(policies) == 0 {
		t.Error("No policies discovered")
	}

	// Should have selected a policy
	currentPolicy := orchestrator.GetCurrentPolicy()
	if currentPolicy == "" {
		t.Error("No initial policy selected")
	}

	t.Logf("Initialized with policy: %s", currentPolicy)
	t.Logf("Discovered policies: %v", policies)

	// Print summary
	orchestrator.PrintStatus()
}

// TestIntelligentOrchestrator_FullCycle_Integration tests complete orchestration cycle
func TestIntelligentOrchestrator_FullCycle_Integration(t *testing.T) {
	t.Skip("Integration test - requires PaperCache and takes ~1 minute")

	client, _ := paperClient.ClientConnect("paper://localhost:3145")
	config := DefaultOrchestratorConfig()
	config.EvaluationInterval = 5 * time.Second // Faster for testing
	config.StabilityPeriod = 10 * time.Second   // Shorter for testing

	orchestrator := NewIntelligentOrchestrator(client, config)

	// Initialize
	if err := orchestrator.Initialize(); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	t.Log("Initialization complete")

	// Start orchestration in background
	go func() {
		if err := orchestrator.Start(); err != nil {
			t.Logf("Orchestrator error: %v", err)
		}
	}()

	// Simulate some cache accesses
	for i := 0; i < 1000; i++ {
		key := "test_key_" + string(rune('a'+(i%26)))
		isHit := i%3 == 0 // 33% hit ratio
		orchestrator.RecordAccess(key, isHit)
		time.Sleep(10 * time.Millisecond)
	}

	t.Log("Simulated 1000 cache accesses")

	// Run for 30 seconds
	time.Sleep(30 * time.Second)

	// Stop orchestration
	orchestrator.Stop()
	time.Sleep(1 * time.Second) // Give it time to stop

	// Check final status
	orchestrator.PrintStatus()

	t.Log("Full cycle test completed")
}
