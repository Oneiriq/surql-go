# Graph Traversal

Six helpers navigate edges from a starting record, plus a fluent `GraphQuery`
builder for composing paths.

## The helpers

```go
rows, err := query.Traverse(ctx, client, "person:alice", "->likes->post", nil)
rows, err := query.TraverseWithDepth(ctx, client, "person:alice", "likes", "post", &depth, nil)
rows, err := query.GetOutgoingEdges(ctx, client, "person:alice", "follows", nil)
rows, err := query.GetIncomingEdges(ctx, client, "person:bob", "follows", nil)
rows, err := query.GetRelatedRecords(ctx, client, "person:alice", "follows", "person", nil)
path, err := query.ShortestPath(ctx, client, "person:alice", "person:bob", "follows", nil)
n, err := query.CountRelated(ctx, client, "person:alice", "follows")
```

`Traverse` injects `path` verbatim, so compose it from validated identifiers or
use `TraverseWithDepth`, which builds the path for you.

## Row-level filtering

Every helper above except `CountRelated` takes a trailing `conditions []any`.
Each entry is rendered through `Query.Where`, so raw SurrealQL strings and
`types.Operator` values both work, and entries combine with `AND`.

```go
rows, err := query.GetOutgoingEdges(ctx, client, "person:alice", "follows",
    []any{types.Gt("since", "2026-01-01"), "active = true"})
```

Pass `nil` to filter nothing, which leaves the emitted statement unchanged from
the unfiltered form.

`CountRelated` takes no conditions, matching surql-py, so the two ports keep
a one-to-one contract.

### Upgrading

The `conditions` parameter is a breaking signature change. Go has no default
arguments, so it follows the convention `TraverseWithDepth` already set with
its `depth *int`. Existing call sites migrate by passing `nil`:

```go
// before
rows, err := query.GetOutgoingEdges(ctx, client, start, "follows")
// after
rows, err := query.GetOutgoingEdges(ctx, client, start, "follows", nil)
```

## Direction and SurrealDB v3

Incoming traversal puts the record at the head of the `FROM` expression:

```sql
SELECT * FROM person:bob<-follows
```

The reverse ordering (`FROM <-follows<-person:bob`) is a parse error on v3. The
helpers emit the correct form; the note matters only when hand-writing a
statement or reading older examples.

See [v3 Patterns](v3-patterns.md) for the rest of the version-specific
grammar.
