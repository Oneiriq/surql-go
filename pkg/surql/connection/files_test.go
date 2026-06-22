package connection

import (
	"context"
	"testing"

	"github.com/Oneiriq/surql-go/pkg/surql/types"
)

// recordingRunner is a fake queryRunner that captures the SurrealQL and bound
// vars of the last call and returns a canned envelope. It lets the Bucket file
// operations be unit-tested without a live server: we assert the exact query
// text and that every caller value is bound (never interpolated).
type recordingRunner struct {
	lastSurql string
	lastVars  map[string]any
	result    any
	err       error
}

func (r *recordingRunner) QueryWithVars(_ context.Context, surql string, vars map[string]any) (any, error) {
	r.lastSurql = surql
	r.lastVars = vars
	if r.err != nil {
		return nil, r.err
	}
	// Wrap result in the per-statement envelope shape DatabaseClient produces.
	return []any{map[string]any{"status": "OK", "result": r.result}}, nil
}

func newTestBucket(result any) (*Bucket, *recordingRunner) {
	r := &recordingRunner{result: result}
	return &Bucket{runner: r, name: "assets"}, r
}

func TestBucket_Put_BindsVars(t *testing.T) {
	b, r := newTestBucket(nil)
	if err := b.Put(context.Background(), "a.txt", "hello"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if r.lastSurql != "RETURN type::file($bucket, $key).put($data);" {
		t.Errorf("surql = %q", r.lastSurql)
	}
	if r.lastVars["bucket"] != "assets" || r.lastVars["key"] != "a.txt" || r.lastVars["data"] != "hello" {
		t.Errorf("vars = %v", r.lastVars)
	}
}

func TestBucket_Put_BytesBoundDirectly(t *testing.T) {
	b, r := newTestBucket(nil)
	payload := []byte{0x00, 0x01, 0x02}
	if err := b.Put(context.Background(), "blob", payload); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, ok := r.lastVars["data"].([]byte)
	if !ok {
		t.Fatalf("data var type = %T, want []byte", r.lastVars["data"])
	}
	if len(got) != 3 || got[0] != 0 || got[2] != 2 {
		t.Errorf("data var = %v", got)
	}
}

func TestBucket_Put_RejectsBadType(t *testing.T) {
	b, _ := newTestBucket(nil)
	if err := b.Put(context.Background(), "k", 12345); err == nil {
		t.Error("expected error for int payload")
	}
}

func TestBucket_PutIfNotExists_Surql(t *testing.T) {
	b, r := newTestBucket(nil)
	if err := b.PutIfNotExists(context.Background(), "k", []byte("x")); err != nil {
		t.Fatalf("PutIfNotExists: %v", err)
	}
	if r.lastSurql != "RETURN type::file($bucket, $key).put_if_not_exists($data);" {
		t.Errorf("surql = %q", r.lastSurql)
	}
}

func TestBucket_Get_Bytes(t *testing.T) {
	b, r := newTestBucket([]byte("file-bytes"))
	got, err := b.Get(context.Background(), "k")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != "file-bytes" {
		t.Errorf("Get = %q", got)
	}
	if r.lastSurql != "RETURN type::file($bucket, $key).get();" {
		t.Errorf("surql = %q", r.lastSurql)
	}
}

func TestBucket_Get_Nil(t *testing.T) {
	b, _ := newTestBucket(nil)
	got, err := b.Get(context.Background(), "k")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != nil {
		t.Errorf("Get = %v, want nil", got)
	}
}

func TestBucket_GetText_Cast(t *testing.T) {
	b, r := newTestBucket("hello text")
	got, err := b.GetText(context.Background(), "k")
	if err != nil {
		t.Fatalf("GetText: %v", err)
	}
	if got != "hello text" {
		t.Errorf("GetText = %q", got)
	}
	if r.lastSurql != "RETURN <string>type::file($bucket, $key).get();" {
		t.Errorf("surql = %q", r.lastSurql)
	}
}

func TestBucket_Exists(t *testing.T) {
	b, r := newTestBucket(true)
	ok, err := b.Exists(context.Background(), "k")
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if !ok {
		t.Error("Exists = false, want true")
	}
	if r.lastSurql != "RETURN type::file($bucket, $key).exists();" {
		t.Errorf("surql = %q", r.lastSurql)
	}
}

func TestBucket_Head(t *testing.T) {
	// Head projects file::bucket/file::key off head() and SELECTs them, so the
	// result is a single-row set carrying canonical bucket/key plus metadata.
	b, r := newTestBucket([]any{
		map[string]any{"bucket": "assets", "key": "/k", "size": int64(10)},
	})
	meta, err := b.Head(context.Background(), "k")
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	if meta["size"] != int64(10) {
		t.Errorf("Head size = %v", meta)
	}
	if meta["key"] != "/k" {
		t.Errorf("Head key = %v, want canonical /k", meta["key"])
	}
	want := "SELECT file::bucket(file) AS bucket, file::key(file) AS key, size, updated " +
		"FROM type::file($bucket, $key).head();"
	if r.lastSurql != want {
		t.Errorf("surql = %q", r.lastSurql)
	}
}

func TestBucket_Head_Missing(t *testing.T) {
	// An empty row set means the file does not exist -> nil, no error.
	b, _ := newTestBucket([]any{})
	meta, err := b.Head(context.Background(), "missing")
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	if meta != nil {
		t.Errorf("Head(missing) = %v, want nil", meta)
	}
}

func TestBucket_Delete(t *testing.T) {
	b, r := newTestBucket(nil)
	if err := b.Delete(context.Background(), "k"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if r.lastSurql != "RETURN type::file($bucket, $key).delete();" {
		t.Errorf("surql = %q", r.lastSurql)
	}
}

func TestBucket_Copy_BindsDst(t *testing.T) {
	b, r := newTestBucket(nil)
	if err := b.Copy(context.Background(), "src", "dst"); err != nil {
		t.Fatalf("Copy: %v", err)
	}
	if r.lastSurql != "RETURN type::file($bucket, $key).copy($dst);" {
		t.Errorf("surql = %q", r.lastSurql)
	}
	if r.lastVars["key"] != "src" || r.lastVars["dst"] != "dst" {
		t.Errorf("vars = %v", r.lastVars)
	}
}

func TestBucket_CopyIfNotExists_Surql(t *testing.T) {
	b, r := newTestBucket(nil)
	if err := b.CopyIfNotExists(context.Background(), "src", "dst"); err != nil {
		t.Fatalf("CopyIfNotExists: %v", err)
	}
	if r.lastSurql != "RETURN type::file($bucket, $key).copy_if_not_exists($dst);" {
		t.Errorf("surql = %q", r.lastSurql)
	}
}

func TestBucket_Rename_Surql(t *testing.T) {
	b, r := newTestBucket(nil)
	if err := b.Rename(context.Background(), "src", "dst"); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if r.lastSurql != "RETURN type::file($bucket, $key).rename($dst);" {
		t.Errorf("surql = %q", r.lastSurql)
	}
}

func TestBucket_RenameIfNotExists_Surql(t *testing.T) {
	b, r := newTestBucket(nil)
	if err := b.RenameIfNotExists(context.Background(), "src", "dst"); err != nil {
		t.Fatalf("RenameIfNotExists: %v", err)
	}
	if r.lastSurql != "RETURN type::file($bucket, $key).rename_if_not_exists($dst);" {
		t.Errorf("surql = %q", r.lastSurql)
	}
}

func TestBucket_List_ParsesRefs(t *testing.T) {
	// List projects file::bucket/file::key, so each row is a {bucket, key, ...}
	// object carrying canonical keys (leading slash). Keys are preserved
	// verbatim — the leading slash is not stripped.
	entries := []any{
		map[string]any{"bucket": "assets", "key": "/a.txt", "size": int64(3)},
		map[string]any{"bucket": "assets", "key": "/nested/b.txt"},
		// Blank bucket field backfills from the queried bucket name.
		map[string]any{"bucket": "", "key": "/c.txt"},
	}
	b, r := newTestBucket(entries)
	refs, err := b.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := "SELECT file::bucket(file) AS bucket, file::key(file) AS key, size, updated " +
		"FROM file::list($bucket);"
	if r.lastSurql != want {
		t.Errorf("surql = %q", r.lastSurql)
	}
	if r.lastVars["bucket"] != "assets" {
		t.Errorf("bucket var = %v", r.lastVars["bucket"])
	}
	wantRefs := []types.FileRef{
		{Bucket: "assets", Key: "/a.txt"},
		{Bucket: "assets", Key: "/nested/b.txt"},
		{Bucket: "assets", Key: "/c.txt"},
	}
	if len(refs) != len(wantRefs) {
		t.Fatalf("got %d refs, want %d: %+v", len(refs), len(wantRefs), refs)
	}
	for i := range wantRefs {
		if refs[i] != wantRefs[i] {
			t.Errorf("ref[%d] = %+v, want %+v", i, refs[i], wantRefs[i])
		}
	}
}

func TestBucket_List_FallbackRawPointer(t *testing.T) {
	// Resilience: an unprojected row carrying a raw `file` pointer is still
	// decoded (map, canonical string, or bare key forms).
	entries := []any{
		map[string]any{"file": map[string]any{"bucket": "assets", "key": "/a.txt"}},
		map[string]any{"file": "assets:/b.txt"},
		map[string]any{"file": "c.txt"},
	}
	b, _ := newTestBucket(entries)
	refs, err := b.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := []types.FileRef{
		{Bucket: "assets", Key: "/a.txt"},
		{Bucket: "assets", Key: "b.txt"},
		{Bucket: "assets", Key: "c.txt"},
	}
	if len(refs) != len(want) {
		t.Fatalf("got %d refs, want %d: %+v", len(refs), len(want), refs)
	}
	for i := range want {
		if refs[i] != want[i] {
			t.Errorf("ref[%d] = %+v, want %+v", i, refs[i], want[i])
		}
	}
}

func TestBucket_List_Empty(t *testing.T) {
	b, _ := newTestBucket(nil)
	refs, err := b.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if refs != nil {
		t.Errorf("List = %v, want nil", refs)
	}
}

func TestDatabaseClient_Bucket_Accessor(t *testing.T) {
	c := &DatabaseClient{}
	h := c.Bucket("assets")
	if h.Name() != "assets" {
		t.Errorf("Name() = %q", h.Name())
	}
}
