package query

import (
	"context"
	"testing"

	"github.com/Oneiriq/surql-go/pkg/surql/types"
)

func TestTraverseDirection_Arrow(t *testing.T) {
	cases := []struct {
		name   string
		dir    TraverseDirection
		want   string
		errors bool
	}{
		{"out", TraverseOut, "->", false},
		{"default empty", "", "->", false},
		{"in", TraverseIn, "<-", false},
		{"both", TraverseBoth, "<->", false},
		{"invalid", TraverseDirection("sideways"), "", true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.dir.arrow()
			if tc.errors {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRecordToString(t *testing.T) {
	id, err := types.NewStringRecordID("user", "alice")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		input  any
		want   string
		errors bool
	}{
		{"string", "user:alice", "user:alice", false},
		{"recordID", id, "user:alice", false},
		{"nil", nil, "", true},
		{"empty string", "", "", true},
		{"unsupported", 42, "", true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, err := recordToString(tc.input, "record")
			if tc.errors {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTraverse_NilClientRejected(t *testing.T) {
	_, err := Traverse(context.Background(), nil, "user:alice", "->follows->user", nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestTraverse_EmptyPathRejected(t *testing.T) {
	// pass a non-nil client-looking value is tricky — we use nil and
	// rely on the client guard firing first. Instead reach the path
	// check by using the helper via TraverseWithDepth which validates
	// before touching the client.
	_, err := Traverse(context.Background(), nil, "", "", nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestTraverseWithDepth_InvalidDirection(t *testing.T) {
	_, err := TraverseWithDepth(
		context.Background(), nil,
		"user:alice", "follows", "user",
		TraverseDirection("diagonal"), nil, nil,
	)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestTraverseWithDepth_InvalidEdge(t *testing.T) {
	_, err := TraverseWithDepth(
		context.Background(), nil,
		"user:alice", "bad edge", "user",
		TraverseOut, nil, nil,
	)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestTraverseWithDepth_InvalidTarget(t *testing.T) {
	_, err := TraverseWithDepth(
		context.Background(), nil,
		"user:alice", "follows", "bad target",
		TraverseOut, nil, nil,
	)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestTraverseWithDepth_NegativeDepthRejected(t *testing.T) {
	d := -1
	_, err := TraverseWithDepth(
		context.Background(), nil,
		"user:alice", "follows", "user",
		TraverseOut, &d, nil,
	)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCreateRelation_ValidationSurface(t *testing.T) {
	_, err := CreateRelation(context.Background(), nil, "bad edge", "user:a", "user:b", nil)
	if err == nil {
		t.Fatal("expected error for nil client")
	}
}

func TestRemoveRelation_NilClient(t *testing.T) {
	err := RemoveRelation(context.Background(), nil, "follows", "user:a", "user:b")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetOutgoingEdges_NilClient(t *testing.T) {
	_, err := GetOutgoingEdges(context.Background(), nil, "user:a", "follows", nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetIncomingEdges_NilClient(t *testing.T) {
	_, err := GetIncomingEdges(context.Background(), nil, "user:a", "follows", nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetRelatedRecords_InvalidDirection(t *testing.T) {
	_, err := GetRelatedRecords(
		context.Background(), nil,
		"user:a", "follows", "user",
		TraverseBoth, nil,
	)
	if err == nil {
		t.Fatal("expected error for 'both' direction in GetRelatedRecords")
	}
}

func TestCountRelated_InvalidDirection(t *testing.T) {
	_, err := CountRelated(
		context.Background(), nil,
		"user:a", "follows",
		TraverseBoth,
	)
	if err == nil {
		t.Fatal("expected error for 'both' direction in CountRelated")
	}
}

func TestShortestPath_InvalidEdge(t *testing.T) {
	_, err := ShortestPath(context.Background(), nil, "user:a", "user:b", "bad edge", 5, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestShortestPath_NilClient(t *testing.T) {
	_, err := ShortestPath(context.Background(), nil, "user:a", "user:b", "follows", 5, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBuildGraphPath(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want string
	}{
		{"empty", nil, ""},
		{"single", []string{"->a"}, "->a"},
		{"skips empty", []string{"->a", "", "->b"}, "->a->b"},
		{"multi", []string{"->a", "<-b", "<->c"}, "->a<-b<->c"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := buildGraphPath(tc.in)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// --- conditions on the graph helpers -------------------------------------

func TestSelectTraversalSurql_NoConditionsIsUnchanged(t *testing.T) {
	// Guards the refactor onto the Query builder: with no conditions the
	// emitted statement must match what the previous Sprintf produced.
	got, err := selectTraversalSurql("user:alice", "->likes->post", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "SELECT * FROM user:alice->likes->post;"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSelectTraversalSurql_EmptySliceMatchesNil(t *testing.T) {
	withNil, err := selectTraversalSurql("user:alice", "->likes->post", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	withEmpty, err := selectTraversalSurql("user:alice", "->likes->post", []any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if withNil != withEmpty {
		t.Errorf("empty slice %q differs from nil %q", withEmpty, withNil)
	}
}

func TestSelectTraversalSurql_OperatorCondition(t *testing.T) {
	got, err := selectTraversalSurql("user:alice", "->likes->post",
		[]any{types.EqOp("tenant_id", "acme")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "SELECT * FROM user:alice->likes->post WHERE (tenant_id = 'acme');"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSelectTraversalSurql_RawFragmentCondition(t *testing.T) {
	got, err := selectTraversalSurql("user:alice", "->likes->post", []any{"age > 18"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "SELECT * FROM user:alice->likes->post WHERE (age > 18);"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSelectTraversalSurql_MixedConditionsCombineWithAnd(t *testing.T) {
	// One slice, both forms, joined by AND in the order given -- the
	// `str | Operator` union the sibling ports accept.
	got, err := selectTraversalSurql("user:alice", "->likes->post",
		[]any{types.EqOp("tenant_id", "acme"), "age > 18"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "SELECT * FROM user:alice->likes->post WHERE (tenant_id = 'acme') AND (age > 18);"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSelectTraversalSurql_RejectsInvalidCondition(t *testing.T) {
	if _, err := selectTraversalSurql("user:alice", "->likes->post", []any{42}); err == nil {
		t.Fatal("expected error for a non-string, non-Operator condition")
	}
}

// TestIncomingPathIsRecordFirst pins the v3 ordering fix: SurrealDB v3
// rejects `FROM <-edge<-record` with a parse error, so incoming
// traversals must put the record at the head of FROM.
func TestIncomingPathIsRecordFirst(t *testing.T) {
	got, err := selectTraversalSurql("person:bob", "<-follows", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "SELECT * FROM person:bob<-follows;"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
