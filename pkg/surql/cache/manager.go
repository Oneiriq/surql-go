package cache

import (
	"context"
	"strings"
	"sync"
	"time"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

// CacheManager orchestrates a CacheBackend with automatic key
// prefixing, table-based invalidation tracking, and hit/miss
// statistics. It mirrors surql-py's CacheManager one-to-one.
//
//nolint:revive // Package/type name repetition is intentional for clarity
type CacheManager struct {
	mu        sync.RWMutex
	config    CacheConfig
	backend   CacheBackend
	stats     *CacheStats
	tableKeys map[string]map[string]struct{} // table -> set of (prefixed) keys
}

// NewCacheManager constructs a CacheManager over an explicit backend.
// When backend is nil, NewCacheManager selects one based on cfg.Backend.
func NewCacheManager(cfg CacheConfig, backend CacheBackend) (*CacheManager, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if backend == nil {
		var err error
		backend, err = buildBackend(cfg)
		if err != nil {
			return nil, err
		}
	}
	return &CacheManager{
		config:    cfg,
		backend:   backend,
		stats:     NewStats(),
		tableKeys: make(map[string]map[string]struct{}),
	}, nil
}

// Config returns the manager's CacheConfig.
func (m *CacheManager) Config() CacheConfig { return m.config }

// Backend returns the underlying CacheBackend.
func (m *CacheManager) Backend() CacheBackend { return m.backend }

// Stats returns a current snapshot of manager-level counters.
func (m *CacheManager) Stats() StatsSnapshot { return m.stats.Snapshot() }

// Enabled reports whether caching is enabled on the manager.
func (m *CacheManager) Enabled() bool { return m.config.Enabled }

// BuildKey prefixes parts with the configured KeyPrefix, joining
// with ":". It is safe to pass an already-prefixed key; duplicates
// are not stacked.
func (m *CacheManager) BuildKey(parts ...string) string {
	key := strings.Join(parts, ":")
	if m.config.KeyPrefix == "" {
		return key
	}
	if strings.HasPrefix(key, m.config.KeyPrefix) {
		return key
	}
	return m.config.KeyPrefix + key
}

// Get returns the cached value and whether it was a hit.
func (m *CacheManager) Get(ctx context.Context, key string) (any, bool, error) {
	if !m.config.Enabled {
		return nil, false, nil
	}
	prefixed := m.BuildKey(key)
	value, ok, err := m.backend.Get(ctx, prefixed)
	if err != nil {
		return nil, false, err
	}
	if ok {
		m.stats.RecordHit()
	} else {
		m.stats.RecordMiss()
	}
	return value, ok, nil
}

// Set stores value under key with the given TTL. A zero ttl falls
// back to config.DefaultTTL. Tables records this key under every
// supplied table name for invalidation bookkeeping.
func (m *CacheManager) Set(
	ctx context.Context,
	key string,
	value any,
	ttl time.Duration,
	tables ...string,
) error {
	if !m.config.Enabled {
		return nil
	}
	if ttl <= 0 {
		ttl = m.config.DefaultTTL
	}
	prefixed := m.BuildKey(key)
	if err := m.backend.Set(ctx, prefixed, value, ttl); err != nil {
		return err
	}
	if len(tables) > 0 {
		m.mu.Lock()
		for _, t := range tables {
			if _, ok := m.tableKeys[t]; !ok {
				m.tableKeys[t] = make(map[string]struct{})
			}
			m.tableKeys[t][prefixed] = struct{}{}
		}
		m.mu.Unlock()
	}
	return nil
}

// Delete removes key (and untracks it from every table).
func (m *CacheManager) Delete(ctx context.Context, key string) error {
	if !m.config.Enabled {
		return nil
	}
	prefixed := m.BuildKey(key)
	if err := m.backend.Delete(ctx, prefixed); err != nil {
		return err
	}
	m.mu.Lock()
	for _, keys := range m.tableKeys {
		delete(keys, prefixed)
	}
	m.mu.Unlock()
	return nil
}

// Exists reports whether key is present and unexpired.
func (m *CacheManager) Exists(ctx context.Context, key string) (bool, error) {
	if !m.config.Enabled {
		return false, nil
	}
	return m.backend.Exists(ctx, m.BuildKey(key))
}

// Clear drops every entry (backend + invalidation tracking).
func (m *CacheManager) Clear(ctx context.Context) (int, error) {
	if !m.config.Enabled {
		return 0, nil
	}
	n, err := m.backend.Clear(ctx)
	if err != nil {
		return 0, err
	}
	m.mu.Lock()
	m.tableKeys = make(map[string]map[string]struct{})
	m.mu.Unlock()
	m.stats.Reset()
	return n, nil
}

// Stats returns the backend's own stats snapshot (falling back to
// the manager's hit/miss counters when the backend cannot report).
func (m *CacheManager) BackendStats(ctx context.Context) (StatsSnapshot, error) {
	if !m.config.Enabled {
		return StatsSnapshot{}, nil
	}
	return m.backend.Stats(ctx)
}

// InvalidateKey removes a single entry.
func (m *CacheManager) InvalidateKey(ctx context.Context, key string) error {
	return m.Delete(ctx, key)
}

// InvalidateTable removes every entry associated with the given
// table name and returns the count dropped.
func (m *CacheManager) InvalidateTable(ctx context.Context, table string) (int, error) {
	if !m.config.Enabled {
		return 0, nil
	}
	m.mu.Lock()
	keys := m.tableKeys[table]
	m.tableKeys[table] = make(map[string]struct{})
	m.mu.Unlock()
	count := 0
	for k := range keys {
		if err := m.backend.Delete(ctx, k); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

// InvalidatePattern clears every key matching the provided glob.
func (m *CacheManager) InvalidatePattern(ctx context.Context, pattern string) (int, error) {
	if !m.config.Enabled {
		return 0, nil
	}
	prefixed := m.BuildKey(pattern)
	return m.backend.ClearPattern(ctx, prefixed)
}

// TrackTable associates key (raw, unprefixed) with table for later
// invalidation.
func (m *CacheManager) TrackTable(table, key string) {
	prefixed := m.BuildKey(key)
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.tableKeys[table]; !ok {
		m.tableKeys[table] = make(map[string]struct{})
	}
	m.tableKeys[table][prefixed] = struct{}{}
}

// TableKeys returns a copy of the tracked keys for the given table.
func (m *CacheManager) TableKeys(table string) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	keys := m.tableKeys[table]
	out := make([]string, 0, len(keys))
	for k := range keys {
		out = append(out, k)
	}
	return out
}

// Close releases any resources held by the backend.
func (m *CacheManager) Close(ctx context.Context) error {
	return m.backend.Close(ctx)
}

// buildBackend selects a CacheBackend based on cfg.Backend.
func buildBackend(cfg CacheConfig) (CacheBackend, error) {
	switch cfg.Backend {
	case BackendMemory:
		return NewMemoryCache(cfg.MaxSize, cfg.DefaultTTL), nil
	case BackendRedis:
		return NewRedisCache(cfg.RedisURL, cfg.KeyPrefix, cfg.DefaultTTL)
	default:
		return nil, surqlerrors.Newf(surqlerrors.ErrValidation, "unknown cache backend %q", cfg.Backend)
	}
}
