# Changelog

All notable changes to this project will be documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and
this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
