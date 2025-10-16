package adaptive

import (
	"testing"
	"time"

	paperclient "github.com/danenherdi/paper-client-go"
)

func TestMetricsAggregator_Initialize(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client, err := paperclient.ClientConnect("paper://localhost:3145")
	if err != nil {
		t.Fatalf("Failed to connect to PaperCache: %v", err)
	}
	defer client.Disconnect()

	aggregator := NewMetricsAggregator(client)

	err = aggregator.Initialize()
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	policies := aggregator.GetConfiguredPolicies()
	if len(policies) == 0 {
		t.Error("Expected at least one configured policy")
	}

	t.Logf("Discovered policies: %v", policies)
}

func TestMetricsAggregator_SimplifiedFastProfile(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client, err := paperclient.ClientConnect("paper://localhost:3145")
	if err != nil {
		t.Fatalf("Failed to connect to PaperCache: %v", err)
	}
	defer client.Disconnect()

	aggregator := NewMetricsAggregator(client)

	// Initialize
	if err := aggregator.Initialize(); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Run simplified fast profile
	startTime := time.Now()
	err = aggregator.SimplifiedFastProfile()
	duration := time.Since(startTime)

	if err != nil {
		t.Fatalf("SimplifiedFastProfile failed: %v", err)
	}

	t.Logf("Profiling completed in %v", duration)

	// Verify we got metrics for LFU and LRU
	lfuMetrics := aggregator.GetPolicyMetrics("lfu")
	lruMetrics := aggregator.GetPolicyMetrics("lru")

	if lfuMetrics == nil {
		t.Error("Expected LFU metrics")
	} else {
		t.Logf("LFU: miss_ratio=%.4f, hit_ratio=%.4f",
			lfuMetrics.MissRatio, lfuMetrics.HitRatio)
	}

	if lruMetrics == nil {
		t.Error("Expected LRU metrics")
	} else {
		t.Logf("LRU: miss_ratio=%.4f, hit_ratio=%.4f",
			lruMetrics.MissRatio, lruMetrics.HitRatio)
	}

	// Check duration (should be around 6 seconds)
	if duration > 10*time.Second {
		t.Errorf("Profiling took too long: %v (expected ~6s)", duration)
	}

	// Print summary
	aggregator.PrintSummary()
}

func TestMetricsAggregator_FastProfile(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client, err := paperclient.ClientConnect("paper://localhost:3145")
	if err != nil {
		t.Fatalf("Failed to connect to PaperCache: %v", err)
	}
	defer client.Disconnect()

	aggregator := NewMetricsAggregator(client)

	if err := aggregator.Initialize(); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Run full fast profile
	startTime := time.Now()
	err = aggregator.FastProfile()
	duration := time.Since(startTime)

	if err != nil {
		t.Fatalf("FastProfile failed: %v", err)
	}

	t.Logf("Full profiling completed in %v", duration)

	// Check if we have data for all policies
	if !aggregator.IsDataComplete() {
		t.Error("Expected complete data for all policies")
	}

	// Verify each policy has metrics
	for _, policyName := range aggregator.GetConfiguredPolicies() {
		metrics := aggregator.GetPolicyMetrics(policyName)
		if metrics == nil {
			t.Errorf("Missing metrics for policy: %s", policyName)
		} else {
			t.Logf("%s: miss_ratio=%.4f, samples=%d",
				metrics.Name, metrics.MissRatio, metrics.SampleCount)
		}
	}

	// Print summary
	aggregator.PrintSummary()
}

func TestMetricsAggregator_CollectCurrentMetrics(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client, err := paperclient.ClientConnect("paper://localhost:3145")
	if err != nil {
		t.Fatalf("Failed to connect to PaperCache: %v", err)
	}
	defer client.Disconnect()

	aggregator := NewMetricsAggregator(client)

	if err := aggregator.Initialize(); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Collect metrics
	metrics, err := aggregator.CollectCurrentMetrics()
	if err != nil {
		t.Fatalf("CollectCurrentMetrics failed: %v", err)
	}

	if metrics == nil {
		t.Fatal("Expected non-nil metrics")
	}

	if metrics.CurrentPolicy == "" {
		t.Error("Expected non-empty current policy")
	}

	t.Logf("Current policy: %s, miss_ratio: %.4f",
		metrics.CurrentPolicy, metrics.CurrentPolicyMissRatio)

	// Verify history
	history := aggregator.GetHistory()
	if len(history) != 1 {
		t.Errorf("Expected 1 snapshot, got %d", len(history))
	}
}

func TestMetricsAggregator_StartMonitoring(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client, err := paperclient.ClientConnect("paper://localhost:3145")
	if err != nil {
		t.Fatalf("Failed to connect to PaperCache: %v", err)
	}
	defer client.Disconnect()

	aggregator := NewMetricsAggregator(client)

	if err := aggregator.Initialize(); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Start monitoring
	stopChan := make(chan struct{})
	go aggregator.StartMonitoring(2*time.Second, stopChan)

	// Let it run for 6 seconds (3 collections)
	time.Sleep(6500 * time.Millisecond)

	// Stop
	close(stopChan)
	time.Sleep(500 * time.Millisecond)

	// Verify history
	history := aggregator.GetHistory()
	if len(history) < 3 {
		t.Errorf("Expected at least 3 snapshots, got %d", len(history))
	}

	t.Logf("Collected %d snapshots", len(history))

	// Print summary
	aggregator.PrintSummary()
}

func TestMetricsAggregator_SelectBestPolicy(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client, err := paperclient.ClientConnect("paper://localhost:3145")
	if err != nil {
		t.Fatalf("Failed to connect to PaperCache: %v", err)
	}
	defer client.Disconnect()

	aggregator := NewMetricsAggregator(client)

	if err := aggregator.Initialize(); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Run simplified profile
	if err := aggregator.SimplifiedFastProfile(); err != nil {
		t.Fatalf("SimplifiedFastProfile failed: %v", err)
	}

	// Select best policy
	bestPolicy := aggregator.SelectBestPolicy()
	if bestPolicy == "" {
		t.Error("Expected non-empty best policy")
	}

	t.Logf("Best policy: %s", bestPolicy)

	// Verify it's either LFU or LRU
	if bestPolicy != "lfu" && bestPolicy != "lru" {
		t.Errorf("Expected LFU or LRU, got %s", bestPolicy)
	}

	bestMetrics := aggregator.GetPolicyMetrics(bestPolicy)
	t.Logf("Best policy %s metrics: miss_ratio=%.4f",
		bestPolicy, bestMetrics.MissRatio)
}
