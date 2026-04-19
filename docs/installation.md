# Installation

## Library

```shell
go get github.com/Oneiriq/surql-go
```

Or in `go.mod`:

```go
require github.com/Oneiriq/surql-go v0.2.1
```

## CLI

```shell
go install github.com/Oneiriq/surql-go/cmd/surql@latest
```

The binary is named `surql`; add `$(go env GOPATH)/bin` to your `$PATH`.

## Build tags

Integration tests use the `integration` build tag and require a running
SurrealDB:

```shell
go test -tags=integration ./...
```

## Requirements

- Go 1.26 or newer.
- SurrealDB **v3.0+** for the full feature set (CI runs against
  `surrealdb/surrealdb:v3.0.5`). Interactive transactions
  (`DatabaseClient.Begin`) and `GROUP ALL` aggregations require v3.
  See [v3 Patterns](v3-patterns.md) for the v3-specific surface.

## What's next

- **[Quick Start](quickstart.md)** -- your first schema and migration.
- **[Schema Definition](schema.md)** -- the full schema DSL reference.
- **[CLI Reference](cli.md)** -- the `surql` subcommand surface.
