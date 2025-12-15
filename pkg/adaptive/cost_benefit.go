package adaptive

import (
	"fmt"
	"log"
	"math"
	"sync"
)

// SwitchingCost represents the estimated cost of switching policies
type SwitchingCost struct {
	TimeCost        float64 // Time cost (normalized 0-1)
	MemoryCost      float64 // Memory overhead cost (normalized 0-1)
	DegradationCost float64 // Performance degradation cost (normalized 0-1)
	TotalCost       float64 // Weighted sum of all costs
}

// CostBenefitAnalysis represents the complete analysis result
type CostBenefitAnalysis struct {
	CurrentPolicy      string
	CandidatePolicy    string
	CurrentMissRatio   float64
	CandidateMissRatio float64
	PotentialGain      float64 // Relative improvement (0-1)
	SwitchingCost      *SwitchingCost
	NetBenefit         float64 // Gain - Cost
	ShouldSwitch       bool    // True if net benefit > threshold
}

// CostBenefitAnalyzer evaluates the trade-off of policy switching
type CostBenefitAnalyzer struct {
	maxMemory       uint64  // System memory limit (bytes)
	switchThreshold float64 // Minimum net benefit to switch (default: 0.05)

	// Cost weights (should sum to 1.0)
	weightTime        float64
	weightMemory      float64
	weightDegradation float64

	mu sync.RWMutex
}

// NewCostBenefitAnalyzer creates a new cost-benefit analyzer
func NewCostBenefitAnalyzer(maxMemory uint64) *CostBenefitAnalyzer {
	return &CostBenefitAnalyzer{
		maxMemory:       maxMemory,
		switchThreshold: 0.05, // 5% minimum net benefit

		// Default weights (from design section)
		weightTime:        0.4,
		weightMemory:      0.2,
		weightDegradation: 0.4,
	}
}

// EstimateSwitchingCost estimates the cost of switching from one policy to another
func (costBenefit *CostBenefitAnalyzer) EstimateSwitchingCost(fromPolicy, toPolicy string, cacheSize uint64) *SwitchingCost {
	costBenefit.mu.RLock()
	defer costBenefit.mu.RUnlock()

	// TIME COST (Reconstruction time)
	// Based on PaperCache paper: ~0.1s per MB
	cacheSizeMB := float64(cacheSize) / (1024.0 * 1024.0)
	estimatedSeconds := cacheSizeMB * 0.01

	// Normalize: 10 seconds = 1.0 (very high cost)
	// Most switches should be much faster
	timeCost := math.Min(estimatedSeconds/10.0, 1.0)

	// MEMORY COST (Temporary overhead during reconstruction)
	// MiniStack and dual metadata use ~10% of cache size
	overheadBytes := float64(cacheSize) * 0.1
	memoryCost := math.Min(overheadBytes/float64(costBenefit.maxMemory), 1.0)

	// DEGRADATION COST (Performance hit during switch)
	// From PaperCache paper: ~2% miss ratio increase during reconstruction
	degradationCost := 0.02

	// Calculate weighted total cost
	totalCost := (costBenefit.weightTime * timeCost) +
		(costBenefit.weightMemory * memoryCost) +
		(costBenefit.weightDegradation * degradationCost)

	return &SwitchingCost{
		TimeCost:        timeCost,
		MemoryCost:      memoryCost,
		DegradationCost: degradationCost,
		TotalCost:       totalCost,
	}
}

// CalculatePotentialGain calculates the relative improvement from switching
func (costBenefit *CostBenefitAnalyzer) CalculatePotentialGain(currentMissRatio, candidateMissRatio float64) float64 {
	if currentMissRatio == 0 {
		return 0 // No improvement possible if already perfect
	}

	if candidateMissRatio >= currentMissRatio {
		return 0 // No gain if candidate is worse or equal
	}

	// Calculate relative improvement
	improvement := (currentMissRatio - candidateMissRatio) / currentMissRatio

	return improvement
}

// CalculateNetBenefit calculates net benefit (gain - cost)
func (costBenefit *CostBenefitAnalyzer) CalculateNetBenefit(potentialGain float64, switchingCost *SwitchingCost) float64 {
	return potentialGain - switchingCost.TotalCost
}

// Analyze performs complete cost-benefit analysis
func (costBenefit *CostBenefitAnalyzer) Analyze(
	currentPolicy string,
	candidatePolicy string,
	currentMissRatio float64,
	candidateMissRatio float64,
	cacheSize uint64,
) *CostBenefitAnalysis {

	// Calculate potential gain
	potentialGain := costBenefit.CalculatePotentialGain(currentMissRatio, candidateMissRatio)

	// Estimate switching cost
	switchingCost := costBenefit.EstimateSwitchingCost(currentPolicy, candidatePolicy, cacheSize)

	// Calculate net benefit
	netBenefit := costBenefit.CalculateNetBenefit(potentialGain, switchingCost)

	// Determine if should switch
	costBenefit.mu.RLock()
	shouldSwitch := netBenefit > costBenefit.switchThreshold
	costBenefit.mu.RUnlock()

	analysis := &CostBenefitAnalysis{
		CurrentPolicy:      currentPolicy,
		CandidatePolicy:    candidatePolicy,
		CurrentMissRatio:   currentMissRatio,
		CandidateMissRatio: candidateMissRatio,
		PotentialGain:      potentialGain,
		SwitchingCost:      switchingCost,
		NetBenefit:         netBenefit,
		ShouldSwitch:       shouldSwitch,
	}

	// Log analysis
	costBenefit.logAnalysis(analysis)

	return analysis
}

