package connection

import (
	"context"
	"errors"
	"testing"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

func TestNewStreamingManager_RejectsNilClient(t *testing.T) {
	t.Parallel()

	_, err := NewStreamingManager(nil)
	if err == nil {
		t.Fatal("NewStreamingManager(nil) should error")
	}
	if !errors.Is(err, surqlerrors.ErrValidation) {
		t.Fatalf("err = %v; want ErrValidation", err)
	}
}

func TestStreamingManager_ZeroCount(t *testing.T) {
	t.Parallel()

	mgr, err := NewStreamingManager(&DatabaseClient{})
	if err != nil {
		t.Fatalf("NewStreamingManager: %v", err)
	}
	if got := mgr.Count(); got != 0 {
		t.Fatalf("Count() = %d, want 0", got)
	}
	if got := mgr.Queries(); len(got) != 0 {
		t.Fatalf("Queries() = %v; want empty", got)
	}
}

func TestStreamingManager_SpawnOnDisconnectedClient(t *testing.T) {
	t.Parallel()

	mgr, err := NewStreamingManager(&DatabaseClient{})
	if err != nil {
		t.Fatalf("NewStreamingManager: %v", err)
	}
	// The DatabaseClient zero value has no cfg.DBURL, so Protocol() will
	// reject before we even reach requireDB. Either way, spawn must fail
	// with a validation error rather than panic.
	_, err = mgr.Spawn(context.Background(), "person", false)
	if err == nil {
		t.Fatal("Spawn on disconnected client should fail")
	}
}

func TestStreamingManager_KillRejectsEmptyID(t *testing.T) {
	t.Parallel()

	mgr, err := NewStreamingManager(&DatabaseClient{})
	if err != nil {
		t.Fatalf("NewStreamingManager: %v", err)
	}
	err = mgr.Kill(context.Background(), "")
	if err == nil {
		t.Fatal("Kill(\"\") should error")
	}
	if !errors.Is(err, surqlerrors.ErrValidation) {
		t.Fatalf("err = %v; want ErrValidation", err)
	}
}

func TestStreamingManager_KillUnknownID(t *testing.T) {
	t.Parallel()

	mgr, err := NewStreamingManager(&DatabaseClient{})
	if err != nil {
		t.Fatalf("NewStreamingManager: %v", err)
	}
	err = mgr.Kill(context.Background(), "never-spawned")
	if err == nil {
		t.Fatal("Kill on unknown id should error")
	}
	if !errors.Is(err, surqlerrors.ErrStreaming) {
		t.Fatalf("err = %v; want ErrStreaming", err)
	}
}

func TestStreamingManager_DrainAllOnEmpty(t *testing.T) {
	t.Parallel()

	mgr, err := NewStreamingManager(&DatabaseClient{})
	if err != nil {
		t.Fatalf("NewStreamingManager: %v", err)
	}
	if err := mgr.DrainAll(context.Background()); err != nil {
		t.Fatalf("DrainAll on empty manager: %v", err)
	}
	if got := mgr.Count(); got != 0 {
		t.Fatalf("Count() = %d after drain; want 0", got)
	}
}
