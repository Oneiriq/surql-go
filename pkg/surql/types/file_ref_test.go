package types

import (
	"encoding/json"
	stdErrors "errors"
	"testing"

	"github.com/surrealdb/surrealdb.go/surrealcbor"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

func TestNewFileRef_StoresKeyVerbatim(t *testing.T) {
	// Keys are stored exactly as given; the server normalises them (a leading
	// slash is canonical and "a.png"/"/a.png" resolve to the same file), so the
	// constructor must not strip or rewrite anything.
	if got := NewFileRef("b", "/users/a.png").Key; got != "/users/a.png" {
		t.Errorf("Key = %q, want /users/a.png", got)
	}
	if got := NewFileRef("b", "users/a.png").Key; got != "users/a.png" {
		t.Errorf("Key = %q, want users/a.png", got)
	}
}

func TestFileRef_String(t *testing.T) {
	f := FileRef{Bucket: "avatars", Key: "users/alice.png"}
	if got := f.String(); got != "avatars:/users/alice.png" {
		t.Errorf("String() = %q", got)
	}
	// A canonical key with a leading slash must still render a single-slash
	// pointer — the separator slash is not doubled.
	canon := FileRef{Bucket: "avatars", Key: "/users/alice.png"}
	if got := canon.String(); got != "avatars:/users/alice.png" {
		t.Errorf("String() with leading-slash key = %q, want avatars:/users/alice.png", got)
	}
}

func TestFileRef_ToSurql_Escapes(t *testing.T) {
	f := FileRef{Bucket: "b'1", Key: `k\2`}
	got := f.ToSurql()
	want := `type::file('b\'1', 'k\\2')`
	if got != want {
		t.Errorf("ToSurql() = %q, want %q", got, want)
	}
}

func TestFileRef_Validate(t *testing.T) {
	if err := (FileRef{Bucket: "b", Key: "k"}).Validate(); err != nil {
		t.Errorf("valid ref errored: %v", err)
	}
	if err := (FileRef{Key: "k"}).Validate(); err == nil {
		t.Error("empty bucket should error")
	} else if !stdErrors.Is(err, surqlerrors.ErrValidation) {
		t.Errorf("kind = %v, want ErrValidation", err)
	}
	if err := (FileRef{Bucket: "b"}).Validate(); err == nil {
		t.Error("empty key should error")
	}
}

func TestParseFileRef(t *testing.T) {
	f, err := ParseFileRef("avatars:/users/alice.png")
	if err != nil {
		t.Fatalf("ParseFileRef: %v", err)
	}
	if f.Bucket != "avatars" || f.Key != "users/alice.png" {
		t.Errorf("parsed = %+v", f)
	}

	for _, bad := range []string{"no-separator", ":/key", "bucket:/"} {
		if _, err := ParseFileRef(bad); err == nil {
			t.Errorf("ParseFileRef(%q) should error", bad)
		}
	}
}

func TestParseFileRef_RoundTripWithString(t *testing.T) {
	orig := FileRef{Bucket: "b", Key: "nested/key.txt"}
	parsed, err := ParseFileRef(orig.String())
	if err != nil {
		t.Fatalf("ParseFileRef: %v", err)
	}
	if parsed != orig {
		t.Errorf("round-trip mismatch: %+v != %+v", parsed, orig)
	}
}

func TestFileRefFromMap(t *testing.T) {
	ref, ok := FileRefFromMap(map[string]any{"bucket": "b", "key": "k"})
	if !ok || ref.Bucket != "b" || ref.Key != "k" {
		t.Errorf("FileRefFromMap = %+v, ok=%v", ref, ok)
	}
	if _, ok := FileRefFromMap(map[string]any{"bucket": "b"}); ok {
		t.Error("missing key should yield ok=false")
	}
	if _, ok := FileRefFromMap(nil); ok {
		t.Error("nil map should yield ok=false")
	}
}

func TestFileRef_JSONRoundTrip(t *testing.T) {
	orig := FileRef{Bucket: "avatars", Key: "users/alice.png"}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(data) != `{"bucket":"avatars","key":"users/alice.png"}` {
		t.Errorf("JSON = %s", data)
	}
	var back FileRef
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if back != orig {
		t.Errorf("round-trip mismatch: %+v != %+v", back, orig)
	}
}

func TestFileRef_UnmarshalJSON_StringForm(t *testing.T) {
	var f FileRef
	if err := json.Unmarshal([]byte(`"avatars:/users/alice.png"`), &f); err != nil {
		t.Fatalf("Unmarshal string form: %v", err)
	}
	if f.Bucket != "avatars" || f.Key != "users/alice.png" {
		t.Errorf("parsed = %+v", f)
	}
}

// TestFileRef_CBORRoundTrip proves the {bucket,key} wire shape survives a
// round-trip through the SDK's surrealcbor codec — the same codec the live
// client uses on the wire.
func TestFileRef_CBORRoundTrip(t *testing.T) {
	codec := surrealcbor.New()
	orig := FileRef{Bucket: "avatars", Key: "users/alice.png"}
	data, err := codec.Marshal(orig)
	if err != nil {
		t.Fatalf("cbor Marshal: %v", err)
	}

	// Decode into a generic map to confirm the encoded shape carries both
	// string fields under bucket / key.
	var generic map[string]any
	if err := codec.Unmarshal(data, &generic); err != nil {
		t.Fatalf("cbor Unmarshal to map: %v", err)
	}
	if generic["bucket"] != "avatars" || generic["key"] != "users/alice.png" {
		t.Errorf("CBOR map shape = %v", generic)
	}

	var back FileRef
	if err := codec.Unmarshal(data, &back); err != nil {
		t.Fatalf("cbor Unmarshal to FileRef: %v", err)
	}
	if back != orig {
		t.Errorf("CBOR round-trip mismatch: %+v != %+v", back, orig)
	}
}
