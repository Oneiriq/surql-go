package migration

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
	"github.com/Oneiriq/surql-go/pkg/surql/schema"
)

// safeDefaultPattern mirrors the Python _SAFE_DEFAULT_PATTERN. It matches
// SurrealDB default-value expressions that are safe to interpolate into a
// DEFINE FIELD statement or a backfill UPDATE:
//
//   - function calls (e.g. time::now(), rand::uuid())
//   - numeric literals (42, 3.14, -1)
//   - boolean literals (true, false)
//   - NONE / NULL sentinels
//   - single-quoted strings
//   - parameter references ($session_id)
var safeDefaultPattern = regexp.MustCompile(
	`^(` +
		`[a-zA-Z_][a-zA-Z0-9_]*(?:::[a-zA-Z_][a-zA-Z0-9_]*)*\([^;]*\)` + // function calls
		`|-?\d+(?:\.\d+)?` + // numeric literals
		`|true|false` + // boolean literals
		`|NONE|NULL` + // null sentinels
		`|'(?:[^'\\]|\\.)*'` + // single-quoted strings
		`|\$[a-zA-Z_][a-zA-Z0-9_]*` + // parameter references
		`)$`,
)

// SchemaSnapshot is a point-in-time view of a SurrealDB schema.
//
// In its simplest form (empty Version / Timestamp / Description / Accesses)
// it is the whole-schema comparison input consumed by DiffSchemas; the Tables
// and Edges slices drive the code-vs-db comparison.
//
// When used via the versioning API (CreateSnapshot / StoreSnapshot /
// LoadSnapshot) the remaining fields record snapshot provenance:
//
//   - Version identifies the snapshot (YYYYMMDD_HHMMSS timestamp).
//   - Timestamp is the UTC creation time.
//   - Description is a human-readable summary.
//   - Accesses holds DEFINE ACCESS definitions present at this point.
//
// The type is JSON-serialisable; fields with no semantic content are emitted
// with `omitempty` so the DiffSchemas contract is preserved.
type SchemaSnapshot struct {
	Version     string                    `json:"version,omitempty"`
	Timestamp   time.Time                 `json:"timestamp,omitempty"`
	Description string                    `json:"description,omitempty"`
	Tables      []schema.TableDefinition  `json:"tables,omitempty"`
	Edges       []schema.EdgeDefinition   `json:"edges,omitempty"`
	Accesses    []schema.AccessDefinition `json:"accesses,omitempty"`
}

// validateEventExpression rejects event condition or action expressions that
// smell like SQL injection attempts (statement separators, SQL comments).
func validateEventExpression(expr, label string) error {
	stripped := strings.TrimSpace(expr)
	if strings.Contains(stripped, "; ") ||
		strings.Contains(stripped, ";--") ||
		strings.HasSuffix(stripped, ";") {
		return surqlerrors.Newf(surqlerrors.ErrValidation,
			"unsafe event %s: %q must not contain statement separators", label, expr)
	}
	if strings.Contains(stripped, "--") {
		return surqlerrors.Newf(surqlerrors.ErrValidation,
			"unsafe event %s: %q must not contain SQL comments", label, expr)
	}
	return nil
}

// validateDefaultValue ensures a field default / value expression matches the
// safe-default allow-list. Returns ErrValidation when the default could be
// used as an SQL injection vector.
func validateDefaultValue(expr string) error {
	if !safeDefaultPattern.MatchString(strings.TrimSpace(expr)) {
		return surqlerrors.Newf(surqlerrors.ErrValidation,
			"unsafe default value expression: %q; defaults must be function calls, literals, or parameter references",
			expr)
	}
	return nil
}

// DiffTables compares two TableDefinition snapshots and returns the list of
// SchemaDiff operations needed to evolve old -> new.
//
// Passing nil for old produces ADD diffs (table, fields, indexes, events,
// permissions) while nil for new yields a single DROP diff. When both are
// non-nil, field / index / event / permission diffs are produced.
func DiffTables(oldTable, newTable *schema.TableDefinition) ([]SchemaDiff, error) {
	switch {
	case oldTable == nil && newTable != nil:
		return generateAddTableDiffs(*newTable)
	case oldTable != nil && newTable == nil:
		return []SchemaDiff{generateDropTableDiff(*oldTable)}, nil
	case oldTable != nil && newTable != nil:
		diffs := make([]SchemaDiff, 0)
		fieldDiffs, err := DiffFields(*oldTable, *newTable)
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, fieldDiffs...)

		idxDiffs, err := DiffIndexes(*oldTable, *newTable)
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, idxDiffs...)

		evtDiffs, err := DiffEvents(*oldTable, *newTable)
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, evtDiffs...)

		diffs = append(diffs, DiffPermissions(*oldTable, *newTable)...)
		return diffs, nil
	}
	return nil, nil
}

