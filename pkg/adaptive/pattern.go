package adaptive

import (
	"log"
	"math"
	"sync"
	"time"
)

// CacheAccess represents a single cache access event
type CacheAccess struct {
	Key       string
	Timestamp time.Time
	IsHit     bool
}

// WorkloadPattern characterizes cache access behavior
type WorkloadPattern struct {
	RecencyScore   float64 // 0-1: temporal locality strength (high = LRU-friendly)
	FrequencyScore float64 // 0-1: frequency locality strength (high = LFU-friendly)
	UniqueKeyRatio float64 // 0-1: scan pattern indicator (high = many unique keys)
	TotalAccesses  int
	UniqueKeys     int
	HitRate        float64
}

// PatternShift indicates detected change in workload behavior
type PatternShift struct {
	Detected       bool
	TemporalShift  float64 // Change in recency score
	FrequencyShift float64 // Change in frequency score
	RecentPattern  *WorkloadPattern
	CurrentPattern *WorkloadPattern
}

// PatternDetector analyzes cache access sequences to characterize workload
type PatternDetector struct {
	accessLog  []*CacheAccess
	windowSize int

	mu sync.RWMutex
}

// NewPatternDetector creates a new pattern detector
func NewPatternDetector(windowSize int) *PatternDetector {
	if windowSize <= 0 { // set default only if input is not valid
		windowSize = 1000 // Default: 1000 accesses
	}

	return &PatternDetector{
		accessLog:  make([]*CacheAccess, 0, windowSize),
		windowSize: windowSize,
	}
}

// RecordAccess adds a cache access to the log
func (patternDetector *PatternDetector) RecordAccess(key string, isHit bool) {
	patternDetector.mu.Lock()
	defer patternDetector.mu.Unlock()

	// Record the access event
	access := &CacheAccess{
		Key:       key,
		Timestamp: time.Now(),
		IsHit:     isHit,
	}

	// Append to access log and maintain size
	patternDetector.accessLog = append(patternDetector.accessLog, access)

	// Maintain sliding window
	if len(patternDetector.accessLog) > patternDetector.windowSize {
		patternDetector.accessLog = patternDetector.accessLog[1:]
	}
}

// DetectShift compares recent vs current patterns to detect significant changes
func (patternDetector *PatternDetector) DetectShift() *PatternShift {
	patternDetector.mu.RLock()
	defer patternDetector.mu.RUnlock()

	// Need at least full window for reliable detection
	if len(patternDetector.accessLog) < patternDetector.windowSize {
		return &PatternShift{
			Detected: false,
		}
	}

	// Split into two halves: recent (first 50%) vs current (last 50%)
	midpoint := len(patternDetector.accessLog) / 2

	recentAccesses := patternDetector.accessLog[:midpoint]
	currentAccesses := patternDetector.accessLog[midpoint:]

	// Extract patterns for each half
	recentPattern := patternDetector.extractPattern(recentAccesses)
	currentPattern := patternDetector.extractPattern(currentAccesses)

	// Calculate shifts
	temporalShift := math.Abs(recentPattern.RecencyScore - currentPattern.RecencyScore)
	frequencyShift := math.Abs(recentPattern.FrequencyScore - currentPattern.FrequencyScore)

	// Detect significant shift (threshold: 10% change)
	// This threshold can be tuned based on sensitivity requirements
	const shiftThreshold = 0.10
	detected := (temporalShift > shiftThreshold) || (frequencyShift > shiftThreshold)

	if detected {
		log.Printf("[PatternDetector] Shift detected: temporal=%.3f, frequency=%.3f",
			temporalShift, frequencyShift)
		log.Printf("[PatternDetector] Recent: recency=%.3f, frequency=%.3f",
			recentPattern.RecencyScore, recentPattern.FrequencyScore)
		log.Printf("[PatternDetector] Current: recency=%.3f, frequency=%.3f",
			currentPattern.RecencyScore, currentPattern.FrequencyScore)
	}

	return &PatternShift{
		Detected:       detected,
		TemporalShift:  temporalShift,
		FrequencyShift: frequencyShift,
		RecentPattern:  recentPattern,
		CurrentPattern: currentPattern,
	}
}

// extractPattern analyzes a sequence of accesses to extract workload characteristics
func (patternDetector *PatternDetector) extractPattern(accesses []*CacheAccess) *WorkloadPattern {
	if len(accesses) == 0 {
		return &WorkloadPattern{}
	}

	// Calculate recency score (temporal locality)
	recencyScore := patternDetector.calculateRecencyScore(accesses)

	// Calculate frequency score (frequency-based locality)
	frequencyMap := make(map[string]int)
	hitCount := 0

	for _, access := range accesses {
		frequencyMap[access.Key]++
		if access.IsHit {
			hitCount++
		}
	}

	frequencyScore := patternDetector.calculateFrequencyScore(frequencyMap, len(accesses))

	// Calculate unique key ratio (scan pattern detection)
	uniqueKeys := len(frequencyMap)
	uniqueKeyRatio := float64(uniqueKeys) / float64(len(accesses))

	// Calculate hit rate
	hitRate := float64(hitCount) / float64(len(accesses))

	return &WorkloadPattern{
		RecencyScore:   recencyScore,
		FrequencyScore: frequencyScore,
		UniqueKeyRatio: uniqueKeyRatio,
		TotalAccesses:  len(accesses),
		UniqueKeys:     uniqueKeys,
		HitRate:        hitRate,
	}
}

