package schema

import (
	"sort"
	"strconv"
	"strings"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

// TableMode enumerates the table schema modes.
type TableMode string

// TableMode values.
const (
	TableModeSchemafull TableMode = "SCHEMAFULL"
	TableModeSchemaless TableMode = "SCHEMALESS"
	TableModeDrop       TableMode = "DROP"
)

// IsValid reports whether the TableMode is recognised.
func (m TableMode) IsValid() bool {
	switch m {
	case TableModeSchemafull, TableModeSchemaless, TableModeDrop:
		return true
	}
	return false
}

// IndexType enumerates the supported DEFINE INDEX flavours.
type IndexType string

// IndexType values.
const (
	IndexTypeStandard IndexType = "INDEX"
	IndexTypeUnique   IndexType = "UNIQUE"
	IndexTypeSearch   IndexType = "SEARCH"
	IndexTypeMTree    IndexType = "MTREE"
	IndexTypeHNSW     IndexType = "HNSW"
)

// IsValid reports whether the IndexType is recognised.
func (t IndexType) IsValid() bool {
	switch t {
	case IndexTypeStandard, IndexTypeUnique, IndexTypeSearch, IndexTypeMTree, IndexTypeHNSW:
		return true
	}
	return false
}

// MTreeDistanceType enumerates distance metrics for MTREE indexes.
type MTreeDistanceType string

// MTreeDistanceType values.
const (
	MTreeDistanceCosine    MTreeDistanceType = "COSINE"
	MTreeDistanceEuclidean MTreeDistanceType = "EUCLIDEAN"
	MTreeDistanceManhattan MTreeDistanceType = "MANHATTAN"
	MTreeDistanceMinkowski MTreeDistanceType = "MINKOWSKI"
)

// IsValid reports whether the MTreeDistanceType is recognised.
func (d MTreeDistanceType) IsValid() bool {
	switch d {
	case MTreeDistanceCosine, MTreeDistanceEuclidean, MTreeDistanceManhattan, MTreeDistanceMinkowski:
		return true
	}
	return false
}

// HnswDistanceType enumerates distance metrics for HNSW indexes (superset of
// MTreeDistanceType).
type HnswDistanceType string

// HnswDistanceType values.
const (
	HnswDistanceChebyshev HnswDistanceType = "CHEBYSHEV"
	HnswDistanceCosine    HnswDistanceType = "COSINE"
	HnswDistanceEuclidean HnswDistanceType = "EUCLIDEAN"
	HnswDistanceHamming   HnswDistanceType = "HAMMING"
	HnswDistanceJaccard   HnswDistanceType = "JACCARD"
	HnswDistanceManhattan HnswDistanceType = "MANHATTAN"
	HnswDistanceMinkowski HnswDistanceType = "MINKOWSKI"
	HnswDistancePearson   HnswDistanceType = "PEARSON"
)

// IsValid reports whether the HnswDistanceType is recognised.
func (d HnswDistanceType) IsValid() bool {
	switch d {
	case HnswDistanceChebyshev, HnswDistanceCosine, HnswDistanceEuclidean,
		HnswDistanceHamming, HnswDistanceJaccard, HnswDistanceManhattan,
		HnswDistanceMinkowski, HnswDistancePearson:
		return true
	}
	return false
}

// MTreeVectorType enumerates the vector component numeric types supported by
// both MTREE and HNSW indexes.
type MTreeVectorType string

// MTreeVectorType values.
const (
	MTreeVectorF64 MTreeVectorType = "F64"
	MTreeVectorF32 MTreeVectorType = "F32"
	MTreeVectorI64 MTreeVectorType = "I64"
	MTreeVectorI32 MTreeVectorType = "I32"
	MTreeVectorI16 MTreeVectorType = "I16"
)

// IsValid reports whether the MTreeVectorType is recognised.
func (v MTreeVectorType) IsValid() bool {
	switch v {
	case MTreeVectorF64, MTreeVectorF32, MTreeVectorI64, MTreeVectorI32, MTreeVectorI16:
		return true
	}
	return false
}

// IndexDefinition captures the fields required to emit a DEFINE INDEX statement.
type IndexDefinition struct {
	Name    string
	Columns []string
	Type    IndexType

	// MTREE & HNSW shared
	Dimension  int
	VectorType MTreeVectorType

	// MTREE-specific
	Distance MTreeDistanceType

	// HNSW-specific
	HnswDistance HnswDistanceType
	EFC          int // zero means unset
	M            int // zero means unset

	// Full-text SEARCH-specific. Analyzer is the analyzer name; an empty
	// string renders the historical default (`ascii`). BM25 emits the
	// relevance-scoring clause (with the engine's default k1/b parameters),
	// required for query.Query.SearchScore to return a value. Highlights
	// stores positional HIGHLIGHTS data (enables search::highlight /
	// search::offsets).
	Analyzer   string
	BM25       bool
	Highlights bool
}

// EventDefinition captures a DEFINE EVENT trigger.
type EventDefinition struct {
	Name      string
	Condition string
	Action    string
}

// TableDefinition captures the fields required to emit a DEFINE TABLE
// statement plus its attendant DEFINE FIELD / INDEX / EVENT children.
type TableDefinition struct {
	Name        string
	Mode        TableMode
	Fields      []FieldDefinition
	Indexes     []IndexDefinition
	Events      []EventDefinition
	Permissions map[string]string
	Drop        bool
}

// TableOption customises a TableDefinition created via NewTable.
type TableOption func(*TableDefinition)

// WithMode sets the schema mode (SCHEMAFULL / SCHEMALESS / DROP).
func WithMode(mode TableMode) TableOption {
	return func(t *TableDefinition) { t.Mode = mode }
}

// WithFields appends fields to the table definition.
func WithFields(fields ...FieldDefinition) TableOption {
	return func(t *TableDefinition) { t.Fields = append(t.Fields, fields...) }
}

// WithIndexes appends indexes to the table definition.
func WithIndexes(indexes ...IndexDefinition) TableOption {
	return func(t *TableDefinition) { t.Indexes = append(t.Indexes, indexes...) }
}

// WithEvents appends events to the table definition.
func WithEvents(events ...EventDefinition) TableOption {
	return func(t *TableDefinition) { t.Events = append(t.Events, events...) }
}

// WithTablePermissions sets permissions for the table.
func WithTablePermissions(perms map[string]string) TableOption {
	return func(t *TableDefinition) {
		if perms == nil {
			t.Permissions = nil
			return
		}
		copied := make(map[string]string, len(perms))
		for k, v := range perms {
			copied[k] = v
		}
		t.Permissions = copied
	}
}

// WithDrop marks the table for deletion.
func WithDrop(drop bool) TableOption {
	return func(t *TableDefinition) { t.Drop = drop }
}

// NewTable constructs a TableDefinition, defaulting to SCHEMAFULL mode.
func NewTable(name string, opts ...TableOption) TableDefinition {
	t := TableDefinition{Name: name, Mode: TableModeSchemafull}
	for _, opt := range opts {
		opt(&t)
	}
	return t
}

// Validate checks table-level invariants plus each child field, index, and event.
func (t TableDefinition) Validate() error {
	if t.Name == "" {
		return surqlerrors.New(surqlerrors.ErrValidation, "table name cannot be empty")
	}
	if !t.Mode.IsValid() {
		return surqlerrors.Newf(surqlerrors.ErrValidation,
			"invalid table mode %q for table %q", string(t.Mode), t.Name)
	}
	for _, f := range t.Fields {
		if err := f.Validate(); err != nil {
			return err
		}
	}
	for _, i := range t.Indexes {
		if err := i.Validate(); err != nil {
			return err
		}
	}
	for _, e := range t.Events {
		if err := e.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// ToSurql emits the DEFINE TABLE statement only (no fields, indexes, events).
func (t TableDefinition) ToSurql() string {
	return t.toSurqlTable(false)
}

// ToSurqlIfNotExists emits the DEFINE TABLE statement with IF NOT EXISTS.
func (t TableDefinition) ToSurqlIfNotExists() string {
	return t.toSurqlTable(true)
}

func (t TableDefinition) toSurqlTable(ifNotExists bool) string {
	var b strings.Builder
	b.WriteString("DEFINE TABLE")
	if ifNotExists {
		b.WriteString(" IF NOT EXISTS")
	}
	b.WriteString(" ")
	b.WriteString(t.Name)
	b.WriteString(" ")
	b.WriteString(string(t.Mode))
	b.WriteString(";")
	return b.String()
}

// ToSurqlStatements returns the full list of DEFINE statements for the table:
// the DEFINE TABLE line followed by each DEFINE FIELD / INDEX / EVENT, plus
// any DEFINE FIELD PERMISSIONS rendered from the permission map.
func (t TableDefinition) ToSurqlStatements() []string {
	return t.toSurqlStatements(false)
}

// ToSurqlStatementsIfNotExists is like ToSurqlStatements but adds IF NOT
// EXISTS to every DEFINE statement that supports it.
func (t TableDefinition) ToSurqlStatementsIfNotExists() []string {
	return t.toSurqlStatements(true)
}

func (t TableDefinition) toSurqlStatements(ifNotExists bool) []string {
	stmts := make([]string, 0, 1+len(t.Fields)+len(t.Indexes)+len(t.Events))
	stmts = append(stmts, t.toSurqlTable(ifNotExists))

	for _, f := range t.Fields {
		stmts = append(stmts, f.toSurql(t.Name, ifNotExists))
	}
	for _, i := range t.Indexes {
		stmts = append(stmts, i.toSurql(t.Name, ifNotExists))
	}
	for _, e := range t.Events {
		stmts = append(stmts, e.toSurql(t.Name, ifNotExists))
	}

	if len(t.Permissions) > 0 {
		keys := make([]string, 0, len(t.Permissions))
		for k := range t.Permissions {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			stmts = append(stmts,
				"DEFINE FIELD PERMISSIONS FOR "+strings.ToUpper(k)+
					" ON TABLE "+t.Name+" WHERE "+t.Permissions[k]+";")
		}
	}
	return stmts
}

// NewIndex builds a standard IndexDefinition.
func NewIndex(name string, columns []string, indexType IndexType) IndexDefinition {
	cols := make([]string, len(columns))
	copy(cols, columns)
	return IndexDefinition{Name: name, Columns: cols, Type: indexType}
}

// UniqueIndex is sugar for NewIndex(name, columns, IndexTypeUnique).
func UniqueIndex(name string, columns []string) IndexDefinition {
	return NewIndex(name, columns, IndexTypeUnique)
}

// SearchIndex is sugar for NewIndex(name, columns, IndexTypeSearch). With no
// analyzer set it renders the historical `ascii` default; chain WithAnalyzer /
// WithBM25 / WithHighlights for a scorable index, or use BM25Index.
func SearchIndex(name string, columns []string) IndexDefinition {
	return NewIndex(name, columns, IndexTypeSearch)
}

// BM25Index builds a BM25-scored full-text SEARCH index over columns, analyzed
// by analyzer. This is the index to pair with query.Query.FullTextSearch and
// query.Query.SearchScore for lexical recall — BM25 is what makes
// search::score return a relevance value.
func BM25Index(name string, columns []string, analyzer string) IndexDefinition {
	return SearchIndex(name, columns).WithAnalyzer(analyzer).WithBM25()
}

// WithAnalyzer sets the full-text SEARCH analyzer (e.g. one defined via
// Analyzer / StandardAnalyzer). Only affects SEARCH indexes; when unset the
// index renders the historical `ascii` analyzer. Returns a modified copy.
func (i IndexDefinition) WithAnalyzer(analyzer string) IndexDefinition {
	i.Analyzer = analyzer
	return i
}

// WithBM25 emits the BM25 relevance-scoring clause on a SEARCH index (with the
// engine's default parameters). Required for query.Query.SearchScore. Returns
// a modified copy.
func (i IndexDefinition) WithBM25() IndexDefinition {
	i.BM25 = true
	return i
}

// WithHighlights stores positional HIGHLIGHTS data on a SEARCH index. Returns
// a modified copy.
func (i IndexDefinition) WithHighlights() IndexDefinition {
	i.Highlights = true
	return i
}

// MTreeIndexOptions configures an MTREE vector index.
type MTreeIndexOptions struct {
	Distance   MTreeDistanceType
	VectorType MTreeVectorType
}

// MTreeIndex builds an MTREE IndexDefinition.
func MTreeIndex(name, column string, dimension int, opts MTreeIndexOptions) IndexDefinition {
	if opts.Distance == "" {
		opts.Distance = MTreeDistanceEuclidean
	}
	if opts.VectorType == "" {
		opts.VectorType = MTreeVectorF64
	}
	return IndexDefinition{
		Name:       name,
		Columns:    []string{column},
		Type:       IndexTypeMTree,
		Dimension:  dimension,
		Distance:   opts.Distance,
		VectorType: opts.VectorType,
	}
}

// HnswIndexOptions configures an HNSW vector index. EFC and M are optional
// (zero means "use SurrealDB default" and the EFC / M clauses are omitted).
type HnswIndexOptions struct {
	Distance   HnswDistanceType
	VectorType MTreeVectorType
	EFC        int
	M          int
}

// HnswIndex builds an HNSW IndexDefinition.
func HnswIndex(name, column string, dimension int, opts HnswIndexOptions) IndexDefinition {
	if opts.Distance == "" {
		opts.Distance = HnswDistanceEuclidean
	}
	if opts.VectorType == "" {
		opts.VectorType = MTreeVectorF64
	}
	return IndexDefinition{
		Name:         name,
		Columns:      []string{column},
		Type:         IndexTypeHNSW,
		Dimension:    dimension,
		VectorType:   opts.VectorType,
		HnswDistance: opts.Distance,
		EFC:          opts.EFC,
		M:            opts.M,
	}
}

// Validate verifies structural invariants of the index definition.
func (i IndexDefinition) Validate() error {
	if i.Name == "" {
		return surqlerrors.New(surqlerrors.ErrValidation, "index name cannot be empty")
	}
	if !i.Type.IsValid() {
		return surqlerrors.Newf(surqlerrors.ErrValidation,
			"invalid index type %q for index %q", string(i.Type), i.Name)
	}
	if len(i.Columns) == 0 {
		return surqlerrors.Newf(surqlerrors.ErrValidation,
			"index %q requires at least one column", i.Name)
	}
	for _, col := range i.Columns {
		if col == "" {
			return surqlerrors.Newf(surqlerrors.ErrValidation,
				"index %q has an empty column name", i.Name)
		}
	}

	switch i.Type {
	case IndexTypeMTree:
		if i.Dimension <= 0 {
			return surqlerrors.Newf(surqlerrors.ErrValidation,
				"MTREE index %q requires a positive dimension", i.Name)
		}
		if i.Distance != "" && !i.Distance.IsValid() {
			return surqlerrors.Newf(surqlerrors.ErrValidation,
				"MTREE index %q has invalid distance %q", i.Name, string(i.Distance))
		}
		if i.VectorType != "" && !i.VectorType.IsValid() {
			return surqlerrors.Newf(surqlerrors.ErrValidation,
				"MTREE index %q has invalid vector type %q", i.Name, string(i.VectorType))
		}
	case IndexTypeHNSW:
		if i.Dimension <= 0 {
			return surqlerrors.Newf(surqlerrors.ErrValidation,
				"HNSW index %q requires a positive dimension", i.Name)
		}
		if i.HnswDistance != "" && !i.HnswDistance.IsValid() {
			return surqlerrors.Newf(surqlerrors.ErrValidation,
				"HNSW index %q has invalid distance %q", i.Name, string(i.HnswDistance))
		}
		if i.VectorType != "" && !i.VectorType.IsValid() {
			return surqlerrors.Newf(surqlerrors.ErrValidation,
				"HNSW index %q has invalid vector type %q", i.Name, string(i.VectorType))
		}
	}
	return nil
}

// ToSurql emits the DEFINE INDEX statement for this index on tableName.
func (i IndexDefinition) ToSurql(tableName string) string {
	return i.toSurql(tableName, false)
}

// ToSurqlIfNotExists emits the DEFINE INDEX statement with IF NOT EXISTS.
func (i IndexDefinition) ToSurqlIfNotExists(tableName string) string {
	return i.toSurql(tableName, true)
}

func (i IndexDefinition) toSurql(tableName string, ifNotExists bool) string {
	var b strings.Builder
	b.WriteString("DEFINE INDEX")
	if ifNotExists {
		b.WriteString(" IF NOT EXISTS")
	}
	b.WriteString(" ")
	b.WriteString(i.Name)
	b.WriteString(" ON TABLE ")
	b.WriteString(tableName)

	switch i.Type {
	case IndexTypeMTree:
		field := ""
		if len(i.Columns) > 0 {
			field = i.Columns[0]
		}
		b.WriteString(" COLUMNS ")
		b.WriteString(field)
		b.WriteString(" MTREE DIMENSION ")
		b.WriteString(strconv.Itoa(i.Dimension))
		if i.Distance != "" {
			b.WriteString(" DIST ")
			b.WriteString(string(i.Distance))
		}
		if i.VectorType != "" {
			b.WriteString(" TYPE ")
			b.WriteString(string(i.VectorType))
		}
		b.WriteString(";")
		return b.String()

	case IndexTypeHNSW:
		field := ""
		if len(i.Columns) > 0 {
			field = i.Columns[0]
		}
		b.WriteString(" COLUMNS ")
		b.WriteString(field)
		b.WriteString(" HNSW DIMENSION ")
		b.WriteString(strconv.Itoa(i.Dimension))
		if i.HnswDistance != "" {
			b.WriteString(" DIST ")
			b.WriteString(string(i.HnswDistance))
		}
		if i.VectorType != "" {
			b.WriteString(" TYPE ")
			b.WriteString(string(i.VectorType))
		}
		if i.EFC > 0 {
			b.WriteString(" EFC ")
			b.WriteString(strconv.Itoa(i.EFC))
		}
		if i.M > 0 {
			b.WriteString(" M ")
			b.WriteString(strconv.Itoa(i.M))
		}
		b.WriteString(";")
		return b.String()
	}

	b.WriteString(" COLUMNS ")
	b.WriteString(strings.Join(i.Columns, ", "))

	switch i.Type {
	case IndexTypeUnique:
		b.WriteString(" UNIQUE")
	case IndexTypeSearch:
		analyzer := i.Analyzer
		if analyzer == "" {
			analyzer = "ascii"
		}
		b.WriteString(" FULLTEXT ANALYZER ")
		b.WriteString(analyzer)
		if i.BM25 {
			b.WriteString(" BM25")
		}
		if i.Highlights {
			b.WriteString(" HIGHLIGHTS")
		}
	}

	b.WriteString(";")
	return b.String()
}

// NewEvent builds an EventDefinition.
func NewEvent(name, condition, action string) EventDefinition {
	return EventDefinition{Name: name, Condition: condition, Action: action}
}

// Validate ensures the event has a non-empty name, condition, and action.
func (e EventDefinition) Validate() error {
	if e.Name == "" {
		return surqlerrors.New(surqlerrors.ErrValidation, "event name cannot be empty")
	}
	if e.Condition == "" {
		return surqlerrors.Newf(surqlerrors.ErrValidation,
			"event %q requires a non-empty condition", e.Name)
	}
	if e.Action == "" {
		return surqlerrors.Newf(surqlerrors.ErrValidation,
			"event %q requires a non-empty action", e.Name)
	}
	return nil
}

// ToSurql emits the DEFINE EVENT statement for this event on tableName.
func (e EventDefinition) ToSurql(tableName string) string {
	return e.toSurql(tableName, false)
}

// ToSurqlIfNotExists emits the DEFINE EVENT statement with IF NOT EXISTS.
func (e EventDefinition) ToSurqlIfNotExists(tableName string) string {
	return e.toSurql(tableName, true)
}

func (e EventDefinition) toSurql(tableName string, ifNotExists bool) string {
	var b strings.Builder
	b.WriteString("DEFINE EVENT")
	if ifNotExists {
		b.WriteString(" IF NOT EXISTS")
	}
	b.WriteString(" ")
	b.WriteString(e.Name)
	b.WriteString(" ON TABLE ")
	b.WriteString(tableName)
	b.WriteString(" WHEN ")
	b.WriteString(e.Condition)
	b.WriteString(" THEN ")
	b.WriteString(e.Action)
	b.WriteString(";")
	return b.String()
}
