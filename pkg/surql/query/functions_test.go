package query

import (
	"testing"

	"github.com/Oneiriq/surql-go/pkg/surql/types"
)

func TestMathFactories(t *testing.T) {
	tests := []struct {
		name string
		got  types.SurrealFn
		want string
	}{
		{"MathAbs", MathAbs("temperature"), "math::abs(temperature)"},
		{"MathCeil", MathCeil("price"), "math::ceil(price)"},
		{"MathFloor", MathFloor("price"), "math::floor(price)"},
		{"MathRound", MathRound("price"), "math::round(price)"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.got.ToSurql(); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestStringFactories(t *testing.T) {
	tests := []struct {
		name string
		got  types.SurrealFn
		want string
	}{
		{"StringLen", StringLen("name"), "string::len(name)"},
		{"StringLower", StringLower("name"), "string::lowercase(name)"},
		{"StringUpper", StringUpper("name"), "string::uppercase(name)"},
		{"StringConcat two", StringConcat("first", "last"), "string::concat(first, last)"},
		{"StringConcat three", StringConcat("a", "' '", "b"), "string::concat(a, ' ', b)"},
		{"StringConcat one", StringConcat("solo"), "string::concat(solo)"},
		{"StringConcat empty", StringConcat(), "string::concat()"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.got.ToSurql(); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCountFactories(t *testing.T) {
	if got := CountAll().ToSurql(); got != "count()" {
		t.Errorf("CountAll: %q", got)
	}
	if got := CountField("id").ToSurql(); got != "count(id)" {
		t.Errorf("CountField: %q", got)
	}
	if got := CountIf("active = true").ToSurql(); got != "count(active = true)" {
		t.Errorf("CountIf: %q", got)
	}
}

func TestSurrealFnFactories_ComposeInWhere(t *testing.T) {
	// SurrealFn satisfies types.Operator; passing it into Where should
	// splice the raw expression.
	q, err := Query{}.Select(nil).FromTable("t")
	if err != nil {
		t.Fatal(err)
	}
	q, err = q.Where(MathAbs("balance"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := q.ToSurql()
	if err != nil {
		t.Fatal(err)
	}
	want := "SELECT * FROM t WHERE (math::abs(balance))"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSurrealFnFactories_ComposeInUpdateSet(t *testing.T) {
	// When used as a map value in UPDATE data, SurrealFn renders raw.
	q, err := Query{}.Update("user:alice", map[string]any{
		"login_count": CountField("id"),
		"last_seen":   types.NewSurrealFn("time::now()"),
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := q.ToSurql()
	if err != nil {
		t.Fatal(err)
	}
	want := "UPDATE user:alice SET last_seen = time::now(), login_count = count(id)"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExpressionRawSurqlValueMarker(t *testing.T) {
	// Expression is declared as a types.RawSurqlValue so that existing
	// helpers like MathMean / TimeNow can be passed as UPDATE values and
	// render raw (not quoted).
	q, err := Query{}.Update("t:1", map[string]any{
		"avg_score": MathMean("score"),
		"updated":   TimeNow(),
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := q.ToSurql()
	if err != nil {
		t.Fatal(err)
	}
	want := "UPDATE t:1 SET avg_score = math::mean(score), updated = time::now()"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
