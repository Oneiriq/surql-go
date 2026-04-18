package schema

import (
	"strings"

	surqlerrors "github.com/albedosehen/surql-go/pkg/surql/errors"
)

// GenerateTableSQL emits the full list of DEFINE statements for a table: the
// DEFINE TABLE line followed by each DEFINE FIELD / INDEX / EVENT, plus any
// DEFINE FIELD PERMISSIONS rendered from the permission map.
//
// It mirrors surql-py's generate_table_sql and delegates to the TableDefinition
// ToSurqlStatements* methods for deterministic output (permissions are sorted
// by action name).
//
// When ifNotExists is true, every DEFINE statement that supports it is emitted
// with an IF NOT EXISTS clause.
func GenerateTableSQL(table TableDefinition, ifNotExists bool) []string {
	if ifNotExists {
		return table.ToSurqlStatementsIfNotExists()
	}
	return table.ToSurqlStatements()
}

// GenerateEdgeSQL emits the full list of DEFINE statements for an edge table.
// It returns an ErrValidation error when the edge has RELATION mode without
// both from_table and to_table set (matching surql-py behaviour).
func GenerateEdgeSQL(edge EdgeDefinition, ifNotExists bool) ([]string, error) {
	if ifNotExists {
		return edge.ToSurqlStatementsIfNotExists()
	}
	return edge.ToSurqlStatements()
}

// GenerateAccessSQL emits the DEFINE ACCESS statement for an access definition.
// It always returns a single-element slice to match the surql-py signature.
func GenerateAccessSQL(access AccessDefinition) []string {
	return []string{access.ToSurql()}
}

// GenerateSchemaSQL composes a full SurrealQL schema script from every table
// and edge registered in r. Tables are emitted first (in sorted order),
// followed by edges (also sorted). Each definition group is separated by a
// blank line for readability, matching the surql-py output format.
//
// A nil registry produces an empty script. When ifNotExists is true, every
// emitted DEFINE statement includes IF NOT EXISTS.
//
// GenerateSchemaSQL surfaces the same validation errors as GenerateEdgeSQL:
// an edge registered in RELATION mode without from/to tables yields an
// ErrValidation error.
func GenerateSchemaSQL(r *SchemaRegistry, ifNotExists bool) (string, error) {
	if r == nil {
		return "", nil
	}

	tables := r.Tables()
	edges := r.Edges()

	return generateSchemaScript(tables, edges, ifNotExists)
}

// GenerateSchemaSQLFromSlices composes a full SurrealQL schema script from the
// supplied table and edge slices without consulting any registry. Inputs are
// emitted in the order provided (callers sort first if determinism is
// required).
func GenerateSchemaSQLFromSlices(tables []TableDefinition, edges []EdgeDefinition, ifNotExists bool) (string, error) {
	return generateSchemaScript(tables, edges, ifNotExists)
}

func generateSchemaScript(tables []TableDefinition, edges []EdgeDefinition, ifNotExists bool) (string, error) {
	if len(tables) == 0 && len(edges) == 0 {
		return "", nil
	}

	stmts := make([]string, 0, len(tables)*3+len(edges)*3)

	for _, t := range tables {
		stmts = append(stmts, GenerateTableSQL(t, ifNotExists)...)
		stmts = append(stmts, "")
	}

	for _, e := range edges {
		edgeStmts, err := GenerateEdgeSQL(e, ifNotExists)
		if err != nil {
			return "", surqlerrors.Wrapf(surqlerrors.ErrValidation, err,
				"failed to generate SQL for edge %q", e.Name)
		}
		stmts = append(stmts, edgeStmts...)
		stmts = append(stmts, "")
	}

	return strings.TrimRight(strings.Join(stmts, "\n"), "\n"), nil
}
