package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeQueryClient returns a canned response for INFO FOR DB, letting the
// bucket list helper be unit-tested without a live database.
type fakeQueryClient struct {
	res any
	err error
}

func (f fakeQueryClient) Query(_ context.Context, _ string) (any, error) {
	return f.res, f.err
}

func TestListBuckets(t *testing.T) {
	// Envelope shape produced by DatabaseClient.Query for INFO FOR DB.
	res := []any{
		map[string]any{
			"result": map[string]any{
				"buckets": map[string]any{
					"zeta":  `DEFINE BUCKET zeta BACKEND "memory"`,
					"alpha": `DEFINE BUCKET alpha BACKEND "memory"`,
				},
			},
		},
	}
	names, err := listBuckets(context.Background(), fakeQueryClient{res: res})
	if err != nil {
		t.Fatalf("listBuckets: %v", err)
	}
	if len(names) != 2 || names[0] != "alpha" || names[1] != "zeta" {
		t.Errorf("names = %v, want sorted [alpha zeta]", names)
	}
}

func TestListBuckets_Empty(t *testing.T) {
	names, err := listBuckets(context.Background(), fakeQueryClient{res: []any{}})
	if err != nil {
		t.Fatalf("listBuckets: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("names = %v, want empty", names)
	}
}

// TestBucketGroup_Subcommands enumerates every bucket subcommand so the parity
// matrix stays enforced.
func TestBucketGroup_Subcommands(t *testing.T) {
	root := NewRootCommand(BuildInfo{Version: "test"})
	bucket, _, err := root.Find([]string{"bucket"})
	if err != nil {
		t.Fatalf("find bucket: %v", err)
	}
	want := []string{"define", "list", "rm", "put", "get", "delete", "exists"}
	got := make(map[string]bool)
	for _, c := range bucket.Commands() {
		got[c.Name()] = true
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("bucket missing subcommand %q", w)
		}
	}
}

func TestResolveBucketPayload(t *testing.T) {
	// inline data
	got, err := resolveBucketPayload("hello", "")
	if err != nil {
		t.Fatalf("resolveBucketPayload(data): %v", err)
	}
	if string(got) != "hello" {
		t.Errorf("payload = %q", got)
	}

	// from file
	dir := t.TempDir()
	path := filepath.Join(dir, "f.bin")
	if err := os.WriteFile(path, []byte{1, 2, 3}, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err = resolveBucketPayload("", path)
	if err != nil {
		t.Fatalf("resolveBucketPayload(file): %v", err)
	}
	if len(got) != 3 || got[0] != 1 {
		t.Errorf("payload = %v", got)
	}

	// neither
	if _, err := resolveBucketPayload("", ""); err == nil {
		t.Error("expected error when neither --data nor --file given")
	}
	// both
	if _, err := resolveBucketPayload("d", path); err == nil {
		t.Error("expected error when both --data and --file given")
	}
	// missing file
	if _, err := resolveBucketPayload("", filepath.Join(dir, "nope")); err == nil {
		t.Error("expected error for missing file")
	}
}

// TestBucketDefine_DryRun drives `bucket define --dry-run` end-to-end and
// asserts the emitted SurrealQL without needing a database connection.
func TestBucketDefine_DryRun(t *testing.T) {
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	code := ExecuteWithArgs(BuildInfo{Version: "test"},
		[]string{"bucket", "define", "assets", "--backend", "memory", "--readonly", "--dry-run"},
		out, errOut)
	if code != ExitSuccess {
		t.Fatalf("exit code = %d, stderr=%s", code, errOut.String())
	}
	if want := `DEFINE BUCKET assets BACKEND "memory" READONLY;`; !strings.Contains(out.String(), want) {
		t.Errorf("dry-run output missing %q:\n%s", want, out.String())
	}
}

// TestBucketDefine_InvalidName rejects a bad bucket name as a usage error
// before any connection is attempted.
func TestBucketDefine_InvalidName(t *testing.T) {
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	code := ExecuteWithArgs(BuildInfo{Version: "test"},
		[]string{"bucket", "define", "1bad", "--dry-run"},
		out, errOut)
	if code != ExitUsage {
		t.Fatalf("exit code = %d, want ExitUsage; stderr=%s", code, errOut.String())
	}
}

// TestBucketRemove_RequiresConfirmation ensures rm without --yes is a usage
// error and never connects.
func TestBucketRemove_RequiresConfirmation(t *testing.T) {
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	code := ExecuteWithArgs(BuildInfo{Version: "test"},
		[]string{"bucket", "rm", "assets"},
		out, errOut)
	if code != ExitUsage {
		t.Fatalf("exit code = %d, want ExitUsage; stderr=%s", code, errOut.String())
	}
}
