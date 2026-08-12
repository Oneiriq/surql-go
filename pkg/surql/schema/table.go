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
	// IndexTypeDiskAnn is an on-disk approximate-nearest-neighbour graph
	// (SurrealDB 3.2+). The graph lives on disk rather than in memory, so an
	// index outgrows RAM without outgrowing the box. Build it with DiskAnnIndex.
	IndexTypeDiskAnn IndexType = "DISKANN"
)

// IsValid reports whether the IndexType is recognised.
func (t IndexType) IsValid() bool {
	switch t {
	case IndexTypeStandard, IndexTypeUnique, IndexTypeSearch, IndexTypeMTree,
		IndexTypeHNSW, IndexTypeDiskAnn:
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

// DiskAnnDistanceType enumerates distance metrics for DISKANN indexes.
//
// Its own type rather than a reuse of HnswDistanceType: the engine's DISKANN
// set both adds metrics HNSW lacks (INNER_PRODUCT, COSINE_NORMALIZED) and
// refuses every HNSW metric outside it, so an out-of-set metric is
// unrepresentable here.
type DiskAnnDistanceType string

// DiskAnnDistanceType values.
const (
	DiskAnnDistanceCosine           DiskAnnDistanceType = "COSINE"
	DiskAnnDistanceCosineNormalized DiskAnnDistanceType = "COSINE_NORMALIZED"
	DiskAnnDistanceEuclidean        DiskAnnDistanceType = "EUCLIDEAN"
	DiskAnnDistanceInnerProduct     DiskAnnDistanceType = "INNER_PRODUCT"
)

// IsValid reports whether the DiskAnnDistanceType is recognised.
func (d DiskAnnDistanceType) IsValid() bool {
	switch d {
	case DiskAnnDistanceCosine, DiskAnnDistanceCosineNormalized,
		DiskAnnDistanceEuclidean, DiskAnnDistanceInnerProduct:
		return true
	}
	return false
}

// MTreeVectorType enumerates the vector component numeric types. One shared
// vocabulary; each index kind accepts a subset. The engine takes every value
// for HNSW, refuses F16 / I8 / U8 for MTREE, and refuses everything but
// F32 / F16 / I8 / U8 for DISKANN. IndexDefinition.Validate teaches those
// limits before a statement is sent.
type MTreeVectorType string

// MTreeVectorType values.
const (
	MTreeVectorF64 MTreeVectorType = "F64"
	MTreeVectorF32 MTreeVectorType = "F32"
	MTreeVectorF16 MTreeVectorType = "F16"
	MTreeVectorI64 MTreeVectorType = "I64"
	MTreeVectorI32 MTreeVectorType = "I32"
	MTreeVectorI16 MTreeVectorType = "I16"
	MTreeVectorI8  MTreeVectorType = "I8"
	MTreeVectorU8  MTreeVectorType = "U8"
)

// IsValid reports whether the MTreeVectorType is recognised.
func (v MTreeVectorType) IsValid() bool {
	switch v {
	case MTreeVectorF64, MTreeVectorF32, MTreeVectorF16, MTreeVectorI64,
		MTreeVectorI32, MTreeVectorI16, MTreeVectorI8, MTreeVectorU8:
		return true
	}
	return false
}

// ValidForMTree reports whether MTREE parses this element type. MTREE takes
// only its historical five; the narrow three are a parse error with no
// teaching message from the engine.
func (v MTreeVectorType) ValidForMTree() bool {
	switch v {
	case MTreeVectorF64, MTreeVectorF32, MTreeVectorI64, MTreeVectorI32, MTreeVectorI16:
		return true
	}
	return false
}

// ValidForDiskAnn reports whether DISKANN accepts this element type.
func (v MTreeVectorType) ValidForDiskAnn() bool {
	switch v {
	case MTreeVectorF32, MTreeVectorF16, MTreeVectorI8, MTreeVectorU8:
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

	// DISKANN-specific. Degree, LBuild, and Alpha carry the engine's own
	// defaults rather than staying unset, because the engine echoes them back
	// filled in; see the DiskAnnDefault constants. Alpha is the decimal literal
	// the statement carries, since the engine echoes a float with a trailing f
	// suffix (ALPHA 1.2f) that the parser strips.
	DiskAnnDistance DiskAnnDistanceType
	Degree          int // zero means unset
	LBuild          int // zero means unset
	Alpha           string
	HashedVector    bool

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

// Engine defaults a DISKANN index echoes back when the definition never stated
// them. The builder fills the same values up front so a definition compares
// equal to its own echo instead of re-applying on every reconcile.
const (
	DiskAnnDefaultDegree = 64
	DiskAnnDefaultLBuild = 100
	DiskAnnDefaultAlpha  = "1.2"
)

// CanonicalAlpha renders a DISKANN ALPHA value the way the engine echoes it: a
// whole number bare (ALPHA 2) and a fractional one as a plain decimal, which is
// what a float echo (ALPHA 1.2f) reads back as once its suffix is stripped.
func CanonicalAlpha(alpha float64) string {
	return strconv.FormatFloat(alpha, 'f', -1, 64)
}

// DiskAnnIndexOptions configures a DISKANN vector index. Degree, LBuild, and
// Alpha are optional; zero (or empty) takes the engine default, which is then
// spelled explicitly in the rendered statement.
type DiskAnnIndexOptions struct {
	Distance     DiskAnnDistanceType
	VectorType   MTreeVectorType
	Degree       int
	LBuild       int
	Alpha        string
	HashedVector bool
}

// DiskAnnIndex builds a DISKANN IndexDefinition.
func DiskAnnIndex(name, column string, dimension int, opts DiskAnnIndexOptions) IndexDefinition {
	if opts.Distance == "" {
		opts.Distance = DiskAnnDistanceEuclidean
	}
	if opts.VectorType == "" {
		opts.VectorType = MTreeVectorF32
	}
	if opts.Degree == 0 {
		opts.Degree = DiskAnnDefaultDegree
	}
	if opts.LBuild == 0 {
		opts.LBuild = DiskAnnDefaultLBuild
	}
	if opts.Alpha == "" {
		opts.Alpha = DiskAnnDefaultAlpha
	}
	return IndexDefinition{
		Name:            name,
		Columns:         []string{column},
		Type:            IndexTypeDiskAnn,
		Dimension:       dimension,
		VectorType:      opts.VectorType,
		DiskAnnDistance: opts.Distance,
		Degree:          opts.Degree,
		LBuild:          opts.LBuild,
		Alpha:           opts.Alpha,
		HashedVector:    opts.HashedVector,
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
		if i.VectorType != "" && !i.VectorType.ValidForMTree() {
			return surqlerrors.Newf(surqlerrors.ErrValidation,
				"MTREE index %q cannot use TYPE %s: the engine only accepts "+
					"F64, F32, I64, I32, or I16 for MTREE", i.Name, string(i.VectorType))
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
	case IndexTypeDiskAnn:
		if i.Dimension <= 0 {
			return surqlerrors.Newf(surqlerrors.ErrValidation,
				"DISKANN index %q requires a positive dimension", i.Name)
		}
		if i.DiskAnnDistance != "" && !i.DiskAnnDistance.IsValid() {
			return surqlerrors.Newf(surqlerrors.ErrValidation,
				"DISKANN index %q has invalid distance %q", i.Name, string(i.DiskAnnDistance))
		}
		if i.VectorType != "" && !i.VectorType.IsValid() {
			return surqlerrors.Newf(surqlerrors.ErrValidation,
				"DISKANN index %q has invalid vector type %q", i.Name, string(i.VectorType))
		}
		if i.VectorType != "" && !i.VectorType.ValidForDiskAnn() {
			return surqlerrors.Newf(surqlerrors.ErrValidation,
				"DISKANN index %q cannot use TYPE %s: the engine only accepts "+
					"F32, F16, I8, or U8 for DISKANN", i.Name, string(i.VectorType))
		}
		// A metric aimed at DISKANN through the MTREE or HNSW member would be
		// dropped by the renderer, so the mistake is refused rather than
		// silently ignored.
		if i.Distance != "" || i.HnswDistance != "" {
			return surqlerrors.Newf(surqlerrors.ErrValidation,
				"DISKANN index %q takes its metric through DiskAnnDistance "+
					"(EUCLIDEAN, COSINE, INNER_PRODUCT, or COSINE_NORMALIZED); the engine "+
					"refuses every other MTREE/HNSW metric for DISKANN", i.Name)
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

	case IndexTypeDiskAnn:
		// The engine always echoes DIST / TYPE / DEGREE / L_BUILD / ALPHA back
		// with its defaults filled in, even when the definition never stated
		// them, so this spells them all. A definition that omitted one would
		// never compare equal to its own echo, and a reconcile would re-apply
		// the index on every boot.
		field := ""
		if len(i.Columns) > 0 {
			field = i.Columns[0]
		}
		distance := i.DiskAnnDistance
		if distance == "" {
			distance = DiskAnnDistanceEuclidean
		}
		vectorType := i.VectorType
		if vectorType == "" {
			vectorType = MTreeVectorF32
		}
		degree := i.Degree
		if degree == 0 {
			degree = DiskAnnDefaultDegree
		}
		lBuild := i.LBuild
		if lBuild == 0 {
			lBuild = DiskAnnDefaultLBuild
		}
		alpha := i.Alpha
		if alpha == "" {
			alpha = DiskAnnDefaultAlpha
		}
		b.WriteString(" COLUMNS ")
		b.WriteString(field)
		b.WriteString(" DISKANN DIMENSION ")
		b.WriteString(strconv.Itoa(i.Dimension))
		b.WriteString(" DIST ")
		b.WriteString(string(distance))
		b.WriteString(" TYPE ")
		b.WriteString(string(vectorType))
		b.WriteString(" DEGREE ")
		b.WriteString(strconv.Itoa(degree))
		b.WriteString(" L_BUILD ")
		b.WriteString(strconv.Itoa(lBuild))
		b.WriteString(" ALPHA ")
		b.WriteString(alpha)
		if i.HashedVector {
			b.WriteString(" HASHED_VECTOR")
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
