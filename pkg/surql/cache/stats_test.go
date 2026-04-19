package cache

import "testing"

func TestCacheStatsRecord(t *testing.T) {
	t.Parallel()
	s := NewStats()
	s.RecordHit()
	s.RecordHit()
	s.RecordMiss()
	s.RecordEviction()
	s.SetSize(42)
	snap := s.Snapshot()
	if snap.Hits != 2 || snap.Misses != 1 || snap.Evictions != 1 || snap.Size != 42 {
		t.Fatalf("unexpected snapshot: %+v", snap)
	}
	if got := snap.HitRatio(); got < 0.66 || got > 0.67 {
		t.Fatalf("unexpected hit ratio: %v", got)
	}
}

func TestCacheStatsHitRatioZero(t *testing.T) {
	t.Parallel()
	s := NewStats().Snapshot()
	if got := s.HitRatio(); got != 0 {
		t.Fatalf("expected 0 ratio, got %v", got)
	}
}

func TestCacheStatsReset(t *testing.T) {
	t.Parallel()
	s := NewStats()
	s.RecordHit()
	s.AddSize(5)
	s.Reset()
	snap := s.Snapshot()
	if snap != (StatsSnapshot{}) {
		t.Fatalf("expected zero snapshot after reset, got %+v", snap)
	}
}

func TestCacheStatsAddSize(t *testing.T) {
	t.Parallel()
	s := NewStats()
	s.AddSize(3)
	s.AddSize(-1)
	if got := s.Snapshot().Size; got != 2 {
		t.Fatalf("expected size 2, got %d", got)
	}
}
