package surql

import (
	"errors"
	"testing"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

func TestExtractOne_Nested(t *testing.T) {
	raw := []any{map[string]any{
		"result": []any{map[string]any{"id": "u:1", "name": "Alice"}},
	}}
	row, err := ExtractOne(raw)
	if err != nil {
		t.Fatal(err)
	}
	if row == nil || row["name"] != "Alice" {
		t.Errorf("got %+v", row)
	}
}

func TestExtractOne_Flat(t *testing.T) {
	raw := []any{map[string]any{"id": "u:1", "name": "Bob"}}
	row, err := ExtractOne(raw)
	if err != nil {
		t.Fatal(err)
	}
	if row == nil || row["name"] != "Bob" {
		t.Errorf("got %+v", row)
	}
}

func TestExtractOne_Empty(t *testing.T) {
	row, err := ExtractOne([]any{})
	if err != nil {
		t.Fatal(err)
	}
	if row != nil {
		t.Errorf("got %+v", row)
	}
}

func TestExtractOne_Nil(t *testing.T) {
	row, err := ExtractOne(nil)
	if err != nil {
		t.Fatal(err)
	}
	if row != nil {
		t.Errorf("got %+v", row)
	}
}

func TestExtractMany_Nested(t *testing.T) {
	raw := []any{map[string]any{
		"result": []any{
			map[string]any{"id": "u:1"},
			map[string]any{"id": "u:2"},
		},
	}}
	rows, err := ExtractMany(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Errorf("got %d rows", len(rows))
	}
}

func TestExtractMany_Empty(t *testing.T) {
	rows, err := ExtractMany(nil)
	if err != nil {
		t.Fatal(err)
	}
	if rows != nil {
		t.Errorf("got %+v", rows)
	}
}

func TestHasResult(t *testing.T) {
	tests := []struct {
		name string
		raw  any
		want bool
	}{
		{"nested populated", []any{map[string]any{"result": []any{map[string]any{"id": "u:1"}}}}, true},
		{"nested empty", []any{map[string]any{"result": []any{}}}, false},
		{"flat populated", []any{map[string]any{"id": "u:1"}}, true},
		{"nil", nil, false},
		{"empty slice", []any{}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := HasResult(tc.raw); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestExtractScalar_Int(t *testing.T) {
	// encoding/json decodes integer literals as float64; ExtractScalar
	// round-trips through json to coerce into the caller's T.
	raw := []any{map[string]any{"result": []any{map[string]any{"count": 42}}}}
	got, err := ExtractScalar[int](raw, "count")
	if err != nil {
		t.Fatal(err)
	}
	if got != 42 {
		t.Errorf("got %d", got)
	}
}

func TestExtractScalar_Int64FromFloat(t *testing.T) {
	raw := []any{map[string]any{"result": []any{map[string]any{"total": float64(9001)}}}}
	got, err := ExtractScalar[int64](raw, "total")
	if err != nil {
		t.Fatal(err)
	}
	if got != 9001 {
		t.Errorf("got %d", got)
	}
}

func TestExtractScalar_String(t *testing.T) {
	raw := []any{map[string]any{"result": []any{map[string]any{"name": "Alice"}}}}
	got, err := ExtractScalar[string](raw, "name")
	if err != nil {
		t.Fatal(err)
	}
	if got != "Alice" {
		t.Errorf("got %q", got)
	}
}

func TestExtractScalar_Bool(t *testing.T) {
	raw := []any{map[string]any{"active": true}}
	got, err := ExtractScalar[bool](raw, "active")
	if err != nil {
		t.Fatal(err)
	}
	if !got {
		t.Error("expected true")
	}
}

func TestExtractScalar_Float64(t *testing.T) {
	raw := []any{map[string]any{"result": []any{map[string]any{"avg": 25.5}}}}
	got, err := ExtractScalar[float64](raw, "avg")
	if err != nil {
		t.Fatal(err)
	}
	if got != 25.5 {
		t.Errorf("got %v", got)
	}
}

func TestExtractScalar_EmptyResponse(t *testing.T) {
	_, err := ExtractScalar[int]([]any{}, "count")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, surqlerrors.ErrValidation) {
		t.Errorf("want ErrValidation, got %v", err)
	}
}

func TestExtractScalar_MissingKey(t *testing.T) {
	raw := []any{map[string]any{"id": "u:1"}}
	_, err := ExtractScalar[int](raw, "count")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, surqlerrors.ErrValidation) {
		t.Errorf("want ErrValidation, got %v", err)
	}
}

func TestExtractScalar_FastPath(t *testing.T) {
	// When the stored value already matches T, the fast path returns it
	// without a json round-trip.
	raw := []any{map[string]any{"name": "Alice"}}
	got, err := ExtractScalar[string](raw, "name")
	if err != nil {
		t.Fatal(err)
	}
	if got != "Alice" {
		t.Errorf("got %q", got)
	}
}

func TestExtractScalar_IncompatibleType(t *testing.T) {
	// A string value decoding into int should fail via json.Unmarshal.
	raw := []any{map[string]any{"count": "not-a-number"}}
	_, err := ExtractScalar[int](raw, "count")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, surqlerrors.ErrSerialization) {
		t.Errorf("want ErrSerialization, got %v", err)
	}
}

func TestVersion(t *testing.T) {
	if Version == "" {
		t.Error("version must not be empty")
	}
}
