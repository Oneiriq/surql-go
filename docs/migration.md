# Upgrading

## v0.1.0 → v0.2.0

v0.2.0 is an **additive** release — no APIs were removed or renamed, so
a straight `go get -u github.com/Oneiriq/surql-go` should be enough for
existing code to keep compiling.

That said, v0.2.0 adds a set of first-class helpers that can simplify
your call sites significantly. This page highlights the patterns worth
migrating to.

### New query UX helpers

See [Query UX](query-ux.md) for the full list. Quick before/after:

Hand-built record-id target →

```go
res, _ := client.Query(ctx, "SELECT * FROM user:alice")
record, _ := surql.ExtractOne(res)
```

becomes

```go
record, _ := query.GetByTarget(ctx, client, types.TypeRecord("user", "alice"))
```

Manual JSON envelope flattening →

```go
raw, _ := client.Query(ctx, "SELECT count() AS n FROM user GROUP ALL")
envelope, _ := raw.([]any)
first, _    := envelope[0].(map[string]any)
result, _   := first["result"].([]any)
row, _      := result[0].(map[string]any)
n, _        := row["n"].(float64)
```

becomes

```go
raw, _  := client.Query(ctx, "SELECT count() AS n FROM user GROUP ALL")
n, _    := surql.ExtractScalar[int64](raw, "n")
```

Aggregation projections →

```go
q, _ := query.Select([]string{"count() AS total", "math::mean(score) AS avg"}).FromTable("match")
```

becomes

```go
q := query.NewQuery().SelectAliased(map[string]types.Operator{
    "total": query.CountAll(),
    "avg":   query.MathMean("score"),
}).From("match").GroupAll()
```

### Interactive transactions (v3)

`DatabaseClient.Begin` is new in v0.1.0 — code written against an earlier
pre-release scratch branch that rolled its own WS transactions can now
be switched to the built-in handle.

See [v3 Patterns](v3-patterns.md#interactive-transactions) for the
reference flow.

### Settings loader

`pkg/surql/settings` gained a unified `LoadSettings(...Option)` entry
point that accepts explicit `WithConfigFile(...)` overrides and falls
back to `SURQL_*` env-var discovery. Existing callers that hand-wired
env vars via `connection.LoadConfigFromEnv` still work, but settings-
driven CLI reuse (and the new orchestration plan files) is a lot less
boilerplate.

### Pre-push hook

`/.githooks/pre-push` mirrors CI (`gofmt -l`, `go vet`, `go test -race`).
Enable it in your local checkout:

```shell
git config core.hooksPath .githooks
```

See [CONTRIBUTING.md](https://github.com/Oneiriq/surql-go/blob/main/CONTRIBUTING.md)
for the full dev workflow.

## v0.2.0 → v0.2.1

**Documentation-only release.** No functional changes — this build
rebuilds the narrative mkdocs site around the v0.2 surface:

- New **[v3 Patterns](v3-patterns.md)** reference covering interactive
  transactions, datetime coercion, `GROUP ALL`, raw record-id targets,
  the v3 missing-table error shape, and the surrealdb.go#398 live-query
  teardown race.
- New **[Query UX](query-ux.md)** reference with before/after Go
  examples for every helper added in v0.2.0.
- New **[CLI Reference](cli.md)** enumerating every `surql` subcommand
  (previously a "planned" placeholder).
- New **[Features](features.md)** page with the package layout.
- README top-level examples updated to use the new first-class
  helpers.

No upgrade steps required beyond `go get -u`.
