package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"time"
)

// Cached is the functional equivalent of surql-py's @cache_query
// decorator. On a hit it returns the cached value cast to T. On a
// miss it invokes fetch, stores the result under key with the given
// ttl, and returns the freshly-computed value.
//
// When mgr is nil or mgr.Enabled() is false, Cached delegates
// straight to fetch with no caching side effects — matching the
// "cache not configured" behaviour of the Python decorator.
func Cached[T any](
	ctx context.Context,
	mgr *CacheManager,
	key string,
	ttl time.Duration,
	fetch func(ctx context.Context) (T, error),
) (T, error) {
	var zero T
	if mgr == nil || !mgr.Enabled() {
		return fetch(ctx)
	}
	if ttl <= 0 {
		ttl = mgr.config.DefaultTTL
	}
	value, ok, err := mgr.Get(ctx, key)
	if err != nil {
		return zero, err
	}
	if ok {
		typed, ok := value.(T)
		if !ok {
			// Defensive: stored value is not of the requested type.
			// Treat as miss rather than returning a zero-value hit.
			_ = mgr.Delete(ctx, key)
		} else {
			return typed, nil
		}
	}
	fresh, err := fetch(ctx)
	if err != nil {
		return zero, err
	}
	if err := mgr.Set(ctx, key, fresh, ttl); err != nil {
		return fresh, err
	}
	return fresh, nil
}

// CacheKeyFor generates a stable cache key for a function + arguments.
// It mirrors surql-py's cache_key_for helper: the function's fully-
// qualified name plus a 16-char SHA-256 prefix over a canonical
// serialisation of the arguments.
//
//nolint:revive // Package/function name repetition is intentional for clarity
func CacheKeyFor(fn any, args ...any) string {
	name := funcName(fn)
	parts := make([]string, 0, len(args))
	for _, a := range args {
		parts = append(parts, serializeArg(a))
	}
	payload := fmt.Sprintf("%s(%s)", name, strings.Join(parts, ","))
	sum := sha256.Sum256([]byte(payload))
	return fmt.Sprintf("%s:%s", name, hex.EncodeToString(sum[:])[:16])
}

// IsCached reports whether key is currently present in the manager.
// It is a no-op (returns false, nil) when mgr is nil or disabled.
func IsCached(ctx context.Context, mgr *CacheManager, key string) (bool, error) {
	if mgr == nil || !mgr.Enabled() {
		return false, nil
	}
	return mgr.Exists(ctx, key)
}

// funcName returns a stable identifier for fn. For non-function
// values it falls back to the Go type name.
func funcName(fn any) string {
	if fn == nil {
		return "nil"
	}
	v := reflect.ValueOf(fn)
	if v.Kind() == reflect.Func {
		if rf := runtime.FuncForPC(v.Pointer()); rf != nil {
			return rf.Name()
		}
	}
	t := reflect.TypeOf(fn)
	if t == nil {
		return "nil"
	}
	return t.String()
}

// serializeArg renders a value into a deterministic string. The
// algorithm mirrors surql-py's _serialize_arg with one difference:
// Go maps iterate in random order, so keys are sorted.
func serializeArg(v any) string {
	if v == nil {
		return "nil"
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Ptr, reflect.Interface:
		if rv.IsNil() {
			return "nil"
		}
		return serializeArg(rv.Elem().Interface())
	case reflect.String:
		return fmt.Sprintf("%q", rv.String())
	case reflect.Bool:
		return fmt.Sprintf("%t", rv.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return fmt.Sprintf("%d", rv.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return fmt.Sprintf("%d", rv.Uint())
	case reflect.Float32, reflect.Float64:
		return fmt.Sprintf("%g", rv.Float())
	case reflect.Slice, reflect.Array:
		parts := make([]string, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			parts[i] = serializeArg(rv.Index(i).Interface())
		}
		return "[" + strings.Join(parts, ",") + "]"
	case reflect.Map:
		keys := rv.MapKeys()
		strs := make([]string, 0, len(keys))
		for _, k := range keys {
			strs = append(strs, fmt.Sprintf("%v=%s", k.Interface(), serializeArg(rv.MapIndex(k).Interface())))
		}
		sort.Strings(strs)
		return "{" + strings.Join(strs, ",") + "}"
	case reflect.Struct:
		t := rv.Type()
		parts := make([]string, 0, rv.NumField())
		for i := 0; i < rv.NumField(); i++ {
			if !t.Field(i).IsExported() {
				continue
			}
			parts = append(parts, fmt.Sprintf("%s=%s", t.Field(i).Name, serializeArg(rv.Field(i).Interface())))
		}
		return "{" + strings.Join(parts, ",") + "}"
	default:
		return fmt.Sprintf("%v", v)
	}
}