// DiffFields computes per-field differences between two tables. The diff uses
// structural field equality (name, type, assertion, default, value, readonly,
// flexible) and produces ADD / DROP / MODIFY entries in a stable order
// (additions in new-table order, drops in old-table order, modifications in
// sorted field-name order).
func DiffFields(oldTable, newTable schema.TableDefinition) ([]SchemaDiff, error) {
	oldFields := make(map[string]schema.FieldDefinition, len(oldTable.Fields))
	for _, f := range oldTable.Fields {
		oldFields[f.Name] = f
	}
	newFields := make(map[string]schema.FieldDefinition, len(newTable.Fields))
	for _, f := range newTable.Fields {
		newFields[f.Name] = f
	}

	diffs := make([]SchemaDiff, 0)

	// Added: iterate in new-table order for determinism.
	for _, f := range newTable.Fields {
		if _, ok := oldFields[f.Name]; ok {
			continue
		}
		d, err := generateAddFieldDiff(newTable.Name, f)
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, d)
	}

	// Dropped: iterate in old-table order.
	for _, f := range oldTable.Fields {
		if _, ok := newFields[f.Name]; ok {
			continue
		}
		d, err := generateDropFieldDiff(newTable.Name, f)
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, d)
	}

	// Modified: intersection in sorted order for deterministic output.
	intersect := make([]string, 0)
	for name := range oldFields {
		if _, ok := newFields[name]; ok {
			intersect = append(intersect, name)
		}
	}
	sort.Strings(intersect)
	for _, name := range intersect {
		oldF := oldFields[name]
		newF := newFields[name]
		if fieldsEqual(oldF, newF) {
			continue
		}
		d, err := generateModifyFieldDiff(newTable.Name, oldF, newF)
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, d)
	}

	return diffs, nil
}

// DiffIndexes computes per-index differences between two tables. Indexes are
// keyed by name; MTREE and HNSW flavours produce their specialised DEFINE
// INDEX statements.
func DiffIndexes(oldTable, newTable schema.TableDefinition) ([]SchemaDiff, error) {
	oldIdx := make(map[string]schema.IndexDefinition, len(oldTable.Indexes))
	for _, i := range oldTable.Indexes {
		oldIdx[i.Name] = i
	}
	newIdx := make(map[string]schema.IndexDefinition, len(newTable.Indexes))
	for _, i := range newTable.Indexes {
		newIdx[i.Name] = i
	}

	diffs := make([]SchemaDiff, 0)

	for _, idx := range newTable.Indexes {
		if _, ok := oldIdx[idx.Name]; ok {
			continue
		}
		d, err := generateAddIndexDiff(newTable.Name, idx)
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, d)
	}

	for _, idx := range oldTable.Indexes {
		if _, ok := newIdx[idx.Name]; ok {
			continue
		}
		d, err := generateDropIndexDiff(newTable.Name, idx)
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, d)
	}

	return diffs, nil
}

// DiffEvents computes per-event differences between two tables. Event
// expressions are validated against the safe-expression allow-list; an unsafe
// expression returns ErrValidation.
func DiffEvents(oldTable, newTable schema.TableDefinition) ([]SchemaDiff, error) {
	oldEv := make(map[string]schema.EventDefinition, len(oldTable.Events))
	for _, e := range oldTable.Events {
		oldEv[e.Name] = e
	}
	newEv := make(map[string]schema.EventDefinition, len(newTable.Events))
	for _, e := range newTable.Events {
		newEv[e.Name] = e
	}

	diffs := make([]SchemaDiff, 0)

	for _, ev := range newTable.Events {
		if _, ok := oldEv[ev.Name]; ok {
			continue
		}
		d, err := generateAddEventDiff(newTable.Name, ev)
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, d)
	}

	for _, ev := range oldTable.Events {
		if _, ok := newEv[ev.Name]; ok {
			continue
		}
		d, err := generateDropEventDiff(newTable.Name, ev)
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, d)
	}

	return diffs, nil
}

