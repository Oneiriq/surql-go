# SurrealDB v3 Patterns

surql-go targets SurrealDB **v3.0+**. CI runs against `surrealdb/surrealdb:v3.0.5`.
This page covers the v3-specific surface you can rely on today and the
single upstream SDK rough edge we currently work around.

## Interactive transactions

v3 added a native `begin` / `commit` / `rollback` RPC. `DatabaseClient.Begin`
returns a live `Transaction` (WebSocket transports only — the RPC is not
exposed over HTTP):

```go
import (
    "context"
    "github.com/Oneiriq/surql-go/pkg/surql/connection"
)

func transfer(ctx context.Context, client *connection.DatabaseClient, from, to string, amount int) error {
    tx, err := client.Begin(ctx)
    if err != nil {
        return err
    }
    // Always finalise exactly once — Commit or Rollback.
    if _, err := tx.Execute(ctx, "UPDATE "+from+" SET balance -= "+amountStr(amount)); err != nil {
        _ = tx.Rollback(ctx)
        return err
    }
    if _, err := tx.Execute(ctx, "UPDATE "+to+" SET balance += "+amountStr(amount)); err != nil {
        _ = tx.Rollback(ctx)
        return err
    }
    return tx.Commit(ctx)
}
```

`Transaction.State()` + `Transaction.IsActive()` let you inspect whether the
handle has already been finalised; calling Commit/Rollback twice returns
`ErrTransaction` from `pkg/surql/errors`.

## `GROUP ALL` aggregations

v3 accepts `GROUP ALL` for whole-result aggregation (no explicit grouping
columns needed). The builder exposes it via `Query.GroupAll()` and the
high-level helper `AggregateRecords`:

```go
q := query.NewQuery().
    SelectExpr(query.CountAll(), query.MathMean("score")).
    From("match").
    GroupAll()

rows, _ := q.Execute(ctx, client)
```

See [Query UX](query-ux.md) for the `AggregateOpts` / `AggregateRecords`
helper that wraps this pattern end-to-end.

## Datetime coercion

v3 rejects implicit string-to-datetime coercion in some contexts. Whenever
you hand-build payloads that contain ISO-8601 timestamps, use
`types.CoerceRecordDatetimes` to wrap the relevant fields in `<datetime>`
casts before sending:

```go
record := map[string]any{
    "created_at": "2026-04-18T19:33:00Z",
    "updated_at": "2026-04-18T19:34:00Z",
}
record, _ = types.CoerceRecordDatetimes(record, []string{"created_at", "updated_at"})
```

The code-first CRUD helpers (`CreateRecord`, `UpdateRecord`, …) already do
this for any `time.Time` value in the payload.

## Raw record-id targets

v3 lets you pass raw record ids — `table:id` — as SELECT / UPDATE / DELETE
targets. surql-go surfaces this via `types.TypeRecord` / `types.TypeThing`
plus the `*ByTarget` CRUD helpers:

```go
id := types.TypeRecord("user", "alice")

record, _ := query.GetByTarget(ctx, client, id)
_, err    := query.UpdateByTarget(ctx, client, id, map[string]any{"status": "active"})
err       := query.DeleteByTarget(ctx, client, id)
exists, _ := query.ExistsByTarget(ctx, client, id)
```

`TypeRecord(table, id)` emits `type::record('<table>:<id>')`; `TypeThing`
emits `type::thing('<table>', <id>)` for cases where the id value is
already a typed value (int, UUID, etc). Both implement `types.Operator`
and render verbatim wherever surql-go accepts an expression.

## Full-text search (BM25)

Full-text (BM25) search is the **sparse / lexical leg of hybrid retrieval** —
pair it with `Query.VectorSearch` (the dense leg) and fuse the two result
orders by rank (Reciprocal Rank Fusion).

### `SEARCH` was renamed `FULLTEXT`

SurrealDB 3.0 renamed the full-text index keyword. The v1/v2 form is a parse
error on v3 (`Unexpected token, expected Eof` at `SEARCH`):

```text
DEFINE INDEX idx ON TABLE t COLUMNS content SEARCH ANALYZER ascii BM25;  -- parse error on v3
```

v3 spells it `FULLTEXT`:

```text
DEFINE INDEX idx ON TABLE t COLUMNS content FULLTEXT ANALYZER ascii BM25 HIGHLIGHTS;
```

