package connection

import (
	"context"
	"errors"
	"testing"

	surqlerrors "github.com/albedosehen/surql-go/pkg/surql/errors"
)

func TestLive_RejectsHTTP(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DBURL = "http://localhost:8000/rpc"
	cfg.EnableLiveQueries = false // so Validate() doesn't veto config on load
	c, err := NewDatabaseClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	c.connected = true // skip the requireDB short-circuit
	defer func() { c.connected = false }()

	_, err = c.Live(context.Background(), "person", false)
	if err == nil {
		t.Fatal("expected Live to reject HTTP transport")
	}
	if !errors.Is(err, surqlerrors.ErrValidation) {
		t.Errorf("want ErrValidation, got %v", err)
	}
}

func TestLive_RejectsHTTPS(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DBURL = "https://localhost:8000/rpc"
	cfg.EnableLiveQueries = false
	c, err := NewDatabaseClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	c.connected = true
	defer func() { c.connected = false }()

	_, err = c.Live(context.Background(), "person", false)
	if err == nil {
		t.Fatal("expected Live to reject HTTPS transport")
	}
	if !errors.Is(err, surqlerrors.ErrValidation) {
		t.Errorf("want ErrValidation, got %v", err)
	}
}

func TestLive_EmptyTable(t *testing.T) {
	c, err := NewDatabaseClient(DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	c.connected = true
	defer func() { c.connected = false }()

	_, err = c.Live(context.Background(), "", false)
	if err == nil || !errors.Is(err, surqlerrors.ErrValidation) {
		t.Errorf("want ErrValidation for empty table, got %v", err)
	}
}
