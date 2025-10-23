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
	Confidence           float64 // R^2 value (0-1, goodness of fit)
	SampleSize           int     // Number of snapshots analyzed
}

// TrendAnalyzer predicts performance trends using linear regression
type TrendAnalyzer struct {
	// Configuration
	degradationThreshold float64 // Slope threshold for degradation (default: 0.01 = 1% per snapshot)
	varianceThreshold    float64 // Variance threshold for instability (default: 0.05 = 5% variance)
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
func (trendAnalyzer *TrendAnalyzer) AnalyzeTrend(history []*MetricsSnapshot, policyName string) *TrendScore {
	trendAnalyzer.mu.RLock()
	defer trendAnalyzer.mu.RUnlock()

	// Check minimum sample requirement
	if len(history) < trendAnalyzer.minSamples {
		return &TrendScore{
			IndicatesDegradation: false,
			Confidence:           0.0,
			SampleSize:           len(history),
		}
	}

	// Extract miss ratios for the specified policy
	missRatios := trendAnalyzer.extractMissRatios(history, policyName)

	if len(missRatios) < trendAnalyzer.minSamples {
		return &TrendScore{
			IndicatesDegradation: false,
			Confidence:           0.0,
			SampleSize:           len(missRatios),
		}
	}

	// Perform linear regression
	slope, intercept, rSquared := trendAnalyzer.linearRegression(missRatios)

	// Calculate variance (stability measure)
	variance := trendAnalyzer.calculateVariance(missRatios)

	// Determine degradation
	degrading := slope > trendAnalyzer.degradationThreshold
	unstable := variance > trendAnalyzer.varianceThreshold

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
func (trendAnalyzer *TrendAnalyzer) extractMissRatios(history []*MetricsSnapshot, policyName string) []float64 {
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
// Returns: slope, intercept, R^2 (coefficient of determination)
func (trendAnalyzer *TrendAnalyzer) linearRegression(values []float64) (slope, intercept, rSquared float64) {
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
	// slope = (n×ΣXY - ΣX×ΣY) / (n×ΣX^2 - (ΣX)^2)
	numerator := n*sumXY - sumX*sumY
	denominator := n*sumX2 - sumX*sumX

	if denominator == 0 {
		return 0, meanY, 0
	}

	slope = numerator / denominator
	intercept = meanY - slope*meanX

	// Calculate R^2 (goodness of fit)
	rSquared = trendAnalyzer.calculateRSquared(values, slope, intercept)

	return slope, intercept, rSquared
}

// calculateRSquared calculates the coefficient of determination (R²)
// R^2 = 1 - (SS_res / SS_tot)
// R^2 closer to 1 means better fit
func (trendAnalyzer *TrendAnalyzer) calculateRSquared(values []float64, slope, intercept float64) float64 {
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

	// Iterate over data points to compute SS_tot and SS_res
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

	// R^2 can be negative if model is worse than mean
	// Clamp to 0 for interpretability
	if rSquared < 0 {
		rSquared = 0
	}

	return rSquared
}

// calculateVariance calculates the variance of miss ratios
func (trendAnalyzer *TrendAnalyzer) calculateVariance(values []float64) float64 {
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
func (trendAnalyzer *TrendAnalyzer) PredictNextValue(history []*MetricsSnapshot, policyName string) (predicted float64, confidence float64, err error) {
	trendAnalyzer.mu.RLock()
	defer trendAnalyzer.mu.RUnlock()

	missRatios := trendAnalyzer.extractMissRatios(history, policyName)

	if len(missRatios) < trendAnalyzer.minSamples {
		return 0, 0, fmt.Errorf("insufficient data: need at least %d samples, got %d",
			trendAnalyzer.minSamples, len(missRatios))
	}

	slope, intercept, rSquared := trendAnalyzer.linearRegression(missRatios)

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
func (trendAnalyzer *TrendAnalyzer) SetThresholds(degradationThreshold, varianceThreshold float64, minSamples int) {
	trendAnalyzer.mu.Lock()
	defer trendAnalyzer.mu.Unlock()

	if degradationThreshold > 0 {
		trendAnalyzer.degradationThreshold = degradationThreshold
	}

	if varianceThreshold > 0 {
		trendAnalyzer.varianceThreshold = varianceThreshold
	}

	if minSamples > 0 {
		trendAnalyzer.minSamples = minSamples
	}
}

// GetThresholds returns current threshold values
func (trendAnalyzer *TrendAnalyzer) GetThresholds() (degradation, variance float64, minSamples int) {
	trendAnalyzer.mu.RLock()
	defer trendAnalyzer.mu.RUnlock()

	return trendAnalyzer.degradationThreshold, trendAnalyzer.varianceThreshold, trendAnalyzer.minSamples
}

// PrintSummary logs trend analysis summary for a policy
func (trendAnalyzer *TrendAnalyzer) PrintSummary(history []*MetricsSnapshot, policyName string) {
	trend := trendAnalyzer.AnalyzeTrend(history, policyName)

	log.Println("TrendAnalyzer Summary")
	log.Printf("Policy: %s", policyName)
	log.Printf("Sample size: %d", trend.SampleSize)
	log.Printf("Slope: %.6f (change per snapshot)", trend.Slope)
	log.Printf("Variance: %.6f", trend.Variance)
	log.Printf("Confidence (R²): %.3f", trend.Confidence)
	log.Printf("Indicates degradation: %v", trend.IndicatesDegradation)

	// Interpretation
	if trend.IndicatesDegradation {
		if trend.Slope > trendAnalyzer.degradationThreshold {
			log.Printf("Degrading trend detected (slope > %.3f)", trendAnalyzer.degradationThreshold)
			log.Printf("Projected increase: %.2f%% per snapshot",
				trend.Slope*100)
		}
		if trend.Variance > trendAnalyzer.varianceThreshold {
			log.Printf("High variance detected (%.6f > %.3f)",
				trend.Variance, trendAnalyzer.varianceThreshold)
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
func (trendAnalyzer *TrendAnalyzer) CalculateStandardDeviation(variance float64) float64 {
	return math.Sqrt(variance)
}
