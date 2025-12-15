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
func (orchestrator *IntelligentOrchestrator) Initialize() error {
	orchestrator.mu.Lock()
	defer orchestrator.mu.Unlock()

	log.Println("[Orchestrator] Initializing...")

	// Discover available policies
	if err := orchestrator.metricsAggregator.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize metrics aggregator: %w", err)
	}

	policies := orchestrator.metricsAggregator.GetConfiguredPolicies()
	log.Printf("[Orchestrator] Discovered %d policies: %v", len(policies), policies)

	// Perform simplified fast profiling
	log.Println("[Orchestrator] Starting simplified fast profiling...")
	if err := orchestrator.metricsAggregator.FastProfile(); err != nil {
		return fmt.Errorf("fast profiling failed: %w", err)
	}

	// Select best initial policy
	bestPolicy := orchestrator.metricsAggregator.SelectBestPolicy()
	if bestPolicy == "" {
		return fmt.Errorf("no suitable initial policy found")
	}

	// Switch to best policy
	log.Printf("[Orchestrator] Switching to initial best policy: %s", bestPolicy)
	if err := orchestrator.papercacheClient.Policy(bestPolicy); err != nil {
		return fmt.Errorf("failed to set initial policy: %w", err)
	}

	orchestrator.currentPolicy = bestPolicy
	orchestrator.lastSwitchTime = time.Now()

	// Configure cost-benefit analyzer threshold
	orchestrator.costBenefitAnalyzer.SetThreshold(orchestrator.config.SwitchThreshold)

	log.Println("[Orchestrator] Initialization complete")
	orchestrator.metricsAggregator.PrintSummary()

	return nil
}

// Start begins the orchestration loop
func (orchestrator *IntelligentOrchestrator) Start() error {
	// Ensure only one instance is running
	orchestrator.mu.Lock()
	if orchestrator.isRunning {
		orchestrator.mu.Unlock()
		return fmt.Errorf("orchestrator already running")
	}
	orchestrator.isRunning = true
	orchestrator.mu.Unlock()

	log.Printf("[Orchestrator] Starting evaluation loop (interval: %v)", orchestrator.config.EvaluationInterval)

	// Set up ticker for periodic evaluation
	ticker := time.NewTicker(orchestrator.config.EvaluationInterval)
	defer ticker.Stop()

	// Main loop for evaluations and stopping signal
	for {
		select {
		case <-ticker.C:
			orchestrator.evaluationLoop()

		case <-orchestrator.stopChan:
			log.Println("[Orchestrator] Stopping...")
			orchestrator.mu.Lock()
			orchestrator.isRunning = false
			orchestrator.mu.Unlock()
			return nil
		}
	}
}

// Stop gracefully stops the orchestration loop
func (orchestrator *IntelligentOrchestrator) Stop() {
	close(orchestrator.stopChan)
}

// evaluationLoop runs the main orchestration logic
func (orchestrator *IntelligentOrchestrator) evaluationLoop() {
	// Read current state, protected by mutex, then release lock for processing
	orchestrator.mu.Lock()
	currentPolicy := orchestrator.currentPolicy
	lastSwitch := orchestrator.lastSwitchTime
	stabilityPeriod := orchestrator.config.StabilityPeriod
	orchestrator.mu.Unlock()

	log.Println("[Orchestrator] Evaluation Cycle Started")

	// Check stability period
	timeSinceSwitch := time.Since(lastSwitch)
	if timeSinceSwitch < stabilityPeriod {
		log.Printf("[Orchestrator] In stability period (%.0fs / %.0fs) - skipping evaluation",
			timeSinceSwitch.Seconds(), stabilityPeriod.Seconds())
		return
	}

	// Collect current metrics and Log the current state
	aggregated, err := orchestrator.metricsAggregator.CollectCurrentMetrics()
	if err != nil {
		log.Printf("[Orchestrator] Failed to collect metrics: %v", err)
		return
	}

	currentSnapshot := orchestrator.metricsAggregator.GetLatestSnapshot()
	if currentSnapshot == nil {
		log.Println("[Orchestrator] No snapshot available")
		return
	}

	log.Printf("[Orchestrator] Current policy: %s, miss ratio: %.4f",
		currentPolicy, currentSnapshot.MissRatio)

	// Detect pattern shift
	patternShift := orchestrator.patternDetector.DetectShift()

	// Analyze performance trend
	history := orchestrator.metricsAggregator.GetHistory()
	trend := orchestrator.trendAnalyzer.AnalyzeTrend(history, currentPolicy)

	// Decide if evaluation needed
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

	// Evaluate candidate policies
	orchestrator.evaluateCandidates(currentPolicy, aggregated)
}

