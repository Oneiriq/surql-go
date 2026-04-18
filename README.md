# surql-go

Code-first database toolkit for SurrealDB -- schema definitions, migrations, query building, typed CRUD.

Go port of [`oneiriq-surql`](https://github.com/Oneiriq/surql-py) (Python) and [`@oneiriq/surql`](https://github.com/Oneiriq/surql) (TypeScript/Deno). Target: 1:1 feature parity.

Status: `0.1.0-dev` -- foundation under active port.

## Features (target parity)

- Connection management (WebSocket/HTTP, pooling, retry, transactions, live queries)
- Query builder (immutable, fluent, expressions, hints, batch operations, graph traversal)
- Typed CRUD backed by `encoding/json` struct tags (create, read, update, merge, upsert, delete)
- Schema DSL (tables, fields, edges, indexes, events, access)
- Migrations (generator, executor, history, rollback, squash, versioning, watcher)
- Cache (in-memory + optional Redis backend)
- Orchestration (multi-environment deploy: sequential, parallel, rolling, canary)
- CLI (`surql` binary) for migrate, schema, db, orchestrate

## Install

```bash
go get github.com/albedosehen/surql-go
```

CLI:

```bash
go install github.com/albedosehen/surql-go/cmd/surql@latest
```

## Quick Start

Under construction. Full API will mirror `surql-py` and `@oneiriq/surql`.

## Development

```bash
make check    # fmt + vet + test
make test     # run all tests
make cover    # coverage report -> coverage.html
```

## License

Apache-2.0
