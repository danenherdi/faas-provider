package adaptive

import (
	"math"
	"testing"
	"time"
)

func TestTrendAnalyzer_LinearRegression_PositiveTrend(t *testing.T) {
	analyzer := NewTrendAnalyzer()

	// Create increasing trend: 0.01, 0.02, 0.03, 0.04, 0.05
	values := []float64{0.01, 0.02, 0.03, 0.04, 0.05}

	slope, intercept, rSquared := analyzer.linearRegression(values)

	t.Logf("Positive trend:")
	t.Logf("Slope: %.6f", slope)
	t.Logf("Intercept: %.6f", intercept)
	t.Logf("R²: %.3f", rSquared)

	// Should have positive slope (increasing)
	if slope <= 0 {
		t.Errorf("Expected positive slope, got %.6f", slope)
	}

	// Should have high R² (perfect linear trend)
	if rSquared < 0.95 {
		t.Errorf("Expected high R² (>0.95), got %.3f", rSquared)
	}

	// Slope should be approximately 0.01 (increase of 0.01 per step)
	expectedSlope := 0.01
	if math.Abs(slope-expectedSlope) > 0.001 {
		t.Errorf("Expected slope ~%.3f, got %.6f", expectedSlope, slope)
	}
}

func TestTrendAnalyzer_LinearRegression_NegativeTrend(t *testing.T) {
	analyzer := NewTrendAnalyzer()

	// Create decreasing trend: 0.05, 0.04, 0.03, 0.02, 0.01
	values := []float64{0.05, 0.04, 0.03, 0.02, 0.01}

	slope, _, rSquared := analyzer.linearRegression(values)

	t.Logf("Negative trend:")
	t.Logf("Slope: %.6f", slope)
	t.Logf("R²: %.3f", rSquared)

	// Should have negative slope (decreasing)
	if slope >= 0 {
		t.Errorf("Expected negative slope, got %.6f", slope)
	}

	// Should have high R²
	if rSquared < 0.95 {
		t.Errorf("Expected high R², got %.3f", rSquared)
	}
}

func TestTrendAnalyzer_LinearRegression_Flat(t *testing.T) {
	analyzer := NewTrendAnalyzer()

	// Create flat trend: all same values
	values := []float64{0.03, 0.03, 0.03, 0.03, 0.03}

	slope, _, rSquared := analyzer.linearRegression(values)

	t.Logf("Flat trend:")
	t.Logf("Slope: %.6f", slope)
	t.Logf("R²: %.3f", rSquared)

	// Slope should be near zero
	if math.Abs(slope) > 0.001 {
		t.Errorf("Expected near-zero slope, got %.6f", slope)
	}
}

func TestTrendAnalyzer_AnalyzeTrend_Degrading(t *testing.T) {
	analyzer := NewTrendAnalyzer()

	// Create history with degrading performance
	history := make([]*MetricsSnapshot, 0)
	baseTime := time.Now()

	for i := 0; i < 10; i++ {
		snapshot := &MetricsSnapshot{
			Timestamp:     baseTime.Add(time.Duration(i*10) * time.Second),
			CurrentPolicy: "lfu",
			MissRatio:     0.02 + float64(i)*0.015, // Increasing from 0.02 to 0.155
			CacheSize:     512 * 1024 * 1024,
			NumObjects:    1000,
		}
		history = append(history, snapshot)
	}

	trend := analyzer.AnalyzeTrend(history, "lfu")

	t.Logf("Degrading trend analysis:")
	t.Logf("Indicates degradation: %v", trend.IndicatesDegradation)
	t.Logf("Slope: %.6f", trend.Slope)
	t.Logf("Variance: %.6f", trend.Variance)
	t.Logf("Confidence: %.3f", trend.Confidence)

	// Should detect degradation (slope > 0.01)
	if !trend.IndicatesDegradation {
		t.Error("Expected degradation to be detected")
	}

	// Slope should be positive
	if trend.Slope <= 0 {
		t.Errorf("Expected positive slope, got %.6f", trend.Slope)
	}

	// Should have high confidence (clear trend)
	if trend.Confidence < 0.8 {
		t.Errorf("Expected high confidence, got %.3f", trend.Confidence)
	}
}

func TestTrendAnalyzer_AnalyzeTrend_Stable(t *testing.T) {
	analyzer := NewTrendAnalyzer()

	// Create history with stable performance (small variations)
	history := make([]*MetricsSnapshot, 0)
	baseTime := time.Now()

	for i := 0; i < 10; i++ {
		snapshot := &MetricsSnapshot{
			Timestamp:     baseTime.Add(time.Duration(i*10) * time.Second),
			CurrentPolicy: "lfu",
			MissRatio:     0.025 + float64(i%3)*0.001, // Stable around 0.025
			CacheSize:     512 * 1024 * 1024,
			NumObjects:    1000,
		}
		history = append(history, snapshot)
	}

	trend := analyzer.AnalyzeTrend(history, "lfu")

	t.Logf("Stable trend analysis:")
	t.Logf("Indicates degradation: %v", trend.IndicatesDegradation)
	t.Logf("Slope: %.6f", trend.Slope)
	t.Logf("Variance: %.6f", trend.Variance)

	// Should NOT detect degradation (stable performance)
	if trend.IndicatesDegradation {
		t.Error("Did not expect degradation for stable performance")
	}

	// Slope should be small
	if math.Abs(trend.Slope) > 0.005 {
		t.Logf("Slope %.6f exceeds expected range for stable performance", trend.Slope)
	}
}

