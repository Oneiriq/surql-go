package connection

import (
	"context"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
	"github.com/Oneiriq/surql-go/pkg/surql/types"
)

// queryRunner is the minimal surface a Bucket needs to issue parameterised
// SurrealQL. Both *DatabaseClient and *Session satisfy it, so a single Bucket
// implementation serves connection-level and session-scoped file operations.
type queryRunner interface {
	QueryWithVars(ctx context.Context, surql string, vars map[string]any) (any, error)
}

// Bucket is a runtime handle for object-storage operations against a single
// SurrealDB v3 storage bucket. Obtain one via DatabaseClient.Bucket or
// Session.Bucket.
//
// Every operation is expressed through the parameterised type::file($bucket,
// $key) constructor with the bucket name, key, destination key, and payload
// bound as query variables. No caller-supplied value is ever interpolated into
// the SurrealQL text, so bucket names, keys, and binary payloads cannot break
// out of their value position.
type Bucket struct {
	runner queryRunner
	name   string
}

// SessionBucket is an alias documenting that Session.Bucket returns the same
// Bucket handle type used at the connection level; the operations run within
// the session's authentication context.
type SessionBucket = Bucket

// Bucket returns a handle bound to the named storage bucket. The handle shares
// the client's connection; operations require a running WebSocket or HTTP
// connection with the experimental files capability enabled on the server.
func (c *DatabaseClient) Bucket(name string) *Bucket {
	return &Bucket{runner: c, name: name}
}

// Name returns the bucket name this handle is bound to.
func (b *Bucket) Name() string { return b.name }

// fileVars returns the base variable map binding the bucket name and key.
func (b *Bucket) fileVars(key string) map[string]any {
	return map[string]any{
		"bucket": b.name,
		"key":    key,
	}
}

// coerceData normalises the accepted data argument (string or []byte) into a
// value the CBOR encoder maps to the SurrealDB type the file API expects: a
// []byte is sent verbatim as a CBOR byte string (bytes), a string as a CBOR
// text string. Any other type is rejected with ErrValidation.
func coerceData(data any) (any, error) {
	switch data.(type) {
	case string, []byte:
		return data, nil
	default:
		return nil, surqlerrors.Newf(surqlerrors.ErrValidation,
			"bucket data must be string or []byte, got %T", data)
	}
}

// firstResult unwraps the per-statement envelope produced by
// DatabaseClient.QueryWithVars and returns the `result` value of the first
// statement (or nil when the response is empty).
func firstResult(raw any) any {
	arr, ok := raw.([]any)
	if !ok || len(arr) == 0 {
		return nil
	}
	env, ok := arr[0].(map[string]any)
	if !ok {
		return nil
	}
	return env["result"]
}

// Put writes data (string or []byte) to key, overwriting any existing file.
// SurrealQL: RETURN type::file($bucket, $key).put($data);
func (b *Bucket) Put(ctx context.Context, key string, data any) error {
	payload, err := coerceData(data)
	if err != nil {
		return err
	}
	vars := b.fileVars(key)
	vars["data"] = payload
	if _, err := b.runner.QueryWithVars(ctx,
		"RETURN type::file($bucket, $key).put($data);", vars); err != nil {
		return surqlerrors.Wrapf(surqlerrors.ErrQuery, err,
			"put %s:/%s failed", b.name, key)
	}
	return nil
}

// PutIfNotExists writes data only when key does not already exist.
// SurrealQL: RETURN type::file($bucket, $key).put_if_not_exists($data);
func (b *Bucket) PutIfNotExists(ctx context.Context, key string, data any) error {
	payload, err := coerceData(data)
	if err != nil {
		return err
	}
	vars := b.fileVars(key)
	vars["data"] = payload
	if _, err := b.runner.QueryWithVars(ctx,
		"RETURN type::file($bucket, $key).put_if_not_exists($data);", vars); err != nil {
		return surqlerrors.Wrapf(surqlerrors.ErrQuery, err,
			"put_if_not_exists %s:/%s failed", b.name, key)
	}
	return nil
}

// Get reads the raw bytes stored at key.
// SurrealQL: RETURN type::file($bucket, $key).get();
func (b *Bucket) Get(ctx context.Context, key string) ([]byte, error) {
	raw, err := b.runner.QueryWithVars(ctx,
		"RETURN type::file($bucket, $key).get();", b.fileVars(key))
	if err != nil {
		return nil, surqlerrors.Wrapf(surqlerrors.ErrQuery, err,
			"get %s:/%s failed", b.name, key)
	}
	result := firstResult(raw)
	if result == nil {
		return nil, nil
	}
	switch v := result.(type) {
	case []byte:
		return v, nil
	case string:
		return []byte(v), nil
	default:
		return nil, surqlerrors.Newf(surqlerrors.ErrQuery,
			"get %s:/%s returned unexpected type %T", b.name, key, result)
	}
}

