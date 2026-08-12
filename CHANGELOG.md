# Changelog

All notable changes to this project will be documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and
this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **Row-level filtering on the graph helpers (`conditions`).** `Traverse`,
  `TraverseWithDepth`, `GetOutgoingEdges`, `GetIncomingEdges`,
  `GetRelatedRecords`, and `ShortestPath` take a trailing `conditions []any`.
  Each entry is rendered by `Query.Where`, so raw SurrealQL strings and
  `types.Operator` values are both accepted and may be mixed in one slice;
  entries combine with `AND`. Pass `nil` to filter nothing. Restores parity
  with the sibling ports, which have accepted `conditions` on their graph
  helpers since surql-py 1.6.0.

  Previously these helpers emitted a bare `SELECT * FROM record->edge` with no
  filtering hook at all, so a caller needing row-level isolation -- a mandatory
  `WHERE tenant_id = ...` alongside engine-enforced `PERMISSIONS` -- could not
  express it and had to abandon the helpers for a hand-rolled
  equality-filtered edge table.

- **DISKANN vector indexes and the F16 element type (SurrealDB 3.2).**
  `IndexTypeDiskAnn` and `DiskAnnIndex(name, column, dimension,
  DiskAnnIndexOptions{...})` define the on-disk ANN graph the 3.2 engine
  parses, with `DiskAnnDistanceType` for the metric (`COSINE` /
  `COSINE_NORMALIZED` / `EUCLIDEAN` / `INNER_PRODUCT`). It is its own type
  because the engine's DISKANN metric set neither contains nor is contained by
  the HNSW one, so an out-of-set metric is unrepresentable rather than merely
  refused. `MTreeVectorType` gained `F16`, `I8`, and `U8`, which HNSW also
  accepts, alongside `ValidForMTree` / `ValidForDiskAnn` predicates. The
  schema emitter, the `INFO FOR TABLE` parser, and the migration diff all
  carry the new form.

  The engine echoes a DISKANN index with `DIST` / `TYPE` / `DEGREE` /
  `L_BUILD` / `ALPHA` always spelled, defaults `EUCLIDEAN` / `F32` / 64 / 100
  / 1.2 filled in even when the definition never stated them, and a float
  `ALPHA` carrying a trailing `f` suffix (`ALPHA 1.2f`). The emitter spells
  the defaults, `CanonicalAlpha` renders a whole number bare (`ALPHA 2`), and
  the parser excludes the `f` from its capture, so a definition compares equal
  to its own echo instead of re-applying on every reconcile.

  `IndexDefinition.Validate` refuses what the engine refuses, by name: a
  DISKANN element type outside `F32` / `F16` / `I8` / `U8`, an MTREE element
  type among the new `F16` / `I8` / `U8` (MTREE still parses only its
  historical five), and an MTREE/HNSW metric aimed at a DISKANN index, which
  only `DiskAnnDistance` can carry.

- **`Query.VectorSearchIndexed(field, vector, k, ef)` reaches a vector
  index.** The second argument of the KNN operator decides the plan: an
  integer is the exploration factor and the engine answers with a `KnnScan`
  over the field's HNSW or DISKANN index, while a metric keyword there asks
  for an exhaustive `KnnTopK` over a table scan. Only the metric form existed,
  so a query against an indexed column scanned the table and the index it was
  paying to build served nothing. `VectorSearch` keeps its meaning for a
  deliberate exhaustive comparison; reach for the new method whenever the
  column carries an index. The metric belongs to the index, so the new method
  takes none.

### Fixed

- **`GetIncomingEdges` and `CountRelated` (incoming direction) were broken on
  SurrealDB v3.** Both emitted the Python port's `FROM <-edge<-record`
  ordering, which v3 rejects outright: `SELECT * FROM <-follows<-person:bob`
  fails to parse with ``Unexpected token `;` `` (verified against v3.0.5).
  Both now put the record at the head of the `FROM` expression
  (`FROM record<-edge`), matching the ordering the Rust port already used.
  Any caller relying on incoming-edge traversal was receiving a parse error.

- **The `INFO FOR TABLE` parser dropped unrecognised vector element types.**
  `extractVectorType` matched against a fixed set of five, so an index
  defined with any newer element type parsed back with an empty type and the
  next reconcile saw a difference that was not there. The map now covers every
  member of `MTreeVectorType`.

### Changed

- **Graph helpers compose through `Query` instead of `fmt.Sprintf`.** Every
  `SELECT`-shaped helper now builds its statement with
  `NewQuery().Select(nil).FromTable(...).Traverse(...)` rather than
  interpolating identifiers into a format string. Statement construction moved
  into `selectTraversalSurql` / `applyConditions`, which are unit-testable
  without a live client.

- **BREAKING -- graph helper signatures.** The six helpers above take a new
  trailing `conditions []any` parameter. Go has no default arguments, so this
  follows the package's existing convention for optional arguments (as
  `TraverseWithDepth`'s `depth *int` already does). Existing call sites migrate
  by passing `nil`, which leaves the emitted SurrealQL unchanged. `CountRelated`
  is deliberately **not** given `conditions` -- surql-py does not filter it
  either, and diverging would break the 1:1 contract.

## [0.4.0] - 2026-07-30

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
