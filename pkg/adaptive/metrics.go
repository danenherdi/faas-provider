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
func (ma *MetricsAggregator) Initialize() error {
	status, err := ma.papercacheClient.Status()
	if err != nil {
		return fmt.Errorf("failed to get initial status: %w", err)
	}

	ma.mu.Lock()
	ma.configuredPolicies = make([]string, len(status.GetPolicies()))
	copy(ma.configuredPolicies, status.GetPolicies())
	ma.mu.Unlock()

	log.Printf("[MetricsAggregator] Initialized with %d policies: %v",
		len(status.GetPolicies()), status.GetPolicies())

	return nil
}

// FastProfile performs rapid profiling of all configured policies
// Switches through each policy, waits for minimal reconstruction, and collects metrics
// Duration: ~10 seconds for 4 policies (2.5s per policy)
func (ma *MetricsAggregator) FastProfile() error {
	ma.mu.RLock()
	policies := make([]string, len(ma.configuredPolicies))
	copy(policies, ma.configuredPolicies)
	ma.mu.RUnlock()

	if len(policies) == 0 {
		return fmt.Errorf("no policies configured, call Initialize() first")
	}

	log.Printf("[MetricsAggregator] Starting fast profiling of %d policies...", len(policies))
	startTime := time.Now()

	for i, policyName := range policies {
		log.Printf("[MetricsAggregator] [%d/%d] Profiling policy: %s",
			i+1, len(policies), policyName)

		// Switch to policy
		if err := ma.papercacheClient.Policy(policyName); err != nil {
			log.Printf("[MetricsAggregator] WARNING: Failed to switch to %s: %v",
				policyName, err)
			continue
		}

		// Wait for minimal reconstruction time
		time.Sleep(2 * time.Second)

		// Collect metrics for this policy
		status, err := ma.papercacheClient.Status()
		if err != nil {
			log.Printf("[MetricsAggregator] WARNING: Failed to get status for %s: %v",
				policyName, err)
			continue
		}

		// Store metrics
		ma.storePolicyMetrics(policyName, status)

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
func (ma *MetricsAggregator) SimplifiedFastProfile() error {
	corePolicies := []string{"lfu", "lru"}

	log.Printf("[MetricsAggregator] Starting simplified fast profiling (LFU vs LRU)...")
	startTime := time.Now()

	for i, policyName := range corePolicies {
		log.Printf("[MetricsAggregator] [%d/2] Profiling policy: %s", i+1, policyName)

		// Switch to policy
		if err := ma.papercacheClient.Policy(policyName); err != nil {
			return fmt.Errorf("failed to switch to %s: %w", policyName, err)
		}

		// Wait for reconstruction
		time.Sleep(2500 * time.Millisecond)

		// Collect metrics
		status, err := ma.papercacheClient.Status()
		if err != nil {
			return fmt.Errorf("failed to get status for %s: %w", policyName, err)
		}

		// Store metrics
		ma.storePolicyMetrics(policyName, status)

		log.Printf("[MetricsAggregator] %s metrics: miss_ratio=%.4f",
			policyName, status.GetMissRatio())
	}

	duration := time.Since(startTime)
	log.Printf("[MetricsAggregator] Simplified profiling completed in %v", duration)

	return nil
}

// storePolicyMetrics stores or updates metrics for a specific policy (internal helper)
func (ma *MetricsAggregator) storePolicyMetrics(policyName string, status *paperClient.PaperStatus) {
	ma.mu.Lock()
	defer ma.mu.Unlock()

	if existing, exists := ma.policyMetrics[policyName]; exists {
		// Update existing metrics
		existing.MissRatio = status.GetMissRatio()
		existing.HitRatio = 1.0 - status.GetMissRatio()
		existing.LastUpdated = time.Now()
		existing.SampleCount++
	} else {
		// Create new metrics entry
		ma.policyMetrics[policyName] = &PolicyMetrics{
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
func (ma *MetricsAggregator) CollectCurrentMetrics() (*AggregatedMetrics, error) {
	status, err := ma.papercacheClient.Status()
	if err != nil {
		return nil, fmt.Errorf("failed to get status: %w", err)
	}

	ma.mu.Lock()
	defer ma.mu.Unlock()

	// Update current policy metrics
	if existing, exists := ma.policyMetrics[status.GetPolicy()]; exists {
		existing.MissRatio = status.GetMissRatio()
		existing.HitRatio = 1.0 - status.GetMissRatio()
		existing.LastUpdated = time.Now()
		existing.SampleCount++
	} else {
		ma.policyMetrics[status.GetPolicy()] = &PolicyMetrics{
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

	ma.history = append(ma.history, snapshot)

	// Trim history if exceeds max size
	if len(ma.history) > ma.maxHistorySize {
		ma.history = ma.history[1:]
	}

	return ma.buildAggregatedMetrics(), nil
}

// buildAggregatedMetrics creates an aggregated view of all metrics
// Must be called with lock held
func (ma *MetricsAggregator) buildAggregatedMetrics() *AggregatedMetrics {
	aggregated := &AggregatedMetrics{
		AllPoliciesMetrics: make(map[string]*PolicyMetrics, len(ma.policyMetrics)),
		History:            make([]*MetricsSnapshot, len(ma.history)),
		IsComplete:         len(ma.policyMetrics) == len(ma.configuredPolicies),
	}

	// Deep copy policy metrics
	for k, v := range ma.policyMetrics {
		aggregated.AllPoliciesMetrics[k] = &PolicyMetrics{
			Name:        v.Name,
			MissRatio:   v.MissRatio,
			HitRatio:    v.HitRatio,
			LastUpdated: v.LastUpdated,
			SampleCount: v.SampleCount,
		}
	}

	// Copy history
	copy(aggregated.History, ma.history)

	// Set current policy info from latest snapshot
	if len(ma.history) > 0 {
		latest := ma.history[len(ma.history)-1]
		aggregated.CurrentPolicy = latest.CurrentPolicy
		aggregated.CurrentPolicyMissRatio = latest.MissRatio
		aggregated.CacheSize = latest.CacheSize
	}

	return aggregated
}

// GetAggregatedMetrics returns the current aggregated metrics view
func (ma *MetricsAggregator) GetAggregatedMetrics() *AggregatedMetrics {
	ma.mu.RLock()
	defer ma.mu.RUnlock()

	return ma.buildAggregatedMetrics()
}

// StartMonitoring begins continuous metrics collection in background
// Collects metrics at the specified interval until stopChan is closed
func (ma *MetricsAggregator) StartMonitoring(interval time.Duration, stopChan <-chan struct{}) {
	log.Printf("[MetricsAggregator] Started monitoring mode (interval: %v)", interval)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			metrics, err := ma.CollectCurrentMetrics()
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
func (ma *MetricsAggregator) GetPolicyMetrics(policyName string) *PolicyMetrics {
	ma.mu.RLock()
	defer ma.mu.RUnlock()

	if metrics, exists := ma.policyMetrics[policyName]; exists {
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
func (ma *MetricsAggregator) GetHistory() []*MetricsSnapshot {
	ma.mu.RLock()
	defer ma.mu.RUnlock()

	history := make([]*MetricsSnapshot, len(ma.history))
	copy(history, ma.history)

	return history
}

// GetLatestSnapshot returns the most recent metrics snapshot
func (ma *MetricsAggregator) GetLatestSnapshot() *MetricsSnapshot {
	ma.mu.RLock()
	defer ma.mu.RUnlock()

	if len(ma.history) == 0 {
		return nil
	}

	return ma.history[len(ma.history)-1]
}

// IsDataComplete checks if we have metrics for all configured policies
func (ma *MetricsAggregator) IsDataComplete() bool {
	ma.mu.RLock()
	defer ma.mu.RUnlock()

	return len(ma.policyMetrics) == len(ma.configuredPolicies)
}

// GetConfiguredPolicies returns the list of configured policies
func (ma *MetricsAggregator) GetConfiguredPolicies() []string {
	ma.mu.RLock()
	defer ma.mu.RUnlock()

	policies := make([]string, len(ma.configuredPolicies))
	copy(policies, ma.configuredPolicies)

	return policies
}

// SelectBestPolicy returns the policy with the lowest miss ratio
// Returns empty string if no metrics available
func (ma *MetricsAggregator) SelectBestPolicy() string {
	ma.mu.RLock()
	defer ma.mu.RUnlock()

	if len(ma.policyMetrics) == 0 {
		return ""
	}

	var bestPolicy string
	bestMissRatio := 1.0

	for policyName, metrics := range ma.policyMetrics {
		if metrics.MissRatio < bestMissRatio {
			bestMissRatio = metrics.MissRatio
			bestPolicy = policyName
		}
	}

	return bestPolicy
}

// PrintSummary logs a summary of collected metrics
func (ma *MetricsAggregator) PrintSummary() {
	ma.mu.RLock()
	defer ma.mu.RUnlock()

	log.Println("MetricsAggregator Summary")
	log.Printf("Configured policies: %v", ma.configuredPolicies)
	log.Printf("Collected metrics for %d policies", len(ma.policyMetrics))
	log.Printf("History snapshots: %d", len(ma.history))

	log.Println("\nPolicy Metrics:")
	for _, policyName := range ma.configuredPolicies {
		if metrics, exists := ma.policyMetrics[policyName]; exists {
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

	if len(ma.policyMetrics) > 0 {
		bestPolicy := ""
		bestMissRatio := 1.0
		for name, metrics := range ma.policyMetrics {
			if metrics.MissRatio < bestMissRatio {
				bestMissRatio = metrics.MissRatio
				bestPolicy = name
			}
		}
		log.Printf("\nBest Policy: %s (miss_ratio=%.4f)", bestPolicy, bestMissRatio)
	}

	log.Println("=================================")
}
