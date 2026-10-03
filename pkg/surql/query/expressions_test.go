package query

import (
	"testing"

	"github.com/Oneiriq/surql-go/pkg/surql/types"
)

func TestFieldAndValue(t *testing.T) {
	if got := Field("user.name").ToSurql(); got != "user.name" {
		t.Errorf("got %q", got)
	}
	if got := Value("Alice").ToSurql(); got != `'Alice'` {
		t.Errorf("got %q", got)
	}
	if got := Value(42).ToSurql(); got != "42" {
		t.Errorf("got %q", got)
	}
	if got := Value(true).ToSurql(); got != "true" {
		t.Errorf("got %q", got)
	}
}

func TestCount_Renders(t *testing.T) {
	if got := Count("").ToSurql(); got != "count()" {
		t.Errorf("got %q", got)
	}
	if got := Count("id").ToSurql(); got != "count(id)" {
		t.Errorf("got %q", got)
	}
}

// The aggregate helpers must render SurrealQL's own functions. The SQL
// spellings (`COUNT(*)`, `SUM(f)`, `AVG(f)`, `MIN(f)`, `MAX(f)`) are parse
// errors on SurrealDB.
func TestAggregateFunctions(t *testing.T) {
	tests := []struct {
		name string
		got  Expression
		want string
	}{
		{"Count all", Count(""), "count()"},
		{"Count field", Count("active"), "count(active)"},
		{"Sum", Sum("price"), "math::sum(price)"},
		{"Avg", Avg("age"), "math::mean(age)"},
		{"MinFn", MinFn("price"), "math::min(price)"},
		{"MaxFn", MaxFn("price"), "math::max(price)"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.got.ToSurql(); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
			if tc.got.Kind != ExprFunction {
				t.Errorf("kind = %q, want %q", tc.got.Kind, ExprFunction)
			}
		})
	}
}

// The Expression aggregates and the native factories are two spellings of
// the same SurrealQL, so they must never drift apart.
func TestAggregateFunctions_MatchNativeFactories(t *testing.T) {
	tests := []struct {
		name   string
		got    string
		native string
	}{
		{"Count all vs CountAll", Count("").ToSurql(), CountAll().ToSurql()},
		{"Count field vs CountField", Count("active").ToSurql(), CountField("active").ToSurql()},
		{"Sum vs MathSum", Sum("price").ToSurql(), MathSum("price").ToSurql()},
		{"Avg vs MathMean", Avg("price").ToSurql(), MathMean("price").ToSurql()},
		{"MinFn vs MathMin", MinFn("price").ToSurql(), MathMin("price").ToSurql()},
		{"MaxFn vs MathMax", MaxFn("price").ToSurql(), MathMax("price").ToSurql()},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.native {
				t.Errorf("got %q, want %q", tc.got, tc.native)
			}
		})
	}
}

func TestAggregateFunctions_InGroupedSelect(t *testing.T) {
	q := Query{}.SelectAliased(map[string]types.Operator{
		"total": Count(""),
		"spent": Sum("price"),
		"avg":   Avg("price"),
		"lo":    MinFn("price"),
		"hi":    MaxFn("price"),
	})
	q, err := q.FromTable("purchase")
	if err != nil {
		t.Fatal(err)
	}
	got, err := q.GroupAll().ToSurql()
	if err != nil {
		t.Fatal(err)
	}
	want := "SELECT math::mean(price) AS avg, math::max(price) AS hi, " +
		"math::min(price) AS lo, math::sum(price) AS spent, count() AS total " +
		"FROM purchase GROUP ALL"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMathNativeAggregates(t *testing.T) {
	if MathMean("score").ToSurql() != "math::mean(score)" ||
		MathSum("price").ToSurql() != "math::sum(price)" ||
		MathMax("score").ToSurql() != "math::max(score)" ||
		MathMin("price").ToSurql() != "math::min(price)" {
		t.Error("math aggregate mismatch")
	}
}

func TestStringFunctions(t *testing.T) {
	if Upper("name").ToSurql() != "string::uppercase(name)" {
		t.Error("upper")
	}
	if Lower("email").ToSurql() != "string::lowercase(email)" {
		t.Error("lower")
	}
	c := Concat(E(Field("first_name")), E(Value(" ")), E(Field("last_name")))
	if c.ToSurql() != "string::concat(first_name, ' ', last_name)" {
		t.Errorf("concat: %q", c.ToSurql())
	}
}

func TestArrayFunctions(t *testing.T) {
	if ArrayLength("tags").ToSurql() != "array::len(tags)" {
		t.Error("array_length")
	}
	if got := ArrayContains("tags", "python").ToSurql(); got != "array::includes(tags, 'python')" {
		t.Errorf("array_contains: %q", got)
	}
}

func TestMathFunctions(t *testing.T) {
	if Abs("temperature").ToSurql() != "math::abs(temperature)" {
		t.Error("abs")
	}
	if Ceil("price").ToSurql() != "math::ceil(price)" {
		t.Error("ceil")
	}
	if Floor("price").ToSurql() != "math::floor(price)" {
		t.Error("floor")
	}
	if Round("price", 2).ToSurql() != "math::round(price, 2)" {
		t.Error("round")
	}
}

func TestTimeFunctions(t *testing.T) {
	if TimeNow().ToSurql() != "time::now()" {
		t.Error("time_now")
	}
	if got := TimeFormat("created_at", "%Y-%m-%d").ToSurql(); got != "time::format(created_at, '%Y-%m-%d')" {
		t.Errorf("time_format: %q", got)
	}
}

func TestTypeFunctions(t *testing.T) {
	if TypeIs("value", "string").ToSurql() != "type::is::string(value)" {
		t.Error("type_is")
	}
	if Cast("id", "string").ToSurql() != "<string>id" {
		t.Error("cast")
	}
}

func TestFunc_AcceptsMixedArgs(t *testing.T) {
	c := Func("CONCAT", E(Field("first")), S("' '"), E(Field("last")))
	if c.ToSurql() != "CONCAT(first, ' ', last)" {
		t.Errorf("got %q", c.ToSurql())
	}
}

func TestAs_AliasesExpressions(t *testing.T) {
	if got := As(Count(""), "total").ToSurql(); got != "count() AS total" {
		t.Errorf("got %q", got)
	}
	inner := Concat(E(Field("first")), E(Field("last")))
	if got := As(inner, "full_name").ToSurql(); got != "string::concat(first, last) AS full_name" {
		t.Errorf("got %q", got)
	}
}

func TestRaw_PassesThrough(t *testing.T) {
	if Raw("time::now()").ToSurql() != "time::now()" {
		t.Error("raw")
	}
}

func TestKindTag_ReflectsConstructor(t *testing.T) {
	if Field("x").Kind != ExprField {
		t.Error("field kind")
	}
	if Value(1).Kind != ExprValue {
		t.Error("value kind")
	}
	if Count("").Kind != ExprFunction {
		t.Error("count kind")
	}
	if Raw("x").Kind != ExprRaw {
		t.Error("raw kind")
	}
}

func TestString_MatchesToSurql(t *testing.T) {
	e := Count("")
	if e.String() != e.ToSurql() {
		t.Error("String / ToSurql mismatch")
	}
}
