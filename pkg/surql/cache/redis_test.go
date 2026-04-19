package cache

import (
	"testing"
	"time"
)

func TestNewRedisCacheInvalidURL(t *testing.T) {
	t.Parallel()
	if _, err := NewRedisCache("", "", time.Minute); err == nil {
		t.Fatalf("expected empty-url error")
	}
	if _, err := NewRedisCache("not a url", "", time.Minute); err == nil {
		t.Fatalf("expected parse error")
	}
}

func TestNewRedisCacheDefaults(t *testing.T) {
	t.Parallel()
	c, err := NewRedisCache("redis://localhost:6379", "surql:", 0)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if c.defaultTTL != DefaultTTL {
		t.Fatalf("expected DefaultTTL fallback, got %v", c.defaultTTL)
	}
	if c.Client() == nil {
		t.Fatalf("expected client")
	}
}

func TestRedisCacheMakeKey(t *testing.T) {
	t.Parallel()
	c, err := NewRedisCache("redis://localhost:6379", "p:", time.Minute)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got := c.makeKey("key"); got != "p:key" {
		t.Fatalf("unexpected: %s", got)
	}
	if got := c.makeKey("p:already"); got != "p:already" {
		t.Fatalf("double prefix: %s", got)
	}
	c2, err := NewRedisCache("redis://localhost:6379", "", time.Minute)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got := c2.makeKey("key"); got != "key" {
		t.Fatalf("no prefix should no-op: %s", got)
	}
}
