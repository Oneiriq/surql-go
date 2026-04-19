// Package cache provides the surql-go query-result caching surface.
//
// The cache module mirrors surql-py/src/surql/cache: a CacheBackend
// interface, an in-memory LRU+TTL backend, a Redis backend for
// distributed deployments, and a CacheManager that layers automatic
// key prefixing, table-based invalidation, and hit/miss statistics on
// top of any backend.
//
// # Backends
//
// MemoryCache is a synchronous single-process cache built on
// container/list (for LRU order) and map[string]*list.Element (for
// O(1) lookup). Entries carry a per-entry TTL enforced on read and
// lazily cleaned during writes when the LRU bound is exceeded.
//
// RedisCache uses github.com/redis/go-redis/v9 to provide a
// distributed cache suitable for multi-instance deployments. Values
// are JSON-encoded.
//
// # Manager
//
// CacheManager wraps a backend with:
//   - config-driven backend selection (memory | redis),
//   - key prefixing via CacheConfig.KeyPrefix,
//   - table-based invalidation tracking,
//   - atomic hit/miss counters exposed through CacheStats.
//
// # Decorator
//
// Cached[T] is the functional equivalent of surql-py's @cache_query
// decorator. It takes a key, a TTL, and a fetch function; on a hit
// it returns the cached value, otherwise it executes the fetch and
// stores the result.
//
// # Global helpers
//
// The top-level ConfigureCache / GetCacheManager / Invalidate /
// ClearCache / CloseCache functions operate on a process-global
// CacheManager for callers that want the Python-style convenience
// surface without threading the manager through every call site.
package cache
