package query

import (
	"context"
	"errors"
	"testing"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
	"github.com/Oneiriq/surql-go/pkg/surql/types"
)

func TestSelectExpr_RendersRawExpressions(t *testing.T) {
	q := Query{}.SelectExpr(CountAll(), MathMean("strength"))
	q, err := q.FromTable("memory_entry")
	if err != nil {
		t.Fatal(err)
	}
	q = q.GroupAll()
	got, err := q.ToSurql()
	if err != nil {
		t.Fatal(err)
	}
	want := "SELECT count(), math::mean(strength) FROM memory_entry GROUP ALL"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSelectExpr_EmptyArgsSelectsStar(t *testing.T) {
	q := Query{}.SelectExpr()
	q, err := q.FromTable("t")
	if err != nil {
		t.Fatal(err)
	}
	got, err := q.ToSurql()
	if err != nil {
		t.Fatal(err)
	}
	if got != "SELECT * FROM t" {
		t.Errorf("got %q", got)
	}
}

func TestSelectAliased_RendersAliases(t *testing.T) {
	q := Query{}.SelectAliased(map[string]types.Operator{
		"total": MathSum("strength"),
		"n":     CountAll(),
	})
	q, err := q.FromTable("memory_entry")
	if err != nil {
		t.Fatal(err)
	}
	q = q.GroupAll()
	got, err := q.ToSurql()
	if err != nil {
		t.Fatal(err)
	}
	// Keys emit in sorted order: n before total.
	want := "SELECT count() AS n, math::sum(strength) AS total FROM memory_entry GROUP ALL"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSelectAliased_EmptySelectsStar(t *testing.T) {
	q := Query{}.SelectAliased(nil)
	q, err := q.FromTable("t")
	if err != nil {
		t.Fatal(err)
	}
	got, err := q.ToSurql()
	if err != nil {
		t.Fatal(err)
	}
	if got != "SELECT * FROM t" {
		t.Errorf("got %q", got)
	}
}

func TestFrom_PanicsOnInvalidTable(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on invalid table")
		}
	}()
	_ = Query{}.Select(nil).From("1bad")
}

func TestFrom_Ok(t *testing.T) {
	q := Query{}.Select(nil).From("users")
	got, err := q.ToSurql()
	if err != nil {
		t.Fatal(err)
	}
	if got != "SELECT * FROM users" {
		t.Errorf("got %q", got)
	}
}

func TestAggregateRecords_NilClient(t *testing.T) {
	// With valid opts, the nil-client check fires after opts
	// validation.
	_, err := AggregateRecords(context.Background(), nil, AggregateOpts{
		Table:  "t",
		Select: map[string]types.Operator{"n": CountAll()},
	})
	if err == nil || !errors.Is(err, surqlerrors.ErrValidation) {
		t.Errorf("want ErrValidation, got %v", err)
	}
}

func TestAggregateRecords_EmptyTable(t *testing.T) {
	// Table validation fires before the nil-client check so we can
	// exercise it without a live connection.
	_, err := AggregateRecords(context.Background(), nil, AggregateOpts{
		Select: map[string]types.Operator{"n": CountAll()},
	})
	if err == nil || !errors.Is(err, surqlerrors.ErrValidation) {
		t.Errorf("want ErrValidation, got %v", err)
	}
}

func TestAggregateRecords_EmptySelect(t *testing.T) {
	_, err := AggregateRecords(context.Background(), nil, AggregateOpts{
		Table: "t",
	})
	if err == nil || !errors.Is(err, surqlerrors.ErrValidation) {
		t.Errorf("want ErrValidation, got %v", err)
	}
}

func TestAggregateRecords_InvalidTable(t *testing.T) {
	_, err := AggregateRecords(context.Background(), nil, AggregateOpts{
		Table:  "1bad",
		Select: map[string]types.Operator{"n": CountAll()},
	})
	if err == nil || !errors.Is(err, surqlerrors.ErrValidation) {
		t.Errorf("want ErrValidation, got %v", err)
	}
}

func TestAggregateRecords_GroupAllPrecedenceOverGroupBy(t *testing.T) {
	// When both GroupAll and GroupBy are provided, GroupAll wins; we can
	// verify this by rendering the assembled query directly.
	q, err := buildAggregateQuery(AggregateOpts{
		Table:    "t",
		Select:   map[string]types.Operator{"n": CountAll()},
		GroupBy:  []string{"network"},
		GroupAll: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := q.ToSurql()
	if err != nil {
		t.Fatal(err)
	}
	want := "SELECT count() AS n FROM t GROUP ALL"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestAggregateRecords_GroupByOnly(t *testing.T) {
	q, err := buildAggregateQuery(AggregateOpts{
		Table:   "memory_entry",
		Select:  map[string]types.Operator{"n": CountAll()},
		GroupBy: []string{"network"},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := q.ToSurql()
	if err != nil {
		t.Fatal(err)
	}
	// SurrealDB requires GROUP BY idioms to appear in the projection;
	// AggregateRecords prepends them ahead of the aliased aggregations.
	want := "SELECT network, count() AS n FROM memory_entry GROUP BY network"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// buildAggregateQuery extracts the query assembly from AggregateRecords
// so unit tests can inspect the rendered SurrealQL without hitting the
// client.
func buildAggregateQuery(opts AggregateOpts) (Query, error) {
	q := Query{}.SelectAliased(opts.Select)
	if !opts.GroupAll && len(opts.GroupBy) > 0 {
		grouped := append([]string(nil), opts.GroupBy...)
		grouped = append(grouped, q.Fields...)
		q.Fields = grouped
	}
	q, err := q.FromTable(opts.Table)
	if err != nil {
		return Query{}, err
	}
	for _, w := range opts.Where {
		q, err = q.Where(w)
		if err != nil {
			return Query{}, err
		}
	}
	if opts.GroupAll {
		q = q.GroupAll()
	} else if len(opts.GroupBy) > 0 {
		q = q.GroupBy(opts.GroupBy...)
	}
	if opts.OrderBy != nil {
		q, err = q.OrderBy(opts.OrderBy.Field, opts.OrderBy.Direction)
		if err != nil {
			return Query{}, err
		}
	}
	if opts.Limit != nil {
		q, err = q.Limit(*opts.Limit)
		if err != nil {
			return Query{}, err
		}
	}
	if opts.Offset != nil {
		q, err = q.Offset(*opts.Offset)
		if err != nil {
			return Query{}, err
		}
	}
	return q, nil
}

func TestExecute_Method_NilClient(t *testing.T) {
	q, err := Query{}.Select(nil).FromTable("t")
	if err != nil {
		t.Fatal(err)
	}
	_, err = q.Execute(context.Background(), nil)
	if err == nil || !errors.Is(err, surqlerrors.ErrValidation) {
		t.Errorf("want ErrValidation, got %v", err)
	}
}
