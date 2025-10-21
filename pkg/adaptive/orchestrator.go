package adaptive

import (
	"fmt"
	"log"
	"sync"
	"time"

	paperClient "github.com/danenherdi/paper-client-go"
)

// OrchestratorConfig holds configuration for the orchestrator
type OrchestratorConfig struct {
	EvaluationInterval time.Duration // How often to evaluate (default: 10s)
	StabilityPeriod    time.Duration // Cooldown after switch (default: 30s)
	SwitchThreshold    float64       // Min net benefit to switch (default: 0.05)
	MaxMemory          uint64        // System memory limit
}

// DefaultOrchestratorConfig returns default configuration
func DefaultOrchestratorConfig() *OrchestratorConfig {
	return &OrchestratorConfig{
		EvaluationInterval: 10 * time.Second,
		StabilityPeriod:    30 * time.Second,
		SwitchThreshold:    0.05,
		MaxMemory:          8 * 1024 * 1024 * 1024, // 8GB default
	}
}

// IntelligentOrchestrator coordinates adaptive policy switching
type IntelligentOrchestrator struct {
	// Core components
	papercacheClient    *paperClient.PaperClient
	metricsAggregator   *MetricsAggregator
	patternDetector     *PatternDetector
	trendAnalyzer       *TrendAnalyzer
	costBenefitAnalyzer *CostBenefitAnalyzer

	// Configuration
	config *OrchestratorConfig

	// State
	currentPolicy  string
	lastSwitchTime time.Time
	isRunning      bool
	stopChan       chan struct{}

	mu sync.RWMutex
}

// NewIntelligentOrchestrator creates a new orchestrator instance
func NewIntelligentOrchestrator(client *paperClient.PaperClient, config *OrchestratorConfig) *IntelligentOrchestrator {
	if config == nil {
		config = DefaultOrchestratorConfig()
	}

	return &IntelligentOrchestrator{
		papercacheClient:    client,
		metricsAggregator:   NewMetricsAggregator(client),
		patternDetector:     NewPatternDetector(1000), // 1000 access window
		trendAnalyzer:       NewTrendAnalyzer(),
		costBenefitAnalyzer: NewCostBenefitAnalyzer(config.MaxMemory),

		config:         config,
		lastSwitchTime: time.Now(),
		stopChan:       make(chan struct{}),
	}
}

// Initialize performs initial setup and policy selection
func (o *IntelligentOrchestrator) Initialize() error {
	o.mu.Lock()
	defer o.mu.Unlock()

	log.Println("[Orchestrator] Initializing...")

	// Step 1: Discover available policies
	if err := o.metricsAggregator.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize metrics aggregator: %w", err)
	}

	policies := o.metricsAggregator.GetConfiguredPolicies()
	log.Printf("[Orchestrator] Discovered %d policies: %v", len(policies), policies)

	// Step 2: Perform simplified fast profiling (6 seconds - LFU vs LRU)
	log.Println("[Orchestrator] Starting simplified fast profiling (6 seconds)...")
	if err := o.metricsAggregator.SimplifiedFastProfile(); err != nil {
		return fmt.Errorf("fast profiling failed: %w", err)
	}

	// Step 3: Select best initial policy
	bestPolicy := o.metricsAggregator.SelectBestPolicy()
	if bestPolicy == "" {
		return fmt.Errorf("no suitable initial policy found")
	}

	// Step 4: Switch to best policy
	log.Printf("[Orchestrator] Switching to initial best policy: %s", bestPolicy)
	if err := o.papercacheClient.Policy(bestPolicy); err != nil {
		return fmt.Errorf("failed to set initial policy: %w", err)
	}

	o.currentPolicy = bestPolicy
	o.lastSwitchTime = time.Now()

	// Configure cost-benefit analyzer threshold
	o.costBenefitAnalyzer.SetThreshold(o.config.SwitchThreshold)

	log.Println("[Orchestrator] Initialization complete")
	o.metricsAggregator.PrintSummary()

	return nil
}

// Start begins the orchestration loop
func (o *IntelligentOrchestrator) Start() error {
	o.mu.Lock()
	if o.isRunning {
		o.mu.Unlock()
		return fmt.Errorf("orchestrator already running")
	}
	o.isRunning = true
	o.mu.Unlock()

	log.Printf("[Orchestrator] Starting evaluation loop (interval: %v)", o.config.EvaluationInterval)

	ticker := time.NewTicker(o.config.EvaluationInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			o.evaluationLoop()

		case <-o.stopChan:
			log.Println("[Orchestrator] Stopping...")
			o.mu.Lock()
			o.isRunning = false
			o.mu.Unlock()
			return nil
		}
	}
}

// Stop gracefully stops the orchestration loop
func (o *IntelligentOrchestrator) Stop() {
	close(o.stopChan)
}

