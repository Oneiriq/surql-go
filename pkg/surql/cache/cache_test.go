package cache

import (
	"context"
	"testing"
)

func resetGlobal(t *testing.T) {
	t.Helper()
	_ = CloseCache(context.Background())
}

func TestConfigureCacheAndGet(t *testing.T) {
	resetGlobal(t)
	defer resetGlobal(t)
	ctx := context.Background()
	mgr, err := ConfigureCache(ctx, DefaultConfig())
	if err != nil {
		t.Fatalf("configure: %v", err)
	}
	if got := GetCacheManager(); got != mgr {
		t.Fatalf("expected returned manager to match global")
	}
}

func TestConfigureCacheReplacesPrevious(t *testing.T) {
	resetGlobal(t)
	defer resetGlobal(t)
	ctx := context.Background()
	first, err := ConfigureCache(ctx, DefaultConfig())
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := ConfigureCache(ctx, DefaultConfig())
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if first == second {
		t.Fatalf("second configure must return a new manager")
	}
	if GetCacheManager() != second {
		t.Fatalf("global must reference the second manager")
	}
}

func TestInvalidateHelper(t *testing.T) {
	resetGlobal(t)
	defer resetGlobal(t)
	ctx := context.Background()
	mgr, err := ConfigureCache(ctx, DefaultConfig())
	if err != nil {
		t.Fatalf("configure: %v", err)
	}
	_ = mgr.Set(ctx, "user:1", 1, 0, "user")
	_ = mgr.Set(ctx, "user:2", 2, 0, "user")
	_ = mgr.Set(ctx, "post:1", 3, 0, "post")
	count, err := Invalidate(ctx, "post:1", "user", "")
	if err != nil {
		t.Fatalf("invalidate: %v", err)
	}
	if count != 3 {
		t.Fatalf("expected 3 invalidations, got %d", count)
	}
}

func TestInvalidatePatternHelper(t *testing.T) {
	resetGlobal(t)
	defer resetGlobal(t)
	ctx := context.Background()
	mgr, err := ConfigureCache(ctx, DefaultConfig())
	if err != nil {
		t.Fatalf("configure: %v", err)
	}
	_ = mgr.Set(ctx, "x:1", 1, 0)
	_ = mgr.Set(ctx, "x:2", 2, 0)
	n, err := Invalidate(ctx, "", "", "x:*")
	if err != nil {
		t.Fatalf("invalidate: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2, got %d", n)
	}
}

func TestInvalidateWithoutManager(t *testing.T) {
	resetGlobal(t)
	defer resetGlobal(t)
	n, err := Invalidate(context.Background(), "k", "t", "p")
	if err != nil || n != 0 {
		t.Fatalf("unexpected: n=%d err=%v", n, err)
	}
}

func TestClearCache(t *testing.T) {
	resetGlobal(t)
	defer resetGlobal(t)
	ctx := context.Background()
	mgr, err := ConfigureCache(ctx, DefaultConfig())
	if err != nil {
		t.Fatalf("configure: %v", err)
	}
	_ = mgr.Set(ctx, "a", 1, 0)
	_ = mgr.Set(ctx, "b", 2, 0)
	n, err := ClearCache(ctx)
	if err != nil || n != 2 {
		t.Fatalf("unexpected: n=%d err=%v", n, err)
	}
}

func TestClearCacheWithoutManager(t *testing.T) {
	resetGlobal(t)
	n, err := ClearCache(context.Background())
	if err != nil || n != 0 {
		t.Fatalf("unexpected: n=%d err=%v", n, err)
	}
}

func TestCloseCacheClearsGlobal(t *testing.T) {
	resetGlobal(t)
	ctx := context.Background()
	if _, err := ConfigureCache(ctx, DefaultConfig()); err != nil {
		t.Fatalf("configure: %v", err)
	}
	if err := CloseCache(ctx); err != nil {
		t.Fatalf("close: %v", err)
	}
	if GetCacheManager() != nil {
		t.Fatalf("expected global cleared")
	}
	// safe to close again
	if err := CloseCache(ctx); err != nil {
		t.Fatalf("second close: %v", err)
	}
}

func TestConfigureCacheWith(t *testing.T) {
	resetGlobal(t)
	defer resetGlobal(t)
	ctx := context.Background()
	mgr, err := NewCacheManager(DefaultConfig(), nil)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	ConfigureCacheWith(ctx, mgr)
	if GetCacheManager() != mgr {
		t.Fatalf("explicit manager not installed")
	}
}

func TestConfigureCacheInvalid(t *testing.T) {
	resetGlobal(t)
	defer resetGlobal(t)
	cfg := DefaultConfig()
	cfg.Backend = ""
	if _, err := ConfigureCache(context.Background(), cfg); err == nil {
		t.Fatalf("expected error")
	}
}
