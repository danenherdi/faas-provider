package adaptive

import (
	"testing"
	"time"

	paperClient "github.com/PaperCache/paper-client-go"
)

// TestMetricsAggregator_CollectMetrics tests basic metrics collection
func TestMetricsAggregator_CollectMetrics(t *testing.T) {
	// Skip if PaperCache not available
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Connect to PaperCache (adjust address if needed)
	client, err := paperclient.ClientConnect("paper://localhost:3145")
	if err != nil {
		t.Fatalf("Failed to connect to PaperCache: %v", err)
	}
	defer client.Disconnect()

	// Create aggregator
	aggregator := NewMetricsAggregator(client)

	// Collect metrics
	metrics, err := aggregator.CollectMetrics()
	if err != nil {
		t.Fatalf("CollectMetrics failed: %v", err)
	}

	// Verify metrics structure
	if metrics == nil {
		t.Fatal("Expected non-nil metrics")
	}

	if len(metrics.MiniStacks) == 0 {
		t.Fatal("Expected at least one policy in MiniStacks")
	}

	// Check that history was updated
	history := aggregator.GetHistory()
	if len(history) != 1 {
		t.Errorf("Expected 1 snapshot in history, got %d", len(history))
	}

	// Verify snapshot data
	snapshot := history[0]
	if snapshot.CurrentPolicy == "" {
		t.Error("Expected non-empty current policy")
	}

	if len(snapshot.MiniStacks) == 0 {
		t.Error("Expected at least one policy in snapshot")
	}

	// Log collected metrics for inspection
	t.Logf("Current policy: %s", snapshot.CurrentPolicy)
	t.Logf("Cache size: %d", snapshot.CacheSize)
	t.Logf("Number of objects: %d", snapshot.NumObjects)

	for policyName, metrics := range snapshot.MiniStacks {
		t.Logf("Policy %s: miss_ratio=%.4f, hit_ratio=%.4f, evictions=%d",
			policyName, metrics.MissRatio, metrics.HitRatio, metrics.Evictions)
	}
}

// TestMetricsAggregator_HistoryManagement tests history trimming
func TestMetricsAggregator_HistoryManagement(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client, err := paperclient.ClientConnect("paper://localhost:3145")
	if err != nil {
		t.Fatalf("Failed to connect to PaperCache: %v", err)
	}
	defer client.Disconnect()

	aggregator := NewMetricsAggregator(client)

	// Collect metrics multiple times
	for i := 0; i < 15; i++ {
		_, err := aggregator.CollectMetrics()
		if err != nil {
			t.Fatalf("CollectMetrics failed on iteration %d: %v", i, err)
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Verify history size is capped at maxHistorySize
	history := aggregator.GetHistory()
	if len(history) != aggregator.maxHistorySize {
		t.Errorf("Expected history size %d, got %d",
			aggregator.maxHistorySize, len(history))
	}

	// Verify history is ordered by time
	for i := 1; i < len(history); i++ {
		if history[i].Timestamp.Before(history[i-1].Timestamp) {
			t.Error("History not properly ordered by timestamp")
		}
	}
}

// TestMetricsAggregator_BackgroundCollection tests the Start method
func TestMetricsAggregator_BackgroundCollection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client, err := paperclient.ClientConnect("paper://localhost:3145")
	if err != nil {
		t.Fatalf("Failed to connect to PaperCache: %v", err)
	}
	defer client.Disconnect()

	aggregator := NewMetricsAggregator(client)

	// Create stop channel
	stopChan := make(chan struct{})

	// Start background collection (1 second interval for testing)
	go aggregator.Start(1*time.Second, stopChan)

	// Let it run for 3 seconds (should collect 3-4 snapshots)
	time.Sleep(3500 * time.Millisecond)

	// Stop collection
	close(stopChan)

	// Verify snapshots were collected
	history := aggregator.GetHistory()
	if len(history) < 3 {
		t.Errorf("Expected at least 3 snapshots, got %d", len(history))
	}

	t.Logf("Collected %d snapshots in 3.5 seconds", len(history))
}

// TestMetricsAggregator_GetLatestSnapshot tests latest snapshot retrieval
func TestMetricsAggregator_GetLatestSnapshot(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client, err := paperclient.ClientConnect("paper://localhost:3145")
	if err != nil {
		t.Fatalf("Failed to connect to PaperCache: %v", err)
	}
	defer client.Disconnect()

	aggregator := NewMetricsAggregator(client)

	// Initially, no snapshots
	latest := aggregator.GetLatestSnapshot()
	if latest != nil {
		t.Error("Expected nil for empty history")
	}

	// Collect one snapshot
	_, err = aggregator.CollectMetrics()
	if err != nil {
		t.Fatalf("CollectMetrics failed: %v", err)
	}

	// Verify latest snapshot
	latest = aggregator.GetLatestSnapshot()
	if latest == nil {
		t.Fatal("Expected non-nil latest snapshot")
	}

	// Collect another snapshot
	time.Sleep(100 * time.Millisecond)
	_, err = aggregator.CollectMetrics()
	if err != nil {
		t.Fatalf("CollectMetrics failed: %v", err)
	}

	// Verify latest is updated
	newLatest := aggregator.GetLatestSnapshot()
	if newLatest.Timestamp.Before(latest.Timestamp) || newLatest.Timestamp.Equal(latest.Timestamp) {
		t.Error("Latest snapshot not updated")
	}
}
