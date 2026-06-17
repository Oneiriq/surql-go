package query

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
	"github.com/Oneiriq/surql-go/pkg/surql/types"
)

// Operation tags the high-level SurrealQL statement a Query produces.
type Operation string

const (
	// OpSelect renders `SELECT ... FROM ...`.
	OpSelect Operation = "SELECT"
	// OpInsert renders `CREATE ... CONTENT {...}`.
	OpInsert Operation = "INSERT"
	// OpUpdate renders `UPDATE ... SET ...`.
	OpUpdate Operation = "UPDATE"
	// OpUpsert renders `UPSERT ... CONTENT {...}`.
	OpUpsert Operation = "UPSERT"
	// OpDelete renders `DELETE ...`.
	OpDelete Operation = "DELETE"
	// OpRelate renders `RELATE from->edge->to`.
	OpRelate Operation = "RELATE"
)

// OrderField is one `(field, direction)` pair used in `ORDER BY`.
type OrderField struct {
	Field     string
	Direction string
}

// Query is an immutable SurrealQL query builder.
//
// Every method returns a new Query; the receiver is never mutated. The
// zero value represents an empty query with no operation yet assigned.
// `ToSurql` renders the accumulated state to a SurrealQL string and
// returns `ErrValidation` if the query is incomplete.
type Query struct {
	// Operation is the statement kind; empty until one of Select/Insert/... is called.
	Operation Operation

	// TableName is the target table or record id.
	TableName string

	// Fields lists the projection columns for SELECT.
	Fields []string

	// Conditions are raw SurrealQL WHERE fragments (already operator-rendered).
	Conditions []string

	// OrderFields are the ORDER BY pairs in insertion order.
	OrderFields []OrderField

	// GroupFields are the explicit GROUP BY columns.
	GroupFields []string

	// GroupAllFlag toggles SurrealQL's `GROUP ALL` aggregation.
	GroupAllFlag bool

	// LimitValue bounds the number of rows when non-nil.
	LimitValue *int

	// OffsetValue skips rows when non-nil (rendered as `START n`).
	OffsetValue *int

	// InsertData carries the body for INSERT statements.
	InsertData map[string]any

	// UpdateData carries the body for UPDATE / UPSERT statements.
	UpdateData map[string]any

	// RelateFrom is the source record for RELATE.
	RelateFrom string

	// RelateTo is the destination record for RELATE.
	RelateTo string

	// RelateData is the edge body for RELATE.
	RelateData map[string]any

	// JoinClauses are raw JOIN fragments appended after the FROM.
	JoinClauses []string

	// GraphTraversal is the `->edge->target` suffix for SELECT.
	GraphTraversal string

	// ReturnFormat controls the `RETURN ...` clause on mutations.
	ReturnFormat ReturnFormat

	// Vector search parameters (MTREE-style `<|k,distance|>`).
	VectorField     string
	VectorValue     []float64
	VectorK         *int
	VectorDistance  VectorDistanceType
	VectorThreshold *float64

	// Full-text search parameters (the `@@` / `@n@` matches operator).
	// FulltextField is the matched column; FulltextReference is the match
	// reference number tying the predicate to `search::score(n)` (the `@n@`
	// form); FulltextQuery is the query text, inlined as a quoted, escaped
	// literal. FulltextSet marks the predicate as configured so a zero
	// reference is distinguishable from "unset".
	FulltextField     string
	FulltextReference int
	FulltextQuery     string
	FulltextSet       bool

	// Hints accumulates optimization comments rendered by [RenderHints].
	Hints []QueryHint
}

// NewQuery returns an empty Query.
func NewQuery() Query { return Query{} }

// ---------------------------------------------------------------------------
// Helpers (private) — copy-and-mutate semantics.
// ---------------------------------------------------------------------------

