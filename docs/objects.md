# Object Storage and Sessions

Two SurrealDB v3 surfaces that the schema and connection layers both reach:
storage buckets for file data, and multiple sessions over one connection.

## Buckets

A bucket is declared in code like any other definition and managed by the
migration engine (`ADD_BUCKET` / `DROP_BUCKET` / `MODIFY_BUCKET`).

```go
b := schema.NewBucket("avatars", "memory")

// Shorthands for the common backends.
b = schema.MemoryBucket("scratch")
b = schema.FileBucket("uploads", "/var/lib/surreal/uploads")
b = schema.S3Bucket("archive", "s3://bucket-name")
```

Options: `WithBucketReadOnly`, `WithBucketPermissions`, `WithBucketComment`.

Buckets require a server with the experimental files capability enabled:

```
SURREAL_CAPS_ALLOW_EXPERIMENTAL=files
```

The capability is not covered by `--allow-all`. Prefer the environment variable
over the `--allow-experimental files` flag, which swallows the trailing
`memory` datastore positional.

### File-typed fields

```go
schema.FileField("avatar")
schema.BytesField("thumbnail")
```

A file-typed column carries a `types.FileRef`, a `{Bucket, Key}` pair that
renders as the SurrealQL pointer `<bucket>:/<key>`. SurrealDB stores keys in a
canonical form with a leading slash, so `a.txt` and `/a.txt` name the same
file.

### Runtime

```go
bucket := client.Bucket("avatars")

err := bucket.Put(ctx, "u/1.png", data)
err = bucket.PutIfNotExists(ctx, "u/1.png", data)

raw, err := bucket.Get(ctx, "u/1.png")
text, err := bucket.GetText(ctx, "notes.md")
ok, err := bucket.Exists(ctx, "u/1.png")
meta, err := bucket.Head(ctx, "u/1.png")
refs, err := bucket.List(ctx)

err = bucket.Copy(ctx, "u/1.png", "u/2.png")
err = bucket.Rename(ctx, "u/1.png", "u/3.png")
err = bucket.Delete(ctx, "u/3.png")
```

`CopyIfNotExists` and `RenameIfNotExists` are the guarded forms. Every
operation binds `type::file($bucket, $key)` parameters rather than
interpolating the key into a statement.

The CLI covers the same surface: `surql bucket define | list | rm | put | get |
delete | exists | files`.

## Sessions

`Attach` opens a second session over the existing connection, which suits work
that needs its own authentication without a second client.

```go
session, err := client.Attach(ctx)
if err != nil {
    return err
}
defer session.Detach(ctx)

if err := session.Authenticate(ctx, token); err != nil {
    return err
}
```

A session mirrors the client surface (`Create`, `Select`, `Merge`, `Delete`,
`Query`, `QueryWithVars`, `Signin`, and `Bucket` through `SessionBucket`). Two
properties matter in practice: a session needs a live WebSocket connection, and
it starts unauthenticated. Sign in on the session when it needs more than guest
access. `CurrentAuthType` and `CurrentToken` report where a session stands.

`Detach` releases the session and `IsDetached` reports whether it already has.
`Invalidate` drops the session's authentication while keeping the session
itself.
