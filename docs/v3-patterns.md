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
