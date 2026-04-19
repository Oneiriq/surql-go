package query

import (
	"fmt"
	"strings"

	"github.com/Oneiriq/surql-go/pkg/surql/types"
)

// ExpressionKind tags an Expression by category (field / value / function /
// raw). Consumers can introspect without re-parsing the SurrealQL string.
type ExpressionKind string

const (
	// ExprRaw is the default / aliased / raw category.
	ExprRaw ExpressionKind = "raw"
	// ExprField is a field reference.
	ExprField ExpressionKind = "field"
	// ExprValue is a quoted literal.
	ExprValue ExpressionKind = "value"
	// ExprFunction is a function-call expression.
	ExprFunction ExpressionKind = "function"
)

// Expression is a typed SurrealQL fragment: a rendered string plus a kind tag.
type Expression struct {
	SQL  string         `json:"sql"`
	Kind ExpressionKind `json:"kind,omitempty"`
}

// ToSurql renders as SurrealQL.
func (e Expression) ToSurql() string { return e.SQL }

// String implements fmt.Stringer.
func (e Expression) String() string { return e.SQL }

// IsRawSurqlValue marks Expression as a [types.RawSurqlValue] so it
// renders verbatim when used as a value in CREATE/UPDATE/UPSERT set
// clauses or in quoted operator values.
func (e Expression) IsRawSurqlValue() {}

// NewRaw builds a raw-kind Expression.
func NewRaw(sql string) Expression { return Expression{SQL: sql, Kind: ExprRaw} }

// NewField builds a field-kind Expression.
func NewField(name string) Expression { return Expression{SQL: name, Kind: ExprField} }

// NewValue builds a value-kind Expression, quoted with SurrealQL rules.
func NewValue(v any) Expression {
	return Expression{SQL: quoteValueExpr(v), Kind: ExprValue}
}

// NewFunction builds a function-kind Expression from a pre-rendered `FN(...)`.
func NewFunction(sql string) Expression { return Expression{SQL: sql, Kind: ExprFunction} }

// Field is the free-function alias for [NewField].
func Field(name string) Expression { return NewField(name) }

// Value is the free-function alias for [NewValue].
func Value(v any) Expression { return NewValue(v) }

// Raw is the free-function alias for [NewRaw].
func Raw(sql string) Expression { return NewRaw(sql) }

// ExprArg is the union type accepted by Func / Concat: either a built
// Expression or a raw string fragment.
type ExprArg struct {
	expr *Expression
	str  *string
}

// E wraps an Expression for Func / Concat.
func E(e Expression) ExprArg { return ExprArg{expr: &e} }

// S wraps a raw string for Func / Concat.
func S(s string) ExprArg { return ExprArg{str: &s} }

func (a ExprArg) render() string {
	if a.expr != nil {
		return a.expr.ToSurql()
	}
	if a.str != nil {
		return *a.str
	}
	return ""
}

// Func builds a function-call Expression: `Func("UPPER", E(Field("name")))`.
func Func(name string, args ...ExprArg) Expression {
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = a.render()
	}
	return NewFunction(fmt.Sprintf("%s(%s)", name, strings.Join(parts, ", ")))
}

// ---------------------------------------------------------------------------
// Aggregate functions
// ---------------------------------------------------------------------------

// Count builds `COUNT(*)` or `COUNT(field)` when fieldName is non-empty.
func Count(fieldName string) Expression {
	if fieldName == "" {
		return NewFunction("COUNT(*)")
	}
	return NewFunction(fmt.Sprintf("COUNT(%s)", fieldName))
}

// Sum builds `SUM(field)`.
func Sum(fieldName string) Expression { return NewFunction(fmt.Sprintf("SUM(%s)", fieldName)) }

// Avg builds `AVG(field)`.
func Avg(fieldName string) Expression { return NewFunction(fmt.Sprintf("AVG(%s)", fieldName)) }

// MinFn builds `MIN(field)`.
func MinFn(fieldName string) Expression { return NewFunction(fmt.Sprintf("MIN(%s)", fieldName)) }

// MaxFn builds `MAX(field)`.
func MaxFn(fieldName string) Expression { return NewFunction(fmt.Sprintf("MAX(%s)", fieldName)) }

// ---------------------------------------------------------------------------
// String functions
// ---------------------------------------------------------------------------

