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
  `-- @up` / `-- @down` section markers.
- **Type-Safe Query Builder** -- Immutable fluent API with operator-typed
  `Where`, expression helpers, and `encoding/json` struct tags.
- **Vector Search** -- HNSW and MTREE index support with 8 distance metrics
  and EFC/M tuning.
- **Graph Traversal** -- Native SurrealDB graph features with edge
  relationships.
- **Schema Visualization** -- Mermaid, GraphViz, and ASCII diagrams with
  modern / dark / forest / minimal themes.
- **CLI Tools** -- Migrations, schema inspection, validation, database
  management *(planned)*.
- **Stdlib-First** -- Minimal dependencies; stdlib `net/http` + official
  SurrealDB SDK *(planned for 0.2)*.

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
- **[Schema Definition](schema.md)** -- the schema DSL in depth.
- **[Migrations](migrations.md)** -- diff-based migration generation, file
  format, and versioning.
- **[Query Builder](queries.md)** -- immutable fluent queries.
- **[Query Hints](query_hints.md)** -- INDEX / PARALLEL / TIMEOUT / FETCH /
  EXPLAIN hints.
- **[Visualization](visualization.md)** -- Mermaid / GraphViz / ASCII
  diagrams.
- **[CLI](cli.md)** -- the `surql` binary *(planned)*.
- **[Changelog](changelog.md)** -- release history.
- **[API reference](https://pkg.go.dev/github.com/Oneiriq/surql-go)** --
  generated godoc.

## Sister projects

- **Python**: [surql-py](https://github.com/Oneiriq/surql-py)
- **TypeScript / Deno / Node.js**: [surql](https://github.com/Oneiriq/surql)
- **Rust**: [surql-rs](https://github.com/Oneiriq/surql-rs)

## License

[Apache License 2.0](https://github.com/Oneiriq/surql-go/blob/main/LICENSE).
