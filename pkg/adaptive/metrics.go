package adaptive

import (
	"fmt"
	"log"
	"sync"
	"time"

	paperClient "github.com/danenherdi/paper-client-go"
)

// PolicyMetrics holds performance metrics for a single policy
type PolicyMetrics struct {
	Name        string
	MissRatio   float64
	HitRatio    float64
	LastUpdated time.Time
	SampleCount int // Number of times this policy has been measured
}

// MetricsSnapshot represents cache state at a specific point in time
type MetricsSnapshot struct {
	Timestamp     time.Time
	CurrentPolicy string
	MissRatio     float64
	CacheSize     uint64
	UsedSize      uint64
	NumObjects    uint64
	TotalGets     uint64
	TotalSets     uint64
}

// AggregatedMetrics provides a comprehensive view of cache performance
type AggregatedMetrics struct {
	CurrentPolicy          string
	CurrentPolicyMissRatio float64
	AllPoliciesMetrics     map[string]*PolicyMetrics
	CacheSize              uint64
	History                []*MetricsSnapshot
	IsComplete             bool // True if we have data for all configured policies
}

// MetricsAggregator collects and aggregates cache performance metrics
// Supports fast profiling mode for rapid baseline data collection
type MetricsAggregator struct {
	papercacheClient *paperClient.PaperClient

	// Metrics storage
	policyMetrics      map[string]*PolicyMetrics
	history            []*MetricsSnapshot
	maxHistorySize     int
	configuredPolicies []string

	mu sync.RWMutex
}

// NewMetricsAggregator creates a new metrics aggregator instance
func NewMetricsAggregator(client *paperClient.PaperClient) *MetricsAggregator {
	return &MetricsAggregator{
		papercacheClient: client,
		policyMetrics:    make(map[string]*PolicyMetrics),
		history:          make([]*MetricsSnapshot, 0, 10),
		maxHistorySize:   10, // Keep last 10 snapshots (100 seconds at 10s interval)
	}
}

// Initialize discovers configured policies from PaperCache
// Must be called before any other operations
func (metricsAggr *MetricsAggregator) Initialize() error {

	// Get initial status to discover policies
	status, err := metricsAggr.papercacheClient.Status()
	if err != nil {
		return fmt.Errorf("failed to get initial status: %w", err)
	}

	// Store configured policies
	metricsAggr.mu.Lock()
	metricsAggr.configuredPolicies = make([]string, len(status.GetPolicies()))
	copy(metricsAggr.configuredPolicies, status.GetPolicies())
	metricsAggr.mu.Unlock()

	log.Printf("[MetricsAggregator] Initialized with %d policies: %v",
		len(status.GetPolicies()), status.GetPolicies())

	return nil
}

// FastProfile performs rapid profiling of all configured policies
// Switches through each policy, waits for minimal reconstruction, and collects metrics
// Example Duration: ~10 seconds for 4 policies (2.5s per policy)
func (metricsAggr *MetricsAggregator) FastProfile() error {
	// Get list of configured policies
	metricsAggr.mu.RLock()
	policies := make([]string, len(metricsAggr.configuredPolicies))
	copy(policies, metricsAggr.configuredPolicies)
	metricsAggr.mu.RUnlock()

	if len(policies) == 0 {
		return fmt.Errorf("no policies configured, call Initialize() first")
	}

	log.Printf("[MetricsAggregator] Starting fast profiling of %d policies...", len(policies))
	startTime := time.Now()

	// Iterate through each policy and collect metrics
	for i, policyName := range policies {
		log.Printf("[MetricsAggregator] [%d/%d] Profiling policy: %s",
			i+1, len(policies), policyName)

		// Switch to policy for profiling
		if err := metricsAggr.papercacheClient.Policy(policyName); err != nil {
			log.Printf("[MetricsAggregator] WARNING: Failed to switch to %s: %v",
				policyName, err)
			continue
		}

		// Wait for minimal reconstruction time
		// is a reasonable compromise for quick profiling while allowing some cache warm-up
		time.Sleep(2 * time.Second)

		// Collect metrics for this policy
		status, err := metricsAggr.papercacheClient.Status()
		if err != nil {
			log.Printf("[MetricsAggregator] WARNING: Failed to get status for %s: %v",
				policyName, err)
			continue
		}

		// Store metrics for this policy
		metricsAggr.storePolicyMetrics(policyName, status)

		log.Printf("[MetricsAggregator] %s metrics collected: miss_ratio=%.4f, hit_ratio=%.4f",
			policyName, status.GetMissRatio(), 1.0-status.GetMissRatio())
	}

	duration := time.Since(startTime)
	log.Printf("[MetricsAggregator] Fast profiling completed in %v", duration)

	return nil
}

