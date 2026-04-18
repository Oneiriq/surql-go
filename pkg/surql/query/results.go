package query

import (
	"encoding/json"

	surqlerrors "github.com/albedosehen/surql-go/pkg/surql/errors"
)

// QueryResult is the generic query execution wrapper.
type QueryResult[T any] struct {
	Data   T      `json:"data"`
	Time   string `json:"time,omitempty"`
	Status string `json:"status"`
}

// Success builds a QueryResult with status="OK".
func Success[T any](data T, execTime string) QueryResult[T] {
	return QueryResult[T]{Data: data, Time: execTime, Status: "OK"}
}

// RecordResult wraps a single (possibly-missing) record.
type RecordResult[T any] struct {
	Record *T   `json:"record"`
	Exists bool `json:"exists"`
}

// Record builds a RecordResult.
func Record[T any](rec *T, exists bool) RecordResult[T] {
	return RecordResult[T]{Record: rec, Exists: exists}
}

// Unwrap returns the record or panics if nil.
func (r RecordResult[T]) Unwrap() T {
	if r.Record == nil {
		panic("RecordResult.Unwrap called on a nil record")
	}
	return *r.Record
}

// TryUnwrap returns the record or ErrValidation when nil.
func (r RecordResult[T]) TryUnwrap() (T, error) {
	var zero T
	if r.Record == nil {
		return zero, surqlerrors.New(surqlerrors.ErrValidation, "Cannot unwrap nil record")
	}
	return *r.Record, nil
}

// UnwrapOr returns the record or the given default when nil.
func (r RecordResult[T]) UnwrapOr(def T) T {
	if r.Record == nil {
		return def
	}
	return *r.Record
}

// ListResult wraps a slice of records with optional pagination hints.
type ListResult[T any] struct {
	Records []T     `json:"records"`
	Total   *uint64 `json:"total,omitempty"`
	Limit   *uint64 `json:"limit,omitempty"`
	Offset  *uint64 `json:"offset,omitempty"`
	HasMore bool    `json:"has_more"`
}

// Records builds a ListResult with has_more inferred from the pagination
// inputs (mirrors surql-py's heuristic).
func Records[T any](items []T, total, limit, offset *uint64) ListResult[T] {
	hasMore := false
	switch {
	case total != nil && limit != nil && offset != nil:
		hasMore = (*offset)+(*limit) < *total
	case total == nil && limit != nil:
		hasMore = uint64(len(items)) == *limit
	}
	return ListResult[T]{
		Records: items, Total: total, Limit: limit, Offset: offset, HasMore: hasMore,
	}
}

// Len returns the number of records.
func (r ListResult[T]) Len() int { return len(r.Records) }

// IsEmpty reports whether the list is empty.
func (r ListResult[T]) IsEmpty() bool { return len(r.Records) == 0 }

// First returns the first record or nil.
func (r ListResult[T]) First() *T {
	if len(r.Records) == 0 {
		return nil
	}
	return &r.Records[0]
}

// Last returns the last record or nil.
func (r ListResult[T]) Last() *T {
	if len(r.Records) == 0 {
		return nil
	}
	return &r.Records[len(r.Records)-1]
}

// CountResult holds a COUNT aggregation.
type CountResult struct {
	Count int64 `json:"count"`
}

// NewCountResult builds a CountResult.
func NewCountResult(n int64) CountResult { return CountResult{Count: n} }

// AggregateResult is the generic aggregation wrapper.
type AggregateResult struct {
	Value     json.RawMessage `json:"value"`
	Operation string          `json:"operation,omitempty"`
	Field     string          `json:"field,omitempty"`
}

// Aggregate builds an AggregateResult. The value can be any JSON-serialisable
// value; it is re-encoded into json.RawMessage for opaque round-tripping.
func Aggregate(value any, operation, field string) (AggregateResult, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return AggregateResult{}, surqlerrors.Wrap(surqlerrors.ErrSerialization, "aggregate value", err)
	}
	return AggregateResult{Value: raw, Operation: operation, Field: field}, nil
}

// PageInfo carries pagination metadata.
type PageInfo struct {
	CurrentPage uint64 `json:"current_page"`
	PageSize    uint64 `json:"page_size"`
	TotalPages  uint64 `json:"total_pages"`
	TotalItems  uint64 `json:"total_items"`
	HasPrevious bool   `json:"has_previous"`
	HasNext     bool   `json:"has_next"`
}

// PaginatedResult wraps a page of items with metadata.
type PaginatedResult[T any] struct {
	Items    []T      `json:"items"`
	PageInfo PageInfo `json:"page_info"`
}

// Paginated builds a PaginatedResult. `page` is 1-indexed.
func Paginated[T any](items []T, page, pageSize, total uint64) PaginatedResult[T] {
	var totalPages uint64
	if pageSize > 0 {
		totalPages = (total + pageSize - 1) / pageSize
	}
	return PaginatedResult[T]{
		Items: items,
		PageInfo: PageInfo{
			CurrentPage: page,
			PageSize:    pageSize,
			TotalPages:  totalPages,
			TotalItems:  total,
			HasPrevious: page > 1,
			HasNext:     page < totalPages,
		},
	}
}

// Len returns the number of items on the current page.
func (p PaginatedResult[T]) Len() int { return len(p.Items) }

// IsEmpty reports whether the current page is empty.
func (p PaginatedResult[T]) IsEmpty() bool { return len(p.Items) == 0 }

// ExtractResult pulls the array of records from a raw SurrealDB response,
// handling both nested (`[{"result": [...]}]` from db.query) and flat
// (`[{...}, ...]` from db.select) formats.
func ExtractResult(result any) []map[string]any {
	if result == nil {
		return nil
	}
	// []any top-level
	if arr, ok := result.([]any); ok {
		if len(arr) == 0 {
			return nil
		}
		// Nested?
		if first, ok := arr[0].(map[string]any); ok {
			if _, hasResult := first["result"]; hasResult {
				var out []map[string]any
				for _, item := range arr {
					m, ok := item.(map[string]any)
					if !ok {
						continue
					}
					inner, has := m["result"]
					if !has {
						continue
					}
					pushValue(&out, inner)
				}
				return out
			}
		}
		// Flat: collect map[string]any elements only
		var out []map[string]any
		for _, item := range arr {
			if m, ok := item.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	}
	// Single object with "result"
	if m, ok := result.(map[string]any); ok {
		if inner, has := m["result"]; has {
			var out []map[string]any
			pushValue(&out, inner)
			return out
		}
	}
	return nil
}

func pushValue(out *[]map[string]any, v any) {
	switch vv := v.(type) {
	case []any:
		for _, x := range vv {
			if m, ok := x.(map[string]any); ok {
				*out = append(*out, m)
			}
		}
	case map[string]any:
		*out = append(*out, vv)
	case nil:
		// no-op
	default:
		*out = append(*out, map[string]any{"value": vv})
	}
}

// ExtractOne returns the first record from a raw response, or nil.
func ExtractOne(result any) map[string]any {
	records := ExtractResult(result)
	if len(records) == 0 {
		return nil
	}
	return records[0]
}

// ExtractScalar pulls a scalar value from an aggregate response. Returns
// `def` when the result is empty or the key is missing.
func ExtractScalar(result any, key string, def any) any {
	one := ExtractOne(result)
	if one == nil {
		return def
	}
	if v, ok := one[key]; ok {
		return v
	}
	return def
}

// HasResults reports whether the response contains at least one record.
func HasResults(result any) bool {
	return len(ExtractResult(result)) > 0
}
