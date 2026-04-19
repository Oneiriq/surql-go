package cache

import (
	"container/list"
	"context"
	"path"
	"sync"
	"time"
)

// memoryEntry is the payload stored inside the LRU list.
type memoryEntry struct {
	key       string
	value     any
	expiresAt time.Time
}

// MemoryCache is an LRU + per-entry TTL cache backed by a linked list
// (for O(1) eviction ordering) and a map of key -> *list.Element (for
// O(1) lookup). Both structures are guarded by a single sync.RWMutex:
// reads that don't mutate the LRU position take the read lock, writes
// and TTL-triggered evictions take the write lock.
//
// MemoryCache is safe for concurrent use.
//
//nolint:revive // Package/type name repetition is intentional for clarity
type MemoryCache struct {
	mu         sync.RWMutex
	capacity   int
	defaultTTL time.Duration
	items      map[string]*list.Element
	order      *list.List
	stats      *CacheStats
	now        func() time.Time // overridable for tests
}

// NewMemoryCache constructs a MemoryCache with the provided capacity
// and default TTL. Capacity <= 0 and ttl <= 0 both fall back to the
// library defaults (DefaultMaxSize / DefaultTTL).
func NewMemoryCache(capacity int, defaultTTL time.Duration) *MemoryCache {
	if capacity <= 0 {
		capacity = DefaultMaxSize
	}
	if defaultTTL <= 0 {
		defaultTTL = DefaultTTL
	}
	return &MemoryCache{
		capacity:   capacity,
		defaultTTL: defaultTTL,
		items:      make(map[string]*list.Element, capacity),
		order:      list.New(),
		stats:      NewStats(),
		now:        time.Now,
	}
}

// Capacity returns the configured LRU capacity.
func (m *MemoryCache) Capacity() int { return m.capacity }

// Size returns the current number of unexpired entries.
func (m *MemoryCache) Size() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.order.Len()
}

// Get implements CacheBackend.
func (m *MemoryCache) Get(_ context.Context, key string) (any, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	elem, ok := m.items[key]
	if !ok {
		m.stats.RecordMiss()
		return nil, false, nil
	}
	entry := elem.Value.(*memoryEntry) //nolint:errcheck // internal invariant
	if !entry.expiresAt.IsZero() && !m.now().Before(entry.expiresAt) {
		m.removeElement(elem)
		m.stats.RecordMiss()
		return nil, false, nil
	}
	m.order.MoveToFront(elem)
	m.stats.RecordHit()
	return entry.value, true, nil
}

// Set implements CacheBackend.
func (m *MemoryCache) Set(_ context.Context, key string, value any, ttl time.Duration) error {
	if ttl <= 0 {
		ttl = m.defaultTTL
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	expiresAt := m.now().Add(ttl)
	if elem, ok := m.items[key]; ok {
		entry := elem.Value.(*memoryEntry) //nolint:errcheck // internal invariant
		entry.value = value
		entry.expiresAt = expiresAt
		m.order.MoveToFront(elem)
		return nil
	}
	entry := &memoryEntry{key: key, value: value, expiresAt: expiresAt}
	elem := m.order.PushFront(entry)
	m.items[key] = elem
	m.stats.AddSize(1)
	for m.order.Len() > m.capacity {
		oldest := m.order.Back()
		if oldest == nil {
			break
		}
		m.removeElement(oldest)
		m.stats.RecordEviction()
	}
	return nil
}

// Delete implements CacheBackend.
func (m *MemoryCache) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if elem, ok := m.items[key]; ok {
		m.removeElement(elem)
	}
	return nil
}

// Exists implements CacheBackend.
func (m *MemoryCache) Exists(_ context.Context, key string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	elem, ok := m.items[key]
	if !ok {
		return false, nil
	}
	entry := elem.Value.(*memoryEntry) //nolint:errcheck // internal invariant
	if !entry.expiresAt.IsZero() && !m.now().Before(entry.expiresAt) {
		m.removeElement(elem)
		return false, nil
	}
	return true, nil
}

// Clear implements CacheBackend.
func (m *MemoryCache) Clear(_ context.Context) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := m.order.Len()
	m.items = make(map[string]*list.Element, m.capacity)
	m.order = list.New()
	m.stats.SetSize(0)
	return count, nil
}

// ClearPattern implements CacheBackend; pattern is a filepath.Match glob.
func (m *MemoryCache) ClearPattern(_ context.Context, pattern string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	removed := 0
	for key, elem := range m.items {
		match, err := path.Match(pattern, key)
		if err != nil {
			return removed, err
		}
		if match {
			m.removeElement(elem)
			removed++
		}
	}
	return removed, nil
}

// Stats implements CacheBackend.
func (m *MemoryCache) Stats(_ context.Context) (StatsSnapshot, error) {
	snap := m.stats.Snapshot()
	snap.Size = int64(m.Size())
	return snap, nil
}

// Close implements CacheBackend; MemoryCache has no resources to release.
func (m *MemoryCache) Close(_ context.Context) error { return nil }

// removeElement removes an LRU element. Caller must hold m.mu (write).
func (m *MemoryCache) removeElement(elem *list.Element) {
	entry := elem.Value.(*memoryEntry) //nolint:errcheck // internal invariant
	delete(m.items, entry.key)
	m.order.Remove(elem)
	m.stats.AddSize(-1)
}