// SimplifiedFastProfile profiles only LFU and LRU policies
// This is faster than full profiling and covers the most common use cases
// Duration: ~6 seconds (2.5s per policy + overhead)
func (metricsAggr *MetricsAggregator) SimplifiedFastProfile() error {
	corePolicies := []string{"lfu", "lru"}

	log.Printf("[MetricsAggregator] Starting simplified fast profiling (LFU vs LRU)...")
	startTime := time.Now()

	// Iterate through core policies and collect metrics
	for i, policyName := range corePolicies {
		log.Printf("[MetricsAggregator] [%d/2] Profiling policy: %s", i+1, policyName)

		// Switch to policy for profiling
		if err := metricsAggr.papercacheClient.Policy(policyName); err != nil {
			return fmt.Errorf("failed to switch to %s: %w", policyName, err)
		}

		// Wait for reconstruction
		// is a reasonable compromise for quick profiling while allowing some cache warm-up
		time.Sleep(2500 * time.Millisecond)

		// Collect metrics
		status, err := metricsAggr.papercacheClient.Status()
		if err != nil {
			return fmt.Errorf("failed to get status for %s: %w", policyName, err)
		}

		// Store metrics
		metricsAggr.storePolicyMetrics(policyName, status)

		log.Printf("[MetricsAggregator] %s metrics: miss_ratio=%.4f",
			policyName, status.GetMissRatio())
	}

	duration := time.Since(startTime)
	log.Printf("[MetricsAggregator] Simplified profiling completed in %v", duration)

	return nil
}

// storePolicyMetrics stores or updates metrics for a specific policy (internal helper)
func (metricsAggr *MetricsAggregator) storePolicyMetrics(policyName string, status *paperClient.PaperStatus) {
	// Must be called with lock held for writing
	metricsAggr.mu.Lock()
	defer metricsAggr.mu.Unlock()

	// Update or create metrics entry for the policy
	if existing, exists := metricsAggr.policyMetrics[policyName]; exists {
		existing.MissRatio = status.GetMissRatio()
		existing.HitRatio = 1.0 - status.GetMissRatio()
		existing.LastUpdated = time.Now()
		existing.SampleCount++
	} else {
		metricsAggr.policyMetrics[policyName] = &PolicyMetrics{
			Name:        policyName,
			MissRatio:   status.GetMissRatio(),
			HitRatio:    1.0 - status.GetMissRatio(),
			LastUpdated: time.Now(),
			SampleCount: 1,
		}
	}
}

