//go:build integration
// +build integration

package query

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestIntegration_TableMissingErrorShape pins the server wording that
// isTableMissingError depends on. v2.x returned an empty result for
// queries against a never-materialised table; v3.0.x returns a
// QueryError whose message is "The table '<name>' does not exist".
//
// The helper must classify the error as "table missing" and the raw
// error text must contain the exact v3 phrase. If SurrealDB changes
// either the prefix or the "does not exist" tail this test fails
// loudly, signalling that the runtime fallbacks in crud.go (GetRecord,
// DeleteRecord, CountRecords, Exists) need a new needle.
func TestIntegration_TableMissingErrorShape(t *testing.T) {
	client, cleanup := newIntegrationClient(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	const missing = "surqlgo_does_not_exist_xyz"

	// Probe the raw query path: this is what GetRecord / DeleteRecord /
	// CountRecords ultimately hit. We do NOT pre-create and remove the
	// table here because REMOVE TABLE IF EXISTS silently succeeds for a
	// never-materialised table; we want the server's genuine "missing"
	// error path.
	_, err := client.Query(ctx, "SELECT * FROM "+missing+";")
	if err == nil {
		t.Fatalf("expected error for SELECT from %q, got nil", missing)
	}

	// The runtime helper must classify this as a table-missing error.
	if !isTableMissingError(err) {
		t.Fatalf("isTableMissingError=false for %q", err.Error())
	}

	// Pin to the exact v3 wording. Case-sensitive here: we want to
	// detect *any* drift -- capitalisation, quote style, or preposition.
	msg := err.Error()
	if !strings.Contains(msg, "The table '"+missing+"' does not exist") {
		t.Fatalf("v3 server wording changed; got %q, expected substring %q",
			msg, "The table '"+missing+"' does not exist")
	}

	// Also assert the two halves independently so the failure mode is
	// clear when only one end of the phrase shifts.
	if !strings.Contains(msg, "The table '") {
		t.Errorf("missing v3 prefix %q in %q", "The table '", msg)
	}
	if !strings.Contains(msg, "does not exist") {
		t.Errorf("missing v3 suffix %q in %q", "does not exist", msg)
	}
}

// TestIntegration_TableMissingErrorHelpersNoOp confirms that the public
// helpers which branch on isTableMissingError degrade to the
// v2-compatible "empty" answer when the table is missing on v3+.
func TestIntegration_TableMissingErrorHelpersNoOp(t *testing.T) {
	client, cleanup := newIntegrationClient(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	const missing = "surqlgo_missing_helpers_xyz"

	rec, err := GetRecord(ctx, client, missing, "alice")
	if err != nil {
		t.Fatalf("GetRecord: %v", err)
	}
	if rec != nil {
		t.Errorf("GetRecord want nil, got %+v", rec)
	}

	ok, err := Exists(ctx, client, missing, "alice")
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if ok {
		t.Error("Exists want false")
	}

	n, err := CountRecords(ctx, client, missing, nil)
	if err != nil {
		t.Fatalf("CountRecords: %v", err)
	}
	if n != 0 {
		t.Errorf("CountRecords want 0, got %d", n)
	}

	if err := DeleteRecord(ctx, client, missing, "alice"); err != nil {
		t.Fatalf("DeleteRecord: %v", err)
	}
}
