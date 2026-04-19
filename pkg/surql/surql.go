// Package surql re-exports the most commonly used builders, CRUD
// helpers, and result-extraction utilities from the sub-packages so
// consumers can import a single path.
//
// The extraction helpers (ExtractOne, ExtractMany, ExtractScalar,
// HasResult) normalise the raw response envelope emitted by the
// SurrealDB client:
//
//   - db.query returns [{"result": [...], "status": "OK"}, ...]
//   - db.select returns [{...}, {...}]
//
// Both shapes collapse to a flat []map[string]any record list.
package surql

import (
	"encoding/json"
	"fmt"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
	"github.com/Oneiriq/surql-go/pkg/surql/query"
)

// Version is the current surql-go release.
const Version = "0.2.1"

// ExtractOne returns the first record from a raw response or
// (nil, nil) when the envelope is empty. Mirrors the Python
// `extract_one` helper.
func ExtractOne(raw any) (map[string]any, error) {
	return query.ExtractOne(raw), nil
}

// ExtractMany returns every record from a raw response as a flat slice
// of maps. Returns (nil, nil) when the envelope is empty. Mirrors the
// Python `extract_many` helper.
func ExtractMany(raw any) ([]map[string]any, error) {
	return query.ExtractResult(raw), nil
}

// HasResult reports whether the raw response carries at least one
// record. Mirrors the Python `has_result` helper.
func HasResult(raw any) bool {
	return query.HasResults(raw)
}

// ExtractScalar pulls a single scalar value identified by key from the
// first row of a raw response and decodes it into T. Returns the zero
// value of T and ErrValidation when the envelope is empty or the key
// is missing. Mirrors the Python `extract_scalar` helper, generalised
// via Go generics.
//
// Numeric coercion follows encoding/json's loose rules: JSON numbers
// decode into any numeric T via a round-trip.
func ExtractScalar[T any](raw any, key string) (T, error) {
	var zero T
	row := query.ExtractOne(raw)
	if row == nil {
		return zero, surqlerrors.Newf(surqlerrors.ErrValidation,
			"ExtractScalar: response contains no rows for key %q", key)
	}
	v, ok := row[key]
	if !ok {
		return zero, surqlerrors.Newf(surqlerrors.ErrValidation,
			"ExtractScalar: key %q not present in row", key)
	}
	// Fast path: the underlying value is already a T.
	if typed, ok := v.(T); ok {
		return typed, nil
	}
	// Slow path: round-trip through encoding/json so float64 coerces
	// into int / int64 / uint / etc without caller-side casts.
	buf, err := json.Marshal(v)
	if err != nil {
		return zero, surqlerrors.Wrap(surqlerrors.ErrSerialization,
			fmt.Sprintf("ExtractScalar: marshal value for key %q", key), err)
	}
	var out T
	if err := json.Unmarshal(buf, &out); err != nil {
		return zero, surqlerrors.Wrap(surqlerrors.ErrSerialization,
			fmt.Sprintf("ExtractScalar: decode value for key %q into target type", key), err)
	}
	return out, nil
}
