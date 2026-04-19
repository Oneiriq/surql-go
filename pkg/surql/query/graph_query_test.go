package query

import (
	"errors"
	"strings"
	"testing"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

func TestGraphQuery_ToSurql_Basic(t *testing.T) {
	g := NewGraphQuery("user:alice").Out("follows", nil)
	got, vars, err := g.ToSurql()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "SELECT * FROM user:alice->follows" {
		t.Errorf("got %q", got)
	}
	if vars == nil || len(vars) != 0 {
		t.Errorf("expected empty vars map, got %+v", vars)
	}
}

func TestGraphQuery_ToSurql_OutWithDepth(t *testing.T) {
	d := 3
	got, _, err := NewGraphQuery("user:alice").Out("follows", &d).ToSurql()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.Contains(got, "->follows3") {
		t.Errorf("missing depth: %q", got)
	}
}

func TestGraphQuery_ToSurql_ChainInOut(t *testing.T) {
	got, _, err := NewGraphQuery("user:alice").
		Out("follows", nil).
		In("likes", nil).
		ToSurql()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	want := "SELECT * FROM user:alice->follows<-likes"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestGraphQuery_ToSurql_BothAndTarget(t *testing.T) {
	got, _, err := NewGraphQuery("user:alice").
		Both("knows", nil).
		To("user").
		ToSurql()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	want := "SELECT * FROM user:alice<->knows->user"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestGraphQuery_ToSurql_WhereMultiple(t *testing.T) {
	got, _, err := NewGraphQuery("user:alice").
		Out("follows", nil).
		Where("age > 18").
		Where("id != user:alice").
		ToSurql()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.Contains(got, "WHERE (age > 18) AND (id != user:alice)") {
		t.Errorf("missing AND-joined WHERE in %q", got)
	}
}

func TestGraphQuery_ToSurql_WhereSkipsEmpty(t *testing.T) {
	got, _, err := NewGraphQuery("user:alice").
		Out("follows", nil).
		Where("").
		Where("age > 18").
		ToSurql()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if strings.Contains(got, "()") {
		t.Errorf("empty condition leaked: %q", got)
	}
}

func TestGraphQuery_ToSurql_SelectFields(t *testing.T) {
	got, _, err := NewGraphQuery("user:alice").
		Out("follows", nil).
		Select("id", "name").
		ToSurql()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.HasPrefix(got, "SELECT id, name FROM") {
		t.Errorf("missing field list: %q", got)
	}
}

func TestGraphQuery_ToSurql_Limit(t *testing.T) {
	got, _, err := NewGraphQuery("user:alice").
		Out("follows", nil).
		Limit(10).
		ToSurql()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.HasSuffix(got, "LIMIT 10") {
		t.Errorf("missing LIMIT: %q", got)
	}
}

func TestGraphQuery_ToSurql_LimitRejectsNegative(t *testing.T) {
	_, _, err := NewGraphQuery("user:alice").
		Out("follows", nil).
		Limit(-1).
		ToSurql()
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, surqlerrors.ErrValidation) {
		t.Errorf("want ErrValidation, got %v", err)
	}
}

func TestGraphQuery_ToSurql_Fetch(t *testing.T) {
	got, _, err := NewGraphQuery("user:alice").
		Out("follows", nil).
		Fetch("author", "tags").
		ToSurql()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.Contains(got, "FETCH author, tags") {
		t.Errorf("missing FETCH clause in %q", got)
	}
}

func TestGraphQuery_ToSurql_RequiresPath(t *testing.T) {
	_, _, err := NewGraphQuery("user:alice").ToSurql()
	if err == nil {
		t.Fatal("expected error: no path")
	}
	if !errors.Is(err, surqlerrors.ErrValidation) {
		t.Errorf("want ErrValidation, got %v", err)
	}
}

func TestGraphQuery_ToSurql_RequiresStart(t *testing.T) {
	g := NewGraphQuery("").Out("follows", nil)
	_, _, err := g.ToSurql()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGraphQuery_ToSurql_NilReceiverRejected(t *testing.T) {
	var g *GraphQuery
	_, _, err := g.ToSurql()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGraphQuery_Clone_Independent(t *testing.T) {
	orig := NewGraphQuery("user:alice").Out("follows", nil).Where("a = 1").Limit(5)
	clone := orig.Clone()

	// mutate the original
	orig.In("likes", nil)
	orig.Where("b = 2")
	orig.Limit(99)

	gotOrig, _, err := orig.ToSurql()
	if err != nil {
		t.Fatal(err)
	}
	gotClone, _, err := clone.ToSurql()
	if err != nil {
		t.Fatal(err)
	}
	if gotOrig == gotClone {
		t.Error("expected divergent queries after mutation")
	}
	if !strings.Contains(gotClone, "LIMIT 5") {
		t.Errorf("clone lost its limit: %q", gotClone)
	}
	if strings.Contains(gotClone, "LIMIT 99") {
		t.Errorf("clone picked up original's new limit: %q", gotClone)
	}
}

func TestGraphQuery_Clone_NilIsNil(t *testing.T) {
	var g *GraphQuery
	if got := g.Clone(); got != nil {
		t.Errorf("want nil, got %+v", got)
	}
}

func TestGraphQuery_Count_NilClientRejected(t *testing.T) {
	_, err := NewGraphQuery("user:alice").Out("follows", nil).Count(nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGraphQuery_Count_RequiresPath(t *testing.T) {
	_, err := NewGraphQuery("user:alice").Count(nil, nil)
	if err == nil {
		t.Fatal("expected error: Count without path must fail (client nil secondary)")
	}
}

func TestGraphQuery_Execute_NilClientRejected(t *testing.T) {
	_, err := NewGraphQuery("user:alice").Out("follows", nil).Execute(nil, nil)
	if err == nil {
		t.Fatal("expected error for nil client")
	}
}

func TestGraphQuery_Exists_PropagatesCountError(t *testing.T) {
	_, err := NewGraphQuery("user:alice").Out("follows", nil).Exists(nil, nil)
	if err == nil {
		t.Fatal("expected error propagated from Count")
	}
}

func TestFormatEdgeStep(t *testing.T) {
	cases := []struct {
		name  string
		arrow string
		edge  string
		depth *int
		want  string
	}{
		{"no depth", "->", "follows", nil, "->follows"},
		{"with depth", "->", "follows", intPtr(3), "->follows3"},
		{"in arrow", "<-", "likes", nil, "<-likes"},
		{"both arrow", "<->", "knows", intPtr(2), "<->knows2"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := formatEdgeStep(tc.arrow, tc.edge, tc.depth)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func intPtr(n int) *int { return &n }
