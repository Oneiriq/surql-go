package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Oneiriq/surql-go/pkg/surql/migration"
	"github.com/Oneiriq/surql-go/pkg/surql/schema"
)

// TestLoadSnapshotFromReference_DirectFile passes an explicit path and
// ensures it round-trips.
func TestLoadSnapshotFromReference_DirectFile(t *testing.T) {
	dir := t.TempDir()
	snap := migration.SchemaSnapshot{
		Version:     "20260101_000000",
		Description: "seed",
	}
	raw, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	path := filepath.Join(dir, "seed.snapshot.json")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := loadSnapshotFromReference(dir, path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Version != snap.Version {
		t.Errorf("version mismatch: got %q want %q", got.Version, snap.Version)
	}
}

// TestLoadSnapshotFromReference_VersionLookup resolves a bare version ref
// to <snapshotsDir>/<version>.snapshot.json.
func TestLoadSnapshotFromReference_VersionLookup(t *testing.T) {
	dir := t.TempDir()
	snap := migration.SchemaSnapshot{Version: "20260201_010101", Description: "v2"}
	raw, _ := json.Marshal(snap)
	path := filepath.Join(dir, "20260201_010101.snapshot.json")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := loadSnapshotFromReference(dir, "20260201_010101")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Description != "v2" {
		t.Errorf("description mismatch: got %q", got.Description)
	}
}

// TestLoadSnapshotFromReference_Missing returns an error for an unknown
// snapshot reference.
func TestLoadSnapshotFromReference_Missing(t *testing.T) {
	dir := t.TempDir()
	_, err := loadSnapshotFromReference(dir, "does_not_exist")
	if err == nil {
		t.Fatal("expected error for missing snapshot")
	}
}

// TestEscapeYAMLRegex escapes dots and backslashes.
func TestEscapeYAMLRegex(t *testing.T) {
	cases := []struct{ in, want string }{
		{"schemas/", "schemas/"},
		{"a.b", `a\.b`},
		{`dir\sub`, `dir\\sub`},
	}
	for _, tc := range cases {
		if got := escapeYAMLRegex(tc.in); got != tc.want {
			t.Errorf("escapeYAMLRegex(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}

// TestExtractTableNamesFromInfo wraps the INFO FOR DB parser used by
// liveSnapshot.
func TestExtractTableNamesFromInfo(t *testing.T) {
	in := map[string]any{
		"tables": map[string]any{
			"user":    "DEFINE TABLE user;",
			"account": "DEFINE TABLE account;",
		},
	}
	got := extractTableNamesFromInfo(in)
	if len(got) != 2 || got[0] != "account" || got[1] != "user" {
		t.Errorf("unexpected sorted names: %v", got)
	}
}

// TestUnwrapQueryResult walks the canonical client envelope.
func TestUnwrapQueryResult(t *testing.T) {
	env := []any{
		map[string]any{
			"status": "OK",
			"result": map[string]any{"key": "value"},
		},
	}
	got := unwrapQueryResult(env)
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", got)
	}
	if m["key"] != "value" {
		t.Errorf("payload mismatch: %v", m)
	}
	if unwrapQueryResult(nil) != nil {
		t.Error("nil envelope should unwrap to nil")
	}
	if unwrapQueryResult([]any{}) != nil {
		t.Error("empty envelope should unwrap to nil")
	}
}

// TestSchemaGenerate_EmptyRegistry confirms the command emits a warning
// and returns ExitSuccess when no tables/edges are registered.
func TestSchemaGenerate_EmptyRegistry(t *testing.T) {
	// Reset the global registry for test isolation.
	schema.GetRegistry().Clear()
	tmp := t.TempDir()
	origCwd, _ := os.Getwd()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(origCwd) //nolint:errcheck

	args := []string{"schema", "generate", "--no-color"}
	code := ExecuteWithArgs(BuildInfo{Version: "test"}, args, nilWriter{}, nilWriter{})
	if code != ExitSuccess {
		t.Fatalf("generate with empty registry should succeed, got %d", code)
	}
}
