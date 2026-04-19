package cache

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCachedHitAvoidsFetch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mgr := newTestManager(t)
	var calls atomic.Int32
	fetch := func(ctx context.Context) (int, error) {
		calls.Add(1)
		return 7, nil
	}
	v, err := Cached(ctx, mgr, "answer", time.Minute, fetch)
	if err != nil || v != 7 {
		t.Fatalf("first call: v=%d err=%v", v, err)
	}
	v, err = Cached(ctx, mgr, "answer", time.Minute, fetch)
	if err != nil || v != 7 {
		t.Fatalf("second call: v=%d err=%v", v, err)
	}
	if calls.Load() != 1 {
		t.Fatalf("expected fetch called once, got %d", calls.Load())
	}
}

func TestCachedFetchError(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mgr := newTestManager(t)
	sentinel := errors.New("boom")
	v, err := Cached(ctx, mgr, "k", 0, func(ctx context.Context) (int, error) {
		return 0, sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
	if v != 0 {
		t.Fatalf("expected zero value, got %d", v)
	}
}

func TestCachedNilManagerPassthrough(t *testing.T) {
	t.Parallel()
	v, err := Cached(context.Background(), nil, "k", time.Minute,
		func(ctx context.Context) (string, error) { return "hi", nil })
	if err != nil || v != "hi" {
		t.Fatalf("unexpected result: v=%q err=%v", v, err)
	}
}

func TestCachedDisabledManager(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	cfg.Enabled = false
	mgr, err := NewCacheManager(cfg, nil)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	var calls atomic.Int32
	fetch := func(ctx context.Context) (int, error) {
		calls.Add(1)
		return 1, nil
	}
	for i := 0; i < 3; i++ {
		if _, err := Cached(context.Background(), mgr, "k", time.Minute, fetch); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if calls.Load() != 3 {
		t.Fatalf("expected fetch 3x, got %d", calls.Load())
	}
}

func TestCachedTypeMismatchFallsBack(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mgr := newTestManager(t)
	// poison the cache with a string
	if err := mgr.Set(ctx, "k", "string-value", 0); err != nil {
		t.Fatalf("seed: %v", err)
	}
	var calls atomic.Int32
	v, err := Cached(ctx, mgr, "k", time.Minute, func(ctx context.Context) (int, error) {
		calls.Add(1)
		return 99, nil
	})
	if err != nil || v != 99 {
		t.Fatalf("expected type-mismatch fallback: v=%d err=%v", v, err)
	}
	if calls.Load() != 1 {
		t.Fatalf("fetch should have been invoked once, got %d", calls.Load())
	}
}

func TestCacheKeyForStability(t *testing.T) {
	t.Parallel()
	fn := func(int, string) {}
	k1 := CacheKeyFor(fn, 1, "a")
	k2 := CacheKeyFor(fn, 1, "a")
	if k1 != k2 {
		t.Fatalf("expected stable key, got %s vs %s", k1, k2)
	}
	k3 := CacheKeyFor(fn, 1, "b")
	if k1 == k3 {
		t.Fatalf("keys must differ on differing args")
	}
	if !strings.Contains(k1, ":") {
		t.Fatalf("key missing name prefix: %s", k1)
	}
}

func TestCacheKeyForMapDeterministic(t *testing.T) {
	t.Parallel()
	fn := func(map[string]int) {}
	m1 := map[string]int{"a": 1, "b": 2, "c": 3}
	m2 := map[string]int{"c": 3, "b": 2, "a": 1}
	if CacheKeyFor(fn, m1) != CacheKeyFor(fn, m2) {
		t.Fatalf("map key order must not affect cache key")
	}
}

func TestIsCached(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mgr := newTestManager(t)
	if ok, _ := IsCached(ctx, mgr, "k"); ok {
		t.Fatalf("expected false before set")
	}
	_ = mgr.Set(ctx, "k", 1, 0)
	ok, err := IsCached(ctx, mgr, "k")
	if err != nil || !ok {
		t.Fatalf("expected true after set, got ok=%v err=%v", ok, err)
	}
	if ok, _ := IsCached(ctx, nil, "k"); ok {
		t.Fatalf("nil manager must report false")
	}
}
