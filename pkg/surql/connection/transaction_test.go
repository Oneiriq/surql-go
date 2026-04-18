package connection

import (
	"context"
	"errors"
	"testing"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

func TestTransactionState_String(t *testing.T) {
	cases := map[TransactionState]string{
		TransactionActive:     "active",
		TransactionCommitted:  "committed",
		TransactionRolledBack: "rolled_back",
		TransactionFailed:     "failed",
		TransactionState(99):  "unknown",
	}
	for state, want := range cases {
		if got := state.String(); got != want {
			t.Errorf("state %d: got %q want %q", state, got, want)
		}
	}
}

func TestTransaction_Execute_NotActive(t *testing.T) {
	c, err := NewDatabaseClient(DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	tx := &Transaction{client: c, state: TransactionCommitted}

	_, err = tx.Execute(context.Background(), "SELECT 1")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, surqlerrors.ErrTransaction) {
		t.Errorf("want ErrTransaction, got %v", err)
	}
}

func TestTransaction_Commit_NotActive(t *testing.T) {
	tx := &Transaction{state: TransactionRolledBack}
	err := tx.Commit(context.Background())
	if err == nil || !errors.Is(err, surqlerrors.ErrTransaction) {
		t.Errorf("want ErrTransaction, got %v", err)
	}
}

func TestTransaction_Rollback_NotActive(t *testing.T) {
	tx := &Transaction{state: TransactionCommitted}
	err := tx.Rollback(context.Background())
	if err == nil || !errors.Is(err, surqlerrors.ErrTransaction) {
		t.Errorf("want ErrTransaction, got %v", err)
	}
}

func TestTransaction_IsActive(t *testing.T) {
	active := &Transaction{state: TransactionActive}
	if !active.IsActive() {
		t.Error("active transaction should report IsActive() = true")
	}
	done := &Transaction{state: TransactionCommitted}
	if done.IsActive() {
		t.Error("committed transaction should report IsActive() = false")
	}
}