// clone returns a deep-enough copy so that caller mutations on the
// returned value cannot leak back into the original slices/maps.
func (q Query) clone() Query {
	out := q
	if q.Fields != nil {
		out.Fields = append([]string(nil), q.Fields...)
	}
	if q.Conditions != nil {
		out.Conditions = append([]string(nil), q.Conditions...)
	}
	if q.OrderFields != nil {
		out.OrderFields = append([]OrderField(nil), q.OrderFields...)
	}
	if q.GroupFields != nil {
		out.GroupFields = append([]string(nil), q.GroupFields...)
	}
	if q.JoinClauses != nil {
		out.JoinClauses = append([]string(nil), q.JoinClauses...)
	}
	if q.VectorValue != nil {
		out.VectorValue = append([]float64(nil), q.VectorValue...)
	}
	if q.Hints != nil {
		out.Hints = append([]QueryHint(nil), q.Hints...)
	}
	if q.InsertData != nil {
		out.InsertData = copyMap(q.InsertData)
	}
	if q.UpdateData != nil {
		out.UpdateData = copyMap(q.UpdateData)
	}
	if q.RelateData != nil {
		out.RelateData = copyMap(q.RelateData)
	}
	return out
}

func copyMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// ---------------------------------------------------------------------------
// Fluent API
// ---------------------------------------------------------------------------

// Select starts a SELECT query. A nil or empty slice selects `*`.
func (q Query) Select(fields []string) Query {
	out := q.clone()
	out.Operation = OpSelect
	if len(fields) == 0 {
		out.Fields = []string{"*"}
	} else {
		out.Fields = append([]string(nil), fields...)
	}
	return out
}

// SelectExpr starts a SELECT query whose projection consists of the
// supplied [types.SurrealFn] function factories (or any other
// [types.Operator] implementer). Unlike [Query.Select] which renders
// each field as a plain identifier, SelectExpr emits each expression's
// SurrealQL form verbatim — enabling aggregations such as
// `SELECT count(), math::mean(strength) FROM t GROUP ALL`.
//
// Pass zero expressions to select `*` (matching [Query.Select]'s
// empty-slice behaviour).
func (q Query) SelectExpr(exprs ...types.Operator) Query {
	out := q.clone()
	out.Operation = OpSelect
	if len(exprs) == 0 {
		out.Fields = []string{"*"}
		return out
	}
	rendered := make([]string, len(exprs))
	for i, e := range exprs {
		rendered[i] = e.ToSurql()
	}
	out.Fields = rendered
	return out
}

// SelectAliased is SelectExpr with per-expression aliases. The map's
// insertion order is not preserved in Go, so aliases are emitted in
// sorted-key order for deterministic output — matching renderDataObject.
//
// Accepts any [types.Operator] implementation so both the SurrealFn
// factories in functions.go and the Expression helpers in
// expressions.go compose through the same interface.
func (q Query) SelectAliased(fields map[string]types.Operator) Query {
	out := q.clone()
	out.Operation = OpSelect
	if len(fields) == 0 {
		out.Fields = []string{"*"}
		return out
	}
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	rendered := make([]string, len(keys))
	for i, k := range keys {
		rendered[i] = fields[k].ToSurql() + " AS " + k
	}
	out.Fields = rendered
	return out
}

// From is an alias for [Query.FromTable] that panics on validation
// error, matching the fluent `NewQuery().Select(...).From(...)` shape.
// Prefer [Query.FromTable] when caller code needs to surface the error.
func (q Query) From(table string) Query {
	out, err := q.FromTable(table)
	if err != nil {
		panic(err)
	}
	return out
}

// FromTable sets the table (or record id) the query targets. Returns
// ErrValidation when the table portion is not a safe identifier.
func (q Query) FromTable(table string) (Query, error) {
	if err := validateIdentifier(splitTablePart(table), "table name"); err != nil {
		return Query{}, err
	}
	out := q.clone()
	out.TableName = table
	return out, nil
}

// Where appends a condition. Accepts either a string (inserted as-is)
// or a `types.Operator` (rendered via its `ToSurql` method).
func (q Query) Where(condition any) (Query, error) {
	var rendered string
	switch c := condition.(type) {
	case string:
		rendered = c
	case types.Operator:
		rendered = c.ToSurql()
	case nil:
		return Query{}, surqlerrors.New(surqlerrors.ErrValidation, "Where condition cannot be nil")
	default:
		return Query{}, surqlerrors.Newf(
			surqlerrors.ErrValidation,
			"Where condition must be a string or types.Operator, got %T", condition,
		)
	}
	out := q.clone()
	out.Conditions = append(out.Conditions, rendered)
	return out, nil
}

