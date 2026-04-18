package query

import "testing"

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
	if got := Count("").ToSurql(); got != "COUNT(*)" {
		t.Errorf("got %q", got)
	}
	if got := Count("id").ToSurql(); got != "COUNT(id)" {
		t.Errorf("got %q", got)
	}
}

func TestAggregateFunctions(t *testing.T) {
	if Sum("price").ToSurql() != "SUM(price)" ||
		Avg("age").ToSurql() != "AVG(age)" ||
		MinFn("price").ToSurql() != "MIN(price)" ||
		MaxFn("price").ToSurql() != "MAX(price)" {
		t.Error("aggregate mismatch")
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
	if got := As(Count(""), "total").ToSurql(); got != "COUNT(*) AS total" {
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
