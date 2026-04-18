# surql-go

[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-1.26%2B-00ADD8)](https://go.dev/)
[![SurrealDB](https://img.shields.io/badge/SurrealDB-2.0%2B-ff00a0)](https://surrealdb.com/)

A code-first database toolkit for [SurrealDB](https://surrealdb.com/). Define schemas, generate migrations, build queries, and perform typed CRUD -- all from Go.

## Features

- **Code-First Migrations** - Schema changes defined in code with automatic migration generation (auto-diff + `.surql` file output with `-- @up` / `-- @down` sections)
- **Type-Safe Query Builder** - Immutable fluent API with operator-typed `Where`, expression helpers, and `encoding/json` struct tags
- **Vector Search** - HNSW and MTREE index support with 8 distance metrics and EFC/M tuning
- **Graph Traversal** - Native SurrealDB graph features with edge relationships
- **Schema Visualization** - Mermaid, GraphViz, and ASCII diagrams with theming
- **CLI Tools** - Migrations, schema inspection, validation, database management *(planned)*
- **Stdlib-First** - Minimal dependencies; stdlib `net/http` + official SurrealDB SDK *(planned)*

## Quick Start

```shell
go get github.com/Oneiriq/surql-go
```

With the CLI:

```shell
go install github.com/Oneiriq/surql-go/cmd/surql@latest
```

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
    // ...
}
```

## Documentation

Full documentation at **[oneiriq.github.io/surql-go](https://oneiriq.github.io/surql-go/)**.

## Requirements

- Go 1.26+
- SurrealDB 2.0+

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