// OrderBy appends an ORDER BY pair. `direction` must be `ASC` or `DESC`
// (case-insensitive); any other value returns ErrValidation.
func (q Query) OrderBy(field, direction string) (Query, error) {
	dir := strings.ToUpper(direction)
	if dir != "ASC" && dir != "DESC" {
		return Query{}, surqlerrors.Newf(
			surqlerrors.ErrValidation,
			"Invalid direction: %s. Must be ASC or DESC", direction,
		)
	}
	out := q.clone()
	out.OrderFields = append(out.OrderFields, OrderField{Field: field, Direction: dir})
	return out, nil
}

// GroupBy appends explicit group keys.
func (q Query) GroupBy(fields ...string) Query {
	out := q.clone()
	out.GroupFields = append(out.GroupFields, fields...)
	return out
}

// GroupAll sets `GROUP ALL` for whole-result aggregation.
func (q Query) GroupAll() Query {
	out := q.clone()
	out.GroupAllFlag = true
	return out
}

// Limit sets the LIMIT clause. Negative values yield ErrValidation.
func (q Query) Limit(n int) (Query, error) {
	if n < 0 {
		return Query{}, surqlerrors.Newf(surqlerrors.ErrValidation,
			"Limit must be non-negative, got %d", n)
	}
	v := n
	out := q.clone()
	out.LimitValue = &v
	return out, nil
}

// Offset sets the OFFSET (rendered as `START n`). Negative values yield ErrValidation.
func (q Query) Offset(n int) (Query, error) {
	if n < 0 {
		return Query{}, surqlerrors.Newf(surqlerrors.ErrValidation,
			"Offset must be non-negative, got %d", n)
	}
	v := n
	out := q.clone()
	out.OffsetValue = &v
	return out, nil
}

// Insert builds an INSERT (SurrealDB `CREATE ... CONTENT {...}`) query.
func (q Query) Insert(table string, data map[string]any) (Query, error) {
	if err := validateIdentifier(table, "table name"); err != nil {
		return Query{}, err
	}
	if err := validateDataKeys(data); err != nil {
		return Query{}, err
	}
	out := q.clone()
	out.Operation = OpInsert
	out.TableName = table
	out.InsertData = copyMap(data)
	return out, nil
}

// Update builds an UPDATE query.
func (q Query) Update(target string, data map[string]any) (Query, error) {
	if err := validateIdentifier(splitTablePart(target), "table name"); err != nil {
		return Query{}, err
	}
	if err := validateDataKeys(data); err != nil {
		return Query{}, err
	}
	out := q.clone()
	out.Operation = OpUpdate
	out.TableName = target
	out.UpdateData = copyMap(data)
	return out, nil
}

// Upsert builds an UPSERT query (insert-or-update).
func (q Query) Upsert(target string, data map[string]any) (Query, error) {
	if err := validateIdentifier(splitTablePart(target), "table name"); err != nil {
		return Query{}, err
	}
	if err := validateDataKeys(data); err != nil {
		return Query{}, err
	}
	out := q.clone()
	out.Operation = OpUpsert
	out.TableName = target
	out.UpdateData = copyMap(data)
	return out, nil
}

// Delete builds a DELETE query.
func (q Query) Delete(target string) (Query, error) {
	if err := validateIdentifier(splitTablePart(target), "table name"); err != nil {
		return Query{}, err
	}
	out := q.clone()
	out.Operation = OpDelete
	out.TableName = target
	return out, nil
}

// Relate builds a RELATE query. `from` and `to` may be a `string` or
// `types.RecordID`; any other concrete type returns ErrValidation.
func (q Query) Relate(edgeTable string, from, to any, data map[string]any) (Query, error) {
	if err := validateIdentifier(edgeTable, "edge table name"); err != nil {
		return Query{}, err
	}
	fromStr, err := relateEndpoint(from)
	if err != nil {
		return Query{}, err
	}
	toStr, err := relateEndpoint(to)
	if err != nil {
		return Query{}, err
	}
	if err := validateIdentifier(splitTablePart(fromStr), "from table name"); err != nil {
		return Query{}, err
	}
	if err := validateIdentifier(splitTablePart(toStr), "to table name"); err != nil {
		return Query{}, err
	}
	if data != nil {
		if err := validateDataKeys(data); err != nil {
			return Query{}, err
		}
	}
	out := q.clone()
	out.Operation = OpRelate
	out.TableName = edgeTable
	out.RelateFrom = fromStr
	out.RelateTo = toStr
	out.RelateData = copyMap(data)
	return out, nil
}

