package cache

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

// RedisCache is a distributed CacheBackend using go-redis/v9. Values
// are JSON-encoded.
//
//nolint:revive // Package/type name repetition is intentional for clarity
type RedisCache struct {
	client     *redis.Client
	prefix     string
	defaultTTL time.Duration
	stats      *CacheStats
}

// NewRedisCache dials url and returns a RedisCache. prefix is used
// for key prefixing at the backend level (CacheManager separately
// applies its own prefix via BuildKey; to avoid double-prefixing,
// pass an empty prefix here when the manager already adds one).
func NewRedisCache(url, prefix string, defaultTTL time.Duration) (*RedisCache, error) {
	if url == "" {
		return nil, surqlerrors.New(surqlerrors.ErrValidation, "redis url cannot be empty")
	}
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, surqlerrors.Wrapf(surqlerrors.ErrValidation, err, "invalid redis url %q", url)
	}
	if defaultTTL <= 0 {
		defaultTTL = DefaultTTL
	}
	return &RedisCache{
		client:     redis.NewClient(opts),
		prefix:     prefix,
		defaultTTL: defaultTTL,
		stats:      NewStats(),
	}, nil
}

// NewRedisCacheFromClient wraps an existing *redis.Client. Useful for
// tests that stand up miniredis and need to inject a client directly.
func NewRedisCacheFromClient(client *redis.Client, prefix string, defaultTTL time.Duration) *RedisCache {
	if defaultTTL <= 0 {
		defaultTTL = DefaultTTL
	}
	return &RedisCache{
		client:     client,
		prefix:     prefix,
		defaultTTL: defaultTTL,
		stats:      NewStats(),
	}
}

// Client exposes the underlying *redis.Client (for diagnostics).
func (r *RedisCache) Client() *redis.Client { return r.client }

// Get implements CacheBackend.
func (r *RedisCache) Get(ctx context.Context, key string) (any, bool, error) {
	raw, err := r.client.Get(ctx, r.makeKey(key)).Result()
	if err != nil {
		if err == redis.Nil {
			r.stats.RecordMiss()
			return nil, false, nil
		}
		return nil, false, err
	}
	var value any
	if jsonErr := json.Unmarshal([]byte(raw), &value); jsonErr != nil {
		// Not JSON (e.g. raw string set outside the cache); return the literal.
		r.stats.RecordHit()
		return raw, true, nil
	}
	r.stats.RecordHit()
	return value, true, nil
}

// Set implements CacheBackend.
func (r *RedisCache) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	if ttl <= 0 {
		ttl = r.defaultTTL
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return r.client.Set(ctx, r.makeKey(key), payload, ttl).Err()
}

// Delete implements CacheBackend.
func (r *RedisCache) Delete(ctx context.Context, key string) error {
	return r.client.Del(ctx, r.makeKey(key)).Err()
}

// Exists implements CacheBackend.
func (r *RedisCache) Exists(ctx context.Context, key string) (bool, error) {
	n, err := r.client.Exists(ctx, r.makeKey(key)).Result()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// Clear implements CacheBackend. It drops every key whose prefix
// matches this backend's prefix (i.e. prefix + "*").
func (r *RedisCache) Clear(ctx context.Context) (int, error) {
	return r.ClearPattern(ctx, "*")
}

// ClearPattern implements CacheBackend. The pattern is combined with
// this backend's prefix.
func (r *RedisCache) ClearPattern(ctx context.Context, pattern string) (int, error) {
	match := r.makeKey(pattern)
	removed := 0
	iter := r.client.Scan(ctx, 0, match, 100).Iterator()
	var batch []string
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := r.client.Del(ctx, batch...).Err(); err != nil {
			return err
		}
		removed += len(batch)
		batch = batch[:0]
		return nil
	}
	for iter.Next(ctx) {
		batch = append(batch, iter.Val())
		if len(batch) >= 100 {
			if err := flush(); err != nil {
				return removed, err
			}
		}
	}
	if err := iter.Err(); err != nil {
		return removed, err
	}
	if err := flush(); err != nil {
		return removed, err
	}
	return removed, nil
}

// Stats implements CacheBackend.
func (r *RedisCache) Stats(_ context.Context) (StatsSnapshot, error) {
	return r.stats.Snapshot(), nil
}

// Close implements CacheBackend.
func (r *RedisCache) Close(_ context.Context) error {
	if r.client == nil {
		return nil
	}
	return r.client.Close()
}

// makeKey prepends the configured prefix if it isn't already there.
// When the caller (CacheManager) already prefixes, pass an empty
// prefix into the constructor and this becomes a no-op.
func (r *RedisCache) makeKey(key string) string {
	if r.prefix == "" || strings.HasPrefix(key, r.prefix) {
		return key
	}
	return r.prefix + key
}
