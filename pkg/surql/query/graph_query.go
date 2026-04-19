package query

import (
	"context"
	"fmt"
	"strings"

	"github.com/Oneiriq/surql-go/pkg/surql/connection"
	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

// GraphQuery is a chainable builder for graph traversal SELECT
// statements. It is a 1:1 port of surql-py's GraphQuery and mirrors
// the same surface: Out/In/Both/To to compose the path, Where/Select/
// Limit/Fetch to shape the projection, Count/Exists for aggregates.
//
// Unlike the typed Query builder in this package the builder mutates
// its receiver in place — callers wanting immutability can Clone the
// builder before each branch. This matches the Python implementation,
// which is a dataclass with mutating methods.
type GraphQuery struct {
	start       string
	path        []string
	conditions  []string
	limit       *int
	fields      []string
	fetchRefs   []string
	targetTable string
}

// NewGraphQuery opens a builder rooted at `start`. `start` may be a
// bare record id ("user:alice"), a table name ("user"), or a SurrealQL
// target fragment.
func NewGraphQuery(start string) *GraphQuery {
	return &GraphQuery{
		start:      start,
		path:       []string{},
		conditions: []string{},
		fields:     []string{},
		fetchRefs:  []string{},
	}
}

// Clone returns an independent copy of g. Handy when branching off a
// partially-built query without mutating the original.
func (g *GraphQuery) Clone() *GraphQuery {
	if g == nil {
		return nil
	}
	cp := &GraphQuery{
		start:       g.start,
		path:        append([]string(nil), g.path...),
		conditions:  append([]string(nil), g.conditions...),
		fields:      append([]string(nil), g.fields...),
		fetchRefs:   append([]string(nil), g.fetchRefs...),
		targetTable: g.targetTable,
	}
	if g.limit != nil {
		v := *g.limit
		cp.limit = &v
	}
	return cp
}

// Out appends an outgoing edge step (`->edge[depth]`). Pass nil for
// depth to emit the edge without a repetition count.
func (g *GraphQuery) Out(edge string, depth *int) *GraphQuery {
	g.path = append(g.path, formatEdgeStep("->", edge, depth))
	return g
}

// In appends an incoming edge step (`<-edge[depth]`).
func (g *GraphQuery) In(edge string, depth *int) *GraphQuery {
	g.path = append(g.path, formatEdgeStep("<-", edge, depth))
	return g
}

// Both appends a bidirectional step (`<->edge[depth]`).
func (g *GraphQuery) Both(edge string, depth *int) *GraphQuery {
	g.path = append(g.path, formatEdgeStep("<->", edge, depth))
	return g
}

// To pins the final hop to a specific target table, appended as
// `->target` on render.
func (g *GraphQuery) To(table string) *GraphQuery {
	g.targetTable = table
	return g
}

// Where appends a WHERE condition. Multiple calls AND their operands.
func (g *GraphQuery) Where(condition string) *GraphQuery {
	if condition == "" {
		return g
	}
	g.conditions = append(g.conditions, condition)
	return g
}

// Select narrows the projection. Calling Select("") (no fields) leaves
// the query as `SELECT *`.
func (g *GraphQuery) Select(fields ...string) *GraphQuery {
	g.fields = append(g.fields, fields...)
	return g
}

// Limit caps the number of rows returned. Negative values are
// rejected by ToSurql at render time.
func (g *GraphQuery) Limit(n int) *GraphQuery {
	g.limit = &n
	return g
}

// Fetch appends record references to the trailing `FETCH` clause,
// matching SurrealDB's eager-load syntax. This is a pure parity
// addition over surql-py; the Python port does not expose it but the
// executor + builder already emit FETCH for the typed Query builder.
func (g *GraphQuery) Fetch(refs ...string) *GraphQuery {
	g.fetchRefs = append(g.fetchRefs, refs...)
	return g
}

// ToSurql renders the builder to a SurrealQL SELECT statement plus an
// empty vars map.
//
// Returns ErrValidation when the builder is incomplete (no start, no
// path steps) or when Limit was set to a negative value.
func (g *GraphQuery) ToSurql() (string, map[string]any, error) {
	if g == nil {
		return "", nil, surqlerrors.New(surqlerrors.ErrValidation, "graph query cannot be nil")
	}
	if g.start == "" {
		return "", nil, surqlerrors.New(surqlerrors.ErrValidation, "graph query requires a start record")
	}
	if len(g.path) == 0 {
		return "", nil, surqlerrors.New(
			surqlerrors.ErrValidation,
			"graph query requires at least one traversal step (Out/In/Both)",
		)
	}
	if g.limit != nil && *g.limit < 0 {
		return "", nil, surqlerrors.Newf(
			surqlerrors.ErrValidation,
			"limit must be non-negative, got %d", *g.limit,
		)
	}

	fields := "*"
	if len(g.fields) > 0 {
		fields = strings.Join(g.fields, ", ")
	}

	path := buildGraphPath(g.path)
	if g.targetTable != "" {
		path = path + "->" + g.targetTable
	}

	var b strings.Builder
	fmt.Fprintf(&b, "SELECT %s FROM %s%s", fields, g.start, path)

	if len(g.conditions) > 0 {
		parts := make([]string, len(g.conditions))
		for i, c := range g.conditions {
			parts[i] = "(" + c + ")"
		}
		b.WriteString(" WHERE ")
		b.WriteString(strings.Join(parts, " AND "))
	}

	if len(g.fetchRefs) > 0 {
		b.WriteString(" FETCH ")
		b.WriteString(strings.Join(g.fetchRefs, ", "))
	}

	if g.limit != nil {
		fmt.Fprintf(&b, " LIMIT %d", *g.limit)
	}

	return b.String(), map[string]any{}, nil
}

// Execute dispatches the built query against client and returns the
// decoded rows. Mirrors surql-py's fetch() on GraphQuery.
func (g *GraphQuery) Execute(ctx context.Context, client *connection.DatabaseClient) ([]map[string]any, error) {
	if client == nil {
		return nil, surqlerrors.New(surqlerrors.ErrValidation, "client cannot be nil")
	}
	surql, vars, err := g.ToSurql()
	if err != nil {
		return nil, err
	}
	raw, err := client.QueryWithVars(ctx, surql, vars)
	if err != nil {
		if isTableMissingError(err) {
			return nil, nil
		}
		return nil, err
	}
	return ExtractResult(raw), nil
}

// Count executes the query with a `count() GROUP ALL` projection and
// returns the aggregate. Mirrors surql-py's count().
//
// The path, WHERE conditions and target table are reused verbatim;
// the Select / Limit / Fetch clauses are intentionally ignored so the
// aggregate reflects the full result set.
func (g *GraphQuery) Count(ctx context.Context, client *connection.DatabaseClient) (int64, error) {
	if client == nil {
		return 0, surqlerrors.New(surqlerrors.ErrValidation, "client cannot be nil")
	}
	if g == nil || g.start == "" {
		return 0, surqlerrors.New(surqlerrors.ErrValidation, "graph query requires a start record")
	}
	if len(g.path) == 0 {
		return 0, surqlerrors.New(
			surqlerrors.ErrValidation,
			"graph query requires at least one traversal step (Out/In/Both)",
		)
	}
	path := buildGraphPath(g.path)
	if g.targetTable != "" {
		path = path + "->" + g.targetTable
	}
	var b strings.Builder
	fmt.Fprintf(&b, "SELECT count() FROM %s%s", g.start, path)
	if len(g.conditions) > 0 {
		parts := make([]string, len(g.conditions))
		for i, c := range g.conditions {
			parts[i] = "(" + c + ")"
		}
		b.WriteString(" WHERE ")
		b.WriteString(strings.Join(parts, " AND "))
	}
	b.WriteString(" GROUP ALL;")
	raw, err := client.QueryWithVars(ctx, b.String(), map[string]any{})
	if err != nil {
		if isTableMissingError(err) {
			return 0, nil
		}
		return 0, err
	}
	first := ExtractOne(raw)
	if first == nil {
		return 0, nil
	}
	if v, ok := first["count"]; ok {
		return toInt64(v), nil
	}
	return 0, nil
}

// Exists reports whether any row matches the query. A thin wrapper
// around Count.
func (g *GraphQuery) Exists(ctx context.Context, client *connection.DatabaseClient) (bool, error) {
	n, err := g.Count(ctx, client)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// formatEdgeStep renders a single `arrow + edge[depth]` segment.
func formatEdgeStep(arrow, edge string, depth *int) string {
	if depth == nil {
		return arrow + edge
	}
	return fmt.Sprintf("%s%s%d", arrow, edge, *depth)
}