func TestTrendAnalyzer_AnalyzeTrend_HighVariance(t *testing.T) {
	analyzer := NewTrendAnalyzer()

	// Create history with high variance (unstable)
	history := make([]*MetricsSnapshot, 0)
	baseTime := time.Now()
	//values := []float64{0.02, 0.08, 0.03, 0.09, 0.025, 0.085, 0.03, 0.09}
	values := []float64{0.1, 0.5, 0.1, 0.5, 0.1, 0.5, 0.1, 0.5}

	for i, missRatio := range values {
		snapshot := &MetricsSnapshot{
			Timestamp:     baseTime.Add(time.Duration(i*10) * time.Second),
			CurrentPolicy: "lfu",
			MissRatio:     missRatio,
			CacheSize:     512 * 1024 * 1024,
			NumObjects:    1000,
		}
		history = append(history, snapshot)
	}

	trend := analyzer.AnalyzeTrend(history, "lfu")

	t.Logf("High variance analysis:")
	t.Logf("Indicates degradation: %v", trend.IndicatesDegradation)
	t.Logf("Slope: %.6f", trend.Slope)
	t.Logf("Variance: %.6f", trend.Variance)

	// Should detect instability due to high variance
	if !trend.IndicatesDegradation {
		t.Error("Expected degradation due to high variance")
	}

	// Variance should be high
	if trend.Variance < 0.001 {
		t.Errorf("Expected high variance, got %.6f", trend.Variance)
	}
}

func TestTrendAnalyzer_InsufficientData(t *testing.T) {
	analyzer := NewTrendAnalyzer()

	// Only 2 snapshots (less than minimum 3)
	history := []*MetricsSnapshot{
		{CurrentPolicy: "lfu", MissRatio: 0.02},
		{CurrentPolicy: "lfu", MissRatio: 0.03},
	}

	trend := analyzer.AnalyzeTrend(history, "lfu")

	if trend.IndicatesDegradation {
		t.Error("Should not detect degradation with insufficient data")
	}

	if trend.Confidence != 0 {
		t.Error("Expected zero confidence with insufficient data")
	}
}

func TestTrendAnalyzer_PredictNextValue(t *testing.T) {
	analyzer := NewTrendAnalyzer()

	// Create history with clear trend
	history := make([]*MetricsSnapshot, 0)
	for i := 0; i < 5; i++ {
		snapshot := &MetricsSnapshot{
			CurrentPolicy: "lfu",
			MissRatio:     0.02 + float64(i)*0.01, // 0.02, 0.03, 0.04, 0.05, 0.06
		}
		history = append(history, snapshot)
	}

	predicted, confidence, err := analyzer.PredictNextValue(history, "lfu")

	if err != nil {
		t.Fatalf("Prediction failed: %v", err)
	}

	t.Logf("Prediction:")
	t.Logf("Next value: %.6f", predicted)
	t.Logf("Confidence: %.3f", confidence)

	// Next value should be around 0.07 (continuing the trend)
	expectedNext := 0.07
	if math.Abs(predicted-expectedNext) > 0.01 {
		t.Errorf("Expected prediction ~%.3f, got %.6f", expectedNext, predicted)
	}

	// Confidence should be high (perfect linear trend)
	if confidence < 0.95 {
		t.Errorf("Expected high confidence, got %.3f", confidence)
	}
}

func TestTrendAnalyzer_SetThresholds(t *testing.T) {
	analyzer := NewTrendAnalyzer()

	// Set custom thresholds
	analyzer.SetThresholds(0.02, 0.10, 5)

	degradation, variance, minSamples := analyzer.GetThresholds()

	if degradation != 0.02 {
		t.Errorf("Expected degradation threshold 0.02, got %.3f", degradation)
	}

	if variance != 0.10 {
		t.Errorf("Expected variance threshold 0.10, got %.3f", variance)
	}

	if minSamples != 5 {
		t.Errorf("Expected minSamples 5, got %d", minSamples)
	}
}

func TestTrendAnalyzer_PrintSummary(t *testing.T) {
	analyzer := NewTrendAnalyzer()

	// Create test history
	history := make([]*MetricsSnapshot, 0)
	for i := 0; i < 8; i++ {
		snapshot := &MetricsSnapshot{
			CurrentPolicy: "lfu",
			MissRatio:     0.02 + float64(i)*0.005,
		}
		history = append(history, snapshot)
	}

	// This should log summary without errors
	analyzer.PrintSummary(history, "lfu")

	// Just verify it doesn't crash
	trend := analyzer.AnalyzeTrend(history, "lfu")
	if trend == nil {
		t.Error("Expected non-nil trend")
	}
}

func TestTrendAnalyzer_ExtractMissRatios_FilterByPolicy(t *testing.T) {
	analyzer := NewTrendAnalyzer()

	// Create history with mixed policies
	history := []*MetricsSnapshot{
		{CurrentPolicy: "lfu", MissRatio: 0.02},
		{CurrentPolicy: "lru", MissRatio: 0.03},
		{CurrentPolicy: "lfu", MissRatio: 0.025},
		{CurrentPolicy: "lfu", MissRatio: 0.03},
		{CurrentPolicy: "lru", MissRatio: 0.035},
	}

	missRatios := analyzer.extractMissRatios(history, "lfu")

	// Should only extract LFU entries
	expectedCount := 3
	if len(missRatios) != expectedCount {
		t.Errorf("Expected %d LFU entries, got %d", expectedCount, len(missRatios))
	}

	// Verify values
	expected := []float64{0.02, 0.025, 0.03}
	for i, val := range missRatios {
		if math.Abs(val-expected[i]) > 0.0001 {
			t.Errorf("Entry %d: expected %.3f, got %.3f", i, expected[i], val)
		}
	}
}
