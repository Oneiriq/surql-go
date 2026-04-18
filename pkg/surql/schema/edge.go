package schema

import (
	"sort"
	"strings"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

// EdgeMode enumerates edge table modes.
type EdgeMode string

// EdgeMode values.
const (
	EdgeModeRelation   EdgeMode = "RELATION"
	EdgeModeSchemafull EdgeMode = "SCHEMAFULL"
	EdgeModeSchemaless EdgeMode = "SCHEMALESS"
)

// IsValid reports whether the EdgeMode is recognised.
func (m EdgeMode) IsValid() bool {
	switch m {
	case EdgeModeRelation, EdgeModeSchemafull, EdgeModeSchemaless:
		return true
	}
	return false
}

// EdgeDefinition captures the fields required to emit a DEFINE TABLE ... TYPE
// RELATION statement (or a SCHEMAFULL / SCHEMALESS edge) plus its attendant
// DEFINE FIELD / INDEX / EVENT children.
type EdgeDefinition struct {
	Name        string
	Mode        EdgeMode
	FromTable   string
	ToTable     string
	Fields      []FieldDefinition
	Indexes     []IndexDefinition
	Events      []EventDefinition
	Permissions map[string]string
}

// EdgeOption customises an EdgeDefinition created via NewEdge.
type EdgeOption func(*EdgeDefinition)

// WithEdgeMode sets the edge table mode.
func WithEdgeMode(mode EdgeMode) EdgeOption {
	return func(e *EdgeDefinition) { e.Mode = mode }
}

// WithFromTable sets the source table constraint.
func WithFromTable(table string) EdgeOption {
	return func(e *EdgeDefinition) { e.FromTable = table }
}

// WithToTable sets the destination table constraint.
func WithToTable(table string) EdgeOption {
	return func(e *EdgeDefinition) { e.ToTable = table }
}

// WithEdgeFields appends fields.
func WithEdgeFields(fields ...FieldDefinition) EdgeOption {
	return func(e *EdgeDefinition) { e.Fields = append(e.Fields, fields...) }
}

// WithEdgeIndexes appends indexes.
func WithEdgeIndexes(indexes ...IndexDefinition) EdgeOption {
	return func(e *EdgeDefinition) { e.Indexes = append(e.Indexes, indexes...) }
}

// WithEdgeEvents appends events.
func WithEdgeEvents(events ...EventDefinition) EdgeOption {
	return func(e *EdgeDefinition) { e.Events = append(e.Events, events...) }
}

// WithEdgePermissions sets permissions for the edge table.
func WithEdgePermissions(perms map[string]string) EdgeOption {
	return func(e *EdgeDefinition) {
		if perms == nil {
			e.Permissions = nil
			return
		}
		copied := make(map[string]string, len(perms))
		for k, v := range perms {
			copied[k] = v
		}
		e.Permissions = copied
	}
}

// NewEdge constructs an EdgeDefinition with EdgeModeRelation as default.
func NewEdge(name string, opts ...EdgeOption) EdgeDefinition {
	e := EdgeDefinition{Name: name, Mode: EdgeModeRelation}
	for _, opt := range opts {
		opt(&e)
	}
	return e
}

// BidirectionalEdge builds an edge whose from and to constraints reference the
// same table (e.g. user->user follows).
func BidirectionalEdge(name, table string, opts ...EdgeOption) EdgeDefinition {
	merged := append([]EdgeOption{WithFromTable(table), WithToTable(table)}, opts...)
	return NewEdge(name, merged...)
}

// TypedEdge builds an edge with specific from/to table constraints.
func TypedEdge(name, fromTable, toTable string, opts ...EdgeOption) EdgeDefinition {
	merged := append([]EdgeOption{WithFromTable(fromTable), WithToTable(toTable)}, opts...)
	return NewEdge(name, merged...)
}

// Validate checks edge-level invariants plus each child definition.
func (e EdgeDefinition) Validate() error {
	if e.Name == "" {
		return surqlerrors.New(surqlerrors.ErrValidation, "edge name cannot be empty")
	}
	if !e.Mode.IsValid() {
		return surqlerrors.Newf(surqlerrors.ErrValidation,
			"invalid edge mode %q for edge %q", string(e.Mode), e.Name)
	}
	if e.Mode == EdgeModeRelation {
		if e.FromTable == "" || e.ToTable == "" {
			return surqlerrors.Newf(surqlerrors.ErrValidation,
				"edge %q with RELATION mode requires both from_table and to_table",
				e.Name)
		}
	}
	for _, f := range e.Fields {
		if err := f.Validate(); err != nil {
			return err
		}
	}
	for _, i := range e.Indexes {
		if err := i.Validate(); err != nil {
			return err
		}
	}
	for _, ev := range e.Events {
		if err := ev.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// ToSurql emits only the DEFINE TABLE line for this edge (no children).
func (e EdgeDefinition) ToSurql() string {
	return e.toSurqlTable(false)
}

// ToSurqlIfNotExists emits the DEFINE TABLE line with IF NOT EXISTS.
func (e EdgeDefinition) ToSurqlIfNotExists() string {
	return e.toSurqlTable(true)
}

func (e EdgeDefinition) toSurqlTable(ifNotExists bool) string {
	var b strings.Builder
	b.WriteString("DEFINE TABLE")
	if ifNotExists {
		b.WriteString(" IF NOT EXISTS")
	}
	b.WriteString(" ")
	b.WriteString(e.Name)

	switch e.Mode {
	case EdgeModeRelation:
		b.WriteString(" TYPE RELATION")
		if e.FromTable != "" {
			b.WriteString(" FROM ")
			b.WriteString(e.FromTable)
		}
		if e.ToTable != "" {
			b.WriteString(" TO ")
			b.WriteString(e.ToTable)
		}
	case EdgeModeSchemafull:
		b.WriteString(" SCHEMAFULL")
	case EdgeModeSchemaless:
		b.WriteString(" SCHEMALESS")
	}

	b.WriteString(";")
	return b.String()
}

// ToSurqlStatements returns the DEFINE TABLE line followed by each DEFINE
// FIELD / INDEX / EVENT / PERMISSIONS for this edge. It returns an error when
// the edge violates structural invariants (e.g. RELATION mode without
// from/to tables).
func (e EdgeDefinition) ToSurqlStatements() ([]string, error) {
	return e.toSurqlStatements(false)
}

// ToSurqlStatementsIfNotExists is like ToSurqlStatements with IF NOT EXISTS.
func (e EdgeDefinition) ToSurqlStatementsIfNotExists() ([]string, error) {
	return e.toSurqlStatements(true)
}

func (e EdgeDefinition) toSurqlStatements(ifNotExists bool) ([]string, error) {
	if e.Mode == EdgeModeRelation && (e.FromTable == "" || e.ToTable == "") {
		return nil, surqlerrors.Newf(surqlerrors.ErrValidation,
			"edge %q with RELATION mode requires both from_table and to_table",
			e.Name)
	}

	stmts := make([]string, 0, 1+len(e.Fields)+len(e.Indexes)+len(e.Events))
	stmts = append(stmts, e.toSurqlTable(ifNotExists))

	for _, f := range e.Fields {
		stmts = append(stmts, f.toSurql(e.Name, ifNotExists))
	}
	for _, i := range e.Indexes {
		stmts = append(stmts, i.toSurql(e.Name, ifNotExists))
	}
	for _, ev := range e.Events {
		stmts = append(stmts, ev.toSurql(e.Name, ifNotExists))
	}

	if len(e.Permissions) > 0 {
		keys := make([]string, 0, len(e.Permissions))
		for k := range e.Permissions {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			stmts = append(stmts,
				"DEFINE FIELD PERMISSIONS FOR "+strings.ToUpper(k)+
					" ON TABLE "+e.Name+" WHERE "+e.Permissions[k]+";")
		}
	}

	return stmts, nil
}
