package connection

import (
	"context"
	"sync"

	surrealdb "github.com/surrealdb/surrealdb.go"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

// TransactionState enumerates the lifecycle states of a Transaction.
type TransactionState int

const (
	// TransactionActive indicates the transaction is open and ready for statements.
	TransactionActive TransactionState = iota
	// TransactionCommitted indicates the transaction was successfully committed.
	TransactionCommitted
	// TransactionRolledBack indicates the transaction was explicitly rolled back.
	TransactionRolledBack
	// TransactionFailed indicates the transaction encountered an unrecoverable error.
	TransactionFailed
)

// String returns the textual form of the transaction state for logging.
func (s TransactionState) String() string {
	switch s {
	case TransactionActive:
		return "active"
	case TransactionCommitted:
		return "committed"
	case TransactionRolledBack:
		return "rolled_back"
	case TransactionFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// Transaction represents a SurrealDB interactive transaction (WebSocket only).
// It delegates to the official SDK's *surrealdb.Transaction under the hood.
//
// Requires SurrealDB v3 or newer (the `begin` RPC was added there). The
// library's CI runs v3.0.5 to cover this path.
//
// A Transaction must be finalised exactly once through Commit or Rollback.
// Calling either after the transaction has been finalised returns
// ErrTransaction.
type Transaction struct {
	client *DatabaseClient
	tx     *surrealdb.Transaction

	mu    sync.Mutex
	state TransactionState
}

// Begin opens a new interactive transaction bound to the client's underlying
// connection. The context controls the BEGIN RPC.
func (c *DatabaseClient) Begin(ctx context.Context) (*Transaction, error) {
	db, err := c.requireDB()
	if err != nil {
		return nil, surqlerrors.Wrap(surqlerrors.ErrTransaction, "cannot begin transaction", err)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return nil, surqlerrors.Wrap(surqlerrors.ErrTransaction, "begin failed", err)
	}
	return &Transaction{
		client: c,
		tx:     tx,
		state:  TransactionActive,
	}, nil
}

// State returns the current transaction lifecycle state.
func (t *Transaction) State() TransactionState {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.state
}

// IsActive reports whether the transaction is still open.
func (t *Transaction) IsActive() bool {
	return t.State() == TransactionActive
}

// Commit finalises the transaction, persisting all statements.
func (t *Transaction) Commit(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.state != TransactionActive {
		return surqlerrors.Newf(surqlerrors.ErrTransaction,
			"cannot commit transaction in state %q", t.state)
	}
	if err := t.tx.Commit(ctx); err != nil {
		t.state = TransactionFailed
		return surqlerrors.Wrap(surqlerrors.ErrTransaction, "commit failed", err)
	}
	t.state = TransactionCommitted
	return nil
}

// Rollback discards all statements issued against the transaction.
// Idempotent: calling Rollback on a finalised transaction returns ErrTransaction.
func (t *Transaction) Rollback(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.state != TransactionActive {
		return surqlerrors.Newf(surqlerrors.ErrTransaction,
			"cannot rollback transaction in state %q", t.state)
	}
	if err := t.tx.Cancel(ctx); err != nil {
		t.state = TransactionFailed
		return surqlerrors.Wrap(surqlerrors.ErrTransaction, "rollback failed", err)
	}
	t.state = TransactionRolledBack
	return nil
}

// Execute runs a SurrealQL statement inside the transaction and returns the
// per-statement response envelope (status/time/result).
func (t *Transaction) Execute(ctx context.Context, surql string) (any, error) {
	return t.ExecuteWithVars(ctx, surql, nil)
}

// ExecuteWithVars is Execute with bound variables.
func (t *Transaction) ExecuteWithVars(ctx context.Context, surql string, vars map[string]any) (any, error) {
	t.mu.Lock()
	active := t.state == TransactionActive
	t.mu.Unlock()
	if !active {
		return nil, surqlerrors.Newf(surqlerrors.ErrTransaction,
			"cannot execute in transaction state %q", t.State())
	}

	results, err := surrealdb.Query[any](ctx, t.tx, surql, vars)
	if err != nil {
		return nil, mapQueryError(err)
	}
	if results == nil {
		return []any{}, nil
	}
	out := make([]any, 0, len(*results))
	for _, r := range *results {
		out = append(out, map[string]any{
			"status": r.Status,
			"time":   r.Time,
			"result": r.Result,
		})
	}
	return out, nil
}
