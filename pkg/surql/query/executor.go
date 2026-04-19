package query

import (
	"context"
	"encoding/json"

	"github.com/Oneiriq/surql-go/pkg/surql/connection"
	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

// ExecuteQuery renders q to SurrealQL and executes it against client. The
// returned value is the raw response envelope produced by DatabaseClient
// (a []any of per-statement results), suitable for ExtractResult / ExtractOne
// or for downstream typed decoding.
//
// Returns ErrValidation if the query is incomplete, or ErrQuery when the
// underlying connection surface fails.
func ExecuteQuery(ctx context.Context, client *connection.DatabaseClient, q Query) (any, error) {
	if client == nil {
		return nil, surqlerrors.New(surqlerrors.ErrValidation, "client cannot be nil")
	}
	surql, err := q.ToSurql()
	if err != nil {
		return nil, err
	}
	return client.Query(ctx, surql)
}

// Execute is the method form of [ExecuteQuery], letting callers chain
// `NewQuery().Select(...).From(...).GroupAll().Execute(ctx, client)`.
func (q Query) Execute(ctx context.Context, client *connection.DatabaseClient) (any, error) {
	return ExecuteQuery(ctx, client, q)
}

// ExecuteRaw runs a raw SurrealQL statement with the supplied parameter map.
// Pass a nil vars map for a parameter-less query.
func ExecuteRaw(ctx context.Context, client *connection.DatabaseClient, surql string, vars map[string]any) (any, error) {
	if client == nil {
		return nil, surqlerrors.New(surqlerrors.ErrValidation, "client cannot be nil")
	}
	if surql == "" {
		return nil, surqlerrors.New(surqlerrors.ErrValidation, "surql cannot be empty")
	}
	return client.QueryWithVars(ctx, surql, vars)
}

// FetchOne executes q and decodes the first record into *T. Returns
// (nil, nil) when the response contains no rows.
func FetchOne[T any](ctx context.Context, client *connection.DatabaseClient, q Query) (*T, error) {
	raw, err := ExecuteQuery(ctx, client, q)
	if err != nil {
		return nil, err
	}
	record := ExtractOne(raw)
	if record == nil {
		return nil, nil
	}
	out, err := decodeRecord[T](record)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// FetchAll executes q and decodes every record into []T. Returns an empty
// slice when the response carries no rows.
func FetchAll[T any](ctx context.Context, client *connection.DatabaseClient, q Query) ([]T, error) {
	raw, err := ExecuteQuery(ctx, client, q)
	if err != nil {
		return nil, err
	}
	return decodeRecords[T](ExtractResult(raw))
}

// FetchMany executes q and wraps the decoded rows in a ListResult, carrying
// any LimitValue/OffsetValue that were set on the query.
func FetchMany[T any](ctx context.Context, client *connection.DatabaseClient, q Query) (ListResult[T], error) {
	items, err := FetchAll[T](ctx, client, q)
	if err != nil {
		return ListResult[T]{}, err
	}
	var limit, offset *uint64
	if q.LimitValue != nil {
		v := uint64(*q.LimitValue)
		limit = &v
	}
	if q.OffsetValue != nil {
		v := uint64(*q.OffsetValue)
		offset = &v
	}
	return Records[T](items, nil, limit, offset), nil
}

// FetchRecord executes q and wraps the first decoded row in a RecordResult.
// Returns a RecordResult whose Exists flag is false when the query yielded
// no rows, mirroring surql-py's `fetch_record`.
func FetchRecord[T any](ctx context.Context, client *connection.DatabaseClient, q Query) (RecordResult[T], error) {
	rec, err := FetchOne[T](ctx, client, q)
	if err != nil {
		return RecordResult[T]{}, err
	}
	return Record(rec, rec != nil), nil
}

// FetchRecords executes q and wraps the decoded rows in a ListResult that
// retains the Limit/Offset hints from q. Mirrors surql-py's `fetch_records`.
func FetchRecords[T any](ctx context.Context, client *connection.DatabaseClient, q Query) (ListResult[T], error) {
	return FetchMany[T](ctx, client, q)
}

// ExecuteRawTyped runs a raw SurrealQL query and decodes every row into T.
func ExecuteRawTyped[T any](ctx context.Context, client *connection.DatabaseClient, surql string, vars map[string]any) ([]T, error) {
	raw, err := ExecuteRaw(ctx, client, surql, vars)
	if err != nil {
		return nil, err
	}
	return decodeRecords[T](ExtractResult(raw))
}

// ---------------------------------------------------------------------------
// JSON round-trip helpers
// ---------------------------------------------------------------------------

// decodeRecord round-trips `record` through encoding/json into a fresh T.
func decodeRecord[T any](record map[string]any) (T, error) {
	var out T
	buf, err := json.Marshal(record)
	if err != nil {
		return out, surqlerrors.Wrap(surqlerrors.ErrSerialization, "encode record", err)
	}
	if err := json.Unmarshal(buf, &out); err != nil {
		return out, surqlerrors.Wrap(surqlerrors.ErrSerialization, "decode record", err)
	}
	return out, nil
}

// decodeRecords round-trips every element of `records` through
// encoding/json into []T, returning a non-nil empty slice when records is
// empty.
func decodeRecords[T any](records []map[string]any) ([]T, error) {
	if len(records) == 0 {
		return []T{}, nil
	}
	out := make([]T, 0, len(records))
	for _, rec := range records {
		v, err := decodeRecord[T](rec)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}
