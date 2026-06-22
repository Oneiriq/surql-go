package types

import (
	"encoding/json"
	"strings"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

// FileRef references a file stored in a SurrealDB v3 storage bucket. It is the
// value type for file-typed fields (schema.FieldTypeFile) and the value
// returned by file-producing queries.
//
// Keys are stored verbatim. SurrealDB exposes file keys in their canonical
// form, which carries a leading slash: file::key() returns "/a.txt" no matter
// whether the file was written as "a.txt" or "/a.txt", and the server treats
// type::file($b,"a.txt") and type::file($b,"/a.txt") as the same file. FileRef
// preserves whatever key the server (or caller) supplies — it never strips or
// rewrites a leading slash — so List/Head surface keys exactly as the server
// reports them.
//
// The canonical SurrealQL literal for a file pointer is `<bucket>:/<key>`
// (for example `avatars:/users/alice.png`). String renders that form for any
// stored key by emitting exactly one slash after the colon, regardless of
// whether Key itself begins with a slash. Over the wire SurrealDB models a
// file as a {bucket, key} object, so FileRef marshals to and from a
// {"bucket","key"} map in both JSON and CBOR.
//
// Runtime reads and writes never interpolate a FileRef into SurrealQL text:
// the connection.Bucket handle binds the bucket and key as parameters to the
// type::file($bucket,$key) constructor. ToSurql is provided for the rare cases
// where a literal is genuinely required (e.g. building a DEFINE statement) and
// single-quote-escapes both components.
type FileRef struct {
	Bucket string `json:"bucket" cbor:"bucket"`
	Key    string `json:"key" cbor:"key"`
}

// NewFileRef constructs a FileRef, storing the key verbatim. SurrealDB
// normalises keys server-side (a leading slash is canonical and
// "a.txt"/"/a.txt" resolve to the same file), so the key is not rewritten here.
func NewFileRef(bucket, key string) FileRef {
	return FileRef{Bucket: bucket, Key: key}
}

// String renders the canonical `<bucket>:/<key>` file literal, emitting a
// single slash after the colon for any stored key (a leading slash on Key is
// not doubled).
func (f FileRef) String() string {
	return f.Bucket + ":/" + strings.TrimPrefix(f.Key, "/")
}

// ToSurql renders the file as a parameter-free type::file('<bucket>','<key>')
// constructor with both components single-quote-escaped. Prefer the bound-
// parameter form via connection.Bucket for runtime operations; this helper is
// for emitting literals into generated SurrealQL.
func (f FileRef) ToSurql() string {
	return "type::file('" + escapeSingleQuotes(f.Bucket) + "', '" + escapeSingleQuotes(f.Key) + "')"
}

func escapeSingleQuotes(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	return strings.ReplaceAll(s, `'`, `\'`)
}

// Validate reports an ErrValidation error when the bucket or key is empty.
func (f FileRef) Validate() error {
	if f.Bucket == "" {
		return surqlerrors.New(surqlerrors.ErrValidation, "file ref bucket cannot be empty")
	}
	if f.Key == "" {
		return surqlerrors.New(surqlerrors.ErrValidation, "file ref key cannot be empty")
	}
	return nil
}

// ParseFileRef parses the canonical `<bucket>:/<key>` literal into a FileRef.
// A missing `:/` separator, an empty bucket, or an empty key returns an
// ErrValidation error. Everything after the `:/` separator becomes the key
// verbatim, so a literal with a doubled slash (`bucket://k`) yields the key
// "/k". The separator slash itself is not part of the key.
func ParseFileRef(input string) (FileRef, error) {
	idx := strings.Index(input, ":/")
	if idx < 0 {
		return FileRef{}, surqlerrors.Newf(surqlerrors.ErrValidation,
			"invalid file reference %q: expected format bucket:/key", input)
	}
	bucket := input[:idx]
	key := input[idx+2:]
	if bucket == "" {
		return FileRef{}, surqlerrors.Newf(surqlerrors.ErrValidation,
			"invalid file reference %q: bucket cannot be empty", input)
	}
	if key == "" {
		return FileRef{}, surqlerrors.Newf(surqlerrors.ErrValidation,
			"invalid file reference %q: key cannot be empty", input)
	}
	return FileRef{Bucket: bucket, Key: key}, nil
}

// FileRefFromMap reconstructs a FileRef from a decoded {bucket, key} map, the
// shape SurrealDB returns for a file value once CBOR/JSON-decoded into a
// map[string]any. It returns (zero, false) when the map does not carry both
// string "bucket" and "key" entries. This mirrors how query result decoding
// surfaces RecordID-shaped values.
func FileRefFromMap(m map[string]any) (FileRef, bool) {
	if m == nil {
		return FileRef{}, false
	}
	bucket, bok := m["bucket"].(string)
	key, kok := m["key"].(string)
	if !bok || !kok {
		return FileRef{}, false
	}
	return FileRef{Bucket: bucket, Key: key}, true
}

// MarshalJSON encodes the file as a {"bucket","key"} object.
func (f FileRef) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Bucket string `json:"bucket"`
		Key    string `json:"key"`
	}{Bucket: f.Bucket, Key: f.Key})
}

// UnmarshalJSON accepts either the {"bucket","key"} object form or the
// canonical `<bucket>:/<key>` string literal.
func (f *FileRef) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if strings.HasPrefix(trimmed, `"`) {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		parsed, err := ParseFileRef(s)
		if err != nil {
			return err
		}
		*f = parsed
		return nil
	}
	var obj struct {
		Bucket string `json:"bucket"`
		Key    string `json:"key"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	*f = FileRef{Bucket: obj.Bucket, Key: obj.Key}
	return nil
}
