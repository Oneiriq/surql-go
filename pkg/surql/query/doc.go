// Package query provides query construction and result handling for the
// surql-go library. It is a 1:1 port of surql-py/src/surql/query/.
//
// This increment exposes:
//   - the hints API (IndexHint, ParallelHint, TimeoutHint, FetchHint, ExplainHint)
//   - the result wrappers (QueryResult, RecordResult, ListResult, CountResult,
//     AggregateResult, PageInfo, PaginatedResult) plus the raw-response
//     extraction helpers (ExtractResult, ExtractOne, ExtractScalar, HasResults)
//   - typed SurrealQL expression helpers (Field, Value, Raw, Func, and function
//     builders such as Count, MathMean, ArrayContains, Concat, ...)
//   - the immutable fluent Query builder with SELECT / INSERT / UPDATE /
//     UPSERT / DELETE / RELATE operations, graph traversal, vector search,
//     return-format, and optimization hints
//   - standalone helper constructors (Select, FromTable, Insert, Update,
//     Upsert, Delete, Relate, VectorSearchQuery, SimilaritySearchQuery)
//   - the query executor (ExecuteQuery, ExecuteRaw, FetchOne, FetchAll,
//     FetchMany, ExecuteRawTyped) layered on connection.DatabaseClient
//   - JSON-in/JSON-out record CRUD helpers (CreateRecord, CreateRecords,
//     GetRecord, UpdateRecord, MergeRecord, UpsertRecord, DeleteRecord,
//     DeleteRecords, QueryRecords, CountRecords, Exists, First, Last) with a
//     QueryOptions value type for filters and pagination
//   - the generic typed variants (CreateTyped, GetTyped, QueryTyped,
//     UpdateTyped, UpsertTyped) that round-trip through encoding/json.
package query
