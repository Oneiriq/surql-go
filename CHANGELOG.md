# Changelog

All notable changes to this project will be documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and
this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **SurrealDB v3 object storage (buckets/files) is now first-class.** Define a
  bucket in code with `NewBucket(name, backend, opts...)` (or the
  `MemoryBucket` / `FileBucket` / `S3Bucket` shorthands) -- `BucketDefinition`
  renders `DEFINE` / `REMOVE` / `ALTER BUCKET`, is parsed back from `INFO FOR
  DB`, and is diffed by the migration engine (`ADD` / `DROP` /
  `MODIFY_BUCKET`). Model file- and bytes-typed fields with `FileField` /
  `BytesField`, and carry file pointers as the `types.FileRef` value type
  (canonical `<bucket>:/<key>` literal, `{bucket,key}` JSON/CBOR round-trip).
  At runtime `DatabaseClient.Bucket(name)` (or `Session.Bucket`) returns a
  handle with `Put` / `PutIfNotExists` / `Get` / `GetText` / `Exists` / `Head`
  / `Delete` / `Copy` / `CopyIfNotExists` / `Rename` / `RenameIfNotExists` /
  `List` -- every operation binds `type::file($bucket, $key)` as query
  parameters (never interpolated), and `Head` / `List` project `file::bucket`
  / `file::key` server-side so results decode as plain scalars (surrealdb.go
  ships no decoder for the SurrealDB `file` CBOR tag). Keys surface in the
  server's canonical leading-slash form (`/a.txt`). The `surql bucket` CLI
  covers define/list/rm/put/get/delete/exists. Requires a server started with
  `SURREAL_CAPS_ALLOW_EXPERIMENTAL=files`.
- **Multiple sessions over a single WebSocket connection (SurrealDB v3).**
  `DatabaseClient.Attach(ctx)` returns a `*Session` that mirrors the client
  surface (`Query` / `QueryWithVars` / `Select` / `Create` / `Update` /
  `Merge` / `Delete` plus `Use` / `Signin` / `Authenticate` / `Invalidate` /
  `Detach` and `Bucket`), with authentication and namespace/database state
  isolated per session. Attaching over HTTP returns `ErrConnection` --
  sessions are WebSocket-only.

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
