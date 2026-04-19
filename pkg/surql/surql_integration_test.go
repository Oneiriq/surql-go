//go:build integration
// +build integration

package surql

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Oneiriq/surql-go/pkg/surql/connection"
)

func getIntegrationURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("SURREAL_URL")
	if url == "" {
		t.Skip("SURREAL_URL not set; skipping integration test")
	}
	return url
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func newIntegrationClient(t *testing.T) (*connection.DatabaseClient, func()) {
	t.Helper()
	cfg := connection.DefaultConfig()
	cfg.DBURL = getIntegrationURL(t)
	cfg.DBNS = "surqlgo_test"
	cfg.DB = "surql_extract"
	cfg.DBRetryMaxAttempts = 3
	cfg.DBRetryMinWait = 0.5
	cfg.DBRetryMaxWait = 2.0
	cfg.DBRetryMultiplier = 2.0

	client, err := connection.NewDatabaseClient(cfg)
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
	if _, err := client.Signin(ctx, connection.NewRootCredentials(user, pass)); err != nil {
		_ = client.Disconnect()
		t.Fatalf("Signin: %v", err)
	}
	return client, func() { _ = client.Disconnect() }
}

func TestIntegration_ExtractScalar_CountRoundTrip(t *testing.T) {
	client, cleanup := newIntegrationClient(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Fresh table so we know the row count.
	if _, err := client.Query(ctx,
		"REMOVE TABLE IF EXISTS surqlgo_extract_scalar;"); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	for _, n := range []string{"alice", "bob", "carol", "dave"} {
		if _, err := client.Query(ctx,
			"CREATE surqlgo_extract_scalar CONTENT { name: '"+n+"' };"); err != nil {
			t.Fatalf("seed %s: %v", n, err)
		}
	}

	// COUNT() with GROUP ALL yields one row with key "count".
	raw, err := client.Query(ctx,
		"SELECT count() FROM surqlgo_extract_scalar GROUP ALL;")
	if err != nil {
		t.Fatalf("query: %v", err)
	}

	if !HasResult(raw) {
		t.Fatal("HasResult returned false for populated response")
	}

	// Decode as int64 via ExtractScalar[T]'s json round-trip.
	n, err := ExtractScalar[int64](raw, "count")
	if err != nil {
		t.Fatalf("ExtractScalar[int64]: %v", err)
	}
	if n != 4 {
		t.Errorf("want 4, got %d", n)
	}

	// ExtractOne should return the same row map.
	row, err := ExtractOne(raw)
	if err != nil {
		t.Fatalf("ExtractOne: %v", err)
	}
	if row == nil {
		t.Fatal("ExtractOne returned nil")
	}
}

func TestIntegration_ExtractMany_ListRows(t *testing.T) {
	client, cleanup := newIntegrationClient(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := client.Query(ctx,
		"REMOVE TABLE IF EXISTS surqlgo_extract_many;"); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	names := []string{"x", "y", "z"}
	for _, n := range names {
		if _, err := client.Query(ctx,
			"CREATE surqlgo_extract_many CONTENT { name: '"+n+"' };"); err != nil {
			t.Fatalf("seed %s: %v", n, err)
		}
	}

	raw, err := client.Query(ctx, "SELECT name FROM surqlgo_extract_many;")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	rows, err := ExtractMany(raw)
	if err != nil {
		t.Fatalf("ExtractMany: %v", err)
	}
	if len(rows) != len(names) {
		t.Errorf("want %d rows, got %d", len(names), len(rows))
	}
}