surql-go emits the `FULLTEXT` keyword from `IndexTypeSearch` / `SearchIndex` /
`BM25Index` (and the migration diff renders it the same way); the `INFO FOR
TABLE` index parser recognises **both** spellings so historical schemas still
round-trip. `COLUMNS` and `FIELDS` are interchangeable in this statement.

Bare `BM25` uses the engine defaults (`k1 = 1.2`, `b = 0.75`); the analyzer
defaults to `ascii` when the clause is omitted — define and name one explicitly
for real lexical recall.

### Defining the analyzer + index

The analyzer is defined separately with `DEFINE ANALYZER` and **must be applied
before** the index that references it:

```go
import "github.com/Oneiriq/surql-go/pkg/surql/schema"

analyzer := schema.StandardAnalyzer("text_en") // class + lowercase + ascii
idx := schema.BM25Index("content_bm25", []string{"content"}, "text_en")

// Analyzer DDL is emitted first, then the table (with its FULLTEXT index):
script, _ := schema.GenerateSchemaSQLFromSlicesWithAnalyzers(
    []schema.AnalyzerDefinition{analyzer},
    []schema.TableDefinition{
        schema.NewTable("memory", schema.WithIndexes(idx)),
    },
    nil,   // edges
    false, // ifNotExists
)
// DEFINE ANALYZER text_en TOKENIZERS class FILTERS lowercase,ascii;
// DEFINE TABLE memory SCHEMAFULL;
// DEFINE INDEX content_bm25 ON TABLE memory COLUMNS content FULLTEXT ANALYZER text_en BM25;
```

### Querying

`Query.FullTextSearch(field, reference, query)` renders the `@reference@`
matches operator in the `WHERE` clause; `Query.SearchScore(reference, alias)`
projects `search::score(reference)`. The `FullTextSearchQuery` helper wires both
together:

```go
import "github.com/Oneiriq/surql-go/pkg/surql/query"

q, _ := query.FullTextSearchQuery(
    "memory", "content", 1, "insider buying", nil, "score",
)
// SELECT *, search::score(1) AS score FROM memory WHERE content @1@ 'insider buying'
```

### `search::score` and scan ordering

The v3 streaming executor's full-text scan yields matching rows **already in
BM25 relevance order**, but does not (in 3.0.x) plumb the per-row score through
to `search::score(<ref>)`, which returns `0` there. So **rank by the scan's
natural order rather than `ORDER BY search::score(...)`**. This is sufficient
for Reciprocal Rank Fusion, which fuses *ranks*, not raw scores:

```go
// The sparse leg: rows come back in relevance order; cap the candidate set.
q, _ := query.FullTextSearchQuery("memory", "content", 1, "insider buying", nil, "score")
q, _ = q.Limit(100)
// Fuse the returned order with the dense (VectorSearch) order via RRF.
```

v3 also ships a native `search::rrf([$dense, $sparse], k, 60)` function that
fuses two result lists server-side, if you prefer in-engine fusion.

## Missing-table error shape

v2 treated queries against an undefined table as returning zero rows. v3
raises an error of the form `The table 'foo' does not exist`. surql-go
absorbs this in the CRUD path so that the zero-rows semantics is
preserved: any error containing `does not exist` (or the defensive
fallback `table not found`) collapses to an empty result in
`QueryRecords`, `First`, `Last`, `Exists`, `CountRecords`, and the
graph/batch helpers.

If you need the raw error for schema-assertion code, call
`DatabaseClient.Query` directly instead of going through `query.*`.

## Known upstream SDK limitation

**Live-query teardown race in `surrealdb.go` v1.4.0.** When a subscription
is closed while the SDK is still draining its internal notification
channel, the shutdown path in `CloseLiveNotifications` can race with the
receiver goroutine and panic or deadlock. Tracked upstream as
[surrealdb.go#398](https://github.com/surrealdb/surrealdb.go/issues/398).

While the fix lands upstream we:

- Skip the two live-query integration tests that exercise the teardown
  sequence (`connection.TestIntegration_Live*`). The build still covers
  `Live` subscription startup and event delivery, only the explicit
  `Close` / `Kill` path is skipped.
- Document this limitation here so downstream consumers know not to rely
  on deterministic `LiveQuery.Close` teardown in production under
  surrealdb.go v1.4.0.

`StreamingManager.DrainAll` will suffer from the same race once the
bug triggers; prefer relying on client-level disconnection (which tears
down the WebSocket wholesale) until the SDK releases the fix.

## What's next

- **[Query UX](query-ux.md)** — the helpers unlocked by the v3 surface.
- **[CLI](cli.md)** — `surql` subcommands that lean on v3 behaviour.
