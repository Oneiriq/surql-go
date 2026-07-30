//go:build integration
// +build integration

package connection

import (
	"bytes"
	"context"
	"testing"
	"time"
)

// defineFilesBucket creates a fresh in-memory bucket for a file test. The
// server must be started with the experimental files capability enabled via
// the SURREAL_CAPS_ALLOW_EXPERIMENTAL=files environment variable, e.g.
//
//	$env:SURREAL_CAPS_ALLOW_EXPERIMENTAL='files'
//	surreal start --bind 127.0.0.1:8202 --user root --pass root --allow-all memory
//
// (The `--allow-experimental files` *flag* form swallows the `memory`
// datastore positional argument, so prefer the env var.) When the DEFINE fails
// because the capability is disabled, the test is skipped rather than failed.
func defineFilesBucket(t *testing.T, client *DatabaseClient, bucket string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stmt := "REMOVE BUCKET IF EXISTS " + bucket + "; DEFINE BUCKET " + bucket + ` BACKEND "memory";`
	if _, err := client.Query(ctx, stmt); err != nil {
		t.Skipf("DEFINE BUCKET failed (start server with SURREAL_CAPS_ALLOW_EXPERIMENTAL=files): %v", err)
	}
}

func TestIntegration_BucketRoundTrip(t *testing.T) {
	client, cleanup := newIntegrationClient(t)
	defer cleanup()

	const bucket = "surqlgo_files_test"
	defineFilesBucket(t, client, bucket)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	b := client.Bucket(bucket)

	// Put + Get a string.
	if err := b.Put(ctx, "hello.txt", "hello world"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, err := b.Get(ctx, "hello.txt")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != "hello world" {
		t.Errorf("Get = %q, want hello world", got)
	}

	// GetText.
	text, err := b.GetText(ctx, "hello.txt")
	if err != nil {
		t.Fatalf("GetText: %v", err)
	}
	if text != "hello world" {
		t.Errorf("GetText = %q", text)
	}

	// Put raw bytes and read them back unchanged.
	raw := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	if err := b.Put(ctx, "blob.bin", raw); err != nil {
		t.Fatalf("Put bytes: %v", err)
	}
	gotRaw, err := b.Get(ctx, "blob.bin")
	if err != nil {
		t.Fatalf("Get bytes: %v", err)
	}
	if !bytes.Equal(gotRaw, raw) {
		t.Errorf("Get bytes = %v, want %v", gotRaw, raw)
	}

	// Exists.
	exists, err := b.Exists(ctx, "hello.txt")
	if err != nil || !exists {
		t.Fatalf("Exists = %v, err = %v", exists, err)
	}
	missing, err := b.Exists(ctx, "nope.txt")
	if err != nil {
		t.Fatalf("Exists(missing): %v", err)
	}
	if missing {
		t.Error("Exists(missing) = true")
	}

	// PutIfNotExists must not overwrite.
	if err := b.PutIfNotExists(ctx, "hello.txt", "OVERWRITTEN"); err != nil {
		t.Fatalf("PutIfNotExists: %v", err)
	}
	after, _ := b.GetText(ctx, "hello.txt")
	if after != "hello world" {
		t.Errorf("PutIfNotExists overwrote existing file: %q", after)
	}

	// Head returns canonical metadata: bucket, the canonical key (leading
	// slash), and size. The file was written as "hello.txt" so the key is
	// reported as "/hello.txt".
	head, err := b.Head(ctx, "hello.txt")
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	if head == nil {
		t.Fatal("Head returned nil for existing file")
	}
	if head["key"] != "/hello.txt" {
		t.Errorf("Head key = %v, want canonical /hello.txt", head["key"])
	}
	if head["bucket"] != bucket {
		t.Errorf("Head bucket = %v, want %q", head["bucket"], bucket)
	}

	// Head on a missing file returns nil (empty projection row set), no error.
	missingHead, err := b.Head(ctx, "does-not-exist.txt")
	if err != nil {
		t.Fatalf("Head(missing): %v", err)
	}
	if missingHead != nil {
		t.Errorf("Head(missing) = %v, want nil", missingHead)
	}

	// Keys accepted as-given: a leading-slash key addresses the same file as
	// the bare key it was written under (server-side normalisation).
	slashGet, err := b.GetText(ctx, "/hello.txt")
	if err != nil {
		t.Fatalf("GetText(/hello.txt): %v", err)
	}
	if slashGet != "hello world" {
		t.Errorf("GetText(/hello.txt) = %q, want hello world", slashGet)
	}
}

func TestIntegration_BucketCopyRenameTargetSemantics(t *testing.T) {
	client, cleanup := newIntegrationClient(t)
	defer cleanup()

	const bucket = "surqlgo_files_copyrename"
	defineFilesBucket(t, client, bucket)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	b := client.Bucket(bucket)

	if err := b.Put(ctx, "src.txt", "payload"); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Copy: the destination argument is a key within the same bucket.
	if err := b.Copy(ctx, "src.txt", "copy.txt"); err != nil {
		t.Fatalf("Copy: %v", err)
	}
	copied, err := b.GetText(ctx, "copy.txt")
	if err != nil {
		t.Fatalf("GetText(copy): %v", err)
	}
	if copied != "payload" {
		t.Errorf("copy content = %q, want payload", copied)
	}
	// Source still present after copy.
	if ok, _ := b.Exists(ctx, "src.txt"); !ok {
		t.Error("source missing after Copy")
	}

	// CopyIfNotExists into an existing key is a no-op on SurrealDB 3.1.3: the
	// server does not raise an error, it simply declines to overwrite. Verified
	// live — put distinct sentinel content at the destination, attempt the
	// guarded copy, and confirm the sentinel survives untouched.
	if err := b.Put(ctx, "copy.txt", "SENTINEL"); err != nil {
		t.Fatalf("Put(sentinel): %v", err)
	}
	if err := b.CopyIfNotExists(ctx, "src.txt", "copy.txt"); err != nil {
		t.Fatalf("CopyIfNotExists (existing dst should no-op, not error): %v", err)
	}
	if guarded, _ := b.GetText(ctx, "copy.txt"); guarded != "SENTINEL" {
		t.Errorf("CopyIfNotExists overwrote existing dst: %q, want SENTINEL", guarded)
	}
	// CopyIfNotExists into a free key copies.
	if err := b.CopyIfNotExists(ctx, "src.txt", "copy2.txt"); err != nil {
		t.Fatalf("CopyIfNotExists(new dst): %v", err)
	}
	if fresh, _ := b.GetText(ctx, "copy2.txt"); fresh != "payload" {
		t.Errorf("CopyIfNotExists(new dst) content = %q, want payload", fresh)
	}

	// Rename: destination is a key; source disappears.
	if err := b.Rename(ctx, "src.txt", "renamed.txt"); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if ok, _ := b.Exists(ctx, "src.txt"); ok {
		t.Error("source still present after Rename")
	}
	renamed, err := b.GetText(ctx, "renamed.txt")
	if err != nil {
		t.Fatalf("GetText(renamed): %v", err)
	}
	if renamed != "payload" {
		t.Errorf("renamed content = %q", renamed)
	}

	// List returns the files present. Keys are canonical (leading slash), so a
	// file written as "copy.txt" is listed as "/copy.txt".
	refs, err := b.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	keys := map[string]bool{}
	for _, ref := range refs {
		keys[ref.Key] = true
		if ref.Bucket != bucket {
			t.Errorf("ref bucket = %q, want %q", ref.Bucket, bucket)
		}
		// Every listed key is canonical: it begins with a slash.
		if len(ref.Key) == 0 || ref.Key[0] != '/' {
			t.Errorf("listed key %q is not canonical (expected leading slash)", ref.Key)
		}
		// String() must still collapse to a single-slash pointer.
		if want := bucket + ":" + ref.Key; ref.String() != want {
			t.Errorf("ref.String() = %q, want %q", ref.String(), want)
		}
	}
	if !keys["/copy.txt"] || !keys["/renamed.txt"] {
		t.Errorf("List missing expected canonical keys: %v", keys)
	}

	// Delete removes a file.
	if err := b.Delete(ctx, "copy.txt"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if ok, _ := b.Exists(ctx, "copy.txt"); ok {
		t.Error("file present after Delete")
	}
}