// DiffPermissions compares the permission map of two tables and returns a
// single MODIFY_PERMISSIONS diff when they differ (or an empty slice when
// they match). Forward SQL reflects the new permissions; backward SQL the
// old. The empty map and a nil map compare equal.
func DiffPermissions(oldTable, newTable schema.TableDefinition) []SchemaDiff {
	if permissionsEqual(oldTable.Permissions, newTable.Permissions) {
		return nil
	}
	return []SchemaDiff{
		generateModifyPermissionsDiff(newTable.Name, newTable.Permissions, oldTable.Permissions),
	}
}

// DiffEdges compares two EdgeDefinition snapshots by delegating the field,
// index, event, and permission comparisons to the table diff helpers via a
// TableDefinition adapter. Adding or dropping an edge produces the full
// suite of child diffs (not just a DROP TABLE).
func DiffEdges(oldEdge, newEdge *schema.EdgeDefinition) ([]SchemaDiff, error) {
	switch {
	case oldEdge == nil && newEdge != nil:
		return generateAddEdgeDiffs(*newEdge)
	case oldEdge != nil && newEdge == nil:
		return []SchemaDiff{generateDropEdgeDiff(*oldEdge)}, nil
	case oldEdge != nil && newEdge != nil:
		oldProxy := edgeToTableProxy(*oldEdge)
		newProxy := edgeToTableProxy(*newEdge)

		diffs := make([]SchemaDiff, 0)
		fieldDiffs, err := DiffFields(oldProxy, newProxy)
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, fieldDiffs...)

		idxDiffs, err := DiffIndexes(oldProxy, newProxy)
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, idxDiffs...)

		evtDiffs, err := DiffEvents(oldProxy, newProxy)
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, evtDiffs...)

		diffs = append(diffs, DiffPermissions(oldProxy, newProxy)...)
		return diffs, nil
	}
	return nil, nil
}

// DiffSchemas aggregates DiffTables and DiffEdges across two whole snapshots.
// Tables and edges are matched by name; the "code" side is treated as the
// desired state and the "db" side as the current state, so ADD diffs refer
// to entries present in code but missing in db.
//
// The returned slice is ordered: added tables, dropped tables, modified
// tables (in sorted name order), then the same for edges. Within each entry
// the child ordering mirrors DiffTables / DiffEdges.
func DiffSchemas(code, db SchemaSnapshot) ([]SchemaDiff, error) {
	codeTables := make(map[string]schema.TableDefinition, len(code.Tables))
	for _, t := range code.Tables {
		codeTables[t.Name] = t
	}
	dbTables := make(map[string]schema.TableDefinition, len(db.Tables))
	for _, t := range db.Tables {
		dbTables[t.Name] = t
	}

	diffs := make([]SchemaDiff, 0)

	// Added tables (code but not db).
	for _, t := range code.Tables {
		if _, ok := dbTables[t.Name]; ok {
			continue
		}
		added, err := DiffTables(nil, tablePtr(t))
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, added...)
	}

	// Dropped tables (db but not code).
	for _, t := range db.Tables {
		if _, ok := codeTables[t.Name]; ok {
			continue
		}
		dropped, err := DiffTables(tablePtr(t), nil)
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, dropped...)
	}

	// Modified tables (present on both sides).
	intersectT := make([]string, 0)
	for name := range dbTables {
		if _, ok := codeTables[name]; ok {
			intersectT = append(intersectT, name)
		}
	}
	sort.Strings(intersectT)
	for _, name := range intersectT {
		oldT := dbTables[name]
		newT := codeTables[name]
		modified, err := DiffTables(&oldT, &newT)
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, modified...)
	}

	codeEdges := make(map[string]schema.EdgeDefinition, len(code.Edges))
	for _, e := range code.Edges {
		codeEdges[e.Name] = e
	}
	dbEdges := make(map[string]schema.EdgeDefinition, len(db.Edges))
	for _, e := range db.Edges {
		dbEdges[e.Name] = e
	}

	for _, e := range code.Edges {
		if _, ok := dbEdges[e.Name]; ok {
			continue
		}
		added, err := DiffEdges(nil, edgePtr(e))
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, added...)
	}

	for _, e := range db.Edges {
		if _, ok := codeEdges[e.Name]; ok {
			continue
		}
		dropped, err := DiffEdges(edgePtr(e), nil)
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, dropped...)
	}

	intersectE := make([]string, 0)
	for name := range dbEdges {
		if _, ok := codeEdges[name]; ok {
			intersectE = append(intersectE, name)
		}
	}
	sort.Strings(intersectE)
	for _, name := range intersectE {
		oldE := dbEdges[name]
		newE := codeEdges[name]
		modified, err := DiffEdges(&oldE, &newE)
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, modified...)
	}

	return diffs, nil
}