// Traverse sets the graph traversal suffix appended to the FROM clause.
func (q Query) Traverse(path string) Query {
	out := q.clone()
	out.GraphTraversal = path
	return out
}

// Join appends a raw JOIN clause.
func (q Query) Join(clause string) Query {
	out := q.clone()
	out.JoinClauses = append(out.JoinClauses, clause)
	return out
}

// VectorSearch configures a SurrealDB MTREE k-nearest-neighbour search.
// Pass a non-nil `threshold` to emit the three-arg operator form.
func (q Query) VectorSearch(
	field string,
	vector []float64,
	k int,
	distance VectorDistanceType,
	threshold *float64,
) (Query, error) {
	if k < 1 {
		return Query{}, surqlerrors.Newf(surqlerrors.ErrValidation,
			"k must be at least 1, got %d", k)
	}
	if len(vector) == 0 {
		return Query{}, surqlerrors.New(surqlerrors.ErrValidation, "Vector cannot be empty")
	}
	kv := k
	out := q.clone()
	out.VectorField = field
	out.VectorValue = append([]float64(nil), vector...)
	out.VectorK = &kv
	out.VectorDistance = distance
	if threshold != nil {
		t := *threshold
		out.VectorThreshold = &t
	}
	return out, nil
}

// SimilarityScore adds `vector::similarity::<metric>(field, vector) AS alias`
// to the projection.
func (q Query) SimilarityScore(field string, vector []float64, metric VectorDistanceType, alias string) Query {
	vec := renderVectorLiteral(vector)
	metricLower := strings.ToLower(string(metric))
	if alias == "" {
		alias = "similarity"
	}
	expr := fmt.Sprintf("vector::similarity::%s(%s, %s) AS %s", metricLower, field, vec, alias)
	out := q.clone()
	out.Fields = append(out.Fields, expr)
	return out
}

// ---------------------------------------------------------------------------
// Full-text search
// ---------------------------------------------------------------------------

// FullTextSearch configures a full-text SEARCH predicate rendered as
// `<field> @<reference>@ <query>` in the WHERE clause.
//
// The reference integer ties the match to a [Query.SearchScore] (or
// search::highlight) call, so a row's BM25 relevance can be projected and
// ordered on. It requires a BM25 SEARCH index on field (see
// schema.BM25Index). The query text is inlined as a quoted, escaped literal.
// An empty field or query yields an ErrValidation error.
func (q Query) FullTextSearch(field string, reference int, queryText string) (Query, error) {
	if field == "" {
		return Query{}, surqlerrors.New(surqlerrors.ErrValidation,
			"Full-text search field cannot be empty")
	}
	if queryText == "" {
		return Query{}, surqlerrors.New(surqlerrors.ErrValidation,
			"Full-text search query cannot be empty")
	}
	out := q.clone()
	out.FulltextField = field
	out.FulltextReference = reference
	out.FulltextQuery = queryText
	out.FulltextSet = true
	return out, nil
}

// SearchScore appends `search::score(<reference>) AS <alias>` to the projected
// fields — the BM25 relevance for the match registered at reference by
// [Query.FullTextSearch]. Order by alias to rank.
func (q Query) SearchScore(reference int, alias string) Query {
	expr := fmt.Sprintf("search::score(%d) AS %s", reference, alias)
	out := q.clone()
	out.Fields = append(out.Fields, expr)
	return out
}

// WithReturnFormat sets the RETURN clause to a specific format.
func (q Query) WithReturnFormat(f ReturnFormat) Query {
	out := q.clone()
	out.ReturnFormat = f
	return out
}

// ReturnNone shortcut for `RETURN NONE`.
func (q Query) ReturnNone() Query { return q.WithReturnFormat(ReturnNone) }

// ReturnDiff shortcut for `RETURN DIFF`.
func (q Query) ReturnDiff() Query { return q.WithReturnFormat(ReturnDiff) }

// ReturnFull shortcut for `RETURN FULL`.
func (q Query) ReturnFull() Query { return q.WithReturnFormat(ReturnFull) }

