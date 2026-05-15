# Changelog

All notable changes to this project will be documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and
this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `connection.Protocol.IsSupported()` reports whether the currently-linked
  `surrealdb.go` SDK can actually open a connection for the scheme.
  Remote transports (`ws://`, `wss://`, `http://`, `https://`) return
  true; embedded schemes (`memory://`, `mem://`, `file://`,
  `surrealkv://`) return false pending
  [surrealdb.go#197](https://github.com/surrealdb/surrealdb.go/issues/197).
- `migration/versioning` -- `SchemaSnapshot` (extended with `Version`,
  `Timestamp`, `Description`, `Accesses`), `VersionGraph` DAG with
  ancestors/descendants/path, and JSON-file snapshot persistence.
- `migration/generator` -- `GenerateMigration`, `GenerateInitialMigration`,
  `CreateBlankMigration`, `GenerateMigrationFromDiffs` with atomic writes.
- `migration/diff` -- schema diff engine (`DiffTables`, `DiffFields`,
  `DiffIndexes`, `DiffEvents`, `DiffPermissions`, `DiffEdges`,
  `DiffSchemas`, `SchemaSnapshot`).
- `migration/{models, discovery}` -- `.surql` file-format migrations with
  `-- @metadata`, `-- @up`, `-- @down` section markers + SHA-256 checksum.
- `schema/{visualize, themes, utils}` -- Mermaid / GraphViz / ASCII
  diagrams with modern / dark / forest / minimal themes.
- `schema/parser` -- `ParseDBInfo` / `ParseTableInfo` / `ParseEdgeInfo`
  accept both short and long INFO response keys.
- `schema/{validator, validator_utils}` -- cross-schema validation with
  severity-filtered reports, `CompareSchemas`.
- `schema/{sql, registry}` -- full DEFINE-statement composition and a
  thread-safe `SchemaRegistry`.
- `schema/{fields, table, edge, access}` -- code-first schema DSL.
- `query/{builder, helpers}` -- immutable `Query` with fluent chaining.
- `query/expressions` -- 25+ function builders and typed `Expression.Kind`.
- `query/{hints, results}` -- query optimization hints + typed result
  wrappers (`QueryResult[T]`, `RecordResult[T]`, `ListResult[T]`,
  `PaginatedResult[T]`) with raw-response extraction helpers.
- `connection/{config, auth}` -- connection configuration + auth
  credential types (Root / Namespace / Database / Scope / Token).
- `types/{operators, record_id, record_ref, surreal_fn, reserved, coerce}`
  -- operator structs + `RecordID` with angle-bracket syntax + reserved
  words + ISO-8601 datetime coercion.
- `errors` -- sentinel errors + typed `SurqlError` with `errors.Is`/`As`.

### Changed

- `DatabaseClient.Connect` now fails fast with a descriptive
  `ErrConnection` when passed an embedded URL scheme (`memory://`,
  `mem://`, `file://`, `surrealkv://`), instead of retrying the upstream
  `"embedded database not enabled"` error from `surrealdb.go`. Remote
  transports are unaffected. README grew a protocol-support table
  documenting the current state (#95).

### Notes

This is a pre-release port of [surql-py](https://github.com/Oneiriq/surql-py)
targeting 1:1 feature parity. The runtime SurrealDB client, CRUD
executor, and CLI land in the 0.1 -> 0.2 window.

## [0.2.3] - 2026-05-15

### Changed

- Pinned `surrealdb.go` to the stable `v1.4.0` release instead of an
  unstable pseudo-version snapshot.
- Code-quality workflows (CI, coverage) now run only on pull requests;
  the nightly cron was disabled (manual `workflow_dispatch` only).
- The docs workflow runs `mkdocs build --strict` on pull requests, so
  broken links and nav errors are caught before merge instead of only in
  the post-merge deploy.

### Docs

- Replaced informal "wave" phrasing with neutral wording.

### Known issues

- `TestIntegration_LiveQueryReceivesChange` is skipped: `surrealdb.go`
  v1.4.0 has a shutdown race in `CloseLiveNotifications` (Kill closes the
  notification channel while the SDK's readLoop still writes to it). It
  will be re-enabled once the SDK patches `CloseLiveNotifications`.