// --- internal helpers ---

func tablePtr(t schema.TableDefinition) *schema.TableDefinition { return &t }
func edgePtr(e schema.EdgeDefinition) *schema.EdgeDefinition    { return &e }

func edgeToTableProxy(e schema.EdgeDefinition) schema.TableDefinition {
	return schema.TableDefinition{
		Name:        e.Name,
		Fields:      append([]schema.FieldDefinition(nil), e.Fields...),
		Indexes:     append([]schema.IndexDefinition(nil), e.Indexes...),
		Events:      append([]schema.EventDefinition(nil), e.Events...),
		Permissions: copyPermissions(e.Permissions),
	}
}

func copyPermissions(p map[string]string) map[string]string {
	if p == nil {
		return nil
	}
	out := make(map[string]string, len(p))
	for k, v := range p {
		out[k] = v
	}
	return out
}

func permissionsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
}

func fieldsEqual(a, b schema.FieldDefinition) bool {
	return a.Name == b.Name &&
		a.Type == b.Type &&
		a.Assertion == b.Assertion &&
		a.Default == b.Default &&
		a.Value == b.Value &&
		a.ReadOnly == b.ReadOnly &&
		a.Flexible == b.Flexible
}

func generateAddTableDiffs(t schema.TableDefinition) ([]SchemaDiff, error) {
	mode := string(t.Mode)
	if mode == "" {
		mode = string(schema.TableModeSchemafull)
	}
	diffs := make([]SchemaDiff, 0, 1+len(t.Fields)+len(t.Indexes)+len(t.Events)+1)
	diffs = append(diffs, SchemaDiff{
		Operation:   DiffOperationAddTable,
		Table:       t.Name,
		Description: fmt.Sprintf("Add table %s", t.Name),
		ForwardSQL:  fmt.Sprintf("DEFINE TABLE %s %s;", t.Name, mode),
		BackwardSQL: fmt.Sprintf("REMOVE TABLE %s;", t.Name),
	})

	for _, f := range t.Fields {
		d, err := generateAddFieldDiff(t.Name, f)
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, d)
	}
	for _, i := range t.Indexes {
		d, err := generateAddIndexDiff(t.Name, i)
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, d)
	}
	for _, e := range t.Events {
		d, err := generateAddEventDiff(t.Name, e)
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, d)
	}
	if len(t.Permissions) > 0 {
		diffs = append(diffs, generateModifyPermissionsDiff(t.Name, t.Permissions, nil))
	}
	return diffs, nil
}

func generateDropTableDiff(t schema.TableDefinition) SchemaDiff {
	mode := string(t.Mode)
	if mode == "" {
		mode = string(schema.TableModeSchemafull)
	}
	return SchemaDiff{
		Operation:   DiffOperationDropTable,
		Table:       t.Name,
		Description: fmt.Sprintf("Drop table %s", t.Name),
		ForwardSQL:  fmt.Sprintf("REMOVE TABLE %s;", t.Name),
		BackwardSQL: fmt.Sprintf("DEFINE TABLE %s %s;", t.Name, mode),
	}
}

func generateAddFieldDiff(tableName string, f schema.FieldDefinition) (SchemaDiff, error) {
	forwardSQL, err := fieldToSQL(tableName, f)
	if err != nil {
		return SchemaDiff{}, err
	}
	if f.Default != "" {
		if err := validateDefaultValue(f.Default); err != nil {
			return SchemaDiff{}, err
		}
		forwardSQL = fmt.Sprintf(
			"%s\nUPDATE %s SET %s = %s WHERE %s IS NONE;",
			forwardSQL, tableName, f.Name, f.Default, f.Name,
		)
	}
	return SchemaDiff{
		Operation:   DiffOperationAddField,
		Table:       tableName,
		Field:       f.Name,
		Description: fmt.Sprintf("Add field %s to %s", f.Name, tableName),
		ForwardSQL:  forwardSQL,
		BackwardSQL: fmt.Sprintf("REMOVE FIELD %s ON TABLE %s;", f.Name, tableName),
		Details:     map[string]any{"type": string(f.Type)},
	}, nil
}

