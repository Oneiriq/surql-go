package query

import (
	"encoding/json"
	"testing"
)

func TestRecordResult_Unwrap(t *testing.T) {
	v := 42
	r := Record(&v, true)
	if got := r.Unwrap(); got != 42 {
		t.Errorf("got %v", got)
	}
}

func TestRecordResult_UnwrapOr(t *testing.T) {
	var r RecordResult[int]
	if got := r.UnwrapOr(5); got != 5 {
		t.Errorf("got %v", got)
	}
	v := 1
	if got := Record(&v, true).UnwrapOr(5); got != 1 {
		t.Errorf("got %v", got)
	}
}

func TestRecordResult_TryUnwrapNil(t *testing.T) {
	var r RecordResult[int]
	if _, err := r.TryUnwrap(); err == nil {
		t.Fatal("expected error")
	}
}

func TestListResult_Helpers(t *testing.T) {
	three := uint64(3)
	ten := uint64(10)
	zero := uint64(0)
	lr := Records([]int{1, 2, 3}, &three, &ten, &zero)
	if lr.Len() != 3 || lr.IsEmpty() {
		t.Errorf("len=%d empty=%v", lr.Len(), lr.IsEmpty())
	}
	if *lr.First() != 1 || *lr.Last() != 3 {
		t.Errorf("first/last")
	}
}

func TestRecords_HasMoreFromTotal(t *testing.T) {
	twenty := uint64(20)
	three := uint64(3)
	zero := uint64(0)
	lr := Records([]int{1, 2, 3}, &twenty, &three, &zero)
	if !lr.HasMore {
		t.Error("expected has_more")
	}
	threeAgain := uint64(3)
	done := Records([]int{1, 2, 3}, &threeAgain, &three, &zero)
	if done.HasMore {
		t.Error("expected no has_more")
	}
}

func TestRecords_HasMoreFromLimitOnly(t *testing.T) {
	three := uint64(3)
	lr := Records([]int{1, 2, 3}, nil, &three, nil)
	if !lr.HasMore {
		t.Error("expected has_more")
	}
	lr2 := Records([]int{1, 2}, nil, &three, nil)
	if lr2.HasMore {
		t.Error("expected no has_more")
	}
}

func TestPaginated_ComputesPages(t *testing.T) {
	p := Paginated([]int{1, 2, 3}, 1, 10, 100)
	if p.PageInfo.TotalPages != 10 {
		t.Errorf("total_pages: %d", p.PageInfo.TotalPages)
	}
	if p.PageInfo.HasPrevious || !p.PageInfo.HasNext {
		t.Errorf("has_prev=%v has_next=%v", p.PageInfo.HasPrevious, p.PageInfo.HasNext)
	}
}

func TestPaginated_LastPageRounding(t *testing.T) {
	p := Paginated[int](nil, 10, 10, 95)
	if p.PageInfo.TotalPages != 10 {
		t.Errorf("got %d", p.PageInfo.TotalPages)
	}
	if p.PageInfo.HasNext || !p.PageInfo.HasPrevious {
		t.Errorf("has_prev=%v has_next=%v", p.PageInfo.HasPrevious, p.PageInfo.HasNext)
	}
}

func TestPaginated_ZeroPageSize(t *testing.T) {
	p := Paginated[int](nil, 1, 0, 100)
	if p.PageInfo.TotalPages != 0 {
		t.Errorf("got %d", p.PageInfo.TotalPages)
	}
}

func TestExtract_Flat(t *testing.T) {
	raw := []any{map[string]any{"id": "user:123", "name": "Alice"}}
	out := ExtractResult(raw)
	if len(out) != 1 || out[0]["name"] != "Alice" {
		t.Errorf("got %+v", out)
	}
}

func TestExtract_Nested(t *testing.T) {
	raw := []any{
		map[string]any{"result": []any{map[string]any{"id": "user:123", "name": "Alice"}}},
	}
	out := ExtractResult(raw)
	if len(out) != 1 || out[0]["name"] != "Alice" {
		t.Errorf("got %+v", out)
	}
}

func TestExtract_NestedMultiple(t *testing.T) {
	raw := []any{
		map[string]any{"result": []any{
			map[string]any{"id": "user:1"},
			map[string]any{"id": "user:2"},
		}},
		map[string]any{"result": []any{map[string]any{"id": "user:3"}}},
	}
	out := ExtractResult(raw)
	if len(out) != 3 {
		t.Errorf("got %d records", len(out))
	}
}

func TestExtract_Empty(t *testing.T) {
	if out := ExtractResult([]any{}); len(out) != 0 {
		t.Errorf("got %+v", out)
	}
}

func TestExtract_Nil(t *testing.T) {
	if out := ExtractResult(nil); out != nil && len(out) != 0 {
		t.Errorf("got %+v", out)
	}
}

func TestExtract_ObjectWithResult(t *testing.T) {
	raw := map[string]any{"result": []any{map[string]any{"id": "u:1"}}}
	if out := ExtractResult(raw); len(out) != 1 {
		t.Errorf("got %d", len(out))
	}
}

func TestExtractOne(t *testing.T) {
	raw := []any{map[string]any{"result": []any{map[string]any{"id": "user:123", "name": "Alice"}}}}
	one := ExtractOne(raw)
	if one == nil || one["name"] != "Alice" {
		t.Errorf("got %+v", one)
	}
	if ExtractOne([]any{}) != nil {
		t.Error("expected nil")
	}
}

func TestExtractScalar(t *testing.T) {
	raw := []any{map[string]any{"result": []any{map[string]any{"count": 42}}}}
	if got := ExtractScalar(raw, "count", 0); got != 42 {
		t.Errorf("got %v", got)
	}
	if got := ExtractScalar([]any{}, "count", 0); got != 0 {
		t.Errorf("got %v", got)
	}
	rawMissing := []any{map[string]any{"id": "u:1"}}
	if got := ExtractScalar(rawMissing, "total", 0); got != 0 {
		t.Errorf("got %v", got)
	}
}

func TestHasResults(t *testing.T) {
	if !HasResults([]any{map[string]any{"result": []any{map[string]any{"id": "u:1"}}}}) {
		t.Error("expected true")
	}
	if HasResults([]any{}) {
		t.Error("expected false")
	}
	if !HasResults([]any{map[string]any{"id": "u:1"}}) {
		t.Error("expected true on flat")
	}
	if HasResults([]any{map[string]any{"result": []any{}}}) {
		t.Error("expected false on empty nested")
	}
}

func TestSuccess(t *testing.T) {
	r := Success([]int{1, 2, 3}, "12ms")
	if r.Status != "OK" || r.Time != "12ms" || len(r.Data) != 3 {
		t.Errorf("got %+v", r)
	}
}

func TestCountAndAggregate(t *testing.T) {
	if NewCountResult(42).Count != 42 {
		t.Error("count mismatch")
	}
	a, err := Aggregate(25.5, "AVG", "age")
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	var got float64
	if err := json.Unmarshal(a.Value, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got != 25.5 {
		t.Errorf("got %v", got)
	}
}
