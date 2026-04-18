package query

import (
	"errors"
	"strings"
	"testing"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

func TestIndexHint_RendersUseAndForce(t *testing.T) {
	if got := NewIndexHint("user", "email_idx").ToSurql(); got != "/* USE INDEX user.email_idx */" {
		t.Errorf("got %q", got)
	}
	if got := NewIndexHint("user", "email_idx").WithForce(true).ToSurql(); got != "/* FORCE INDEX user.email_idx */" {
		t.Errorf("got %q", got)
	}
}

func TestParallelHint_Renders(t *testing.T) {
	if got := ParallelEnabled().ToSurql(); got != "/* PARALLEL ON */" {
		t.Errorf("got %q", got)
	}
	if got := ParallelDisabled().ToSurql(); got != "/* PARALLEL OFF */" {
		t.Errorf("got %q", got)
	}
	h, err := ParallelWithWorkers(4)
	if err != nil {
		t.Fatal(err)
	}
	if got := h.ToSurql(); got != "/* PARALLEL 4 */" {
		t.Errorf("got %q", got)
	}
}

func TestParallelHint_RejectsBadWorkerCount(t *testing.T) {
	if _, err := ParallelWithWorkers(0); err == nil {
		t.Error("expected error for 0 workers")
	}
	if _, err := ParallelWithWorkers(33); err == nil {
		t.Error("expected error for 33 workers")
	}
}

func TestTimeoutHint_Renders(t *testing.T) {
	h, err := NewTimeoutHint(30.0)
	if err != nil {
		t.Fatal(err)
	}
	if got := h.ToSurql(); got != "/* TIMEOUT 30s */" {
		t.Errorf("got %q", got)
	}
}

func TestTimeoutHint_RejectsNonPositive(t *testing.T) {
	if _, err := NewTimeoutHint(0); err == nil {
		t.Error("expected error for 0")
	}
	if _, err := NewTimeoutHint(-1.0); err == nil {
		t.Error("expected error for negative")
	}
}

func TestFetchHint_Renders(t *testing.T) {
	if got := FetchEagerHint().ToSurql(); got != "/* FETCH EAGER */" {
		t.Errorf("got %q", got)
	}
	if got := FetchLazyHint().ToSurql(); got != "/* FETCH LAZY */" {
		t.Errorf("got %q", got)
	}
	h, err := FetchBatchHint(100)
	if err != nil {
		t.Fatal(err)
	}
	if got := h.ToSurql(); got != "/* FETCH BATCH 100 */" {
		t.Errorf("got %q", got)
	}
}

func TestFetchHint_BatchValidatesSize(t *testing.T) {
	if _, err := FetchBatchHint(0); err == nil {
		t.Error("expected error for 0")
	}
	if _, err := FetchBatchHint(10_001); err == nil {
		t.Error("expected error for 10001")
	}
	bad := FetchHint{Strategy: FetchBatch}
	if err := bad.Validate(); err == nil {
		t.Error("expected validation error")
	}
}

func TestExplainHint_Renders(t *testing.T) {
	if got := ExplainShort().ToSurql(); got != "/* EXPLAIN */" {
		t.Errorf("got %q", got)
	}
	if got := ExplainFull().ToSurql(); got != "/* EXPLAIN FULL */" {
		t.Errorf("got %q", got)
	}
}

func TestValidateHint_ChecksTable(t *testing.T) {
	idx := NewIndexHint("user", "email_idx")
	if errs := ValidateHint(idx, "user"); len(errs) != 0 {
		t.Errorf("matching table should be valid: %v", errs)
	}
	errs := ValidateHint(idx, "post")
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %v", errs)
	}
	if !strings.Contains(errs[0], "user") || !strings.Contains(errs[0], "post") {
		t.Errorf("unexpected message: %q", errs[0])
	}
}

func TestMergeHints_ReplacesDuplicates(t *testing.T) {
	h1, _ := NewTimeoutHint(10.0)
	h2, _ := NewTimeoutHint(20.0)
	merged := MergeHints([]QueryHint{h1, h2})
	if len(merged) != 1 {
		t.Fatalf("expected 1, got %d", len(merged))
	}
	if got := merged[0].(TimeoutHint).Seconds; got != 20.0 {
		t.Errorf("got %v", got)
	}
}

func TestMergeHints_KeepsDistinctTypes(t *testing.T) {
	t1, _ := NewTimeoutHint(30.0)
	t2, _ := NewTimeoutHint(60.0)
	merged := MergeHints([]QueryHint{t1, ParallelEnabled(), t2})
	if len(merged) != 2 {
		t.Fatalf("expected 2, got %d", len(merged))
	}
}

func TestRenderHints_Empty(t *testing.T) {
	if got := RenderHints(nil); got != "" {
		t.Errorf("got %q", got)
	}
}

func TestRenderHints_JoinsAll(t *testing.T) {
	t1, _ := NewTimeoutHint(30.0)
	out := RenderHints([]QueryHint{t1, ParallelEnabled()})
	if !strings.Contains(out, "/* TIMEOUT 30s */") {
		t.Errorf("missing timeout: %q", out)
	}
	if !strings.Contains(out, "/* PARALLEL ON */") {
		t.Errorf("missing parallel: %q", out)
	}
}

func TestRenderer_MatchesFreeFunction(t *testing.T) {
	hints := []QueryHint{ExplainFull()}
	renderer := HintRenderer{}
	if renderer.RenderHints(hints) != RenderHints(hints) {
		t.Error("renderer and free function should match")
	}
}

func TestValidationErrorsAreClassified(t *testing.T) {
	_, err := NewTimeoutHint(-1)
	if !errors.Is(err, surqlerrors.ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}