// Upper builds `string::uppercase(field)`.
func Upper(fieldName string) Expression {
	return NewFunction(fmt.Sprintf("string::uppercase(%s)", fieldName))
}

// Lower builds `string::lowercase(field)`.
func Lower(fieldName string) Expression {
	return NewFunction(fmt.Sprintf("string::lowercase(%s)", fieldName))
}

// Concat builds `string::concat(a, b, c, ...)`.
func Concat(parts ...ExprArg) Expression {
	rendered := make([]string, len(parts))
	for i, p := range parts {
		rendered[i] = p.render()
	}
	return NewFunction(fmt.Sprintf("string::concat(%s)", strings.Join(rendered, ", ")))
}

// ---------------------------------------------------------------------------
// Array functions
// ---------------------------------------------------------------------------

// ArrayLength builds `array::len(field)`.
func ArrayLength(fieldName string) Expression {
	return NewFunction(fmt.Sprintf("array::len(%s)", fieldName))
}

// ArrayContains builds `array::includes(field, value)`.
func ArrayContains(fieldName string, v any) Expression {
	return NewFunction(fmt.Sprintf("array::includes(%s, %s)", fieldName, quoteValueExpr(v)))
}

// ---------------------------------------------------------------------------
// Math functions
// ---------------------------------------------------------------------------

// Abs builds `math::abs(field)`.
func Abs(fieldName string) Expression {
	return NewFunction(fmt.Sprintf("math::abs(%s)", fieldName))
}

// Ceil builds `math::ceil(field)`.
func Ceil(fieldName string) Expression {
	return NewFunction(fmt.Sprintf("math::ceil(%s)", fieldName))
}

// Floor builds `math::floor(field)`.
func Floor(fieldName string) Expression {
	return NewFunction(fmt.Sprintf("math::floor(%s)", fieldName))
}

// Round builds `math::round(field, precision)`. Pass 0 for integer rounding.
func Round(fieldName string, precision int) Expression {
	return NewFunction(fmt.Sprintf("math::round(%s, %d)", fieldName, precision))
}

// MathMean builds `math::mean(field)`.
func MathMean(fieldName string) Expression {
	return NewFunction(fmt.Sprintf("math::mean(%s)", fieldName))
}

// MathSum builds `math::sum(field)`.
func MathSum(fieldName string) Expression {
	return NewFunction(fmt.Sprintf("math::sum(%s)", fieldName))
}

// MathMax builds `math::max(field)`.
func MathMax(fieldName string) Expression {
	return NewFunction(fmt.Sprintf("math::max(%s)", fieldName))
}

// MathMin builds `math::min(field)`.
func MathMin(fieldName string) Expression {
	return NewFunction(fmt.Sprintf("math::min(%s)", fieldName))
}

// ---------------------------------------------------------------------------
// Time, type, composition
// ---------------------------------------------------------------------------

// TimeNow builds `time::now()`.
func TimeNow() Expression { return NewFunction("time::now()") }

// TimeFormat builds `time::format(field, 'fmt')`.
func TimeFormat(fieldName, format string) Expression {
	return NewFunction(fmt.Sprintf("time::format(%s, %s)", fieldName, quoteValueExpr(format)))
}

// TypeIs builds `type::is::<type_name>(field)`.
func TypeIs(fieldName, typeName string) Expression {
	return NewFunction(fmt.Sprintf("type::is::%s(%s)", typeName, fieldName))
}

// Cast builds `<target_type>field` — SurrealQL cast syntax.
func Cast(fieldName, targetType string) Expression {
	return NewRaw(fmt.Sprintf("<%s>%s", targetType, fieldName))
}

// As aliases an expression: `As(Count(""), "total")` -> `COUNT(*) AS total`.
func As(expr Expression, alias string) Expression {
	return NewRaw(fmt.Sprintf("%s AS %s", expr.ToSurql(), alias))
}

// ---------------------------------------------------------------------------
// quoteValueExpr: SurrealQL literal renderer used by expression helpers.
// Delegates to the types package's operator rendering to stay consistent.
// ---------------------------------------------------------------------------

func quoteValueExpr(v any) string {
	// Reuse the rendering path via a scratch Eq operator. This keeps quoting
	// logic DRY with the types/operators package.
	rendered := types.EqOp("__x__", v).ToSurql()
	const prefix = "__x__ = "
	return strings.TrimPrefix(rendered, prefix)
}
