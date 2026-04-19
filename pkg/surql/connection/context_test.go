package connection

import (
	"context"
	"errors"
	"testing"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

func TestSetDB_and_GetDB(t *testing.T) {
	t.Parallel()

	client := &DatabaseClient{}
	ctx := SetDB(context.Background(), client)

	got, ok := GetDB(ctx)
	if !ok {
		t.Fatal("GetDB should find the client set by SetDB")
	}
	if got != client {
		t.Fatalf("GetDB = %p, want %p", got, client)
	}
}

func TestGetDB_MissingReturnsFalse(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		ctx  context.Context
	}{
		{"background", context.Background()},
		{"nil context", nil},
		{"cleared", ClearDB(SetDB(context.Background(), &DatabaseClient{}))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client, ok := GetDB(tc.ctx)
			if ok {
				t.Errorf("GetDB ok = true, want false")
			}
			if client != nil {
				t.Errorf("GetDB client = %v, want nil", client)
			}
		})
	}
}

func TestSetDB_NilContextFallback(t *testing.T) {
	t.Parallel()

	client := &DatabaseClient{}
	ctx := SetDB(nil, client) //nolint:staticcheck // explicit nil is the point
	got, ok := GetDB(ctx)
	if !ok || got != client {
		t.Fatalf("SetDB with nil ctx should fall back to Background; got=%v ok=%v", got, ok)
	}
}

func TestHasDB(t *testing.T) {
	t.Parallel()

	if HasDB(context.Background()) {
		t.Fatal("HasDB on bare ctx should be false")
	}
	if !HasDB(SetDB(context.Background(), &DatabaseClient{})) {
		t.Fatal("HasDB after SetDB should be true")
	}
}

func TestMustGetDB(t *testing.T) {
	t.Parallel()

	if _, err := MustGetDB(context.Background()); err == nil {
		t.Fatal("MustGetDB on bare ctx should error")
	} else if !errors.Is(err, surqlerrors.ErrContext) {
		t.Fatalf("MustGetDB error = %v; want ErrContext", err)
	}

	client := &DatabaseClient{}
	ctx := SetDB(context.Background(), client)
	got, err := MustGetDB(ctx)
	if err != nil {
		t.Fatalf("MustGetDB unexpected err: %v", err)
	}
	if got != client {
		t.Fatalf("MustGetDB client = %p, want %p", got, client)
	}
}

func TestConnectionOverride_RestoresScope(t *testing.T) {
	t.Parallel()

	base := &DatabaseClient{}
	ctx := SetDB(context.Background(), base)

	override := &DatabaseClient{}
	overridden, restore := ConnectionOverride(ctx, override)
	got, _ := GetDB(overridden)
	if got != override {
		t.Fatalf("override ctx should carry override client; got=%p want=%p", got, override)
	}
	// Original context should still carry the base client because Go
	// contexts are immutable.
	if orig, _ := GetDB(ctx); orig != base {
		t.Fatalf("original ctx lost its client after override: got=%p want=%p", orig, base)
	}
	restore()
}

func TestClearDB_ReturnsBackgroundWhenNil(t *testing.T) {
	t.Parallel()

	ctx := ClearDB(nil) //nolint:staticcheck // explicit nil is the point
	if ctx == nil {
		t.Fatal("ClearDB(nil) must return a non-nil context")
	}
	if HasDB(ctx) {
		t.Fatal("ClearDB(nil) should carry no client")
	}
}