func generateDropFieldDiff(tableName string, f schema.FieldDefinition) (SchemaDiff, error) {
	backwardSQL, err := fieldToSQL(tableName, f)
	if err != nil {
		return SchemaDiff{}, err
	}
	return SchemaDiff{
		Operation:   DiffOperationDropField,
		Table:       tableName,
		Field:       f.Name,
		Description: fmt.Sprintf("Drop field %s from %s", f.Name, tableName),
		ForwardSQL:  fmt.Sprintf("REMOVE FIELD %s ON TABLE %s;", f.Name, tableName),
		BackwardSQL: backwardSQL,
	}, nil
}

func generateModifyFieldDiff(tableName string, oldF, newF schema.FieldDefinition) (SchemaDiff, error) {
	forwardSQL, err := fieldToSQL(tableName, newF)
	if err != nil {
		return SchemaDiff{}, err
	}
	backwardSQL, err := fieldToSQL(tableName, oldF)
	if err != nil {
		return SchemaDiff{}, err
	}
	return SchemaDiff{
		Operation:   DiffOperationModifyField,
		Table:       tableName,
		Field:       newF.Name,
		Description: fmt.Sprintf("Modify field %s in %s", newF.Name, tableName),
		ForwardSQL:  forwardSQL,
		BackwardSQL: backwardSQL,
		Details: map[string]any{
			"old_type": string(oldF.Type),
			"new_type": string(newF.Type),
		},
	}, nil
}

func generateAddIndexDiff(tableName string, idx schema.IndexDefinition) (SchemaDiff, error) {
	forwardSQL, err := indexToSQL(tableName, idx)
	if err != nil {
		return SchemaDiff{}, err
	}
	return SchemaDiff{
		Operation:   DiffOperationAddIndex,
		Table:       tableName,
		Index:       idx.Name,
		Description: fmt.Sprintf("Add index %s to %s", idx.Name, tableName),
		ForwardSQL:  forwardSQL,
		BackwardSQL: fmt.Sprintf("REMOVE INDEX %s ON TABLE %s;", idx.Name, tableName),
	}, nil
}

func generateDropIndexDiff(tableName string, idx schema.IndexDefinition) (SchemaDiff, error) {
	backwardSQL, err := indexToSQL(tableName, idx)
	if err != nil {
		return SchemaDiff{}, err
	}
	return SchemaDiff{
		Operation:   DiffOperationDropIndex,
		Table:       tableName,
		Index:       idx.Name,
		Description: fmt.Sprintf("Drop index %s from %s", idx.Name, tableName),
		ForwardSQL:  fmt.Sprintf("REMOVE INDEX %s ON TABLE %s;", idx.Name, tableName),
		BackwardSQL: backwardSQL,
	}, nil
}

func generateAddEventDiff(tableName string, ev schema.EventDefinition) (SchemaDiff, error) {
	if err := validateEventExpression(ev.Condition, "condition"); err != nil {
		return SchemaDiff{}, err
	}
	if err := validateEventExpression(ev.Action, "action"); err != nil {
		return SchemaDiff{}, err
	}
	return SchemaDiff{
		Operation: DiffOperationAddEvent,
		Table:     tableName,
		Event:     ev.Name,
		Description: fmt.Sprintf(
			"Add event %s to %s", ev.Name, tableName,
		),
		ForwardSQL: fmt.Sprintf(
			"DEFINE EVENT %s ON TABLE %s WHEN %s THEN { %s };",
			ev.Name, tableName, ev.Condition, ev.Action,
		),
		BackwardSQL: fmt.Sprintf("REMOVE EVENT %s ON TABLE %s;", ev.Name, tableName),
	}, nil
}

func generateDropEventDiff(tableName string, ev schema.EventDefinition) (SchemaDiff, error) {
	if err := validateEventExpression(ev.Condition, "condition"); err != nil {
		return SchemaDiff{}, err
	}
	if err := validateEventExpression(ev.Action, "action"); err != nil {
		return SchemaDiff{}, err
	}
	return SchemaDiff{
		Operation:   DiffOperationDropEvent,
		Table:       tableName,
		Event:       ev.Name,
		Description: fmt.Sprintf("Drop event %s from %s", ev.Name, tableName),
		ForwardSQL:  fmt.Sprintf("REMOVE EVENT %s ON TABLE %s;", ev.Name, tableName),
		BackwardSQL: fmt.Sprintf(
			"DEFINE EVENT %s ON TABLE %s WHEN %s THEN { %s };",
			ev.Name, tableName, ev.Condition, ev.Action,
		),
	}, nil
}

