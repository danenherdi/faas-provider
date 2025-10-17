package adaptive

import (
	"fmt"
	"log"
	"math"
	"sync"
)

// TrendScore represents performance trend analysis results
type TrendScore struct {
	IndicatesDegradation bool    // True if performance is degrading
	Slope                float64 // Miss ratio change rate (per snapshot)
	Variance             float64 // Miss ratio variance (stability measure)
	Intercept            float64 // Starting miss ratio
	Confidence           float64 // R² value (0-1, goodness of fit)
	SampleSize           int     // Number of snapshots analyzed
}

// TrendAnalyzer predicts performance trends using linear regression
type TrendAnalyzer struct {
	// Configuration
	degradationThreshold float64 // Slope threshold for degradation (default: 0.01)
	varianceThreshold    float64 // Variance threshold for instability (default: 0.05)
	minSamples           int     // Minimum samples for analysis (default: 3)

	mu sync.RWMutex
}

// NewTrendAnalyzer creates a new trend analyzer
func NewTrendAnalyzer() *TrendAnalyzer {
	return &TrendAnalyzer{
		degradationThreshold: 0.01, // 1% miss ratio increase per snapshot
		varianceThreshold:    0.05, // 5% variance threshold
		minSamples:           3,    // Need at least 3 points for regression
	}
}

// AnalyzeTrend analyzes miss ratio trend for a specific policy
func (ta *TrendAnalyzer) AnalyzeTrend(history []*MetricsSnapshot, policyName string) *TrendScore {
	ta.mu.RLock()
	defer ta.mu.RUnlock()

	// Check minimum sample requirement
	if len(history) < ta.minSamples {
		return &TrendScore{
			IndicatesDegradation: false,
			Confidence:           0.0,
			SampleSize:           len(history),
		}
	}

	// Extract miss ratios for the specified policy
	missRatios := ta.extractMissRatios(history, policyName)

	if len(missRatios) < ta.minSamples {
		return &TrendScore{
			IndicatesDegradation: false,
			Confidence:           0.0,
			SampleSize:           len(missRatios),
		}
	}

	// Perform linear regression
	slope, intercept, rSquared := ta.linearRegression(missRatios)

	// Calculate variance (stability measure)
	variance := ta.calculateVariance(missRatios)

	// Determine degradation
	degrading := slope > ta.degradationThreshold
	unstable := variance > ta.varianceThreshold

	indicatesDegradation := degrading || unstable

	if indicatesDegradation {
		log.Printf("[TrendAnalyzer] Degradation detected for %s: slope=%.6f, variance=%.6f, R²=%.3f",
			policyName, slope, variance, rSquared)
	}

	return &TrendScore{
		IndicatesDegradation: indicatesDegradation,
		Slope:                slope,
		Variance:             variance,
		Confidence:           rSquared,
		SampleSize:           len(missRatios),
		Intercept:            intercept,
	}
}

// extractMissRatios extracts miss ratio values from history snapshots
func (ta *TrendAnalyzer) extractMissRatios(history []*MetricsSnapshot, policyName string) []float64 {
	missRatios := make([]float64, 0, len(history))

	for _, snapshot := range history {
		// Only include snapshots where this policy was active
		if snapshot.CurrentPolicy == policyName {
			missRatios = append(missRatios, snapshot.MissRatio)
		}
	}

	return missRatios
}

// linearRegression performs simple linear regression on the data
// Returns: slope, intercept, R² (coefficient of determination)
func (ta *TrendAnalyzer) linearRegression(values []float64) (slope, intercept, rSquared float64) {
	n := float64(len(values))

	if n < 2 {
		return 0, 0, 0
	}

	// Calculate sums for regression
	var sumX, sumY, sumXY, sumX2 float64

	for i, y := range values {
		x := float64(i) // Time index (0, 1, 2, ...)
		sumX += x
		sumY += y
		sumXY += x * y
		sumX2 += x * x
	}

	// Calculate means
	meanX := sumX / n
	meanY := sumY / n

	// Calculate slope and intercept
	// slope = (n×ΣXY - ΣX×ΣY) / (n×ΣX² - (ΣX)²)
	numerator := n*sumXY - sumX*sumY
	denominator := n*sumX2 - sumX*sumX

	if denominator == 0 {
		return 0, meanY, 0
	}

	slope = numerator / denominator
	intercept = meanY - slope*meanX

	// Calculate R² (goodness of fit)
	rSquared = ta.calculateRSquared(values, slope, intercept)

	return slope, intercept, rSquared
}

