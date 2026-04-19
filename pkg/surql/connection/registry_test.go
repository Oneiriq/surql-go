package connection

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"sync"
	"testing"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

// ptrBool returns &v; a small helper for RegisterOptions fields.
func ptrBool(v bool) *bool { return &v }

func TestRegistry_Register_BasicFlow(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	cfg := DefaultConfig()

	client, err := r.Register(context.Background(), "primary", cfg, &RegisterOptions{Connect: ptrBool(false)})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if client == nil {
		t.Fatal("Register returned nil client")
	}
	if r.DefaultName() != "primary" {
		t.Fatalf("default name = %q, want primary (first registered auto-promotes)", r.DefaultName())
	}

	got, err := r.Get("primary")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != client {
		t.Fatalf("Get returned %p, want %p", got, client)
	}

	// Default lookup (empty name) should hit the same entry.
	defaultGot, err := r.Get("")
	if err != nil {
		t.Fatalf("Get default: %v", err)
	}
	if defaultGot != client {
		t.Fatalf("Get default returned %p, want %p", defaultGot, client)
	}
}

func TestRegistry_Register_DuplicateRejected(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	cfg := DefaultConfig()
	if _, err := r.Register(context.Background(), "primary", cfg, &RegisterOptions{Connect: ptrBool(false)}); err != nil {
		t.Fatalf("first Register: %v", err)
	}

	_, err := r.Register(context.Background(), "primary", cfg, &RegisterOptions{Connect: ptrBool(false)})
	if err == nil {
		t.Fatal("second Register should fail")
	}
	if !errors.Is(err, surqlerrors.ErrRegistry) {
		t.Fatalf("err = %v; want ErrRegistry", err)
	}
}

func TestRegistry_EmptyName(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	_, err := r.Register(context.Background(), "", DefaultConfig(), &RegisterOptions{Connect: ptrBool(false)})
	if err == nil {
		t.Fatal("Register with empty name should fail")
	}
	if !errors.Is(err, surqlerrors.ErrValidation) {
		t.Fatalf("err = %v; want ErrValidation", err)
	}
}

func TestRegistry_SetDefault_Promotion(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	cfg := DefaultConfig()

	if _, err := r.Register(context.Background(), "primary", cfg, &RegisterOptions{Connect: ptrBool(false)}); err != nil {
		t.Fatalf("Register primary: %v", err)
	}
	if _, err := r.Register(
		context.Background(), "replica", cfg,
		&RegisterOptions{Connect: ptrBool(false), SetDefault: ptrBool(true)},
	); err != nil {
		t.Fatalf("Register replica: %v", err)
	}

	if got := r.DefaultName(); got != "replica" {
		t.Fatalf("default name = %q, want replica", got)
	}
	if err := r.SetDefault("primary"); err != nil {
		t.Fatalf("SetDefault primary: %v", err)
	}
	if got := r.DefaultName(); got != "primary" {
		t.Fatalf("default name after SetDefault = %q, want primary", got)
	}

	if err := r.SetDefault("missing"); err == nil {
		t.Fatal("SetDefault on unknown name should fail")
	}
}

func TestRegistry_Unregister(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	cfg := DefaultConfig()
	for _, name := range []string{"a", "b", "c"} {
		if _, err := r.Register(context.Background(), name, cfg, &RegisterOptions{Connect: ptrBool(false)}); err != nil {
			t.Fatalf("Register %q: %v", name, err)
		}
	}

	if err := r.Unregister(context.Background(), "a", false); err != nil {
		t.Fatalf("Unregister a: %v", err)
	}

	// "a" was the default (registered first); the fallback goes to the
	// next entry in sort order ("b").
	if got := r.DefaultName(); got != "b" {
		t.Fatalf("default name after unregistering 'a' = %q, want b", got)
	}

	if err := r.Unregister(context.Background(), "missing", false); err == nil {
		t.Fatal("Unregister unknown should fail")
	} else if !errors.Is(err, surqlerrors.ErrRegistry) {
		t.Fatalf("err = %v; want ErrRegistry", err)
	}
}

func TestRegistry_List_Sorted(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	for _, name := range []string{"zeta", "alpha", "mu"} {
		if _, err := r.Register(context.Background(), name, DefaultConfig(), &RegisterOptions{Connect: ptrBool(false)}); err != nil {
			t.Fatalf("Register %q: %v", name, err)
		}
	}
	got := r.List()
	want := []string{"alpha", "mu", "zeta"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("List() = %v; want %v", got, want)
	}
	// Defensive: a second call should return a distinct slice but the same content.
	if &got[0] == &r.List()[0] {
		t.Fatal("List returned the internal slice, not a copy")
	}
}

func TestRegistry_GetConfig(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	cfg := DefaultConfig()
	cfg.DBNS = "custom"
	if _, err := r.Register(context.Background(), "primary", cfg, &RegisterOptions{Connect: ptrBool(false)}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	got, err := r.GetConfig("primary")
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if got.DBNS != "custom" {
		t.Fatalf("cfg.DBNS = %q, want custom", got.DBNS)
	}
	if _, err := r.GetConfig("missing"); err == nil {
		t.Fatal("GetConfig unknown should fail")
	}
}

func TestRegistry_Clear(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	for _, name := range []string{"a", "b"} {
		if _, err := r.Register(context.Background(), name, DefaultConfig(), &RegisterOptions{Connect: ptrBool(false)}); err != nil {
			t.Fatalf("Register %q: %v", name, err)
		}
	}
	if err := r.Clear(context.Background()); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if got := r.List(); len(got) != 0 {
		t.Fatalf("List() after Clear = %v; want empty", got)
	}
	if got := r.DefaultName(); got != "" {
		t.Fatalf("DefaultName() after Clear = %q; want empty", got)
	}
}

func TestRegistry_ConcurrentRegistration(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	cfg := DefaultConfig()
	names := []string{"n1", "n2", "n3", "n4", "n5"}

	var wg sync.WaitGroup
	wg.Add(len(names))
	errs := make(chan error, len(names))
	for _, name := range names {
		go func(n string) {
			defer wg.Done()
			if _, err := r.Register(context.Background(), n, cfg, &RegisterOptions{Connect: ptrBool(false)}); err != nil {
				errs <- err
			}
		}(name)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent Register: %v", err)
	}

	got := r.List()
	want := append([]string{}, names...)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("List() = %v; want %v", got, want)
	}
}

func TestGetRegistry_Singleton(t *testing.T) {
	t.Parallel()

	if GetRegistry() != GetRegistry() {
		t.Fatal("GetRegistry should return the same singleton on every call")
	}
}
