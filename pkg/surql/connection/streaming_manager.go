package connection

import (
	"context"
	"sync"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

// StreamingManager owns a set of LiveQuery subscriptions belonging to a
// single DatabaseClient. It exposes a lifecycle surface (Spawn/Kill/Count/
// DrainAll) so consumers can fan out many live queries and tear them down
// uniformly. Mirrors surql-py's `connection.streaming.StreamingManager`,
// adapted to idiomatic Go with a sync.Mutex rather than anyio.
//
// The zero value is not usable; construct via NewStreamingManager. All
// methods are safe for concurrent use.
type StreamingManager struct {
	client *DatabaseClient

	mu      sync.Mutex
	queries map[string]*LiveQuery
}

// NewStreamingManager wraps client with a LiveQuery registry. The client
// must be non-nil; spawning a LiveQuery further requires the client to use
// a WebSocket transport (non-HTTP).
func NewStreamingManager(client *DatabaseClient) (*StreamingManager, error) {
	if client == nil {
		return nil, surqlerrors.New(surqlerrors.ErrValidation, "client cannot be nil")
	}
	return &StreamingManager{
		client:  client,
		queries: map[string]*LiveQuery{},
	}, nil
}

// Spawn opens a LIVE SELECT on table and registers the resulting LiveQuery
// so it can be killed or drained later. Returns the LiveQuery as-is so
// callers can range over its notification channel.
func (m *StreamingManager) Spawn(ctx context.Context, table string, diff bool) (*LiveQuery, error) {
	live, err := m.client.Live(ctx, table, diff)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	m.queries[live.ID()] = live
	m.mu.Unlock()
	return live, nil
}

// Kill closes the subscription identified by id and removes it from the
// registry. Returns ErrValidation if id is unknown.
func (m *StreamingManager) Kill(ctx context.Context, id string) error {
	if id == "" {
		return surqlerrors.New(surqlerrors.ErrValidation, "live query id cannot be empty")
	}
	m.mu.Lock()
	live, ok := m.queries[id]
	if ok {
		delete(m.queries, id)
	}
	m.mu.Unlock()

	if !ok {
		return surqlerrors.Newf(surqlerrors.ErrStreaming, "live query %q not found", id)
	}
	return live.Close(ctx)
}

// Count returns the number of live subscriptions currently tracked.
func (m *StreamingManager) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.queries)
}

// Queries returns a snapshot slice of the tracked LiveQuery values. The
// slice is a copy so callers are free to iterate without holding the lock.
func (m *StreamingManager) Queries() []*LiveQuery {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*LiveQuery, 0, len(m.queries))
	for _, q := range m.queries {
		out = append(out, q)
	}
	return out
}

// DrainAll closes every tracked subscription. The first non-nil Close error
// is returned after every queue entry has been attempted, so one bad
// subscription cannot starve the rest of the drain.
func (m *StreamingManager) DrainAll(ctx context.Context) error {
	m.mu.Lock()
	queries := make([]*LiveQuery, 0, len(m.queries))
	for _, q := range m.queries {
		queries = append(queries, q)
	}
	m.queries = map[string]*LiveQuery{}
	m.mu.Unlock()

	var firstErr error
	for _, q := range queries {
		if err := q.Close(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
