# surql-go

[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-1.26%2B-00ADD8)](https://go.dev/)
[![SurrealDB](https://img.shields.io/badge/SurrealDB-3.0%2B-ff00a0)](https://surrealdb.com/)

A code-first database toolkit for [SurrealDB](https://surrealdb.com/). Define schemas, generate migrations, build queries, and perform typed CRUD -- all from Go.

## Features

- **Code-First Migrations** -- Schema changes defined in code with automatic migration generation (auto-diff + `.surql` file output with `-- @up` / `-- @down` sections, squash, watcher, auto-snapshot hooks)
- **Type-Safe Query Builder** -- Immutable fluent API with operator-typed `Where`, `SelectExpr` / `SelectAliased` aggregations, function factories, and `encoding/json` struct tags
- **v3 Interactive Transactions** -- Native `BEGIN` / `COMMIT` / `ROLLBACK` via `DatabaseClient.Begin`, plus raw record-id targets and `GROUP ALL`
- **Vector Search** -- HNSW and MTREE index support with 8 distance metrics and EFC/M tuning
- **Full-Text Search (BM25)** -- the lexical leg of hybrid retrieval: `DEFINE ANALYZER` in code (`Analyzer` / `StandardAnalyzer`), BM25-scored `FULLTEXT` indexes (`BM25Index`), and `Query.FullTextSearch` / `Query.SearchScore` to query and rank
- **Graph Traversal** -- Native SurrealDB graph features with edge relationships and a fluent `GraphQuery` builder
- **Schema Visualization** -- Mermaid, GraphViz, and ASCII diagrams with theming
- **Full CLI** -- `surql migrate` / `schema` / `db` / `orchestrate` subcommands for the entire lifecycle
- **Cache + Orchestration** -- Memory + Redis cache backends, sequential / parallel / rolling / canary deployment strategies

## Install

```shell
go get github.com/Oneiriq/surql-go
```

With the CLI:

```shell
go install github.com/Oneiriq/surql-go/cmd/surql@latest
```

## Quick Start

### Define a schema

```go
package main

import (
    "github.com/Oneiriq/surql-go/pkg/surql/schema"
)

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

### Fetch a record by id (v3 raw target)

```go
import (
    "github.com/Oneiriq/surql-go/pkg/surql/query"
    "github.com/Oneiriq/surql-go/pkg/surql/types"
)

target := types.TypeRecord("user", "alice")
record, err := query.GetByTarget(ctx, client, target)
```

### Aggregate with `GROUP ALL`

```go
opts := query.AggregateOpts{
    Table: "match",
    Select: map[string]types.Operator{
        "count": query.CountAll(),
        "avg":   query.MathMean("score"),
    },
    GroupAll: true,
}
rows, _ := query.AggregateRecords(ctx, client, opts)
```

### Extract a scalar from a raw query

```go
import "github.com/Oneiriq/surql-go/pkg/surql"

raw, _ := client.Query(ctx, "SELECT count() AS n FROM user GROUP ALL")
count, _ := surql.ExtractScalar[int64](raw, "n")
```

### Interactive transaction

```go
tx, _ := client.Begin(ctx)
if _, err := tx.Execute(ctx, "UPDATE user:alice SET status = 'active'"); err != nil {
    _ = tx.Rollback(ctx)
    return err
}
return tx.Commit(ctx)
```

## Documentation

Full documentation at **[oneiriq.github.io/surql-go](https://oneiriq.github.io/surql-go/)**.

Key pages:

- [Features & Package Layout](https://oneiriq.github.io/surql-go/features/)
- [v3 Patterns](https://oneiriq.github.io/surql-go/v3-patterns/)
- [Query UX Helpers](https://oneiriq.github.io/surql-go/query-ux/)
- [CLI Reference](https://oneiriq.github.io/surql-go/cli/)
- [Upgrading](https://oneiriq.github.io/surql-go/migration/)

## Supported connection protocols

`ConnectionConfig.DBURL` accepts all URL schemes the upstream
[`surrealdb.go`](https://github.com/surrealdb/surrealdb.go) SDK parses, but
only the remote transports can actually open a connection. Embedded schemes
are parsed (so protocol detection still works) and then rejected at
`DatabaseClient.Connect` with `ErrConnection` pending upstream support.

| Scheme         | Status       | Notes                                                                                              |
| -------------- | ------------ | -------------------------------------------------------------------------------------------------- |
| `ws://`        | Supported    | WebSocket. Full feature set including live queries.                                                |
| `wss://`       | Supported    | TLS WebSocket. Full feature set including live queries.                                            |
| `http://`      | Supported    | HTTP RPC. No live queries.                                                                         |
| `https://`     | Supported    | TLS HTTP RPC. No live queries.                                                                     |
| `memory://`    | Not yet      | Parsed; `Connect` returns `ErrConnection`. Blocked by upstream [surrealdb.go#197].                 |
| `mem://`       | Not yet      | Alias of `memory://`.                                                                              |
| `file://`      | Not yet      | Parsed; `Connect` returns `ErrConnection`. Blocked by upstream [surrealdb.go#197].                 |
| `surrealkv://` | Not yet      | Parsed; `Connect` returns `ErrConnection`. Blocked by upstream [surrealdb.go#197].                 |

[surrealdb.go#197]: https://github.com/surrealdb/surrealdb.go/issues/197

Use `connection.Protocol.IsSupported()` to check programmatically whether
a configured URL can open. The library will re-enable embedded schemes
automatically as soon as the SDK ships an embedded engine -- callers will
not need to change any code.

Downstream consumers that need an offline / embedded store today (for
example [kushtaka-cli](https://github.com/Oneiriq/kushtakas)'s local
cache) should keep their file-based fallback until upstream lands.

## Requirements

- Go 1.26+
- SurrealDB 3.0+ (CI runs against `surrealdb/surrealdb:v3.0.5`)

## License

Apache License 2.0 - see [LICENSE](LICENSE).

## Python / TypeScript / Rust

- **Python**: [surql-py](https://github.com/Oneiriq/surql-py) -- the original, reference implementation (Python 3.12+).
- **TypeScript / Deno / Node.js**: [surql](https://github.com/Oneiriq/surql) -- type-safe query builder and client.
- **Rust**: [surql-rs](https://github.com/Oneiriq/surql-rs) -- Rust port of this library, sharing the same schema + migration model.

## Support

- Documentation: [oneiriq.github.io/surql-go](https://oneiriq.github.io/surql-go/)
- Issues: [GitHub Issues](https://github.com/Oneiriq/surql-go/issues)
- Changelog: [CHANGELOG.md](CHANGELOG.md)
