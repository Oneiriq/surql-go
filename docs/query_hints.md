# Query Hints

Hints are SurrealQL comment annotations that the query planner consumes.
They're strongly typed so builder chains can dedup + validate them before
rendering.

## Kinds

| Hint            | Rendering                                 |
|-----------------|-------------------------------------------|
| `IndexHint`     | `/* USE INDEX table.index */` or `/* FORCE INDEX … */` |
| `ParallelHint`  | `/* PARALLEL ON */` / `OFF` / `/* PARALLEL N */`       |
| `TimeoutHint`   | `/* TIMEOUT Ns */`                        |
| `FetchHint`     | `/* FETCH EAGER */` / `LAZY` / `/* FETCH BATCH N */`   |
| `ExplainHint`   | `/* EXPLAIN */` or `/* EXPLAIN FULL */`   |

## Construction

```go
idx := query.NewIndexHint("user", "email_idx").WithForce(true)
par, _ := query.ParallelWithWorkers(4)
tmo, _ := query.NewTimeoutHint(30.0)
fch, _ := query.FetchBatchHint(100)
xpn    := query.ExplainFull()
```

## Composition

Multiple hints can coexist; `MergeHints` collapses duplicates of the same
kind to the latest value (preserving insertion order of unique kinds).

```go
tmoA, _ := query.NewTimeoutHint(10.0)
tmoB, _ := query.NewTimeoutHint(20.0)
merged  := query.MergeHints([]query.QueryHint{tmoA, par, tmoB})
fmt.Println(query.RenderHints(merged))
```

## Validation

```go
idx := query.NewIndexHint("user", "email_idx")
errors := query.ValidateHint(idx, "user") // []string{}
errors  = query.ValidateHint(idx, "post") // one mismatch error
```

## What's next

- **[Query Builder](queries.md)** -- chaining hints onto queries.