// GetText reads the file at key and returns it as a string. The server-side
// <string> cast turns the byte payload into text.
// SurrealQL: RETURN <string>type::file($bucket, $key).get();
func (b *Bucket) GetText(ctx context.Context, key string) (string, error) {
	raw, err := b.runner.QueryWithVars(ctx,
		"RETURN <string>type::file($bucket, $key).get();", b.fileVars(key))
	if err != nil {
		return "", surqlerrors.Wrapf(surqlerrors.ErrQuery, err,
			"get_text %s:/%s failed", b.name, key)
	}
	result := firstResult(raw)
	if result == nil {
		return "", nil
	}
	switch v := result.(type) {
	case string:
		return v, nil
	case []byte:
		return string(v), nil
	default:
		return "", surqlerrors.Newf(surqlerrors.ErrQuery,
			"get_text %s:/%s returned unexpected type %T", b.name, key, result)
	}
}

// Exists reports whether a file is stored at key.
// SurrealQL: RETURN type::file($bucket, $key).exists();
func (b *Bucket) Exists(ctx context.Context, key string) (bool, error) {
	raw, err := b.runner.QueryWithVars(ctx,
		"RETURN type::file($bucket, $key).exists();", b.fileVars(key))
	if err != nil {
		return false, surqlerrors.Wrapf(surqlerrors.ErrQuery, err,
			"exists %s:/%s failed", b.name, key)
	}
	if v, ok := firstResult(raw).(bool); ok {
		return v, nil
	}
	return false, nil
}