// generateModifyPermissionsDiff renders a single MODIFY_PERMISSIONS diff.
// Forward SQL is the concatenation of DEFINE FIELD PERMISSIONS statements for
// permissions (the desired state); backward SQL reproduces old_permissions.
// Keys are emitted in sorted order for stable output.
func generateModifyPermissionsDiff(tableName string, permissions, oldPermissions map[string]string) SchemaDiff {
	return SchemaDiff{
		Operation:   DiffOperationModifyPermissions,
		Table:       tableName,
		Description: fmt.Sprintf("Modify permissions for %s", tableName),
		ForwardSQL:  permissionsToSQL(tableName, permissions),
		BackwardSQL: permissionsToSQL(tableName, oldPermissions),
	}
}

func permissionsToSQL(tableName string, permissions map[string]string) string {
	if len(permissions) == 0 {
		return ""
	}
	keys := make([]string, 0, len(permissions))
	for k := range permissions {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, op := range keys {
		parts = append(parts, fmt.Sprintf(
			"DEFINE FIELD PERMISSIONS FOR %s ON TABLE %s WHERE %s;",
			strings.ToUpper(op), tableName, permissions[op],
		))
	}
	return strings.Join(parts, " ")
}

func generateAddEdgeDiffs(e schema.EdgeDefinition) ([]SchemaDiff, error) {
	diffs := make([]SchemaDiff, 0, 1+len(e.Fields)+len(e.Indexes)+len(e.Events))

	var forwardSQL string
	switch e.Mode {
	case schema.EdgeModeRelation:
		forwardSQL = fmt.Sprintf("DEFINE TABLE %s TYPE RELATION", e.Name)
		if e.FromTable != "" {
			forwardSQL += " FROM " + e.FromTable
		}
		if e.ToTable != "" {
			forwardSQL += " TO " + e.ToTable
		}
		forwardSQL += ";"
	case schema.EdgeModeSchemafull:
		forwardSQL = fmt.Sprintf("DEFINE TABLE %s SCHEMAFULL;", e.Name)
	case schema.EdgeModeSchemaless:
		forwardSQL = fmt.Sprintf("DEFINE TABLE %s SCHEMALESS;", e.Name)
	default:
		// Unrecognised modes default to RELATION to mirror NewEdge's default.
		forwardSQL = fmt.Sprintf("DEFINE TABLE %s TYPE RELATION;", e.Name)
	}

	diffs = append(diffs, SchemaDiff{
		Operation:   DiffOperationAddTable,
		Table:       e.Name,
		Description: fmt.Sprintf("Add edge %s", e.Name),
		ForwardSQL:  forwardSQL,
		BackwardSQL: fmt.Sprintf("REMOVE TABLE %s;", e.Name),
	})

	for _, f := range e.Fields {
		d, err := generateAddFieldDiff(e.Name, f)
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, d)
	}
	for _, i := range e.Indexes {
		d, err := generateAddIndexDiff(e.Name, i)
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, d)
	}
	for _, ev := range e.Events {
		d, err := generateAddEventDiff(e.Name, ev)
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, d)
	}
	return diffs, nil
}

func generateDropEdgeDiff(e schema.EdgeDefinition) SchemaDiff {
	return SchemaDiff{
		Operation:   DiffOperationDropTable,
		Table:       e.Name,
		Description: fmt.Sprintf("Drop edge %s", e.Name),
		ForwardSQL:  fmt.Sprintf("REMOVE TABLE %s;", e.Name),
		BackwardSQL: "",
	}
}

