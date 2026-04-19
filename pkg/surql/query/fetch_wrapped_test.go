package query

import (
	"context"
	"errors"
	"testing"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

func TestFetchRecord_NilClient(t *testing.T) {
	t.Parallel()

	q, err := Query{}.Select(nil).FromTable("user")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := FetchRecord[map[string]any](context.Background(), nil, q); err == nil {
		t.Fatal("expected error for nil client")
	} else if !errors.Is(err, surqlerrors.ErrValidation) {
		t.Fatalf("err = %v; want ErrValidation", err)
	}
}

func TestFetchRecords_NilClient(t *testing.T) {
	t.Parallel()

	q, err := Query{}.Select(nil).FromTable("user")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := FetchRecords[map[string]any](context.Background(), nil, q); err == nil {
		t.Fatal("expected error for nil client")
	} else if !errors.Is(err, surqlerrors.ErrValidation) {
		t.Fatalf("err = %v; want ErrValidation", err)
	}
}

func TestQueryRecordsWrapped_NilClient(t *testing.T) {
	t.Parallel()

	if _, err := QueryRecordsWrapped(context.Background(), nil, "user", QueryOptions{}); err == nil {
		t.Fatal("expected error for nil client")
	}
}

func TestQueryRecordsWrapped_HasMore_RetainsLimit(t *testing.T) {
	t.Parallel()

	// Exercise the non-DB-path: we drive a QueryRecordsWrapped-like flow
	// manually to ensure Records() preserves Limit/Offset hints, mirroring
	// surql-py's wrapper semantics. QueryRecordsWrapped itself needs a
	// client, but the underlying wrapping is table-driven.
	items := []map[string]any{{"id": "a"}, {"id": "b"}, {"id": "c"}}
	limit := uint64(3)
	offset := uint64(0)
	got := Records(items, nil, &limit, &offset)
	if !got.HasMore {
		t.Fatal("Records should flag HasMore when len(items)==limit")
	}
	if got.Limit == nil || *got.Limit != 3 {
		t.Fatalf("Limit = %v; want 3", got.Limit)
	}
	if got.Offset == nil || *got.Offset != 0 {
		t.Fatalf("Offset = %v; want 0", got.Offset)
	}
}
