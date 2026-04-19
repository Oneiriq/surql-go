package cache

import (
	"time"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

// BackendKind selects a CacheBackend implementation.
type BackendKind string

const (
	// BackendMemory is the single-process LRU+TTL backend.
	BackendMemory BackendKind = "memory"
	// BackendRedis is the Redis-backed distributed backend.
	BackendRedis BackendKind = "redis"
)

// DefaultTTL is the default per-entry TTL when callers do not supply one.
const DefaultTTL = 5 * time.Minute

// DefaultMaxSize is the default MemoryCache capacity.
const DefaultMaxSize = 1000

// DefaultRedisURL is the default Redis DSN.
const DefaultRedisURL = "redis://localhost:6379"

// DefaultKeyPrefix is the default cache key prefix.
const DefaultKeyPrefix = "surql:"

// CacheConfig controls CacheManager behaviour.
//
// Field names mirror surql-py's CacheConfig so env loaders and
// consumer code can round-trip between the two ports.
//
//nolint:revive // Package/field name repetition is intentional for clarity
type CacheConfig struct {
	// Enabled toggles all cache operations. When false the manager
	// short-circuits every call (reads miss, writes no-op).
	Enabled bool
	// Backend selects the CacheBackend implementation.
	Backend BackendKind
	// DefaultTTL is applied to entries stored without an explicit TTL.
	DefaultTTL time.Duration
	// MaxSize bounds the MemoryCache (entries).
	MaxSize int
	// RedisURL is the connection URL for BackendRedis.
	RedisURL string
	// KeyPrefix is prepended to every stored key by CacheManager.
	KeyPrefix string
}

// DefaultConfig returns the library defaults (equivalent to
// surql-py's CacheConfig default factory).
func DefaultConfig() CacheConfig {
	return CacheConfig{
		Enabled:    true,
		Backend:    BackendMemory,
		DefaultTTL: DefaultTTL,
		MaxSize:    DefaultMaxSize,
		RedisURL:   DefaultRedisURL,
		KeyPrefix:  DefaultKeyPrefix,
	}
}

// Validate applies the same rules as surql-py.
func (c CacheConfig) Validate() error {
	switch c.Backend {
	case BackendMemory, BackendRedis:
	case "":
		return surqlerrors.New(surqlerrors.ErrValidation, "cache backend cannot be empty")
	default:
		return surqlerrors.Newf(surqlerrors.ErrValidation, "unknown cache backend %q", c.Backend)
	}
	if c.DefaultTTL <= 0 {
		return surqlerrors.New(surqlerrors.ErrValidation, "cache default_ttl must be positive")
	}
	if c.MaxSize <= 0 {
		return surqlerrors.New(surqlerrors.ErrValidation, "cache max_size must be positive")
	}
	if c.Backend == BackendRedis && c.RedisURL == "" {
		return surqlerrors.New(surqlerrors.ErrValidation, "cache redis_url cannot be empty when backend=redis")
	}
	return nil
}

// CacheOptions is a per-query override (TTL / key / table list).
// Equivalent to surql-py's CacheOptions dataclass.
//
//nolint:revive // Package/field name repetition is intentional for clarity
type CacheOptions struct {
	// TTL overrides CacheConfig.DefaultTTL for a specific entry when non-zero.
	TTL time.Duration
	// Key is an explicit cache key; when empty the caller supplies one.
	Key string
	// InvalidateOn lists table names that, when modified, should evict this entry.
	InvalidateOn []string
}

// Validate applies the same TTL rule as surql-py.
func (o CacheOptions) Validate() error {
	if o.TTL < 0 {
		return surqlerrors.New(surqlerrors.ErrValidation, "cache options TTL must be non-negative")
	}
	return nil
}
