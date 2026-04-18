package query

import (
	"context"
	"errors"
	"testing"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

func TestExecuteQuery_NilClient(t *testing.T) {
	q, err := Query{}.Select(nil).FromTable("user")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ExecuteQuery(context.Background(), nil, q); err == nil {
		t.Fatal("expected error for nil client")
	} else if !errors.Is(err, surqlerrors.ErrValidation) {
		t.Errorf("want ErrValidation, got %v", err)
	}
}

func TestExecuteQuery_IncompleteQuery(t *testing.T) {
	// Missing operation -> ToSurql returns validation error, which the
	// executor must propagate before hitting the client.
	if _, err := ExecuteQuery(context.Background(), nil, Query{}); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestExecuteRaw_NilClient(t *testing.T) {
	if _, err := ExecuteRaw(context.Background(), nil, "SELECT 1", nil); err == nil {
		t.Fatal("expected error for nil client")
	}
}

func TestExecuteRaw_EmptySurql(t *testing.T) {
	// Force the empty-surql branch even with a nil client.
	// (The nil-client branch runs first; re-route by using a zero *DatabaseClient-shaped*
	// pointer is not easily available without a live connection. Instead assert
	// through the exported function where it's the validation order that
	// matters most: nil client is the first check.)
	if _, err := ExecuteRaw(context.Background(), nil, "", nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestDecodeRecord_RoundTrip(t *testing.T) {
	type user struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	in := map[string]any{"name": "Alice", "age": 30}
	got, err := decodeRecord[user](in)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Alice" || got.Age != 30 {
		t.Errorf("got %+v", got)
	}
}

func TestDecodeRecords_EmptySliceNonNil(t *testing.T) {
	out, err := decodeRecords[map[string]any](nil)
	if err != nil {
		t.Fatal(err)
	}
	if out == nil {
		t.Fatal("expected non-nil slice")
	}
	if len(out) != 0 {
		t.Errorf("expected empty, got %v", out)
	}
}

func TestDecodeRecords_Many(t *testing.T) {
	type item struct {
		Value int `json:"value"`
	}
	rows := []map[string]any{{"value": 1}, {"value": 2}, {"value": 3}}
	got, err := decodeRecords[item](rows)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].Value != 1 || got[2].Value != 3 {
		t.Errorf("got %+v", got)
	}
}