// calculateRSquared calculates the coefficient of determination (R²)
// R² = 1 - (SS_res / SS_tot)
// R² closer to 1 means better fit
func (ta *TrendAnalyzer) calculateRSquared(values []float64, slope, intercept float64) float64 {
	n := len(values)

	if n == 0 {
		return 0
	}

	// Calculate mean
	var mean float64
	for _, y := range values {
		mean += y
	}
	mean /= float64(n)

	// Calculate total sum of squares (SS_tot) and residual sum of squares (SS_res)
	var ssTot, ssRes float64

	for i, y := range values {
		x := float64(i)
		predicted := slope*x + intercept

		ssTot += (y - mean) * (y - mean)
		ssRes += (y - predicted) * (y - predicted)
	}

	if ssTot == 0 {
		return 0
	}

	rSquared := 1 - (ssRes / ssTot)

	// R² can be negative if model is worse than mean
	// Clamp to 0 for interpretability
	if rSquared < 0 {
		rSquared = 0
	}

	return rSquared
}

// calculateVariance calculates the variance of miss ratios
func (ta *TrendAnalyzer) calculateVariance(values []float64) float64 {
	n := len(values)

	if n == 0 {
		return 0
	}

	// Calculate mean
	var mean float64
	for _, v := range values {
		mean += v
	}
	mean /= float64(n)

	// Calculate variance
	var variance float64
	for _, v := range values {
		diff := v - mean
		variance += diff * diff
	}
	variance /= float64(n)

	return variance
}

// PredictNextValue predicts the next miss ratio value based on trend
func (ta *TrendAnalyzer) PredictNextValue(history []*MetricsSnapshot, policyName string) (predicted float64, confidence float64, err error) {
	ta.mu.RLock()
	defer ta.mu.RUnlock()

	missRatios := ta.extractMissRatios(history, policyName)

	if len(missRatios) < ta.minSamples {
		return 0, 0, fmt.Errorf("insufficient data: need at least %d samples, got %d",
			ta.minSamples, len(missRatios))
	}

	slope, intercept, rSquared := ta.linearRegression(missRatios)

	// Predict next value (x = len(missRatios))
	nextX := float64(len(missRatios))
	predicted = slope*nextX + intercept

	// Clamp prediction to valid range [0, 1]
	if predicted < 0 {
		predicted = 0
	} else if predicted > 1.0 {
		predicted = 1.0
	}

	return predicted, rSquared, nil
}

// SetThresholds allows customization of analysis thresholds
func (ta *TrendAnalyzer) SetThresholds(degradationThreshold, varianceThreshold float64, minSamples int) {
	ta.mu.Lock()
	defer ta.mu.Unlock()

	if degradationThreshold > 0 {
		ta.degradationThreshold = degradationThreshold
	}

	if varianceThreshold > 0 {
		ta.varianceThreshold = varianceThreshold
	}

	if minSamples > 0 {
		ta.minSamples = minSamples
	}
}

// GetThresholds returns current threshold values
func (ta *TrendAnalyzer) GetThresholds() (degradation, variance float64, minSamples int) {
	ta.mu.RLock()
	defer ta.mu.RUnlock()

	return ta.degradationThreshold, ta.varianceThreshold, ta.minSamples
}

// PrintSummary logs trend analysis summary for a policy
func (ta *TrendAnalyzer) PrintSummary(history []*MetricsSnapshot, policyName string) {
	trend := ta.AnalyzeTrend(history, policyName)

	log.Println("TrendAnalyzer Summary")
	log.Printf("Policy: %s", policyName)
	log.Printf("Sample size: %d", trend.SampleSize)
	log.Printf("Slope: %.6f (change per snapshot)", trend.Slope)
	log.Printf("Variance: %.6f", trend.Variance)
	log.Printf("Confidence (R²): %.3f", trend.Confidence)
	log.Printf("Indicates degradation: %v", trend.IndicatesDegradation)

	// Interpretation
	if trend.IndicatesDegradation {
		if trend.Slope > ta.degradationThreshold {
			log.Printf("Degrading trend detected (slope > %.3f)", ta.degradationThreshold)
			log.Printf("Projected increase: %.2f%% per snapshot",
				trend.Slope*100)
		}
		if trend.Variance > ta.varianceThreshold {
			log.Printf("High variance detected (%.6f > %.3f)",
				trend.Variance, ta.varianceThreshold)
			log.Println("Performance is unstable")
		}
	} else {
		log.Println("Performance is stable")
	}

	if trend.Confidence < 0.5 {
		log.Println("Low confidence (R² < 0.5) - trend may not be reliable")
	}

	log.Println("=============================")
}

// CalculateStandardDeviation calculates standard deviation from variance
func (ta *TrendAnalyzer) CalculateStandardDeviation(variance float64) float64 {
	return math.Sqrt(variance)
}
