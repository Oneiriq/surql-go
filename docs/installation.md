# Installation

## Library

```shell
go get github.com/Oneiriq/surql-go
```

Or in `go.mod`:

```go
require github.com/Oneiriq/surql-go v0.1.0
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
- For the client feature: SurrealDB 2.0 or newer.

## What's next

- **[Quick Start](quickstart.md)** -- your first schema and migration.
- **[Schema Definition](schema.md)** -- the full schema DSL reference.
