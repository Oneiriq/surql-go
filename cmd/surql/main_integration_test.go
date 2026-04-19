//go:build integration

// Package main's integration tests exercise the full `surql` CLI against a
// running SurrealDB v3.0.5 container. Opt in with:
//
//	go test -tags=integration ./cmd/surql/...
//
// The suite expects the following environment variables to be set (or
// acceptable defaults):
//
//	SURQL_URL      default ws://127.0.0.1:8000
//	SURQL_DB_USER  default root
//	SURQL_DB_PASS  default root
//	SURQL_NS       default test
//	SURQL_DB       default test
package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// surqlCommand runs the CLI binary via `go run` with the supplied args. We
// prefer `go run` over invoking the package directly so the integration
// tests exercise the exact same entry point users hit.
func surqlCommand(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	root := moduleRoot(t)
	cmd := exec.CommandContext(ctx, "go", append([]string{"run", "./cmd/surql"}, args...)...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "NO_COLOR=1")

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("run failed: %v (stderr=%q)", err, stderr.String())
	}
	return stdout.String(), stderr.String(), code
}

// moduleRoot walks up from the test file to the module root.
func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(file)
	// cmd/surql/main_integration_test.go -> module root is two levels up.
	return filepath.Clean(filepath.Join(dir, "..", ".."))
}

// TestIntegration_Version runs the `version` command and verifies the
// output shape. This smoke test is also a good sanity check that the
// build + entry point still compile.
func TestIntegration_Version(t *testing.T) {
	stdout, stderr, code := surqlCommand(t, "version")
	if code != 0 {
		t.Fatalf("version exit %d stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "surql ") {
		t.Fatalf("expected version output, got %q", stdout)
	}
}

// TestIntegration_DBPing exercises the end-to-end round trip against a
// live SurrealDB v3.0.5 container. Requires SURQL_URL / SURQL_DB_USER /
// SURQL_DB_PASS to be pointed at a reachable server.
func TestIntegration_DBPing(t *testing.T) {
	if os.Getenv("SURQL_URL") == "" {
		t.Skip("SURQL_URL not set; skipping ping integration test")
	}
	stdout, stderr, code := surqlCommand(t, "db", "ping")
	if code != 0 {
		t.Fatalf("db ping exit %d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "reachable") {
		t.Fatalf("expected success marker, got %q", stdout)
	}
}

// TestIntegration_MigrateStatus runs the status subcommand against an
// empty migrations directory. The history table is created on first use
// by the migration runtime.
func TestIntegration_MigrateStatus(t *testing.T) {
	if os.Getenv("SURQL_URL") == "" {
		t.Skip("SURQL_URL not set; skipping migrate status integration test")
	}
	dir := t.TempDir()
	stdout, stderr, code := surqlCommand(t, "migrate", "status", "-d", dir)
	if code != 0 {
		t.Fatalf("migrate status exit %d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "total") {
		t.Fatalf("expected status output, got %q", stdout)
	}
}

// TestIntegration_SchemaTables lists tables; the set may be empty but the
// command must not error.
func TestIntegration_SchemaTables(t *testing.T) {
	if os.Getenv("SURQL_URL") == "" {
		t.Skip("SURQL_URL not set; skipping schema tables integration test")
	}
	stdout, stderr, code := surqlCommand(t, "schema", "tables")
	if code != 0 {
		t.Fatalf("schema tables exit %d stdout=%q stderr=%q", code, stdout, stderr)
	}
	_ = stdout
}
