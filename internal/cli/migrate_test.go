package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/Oneiriq/surql-go/pkg/surql/migration"
)

// TestClampPendingToTarget verifies the target clamp returns the correct
// prefix and leaves the input unchanged when the target is unknown.
func TestClampPendingToTarget(t *testing.T) {
	pending := []migration.Migration{
		{Version: "20260101_000000", Description: "a"},
		{Version: "20260102_000000", Description: "b"},
		{Version: "20260103_000000", Description: "c"},
	}
	cases := []struct {
		name   string
		target string
		want   []string
	}{
		{"first", "20260101_000000", []string{"20260101_000000"}},
		{"middle", "20260102_000000", []string{"20260101_000000", "20260102_000000"}},
		{"last", "20260103_000000", []string{
			"20260101_000000", "20260102_000000", "20260103_000000",
		}},
		{"unknown", "99999999_999999", []string{
			"20260101_000000", "20260102_000000", "20260103_000000",
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := clampPendingToTarget(pending, tc.target)
			if len(got) != len(tc.want) {
				t.Fatalf("length mismatch: got %d, want %d", len(got), len(tc.want))
			}
			for i, m := range got {
				if m.Version != tc.want[i] {
					t.Errorf("index %d: got %q, want %q", i, m.Version, tc.want[i])
				}
			}
		})
	}
}

// TestMigrateCreate_WritesFile uses the migrate create command to produce
// a file in a temporary directory and validates the generated migration
// is discoverable by the discovery pass.
func TestMigrateCreate_WritesFile(t *testing.T) {
	dir := t.TempDir()
	build := BuildInfo{Version: "test"}
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}

	// Use --no-color so the table / marker output is ASCII-only.
	args := []string{"migrate", "create", "add users table", "-d", dir, "--no-color"}

	// Point the settings resolver at a fresh cwd so there's no config file.
	tmpCwd := t.TempDir()
	origCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmpCwd); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(origCwd) //nolint:errcheck

	if code := ExecuteWithArgs(build, args, out, errOut); code != ExitSuccess {
		t.Fatalf("migrate create failed: code=%d stderr=%q", code, errOut.String())
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 migration, got %d: %v", len(entries), entries)
	}
	migrations, err := migration.DiscoverMigrations(dir)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(migrations) != 1 {
		t.Fatalf("expected discovery to see 1 migration, got %d", len(migrations))
	}
	got := migrations[0]
	if got.Description != "add users table" {
		t.Errorf("description = %q, want %q", got.Description, "add users table")
	}
}

// TestMigrateValidate_PassesOnEmptyDir ensures validate treats an empty
// directory as valid (no files to validate).
func TestMigrateValidate_PassesOnEmptyDir(t *testing.T) {
	dir := t.TempDir()
	tmpCwd := t.TempDir()
	origCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmpCwd); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(origCwd) //nolint:errcheck

	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	args := []string{"migrate", "validate", "-d", dir, "--no-color"}
	code := ExecuteWithArgs(BuildInfo{Version: "test"}, args, out, errOut)
	if code != ExitSuccess {
		t.Fatalf("validate empty dir should succeed, got code=%d stderr=%q", code, errOut.String())
	}
}

// TestMigrateValidate_DetectsDuplicates sets up two migrations with the
// same version so the validator should report a duplicate.
func TestMigrateValidate_DetectsDuplicates(t *testing.T) {
	dir := t.TempDir()
	// Write two files with the same timestamp prefix.
	names := []string{"20260101_000000_one.surql", "20260101_000000_two.surql"}
	for _, n := range names {
		path := filepath.Join(dir, n)
		body := "-- @up\nDEFINE TABLE foo SCHEMAFULL;\n-- @down\nREMOVE TABLE foo;\n"
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	tmpCwd := t.TempDir()
	origCwd, _ := os.Getwd()
	if err := os.Chdir(tmpCwd); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(origCwd) //nolint:errcheck

	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	args := []string{"migrate", "validate", "-d", dir, "--no-color"}
	code := ExecuteWithArgs(BuildInfo{Version: "test"}, args, out, errOut)
	if code == ExitSuccess {
		t.Fatalf("validate should fail for duplicate versions, stdout=%q stderr=%q",
			out.String(), errOut.String())
	}
}

// TestMigrateDown_UsageError asserts the down command rejects steps=0
// without --target.
func TestMigrateDown_UsageError(t *testing.T) {
	tmpCwd := t.TempDir()
	origCwd, _ := os.Getwd()
	if err := os.Chdir(tmpCwd); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(origCwd) //nolint:errcheck

	// Use a non-connectable URL so we exit before actually running.
	os.Setenv("SURQL_URL", "ws://127.0.0.1:0")
	defer os.Unsetenv("SURQL_URL")

	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	// Provide a real directory so validation does not short-circuit.
	dir := t.TempDir()
	args := []string{"migrate", "down", "-d", dir, "--steps", "0", "--no-color"}
	code := ExecuteWithArgs(BuildInfo{Version: "test"}, args, out, errOut)
	if code != ExitUsage && code != ExitFailure {
		t.Fatalf("expected usage or failure exit, got %d (stderr=%q)", code, errOut.String())
	}
}

// TestParseUint32 covers the helper used for uint flag parsing. Delta
// tests confirm both success and malformed cases.
func TestParseUint32(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    uint32
		wantErr bool
	}{
		{"zero", "0", 0, false},
		{"positive", "42", 42, false},
		{"negative", "-1", 0, true},
		{"nonnumeric", "abc", 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseUint32(tc.in)
			if tc.wantErr && err == nil {
				t.Errorf("expected error for %q", tc.in)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error %v", err)
			}
			if !tc.wantErr && got != tc.want {
				t.Errorf("got %d, want %d", got, tc.want)
			}
		})
	}
}
