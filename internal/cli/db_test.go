package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestResolveQueryText exercises the --file / inline argument selection.
func TestResolveQueryText(t *testing.T) {
	t.Run("inline", func(t *testing.T) {
		got, err := resolveQueryText([]string{"SELECT * FROM foo"}, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(got, "SELECT") {
			t.Fatalf("unexpected query text: %q", got)
		}
	})

	t.Run("from-file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "query.surql")
		if err := os.WriteFile(path, []byte("INFO FOR DB;\n"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		got, err := resolveQueryText(nil, path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "INFO FOR DB;" {
			t.Errorf("unexpected text after trim: %q", got)
		}
	})

	t.Run("missing-file", func(t *testing.T) {
		_, err := resolveQueryText(nil, "/definitely/not/here.surql")
		if err == nil {
			t.Fatal("expected error for missing file")
		}
	})

	t.Run("empty-file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "q.surql")
		if err := os.WriteFile(path, []byte("   \n\t\n"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		_, err := resolveQueryText(nil, path)
		if err == nil {
			t.Fatal("expected error for empty file")
		}
	})

	t.Run("no-args-no-file", func(t *testing.T) {
		_, err := resolveQueryText(nil, "")
		if err == nil {
			t.Fatal("expected error when neither arg nor file provided")
		}
	})
}

// TestExtractTableNames covers the INFO FOR DB parsing that backs `db
// reset` and `schema tables`.
func TestExtractTableNames(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want []string
	}{
		{
			name: "nil",
			in:   nil,
			want: nil,
		},
		{
			name: "v3-tables-key",
			in: []any{
				map[string]any{
					"result": map[string]any{
						"tables": map[string]any{
							"user":    "DEFINE TABLE user;",
							"account": "DEFINE TABLE account;",
						},
					},
				},
			},
			want: []string{"account", "user"},
		},
		{
			name: "v2-tb-key",
			in: []any{
				map[string]any{
					"result": map[string]any{
						"tb": map[string]any{
							"product": "DEFINE TABLE product;",
						},
					},
				},
			},
			want: []string{"product"},
		},
		{
			name: "empty",
			in:   []any{},
			want: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractTableNames(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("length mismatch: got %v, want %v", got, tc.want)
			}
			for i, n := range got {
				if n != tc.want[i] {
					t.Errorf("index %d: got %q, want %q", i, n, tc.want[i])
				}
			}
		})
	}
}

// TestOptionalString renders "(none)" for nil pointers and the value
// otherwise.
func TestOptionalString(t *testing.T) {
	if got := optionalString(nil); got != "(none)" {
		t.Errorf("nil -> %q", got)
	}
	v := "alice"
	if got := optionalString(&v); got != "alice" {
		t.Errorf("set -> %q", got)
	}
}

// TestMaskOptionalString masks set values and leaves nil as (none).
func TestMaskOptionalString(t *testing.T) {
	if got := maskOptionalString(nil); got != "(none)" {
		t.Errorf("nil -> %q", got)
	}
	empty := ""
	if got := maskOptionalString(&empty); got != "(none)" {
		t.Errorf("empty string -> %q", got)
	}
	secret := "hunter2"
	if got := maskOptionalString(&secret); got != "***" {
		t.Errorf("set -> %q, want ***", got)
	}
}

// TestSortStringsStable confirms insertion-sort ordering.
func TestSortStringsStable(t *testing.T) {
	in := []string{"charlie", "alpha", "bravo"}
	got := sortStringsStable(in)
	want := []string{"alpha", "bravo", "charlie"}
	for i, s := range got {
		if s != want[i] {
			t.Errorf("index %d: got %q, want %q", i, s, want[i])
		}
	}
	// Input should not be mutated.
	if in[0] != "charlie" {
		t.Errorf("sortStringsStable mutated its input: %v", in)
	}
}

// TestDBReset_RequiresYes ensures we refuse to run a destructive reset
// without the --yes flag, surfacing a usage error (not a connection
// attempt).
func TestDBReset_RequiresYes(t *testing.T) {
	tmp := t.TempDir()
	origCwd, _ := os.Getwd()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(origCwd) //nolint:errcheck

	// Provide a bogus URL so that if --yes handling broke the command
	// would still fail fast rather than dialing a real DB.
	os.Setenv("SURQL_URL", "ws://127.0.0.1:0")
	defer os.Unsetenv("SURQL_URL")

	args := []string{"db", "reset", "--no-color"}
	code := ExecuteWithArgs(BuildInfo{Version: "test"}, args, nilWriter{}, nilWriter{})
	if code != ExitUsage {
		t.Fatalf("db reset without --yes should be usage error, got %d", code)
	}
}

// nilWriter discards all writes and is used by tests that do not care
// about captured output.
type nilWriter struct{}

func (nilWriter) Write(p []byte) (int, error) { return len(p), nil }
