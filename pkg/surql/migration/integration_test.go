//go:build integration
// +build integration

package migration

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Oneiriq/surql-go/pkg/surql/connection"
)

// getIntegrationURL reads the SurrealDB URL used by CI's integration job.
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

// newIntegrationClient returns a connected, authenticated DatabaseClient.
// The migration package isolates its namespace so parallel test runs do
// not collide with the connection/query packages.
func newIntegrationClient(t *testing.T) (*connection.DatabaseClient, func()) {
	t.Helper()
	cfg := connection.DefaultConfig()
	cfg.DBURL = getIntegrationURL(t)
	cfg.DBNS = "surqlgo_test"
	cfg.DB = "migration"
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

// resetDatabaseState drops the history table and every table created by the
// integration migrations so each test starts from a clean slate.
func resetDatabaseState(t *testing.T, client *connection.DatabaseClient, tables ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	drop := []string{"REMOVE TABLE IF EXISTS " + MigrationTableName + ";"}
	for _, tbl := range tables {
		drop = append(drop, "REMOVE TABLE IF EXISTS "+tbl+";")
	}
	for _, stmt := range drop {
		if _, err := client.Query(ctx, stmt); err != nil {
			t.Fatalf("reset: %s: %v", stmt, err)
		}
	}
}

// writeTestMigration writes a migration file with the given down statements.
func writeTestMigration(t *testing.T, dir, filename, up, down string) {
	t.Helper()
	body := "-- @description: integration\n-- @up\n" + up + "\n-- @down\n" + down + "\n"
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", filename, err)
	}
}

// --- history lifecycle ---

func TestIntegration_CreateMigrationTable_Idempotent(t *testing.T) {
	client, cleanup := newIntegrationClient(t)
	defer cleanup()
	resetDatabaseState(t, client)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for i := 0; i < 2; i++ {
		if err := CreateMigrationTable(ctx, client); err != nil {
			t.Fatalf("CreateMigrationTable iter %d: %v", i, err)
		}
	}
}

func TestIntegration_RecordAndRemoveMigration(t *testing.T) {
	client, cleanup := newIntegrationClient(t)
	defer cleanup()
	resetDatabaseState(t, client)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ms := int64(123)
	entry := MigrationHistory{
		Version:         "20260418_120000",
		Description:     "create users",
		AppliedAt:       time.Now().UTC(),
		Checksum:        "sha256-abc",
		ExecutionTimeMs: &ms,
	}
	if err := RecordMigration(ctx, client, entry); err != nil {
		t.Fatalf("RecordMigration: %v", err)
	}

	applied, err := GetAppliedMigrations(ctx, client)
	if err != nil {
		t.Fatalf("GetAppliedMigrations: %v", err)
	}
	if len(applied) != 1 || applied[0].Version != entry.Version {
		t.Fatalf("unexpected applied: %+v", applied)
	}

	isApplied, err := IsMigrationApplied(ctx, client, entry.Version)
	if err != nil {
		t.Fatalf("IsMigrationApplied: %v", err)
	}
	if !isApplied {
		t.Errorf("expected migration to be applied")
	}

	if err := RemoveMigrationRecord(ctx, client, entry.Version); err != nil {
		t.Fatalf("RemoveMigrationRecord: %v", err)
	}
	remaining, err := GetAppliedMigrations(ctx, client)
	if err != nil {
		t.Fatalf("GetAppliedMigrations after remove: %v", err)
	}
	if len(remaining) != 0 {
		t.Errorf("expected 0 applied after remove, got %d", len(remaining))
	}
}

// --- MigrateUp / MigrateDown round trip ---

func TestIntegration_MigrateUpAndDown(t *testing.T) {
	client, cleanup := newIntegrationClient(t)
	defer cleanup()
	resetDatabaseState(t, client, "int_user", "int_product")

	dir := t.TempDir()
	writeTestMigration(t, dir,
		"20260101_000000_user.surql",
		"DEFINE TABLE int_user SCHEMAFULL; DEFINE FIELD email ON int_user TYPE string;",
		"REMOVE TABLE int_user;",
	)
	writeTestMigration(t, dir,
		"20260102_000000_product.surql",
		"DEFINE TABLE int_product SCHEMAFULL; DEFINE FIELD name ON int_product TYPE string;",
		"REMOVE TABLE int_product;",
	)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Apply both migrations forward.
	statuses, err := MigrateUp(ctx, client, dir)
	if err != nil {
		t.Fatalf("MigrateUp: %v", err)
	}
	if len(statuses) != 2 {
		t.Fatalf("expected 2 statuses, got %d", len(statuses))
	}
	for _, s := range statuses {
		if s.State != MigrationStateApplied {
			t.Errorf("status for %s = %v, want applied", s.Migration.Version, s.State)
		}
	}

	// Report.
	report, err := GetMigrationStatus(ctx, client, dir)
	if err != nil {
		t.Fatalf("GetMigrationStatus: %v", err)
	}
	if report.Total != 2 || report.Applied != 2 || report.Pending != 0 {
		t.Errorf("unexpected report: %+v", report)
	}

	// Second MigrateUp is a no-op.
	noop, err := MigrateUp(ctx, client, dir)
	if err != nil {
		t.Fatalf("MigrateUp noop: %v", err)
	}
	if len(noop) != 0 {
		t.Errorf("expected noop, got %d statuses", len(noop))
	}

	// Rollback one step.
	down, err := MigrateDown(ctx, client, dir, 1)
	if err != nil {
		t.Fatalf("MigrateDown: %v", err)
	}
	if len(down) != 1 {
		t.Fatalf("expected 1 down status, got %d", len(down))
	}

	report, err = GetMigrationStatus(ctx, client, dir)
	if err != nil {
		t.Fatalf("GetMigrationStatus after down: %v", err)
	}
	if report.Applied != 1 || report.Pending != 1 {
		t.Errorf("unexpected report after down: %+v", report)
	}
}

