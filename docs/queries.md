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

Once the SurrealDB client lands, queries produce typed
`QueryResult[T]` / `RecordResult[T]` / `ListResult[T]` /
`PaginatedResult[T]` values via the raw-response helpers in
`pkg/surql/query`: `ExtractResult`, `ExtractOne`, `ExtractScalar`,
`HasResults`.

## What's next

- **[Query Hints](query_hints.md)** -- every supported optimization hint.
- **[Visualization](visualization.md)** -- schema diagrams.