// calculateRecencyScore measures temporal locality strength
// High score = strong temporal locality (recent items likely to be accessed again)
func (patternDetector *PatternDetector) calculateRecencyScore(accesses []*CacheAccess) float64 {
	if len(accesses) < 4 {
		return 0.0
	}

	// Analyze last 25% of accesses
	recentSize := len(accesses) / 4
	if recentSize < 1 {
		recentSize = 1
	}

	recentAccesses := accesses[len(accesses)-recentSize:]

	// Count hits in recent window
	recentHits := 0
	for _, access := range recentAccesses {
		if access.IsHit {
			recentHits++
		}
	}

	// Recency score = hit rate in recent window
	recencyScore := float64(recentHits) / float64(len(recentAccesses))

	return recencyScore
}

// calculateFrequencyScore measures access frequency skew
// High score = high skew (some keys accessed much more frequently than others)
func (patternDetector *PatternDetector) calculateFrequencyScore(frequencyMap map[string]int, totalAccesses int) float64 {
	if len(frequencyMap) < 2 {
		return 0.0
	}

	// Calculate mean frequency
	mean := float64(totalAccesses) / float64(len(frequencyMap))

	// Calculate standard deviation
	variance := 0.0
	for _, count := range frequencyMap {
		diff := float64(count) - mean
		variance += diff * diff
	}
	variance = variance / float64(len(frequencyMap))
	stddev := math.Sqrt(variance)

	// High Coefficient of variation (CV) indicates high skew (some keys accessed much more than others)
	cv := stddev / mean

	// Normalize to 0-1 range (CV can be > 1, so cap it)
	// CV around 0.5-1.0 indicates moderate skew (frequency locality or LFU-friendly)
	// CV > 2.0 indicates high skew (LFU-friendly because few keys dominate accesses)
	frequencyScore := math.Min(cv/2.0, 1.0)

	return frequencyScore
}

// GetCurrentPattern returns the current workload pattern
func (patternDetector *PatternDetector) GetCurrentPattern() *WorkloadPattern {
	patternDetector.mu.RLock()
	defer patternDetector.mu.RUnlock()

	if len(patternDetector.accessLog) == 0 {
		return &WorkloadPattern{}
	}

	return patternDetector.extractPattern(patternDetector.accessLog)
}

// GetAccessLogSize returns the current size of the access log
func (patternDetector *PatternDetector) GetAccessLogSize() int {
	patternDetector.mu.RLock()
	defer patternDetector.mu.RUnlock()

	return len(patternDetector.accessLog)
}

// Clear resets the access log (useful for testing)
func (patternDetector *PatternDetector) Clear() {
	patternDetector.mu.Lock()
	defer patternDetector.mu.Unlock()

	patternDetector.accessLog = make([]*CacheAccess, 0, patternDetector.windowSize)
}

// PrintSummary logs current pattern statistics
func (patternDetector *PatternDetector) PrintSummary() {
	patternDetector.mu.RLock()
	defer patternDetector.mu.RUnlock()

	if len(patternDetector.accessLog) == 0 {
		log.Println("[PatternDetector] No access data")
		return
	}

	pattern := patternDetector.extractPattern(patternDetector.accessLog)

	log.Println("PatternDetector Summary")
	log.Printf("Access log size: %d / %d", len(patternDetector.accessLog), patternDetector.windowSize)
	log.Printf("Total accesses: %d", pattern.TotalAccesses)
	log.Printf("Unique keys: %d", pattern.UniqueKeys)
	log.Printf("Hit rate: %.3f", pattern.HitRate)
	log.Printf("Recency score: %.3f (temporal locality)", pattern.RecencyScore)
	log.Printf("Frequency score: %.3f (frequency skew)", pattern.FrequencyScore)
	log.Printf("Unique key ratio: %.3f (scan indicator)", pattern.UniqueKeyRatio)

	// Interpretation
	if pattern.RecencyScore > 0.6 {
		log.Println(": High temporal locality (LRU-friendly)")
	} else if pattern.RecencyScore < 0.4 {
		log.Println(": Low temporal locality")
	}

	if pattern.FrequencyScore > 0.6 {
		log.Println(": High frequency skew (LFU-friendly)")
	} else if pattern.FrequencyScore < 0.4 {
		log.Println(": Low frequency skew")
	}

	if pattern.UniqueKeyRatio > 0.7 {
		log.Println(": High unique key ratio (scan-like pattern)")
	}

	log.Println("===============================")
}