// logAnalysis logs the cost-benefit analysis details
func (costBenefit *CostBenefitAnalyzer) logAnalysis(analysis *CostBenefitAnalysis) {
	log.Printf("[CostBenefitAnalyzer] Evaluating switch: %s → %s",
		analysis.CurrentPolicy, analysis.CandidatePolicy)
	log.Printf("Current miss ratio: %.4f", analysis.CurrentMissRatio)
	log.Printf("Candidate miss ratio: %.4f", analysis.CandidateMissRatio)
	log.Printf("Potential gain: %.2f%%", analysis.PotentialGain*100)
	log.Printf("Switching costs:")
	log.Printf("Time cost: %.4f", analysis.SwitchingCost.TimeCost)
	log.Printf("Memory cost: %.4f", analysis.SwitchingCost.MemoryCost)
	log.Printf("Degradation cost: %.4f", analysis.SwitchingCost.DegradationCost)
	log.Printf("Total cost: %.4f (%.2f%%)", analysis.SwitchingCost.TotalCost,
		analysis.SwitchingCost.TotalCost*100)
	log.Printf("Net benefit: %.4f (%.2f%%)", analysis.NetBenefit, analysis.NetBenefit*100)

	if analysis.ShouldSwitch {
		log.Printf("RECOMMENDATION: SWITCH (net benefit > %.2f%%)",
			costBenefit.switchThreshold*100)
	} else {
		log.Printf("RECOMMENDATION: STAY (net benefit < %.2f%%)",
			costBenefit.switchThreshold*100)
	}
}

// SetThreshold updates the minimum net benefit threshold for switching
func (costBenefit *CostBenefitAnalyzer) SetThreshold(threshold float64) {
	costBenefit.mu.Lock()
	defer costBenefit.mu.Unlock()

	if threshold >= 0 && threshold <= 1.0 {
		costBenefit.switchThreshold = threshold
	}
}

// GetThreshold returns the current switching threshold
func (costBenefit *CostBenefitAnalyzer) GetThreshold() float64 {
	costBenefit.mu.RLock()
	defer costBenefit.mu.RUnlock()

	return costBenefit.switchThreshold
}

// SetWeights updates the cost component weights
func (costBenefit *CostBenefitAnalyzer) SetWeights(time, memory, degradation float64) error {
	// Validate that weights sum to approximately 1.0
	sum := time + memory + degradation
	if math.Abs(sum-1.0) > 0.01 {
		return fmt.Errorf("weights must sum to 1.0, got %.3f", sum)
	}

	costBenefit.mu.Lock()
	defer costBenefit.mu.Unlock()

	costBenefit.weightTime = time
	costBenefit.weightMemory = memory
	costBenefit.weightDegradation = degradation

	return nil
}

// GetWeights returns the current cost weights
func (costBenefit *CostBenefitAnalyzer) GetWeights() (time, memory, degradation float64) {
	costBenefit.mu.RLock()
	defer costBenefit.mu.RUnlock()

	return costBenefit.weightTime, costBenefit.weightMemory, costBenefit.weightDegradation
}

// EstimateReconstructionTime estimates the time needed to reconstruct policy
func (costBenefit *CostBenefitAnalyzer) EstimateReconstructionTime(cacheSize uint64) float64 {
	// Based on PaperCache paper: ~0.1s per MB
	cacheSizeMB := float64(cacheSize) / (1024.0 * 1024.0)
	return cacheSizeMB * 0.1
}

// EstimateMemoryOverhead estimates temporary memory overhead during switch
func (costBenefit *CostBenefitAnalyzer) EstimateMemoryOverhead(cacheSize uint64) uint64 {
	// MiniStack + dual metadata: ~10% of cache size
	return uint64(float64(cacheSize) * 0.1)
}

// PrintSummary logs a summary of cost-benefit parameters
func (costBenefit *CostBenefitAnalyzer) PrintSummary() {
	costBenefit.mu.RLock()
	defer costBenefit.mu.RUnlock()

	log.Println("CostBenefitAnalyzer Configuration")
	log.Printf("Max memory: %d bytes (%.2f MB)",
		costBenefit.maxMemory, float64(costBenefit.maxMemory)/(1024.0*1024.0))
	log.Printf("Switch threshold: %.2f%% net benefit", costBenefit.switchThreshold*100)
	log.Println("Cost weights:")
	log.Printf("Time: %.2f", costBenefit.weightTime)
	log.Printf("Memory: %.2f", costBenefit.weightMemory)
	log.Printf("Degradation: %.2f", costBenefit.weightDegradation)
	log.Println("=========================================")
}

// CompareMultipleCandidates evaluates multiple candidate policies and returns the best
func (costBenefit *CostBenefitAnalyzer) CompareMultipleCandidates(
	currentPolicy string,
	currentMissRatio float64,
	candidates map[string]float64, // policy -> miss ratio
	cacheSize uint64,
) *CostBenefitAnalysis {

	var bestAnalysis *CostBenefitAnalysis
	bestNetBenefit := -1.0

	for candidatePolicy, candidateMissRatio := range candidates {
		if candidatePolicy == currentPolicy {
			continue // Skip current policy
		}

		analysis := costBenefit.Analyze(
			currentPolicy,
			candidatePolicy,
			currentMissRatio,
			candidateMissRatio,
			cacheSize,
		)

		if analysis.NetBenefit > bestNetBenefit {
			bestNetBenefit = analysis.NetBenefit
			bestAnalysis = analysis
		}
	}

	if bestAnalysis != nil {
		log.Printf("[CostBenefitAnalyzer] Best candidate: %s (net benefit: %.2f%%)",
			bestAnalysis.CandidatePolicy, bestAnalysis.NetBenefit*100)
	} else {
		log.Println("[CostBenefitAnalyzer] No beneficial candidate found")
	}

	return bestAnalysis
}
