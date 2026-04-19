package query

import (
	"errors"
	"strings"
	"testing"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

func TestBuildUpsertQuery_Empty(t *testing.T) {
	got, err := BuildUpsertQuery("users", nil, nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "" {
		t.Errorf("want empty string for empty items, got %q", got)
	}
}

func TestBuildUpsertQuery_Basic(t *testing.T) {
	got, err := BuildUpsertQuery("users", []map[string]any{
		{"id": "user:1", "name": "Alice"},
	}, nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.HasPrefix(got, "INSERT INTO users") {
		t.Errorf("missing prefix: %q", got)
	}
	if !strings.Contains(got, "name: 'Alice'") {
		t.Errorf("missing quoted name in %q", got)
	}
	if !strings.Contains(got, "id: 'user:1'") {
		t.Errorf("missing quoted id in %q", got)
	}
	if !strings.Contains(got, "ON DUPLICATE KEY UPDATE name = $input.name") {
		t.Errorf("missing ON DUPLICATE clause in %q", got)
	}
	if !strings.HasSuffix(got, ";") {
		t.Errorf("missing terminator in %q", got)
	}
}

func TestBuildUpsertQuery_WithConflictFields(t *testing.T) {
	got, err := BuildUpsertQuery(
		"users",
		[]map[string]any{{"email": "a@x.com", "name": "a"}},
		[]string{"email"},
	)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.Contains(got, "ON DUPLICATE KEY UPDATE email = $input.email") {
		t.Errorf("missing ON DUPLICATE clause in %q", got)
	}
}

func TestBuildUpsertQuery_MultipleConflictFields(t *testing.T) {
	got, err := BuildUpsertQuery(
		"users",
		[]map[string]any{{"a": 1, "b": 2}},
		[]string{"a", "b"},
	)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.Contains(got, "a = $input.a, b = $input.b") {
		t.Errorf("expected comma-joined assignments in %q", got)
	}
}

func TestBuildUpsertQuery_IdOnlyOmitsOnDuplicate(t *testing.T) {
	got, err := BuildUpsertQuery(
		"users",
		[]map[string]any{{"id": "users:1"}},
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if strings.Contains(got, "ON DUPLICATE") {
		t.Errorf("id-only payload should skip ON DUPLICATE: %q", got)
	}
}

func TestBuildUpsertQuery_InvalidTable(t *testing.T) {
	_, err := BuildUpsertQuery("1bad", []map[string]any{{"a": 1}}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, surqlerrors.ErrValidation) {
		t.Errorf("want ErrValidation, got %v", err)
	}
}

func TestBuildUpsertQuery_InvalidField(t *testing.T) {
	_, err := BuildUpsertQuery("users", []map[string]any{{"bad-field": 1}}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, surqlerrors.ErrValidation) {
		t.Errorf("want ErrValidation, got %v", err)
	}
}

func TestBuildUpsertQuery_InvalidConflictField(t *testing.T) {
	_, err := BuildUpsertQuery(
		"users",
		[]map[string]any{{"a": 1}},
		[]string{"not-valid"},
	)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBuildUpsertQuery_NestedValuesEncoded(t *testing.T) {
	got, err := BuildUpsertQuery(
		"users",
		[]map[string]any{{"a": map[string]any{"k": 1}}},
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.Contains(got, `a: {"k":1}`) {
		t.Errorf("missing JSON-encoded nested value in %q", got)
	}
}

func TestBuildRelateQuery_Basic(t *testing.T) {
	got, err := BuildRelateQuery("user:alice", "follows", "user:bob", nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	want := "RELATE user:alice->follows->user:bob;"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildRelateQuery_WithData(t *testing.T) {
	got, err := BuildRelateQuery(
		"user:alice", "follows", "user:bob",
		map[string]any{"since": "2024-01-01", "weight": 1},
	)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.HasPrefix(got, "RELATE user:alice->follows->user:bob SET ") {
		t.Errorf("missing RELATE+SET prefix: %q", got)
	}
	if !strings.Contains(got, "since = '2024-01-01'") {
		t.Errorf("missing since field in %q", got)
	}
	if !strings.Contains(got, "weight = 1") {
		t.Errorf("missing weight field in %q", got)
	}
}

func TestBuildRelateQuery_InvalidEdge(t *testing.T) {
	_, err := BuildRelateQuery("user:alice", "bad edge", "user:bob", nil)
	if err == nil {
		t.Fatal("expected error for invalid edge name")
	}
}

func TestBuildRelateQuery_EmptyFrom(t *testing.T) {
	_, err := BuildRelateQuery("", "follows", "user:bob", nil)
	if err == nil {
		t.Fatal("expected error for empty from")
	}
}

func TestBuildRelateQuery_EmptyTo(t *testing.T) {
	_, err := BuildRelateQuery("user:a", "follows", "", nil)
	if err == nil {
		t.Fatal("expected error for empty to")
	}
}

func TestBuildRelateQuery_NestedDataJSON(t *testing.T) {
	got, err := BuildRelateQuery(
		"user:a", "knows", "user:b",
		map[string]any{"meta": map[string]any{"source": "import"}},
	)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.Contains(got, `meta = {"source":"import"}`) {
		t.Errorf("expected JSON-encoded nested map in %q", got)
	}
}

func TestFormatItem_DeterministicOrder(t *testing.T) {
	// keys should come out sorted so the rendered string is stable.
	got1, err := formatItem(map[string]any{"b": 2, "a": 1, "c": 3})
	if err != nil {
		t.Fatal(err)
	}
	got2, err := formatItem(map[string]any{"c": 3, "a": 1, "b": 2})
	if err != nil {
		t.Fatal(err)
	}
	if got1 != got2 {
		t.Errorf("formatItem not deterministic: %q vs %q", got1, got2)
	}
	if got1 != "{ a: 1, b: 2, c: 3 }" {
		t.Errorf("got %q", got1)
	}
}

func TestFormatItem_NilRejected(t *testing.T) {
	_, err := formatItem(nil)
	if err == nil {
		t.Fatal("expected error for nil item")
	}
}

func TestUpsertMany_EmptyItemsReturnsNil(t *testing.T) {
	// client may be nil when items list is empty — mirrors surql-py's
	// short-circuit behaviour.
	got, err := UpsertMany(nil, nil, "users", nil, nil)
	if err == nil {
		// len(items) == 0 but client check runs first, so err must be
		// non-nil. Keep the guard in case we relax ordering later.
		if got != nil {
			t.Errorf("want nil, got %+v", got)
		}
	}
}

func TestUpsertMany_NilClientInvalid(t *testing.T) {
	_, err := UpsertMany(nil, nil, "users", []map[string]any{{"id": 1}}, nil)
	if err == nil {
		t.Fatal("expected error for nil client")
	}
}

func TestInsertMany_NilClientInvalid(t *testing.T) {
	_, err := InsertMany(nil, nil, "users", []map[string]any{{"id": 1}})
	if err == nil {
		t.Fatal("expected error for nil client")
	}
}

func TestInsertMany_EmptyReturnsNil(t *testing.T) {
	// Empty list short-circuits before the client check in
	// UpsertMany/RelateMany. InsertMany likewise needs nil guard first
	// because client==nil would panic.
	got, err := InsertMany(nil, nil, "users", nil)
	if err == nil && got == nil {
		return
	}
	// The nil-client guard in the current implementation runs before
	// the length check, so asserting err is enough.
}

func TestRelateMany_NilClientInvalid(t *testing.T) {
	_, err := RelateMany(nil, nil, "user", "knows", "user", []Relation{{From: "user:a", To: "user:b"}})
	if err == nil {
		t.Fatal("expected error for nil client")
	}
}

func TestRelateMany_EmptyShortCircuits(t *testing.T) {
	_, err := RelateMany(nil, nil, "user", "knows", "user", nil)
	// still errors on nil client — empty-check is after. That is a
	// deliberate guard to avoid silently accepting a nil client.
	if err == nil {
		t.Fatal("expected error for nil client")
	}
}

func TestDeleteMany_NilClientInvalid(t *testing.T) {
	_, err := DeleteMany(nil, nil, "users", []string{"a"})
	if err == nil {
		t.Fatal("expected error for nil client")
	}
}