// Head returns the file's metadata at key as a decoded map, or nil when the
// file does not exist. The returned map carries the canonical "bucket" and
// "key" (file::key returns the key in canonical form, with a leading slash)
// plus "size" and "updated".
//
// The raw head() result embeds the file under a "file" pointer that the SDK's
// CBOR codec cannot decode into a structured value (surrealdb.go ships no File
// tag, so the pointer decodes to a bare [bucket, key] slice). The query
// therefore projects file::bucket / file::key off head() so the result is a
// portable map of plain scalars. SELECT over head() yields a single-row set
// for an existing file and an empty set for a missing one.
// SurrealQL: SELECT file::bucket(file) AS bucket, file::key(file) AS key,
// size, updated FROM type::file($bucket, $key).head();
func (b *Bucket) Head(ctx context.Context, key string) (map[string]any, error) {
	raw, err := b.runner.QueryWithVars(ctx,
		"SELECT file::bucket(file) AS bucket, file::key(file) AS key, size, updated "+
			"FROM type::file($bucket, $key).head();", b.fileVars(key))
	if err != nil {
		return nil, surqlerrors.Wrapf(surqlerrors.ErrQuery, err,
			"head %s:/%s failed", b.name, key)
	}
	result := firstResult(raw)
	if result == nil {
		return nil, nil
	}
	rows, ok := result.([]any)
	if !ok {
		return nil, surqlerrors.Newf(surqlerrors.ErrQuery,
			"head %s:/%s returned unexpected type %T", b.name, key, result)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	if m, ok := rows[0].(map[string]any); ok {
		return m, nil
	}
	return nil, surqlerrors.Newf(surqlerrors.ErrQuery,
		"head %s:/%s returned unexpected row type %T", b.name, key, rows[0])
}

// Delete removes the file stored at key.
// SurrealQL: RETURN type::file($bucket, $key).delete();
func (b *Bucket) Delete(ctx context.Context, key string) error {
	if _, err := b.runner.QueryWithVars(ctx,
		"RETURN type::file($bucket, $key).delete();", b.fileVars(key)); err != nil {
		return surqlerrors.Wrapf(surqlerrors.ErrQuery, err,
			"delete %s:/%s failed", b.name, key)
	}
	return nil
}

// Copy copies the file at key to dst within the same bucket, overwriting any
// existing file at dst. The destination argument is a key, not a full path.
// SurrealQL: RETURN type::file($bucket, $key).copy($dst);
func (b *Bucket) Copy(ctx context.Context, key, dst string) error {
	vars := b.fileVars(key)
	vars["dst"] = dst
	if _, err := b.runner.QueryWithVars(ctx,
		"RETURN type::file($bucket, $key).copy($dst);", vars); err != nil {
		return surqlerrors.Wrapf(surqlerrors.ErrQuery, err,
			"copy %s:/%s -> %s failed", b.name, key, dst)
	}
	return nil
}

// CopyIfNotExists copies the file at key to dst only when dst does not already
// exist. The destination argument is a key, not a full path.
// SurrealQL: RETURN type::file($bucket, $key).copy_if_not_exists($dst);
func (b *Bucket) CopyIfNotExists(ctx context.Context, key, dst string) error {
	vars := b.fileVars(key)
	vars["dst"] = dst
	if _, err := b.runner.QueryWithVars(ctx,
		"RETURN type::file($bucket, $key).copy_if_not_exists($dst);", vars); err != nil {
		return surqlerrors.Wrapf(surqlerrors.ErrQuery, err,
			"copy_if_not_exists %s:/%s -> %s failed", b.name, key, dst)
	}
	return nil
}

// Rename moves the file at key to dst within the same bucket, overwriting any
// existing file at dst. The destination argument is a key, not a full path.
// SurrealQL: RETURN type::file($bucket, $key).rename($dst);
func (b *Bucket) Rename(ctx context.Context, key, dst string) error {
	vars := b.fileVars(key)
	vars["dst"] = dst
	if _, err := b.runner.QueryWithVars(ctx,
		"RETURN type::file($bucket, $key).rename($dst);", vars); err != nil {
		return surqlerrors.Wrapf(surqlerrors.ErrQuery, err,
			"rename %s:/%s -> %s failed", b.name, key, dst)
	}
	return nil
}

// RenameIfNotExists moves the file at key to dst only when dst does not already
// exist. The destination argument is a key, not a full path.
// SurrealQL: RETURN type::file($bucket, $key).rename_if_not_exists($dst);
func (b *Bucket) RenameIfNotExists(ctx context.Context, key, dst string) error {
	vars := b.fileVars(key)
	vars["dst"] = dst
	if _, err := b.runner.QueryWithVars(ctx,
		"RETURN type::file($bucket, $key).rename_if_not_exists($dst);", vars); err != nil {
		return surqlerrors.Wrapf(surqlerrors.ErrQuery, err,
			"rename_if_not_exists %s:/%s -> %s failed", b.name, key, dst)
	}
	return nil
}

// List returns the files stored in the bucket as FileRef values, with keys in
// their canonical form (file::key returns a leading slash, e.g. "/a.txt").
//
// Each raw file::list row embeds the file under a "file" pointer the SDK's
// CBOR codec cannot decode into a structured value (surrealdb.go ships no File
// tag). The query therefore projects file::bucket / file::key so every row
// carries plain "bucket" and "key" strings.
// SurrealQL: SELECT file::bucket(file) AS bucket, file::key(file) AS key,
// size, updated FROM file::list($bucket);
func (b *Bucket) List(ctx context.Context) ([]types.FileRef, error) {
	raw, err := b.runner.QueryWithVars(ctx,
		"SELECT file::bucket(file) AS bucket, file::key(file) AS key, size, updated "+
			"FROM file::list($bucket);", map[string]any{"bucket": b.name})
	if err != nil {
		return nil, surqlerrors.Wrapf(surqlerrors.ErrQuery, err,
			"list %s failed", b.name)
	}
	result := firstResult(raw)
	if result == nil {
		return nil, nil
	}
	entries, ok := result.([]any)
	if !ok {
		return nil, surqlerrors.Newf(surqlerrors.ErrQuery,
			"list %s returned unexpected type %T", b.name, result)
	}
	refs := make([]types.FileRef, 0, len(entries))
	for _, entry := range entries {
		ref, ok := fileRefFromListEntry(b.name, entry)
		if !ok {
			continue
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

// fileRefFromListEntry extracts a FileRef from one projected file::list row.
// The projection surfaces the file's "bucket" and "key" as top-level string
// fields (key in canonical form), so the common path reads those directly. The
// queried bucket name backfills a missing/blank bucket field. For resilience
// against an unprojected row, a raw "file" pointer is also handled: a
// {bucket,key} map, a canonical "bucket:/key" string, or a bare key (which is
// stored verbatim). Keys are never rewritten — a leading slash is preserved.
func fileRefFromListEntry(bucket string, entry any) (types.FileRef, bool) {
	m, ok := entry.(map[string]any)
	if !ok {
		return types.FileRef{}, false
	}
	// Projected row: top-level bucket/key strings.
	if key, kok := m["key"].(string); kok && key != "" {
		b := bucket
		if pb, ok := m["bucket"].(string); ok && pb != "" {
			b = pb
		}
		return types.FileRef{Bucket: b, Key: key}, true
	}
	// Fallback: a raw, unprojected "file" pointer.
	file, ok := m["file"]
	if !ok {
		return types.FileRef{}, false
	}
	switch v := file.(type) {
	case map[string]any:
		if ref, ok := types.FileRefFromMap(v); ok {
			return ref, true
		}
	case string:
		if ref, err := types.ParseFileRef(v); err == nil {
			return ref, true
		}
		// Bare key without a bucket prefix.
		return types.FileRef{Bucket: bucket, Key: v}, true
	}
	return types.FileRef{}, false
}
