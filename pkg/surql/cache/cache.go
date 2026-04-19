package cache

import (
	"context"
	"sync"
)

// globalMu guards globalManager.
var globalMu sync.RWMutex

// globalManager is the process-wide CacheManager used by the
// top-level helpers (ConfigureCache / GetCacheManager / ...). It is
// nil until ConfigureCache has been called.
var globalManager *CacheManager

// ConfigureCache installs a CacheManager built from cfg as the global
// manager and returns it. Calling ConfigureCache replaces any previous
// manager; the prior manager is closed best-effort.
func ConfigureCache(ctx context.Context, cfg CacheConfig) (*CacheManager, error) {
	mgr, err := NewCacheManager(cfg, nil)
	if err != nil {
		return nil, err
	}
	globalMu.Lock()
	prev := globalManager
	globalManager = mgr
	globalMu.Unlock()
	if prev != nil {
		_ = prev.Close(ctx)
	}
	return mgr, nil
}

// ConfigureCacheWith installs an explicit CacheManager as the global.
// Useful for tests that need to inject a pre-built backend.
func ConfigureCacheWith(ctx context.Context, mgr *CacheManager) {
	globalMu.Lock()
	prev := globalManager
	globalManager = mgr
	globalMu.Unlock()
	if prev != nil && prev != mgr {
		_ = prev.Close(ctx)
	}
}

// GetCacheManager returns the global manager, or nil when cache has
// not been configured.
func GetCacheManager() *CacheManager {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalManager
}

// Invalidate clears entries on the global manager:
//   - when key != "" the single entry is dropped;
//   - when table != "" every entry associated with that table is dropped;
//   - when pattern != "" every key matching the glob is dropped.
//
// The three parameters may be combined; the returned count is the
// sum of everything removed. When no manager is configured the call
// is a no-op (returns 0, nil).
func Invalidate(ctx context.Context, key, table, pattern string) (int, error) {
	mgr := GetCacheManager()
	if mgr == nil {
		return 0, nil
	}
	total := 0
	if key != "" {
		if err := mgr.InvalidateKey(ctx, key); err != nil {
			return total, err
		}
		total++
	}
	if table != "" {
		n, err := mgr.InvalidateTable(ctx, table)
		if err != nil {
			return total, err
		}
		total += n
	}
	if pattern != "" {
		n, err := mgr.InvalidatePattern(ctx, pattern)
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

// ClearCache wipes the global manager. Returns 0 (nil) when no
// manager is configured.
func ClearCache(ctx context.Context) (int, error) {
	mgr := GetCacheManager()
	if mgr == nil {
		return 0, nil
	}
	return mgr.Clear(ctx)
}

// CloseCache closes the global manager and clears the reference.
func CloseCache(ctx context.Context) error {
	globalMu.Lock()
	mgr := globalManager
	globalManager = nil
	globalMu.Unlock()
	if mgr == nil {
		return nil
	}
	return mgr.Close(ctx)
}
