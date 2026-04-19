package connection

import (
	"context"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

// clientCtxKey is the unexported type used as a context value key so only this
// package can read or write the ambient *DatabaseClient. Mirrors the
// contextvars-based API of surql-py's `connection.context` module, but uses
// idiomatic context.Context propagation — the caller owns the scope.
type clientCtxKey struct{}

// clientKey is the singleton key value used with context.WithValue.
var clientKey = clientCtxKey{}

// SetDB returns a new context carrying client as the ambient DatabaseClient.
// Matches surql-py's `set_db`, except callers must thread the returned
// context through the goroutines that need it — Go has no goroutine-local
// storage.
func SetDB(ctx context.Context, client *DatabaseClient) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, clientKey, client)
}

// GetDB returns the ambient DatabaseClient attached to ctx, and whether one
// was present. Matches surql-py's `get_db`, but returns the boolean flag
// rather than raising — callers decide whether absence is fatal.
func GetDB(ctx context.Context) (*DatabaseClient, bool) {
	if ctx == nil {
		return nil, false
	}
	client, ok := ctx.Value(clientKey).(*DatabaseClient)
	if !ok || client == nil {
		return nil, false
	}
	return client, true
}

// MustGetDB returns the ambient DatabaseClient or an ErrContext error when no
// client is attached. This is the closest equivalent to surql-py's raising
// `get_db()` — useful for helper functions that want to fail loudly rather
// than swallow a missing ambient connection.
func MustGetDB(ctx context.Context) (*DatabaseClient, error) {
	if client, ok := GetDB(ctx); ok {
		return client, nil
	}
	return nil, surqlerrors.New(
		surqlerrors.ErrContext,
		"no active database connection; use ConnectionScope or SetDB first",
	)
}

// HasDB reports whether ctx carries an ambient DatabaseClient. Matches
// surql-py's `has_db`.
func HasDB(ctx context.Context) bool {
	_, ok := GetDB(ctx)
	return ok
}

// ClearDB returns a derived context with the ambient DatabaseClient removed.
// Matches surql-py's `clear_db`. The original ctx is untouched (contexts are
// immutable).
func ClearDB(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	// context.WithValue with nil typed value ensures GetDB returns (nil, false).
	return context.WithValue(ctx, clientKey, (*DatabaseClient)(nil))
}

// ConnectionScope establishes a fresh DatabaseClient from cfg, connects it,
// attaches it to the returned context, and returns a cleanup function.
//
// The cleanup disconnects the client and releases the context scope. It is
// safe to call cleanup multiple times; only the first call performs I/O.
// Errors from Connect propagate to the caller; a non-nil cleanup is only
// returned on success.
//
// Usage:
//
//	ctx, cleanup, err := connection.ConnectionScope(ctx, cfg)
//	if err != nil { ... }
//	defer cleanup()
//
// Mirrors surql-py's `connection_scope` async context manager.
func ConnectionScope(ctx context.Context, cfg ConnectionConfig) (context.Context, func(), error) {
	client, err := NewDatabaseClient(cfg)
	if err != nil {
		return ctx, nil, err
	}
	if err := client.Connect(ctx); err != nil {
		return ctx, nil, err
	}
	scoped := SetDB(ctx, client)
	cleanup := disconnectOnce(client)
	return scoped, cleanup, nil
}

// ConnectionOverride temporarily swaps the ambient DatabaseClient for the
// provided one. The returned restore function removes the override and
// returns the context to its prior state. Useful for tests or transient
// redirection of downstream helpers that consume GetDB.
//
// The returned context carries the override; the original is unchanged.
// Mirrors surql-py's `connection_override`.
func ConnectionOverride(ctx context.Context, client *DatabaseClient) (context.Context, func()) {
	overridden := SetDB(ctx, client)
	// Nothing to restore on the shared context — Go contexts are immutable,
	// so the cleanup is a no-op. The name exists so call sites can mirror
	// the py surface and clearly delimit override scope.
	return overridden, func() {}
}

// disconnectOnce builds a one-shot cleanup function that disconnects client
// exactly once regardless of how many times it is invoked.
func disconnectOnce(client *DatabaseClient) func() {
	done := false
	return func() {
		if done {
			return
		}
		done = true
		_ = client.Disconnect()
	}
}
