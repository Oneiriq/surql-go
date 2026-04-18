package query

import (
	"context"
	"encoding/json"

	"github.com/albedosehen/surql-go/pkg/surql/connection"
	surqlerrors "github.com/albedosehen/surql-go/pkg/surql/errors"
)

// CreateTyped creates a record from data and returns a validated T. The
// value is serialised through encoding/json before submission and the
// response is decoded back into a fresh T, guaranteeing tag-driven field
// mapping in both directions.
func CreateTyped[T any](ctx context.Context, client *connection.DatabaseClient, table string, data T) (T, error) {
	var zero T
	payload, err := toJSONMap(data)
	if err != nil {
		return zero, err
	}
	res, err := CreateRecord(ctx, client, table, payload)
	if err != nil {
		return zero, err
	}
	return decodeRecord[T](res)
}

// GetTyped fetches a record and validates it into T. Returns (zero, nil,
// false, nil) when the record does not exist; callers should check the
// boolean before using the value.
func GetTyped[T any](ctx context.Context, client *connection.DatabaseClient, table string, recordID any) (T, bool, error) {
	var zero T
	rec, err := GetRecord(ctx, client, table, recordID)
	if err != nil {
		return zero, false, err
	}
	if rec == nil {
		return zero, false, nil
	}
	out, err := decodeRecord[T](rec)
	if err != nil {
		return zero, false, err
	}
	return out, true, nil
}

// QueryTyped runs a raw SurrealQL statement and decodes every row into T.
func QueryTyped[T any](ctx context.Context, client *connection.DatabaseClient, surql string, vars map[string]any) ([]T, error) {
	return ExecuteRawTyped[T](ctx, client, surql, vars)
}

// UpdateTyped replaces the record identified by recordID with data (PUT
// semantics) and returns the server's post-update record validated into T.
func UpdateTyped[T any](ctx context.Context, client *connection.DatabaseClient, table string, recordID any, data T) (T, error) {
	var zero T
	payload, err := toJSONMap(data)
	if err != nil {
		return zero, err
	}
	res, err := UpdateRecord(ctx, client, table, recordID, payload)
	if err != nil {
		return zero, err
	}
	return decodeRecord[T](res)
}

// UpsertTyped inserts or replaces the record identified by recordID and
// returns the resulting record validated into T.
func UpsertTyped[T any](ctx context.Context, client *connection.DatabaseClient, table string, recordID any, data T) (T, error) {
	var zero T
	payload, err := toJSONMap(data)
	if err != nil {
		return zero, err
	}
	res, err := UpsertRecord(ctx, client, table, recordID, payload)
	if err != nil {
		return zero, err
	}
	return decodeRecord[T](res)
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// toJSONMap serialises v through encoding/json and decodes it into a
// map[string]any so that downstream CRUD helpers (which speak map) see
// exactly the JSON payload the wire would carry.
func toJSONMap[T any](v T) (map[string]any, error) {
	// map[string]any short-circuit: preserves nil vs empty semantics.
	if m, ok := any(v).(map[string]any); ok {
		if m == nil {
			return nil, surqlerrors.New(surqlerrors.ErrValidation, "data cannot be nil")
		}
		return m, nil
	}
	buf, err := json.Marshal(v)
	if err != nil {
		return nil, surqlerrors.Wrap(surqlerrors.ErrSerialization, "encode payload", err)
	}
	if len(buf) == 0 || string(buf) == "null" {
		return nil, surqlerrors.New(surqlerrors.ErrValidation, "data cannot be nil")
	}
	out := map[string]any{}
	if err := json.Unmarshal(buf, &out); err != nil {
		return nil, surqlerrors.Wrap(surqlerrors.ErrSerialization, "decode payload to map", err)
	}
	return out, nil
}