// evaluationLoop runs the main orchestration logic
func (o *IntelligentOrchestrator) evaluationLoop() {
	o.mu.Lock()
	currentPolicy := o.currentPolicy
	lastSwitch := o.lastSwitchTime
	stabilityPeriod := o.config.StabilityPeriod
	o.mu.Unlock()

	log.Println("[Orchestrator] === Evaluation Cycle ===")

	// Step 1: Check stability period
	timeSinceSwitch := time.Since(lastSwitch)
	if timeSinceSwitch < stabilityPeriod {
		log.Printf("[Orchestrator] In stability period (%.0fs / %.0fs) - skipping evaluation",
			timeSinceSwitch.Seconds(), stabilityPeriod.Seconds())
		return
	}

	// Step 2: Collect current metrics
	aggregated, err := o.metricsAggregator.CollectCurrentMetrics()
	if err != nil {
		log.Printf("[Orchestrator] Failed to collect metrics: %v", err)
		return
	}

	currentSnapshot := o.metricsAggregator.GetLatestSnapshot()
	if currentSnapshot == nil {
		log.Println("[Orchestrator] No snapshot available")
		return
	}

	log.Printf("[Orchestrator] Current policy: %s, miss ratio: %.4f",
		currentPolicy, currentSnapshot.MissRatio)

	// Step 3: Detect pattern shift
	patternShift := o.patternDetector.DetectShift()

	// Step 4: Analyze performance trend
	history := o.metricsAggregator.GetHistory()
	trend := o.trendAnalyzer.AnalyzeTrend(history, currentPolicy)

	// Step 5: Decide if evaluation needed
	needsEvaluation := patternShift.Detected || trend.IndicatesDegradation

	if !needsEvaluation {
		log.Println("[Orchestrator] System stable - no action needed")
		return
	}

	// Log reason for evaluation
	if patternShift.Detected {
		log.Printf("[Orchestrator] Pattern shift detected (temporal: %.3f, frequency: %.3f)",
			patternShift.TemporalShift, patternShift.FrequencyShift)
	}
	if trend.IndicatesDegradation {
		log.Printf("[Orchestrator] Performance degradation detected (slope: %.6f, variance: %.6f)",
			trend.Slope, trend.Variance)
	}

	// Step 6: Evaluate candidate policies
	o.evaluateCandidates(currentPolicy, aggregated)
}

// evaluateCandidates performs cost-benefit analysis on alternative policies
func (o *IntelligentOrchestrator) evaluateCandidates(currentPolicy string, aggregated *AggregatedMetrics) {
	log.Println("[Orchestrator] Evaluating candidate policies...")

	// Get current policy miss ratio
	currentSnapshot := o.metricsAggregator.GetLatestSnapshot()
	if currentSnapshot == nil {
		log.Println("[Orchestrator] No current snapshot available")
		return
	}

	// Build candidates map from AllPoliciesMetrics
	candidates := make(map[string]float64)
	for policyName, metrics := range aggregated.AllPoliciesMetrics {
		candidates[policyName] = metrics.MissRatio
	}

	if len(candidates) == 0 {
		log.Println("[Orchestrator] No candidate policies available")
		return
	}

	// Perform cost-benefit analysis
	bestAnalysis := o.costBenefitAnalyzer.CompareMultipleCandidates(
		currentPolicy,
		currentSnapshot.MissRatio,
		candidates,
		currentSnapshot.CacheSize,
	)

	if bestAnalysis == nil {
		log.Println("[Orchestrator] No suitable candidate found")
		return
	}

	// Check if should switch
	if !bestAnalysis.ShouldSwitch {
		log.Printf("[Orchestrator] Best candidate (%s) has insufficient net benefit (%.2f%% < %.2f%%)",
			bestAnalysis.CandidatePolicy,
			bestAnalysis.NetBenefit*100,
			o.config.SwitchThreshold*100)
		return
	}

	// Perform the switch
	o.performSwitch(bestAnalysis)
}

// performSwitch executes a policy switch
func (o *IntelligentOrchestrator) performSwitch(analysis *CostBenefitAnalysis) {
	log.Printf("[Orchestrator] Switching policy: %s → %s",
		analysis.CurrentPolicy, analysis.CandidatePolicy)
	log.Printf("[Orchestrator] Expected improvement: %.2f%% net benefit",
		analysis.NetBenefit*100)

	// Execute switch
	if err := o.papercacheClient.Policy(analysis.CandidatePolicy); err != nil {
		log.Printf("[Orchestrator] Switch failed: %v", err)
		return
	}

	// Update state
	o.mu.Lock()
	o.currentPolicy = analysis.CandidatePolicy
	o.lastSwitchTime = time.Now()
	o.mu.Unlock()

	// Clear pattern detector (new baseline)
	o.patternDetector.Clear()

	log.Printf("[Orchestrator] Switch completed successfully")
}

// RecordAccess records a cache access for pattern detection
func (o *IntelligentOrchestrator) RecordAccess(key string, isHit bool) {
	o.patternDetector.RecordAccess(key, isHit)
}

// GetCurrentPolicy returns the current active policy
func (o *IntelligentOrchestrator) GetCurrentPolicy() string {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.currentPolicy
}

// GetStatus returns orchestrator status information
func (o *IntelligentOrchestrator) GetStatus() map[string]interface{} {
	o.mu.RLock()
	defer o.mu.RUnlock()

	timeSinceSwitch := time.Since(o.lastSwitchTime)

	return map[string]interface{}{
		"is_running":          o.isRunning,
		"current_policy":      o.currentPolicy,
		"last_switch":         o.lastSwitchTime.Format(time.RFC3339),
		"time_since_switch":   timeSinceSwitch.String(),
		"evaluation_interval": o.config.EvaluationInterval.String(),
		"stability_period":    o.config.StabilityPeriod.String(),
		"switch_threshold":    fmt.Sprintf("%.1f%%", o.config.SwitchThreshold*100),
	}
}

// PrintStatus logs current orchestrator status
func (o *IntelligentOrchestrator) PrintStatus() {
	status := o.GetStatus()

	log.Println("=== Orchestrator Status ===")
	for key, value := range status {
		log.Printf("  %s: %v", key, value)
	}

	// Print component summaries
	log.Println("\n=== Component Status ===")
	o.metricsAggregator.PrintSummary()
	o.patternDetector.PrintSummary()
	o.costBenefitAnalyzer.PrintSummary()
	log.Println("===========================")
}
