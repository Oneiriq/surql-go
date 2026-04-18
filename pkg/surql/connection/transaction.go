package connection

import (
	"context"
	"sync"

	surqlerrors "github.com/albedosehen/surql-go/pkg/surql/errors"
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

// Transaction represents a SurrealDB transaction scoped to a single client
// connection. It issues BEGIN/COMMIT/CANCEL as text statements via the
// client's Query RPC, which works on every SurrealDB release (unlike the
// SDK's interactive surrealdb.Transaction helper, which requires >= v3).
//
// A Transaction must be finalised exactly once through Commit or Rollback.
// Calling either after the transaction has been finalised returns
// ErrTransaction.
type Transaction struct {
	client *DatabaseClient

	mu    sync.Mutex
	state TransactionState
}

// Begin opens a new transaction bound to the client's underlying connection
// by executing `BEGIN TRANSACTION`. The context controls the RPC.
func (c *DatabaseClient) Begin(ctx context.Context) (*Transaction, error) {
	if _, err := c.requireDB(); err != nil {
		return nil, surqlerrors.Wrap(surqlerrors.ErrTransaction, "cannot begin transaction", err)
	}
	if _, err := c.QueryWithVars(ctx, "BEGIN TRANSACTION;", nil); err != nil {
		return nil, surqlerrors.Wrap(surqlerrors.ErrTransaction, "begin failed", err)
	}
	return &Transaction{
		client: c,
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

// Commit finalises the transaction, persisting all statements, by issuing
// `COMMIT TRANSACTION`.
func (t *Transaction) Commit(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.state != TransactionActive {
		return surqlerrors.Newf(surqlerrors.ErrTransaction,
			"cannot commit transaction in state %q", t.state)
	}
	if _, err := t.client.QueryWithVars(ctx, "COMMIT TRANSACTION;", nil); err != nil {
		t.state = TransactionFailed
		return surqlerrors.Wrap(surqlerrors.ErrTransaction, "commit failed", err)
	}
	t.state = TransactionCommitted
	return nil
}

// Rollback discards all statements issued against the transaction by
// executing `CANCEL TRANSACTION`.
// Idempotent: calling Rollback on a finalised transaction returns ErrTransaction.
func (t *Transaction) Rollback(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.state != TransactionActive {
		return surqlerrors.Newf(surqlerrors.ErrTransaction,
			"cannot rollback transaction in state %q", t.state)
	}
	if _, err := t.client.QueryWithVars(ctx, "CANCEL TRANSACTION;", nil); err != nil {
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
	return t.client.QueryWithVars(ctx, surql, vars)
}
