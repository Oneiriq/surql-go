//go:build integration
// +build integration

package query

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Oneiriq/surql-go/pkg/surql/connection"
	"github.com/Oneiriq/surql-go/pkg/surql/types"
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
	cfg.DB = "query"
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

func cleanupTable(t *testing.T, client *connection.DatabaseClient, table string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := client.Query(ctx, "REMOVE TABLE IF EXISTS "+table+";"); err != nil {
		t.Fatalf("pre-test cleanup (%s): %v", table, err)
	}
}

type integrationUser struct {
	Name  string `json:"name"`
	Age   int    `json:"age"`
	Email string `json:"email,omitempty"`
}

func TestIntegration_CreateAndGetRecord(t *testing.T) {
	client, cleanup := newIntegrationClient(t)
	defer cleanup()
	cleanupTable(t, client, "surqlgo_query_user")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	created, err := CreateRecord(ctx, client, "surqlgo_query_user", map[string]any{
		"name": "alice",
		"age":  30,
	})
	if err != nil {
		t.Fatalf("CreateRecord: %v", err)
	}
	if created == nil {
		t.Fatal("CreateRecord returned nil record")
	}
	if created["name"] != "alice" {
		t.Errorf("name = %v", created["name"])
	}
}

func TestIntegration_QueryRecordsAndCount(t *testing.T) {
	client, cleanup := newIntegrationClient(t)
	defer cleanup()
	cleanupTable(t, client, "surqlgo_query_cnt")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, name := range []string{"alice", "bob", "carol"} {
		if _, err := CreateRecord(ctx, client, "surqlgo_query_cnt", map[string]any{
			"name": name,
		}); err != nil {
			t.Fatalf("CreateRecord(%s): %v", name, err)
		}
	}

	rows, err := QueryRecords(ctx, client, "surqlgo_query_cnt", QueryOptions{})
	if err != nil {
		t.Fatalf("QueryRecords: %v", err)
	}
	if len(rows) != 3 {
		t.Errorf("want 3 rows, got %d", len(rows))
	}

	n, err := CountRecords(ctx, client, "surqlgo_query_cnt", nil)
	if err != nil {
		t.Fatalf("CountRecords: %v", err)
	}
	if n != 3 {
		t.Errorf("want 3 count, got %d", n)
	}
}

