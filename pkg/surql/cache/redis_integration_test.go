//go:build integration

package cache

import (
	"context"
	"os"
	"testing"
	"time"
)

// redisURL returns the DSN integration tests should dial. Tests skip
// cleanly when no URL is configured so local `go test -tags=integration`
// runs stay green without a live Redis.
func redisURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("SURQL_TEST_REDIS_URL")
	if url == "" {
		t.Skip("SURQL_TEST_REDIS_URL not set")
	}
	return url
}

func TestRedisCacheSetGetIntegration(t *testing.T) {
	url := redisURL(t)
	ctx := context.Background()
	c, err := NewRedisCache(url, "surqltest:", time.Minute)
	if err != nil {
		t.Fatalf("new redis: %v", err)
	}
	defer c.Close(ctx)
	if err := c.Set(ctx, "k", map[string]any{"x": 1.0}, 0); err != nil {
		t.Fatalf("set: %v", err)
	}
	v, ok, err := c.Get(ctx, "k")
	if err != nil || !ok {
		t.Fatalf("get: v=%v ok=%v err=%v", v, ok, err)
	}
	m, ok := v.(map[string]any)
	if !ok || m["x"].(float64) != 1.0 {
		t.Fatalf("unexpected payload: %v", v)
	}
}

func TestRedisCacheClearPatternIntegration(t *testing.T) {
	url := redisURL(t)
	ctx := context.Background()
	c, err := NewRedisCache(url, "surqltest:", time.Minute)
	if err != nil {
		t.Fatalf("new redis: %v", err)
	}
	defer c.Close(ctx)
	_ = c.Set(ctx, "user:1", 1, 0)
	_ = c.Set(ctx, "user:2", 2, 0)
	_ = c.Set(ctx, "other", 3, 0)
	n, err := c.ClearPattern(ctx, "user:*")
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	if n < 2 {
		t.Fatalf("expected >= 2 cleared, got %d", n)
	}
	if ok, _ := c.Exists(ctx, "other"); !ok {
		t.Fatalf("other should survive")
	}
	_, _ = c.ClearPattern(ctx, "other")
}

func TestRedisCacheExistsDeleteIntegration(t *testing.T) {
	url := redisURL(t)
	ctx := context.Background()
	c, err := NewRedisCache(url, "surqltest:", time.Minute)
	if err != nil {
		t.Fatalf("new redis: %v", err)
	}
	defer c.Close(ctx)
	_ = c.Set(ctx, "k", 1, 0)
	if ok, _ := c.Exists(ctx, "k"); !ok {
		t.Fatalf("expected exists")
	}
	if err := c.Delete(ctx, "k"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if ok, _ := c.Exists(ctx, "k"); ok {
		t.Fatalf("expected absent after delete")
	}
}
