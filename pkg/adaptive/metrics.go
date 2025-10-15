package adaptive

import (
	"log"
	"sync"
	"time"

	paperClient "github.com/PaperCache/paper-client-go"
)

// PolicyMetrics holds performance metrics for a single policy
type PolicyMetrics struct {
	Name      string
	MissRatio float64
	HitRatio  float64
	Evictions int64
}

// MetricsSnapshot represents cache state at a point in time
type MetricsSnapshot struct {
	Timestamp     time.Time
	CurrentPolicy string
	MiniStacks    map[string]*PolicyMetrics // policy_name -> metrics
	CacheSize     int64
	NumObjects    int64
}

// AggregatedMetrics provides a comprehensive view of cache performance
type AggregatedMetrics struct {
	CurrentPolicyMissRatio float64
	MiniStacks             map[string]*PolicyMetrics
	CacheSize              int64
	History                []*MetricsSnapshot
}

// MetricsAggregator collects and stores cache performance metrics
type MetricsAggregator struct {
	papercacheClient *paperclient.PaperClient

	history        []*MetricsSnapshot
	maxHistorySize int

	mu sync.RWMutex
}

// NewMetricsAggregator creates a new metrics aggregator
func NewMetricsAggregator(client *paperclient.PaperClient) *MetricsAggregator {
	return &MetricsAggregator{
		papercacheClient: client,
		history:          make([]*MetricsSnapshot, 0),
		maxHistorySize:   10, // Keep last 10 snapshots (100 seconds)
	}
}

// CollectMetrics queries PaperCache and stores a new snapshot
func (ma *MetricsAggregator) CollectMetrics() (*AggregatedMetrics, error) {
	// Query PaperCache status
	status, err := ma.papercacheClient.Status()
	if err != nil {
		return nil, err
	}

	// Create snapshot
	snapshot := &MetricsSnapshot{
		Timestamp:     time.Now(),
		CurrentPolicy: status.Policy,
		MiniStacks:    make(map[string]*PolicyMetrics),
		CacheSize:     int64(status.CacheSize),
		NumObjects:    int64(status.NumObjects),
	}

	// Convert PaperCache MiniStacks to our format
	for policyName, miniStack := range status.MiniStacks {
		snapshot.MiniStacks[policyName] = &PolicyMetrics{
			Name:      policyName,
			MissRatio: miniStack.MissRatio,
			HitRatio:  1.0 - miniStack.MissRatio, // Calculate hit ratio
			Evictions: int64(miniStack.Evictions),
		}
	}

	// Store in history (thread-safe)
	ma.mu.Lock()
	ma.history = append(ma.history, snapshot)

	// Trim history if exceeds max size
	if len(ma.history) > ma.maxHistorySize {
		ma.history = ma.history[1:]
	}
	ma.mu.Unlock()

	// Build aggregated metrics
	aggregated := &AggregatedMetrics{
		MiniStacks: snapshot.MiniStacks,
		CacheSize:  snapshot.CacheSize,
		History:    ma.GetHistory(),
	}

	// Set current policy miss ratio
	if currentPolicyMetrics, exists := snapshot.MiniStacks[status.Policy]; exists {
		aggregated.CurrentPolicyMissRatio = currentPolicyMetrics.MissRatio
	}

	return aggregated, nil
}

// GetHistory returns a copy of the metrics history (thread-safe)
func (ma *MetricsAggregator) GetHistory() []*MetricsSnapshot {
	ma.mu.RLock()
	defer ma.mu.RUnlock()

	// Return a copy to prevent external modification
	historyCopy := make([]*MetricsSnapshot, len(ma.history))
	copy(historyCopy, ma.history)

	return historyCopy
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

// Start begins periodic metrics collection
func (ma *MetricsAggregator) Start(interval time.Duration, stopChan <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Printf("[MetricsAggregator] Started with %v interval", interval)

	for {
		select {
		case <-ticker.C:
			metrics, err := ma.CollectMetrics()
			if err != nil {
				log.Printf("[MetricsAggregator] Error collecting metrics: %v", err)
				continue
			}

			// Log summary
			log.Printf("[MetricsAggregator] Collected metrics: policy=%s, miss_ratio=%.4f, cache_size=%d, objects=%d",
				metrics.History[len(metrics.History)-1].CurrentPolicy,
				metrics.CurrentPolicyMissRatio,
				metrics.CacheSize,
				metrics.History[len(metrics.History)-1].NumObjects)

		case <-stopChan:
			log.Println("[MetricsAggregator] Stopped")
			return
		}
	}
}