func TestIntegration_ExistsAndDelete(t *testing.T) {
	client, cleanup := newIntegrationClient(t)
	defer cleanup()
	cleanupTable(t, client, "surqlgo_query_del")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := client.Query(ctx, "CREATE surqlgo_query_del:alice SET name = 'alice';"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	present, err := Exists(ctx, client, "surqlgo_query_del", "alice")
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if !present {
		t.Fatal("expected record to exist")
	}

	if err := DeleteRecord(ctx, client, "surqlgo_query_del", "alice"); err != nil {
		t.Fatalf("DeleteRecord: %v", err)
	}

	present, err = Exists(ctx, client, "surqlgo_query_del", "alice")
	if err != nil {
		t.Fatalf("Exists after delete: %v", err)
	}
	if present {
		t.Fatal("expected record to be deleted")
	}
}

func TestIntegration_FetchOneAndFetchAll(t *testing.T) {
	client, cleanup := newIntegrationClient(t)
	defer cleanup()
	cleanupTable(t, client, "surqlgo_query_fetch")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for i, name := range []string{"alice", "bob"} {
		if _, err := CreateRecord(ctx, client, "surqlgo_query_fetch", map[string]any{
			"name": name,
			"age":  20 + i,
		}); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	q, err := Query{}.Select(nil).FromTable("surqlgo_query_fetch")
	if err != nil {
		t.Fatal(err)
	}
	all, err := FetchAll[integrationUser](ctx, client, q)
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("want 2 rows, got %d", len(all))
	}

	one, err := FetchOne[integrationUser](ctx, client, q)
	if err != nil {
		t.Fatalf("FetchOne: %v", err)
	}
	if one == nil {
		t.Fatal("expected record")
	}
}

func TestIntegration_CreateTypedAndGetTyped(t *testing.T) {
	client, cleanup := newIntegrationClient(t)
	defer cleanup()
	cleanupTable(t, client, "surqlgo_typed_user")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	payload := integrationUser{Name: "alice", Age: 30, Email: "alice@example.com"}
	created, err := CreateTyped[integrationUser](ctx, client, "surqlgo_typed_user", payload)
	if err != nil {
		t.Fatalf("CreateTyped: %v", err)
	}
	if created.Name != "alice" || created.Age != 30 {
		t.Errorf("got %+v", created)
	}
}

func TestIntegration_ExecuteRawTyped(t *testing.T) {
	client, cleanup := newIntegrationClient(t)
	defer cleanup()
	cleanupTable(t, client, "surqlgo_typed_raw")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := client.Query(ctx, "CREATE surqlgo_typed_raw:alice SET name = 'alice', age = 40;"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rows, err := ExecuteRawTyped[integrationUser](
		ctx, client,
		"SELECT name, age FROM surqlgo_typed_raw WHERE age > $min;",
		map[string]any{"min": 30},
	)
	if err != nil {
		t.Fatalf("ExecuteRawTyped: %v", err)
	}
	if len(rows) != 1 || rows[0].Name != "alice" {
		t.Errorf("got %+v", rows)
	}
}

func TestIntegration_TypeRecordTarget_CRUD(t *testing.T) {
	client, cleanup := newIntegrationClient(t)
	defer cleanup()
	cleanupTable(t, client, "surqlgo_type_record")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Seed via CreateRecord so the record id is server-generated, then
	// use a stable string id for the target-driven assertions.
	if _, err := client.Query(ctx,
		"CREATE surqlgo_type_record:alice SET name = 'alice', age = 30;"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// GetByTarget
	target := types.TypeRecord("surqlgo_type_record", "alice")
	got, err := GetByTarget(ctx, client, target)
	if err != nil {
		t.Fatalf("GetByTarget: %v", err)
	}
	if got == nil || got["name"] != "alice" {
		t.Fatalf("GetByTarget: got %+v", got)
	}

	// ExistsByTarget
	present, err := ExistsByTarget(ctx, client, target)
	if err != nil {
		t.Fatalf("ExistsByTarget: %v", err)
	}
	if !present {
		t.Fatal("ExistsByTarget: want true")
	}

	// UpdateByTarget (PUT semantics: age alone replaces the record body)
	if _, err := UpdateByTarget(ctx, client, target, map[string]any{"age": 40}); err != nil {
		t.Fatalf("UpdateByTarget: %v", err)
	}
	got, err = GetByTarget(ctx, client, target)
	if err != nil || got == nil {
		t.Fatalf("after UpdateByTarget: err=%v got=%+v", err, got)
	}
	if _, ok := got["name"]; ok {
		t.Errorf("UpdateByTarget did not replace body: name still present %+v", got)
	}

	// MergeByTarget restores name alongside age.
	if _, err := MergeByTarget(ctx, client, target, map[string]any{"name": "alice"}); err != nil {
		t.Fatalf("MergeByTarget: %v", err)
	}
	got, err = GetByTarget(ctx, client, target)
	if err != nil || got == nil {
		t.Fatalf("after MergeByTarget: err=%v got=%+v", err, got)
	}
	if got["name"] != "alice" {
		t.Errorf("MergeByTarget: name=%v", got["name"])
	}

	// UpsertByTarget on a new id inserts.
	newTarget := types.TypeRecord("surqlgo_type_record", "carol")
	if _, err := UpsertByTarget(ctx, client, newTarget, map[string]any{"name": "carol"}); err != nil {
		t.Fatalf("UpsertByTarget insert: %v", err)
	}
	got, err = GetByTarget(ctx, client, newTarget)
	if err != nil || got == nil {
		t.Fatalf("after UpsertByTarget insert: err=%v got=%+v", err, got)
	}

	// DeleteByTarget removes.
	if err := DeleteByTarget(ctx, client, target); err != nil {
		t.Fatalf("DeleteByTarget: %v", err)
	}
	present, err = ExistsByTarget(ctx, client, target)
	if err != nil {
		t.Fatalf("ExistsByTarget after delete: %v", err)
	}
	if present {
		t.Fatal("ExistsByTarget after delete: want false")
	}
}

func TestIntegration_TypeThingTarget_Get(t *testing.T) {
	client, cleanup := newIntegrationClient(t)
	defer cleanup()
	cleanupTable(t, client, "surqlgo_type_thing")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := client.Query(ctx,
		"CREATE surqlgo_type_thing:xyz SET name = 'xyz';"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	got, err := GetByTarget(ctx, client, types.TypeThing("surqlgo_type_thing", "xyz"))
	if err != nil {
		t.Fatalf("GetByTarget: %v", err)
	}
	if got == nil || got["name"] != "xyz" {
		t.Errorf("got %+v", got)
	}
}
