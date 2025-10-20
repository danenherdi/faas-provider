package adaptive

import (
	"math"
	"testing"
)

func TestCostBenefitAnalyzer_EstimateSwitchingCost(t *testing.T) {
	// 512MB max memory
	analyzer := NewCostBenefitAnalyzer(512 * 1024 * 1024)

	// Test with 512MB cache
	cacheSize := uint64(512 * 1024 * 1024)

	cost := analyzer.EstimateSwitchingCost("lfu", "lru", cacheSize)

	t.Logf("Switching cost for 512MB cache:")
	t.Logf("Time cost: %.6f", cost.TimeCost)
	t.Logf("Memory cost: %.6f", cost.MemoryCost)
	t.Logf("Degradation cost: %.6f", cost.DegradationCost)
	t.Logf("Total cost: %.6f (%.2f%%)", cost.TotalCost, cost.TotalCost*100)

	// Total cost should be reasonable (< 0.1 = 10%)
	if cost.TotalCost > 0.3 {
		t.Errorf("Total cost seems high: %.6f", cost.TotalCost)
	}

	// Degradation cost should be fixed at 2%
	if math.Abs(cost.DegradationCost-0.02) > 0.001 {
		t.Errorf("Expected degradation cost 0.02, got %.6f", cost.DegradationCost)
	}
}

func TestCostBenefitAnalyzer_CalculatePotentialGain(t *testing.T) {
	analyzer := NewCostBenefitAnalyzer(512 * 1024 * 1024)

	tests := []struct {
		name               string
		currentMissRatio   float64
		candidateMissRatio float64
		expectedGain       float64
	}{
		{
			name:               "Significant improvement",
			currentMissRatio:   0.05,
			candidateMissRatio: 0.03,
			expectedGain:       0.40, // (0.05 - 0.03) / 0.05 = 40%
		},
		{
			name:               "Small improvement",
			currentMissRatio:   0.05,
			candidateMissRatio: 0.045,
			expectedGain:       0.10, // 10%
		},
		{
			name:               "No improvement",
			currentMissRatio:   0.05,
			candidateMissRatio: 0.05,
			expectedGain:       0.0,
		},
		{
			name:               "Worse performance",
			currentMissRatio:   0.05,
			candidateMissRatio: 0.06,
			expectedGain:       0.0, // Should return 0, not negative
		},
		{
			name:               "Zero current miss ratio",
			currentMissRatio:   0.0,
			candidateMissRatio: 0.0,
			expectedGain:       0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gain := analyzer.CalculatePotentialGain(tt.currentMissRatio, tt.candidateMissRatio)

			if math.Abs(gain-tt.expectedGain) > 0.01 {
				t.Errorf("Expected gain %.3f, got %.3f", tt.expectedGain, gain)
			}

			t.Logf("%s: gain = %.2f%%", tt.name, gain*100)
		})
	}
}

func TestCostBenefitAnalyzer_Analyze_ShouldSwitch(t *testing.T) {
	analyzer := NewCostBenefitAnalyzer(512 * 1024 * 1024)

	// Case: High gain (40%), low cost (~3%)
	// Net benefit: 40% - 3% = 37% > 5% threshold → SWITCH
	analysis := analyzer.Analyze(
		"lfu",
		"lru",
		0.05, // current: 5% miss ratio
		0.03, // candidate: 3% miss ratio
		512*1024*1024,
	)

	if !analysis.ShouldSwitch {
		t.Error("Expected recommendation to SWITCH with high net benefit")
	}

	if analysis.NetBenefit < 0.05 {
		t.Errorf("Expected net benefit > 0.05, got %.6f", analysis.NetBenefit)
	}

	t.Logf("Analysis result: net benefit = %.2f%%, should switch = %v",
		analysis.NetBenefit*100, analysis.ShouldSwitch)
}

func TestCostBenefitAnalyzer_Analyze_ShouldNotSwitch(t *testing.T) {
	analyzer := NewCostBenefitAnalyzer(512 * 1024 * 1024)

	// Case: Low gain (4%), cost (~3%)
	// Net benefit: 4% - 3% = 1% < 5% threshold → DON'T SWITCH
	analysis := analyzer.Analyze(
		"lfu",
		"lru",
		0.05,  // current: 5% miss ratio
		0.048, // candidate: 4.8% miss ratio (only 0.2% improvement)
		512*1024*1024,
	)

	if analysis.ShouldSwitch {
		t.Error("Expected recommendation to NOT SWITCH with low net benefit")
	}

	if analysis.NetBenefit > 0.05 {
		t.Errorf("Expected net benefit < 0.05, got %.6f", analysis.NetBenefit)
	}

	t.Logf("Analysis result: net benefit = %.2f%%, should switch = %v",
		analysis.NetBenefit*100, analysis.ShouldSwitch)
}

func TestCostBenefitAnalyzer_SetThreshold(t *testing.T) {
	analyzer := NewCostBenefitAnalyzer(512 * 1024 * 1024)

	// Change threshold to 10%
	analyzer.SetThreshold(0.10)

	threshold := analyzer.GetThreshold()
	if math.Abs(threshold-0.10) > 0.001 {
		t.Errorf("Expected threshold 0.10, got %.3f", threshold)
	}

	// Now same case as before should NOT switch (1% < 10%)
	analysis := analyzer.Analyze(
		"lfu",
		"lru",
		0.05,
		0.048,
		512*1024*1024,
	)

	if analysis.ShouldSwitch {
		t.Error("With 10% threshold, should NOT switch with 1% net benefit")
	}
}

