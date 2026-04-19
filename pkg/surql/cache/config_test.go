package cache

import (
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	if !cfg.Enabled {
		t.Fatalf("expected Enabled=true, got false")
	}
	if cfg.Backend != BackendMemory {
		t.Fatalf("expected BackendMemory, got %q", cfg.Backend)
	}
	if cfg.DefaultTTL != DefaultTTL {
		t.Fatalf("expected DefaultTTL=%v, got %v", DefaultTTL, cfg.DefaultTTL)
	}
	if cfg.MaxSize != DefaultMaxSize {
		t.Fatalf("expected MaxSize=%d, got %d", DefaultMaxSize, cfg.MaxSize)
	}
	if cfg.RedisURL != DefaultRedisURL {
		t.Fatalf("expected RedisURL=%q, got %q", DefaultRedisURL, cfg.RedisURL)
	}
	if cfg.KeyPrefix != DefaultKeyPrefix {
		t.Fatalf("expected KeyPrefix=%q, got %q", DefaultKeyPrefix, cfg.KeyPrefix)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected default config to validate: %v", err)
	}
}

func TestCacheConfigValidate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		cfg     CacheConfig
		wantErr bool
	}{
		{
			name: "valid memory",
			cfg:  DefaultConfig(),
		},
		{
			name: "valid redis",
			cfg: CacheConfig{
				Enabled: true, Backend: BackendRedis,
				DefaultTTL: time.Minute, MaxSize: 10, RedisURL: "redis://localhost:6379",
			},
		},
		{
			name: "empty backend",
			cfg: CacheConfig{
				Enabled: true, Backend: "",
				DefaultTTL: time.Minute, MaxSize: 10,
			},
			wantErr: true,
		},
		{
			name: "unknown backend",
			cfg: CacheConfig{
				Enabled: true, Backend: "bogus",
				DefaultTTL: time.Minute, MaxSize: 10,
			},
			wantErr: true,
		},
		{
			name: "zero ttl",
			cfg: CacheConfig{
				Enabled: true, Backend: BackendMemory,
				DefaultTTL: 0, MaxSize: 10,
			},
			wantErr: true,
		},
		{
			name: "zero max size",
			cfg: CacheConfig{
				Enabled: true, Backend: BackendMemory,
				DefaultTTL: time.Minute, MaxSize: 0,
			},
			wantErr: true,
		},
		{
			name: "redis without url",
			cfg: CacheConfig{
				Enabled: true, Backend: BackendRedis,
				DefaultTTL: time.Minute, MaxSize: 10,
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.cfg.Validate()
			if tt.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestCacheOptionsValidate(t *testing.T) {
	t.Parallel()
	if err := (CacheOptions{TTL: time.Minute}).Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := (CacheOptions{TTL: -1}).Validate(); err == nil {
		t.Fatalf("expected negative-ttl error")
	}
	if err := (CacheOptions{}).Validate(); err != nil {
		t.Fatalf("zero-ttl should be fine: %v", err)
	}
}
