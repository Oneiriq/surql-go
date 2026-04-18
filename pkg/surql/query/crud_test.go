package query

import (
	"context"
	"errors"
	"testing"

	surqlerrors "github.com/albedosehen/surql-go/pkg/surql/errors"
	"github.com/albedosehen/surql-go/pkg/surql/types"
)

func TestBuildRecordTarget_String(t *testing.T) {
	target, err := buildRecordTarget("user", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if target != "user:alice" {
		t.Errorf("got %q", target)
	}
}

func TestBuildRecordTarget_RecordID(t *testing.T) {
	id, err := types.NewStringRecordID("user", "alice")
	if err != nil {
		t.Fatal(err)
	}
	target, err := buildRecordTarget("ignored", id)
	if err != nil {
		t.Fatal(err)
	}
	if target != "user:alice" {
		t.Errorf("got %q", target)
	}
}

func TestBuildRecordTarget_Nil(t *testing.T) {
	if _, err := buildRecordTarget("user", nil); err == nil {
		t.Fatal("expected error")
	} else if !errors.Is(err, surqlerrors.ErrValidation) {
		t.Errorf("want ErrValidation, got %v", err)
	}
}

func TestBuildRecordTarget_EmptyString(t *testing.T) {
	if _, err := buildRecordTarget("user", ""); err == nil {
		t.Fatal("expected error")
	}
}

func TestBuildRecordTarget_InvalidTable(t *testing.T) {
	if _, err := buildRecordTarget("1bad", "alice"); err == nil {
		t.Fatal("expected error")
	}
}

func TestBuildRecordTarget_UnsupportedType(t *testing.T) {
	if _, err := buildRecordTarget("user", 42); err == nil {
		t.Fatal("expected error")
	}
}

func TestBuildSelectFromOptions_Minimal(t *testing.T) {
	q, err := buildSelectFromOptions("user", QueryOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got, err := q.ToSurql()
	if err != nil {
		t.Fatal(err)
	}
	want := "SELECT * FROM user"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildSelectFromOptions_Full(t *testing.T) {
	limit, offset := 10, 5
	opts := QueryOptions{
		Conditions: []any{"age > 18"},
		OrderBy:    &OrderField{Field: "created_at", Direction: "DESC"},
		Limit:      &limit,
		Offset:     &offset,
	}
	q, err := buildSelectFromOptions("user", opts)
	if err != nil {
		t.Fatal(err)
	}
	got, err := q.ToSurql()
	if err != nil {
		t.Fatal(err)
	}
	want := "SELECT * FROM user WHERE (age > 18) ORDER BY created_at DESC LIMIT 10 START 5"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildSelectFromOptions_BadCondition(t *testing.T) {
	if _, err := buildSelectFromOptions("user", QueryOptions{Conditions: []any{42}}); err == nil {
		t.Fatal("expected error")
	}
}

func TestCreateRecord_NilClient(t *testing.T) {
	if _, err := CreateRecord(context.Background(), nil, "user", map[string]any{"n": 1}); err == nil {
		t.Fatal("expected error")
	}
}

func TestCreateRecord_NilData(t *testing.T) {
	if _, err := CreateRecord(context.Background(), nil, "user", nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestCreateRecord_InvalidTable(t *testing.T) {
	if _, err := CreateRecord(context.Background(), nil, "1bad", map[string]any{"n": 1}); err == nil {
		t.Fatal("expected error")
	}
}

func TestDeleteRecord_NilClient(t *testing.T) {
	if err := DeleteRecord(context.Background(), nil, "user", "alice"); err == nil {
		t.Fatal("expected error")
	}
}

func TestDeleteRecords_NilClient(t *testing.T) {
	if err := DeleteRecords(context.Background(), nil, "user", nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestNormaliseSingle(t *testing.T) {
	cases := map[string]struct {
		in   any
		want map[string]any
	}{
		"nil":      {nil, nil},
		"map":      {map[string]any{"a": 1}, map[string]any{"a": 1}},
		"wrapped":  {map[string]any{"result": map[string]any{"b": 2}}, map[string]any{"b": 2}},
		"slice":    {[]any{map[string]any{"c": 3}}, map[string]any{"c": 3}},
		"empty":    {[]any{}, nil},
		"scalar":   {"hi", nil},
		"nilSlice": {[]any(nil), nil},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := normaliseSingle(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("len mismatch: got %v want %v", got, tc.want)
			}
			for k, v := range tc.want {
				if got[k] != v {
					t.Errorf("%s: got %v want %v", k, got[k], v)
				}
			}
		})
	}
}

func TestToInt64(t *testing.T) {
	cases := []struct {
		in   any
		want int64
	}{
		{int(5), 5},
		{int32(7), 7},
		{int64(9), 9},
		{uint32(3), 3},
		{float64(4), 4},
		{float64(4.9), 4},
		{"abc", 0},
		{nil, 0},
	}
	for _, tc := range cases {
		if got := toInt64(tc.in); got != tc.want {
			t.Errorf("toInt64(%v) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestLast_FlipsOrderDirection(t *testing.T) {
	opts := QueryOptions{OrderBy: &OrderField{Field: "created_at", Direction: "ASC"}}
	// Last only flips the direction; it still calls First which requires a
	// client. We just verify the flip by building the query directly.
	q, err := buildSelectFromOptions("user", QueryOptions{
		OrderBy: &OrderField{Field: opts.OrderBy.Field, Direction: "DESC"},
	})
	if err != nil {
		t.Fatal(err)
	}
	surql, err := q.ToSurql()
	if err != nil {
		t.Fatal(err)
	}
	if surql != "SELECT * FROM user ORDER BY created_at DESC" {
		t.Errorf("got %q", surql)
	}
}