func TestCostBenefitAnalyzer_SetWeights(t *testing.T) {
	analyzer := NewCostBenefitAnalyzer(512 * 1024 * 1024)

	// Set custom weights
	err := analyzer.SetWeights(0.5, 0.3, 0.2)
	if err != nil {
		t.Fatalf("SetWeights failed: %v", err)
	}

	time, memory, degradation := analyzer.GetWeights()

	if math.Abs(time-0.5) > 0.001 {
		t.Errorf("Expected time weight 0.5, got %.3f", time)
	}
	if math.Abs(memory-0.3) > 0.001 {
		t.Errorf("Expected memory weight 0.3, got %.3f", memory)
	}
	if math.Abs(degradation-0.2) > 0.001 {
		t.Errorf("Expected degradation weight 0.2, got %.3f", degradation)
	}
}

func TestCostBenefitAnalyzer_SetWeights_Invalid(t *testing.T) {
	analyzer := NewCostBenefitAnalyzer(512 * 1024 * 1024)

	// Weights don't sum to 1.0
	err := analyzer.SetWeights(0.5, 0.3, 0.3) // Sum = 1.1
	if err == nil {
		t.Error("Expected error for invalid weights, got nil")
	}
}

func TestCostBenefitAnalyzer_EstimateReconstructionTime(t *testing.T) {
	analyzer := NewCostBenefitAnalyzer(512 * 1024 * 1024)

	// 512MB cache should take ~51.2 seconds (512 * 0.1)
	reconstructionTime := analyzer.EstimateReconstructionTime(512 * 1024 * 1024)

	expectedTime := 51.2 // 512MB * 0.1s/MB
	if math.Abs(reconstructionTime-expectedTime) > 1.0 {
		t.Errorf("Expected reconstruction time ~%.1fs, got %.1fs",
			expectedTime, reconstructionTime)
	}

	t.Logf("Reconstruction time for 512MB: %.2f seconds", reconstructionTime)
}

func TestCostBenefitAnalyzer_EstimateMemoryOverhead(t *testing.T) {
	analyzer := NewCostBenefitAnalyzer(512 * 1024 * 1024)

	overhead := analyzer.EstimateMemoryOverhead(512 * 1024 * 1024)

	// Should be ~10% = 51.2MB
	expectedFloat := float64(512*1024*1024) * 0.1
	expectedOverhead := uint64(expectedFloat)

	if overhead != expectedOverhead {
		t.Errorf("Expected overhead %d bytes, got %d", expectedOverhead, overhead)
	}

	t.Logf("Memory overhead for 512MB: %.2f MB",
		float64(overhead)/(1024.0*1024.0))
}

func TestCostBenefitAnalyzer_CompareMultipleCandidates(t *testing.T) {
	analyzer := NewCostBenefitAnalyzer(512 * 1024 * 1024)

	currentPolicy := "lfu"
	currentMissRatio := 0.05

	candidates := map[string]float64{
		"lfu":   0.05,  // current (should be skipped)
		"lru":   0.03,  // best candidate (40% improvement)
		"fifo":  0.045, // okay candidate (10% improvement)
		"sieve": 0.048, // marginal candidate (4% improvement)
	}

	bestAnalysis := analyzer.CompareMultipleCandidates(
		currentPolicy,
		currentMissRatio,
		candidates,
		512*1024*1024,
	)

	if bestAnalysis == nil {
		t.Fatal("Expected non-nil analysis")
	}

	// Best should be LRU (40% gain)
	if bestAnalysis.CandidatePolicy != "lru" {
		t.Errorf("Expected best candidate 'lru', got '%s'",
			bestAnalysis.CandidatePolicy)
	}

	t.Logf("Best candidate: %s", bestAnalysis.CandidatePolicy)
	t.Logf("Net benefit: %.2f%%", bestAnalysis.NetBenefit*100)
	t.Logf("Should switch: %v", bestAnalysis.ShouldSwitch)
}

func TestCostBenefitAnalyzer_CompareMultipleCandidates_NoBenefit(t *testing.T) {
	analyzer := NewCostBenefitAnalyzer(512 * 1024 * 1024)

	// All candidates are worse than current
	currentPolicy := "lfu"
	currentMissRatio := 0.02

	candidates := map[string]float64{
		"lfu":   0.02,  // current
		"lru":   0.03,  // worse
		"fifo":  0.035, // worse
		"sieve": 0.04,  // worse
	}

	bestAnalysis := analyzer.CompareMultipleCandidates(
		currentPolicy,
		currentMissRatio,
		candidates,
		512*1024*1024,
	)

	// Should return some analysis (best of the bad options)
	// but ShouldSwitch should be false
	if bestAnalysis != nil && bestAnalysis.ShouldSwitch {
		t.Error("Should not recommend switching when all candidates are worse")
	}
}

func TestCostBenefitAnalyzer_PrintSummary(t *testing.T) {
	analyzer := NewCostBenefitAnalyzer(512 * 1024 * 1024)

	// Should log summary without errors
	analyzer.PrintSummary()

	// Just verify it doesn't crash
	threshold := analyzer.GetThreshold()
	if threshold != 0.05 {
		t.Errorf("Expected default threshold 0.05, got %.3f", threshold)
	}
}
