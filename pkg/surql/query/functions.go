// Package query — SurrealQL function factories.
//
// These builders complement the existing expressions.go helpers. They
// return [types.SurrealFn] so they are recognised by the raw-value
// renderer in pkg/surql/types (without relying on
// [types.RawSurqlValue]) and compose transparently in:
//
//   - SET / update payloads (via quoteValue's SurrealFn case)
//   - WHERE conditions (SurrealFn implements [types.Operator])
//   - SELECT projections (via [Query.SelectExpr])
//
// Naming follows the surql-py port's snake_case camelCased:
// `math_mean` -> MathMean, `string_len` -> StringLen, `count_if` ->
// CountIf, etc. Where a name already existed as an [Expression] helper
// (TimeNow, MathMean, MathSum, Count, StringLower, StringUpper) those
// originals stay put; Expression now implements [types.RawSurqlValue]
// so both paths compose identically.
package query

import (
	"fmt"

	"github.com/Oneiriq/surql-go/pkg/surql/types"
)

// ---------------------------------------------------------------------------
// math::* factories returning types.SurrealFn.
//
// MathAbs, MathCeil, MathFloor, MathRound are new (no prior Expression
// equivalents under those names).
// ---------------------------------------------------------------------------

// MathAbs returns `math::abs(field)`.
func MathAbs(fieldName string) types.SurrealFn {
	return types.NewSurrealFn(fmt.Sprintf("math::abs(%s)", fieldName))
}

// MathCeil returns `math::ceil(field)`.
func MathCeil(fieldName string) types.SurrealFn {
	return types.NewSurrealFn(fmt.Sprintf("math::ceil(%s)", fieldName))
}

// MathFloor returns `math::floor(field)`.
func MathFloor(fieldName string) types.SurrealFn {
	return types.NewSurrealFn(fmt.Sprintf("math::floor(%s)", fieldName))
}

// MathRound returns `math::round(field)` (integer rounding).
func MathRound(fieldName string) types.SurrealFn {
	return types.NewSurrealFn(fmt.Sprintf("math::round(%s)", fieldName))
}

// ---------------------------------------------------------------------------
// string::* factories returning types.SurrealFn.
// ---------------------------------------------------------------------------

// StringLen returns `string::len(field)`.
func StringLen(fieldName string) types.SurrealFn {
	return types.NewSurrealFn(fmt.Sprintf("string::len(%s)", fieldName))
}

// StringLower returns `string::lowercase(field)`.
func StringLower(fieldName string) types.SurrealFn {
	return types.NewSurrealFn(fmt.Sprintf("string::lowercase(%s)", fieldName))
}

// StringUpper returns `string::uppercase(field)`.
func StringUpper(fieldName string) types.SurrealFn {
	return types.NewSurrealFn(fmt.Sprintf("string::uppercase(%s)", fieldName))
}

// StringConcat returns `string::concat(a, b, ...)`. Each arg is rendered
// verbatim; wrap literal strings with single-quote helpers if needed.
func StringConcat(args ...string) types.SurrealFn {
	joined := ""
	for i, a := range args {
		if i > 0 {
			joined += ", "
		}
		joined += a
	}
	return types.NewSurrealFn(fmt.Sprintf("string::concat(%s)", joined))
}

// ---------------------------------------------------------------------------
// Count factories returning types.SurrealFn.
//
// The existing [Count] helper in expressions.go takes an optional field
// name and returns Expression (`COUNT(*)` / `COUNT(id)`). CountAll and
// CountIf provide SurrealDB-native lowercase `count()` / `count(expr)`
// forms useful in aggregation queries.
// ---------------------------------------------------------------------------

// CountAll returns `count()` — the SurrealDB aggregation function with
// no arguments (one-per-row semantic, useful with `GROUP ALL`).
func CountAll() types.SurrealFn { return types.NewSurrealFn("count()") }

// CountField returns `count(field)`.
func CountField(fieldName string) types.SurrealFn {
	return types.NewSurrealFn(fmt.Sprintf("count(%s)", fieldName))
}

// CountIf returns `count(condition)` where condition is a SurrealQL
// boolean expression rendered verbatim (e.g. "active = true").
func CountIf(condition string) types.SurrealFn {
	return types.NewSurrealFn(fmt.Sprintf("count(%s)", condition))
}
