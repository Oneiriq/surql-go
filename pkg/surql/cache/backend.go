package cache

import (
	"context"
	"time"
)

// CacheBackend is the storage contract implemented by MemoryCache, RedisCache,
// and any future backends. Every method takes a Context so implementations
// may honour request cancellation (Redis in particular).
//
//nolint:revive // Package/type name repetition is intentional for clarity
type CacheBackend interface {
	// Get returns the cached value, a hit flag, and any lookup error.
	// Implementations must treat "expired" the same as "not present"
	// (ok=false, err=nil).
	Get(ctx context.Context, key string) (value any, ok bool, err error)
	// Set stores value under key with the given TTL. A zero ttl means
	// "use the backend default"; implementations must not treat zero as
	// "never expire" — the Cache subsystem does not support permanent
	// entries.
	Set(ctx context.Context, key string, value any, ttl time.Duration) error
	// Delete removes key. A missing key is not an error.
	Delete(ctx context.Context, key string) error
	// Exists reports whether key is present and unexpired.
	Exists(ctx context.Context, key string) (bool, error)
	// Clear drops every entry. Returns the count removed when the
	// backend can provide one, otherwise -1.
	Clear(ctx context.Context) (int, error)
	// ClearPattern drops every entry whose key matches pattern (glob).
	// Returns the count removed when the backend can provide one,
	// otherwise -1.
	ClearPattern(ctx context.Context, pattern string) (int, error)
	// Stats returns a current snapshot of backend-level counters.
	Stats(ctx context.Context) (StatsSnapshot, error)
	// Close releases any held resources (Redis connections, goroutines,
	// ...). Calling Close on an already-closed backend is a no-op.
	Close(ctx context.Context) error
}