// ReturnBefore shortcut for `RETURN BEFORE`.
func (q Query) ReturnBefore() Query { return q.WithReturnFormat(ReturnBefore) }

// ReturnAfter shortcut for `RETURN AFTER`.
func (q Query) ReturnAfter() Query { return q.WithReturnFormat(ReturnAfter) }

// Hint appends a query optimization hint.
func (q Query) Hint(hint QueryHint) Query {
	out := q.clone()
	out.Hints = append(out.Hints, hint)
	return out
}

// WithHints appends multiple hints in order.
func (q Query) WithHints(hints ...QueryHint) Query {
	out := q.clone()
	out.Hints = append(out.Hints, hints...)
	return out
}

// ForceIndex adds an `IndexHint` with the force flag set. Requires the
// table to have been set first.
func (q Query) ForceIndex(index string) (Query, error) {
	if q.TableName == "" {
		return Query{}, surqlerrors.New(surqlerrors.ErrValidation, "Table name required for index hint")
	}
	return q.Hint(IndexHint{Table: q.TableName, Index: index, Force: true}), nil
}

// UseIndex adds a soft `IndexHint` (non-forcing).
func (q Query) UseIndex(index string) (Query, error) {
	if q.TableName == "" {
		return Query{}, surqlerrors.New(surqlerrors.ErrValidation, "Table name required for index hint")
	}
	return q.Hint(IndexHint{Table: q.TableName, Index: index, Force: false}), nil
}

// WithTimeout adds a TimeoutHint.
func (q Query) WithTimeout(seconds float64) (Query, error) {
	hint, err := NewTimeoutHint(seconds)
	if err != nil {
		return Query{}, err
	}
	return q.Hint(hint), nil
}

// Parallel adds a ParallelHint. When maxWorkers is nil, enables with
// the server default worker count.
func (q Query) Parallel(maxWorkers *uint) (Query, error) {
	if maxWorkers == nil {
		return q.Hint(ParallelEnabled()), nil
	}
	hint, err := ParallelWithWorkers(*maxWorkers)
	if err != nil {
		return Query{}, err
	}
	return q.Hint(hint), nil
}

// WithFetch adds a FetchHint.
func (q Query) WithFetch(strategy FetchStrategy, batchSize *uint32) (Query, error) {
	switch strategy {
	case FetchEager:
		return q.Hint(FetchEagerHint()), nil
	case FetchLazy:
		return q.Hint(FetchLazyHint()), nil
	case FetchBatch:
		if batchSize == nil {
			return Query{}, surqlerrors.New(surqlerrors.ErrValidation,
				"FetchHint batch_size required when strategy is batch")
		}
		hint, err := FetchBatchHint(*batchSize)
		if err != nil {
			return Query{}, err
		}
		return q.Hint(hint), nil
	default:
		return Query{}, surqlerrors.Newf(surqlerrors.ErrValidation,
			"unknown fetch strategy: %q", strategy)
	}
}

// Explain adds an ExplainHint. `full=true` asks for the full plan.
func (q Query) Explain(full bool) Query {
	if full {
		return q.Hint(ExplainFull())
	}
	return q.Hint(ExplainShort())
}

// ---------------------------------------------------------------------------
// Rendering
// ---------------------------------------------------------------------------

// ToSurql renders the Query to a SurrealQL string. Returns ErrValidation
// when the query is incomplete or internally inconsistent.
func (q Query) ToSurql() (string, error) {
	if q.Operation == "" {
		return "", surqlerrors.New(surqlerrors.ErrValidation, "Query operation not specified")
	}
	var (
		base string
		err  error
	)
	switch q.Operation {
	case OpSelect:
		base, err = q.buildSelect()
	case OpInsert:
		base, err = q.buildInsert()
	case OpUpdate:
		base, err = q.buildUpdate()
	case OpDelete:
		base, err = q.buildDelete()
	case OpUpsert:
		base, err = q.buildUpsert()
	case OpRelate:
		base, err = q.buildRelate()
	default:
		return "", surqlerrors.Newf(surqlerrors.ErrValidation, "Unsupported operation: %s", q.Operation)
	}
	if err != nil {
		return "", err
	}
	if len(q.Hints) > 0 {
		return RenderHints(q.Hints) + "\n" + base, nil
	}
	return base, nil
}

