//go:build integration
// +build integration

package connection

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	sdkconn "github.com/surrealdb/surrealdb.go/pkg/connection"
)

// getIntegrationURL reads the SurrealDB URL used by CI's integration job.
// Tests skip when SURREAL_URL is unset so local `go test -tags=integration`
// without a server still short-circuits gracefully.
func getIntegrationURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("SURREAL_URL")
	if url == "" {
		t.Skip("SURREAL_URL not set; skipping integration test")
	}
	return url
}

func newIntegrationClient(t *testing.T) (*DatabaseClient, func()) {
	t.Helper()
	cfg := DefaultConfig()
	cfg.DBURL = getIntegrationURL(t)
	cfg.DBNS = "surqlgo_test"
	cfg.DB = "client"
	cfg.DBRetryMaxAttempts = 3
	cfg.DBRetryMinWait = 0.5
	cfg.DBRetryMaxWait = 2.0
	cfg.DBRetryMultiplier = 2.0

	client, err := NewDatabaseClient(cfg)
	if err != nil {
		t.Fatalf("NewDatabaseClient: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	user := envOr("SURREAL_USER", "root")
	pass := envOr("SURREAL_PASS", "root")
	if _, err := client.Signin(ctx, NewRootCredentials(user, pass)); err != nil {
		_ = client.Disconnect()
		t.Fatalf("Signin: %v", err)
	}

	return client, func() { _ = client.Disconnect() }
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// cleanupTable deletes all records in `table` and tolerates "table does
// not exist" errors (SurrealDB v3+ surfaces these instead of treating the
// missing table as a no-op). Use as a pre-test reset.
func cleanupTable(t *testing.T, client *DatabaseClient, table string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := client.Query(ctx, "REMOVE TABLE IF EXISTS "+table+";")
	if err != nil {
		t.Fatalf("pre-test cleanup (%s): %v", table, err)
	}
}

func TestIntegration_ConnectAndSignin(t *testing.T) {
	client, cleanup := newIntegrationClient(t)
	defer cleanup()

	if !client.IsConnected() {
		t.Fatal("client should be connected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ok, err := client.Health(ctx)
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if !ok {
		t.Fatal("Health returned false")
	}
}

func TestIntegration_CRUDRoundTrip(t *testing.T) {
	client, cleanup := newIntegrationClient(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Clean slate.
	cleanupTable(t, client, "surqlgo_test_user")

	created, err := client.Create(ctx, "surqlgo_test_user", map[string]any{
		"name": "alice",
		"age":  30,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created == nil {
		t.Fatal("Create returned nil")
	}

	selected, err := client.Select(ctx, "surqlgo_test_user")
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if selected == nil {
		t.Fatal("Select returned nil")
	}

	merged, err := client.Merge(ctx, "surqlgo_test_user", map[string]any{
		"verified": true,
	})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if merged == nil {
		t.Fatal("Merge returned nil")
	}

	if _, err := client.Delete(ctx, "surqlgo_test_user"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

func TestIntegration_RawQuery(t *testing.T) {
	client, cleanup := newIntegrationClient(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := client.Query(ctx, "RETURN 1 + 1;")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	list, ok := res.([]any)
	if !ok || len(list) == 0 {
		t.Fatalf("Query: unexpected response shape: %+v", res)
	}
}

func TestIntegration_TransactionCommit(t *testing.T) {
	client, cleanup := newIntegrationClient(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cleanupTable(t, client, "surqlgo_txn_commit")

	tx, err := client.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}

	if _, err := tx.Execute(ctx, "CREATE surqlgo_txn_commit:alice SET name = 'alice';"); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("Execute: %v", err)
	}

	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if tx.State() != TransactionCommitted {
		t.Fatalf("state after commit: %v", tx.State())
	}

	res, err := client.Select(ctx, "surqlgo_txn_commit")
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if res == nil {
		t.Fatal("commit did not persist data")
	}
	_, _ = client.Delete(ctx, "surqlgo_txn_commit")
}

func TestIntegration_TransactionRollback(t *testing.T) {
	client, cleanup := newIntegrationClient(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cleanupTable(t, client, "surqlgo_txn_rollback")

	tx, err := client.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}

	if _, err := tx.Execute(ctx, "CREATE surqlgo_txn_rollback:bob SET name = 'bob';"); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("Execute: %v", err)
	}

	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if tx.State() != TransactionRolledBack {
		t.Fatalf("state after rollback: %v", tx.State())
	}

	// Expect no records (or table not yet materialised) after rollback.
	res, err := client.Select(ctx, "surqlgo_txn_rollback")
	if err != nil {
		// On SurrealDB v3+, selecting from a never-materialised table returns
		// "table does not exist" instead of an empty result. That is equally
		// valid evidence that the rollback worked.
		if !strings.Contains(err.Error(), "does not exist") {
			t.Fatalf("Select: %v", err)
		}
		return
	}
	if records, ok := res.([]any); ok && len(records) > 0 {
		t.Fatalf("rollback did not discard data: %+v", records)
	}
}

func TestIntegration_LiveQueryReceivesChange(t *testing.T) {
	// surrealdb.go v1.4.0 panics with "send on closed channel" during test
	// teardown: Kill closes the notification channel while the SDK's
	// readLoop goroutine is still writing to it. The panic lives in an
	// unrecoverable goroutine and fails the whole test binary even though
	// the LiveQuery round-trip itself works. Re-enable this test once the
	// SDK patches CloseLiveNotifications. Until then, streaming is still
	// exercised by the connection unit tests.
	t.Skip("surrealdb.go v1.4.0 has a shutdown race in CloseLiveNotifications; re-enable once fixed upstream")

	client, cleanup := newIntegrationClient(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	table := "surqlgo_live_demo"

	// Reset table.
	cleanupTable(t, client, table)
	if _, err := client.Query(ctx, "DEFINE TABLE "+table+";"); err != nil {
		t.Fatalf("DEFINE TABLE: %v", err)
	}

	live, err := client.Live(ctx, table, false)
	if err != nil {
		t.Fatalf("Live: %v", err)
	}
	defer func() { _ = live.Close(ctx) }()

	// Produce a change the subscription will see.
	if _, err := client.Create(ctx, table, map[string]any{"name": "carol"}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	select {
	case notif, ok := <-live.Notifications():
		if !ok {
			t.Fatal("notification channel closed before event")
		}
		if notif.Action != sdkconn.CreateAction {
			t.Errorf("action = %s, want CREATE", notif.Action)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for notification")
	}

	_, _ = client.Delete(ctx, table)
}