// fieldToSQL renders a standalone DEFINE FIELD statement matching the Python
// port. It does not call FieldDefinition.ToSurql because that helper emits
// clauses in a different order (assertion before default/value) — we preserve
// the Python ordering plus the default/value allow-list validation.
func fieldToSQL(tableName string, f schema.FieldDefinition) (string, error) {
	var b strings.Builder
	b.WriteString("DEFINE FIELD ")
	b.WriteString(f.Name)
	b.WriteString(" ON TABLE ")
	b.WriteString(tableName)
	b.WriteString(" TYPE ")
	b.WriteString(string(f.Type))

	if f.Assertion != "" {
		b.WriteString(" ASSERT ")
		b.WriteString(f.Assertion)
	}
	if f.Default != "" {
		if err := validateDefaultValue(f.Default); err != nil {
			return "", err
		}
		b.WriteString(" DEFAULT ")
		b.WriteString(f.Default)
	}
	if f.Value != "" {
		if err := validateDefaultValue(f.Value); err != nil {
			return "", err
		}
		b.WriteString(" VALUE ")
		b.WriteString(f.Value)
	}
	if f.ReadOnly {
		b.WriteString(" READONLY")
	}
	if f.Flexible {
		b.WriteString(" FLEXIBLE")
	}
	b.WriteString(";")
	return b.String(), nil
}

// indexToSQL renders a DEFINE INDEX statement. Standard / UNIQUE / SEARCH
// indexes use the basic form; MTREE and HNSW flavours route through their
// dedicated helpers. Returns ErrValidation for MTREE / HNSW indexes missing
// a dimension.
func indexToSQL(tableName string, idx schema.IndexDefinition) (string, error) {
	switch idx.Type {
	case schema.IndexTypeMTree:
		return mtreeIndexToSQL(tableName, idx)
	case schema.IndexTypeHNSW:
		return hnswIndexToSQL(tableName, idx)
	}

	var b strings.Builder
	b.WriteString("DEFINE INDEX ")
	b.WriteString(idx.Name)
	b.WriteString(" ON TABLE ")
	b.WriteString(tableName)
	b.WriteString(" COLUMNS ")
	b.WriteString(strings.Join(idx.Columns, ", "))

	if string(idx.Type) != string(schema.IndexTypeStandard) && idx.Type != "" {
		b.WriteString(" ")
		b.WriteString(string(idx.Type))
	}
	b.WriteString(";")
	return b.String(), nil
}

func mtreeIndexToSQL(tableName string, idx schema.IndexDefinition) (string, error) {
	if idx.Dimension <= 0 {
		return "", surqlerrors.Newf(surqlerrors.ErrValidation,
			"MTREE index %q must have dimension specified", idx.Name)
	}
	field := ""
	if len(idx.Columns) > 0 {
		field = idx.Columns[0]
	}

	var b strings.Builder
	b.WriteString("DEFINE INDEX ")
	b.WriteString(idx.Name)
	b.WriteString(" ON TABLE ")
	b.WriteString(tableName)
	b.WriteString(" COLUMNS ")
	b.WriteString(field)
	b.WriteString(" MTREE DIMENSION ")
	b.WriteString(strconv.Itoa(idx.Dimension))
	if idx.Distance != "" {
		b.WriteString(" DIST ")
		b.WriteString(string(idx.Distance))
	}
	if idx.VectorType != "" {
		b.WriteString(" TYPE ")
		b.WriteString(string(idx.VectorType))
	}
	b.WriteString(";")
	return b.String(), nil
}

func hnswIndexToSQL(tableName string, idx schema.IndexDefinition) (string, error) {
	if idx.Dimension <= 0 {
		return "", surqlerrors.Newf(surqlerrors.ErrValidation,
			"HNSW index %q must have dimension specified", idx.Name)
	}
	field := ""
	if len(idx.Columns) > 0 {
		field = idx.Columns[0]
	}

	var b strings.Builder
	b.WriteString("DEFINE INDEX ")
	b.WriteString(idx.Name)
	b.WriteString(" ON TABLE ")
	b.WriteString(tableName)
	b.WriteString(" COLUMNS ")
	b.WriteString(field)
	b.WriteString(" HNSW DIMENSION ")
	b.WriteString(strconv.Itoa(idx.Dimension))
	if idx.HnswDistance != "" {
		b.WriteString(" DIST ")
		b.WriteString(string(idx.HnswDistance))
	}
	if idx.VectorType != "" {
		b.WriteString(" TYPE ")
		b.WriteString(string(idx.VectorType))
	}
	if idx.EFC > 0 {
		b.WriteString(" EFC ")
		b.WriteString(strconv.Itoa(idx.EFC))
	}
	if idx.M > 0 {
		b.WriteString(" M ")
		b.WriteString(strconv.Itoa(idx.M))
	}
	b.WriteString(";")
	return b.String(), nil
}