// CollectCurrentMetrics collects metrics for the currently active policy
// This should be called periodically during normal operation
func (metricsAggr *MetricsAggregator) CollectCurrentMetrics() (*AggregatedMetrics, error) {
	// Get current status from PaperCache
	status, err := metricsAggr.papercacheClient.Status()
	if err != nil {
		return nil, fmt.Errorf("failed to get status: %w", err)
	}

	metricsAggr.mu.Lock()
	defer metricsAggr.mu.Unlock()

	// Update current policy metrics or create new entry if not exists
	if existing, exists := metricsAggr.policyMetrics[status.GetPolicy()]; exists {
		existing.MissRatio = status.GetMissRatio()
		existing.HitRatio = 1.0 - status.GetMissRatio()
		existing.LastUpdated = time.Now()
		existing.SampleCount++
	} else {
		metricsAggr.policyMetrics[status.GetPolicy()] = &PolicyMetrics{
			Name:        status.GetPolicy(),
			MissRatio:   status.GetMissRatio(),
			HitRatio:    1.0 - status.GetMissRatio(),
			LastUpdated: time.Now(),
			SampleCount: 1,
		}
	}
	// Create and store snapshot
	snapshot := &MetricsSnapshot{
		Timestamp:     time.Now(),
		CurrentPolicy: status.GetPolicy(),
		MissRatio:     status.GetMissRatio(),
		CacheSize:     status.GetMaxSize(),
		UsedSize:      status.GetUsedSize(),
		NumObjects:    status.GetNumObjects(),
		TotalGets:     status.GetTotalGets(),
		TotalSets:     status.GetTotalSets(),
	}

	metricsAggr.history = append(metricsAggr.history, snapshot)

	// Trim history if exceeds max size
	if len(metricsAggr.history) > metricsAggr.maxHistorySize {
		metricsAggr.history = metricsAggr.history[1:]
	}

	return metricsAggr.buildAggregatedMetrics(), nil
}

// buildAggregatedMetrics creates an aggregated view of all metrics
// Must be called with lock held for reading or writing
func (metricsAggr *MetricsAggregator) buildAggregatedMetrics() *AggregatedMetrics {
	aggregated := &AggregatedMetrics{
		AllPoliciesMetrics: make(map[string]*PolicyMetrics, len(metricsAggr.policyMetrics)),
		History:            make([]*MetricsSnapshot, len(metricsAggr.history)),
		IsComplete:         len(metricsAggr.policyMetrics) == len(metricsAggr.configuredPolicies),
	}

	// Deep copy policy metrics to avoid external modification
	for k, v := range metricsAggr.policyMetrics {
		aggregated.AllPoliciesMetrics[k] = &PolicyMetrics{
			Name:        v.Name,
			MissRatio:   v.MissRatio,
			HitRatio:    v.HitRatio,
			LastUpdated: v.LastUpdated,
			SampleCount: v.SampleCount,
		}
	}

	// Copy history snapshots
	copy(aggregated.History, metricsAggr.history)

	// Set current policy info from latest snapshot
	if len(metricsAggr.history) > 0 {
		latest := metricsAggr.history[len(metricsAggr.history)-1]
		aggregated.CurrentPolicy = latest.CurrentPolicy
		aggregated.CurrentPolicyMissRatio = latest.MissRatio
		aggregated.CacheSize = latest.CacheSize
	}

	return aggregated
}

// GetAggregatedMetrics returns the current aggregated metrics view
func (metricsAggr *MetricsAggregator) GetAggregatedMetrics() *AggregatedMetrics {
	metricsAggr.mu.RLock()
	defer metricsAggr.mu.RUnlock()

	return metricsAggr.buildAggregatedMetrics()
}

// StartMonitoring begins continuous metrics collection in background
// Collects metrics at the specified interval until stopChan is closed
func (metricsAggr *MetricsAggregator) StartMonitoring(interval time.Duration, stopChan <-chan struct{}) {
	log.Printf("[MetricsAggregator] Started monitoring mode (interval: %v)", interval)

	// Ticker for periodic collection
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Monitoring loop
	for {
		// Wait for next tick or stop signal
		select {
		case <-ticker.C:
			metrics, err := metricsAggr.CollectCurrentMetrics()
			if err != nil {
				log.Printf("[MetricsAggregator] Collection error: %v", err)
				continue
			}

			log.Printf("[MetricsAggregator] Current: policy=%s, miss_ratio=%.4f, objects=%d",
				metrics.CurrentPolicy,
				metrics.CurrentPolicyMissRatio,
				metrics.History[len(metrics.History)-1].NumObjects)

		case <-stopChan:
			log.Println("[MetricsAggregator] Monitoring stopped")
			return
		}
	}
}

