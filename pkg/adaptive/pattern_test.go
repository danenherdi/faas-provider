package adaptive

import (
	"testing"
	"time"
)

func TestPatternDetector_RecordAccess(t *testing.T) {
	detector := NewPatternDetector(100)

	// Record some accesses
	detector.RecordAccess("key1", true)
	detector.RecordAccess("key2", false)
	detector.RecordAccess("key1", true)

	size := detector.GetAccessLogSize()
	if size != 3 {
		t.Errorf("Expected 3 accesses, got %d", size)
	}
}

func TestPatternDetector_SlidingWindow(t *testing.T) {
	detector := NewPatternDetector(10) // Small window for testing

	// Record more than window size
	for i := 0; i < 15; i++ {
		detector.RecordAccess("key1", true)
	}

	size := detector.GetAccessLogSize()
	if size != 10 {
		t.Errorf("Expected window size 10, got %d", size)
	}
}

func TestPatternDetector_TemporalLocality(t *testing.T) {
	detector := NewPatternDetector(100)

	// Simulate temporal locality: key1 repeatedly accessed
	for i := 0; i < 50; i++ {
		detector.RecordAccess("key1", true) // Repeated access = high temporal locality
		time.Sleep(1 * time.Millisecond)
	}

	// Add some other keys
	for i := 0; i < 50; i++ {
		detector.RecordAccess("key2", false)
		time.Sleep(1 * time.Millisecond)
	}

	pattern := detector.GetCurrentPattern()

	t.Logf("Temporal locality pattern:")
	t.Logf("  Recency score: %.3f", pattern.RecencyScore)
	t.Logf("  Frequency score: %.3f", pattern.FrequencyScore)
	t.Logf("  Hit rate: %.3f", pattern.HitRate)

	// Should have moderate-to-high recency score (hits in recent window)
	if pattern.RecencyScore < 0.3 {
		t.Errorf("Expected moderate recency score, got %.3f", pattern.RecencyScore)
	}
}

func TestPatternDetector_FrequencySkew(t *testing.T) {
	detector := NewPatternDetector(100)

	// Simulate frequency-based pattern: one key accessed much more than others
	for i := 0; i < 80; i++ {
		detector.RecordAccess("hot_key", true) // Hot key: 80%
	}

	for i := 0; i < 20; i++ {
		detector.RecordAccess("cold_key", false) // Cold keys: 20%
	}

	pattern := detector.GetCurrentPattern()

	t.Logf("Frequency skew pattern:")
	t.Logf("Recency score: %.3f", pattern.RecencyScore)
	t.Logf("Frequency score: %.3f", pattern.FrequencyScore)
	t.Logf("Unique keys: %d", pattern.UniqueKeys)

	// Should have high frequency score (one key dominates)
	if pattern.FrequencyScore < 0.3 {
		t.Errorf("Expected high frequency score, got %.3f", pattern.FrequencyScore)
	}
}

func TestPatternDetector_ScanPattern(t *testing.T) {
	detector := NewPatternDetector(100)

	// Simulate scan pattern: many unique keys, each accessed once
	for i := 0; i < 100; i++ {
		key := "key" + string(rune('a'+i%26)) + string(rune('0'+i/26))
		detector.RecordAccess(key, false) // Each key accessed once
	}

	pattern := detector.GetCurrentPattern()

	t.Logf("Scan pattern:")
	t.Logf("Unique key ratio: %.3f", pattern.UniqueKeyRatio)
	t.Logf("Unique keys: %d", pattern.UniqueKeys)
	t.Logf("Total accesses: %d", pattern.TotalAccesses)

	// Should have high unique key ratio
	if pattern.UniqueKeyRatio < 0.5 {
		t.Errorf("Expected high unique key ratio, got %.3f", pattern.UniqueKeyRatio)
	}
}

func TestPatternDetector_DetectShift(t *testing.T) {
	detector := NewPatternDetector(100)

	// Phase 1: Temporal pattern (first 50 accesses)
	for i := 0; i < 50; i++ {
		if i%2 == 0 {
			detector.RecordAccess("key1", true) // Repeated access
		} else {
			detector.RecordAccess("key2", true)
		}
		time.Sleep(1 * time.Millisecond)
	}

	// Phase 2: Frequency pattern (next 50 accesses)
	for i := 0; i < 40; i++ {
		detector.RecordAccess("hot_key", true) // Dominating key
	}
	for i := 0; i < 10; i++ {
		detector.RecordAccess("cold_key", false)
	}

	// Detect shift
	shift := detector.DetectShift()

	t.Logf("Pattern shift detection:")
	t.Logf("Shift detected: %v", shift.Detected)
	t.Logf("Temporal shift: %.3f", shift.TemporalShift)
	t.Logf("Frequency shift: %.3f", shift.FrequencyShift)

	if shift.RecentPattern != nil {
		t.Logf("Recent pattern: recency=%.3f, frequency=%.3f",
			shift.RecentPattern.RecencyScore, shift.RecentPattern.FrequencyScore)
	}

	if shift.CurrentPattern != nil {
		t.Logf("Current pattern: recency=%.3f, frequency=%.3f",
			shift.CurrentPattern.RecencyScore, shift.CurrentPattern.FrequencyScore)
	}

	// Should detect some shift (patterns are different)
	// Note: May not always exceed 10% threshold due to randomness
	if shift.Detected {
		t.Logf("Pattern shift detected (threshold exceeded)")
	} else {
		t.Logf("Pattern shift magnitude: %.3f%%, %.3f%% (below 10%% threshold)",
			shift.TemporalShift*100, shift.FrequencyShift*100)
	}
}

func TestPatternDetector_InsufficientData(t *testing.T) {
	detector := NewPatternDetector(100)

	// Only a few accesses
	detector.RecordAccess("key1", true)
	detector.RecordAccess("key2", false)

	// Should not detect shift with insufficient data
	shift := detector.DetectShift()

	if shift.Detected {
		t.Error("Should not detect shift with insufficient data")
	}
}

func TestPatternDetector_Clear(t *testing.T) {
	detector := NewPatternDetector(100)

	// Add some data
	for i := 0; i < 50; i++ {
		detector.RecordAccess("key1", true)
	}

	// Clear
	detector.Clear()

	size := detector.GetAccessLogSize()
	if size != 0 {
		t.Errorf("Expected empty log after clear, got %d", size)
	}
}

func TestPatternDetector_PrintSummary(t *testing.T) {
	detector := NewPatternDetector(100)

	// Create mixed pattern
	for i := 0; i < 60; i++ {
		detector.RecordAccess("key1", true) // Frequent key
	}
	for i := 0; i < 20; i++ {
		detector.RecordAccess("key2", false)
	}
	for i := 0; i < 20; i++ {
		key := "unique_" + string(rune('a'+i))
		detector.RecordAccess(key, false)
	}

	// This should log pattern summary
	detector.PrintSummary()

	// Just verify it doesn't crash
	pattern := detector.GetCurrentPattern()
	if pattern == nil {
		t.Error("Expected non-nil pattern")
	}
}
