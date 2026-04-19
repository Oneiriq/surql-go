package cache

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestMemoryCacheSetGet(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c := NewMemoryCache(8, time.Minute)
	if err := c.Set(ctx, "alpha", 1, 0); err != nil {
		t.Fatalf("set: %v", err)
	}
	v, ok, err := c.Get(ctx, "alpha")
	if err != nil || !ok || v.(int) != 1 {
		t.Fatalf("get: v=%v ok=%v err=%v", v, ok, err)
	}
}

func TestMemoryCacheMissReturnsZero(t *testing.T) {
	t.Parallel()
	c := NewMemoryCache(2, time.Minute)
	v, ok, err := c.Get(context.Background(), "missing")
	if err != nil || ok || v != nil {
		t.Fatalf("unexpected miss result: v=%v ok=%v err=%v", v, ok, err)
	}
}

func TestMemoryCacheDelete(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c := NewMemoryCache(2, time.Minute)
	_ = c.Set(ctx, "k", "v", 0)
	if err := c.Delete(ctx, "k"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, ok, _ := c.Get(ctx, "k")
	if ok {
		t.Fatalf("expected miss after delete")
	}
	if err := c.Delete(ctx, "absent"); err != nil {
		t.Fatalf("delete absent should not error: %v", err)
	}
}

func TestMemoryCacheExists(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c := NewMemoryCache(2, time.Minute)
	_ = c.Set(ctx, "k", "v", 0)
	ok, err := c.Exists(ctx, "k")
	if err != nil || !ok {
		t.Fatalf("expected exists: ok=%v err=%v", ok, err)
	}
	ok, err = c.Exists(ctx, "other")
	if err != nil || ok {
		t.Fatalf("expected miss: ok=%v err=%v", ok, err)
	}
}

func TestMemoryCacheTTLExpiry(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c := NewMemoryCache(4, time.Hour)
	fake := time.Now()
	c.now = func() time.Time { return fake }
	_ = c.Set(ctx, "short", "x", 10*time.Millisecond)
	fake = fake.Add(20 * time.Millisecond)
	_, ok, _ := c.Get(ctx, "short")
	if ok {
		t.Fatalf("expected expired miss")
	}
	ok, _ = c.Exists(ctx, "short")
	if ok {
		t.Fatalf("expected exists=false after expiry")
	}
}

func TestMemoryCacheLRUEviction(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c := NewMemoryCache(2, time.Minute)
	_ = c.Set(ctx, "a", 1, 0)
	_ = c.Set(ctx, "b", 2, 0)
	// touch a to move it to front
	if _, _, err := c.Get(ctx, "a"); err != nil {
		t.Fatalf("get a: %v", err)
	}
	_ = c.Set(ctx, "c", 3, 0) // should evict "b"
	if _, ok, _ := c.Get(ctx, "b"); ok {
		t.Fatalf("expected b evicted")
	}
	if _, ok, _ := c.Get(ctx, "a"); !ok {
		t.Fatalf("expected a to survive")
	}
	if _, ok, _ := c.Get(ctx, "c"); !ok {
		t.Fatalf("expected c present")
	}
	if got := c.Size(); got != 2 {
		t.Fatalf("expected size 2, got %d", got)
	}
}

func TestMemoryCacheReplace(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c := NewMemoryCache(4, time.Minute)
	_ = c.Set(ctx, "k", "old", 0)
	_ = c.Set(ctx, "k", "new", 0)
	v, ok, _ := c.Get(ctx, "k")
	if !ok || v.(string) != "new" {
		t.Fatalf("expected new value, got %v", v)
	}
	if got := c.Size(); got != 1 {
		t.Fatalf("expected size 1 after replace, got %d", got)
	}
}

func TestMemoryCacheClear(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c := NewMemoryCache(4, time.Minute)
	_ = c.Set(ctx, "a", 1, 0)
	_ = c.Set(ctx, "b", 2, 0)
	count, err := c.Clear(ctx)
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 cleared, got %d", count)
	}
	if got := c.Size(); got != 0 {
		t.Fatalf("expected empty cache, got size %d", got)
	}
}

func TestMemoryCacheClearPattern(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c := NewMemoryCache(8, time.Minute)
	_ = c.Set(ctx, "user:1", 1, 0)
	_ = c.Set(ctx, "user:2", 2, 0)
	_ = c.Set(ctx, "post:1", 3, 0)
	count, err := c.ClearPattern(ctx, "user:*")
	if err != nil {
		t.Fatalf("clear pattern: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 cleared, got %d", count)
	}
	if _, ok, _ := c.Get(ctx, "post:1"); !ok {
		t.Fatalf("post key should have survived")
	}
}

func TestMemoryCacheStats(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c := NewMemoryCache(2, time.Minute)
	_ = c.Set(ctx, "a", 1, 0)
	_, _, _ = c.Get(ctx, "a")
	_, _, _ = c.Get(ctx, "missing")
	snap, err := c.Stats(ctx)
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if snap.Hits != 1 || snap.Misses != 1 || snap.Size != 1 {
		t.Fatalf("unexpected snap: %+v", snap)
	}
}

func TestMemoryCacheConcurrent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c := NewMemoryCache(128, time.Minute)
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := "k" + itoa(i%16)
			for j := 0; j < 100; j++ {
				_ = c.Set(ctx, key, j, 0)
				_, _, _ = c.Get(ctx, key)
			}
		}(i)
	}
	wg.Wait()
	if got := c.Size(); got <= 0 || got > 16 {
		t.Fatalf("unexpected size after concurrency: %d", got)
	}
}

func TestMemoryCacheClose(t *testing.T) {
	t.Parallel()
	c := NewMemoryCache(2, time.Minute)
	if err := c.Close(context.Background()); err != nil {
		t.Fatalf("close: %v", err)
	}
}

// itoa avoids importing strconv in a single test helper.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	buf := [20]byte{}
	n := len(buf)
	negative := false
	if i < 0 {
		negative = true
		i = -i
	}
	for i > 0 {
		n--
		buf[n] = byte('0' + i%10)
		i /= 10
	}
	if negative {
		n--
		buf[n] = '-'
	}
	return string(buf[n:])
}