// GetPolicyMetrics returns metrics for a specific policy
// Returns nil if the policy hasn't been measured yet
func (metricsAggr *MetricsAggregator) GetPolicyMetrics(policyName string) *PolicyMetrics {
	metricsAggr.mu.RLock()
	defer metricsAggr.mu.RUnlock()

	if metrics, exists := metricsAggr.policyMetrics[policyName]; exists {
		// Return a copy to prevent external modification
		return &PolicyMetrics{
			Name:        metrics.Name,
			MissRatio:   metrics.MissRatio,
			HitRatio:    metrics.HitRatio,
			LastUpdated: metrics.LastUpdated,
			SampleCount: metrics.SampleCount,
		}
	}

	return nil
}

// GetHistory returns a copy of the metrics history
func (metricsAggr *MetricsAggregator) GetHistory() []*MetricsSnapshot {
	metricsAggr.mu.RLock()
	defer metricsAggr.mu.RUnlock()

	history := make([]*MetricsSnapshot, len(metricsAggr.history))
	copy(history, metricsAggr.history)

	return history
}

// GetLatestSnapshot returns the most recent metrics snapshot
func (metricsAggr *MetricsAggregator) GetLatestSnapshot() *MetricsSnapshot {
	metricsAggr.mu.RLock()
	defer metricsAggr.mu.RUnlock()

	if len(metricsAggr.history) == 0 {
		return nil
	}

	return metricsAggr.history[len(metricsAggr.history)-1]
}

// IsDataComplete checks if we have metrics for all configured policies
func (metricsAggr *MetricsAggregator) IsDataComplete() bool {
	metricsAggr.mu.RLock()
	defer metricsAggr.mu.RUnlock()

	return len(metricsAggr.policyMetrics) == len(metricsAggr.configuredPolicies)
}

// GetConfiguredPolicies returns the list of configured policies
func (metricsAggr *MetricsAggregator) GetConfiguredPolicies() []string {
	metricsAggr.mu.RLock()
	defer metricsAggr.mu.RUnlock()

	policies := make([]string, len(metricsAggr.configuredPolicies))
	copy(policies, metricsAggr.configuredPolicies)

	return policies
}

// SelectBestPolicy returns the policy with the lowest miss ratio
// Returns empty string if no metrics available
func (metricsAggr *MetricsAggregator) SelectBestPolicy() string {
	metricsAggr.mu.RLock()
	defer metricsAggr.mu.RUnlock()

	if len(metricsAggr.policyMetrics) == 0 {
		return ""
	}

	var bestPolicy string
	bestMissRatio := 1.0

	for policyName, metrics := range metricsAggr.policyMetrics {
		if metrics.MissRatio <= bestMissRatio {
			bestMissRatio = metrics.MissRatio
			bestPolicy = policyName
		}
	}

	return bestPolicy
}

// PrintSummary logs a summary of collected metrics
func (metricsAggr *MetricsAggregator) PrintSummary() {
	metricsAggr.mu.RLock()
	defer metricsAggr.mu.RUnlock()

	log.Println("MetricsAggregator Summary")
	log.Printf("Configured policies: %v", metricsAggr.configuredPolicies)
	log.Printf("Collected metrics for %d policies", len(metricsAggr.policyMetrics))
	log.Printf("History snapshots: %d", len(metricsAggr.history))

	log.Println("\nPolicy Metrics:")
	for _, policyName := range metricsAggr.configuredPolicies {
		if metrics, exists := metricsAggr.policyMetrics[policyName]; exists {
			log.Printf("  %s: miss_ratio=%.4f, hit_ratio=%.4f, samples=%d, last_updated=%s",
				metrics.Name,
				metrics.MissRatio,
				metrics.HitRatio,
				metrics.SampleCount,
				metrics.LastUpdated.Format("15:04:05"))
		} else {
			log.Printf("  %s: NO DATA", policyName)
		}
	}

	if len(metricsAggr.policyMetrics) > 0 {
		bestPolicy := ""
		bestMissRatio := 1.0
		for name, metrics := range metricsAggr.policyMetrics {
			if metrics.MissRatio <= bestMissRatio {
				bestMissRatio = metrics.MissRatio
				bestPolicy = name
			}
		}
		log.Printf("\nBest Policy: %s (miss_ratio=%.4f)", bestPolicy, bestMissRatio)
	}

	log.Println("=================================")
}