// MustToSurql is ToSurql that panics on error. Convenient for tests.
func (q Query) MustToSurql() string {
	s, err := q.ToSurql()
	if err != nil {
		panic(err)
	}
	return s
}

func (q Query) buildSelect() (string, error) {
	if q.TableName == "" {
		return "", surqlerrors.New(surqlerrors.ErrValidation, "Table name required for SELECT query")
	}
	fieldsStr := "*"
	if len(q.Fields) > 0 {
		fieldsStr = strings.Join(q.Fields, ", ")
	}
	head := "SELECT " + fieldsStr + " FROM " + q.TableName
	if q.GraphTraversal != "" {
		head += q.GraphTraversal
	}
	parts := []string{head}
	parts = append(parts, q.JoinClauses...)

	var whereParts []string
	if q.VectorField != "" && len(q.VectorValue) > 0 && q.VectorK != nil && q.VectorDistance != "" {
		vec := renderVectorLiteral(q.VectorValue)
		var op string
		if q.VectorThreshold != nil {
			op = fmt.Sprintf("<|%d,%s,%s|>", *q.VectorK, string(q.VectorDistance), formatFloat(*q.VectorThreshold))
		} else {
			op = fmt.Sprintf("<|%d,%s|>", *q.VectorK, string(q.VectorDistance))
		}
		whereParts = append(whereParts, fmt.Sprintf("%s %s %s", q.VectorField, op, vec))
	}
	if q.FulltextSet && q.FulltextField != "" && q.FulltextQuery != "" {
		quoted := quoteValueExpr(q.FulltextQuery)
		whereParts = append(whereParts, fmt.Sprintf("%s @%d@ %s", q.FulltextField, q.FulltextReference, quoted))
	}
	for _, c := range q.Conditions {
		whereParts = append(whereParts, "("+c+")")
	}
	if len(whereParts) > 0 {
		parts = append(parts, "WHERE "+strings.Join(whereParts, " AND "))
	}

	if q.GroupAllFlag {
		parts = append(parts, "GROUP ALL")
	} else if len(q.GroupFields) > 0 {
		parts = append(parts, "GROUP BY "+strings.Join(q.GroupFields, ", "))
	}

	if len(q.OrderFields) > 0 {
		ordered := make([]string, len(q.OrderFields))
		for i, of := range q.OrderFields {
			ordered[i] = of.Field + " " + of.Direction
		}
		parts = append(parts, "ORDER BY "+strings.Join(ordered, ", "))
	}

	if q.LimitValue != nil {
		parts = append(parts, "LIMIT "+strconv.Itoa(*q.LimitValue))
	}
	if q.OffsetValue != nil {
		parts = append(parts, "START "+strconv.Itoa(*q.OffsetValue))
	}
	return strings.Join(parts, " "), nil
}

func (q Query) buildInsert() (string, error) {
	if q.TableName == "" {
		return "", surqlerrors.New(surqlerrors.ErrValidation, "Table name required for INSERT query")
	}
	if len(q.InsertData) == 0 {
		return "", surqlerrors.New(surqlerrors.ErrValidation, "Insert data required for INSERT query")
	}
	body := renderDataObject(q.InsertData)
	parts := []string{"CREATE " + q.TableName + " CONTENT " + body}
	if q.ReturnFormat != "" {
		parts = append(parts, "RETURN "+q.ReturnFormat.ToSurql())
	}
	return strings.Join(parts, " "), nil
}

func (q Query) buildUpdate() (string, error) {
	if q.TableName == "" {
		return "", surqlerrors.New(surqlerrors.ErrValidation, "Table name required for UPDATE query")
	}
	if len(q.UpdateData) == 0 {
		return "", surqlerrors.New(surqlerrors.ErrValidation, "Update data required for UPDATE query")
	}
	keys := sortedKeys(q.UpdateData)
	setParts := make([]string, 0, len(keys))
	for _, k := range keys {
		setParts = append(setParts, k+" = "+quoteValueExpr(q.UpdateData[k]))
	}
	parts := []string{"UPDATE " + q.TableName + " SET " + strings.Join(setParts, ", ")}
	if len(q.Conditions) > 0 {
		parts = append(parts, "WHERE "+joinConditions(q.Conditions))
	}
	if q.ReturnFormat != "" {
		parts = append(parts, "RETURN "+q.ReturnFormat.ToSurql())
	}
	return strings.Join(parts, " "), nil
}

