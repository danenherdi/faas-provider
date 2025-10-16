package paperclient

// Getter methods for PaperStatus fields
// These methods expose the private fields to external packages

// GetPID returns the process ID
func (s *PaperStatus) GetPID() uint32 {
	return s.pid
}

// GetMaxSize returns the maximum cache size in bytes
func (s *PaperStatus) GetMaxSize() uint64 {
	return s.max_size
}

// GetUsedSize returns the currently used cache size in bytes
func (s *PaperStatus) GetUsedSize() uint64 {
	return s.used_size
}

// GetNumObjects returns the number of objects in cache
func (s *PaperStatus) GetNumObjects() uint64 {
	return s.num_objects
}

// GetRSS returns the resident set size
func (s *PaperStatus) GetRSS() uint64 {
	return s.rss
}

// GetHWM returns the high water mark
func (s *PaperStatus) GetHWM() uint64 {
	return s.hwm
}

// GetTotalGets returns the total number of GET operations
func (s *PaperStatus) GetTotalGets() uint64 {
	return s.total_gets
}

// GetTotalSets returns the total number of SET operations
func (s *PaperStatus) GetTotalSets() uint64 {
	return s.total_sets
}

// GetTotalDels returns the total number of DEL operations
func (s *PaperStatus) GetTotalDels() uint64 {
	return s.total_dels
}

// GetMissRatio returns the cache miss ratio (0.0 to 1.0)
func (s *PaperStatus) GetMissRatio() float64 {
	return s.miss_ratio
}

// GetPolicies returns the list of configured eviction policies
func (s *PaperStatus) GetPolicies() []string {
	// Return a copy to prevent external modification
	policiesCopy := make([]string, len(s.policies))
	copy(policiesCopy, s.policies)
	return policiesCopy
}

// GetPolicy returns the currently active eviction policy
func (s *PaperStatus) GetPolicy() string {
	return s.policy
}

// IsAutoPolicy returns true if auto policy selection is enabled
func (s *PaperStatus) IsAutoPolicy() bool {
	return s.is_auto_policy
}

// GetUptime returns the cache uptime in milliseconds
func (s *PaperStatus) GetUptime() uint64 {
	return s.uptime
}
