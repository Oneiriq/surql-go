# Query Builder

The immutable fluent builder composes SurrealQL statements without
executing them. Methods on `Query` return new values; the input is never
mutated.

## SELECT

```go
import (
    "github.com/Oneiriq/surql-go/pkg/surql/query"
    "github.com/Oneiriq/surql-go/pkg/surql/types"
)

q, _ := query.Select([]string{"name", "email"}).FromTable("user")
q     = q.Where(types.GtOp("age", 18)).Where(types.EqOp("status", "active"))
q, _  = q.OrderBy("created_at", "DESC")
q     = q.Limit(10)
fmt.Println(q.ToSurql())
```

## INSERT / UPDATE / UPSERT / DELETE / RELATE

```go
ins, _ := query.Insert("user", map[string]any{
    "name":  "Alice",
    "email": "alice@example.com",
})
upd, _ := query.Update("user:alice", map[string]any{"status": "active"})
del, _ := query.Delete("user:bob")
rel, _ := query.Relate("user:alice", "likes", "post:1")
_ = ins; _ = upd; _ = del; _ = rel
```

## Where

`Where` accepts a `string`, any `types.Operator`, or `nil`. Anything else
returns an `ErrValidation` error.

```go
q, _ := query.Select(nil).FromTable("user")
q     = q.Where(types.AndOp(types.GtOp("age", 18), types.EqOp("status", "active")))
```

## Expressions

Expressions build typed SurrealQL fragments you can embed in select lists
or `Where` clauses.

```go
sel := []string{
    query.Field("id").ToSurql(),
    query.As(query.MathMean("score"), "avg_score").ToSurql(),
    query.As(query.Count(""), "total").ToSurql(),
}
```

## Hints

```go
tmo, _ := query.NewTimeoutHint(30.0)
q     = q.Hint(tmo).Hint(query.ParallelEnabled())
```

Hints render as SurrealQL comments; duplicates of the same kind are merged
so only the latest value survives.

## Result wrappers

Queries produce typed `QueryResult[T]` / `RecordResult[T]` /
`ListResult[T]` / `PaginatedResult[T]` values via the raw-response
helpers in `pkg/surql/query` (or the top-level re-exports in
`pkg/surql`): `ExtractResult`, `ExtractOne`, `ExtractScalar`,
`HasResults`. See **[Query UX](query-ux.md)** for worked examples.

## Aggregations (`GROUP ALL` + `SelectExpr`)

The builder's `SelectExpr` / `SelectAliased` / `GroupAll` methods
compose with the `types.SurrealFn` factories (`CountAll`, `MathAbs`,
`StringLen`, …) to render v3 aggregation queries directly:

```go
q := query.NewQuery().
    SelectAliased(map[string]types.Operator{
        "count": query.CountAll(),
        "avg":   query.MathMean("score"),
    }).
    From("match").
    GroupAll()

rows, _ := q.Execute(ctx, client)
```

For the same pattern without the builder boilerplate, use
`AggregateRecords` + `AggregateOpts` (see
[Query UX](query-ux.md)).

## What's next

- **[Query UX](query-ux.md)** -- first-class helpers added in v0.2.0.
- **[Query Hints](query_hints.md)** -- every supported optimization hint.
- **[v3 Patterns](v3-patterns.md)** -- interactive transactions, `GROUP ALL`,
  raw record-id targets.
- **[Visualization](visualization.md)** -- schema diagrams.
