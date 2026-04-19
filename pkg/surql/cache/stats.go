package cache

import "sync/atomic"

// CacheStats exposes atomic hit/miss/size/eviction counters.
//
// CacheStats is intentionally copy-by-value-unsafe (it contains
// atomic.Int64 fields); callers that need a snapshot should call
// Snapshot instead of dereferencing the pointer.
//
//nolint:revive // Package/field name repetition is intentional for clarity
type CacheStats struct {
	hits      atomic.Int64
	misses    atomic.Int64
	size      atomic.Int64
	evictions atomic.Int64
}

// NewStats returns a zero-valued CacheStats.
func NewStats() *CacheStats { return &CacheStats{} }

// RecordHit increments the hit counter.
func (s *CacheStats) RecordHit() { s.hits.Add(1) }

// RecordMiss increments the miss counter.
func (s *CacheStats) RecordMiss() { s.misses.Add(1) }

// RecordEviction increments the eviction counter.
func (s *CacheStats) RecordEviction() { s.evictions.Add(1) }

// SetSize overwrites the current entry-count gauge.
func (s *CacheStats) SetSize(n int64) { s.size.Store(n) }

// AddSize adjusts the gauge by delta (positive to grow, negative to shrink).
func (s *CacheStats) AddSize(delta int64) { s.size.Add(delta) }

// Reset zeroes every counter.
func (s *CacheStats) Reset() {
	s.hits.Store(0)
	s.misses.Store(0)
	s.size.Store(0)
	s.evictions.Store(0)
}

// StatsSnapshot is an immutable CacheStats view for callers.
type StatsSnapshot struct {
	Hits      int64
	Misses    int64
	Size      int64
	Evictions int64
}

// Snapshot returns a concurrent-safe copy of the current counters.
func (s *CacheStats) Snapshot() StatsSnapshot {
	return StatsSnapshot{
		Hits:      s.hits.Load(),
		Misses:    s.misses.Load(),
		Size:      s.size.Load(),
		Evictions: s.evictions.Load(),
	}
}

// HitRatio returns hits / (hits + misses) or 0 when no requests have landed.
func (sn StatsSnapshot) HitRatio() float64 {
	total := sn.Hits + sn.Misses
	if total == 0 {
		return 0.0
	}
	return float64(sn.Hits) / float64(total)
}
