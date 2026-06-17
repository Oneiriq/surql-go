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

## [0.3.0] - 2026-06-17

### Added

- **Full-text search (BM25) is now first-class -- the sparse leg of hybrid
  retrieval.** Define a `DEFINE ANALYZER` in code with `Analyzer(name, opts...)` /
  `StandardAnalyzer(name, opts...)` (`AnalyzerDefinition` + `Tokenizer` +
  `TokenFilter`, rendered via `GenerateAnalyzerSQL` /
  `GenerateAnalyzerSQLIfNotExists`); build a BM25-scored full-text index with
  `BM25Index(name, columns, analyzer)` (or
  `SearchIndex(...).WithAnalyzer(...).WithBM25().WithHighlights()`); and run the
  lexical query with `Query.FullTextSearch(field, reference, query)` +
  `Query.SearchScore(reference, alias)`, or the `FullTextSearchQuery(...)`
  helper. `GenerateSchemaSQLFromSlicesWithAnalyzers` emits analyzer DDL before
  the tables that reference it. Pair it with `Query.VectorSearch` and fuse the
  two result orders by rank (Reciprocal Rank Fusion).

### Fixed

- **Full-text index now emits the SurrealDB 3.x `FULLTEXT` keyword.** The
  full-text index keyword was renamed from `SEARCH` to `FULLTEXT` in SurrealDB
  3.0, so the previous output (`... SEARCH ANALYZER ascii`) was a parse error on
  v3. `IndexTypeSearch` / `SearchIndex` / `IndexDefinition.ToSurql*` and the
  migration diff now emit `FULLTEXT`, and the `INFO FOR TABLE` index parser
  recognises both spellings (and extracts the analyzer / BM25 / HIGHLIGHTS
  clauses). See `docs/v3-patterns.md` "Full-text search (BM25)" -- including the
  note that the v3 streaming executor's full-text scan returns rows in BM25
  relevance order but `search::score` is not plumbed through it (returns 0), so
  rank by the scan's natural order.