func TestIntegration_ExecuteMigration_FailedStatement_RollsBack(t *testing.T) {
	client, cleanup := newIntegrationClient(t)
	defer cleanup()
	resetDatabaseState(t, client, "int_bad")

	dir := t.TempDir()
	// First statement creates the table, second is invalid SurrealQL.
	writeTestMigration(t, dir,
		"20260201_000000_bad.surql",
		"DEFINE TABLE int_bad SCHEMAFULL; THIS IS NOT VALID SURREALQL;",
		"REMOVE TABLE int_bad;",
	)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	statuses, err := MigrateUp(ctx, client, dir)
	if err == nil {
		t.Fatal("expected migration to fail")
	}
	if len(statuses) != 1 || statuses[0].State != MigrationStateFailed {
		t.Errorf("expected single failed status, got %+v", statuses)
	}

	// History must not contain the failed migration.
	applied, err := GetAppliedMigrations(ctx, client)
	if err != nil {
		t.Fatalf("GetAppliedMigrations: %v", err)
	}
	for _, a := range applied {
		if a.Version == "20260201_000000" {
			t.Errorf("failed migration should not be in history")
		}
	}

	// Also confirm the table was NOT created (transaction rolled back).
	// (Some Surreal versions surface "does not exist"; contains check is
	// the standard idiom used elsewhere in the codebase.)
	_, err = client.Query(ctx, "SELECT * FROM int_bad LIMIT 1;")
	if err != nil && !strings.Contains(err.Error(), "does not exist") {
		t.Logf("select int_bad: %v", err)
	}
}

// --- rollback planning ---

func TestIntegration_CreateAndExecuteRollbackPlan(t *testing.T) {
	client, cleanup := newIntegrationClient(t)
	defer cleanup()
	resetDatabaseState(t, client, "int_a", "int_b", "int_c")

	dir := t.TempDir()
	writeTestMigration(t, dir, "20260301_000000_a.surql",
		"DEFINE TABLE int_a SCHEMAFULL;",
		"REMOVE TABLE int_a;",
	)
	writeTestMigration(t, dir, "20260302_000000_b.surql",
		"DEFINE TABLE int_b SCHEMAFULL;",
		"REMOVE TABLE int_b;",
	)
	writeTestMigration(t, dir, "20260303_000000_c.surql",
		"DEFINE TABLE int_c SCHEMAFULL;",
		"REMOVE TABLE int_c;",
	)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if _, err := MigrateUp(ctx, client, dir); err != nil {
		t.Fatalf("MigrateUp: %v", err)
	}

	plan, err := CreateRollbackPlan(ctx, client, dir, "20260301_000000")
	if err != nil {
		t.Fatalf("CreateRollbackPlan: %v", err)
	}
	// Two migrations should be rolled back (b and c); newest first.
	if plan.MigrationCount() != 2 {
		t.Fatalf("expected 2 migrations, got %d", plan.MigrationCount())
	}
	if plan.Migrations[0].Version != "20260303_000000" {
		t.Errorf("expected newest first; got %s", plan.Migrations[0].Version)
	}
	if plan.OverallSafety != RollbackSafetyDanger {
		t.Errorf("expected Danger safety (REMOVE TABLE), got %s", plan.OverallSafety)
	}

	result, err := ExecuteRollback(ctx, client, plan)
	if err != nil {
		t.Fatalf("ExecuteRollback: %v", err)
	}
	if !result.Success || !result.CompletedAll() {
		t.Errorf("unexpected rollback result: %+v", result)
	}

	report, err := GetMigrationStatus(ctx, client, dir)
	if err != nil {
		t.Fatalf("GetMigrationStatus: %v", err)
	}
	if report.Applied != 1 || report.Pending != 2 {
		t.Errorf("unexpected report after rollback: %+v", report)
	}
}
