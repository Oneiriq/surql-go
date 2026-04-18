// Package query provides query construction and result handling for the
// surql-go library. It is a 1:1 port of surql-py/src/surql/query/.
//
// This increment exposes the hints API (IndexHint, ParallelHint, TimeoutHint,
// FetchHint, ExplainHint) and the result wrappers (QueryResult, RecordResult,
// ListResult, CountResult, AggregateResult, PageInfo, PaginatedResult) plus
// the raw-response extraction helpers (ExtractResult, ExtractOne,
// ExtractScalar, HasResults).
//
// Subsequent increments add the immutable Query builder, expressions,
// typed/batch/graph CRUD, and the async executor.
package query
