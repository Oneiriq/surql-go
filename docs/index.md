# surql-go

A code-first database toolkit for [SurrealDB](https://surrealdb.com/).
Define schemas, generate migrations, build queries, and perform typed CRUD
-- all from Go.

> Go port of [surql-py](https://github.com/Oneiriq/surql-py) (Python) and
> [@oneiriq/surql](https://github.com/Oneiriq/surql) (TypeScript / Deno).
> 1:1 feature parity is the target.

## Features

- **Code-First Migrations** -- Schema changes defined in code with automatic
  migration generation. Files use a portable `.surql` format with
  `-- @up` / `-- @down` section markers. Squash, watcher, and auto-snapshot
  hooks included.
- **Type-Safe Query Builder** -- Immutable fluent API with operator-typed
  `Where`, expression helpers, `SelectExpr` / `SelectAliased` aggregations,
  and `encoding/json` struct tags.
- **v3 Transactions** -- Native interactive `BEGIN` / `COMMIT` / `ROLLBACK`
  via `DatabaseClient.Begin`, plus raw record-id targets
  (`types.TypeRecord` / `types.TypeThing`) and `GROUP ALL` aggregation.
- **Vector Search** -- HNSW and MTREE index support with 8 distance metrics
  and EFC/M tuning.
- **Graph Traversal** -- Native SurrealDB graph features with edge
  relationships + fluent `GraphQuery` builder.
- **Schema Visualization** -- Mermaid, GraphViz, and ASCII diagrams with
  modern / dark / forest / minimal themes.
- **CLI Tools** -- Full `surql` binary for migrations, schema inspection,
  validation, database management, and multi-environment orchestration.
- **Cache + Orchestration** -- Memory + Redis cache backends,
  sequential / parallel / rolling / canary deployment strategies.

## Quick Start

```shell
go get github.com/Oneiriq/surql-go
```

```go
package main

import "github.com/Oneiriq/surql-go/pkg/surql/schema"

func main() {
    user, _ := schema.NewTableDefinition(
        "user",
        schema.WithMode(schema.TableModeSchemafull),
        schema.WithFields(
            schema.StringField("name"),
            schema.StringField("email").WithAssertion("string::is::email($value)"),
            schema.IntField("age").WithAssertion("$value >= 0 AND $value <= 150"),
            schema.DatetimeField("created_at").WithDefault("time::now()").WithReadonly(true),
        ),
        schema.WithIndexes(schema.UniqueIndex("email_idx", []string{"email"})),
    )
    _ = user
}
```

## Documentation

- **[Installation](installation.md)** -- getting the module installed.
- **[Quick Start](quickstart.md)** -- your first schema and migration.
- **[Features & Package Layout](features.md)** -- per-package surface map.
- **[Schema Definition](schema.md)** -- the schema DSL in depth.
- **[Migrations](migrations.md)** -- diff-based migration generation, file
  format, and versioning.
- **[Query Builder](queries.md)** -- immutable fluent queries.
- **[Query UX](query-ux.md)** -- first-class helpers added in v0.2.0.
- **[Query Hints](query_hints.md)** -- INDEX / PARALLEL / TIMEOUT / FETCH /
  EXPLAIN hints.
- **[v3 Patterns](v3-patterns.md)** -- interactive transactions,
  `GROUP ALL`, raw record-id targets, known upstream SDK limitations.
- **[Visualization](visualization.md)** -- Mermaid / GraphViz / ASCII
  diagrams.
- **[CLI Reference](cli.md)** -- the `surql` subcommand surface.
- **[Upgrading](migration.md)** -- version-to-version upgrade notes.
- **[Changelog](changelog.md)** -- release history.
- **[API reference](https://pkg.go.dev/github.com/Oneiriq/surql-go)** --
  generated godoc.

## Sister projects

- **Python**: [surql-py](https://github.com/Oneiriq/surql-py)
- **TypeScript / Deno / Node.js**: [surql](https://github.com/Oneiriq/surql)
- **Rust**: [surql-rs](https://github.com/Oneiriq/surql-rs)

## License

[Apache License 2.0](https://github.com/Oneiriq/surql-go/blob/main/LICENSE).
