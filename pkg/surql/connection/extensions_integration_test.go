//go:build integration
// +build integration

package connection

import (
	"context"
	"testing"
	"time"
)

func TestIntegration_ConnectionScope(t *testing.T) {
	url := getIntegrationURL(t)

	cfg := DefaultConfig()
	cfg.DBURL = url
	cfg.DBNS = "surqlgo_test"
	cfg.DB = "connection_scope"

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	scoped, cleanup, err := ConnectionScope(ctx, cfg)
	if err != nil {
		t.Fatalf("ConnectionScope: %v", err)
	}
	defer cleanup()

	client, ok := GetDB(scoped)
	if !ok {
		t.Fatal("scope ctx should carry a DatabaseClient")
	}
	if !client.IsConnected() {
		t.Fatal("scope client should be connected")
	}

	// Sanity: the scoped client responds to a health probe.
	ok2, err := client.Health(ctx)
	if err != nil || !ok2 {
		t.Fatalf("Health: ok=%v err=%v", ok2, err)
	}

	// cleanup() should disconnect and subsequent calls should be no-ops.
	cleanup()
	if client.IsConnected() {
		t.Fatal("cleanup should disconnect the scoped client")
	}
}

func TestIntegration_Registry_LiveClient(t *testing.T) {
	url := getIntegrationURL(t)

	r := NewRegistry()

	cfg := DefaultConfig()
	cfg.DBURL = url
	cfg.DBNS = "surqlgo_test"
	cfg.DB = "registry"

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	client, err := r.Register(ctx, "primary", cfg, nil)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	defer func() { _ = r.Clear(ctx) }()

	if !client.IsConnected() {
		t.Fatal("registered client should be connected when Connect!=false")
	}
	if got, err := r.Get(""); err != nil || got != client {
		t.Fatalf("Get default: got=%p err=%v want=%p", got, err, client)
	}

	if err := r.Unregister(ctx, "primary", true); err != nil {
		t.Fatalf("Unregister: %v", err)
	}
	if client.IsConnected() {
		t.Fatal("Unregister with disconnect should close the client")
	}
}

func TestIntegration_AuthManager_SigninRoot(t *testing.T) {
	client, cleanup := newIntegrationClient(t)
	defer cleanup()

	mgr, err := NewAuthManager(client)
	if err != nil {
		t.Fatalf("NewAuthManager: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	user := envOr("SURREAL_USER", "root")
	pass := envOr("SURREAL_PASS", "root")

	if _, err := mgr.Signin(ctx, NewRootCredentials(user, pass)); err != nil {
		t.Fatalf("Signin: %v", err)
	}
	if !mgr.IsAuthenticated() {
		t.Fatal("manager should be authenticated after Signin")
	}
	if got := mgr.AuthType(); got != AuthRoot {
		t.Fatalf("AuthType = %q; want root", got)
	}
	if mgr.CurrentToken() == "" {
		t.Fatal("CurrentToken should be populated after Signin")
	}

	// Refresh should succeed with the cached token.
	if err := mgr.Refresh(ctx); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
}

func TestIntegration_ClientInfo(t *testing.T) {
	client, cleanup := newIntegrationClient(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Root sessions return nil (or empty) for Info; scope sessions return
	// the record. Here we just assert the RPC doesn't error — content is
	// out of scope for parity.
	if _, err := client.Info(ctx); err != nil {
		t.Fatalf("Info: %v", err)
	}
}

func TestIntegration_StreamingManager_Lifecycle(t *testing.T) {
	// surrealdb.go v1.4.0 panics with "send on closed channel" during
	// teardown: Kill closes the notification channel while the SDK's
	// readLoop goroutine is still writing to it. Same symptom as issue #59
	// / the TestIntegration_LiveQueryReceivesChange skip.
	t.Skip("surrealdb.go v1.4.0 has a shutdown race in CloseLiveNotifications; re-enable once fixed upstream")

	client, cleanup := newIntegrationClient(t)
	defer cleanup()

	mgr, err := NewStreamingManager(client)
	if err != nil {
		t.Fatalf("NewStreamingManager: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	table := "surqlgo_stream_mgr"
	cleanupTable(t, client, table)
	if _, err := client.Query(ctx, "DEFINE TABLE "+table+";"); err != nil {
		t.Fatalf("DEFINE TABLE: %v", err)
	}

	live, err := mgr.Spawn(ctx, table, false)
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if mgr.Count() != 1 {
		t.Fatalf("Count after Spawn = %d; want 1", mgr.Count())
	}
	if live.ID() == "" {
		t.Fatal("Live query should have an ID")
	}

	if err := mgr.DrainAll(ctx); err != nil {
		t.Fatalf("DrainAll: %v", err)
	}
	if mgr.Count() != 0 {
		t.Fatalf("Count after DrainAll = %d; want 0", mgr.Count())
	}
}