// evaluateCandidates performs cost-benefit analysis on alternative policies
func (orchestrator *IntelligentOrchestrator) evaluateCandidates(currentPolicy string, aggregated *AggregatedMetrics) {
	log.Println("[Orchestrator] Evaluating candidate policies...")

	// Get current policy miss ratio
	currentSnapshot := orchestrator.metricsAggregator.GetLatestSnapshot()
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
	bestAnalysis := orchestrator.costBenefitAnalyzer.CompareMultipleCandidates(
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
			orchestrator.config.SwitchThreshold*100)
		return
	}

	// Perform the switch
	orchestrator.performSwitch(bestAnalysis)
}

// performSwitch executes a policy switch
func (orchestrator *IntelligentOrchestrator) performSwitch(analysis *CostBenefitAnalysis) {
	log.Printf("[Orchestrator] Switching policy: %s → %s",
		analysis.CurrentPolicy, analysis.CandidatePolicy)
	log.Printf("[Orchestrator] Expected improvement: %.2f%% net benefit",
		analysis.NetBenefit*100)

	// Execute switch
	if err := orchestrator.papercacheClient.Policy(analysis.CandidatePolicy); err != nil {
		log.Printf("[Orchestrator] Switch failed: %v", err)
		return
	}

	// Update state
	orchestrator.mu.Lock()
	orchestrator.currentPolicy = analysis.CandidatePolicy
	orchestrator.lastSwitchTime = time.Now()
	orchestrator.mu.Unlock()

	// Clear pattern detector (new baseline)
	orchestrator.patternDetector.Clear()

	log.Printf("[Orchestrator] Switch completed successfully")
}

// RecordAccess records a cache access for pattern detection
func (orchestrator *IntelligentOrchestrator) RecordAccess(key string, isHit bool) {
	orchestrator.patternDetector.RecordAccess(key, isHit)
}

// GetCurrentPolicy returns the current active policy
func (orchestrator *IntelligentOrchestrator) GetCurrentPolicy() string {
	orchestrator.mu.RLock()
	defer orchestrator.mu.RUnlock()
	return orchestrator.currentPolicy
}

// GetStatus returns orchestrator status information
func (orchestrator *IntelligentOrchestrator) GetStatus() map[string]interface{} {
	orchestrator.mu.RLock()
	defer orchestrator.mu.RUnlock()

	timeSinceSwitch := time.Since(orchestrator.lastSwitchTime)

	return map[string]interface{}{
		"is_running":          orchestrator.isRunning,
		"current_policy":      orchestrator.currentPolicy,
		"last_switch":         orchestrator.lastSwitchTime.Format(time.RFC3339),
		"time_since_switch":   timeSinceSwitch.String(),
		"evaluation_interval": orchestrator.config.EvaluationInterval.String(),
		"stability_period":    orchestrator.config.StabilityPeriod.String(),
		"switch_threshold":    fmt.Sprintf("%.1f%%", orchestrator.config.SwitchThreshold*100),
	}
}

// PrintStatus logs current orchestrator status
func (orchestrator *IntelligentOrchestrator) PrintStatus() {
	status := orchestrator.GetStatus()

	log.Println("Orchestrator Status:")
	for key, value := range status {
		log.Printf("  %s: %v", key, value)
	}

	// Print component summaries
	log.Println("\nComponent Status:")
	orchestrator.metricsAggregator.PrintSummary()
	orchestrator.patternDetector.PrintSummary()
	orchestrator.costBenefitAnalyzer.PrintSummary()
	log.Println("===========================")
}