func (q Query) buildUpsert() (string, error) {
	if q.TableName == "" {
		return "", surqlerrors.New(surqlerrors.ErrValidation, "Table name required for UPSERT query")
	}
	if len(q.UpdateData) == 0 {
		return "", surqlerrors.New(surqlerrors.ErrValidation, "Data required for UPSERT query")
	}
	body := renderDataObject(q.UpdateData)
	parts := []string{"UPSERT " + q.TableName + " CONTENT " + body}
	if len(q.Conditions) > 0 {
		parts = append(parts, "WHERE "+joinConditions(q.Conditions))
	}
	if q.ReturnFormat != "" {
		parts = append(parts, "RETURN "+q.ReturnFormat.ToSurql())
	}
	return strings.Join(parts, " "), nil
}

func (q Query) buildDelete() (string, error) {
	if q.TableName == "" {
		return "", surqlerrors.New(surqlerrors.ErrValidation, "Table name required for DELETE query")
	}
	parts := []string{"DELETE " + q.TableName}
	if len(q.Conditions) > 0 {
		parts = append(parts, "WHERE "+joinConditions(q.Conditions))
	}
	if q.ReturnFormat != "" {
		parts = append(parts, "RETURN "+q.ReturnFormat.ToSurql())
	}
	return strings.Join(parts, " "), nil
}

func (q Query) buildRelate() (string, error) {
	if q.TableName == "" {
		return "", surqlerrors.New(surqlerrors.ErrValidation, "Edge table name required for RELATE query")
	}
	if q.RelateFrom == "" || q.RelateTo == "" {
		return "", surqlerrors.New(surqlerrors.ErrValidation, "From and to records required for RELATE query")
	}
	head := fmt.Sprintf("RELATE %s->%s->%s", q.RelateFrom, q.TableName, q.RelateTo)
	parts := []string{head}
	if len(q.RelateData) > 0 {
		parts = append(parts, "CONTENT "+renderDataObject(q.RelateData))
	}
	if q.ReturnFormat != "" {
		parts = append(parts, "RETURN "+q.ReturnFormat.ToSurql())
	}
	return strings.Join(parts, " "), nil
}

// ---------------------------------------------------------------------------
// Shared rendering helpers
// ---------------------------------------------------------------------------

func joinConditions(conds []string) string {
	wrapped := make([]string, len(conds))
	for i, c := range conds {
		wrapped[i] = "(" + c + ")"
	}
	return strings.Join(wrapped, " AND ")
}

// renderDataObject renders a map as `{k: v, ...}` with keys sorted so
// that the output is deterministic across runs.
func renderDataObject(m map[string]any) string {
	keys := sortedKeys(m)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+": "+quoteValueExpr(m[k]))
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func renderVectorLiteral(vec []float64) string {
	parts := make([]string, len(vec))
	for i, v := range vec {
		parts[i] = formatFloat(v)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// validateDataKeys checks every key in a user-supplied map is a safe
// identifier.
func validateDataKeys(data map[string]any) error {
	for k := range data {
		if err := validateIdentifier(k, "field name"); err != nil {
			return err
		}
	}
	return nil
}

// relateEndpoint normalizes a from/to endpoint into its string form.
func relateEndpoint(v any) (string, error) {
	switch x := v.(type) {
	case string:
		if x == "" {
			return "", surqlerrors.New(surqlerrors.ErrValidation, "relate endpoint cannot be empty")
		}
		return x, nil
	case types.RecordID:
		return x.String(), nil
	case fmt.Stringer:
		s := x.String()
		if s == "" {
			return "", surqlerrors.New(surqlerrors.ErrValidation, "relate endpoint cannot be empty")
		}
		return s, nil
	case nil:
		return "", surqlerrors.New(surqlerrors.ErrValidation, "relate endpoint cannot be nil")
	default:
		return "", surqlerrors.Newf(surqlerrors.ErrValidation,
			"relate endpoint must be string or types.RecordID, got %T", v)
	}
}
