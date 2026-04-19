package cache

import (
	"context"
	"testing"
	"time"
)

func newTestManager(t *testing.T) *CacheManager {
	t.Helper()
	mgr, err := NewCacheManager(DefaultConfig(), nil)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	return mgr
}

func TestNewCacheManagerInvalid(t *testing.T) {
	t.Parallel()
	bad := CacheConfig{Enabled: true, Backend: "", DefaultTTL: time.Minute, MaxSize: 10}
	if _, err := NewCacheManager(bad, nil); err == nil {
		t.Fatalf("expected error")
	}
}

func TestNewCacheManagerUnknownBackend(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	cfg.Backend = "whatever"
	if _, err := NewCacheManager(cfg, nil); err == nil {
		t.Fatalf("expected error for unknown backend")
	}
}

func TestManagerBuildKey(t *testing.T) {
	t.Parallel()
	mgr := newTestManager(t)
	if got := mgr.BuildKey("user", "42"); got != "surql:user:42" {
		t.Fatalf("unexpected key: %s", got)
	}
	if got := mgr.BuildKey("surql:already"); got != "surql:already" {
		t.Fatalf("unexpected double prefix: %s", got)
	}
}

func TestManagerSetGetDeleteStats(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mgr := newTestManager(t)
	if err := mgr.Set(ctx, "k", "v", 0); err != nil {
		t.Fatalf("set: %v", err)
	}
	v, ok, err := mgr.Get(ctx, "k")
	if err != nil || !ok || v.(string) != "v" {
		t.Fatalf("get: %v %v %v", v, ok, err)
	}
	if _, ok, _ := mgr.Get(ctx, "missing"); ok {
		t.Fatalf("expected miss")
	}
	snap := mgr.Stats()
	if snap.Hits != 1 || snap.Misses != 1 {
		t.Fatalf("unexpected stats: %+v", snap)
	}
	if err := mgr.Delete(ctx, "k"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if ok, _ := mgr.Exists(ctx, "k"); ok {
		t.Fatalf("expected absent after delete")
	}
}

func TestManagerDisabled(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cfg := DefaultConfig()
	cfg.Enabled = false
	mgr, err := NewCacheManager(cfg, nil)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if err := mgr.Set(ctx, "k", "v", 0); err != nil {
		t.Fatalf("set: %v", err)
	}
	if _, ok, _ := mgr.Get(ctx, "k"); ok {
		t.Fatalf("disabled manager must miss")
	}
	if ok, _ := mgr.Exists(ctx, "k"); ok {
		t.Fatalf("disabled manager must not report Exists true")
	}
	n, _ := mgr.Clear(ctx)
	if n != 0 {
		t.Fatalf("disabled manager Clear must return 0, got %d", n)
	}
	if _, err := mgr.InvalidateTable(ctx, "t"); err != nil {
		t.Fatalf("invalidate table: %v", err)
	}
}

func TestManagerInvalidateTable(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mgr := newTestManager(t)
	if err := mgr.Set(ctx, "user:1", 1, 0, "user"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := mgr.Set(ctx, "user:2", 2, 0, "user"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := mgr.Set(ctx, "post:1", 3, 0, "post"); err != nil {
		t.Fatalf("set: %v", err)
	}
	keys := mgr.TableKeys("user")
	if len(keys) != 2 {
		t.Fatalf("expected 2 tracked user keys, got %d", len(keys))
	}
	n, err := mgr.InvalidateTable(ctx, "user")
	if err != nil || n != 2 {
		t.Fatalf("invalidate table: n=%d err=%v", n, err)
	}
	if ok, _ := mgr.Exists(ctx, "user:1"); ok {
		t.Fatalf("user:1 should be invalidated")
	}
	if ok, _ := mgr.Exists(ctx, "post:1"); !ok {
		t.Fatalf("post:1 should survive")
	}
}

func TestManagerInvalidatePattern(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mgr := newTestManager(t)
	_ = mgr.Set(ctx, "a:1", 1, 0)
	_ = mgr.Set(ctx, "a:2", 2, 0)
	_ = mgr.Set(ctx, "b:1", 3, 0)
	n, err := mgr.InvalidatePattern(ctx, "a:*")
	if err != nil {
		t.Fatalf("pattern: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2, got %d", n)
	}
	if ok, _ := mgr.Exists(ctx, "b:1"); !ok {
		t.Fatalf("b:1 should survive")
	}
}

func TestManagerTrackTable(t *testing.T) {
	t.Parallel()
	mgr := newTestManager(t)
	mgr.TrackTable("user", "user:5")
	keys := mgr.TableKeys("user")
	if len(keys) != 1 {
		t.Fatalf("expected 1 tracked key, got %d", len(keys))
	}
}

func TestManagerClear(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mgr := newTestManager(t)
	_ = mgr.Set(ctx, "a", 1, 0)
	_ = mgr.Set(ctx, "b", 2, 0)
	n, err := mgr.Clear(ctx)
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 cleared, got %d", n)
	}
	if len(mgr.TableKeys("user")) != 0 {
		t.Fatalf("tables should be cleared too")
	}
}

func TestManagerClose(t *testing.T) {
	t.Parallel()
	mgr := newTestManager(t)
	if err := mgr.Close(context.Background()); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestManagerBackendStats(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mgr := newTestManager(t)
	_ = mgr.Set(ctx, "k", 1, 0)
	_, _, _ = mgr.Get(ctx, "k")
	snap, err := mgr.BackendStats(ctx)
	if err != nil {
		t.Fatalf("backend stats: %v", err)
	}
	if snap.Hits < 1 {
		t.Fatalf("expected backend stats to register hits, got %+v", snap)
	}
}
