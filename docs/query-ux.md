# Query UX Helpers

The v0.2 "query UX" release adds a handful of first-class helpers that
collapse previously verbose patterns into one-liners. This page walks
each one with a before/after Go example.

## `types.TypeRecord` / `types.TypeThing`

Construct a v3 raw-record-id target that any CRUD or query helper will
accept. Both implement `types.Operator`, so they compose inside WHERE
clauses, SET bodies, and SELECT projections.

Before:

```go
// Hand-rolled record target: no type safety, quoting bugs waiting to happen.
res, err := client.Query(ctx, "SELECT * FROM type::record('user:alice')")
```

After:

```go
target := types.TypeRecord("user", "alice")
record, err := query.GetByTarget(ctx, client, target)
```

`TypeThing(table, id)` is the right pick when `id` is already a typed
value (int, UUID, nested object):

```go
user := types.TypeThing("user", 42)
```

## `types.RawSurqlValue`

Marker interface that tells surql-go to emit the wrapped value verbatim
rather than JSON-quoting it. `types.SurrealFn` and `query.Expression`
both implement it so math / string / count builders drop into SET /
WHERE / SELECT without extra casts:

```go
payload := map[string]any{
    "updated_at": types.SurqlFn("time::now"), // emits bare time::now()
    "score":      query.MathMean("scores"),   // emits math::mean(scores)
}
_, _ = query.UpdateByTarget(ctx, client, target, payload)
```

## Function factories (`MathAbs`, `StringLen`, `CountAll`, …)

Each factory returns a `types.SurrealFn` that composes through the same
`types.Operator` interface as the existing `Expression` helpers.

| Factory                 | Rendering                          |
|-------------------------|------------------------------------|
| `MathAbs(f)`            | `math::abs(f)`                     |
| `MathCeil(f)`           | `math::ceil(f)`                    |
| `MathFloor(f)`          | `math::floor(f)`                   |
| `MathRound(f)`          | `math::round(f)`                   |
| `StringLen(f)`          | `string::len(f)`                   |
| `StringLower(f)`        | `string::lowercase(f)`             |
| `StringUpper(f)`        | `string::uppercase(f)`             |
| `StringConcat(a, b...)` | `string::concat(a, b, ...)`        |
| `CountAll()`            | `count()`                          |
| `CountField(f)`         | `count(f)`                         |
| `CountIf(expr)`         | `count(<expr>)`                    |

Before:

```go
q, _ := query.Select([]string{"name", "math::abs(score) AS abs_score"}).FromTable("match")
```

After:

```go
q := query.NewQuery().
    Select([]string{"name"}).
    From("match")
q = q.SelectAliased(map[string]types.Operator{
    "abs_score": query.MathAbs("score"),
})
```

## `Query.SelectExpr` / `Query.SelectAliased` / `Query.From` / `Query.Execute`

Four new methods on the immutable builder:

- `SelectExpr(exprs ...types.Operator)` — project raw expressions.
- `SelectAliased(map[string]types.Operator)` — project with `AS alias`
  suffixes (alphabetical-key ordering for deterministic rendering).
- `From(table string)` — non-returning variant of `FromTable` intended
  for chains that don't need to fork on validation.
- `Execute(ctx, client)` — runs the query against a
  `*connection.DatabaseClient` without hopping through the package-level
  `ExecuteQuery`.

```go
q := query.NewQuery().
    SelectExpr(query.CountAll(), query.MathMean("score")).
    From("match").
    GroupAll()

rows, _ := q.Execute(ctx, client)
```

## `ExtractOne` / `ExtractMany` / `ExtractScalar[T]` / `HasResult`

The Python port shipped `extract_one` / `extract_scalar` helpers that
flatten the SurrealDB envelope. The Go equivalents live at the package
root (`pkg/surql`) so a single import handles the common case:

```go
import "github.com/Oneiriq/surql-go/pkg/surql"

raw, _ := client.Query(ctx, "SELECT count() AS n FROM user GROUP ALL")

first,  _ := surql.ExtractOne(raw)                 // map[string]any
all,    _ := surql.ExtractMany(raw)                // []map[string]any
count,  _ := surql.ExtractScalar[int64](raw, "n")  // int64, via JSON round-trip
hasAny    := surql.HasResult(raw)                  // bool
```

`ExtractScalar[T]` uses `encoding/json`'s loose numeric coercion so a
SurrealDB count (`float64` in the JSON envelope) decodes cleanly into an
`int`, `int64`, `uint`, etc. Missing keys surface as `ErrValidation`.

## `AggregateRecords` + `AggregateOpts`

High-level helper that wraps SELECT-with-aggregations in a single call.
Accepts any `types.Operator` value as the aggregation expression.

```go
opts := query.AggregateOpts{
    Table: "memory_entry",
    Select: map[string]types.Operator{
        "count":    query.CountAll(),
        "strength": query.MathSum("strength"),
    },
    GroupBy: []string{"network"},
}
rows, _ := query.AggregateRecords(ctx, client, opts)
```

`GroupAll: true` switches the helper to v3 `GROUP ALL`. `Where`,
`OrderBy`, `Limit`, and `Offset` are all optional; set at least one
aggregation expression via `Select`.

## `*ByTarget` CRUD helpers

Sibling of the table-oriented CRUD functions, but accepting a raw
target expression (`types.TypeRecord` / `types.TypeThing`) so callers
don't have to split a known record id into `(table, id)` pairs.

| Helper              | Behaviour                                                    |
|---------------------|--------------------------------------------------------------|
| `GetByTarget`       | `SELECT * FROM <target>` returning the first record or nil.  |
| `UpdateByTarget`    | `UPDATE <target> CONTENT {...}` (full replace).              |
| `UpsertByTarget`    | `UPSERT <target> CONTENT {...}`.                             |
| `DeleteByTarget`    | `DELETE <target>`; missing-table errors collapse to no-op.   |
| `ExistsByTarget`    | `SELECT id FROM <target> LIMIT 1` returning a bool.          |

```go
target := types.TypeRecord("user", "alice")

// Check existence
exists, _ := query.ExistsByTarget(ctx, client, target)

// Partial update (merge semantics via UpdateByTarget + full payload)
_, _ = query.UpdateByTarget(ctx, client, target, map[string]any{
    "status":     "active",
    "updated_at": types.SurqlFn("time::now"),
})

// Delete
_ = query.DeleteByTarget(ctx, client, target)
```

## What's next

- **[v3 Patterns](v3-patterns.md)** — the v3 surface these helpers build on.
- **[Query Builder](queries.md)** — the fluent builder reference.
- **[CLI](cli.md)** — `surql` subcommands that surface these helpers.
