package schema

import (
	"regexp"
	"strconv"
	"strings"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

// DatabaseInfo is the parsed shape of an INFO FOR DB response. Tables, Edges,
// and Accesses are keyed by definition name. Edges are tables whose tb
// definition includes TYPE RELATION; everything else is a regular table.
type DatabaseInfo struct {
	Tables   map[string]TableDefinition
	Edges    map[string]EdgeDefinition
	Accesses map[string]AccessDefinition
}

// ParseDBInfo parses a SurrealDB INFO FOR DB response into a DatabaseInfo.
//
// The accepted shape is the object map returned by SurrealDB (e.g. {"tb": {
// "user": "DEFINE TABLE user SCHEMAFULL;", ... }, "ac": {...}}). Each value in
// the inner maps is the DEFINE statement string for that object. Unparseable
// entries are skipped silently, matching the Python behaviour which logs and
// moves on.
//
// The returned DatabaseInfo always has non-nil maps, even when the input is
// empty.
func ParseDBInfo(info map[string]any) (DatabaseInfo, error) {
	if info == nil {
		return DatabaseInfo{
			Tables:   map[string]TableDefinition{},
			Edges:    map[string]EdgeDefinition{},
			Accesses: map[string]AccessDefinition{},
		}, nil
	}

	tables := map[string]TableDefinition{}
	edges := map[string]EdgeDefinition{}
	accesses := map[string]AccessDefinition{}

	tbDict := extractStringMap(info, "tables", "tb")
	for name, def := range tbDict {
		if name == "" {
			continue
		}
		if isEdgeDefinitionSource(def) {
			if edge, err := parseEdgeFromDefinition(name, def); err == nil {
				edges[name] = edge
			}
			continue
		}
		tables[name] = TableDefinition{
			Name: name,
			Mode: parseTableMode(def),
		}
	}

	acDict := extractStringMap(info, "accesses", "ac")
	for name, def := range acDict {
		if name == "" {
			continue
		}
		access, err := ParseAccess(name, def)
		if err != nil {
			continue
		}
		accesses[name] = access
	}

	return DatabaseInfo{
		Tables:   tables,
		Edges:    edges,
		Accesses: accesses,
	}, nil
}

// ParseTableInfo parses a SurrealDB INFO FOR TABLE response into a
// TableDefinition. The tableName identifies the table (the INFO payload itself
// does not carry it). Unknown or malformed fields/indexes/events are skipped.
//
// Returns an error wrapping ErrSchemaParse only for catastrophic failures
// (currently: an empty name). Individual child parse failures are silent.
func ParseTableInfo(tableName string, info map[string]any) (TableDefinition, error) {
	if tableName == "" {
		return TableDefinition{}, surqlerrors.New(surqlerrors.ErrSchemaParse,
			"table name cannot be empty")
	}

	tb := stringAt(info, "tb")
	mode := parseTableMode(tb)

	fieldsDict := extractStringMap(info, "fields", "fd")
	fields := parseFields(fieldsDict)

	indexesDict := extractStringMap(info, "indexes", "ix")
	indexes := parseIndexes(indexesDict)

	eventsDict := extractStringMap(info, "events", "ev")
	events := parseEvents(eventsDict)

	return TableDefinition{
		Name:    tableName,
		Mode:    mode,
		Fields:  fields,
		Indexes: indexes,
		Events:  events,
	}, nil
}

// ParseEdgeInfo parses a SurrealDB INFO FOR TABLE response into an
// EdgeDefinition. The edgeName identifies the edge. The tb definition (when
// present) is consulted to extract RELATION / FROM / TO clauses.
func ParseEdgeInfo(edgeName string, info map[string]any) (EdgeDefinition, error) {
	if edgeName == "" {
		return EdgeDefinition{}, surqlerrors.New(surqlerrors.ErrSchemaParse,
			"edge name cannot be empty")
	}

	tb := stringAt(info, "tb")
	edge, _ := parseEdgeFromDefinition(edgeName, tb)

	fieldsDict := extractStringMap(info, "fields", "fd")
	edge.Fields = parseFields(fieldsDict)

	indexesDict := extractStringMap(info, "indexes", "ix")
	edge.Indexes = parseIndexes(indexesDict)

	eventsDict := extractStringMap(info, "events", "ev")
	edge.Events = parseEvents(eventsDict)

	return edge, nil
}

// ParseField parses a single DEFINE FIELD statement into a FieldDefinition.
// Returns an error wrapping ErrSchemaParse when the definition string is empty.
func ParseField(fieldName, definition string) (FieldDefinition, error) {
	if fieldName == "" {
		return FieldDefinition{}, surqlerrors.New(surqlerrors.ErrSchemaParse,
			"field name cannot be empty")
	}
	if strings.TrimSpace(definition) == "" {
		return FieldDefinition{}, surqlerrors.Newf(surqlerrors.ErrSchemaParse,
			"field %q has empty definition", fieldName)
	}
	return FieldDefinition{
		Name:      fieldName,
		Type:      extractFieldType(definition),
		Assertion: extractAssertion(definition),
		Default:   extractDefault(definition),
		Value:     extractValue(definition),
		ReadOnly:  extractReadOnly(definition),
		Flexible:  extractFlexible(definition),
	}, nil
}

// ParseIndex parses a single DEFINE INDEX statement into an IndexDefinition.
// Returns an error wrapping ErrSchemaParse when the definition string is empty.
func ParseIndex(indexName, definition string) (IndexDefinition, error) {
	if indexName == "" {
		return IndexDefinition{}, surqlerrors.New(surqlerrors.ErrSchemaParse,
			"index name cannot be empty")
	}
	if strings.TrimSpace(definition) == "" {
		return IndexDefinition{}, surqlerrors.Newf(surqlerrors.ErrSchemaParse,
			"index %q has empty definition", indexName)
	}

	columns := extractIndexColumns(definition)
	if len(columns) == 0 {
		columns = extractIndexFields(definition)
	}

	indexType := extractIndexType(definition)

	idx := IndexDefinition{
		Name:    indexName,
		Columns: columns,
		Type:    indexType,
	}

	switch indexType {
	case IndexTypeMTree:
		idx.Dimension = extractMtreeDimension(definition)
		idx.Distance = extractMtreeDistance(definition)
		idx.VectorType = extractVectorType(definition)
	case IndexTypeHNSW:
		idx.Dimension = extractMtreeDimension(definition)
		idx.VectorType = extractVectorType(definition)
		idx.HnswDistance = extractHnswDistance(definition)
		idx.EFC = extractHnswEFC(definition)
		idx.M = extractHnswM(definition)
	case IndexTypeSearch:
		idx.Analyzer = extractIndexAnalyzer(definition)
		up := strings.ToUpper(definition)
		idx.BM25 = strings.Contains(up, "BM25")
		idx.Highlights = strings.Contains(up, "HIGHLIGHTS")
	}

	return idx, nil
}

// ParseEvent parses a single DEFINE EVENT statement into an EventDefinition.
// Returns an error wrapping ErrSchemaParse when the definition is empty or
// when neither a WHEN clause nor a THEN clause can be located.
func ParseEvent(eventName, definition string) (EventDefinition, error) {
	if eventName == "" {
		return EventDefinition{}, surqlerrors.New(surqlerrors.ErrSchemaParse,
			"event name cannot be empty")
	}
	if strings.TrimSpace(definition) == "" {
		return EventDefinition{}, surqlerrors.Newf(surqlerrors.ErrSchemaParse,
			"event %q has empty definition", eventName)
	}

	condition := extractEventCondition(definition)
	action := extractEventAction(definition)
	if condition == "" || action == "" {
		return EventDefinition{}, surqlerrors.Newf(surqlerrors.ErrSchemaParse,
			"event %q missing WHEN or THEN clause", eventName)
	}

	return EventDefinition{
		Name:      eventName,
		Condition: condition,
		Action:    action,
	}, nil
}

// ParseAccess parses a single DEFINE ACCESS statement into an AccessDefinition.
// Returns an error wrapping ErrSchemaParse when the definition is empty or
// when the access type cannot be determined.
func ParseAccess(accessName, definition string) (AccessDefinition, error) {
	if accessName == "" {
		return AccessDefinition{}, surqlerrors.New(surqlerrors.ErrSchemaParse,
			"access name cannot be empty")
	}
	if strings.TrimSpace(definition) == "" {
		return AccessDefinition{}, surqlerrors.Newf(surqlerrors.ErrSchemaParse,
			"access %q has empty definition", accessName)
	}

	accessType := extractAccessType(definition)
	if !accessType.IsValid() {
		return AccessDefinition{}, surqlerrors.Newf(surqlerrors.ErrSchemaParse,
			"access %q has unknown type", accessName)
	}

	ad := AccessDefinition{
		Name:            accessName,
		Type:            accessType,
		DurationSession: extractAccessDuration(definition, "SESSION"),
		DurationToken:   extractAccessDuration(definition, "TOKEN"),
	}

	switch accessType {
	case AccessTypeJWT:
		ad.JWT = &JwtConfig{
			Algorithm: extractJwtAlgorithm(definition),
			Key:       extractQuoted(definition, "KEY"),
			URL:       extractQuoted(definition, "URL"),
			Issuer:    extractJwtIssuer(definition),
		}
	case AccessTypeRecord:
		ad.Record = &RecordAccessConfig{
			Signup: extractParenClause(definition, "SIGNUP"),
			Signin: extractParenClause(definition, "SIGNIN"),
		}
	}

	return ad, nil
}

// -----------------------------------------------------------------------------
// Internal helpers
// -----------------------------------------------------------------------------

func stringAt(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// extractStringMap returns the first map[string]string-shaped value found
// under any of keys. The underlying value may be map[string]any or
// map[string]string; each entry that is not a string is dropped.
func extractStringMap(info map[string]any, keys ...string) map[string]string {
	if info == nil {
		return nil
	}
	for _, k := range keys {
		raw, ok := info[k]
		if !ok {
			continue
		}
		switch typed := raw.(type) {
		case map[string]string:
			if len(typed) == 0 {
				continue
			}
			out := make(map[string]string, len(typed))
			for k2, v2 := range typed {
				out[k2] = v2
			}
			return out
		case map[string]any:
			if len(typed) == 0 {
				continue
			}
			out := make(map[string]string, len(typed))
			for k2, v2 := range typed {
				if s, ok := v2.(string); ok {
					out[k2] = s
				}
			}
			if len(out) > 0 {
				return out
			}
		}
	}
	return nil
}

// parseTableMode recovers the TableMode from a DEFINE TABLE statement. The
// default (matching surql-py) is SCHEMALESS.
func parseTableMode(def string) TableMode {
	if def == "" {
		return TableModeSchemaless
	}
	up := strings.ToUpper(def)
	switch {
	case strings.Contains(up, "SCHEMAFULL"):
		return TableModeSchemafull
	case strings.Contains(up, "SCHEMALESS"):
		return TableModeSchemaless
	case strings.Contains(up, "DROP"):
		return TableModeDrop
	default:
		return TableModeSchemaless
	}
}

var reRelationFrom = regexp.MustCompile(`(?i)FROM\s+([A-Za-z_][A-Za-z0-9_]*)`)
var reRelationTo = regexp.MustCompile(`(?i)\bTO\s+([A-Za-z_][A-Za-z0-9_]*)`)

func isEdgeDefinitionSource(def string) bool {
	return strings.Contains(strings.ToUpper(def), "TYPE RELATION")
}

func parseEdgeFromDefinition(name, def string) (EdgeDefinition, error) {
	edge := EdgeDefinition{Name: name, Mode: EdgeModeRelation}
	up := strings.ToUpper(def)

	switch {
	case strings.Contains(up, "TYPE RELATION"):
		edge.Mode = EdgeModeRelation
		if m := reRelationFrom.FindStringSubmatch(def); len(m) == 2 {
			edge.FromTable = m[1]
		}
		if m := reRelationTo.FindStringSubmatch(def); len(m) == 2 {
			edge.ToTable = m[1]
		}
	case strings.Contains(up, "SCHEMAFULL"):
		edge.Mode = EdgeModeSchemafull
	case strings.Contains(up, "SCHEMALESS"):
		edge.Mode = EdgeModeSchemaless
	}

	return edge, nil
}

func parseFields(dict map[string]string) []FieldDefinition {
	if len(dict) == 0 {
		return nil
	}
	out := make([]FieldDefinition, 0, len(dict))
	for name, def := range dict {
		fd, err := ParseField(name, def)
		if err != nil {
			continue
		}
		out = append(out, fd)
	}
	return out
}

func parseIndexes(dict map[string]string) []IndexDefinition {
	if len(dict) == 0 {
		return nil
	}
	out := make([]IndexDefinition, 0, len(dict))
	for name, def := range dict {
		idx, err := ParseIndex(name, def)
		if err != nil {
			continue
		}
		out = append(out, idx)
	}
	return out
}

func parseEvents(dict map[string]string) []EventDefinition {
	if len(dict) == 0 {
		return nil
	}
	out := make([]EventDefinition, 0, len(dict))
	for name, def := range dict {
		ev, err := ParseEvent(name, def)
		if err != nil {
			continue
		}
		out = append(out, ev)
	}
	return out
}

// -----------------------------------------------------------------------------
// Field extraction helpers
// -----------------------------------------------------------------------------

// Leading-keyword anchor: match only at start of input OR when the
// previous character is whitespace. This prevents the leading DEFAULT /
// VALUE keyword from matching inside a `$default` / `$value` identifier
// (Go's regexp package has no lookbehind, so the anchor is emitted as a
// prefix group and the captured expression trims it). Same spirit as the
// Python fix in surql-py#8.
var (
	reFieldType      = regexp.MustCompile(`(?i)TYPE\s+([A-Za-z_]\w*)`)
	reFieldAssert    = regexp.MustCompile(`(?is)(?:^|\s)ASSERT\s+(.+?)(?:\s+DEFAULT\b|\s+VALUE\b|\s+READONLY\b|\s+FLEXIBLE\b|\s+PERMISSIONS\b|\s*;|\s*$)`)
	reFieldDefault   = regexp.MustCompile(`(?is)(?:^|\s)DEFAULT\s+(.+?)(?:\s+VALUE\b|\s+READONLY\b|\s+FLEXIBLE\b|\s+PERMISSIONS\b|\s+ASSERT\b|\s*;|\s*$)`)
	reFieldValueExpr = regexp.MustCompile(`(?is)(?:^|\s)VALUE\s+(.+?)(?:\s+DEFAULT\b|\s+READONLY\b|\s+FLEXIBLE\b|\s+PERMISSIONS\b|\s+ASSERT\b|\s*;|\s*$)`)
	reFieldReadOnly  = regexp.MustCompile(`(?i)\bREADONLY\b`)
	reFieldFlexible  = regexp.MustCompile(`(?i)\bFLEXIBLE\b`)
)

var fieldTypeMap = map[string]FieldType{
	"string":   FieldTypeString,
	"int":      FieldTypeInt,
	"float":    FieldTypeFloat,
	"bool":     FieldTypeBool,
	"datetime": FieldTypeDatetime,
	"duration": FieldTypeDuration,
	"decimal":  FieldTypeDecimal,
	"number":   FieldTypeNumber,
	"object":   FieldTypeObject,
	"array":    FieldTypeArray,
	"record":   FieldTypeRecord,
	"geometry": FieldTypeGeometry,
	"any":      FieldTypeAny,
}

func extractFieldType(def string) FieldType {
	m := reFieldType.FindStringSubmatch(def)
	if len(m) != 2 {
		return FieldTypeAny
	}
	if ft, ok := fieldTypeMap[strings.ToLower(m[1])]; ok {
		return ft
	}
	return FieldTypeAny
}

func extractAssertion(def string) string {
	if m := reFieldAssert.FindStringSubmatch(def); len(m) == 2 {
		return strings.TrimSpace(strings.TrimSuffix(m[1], ";"))
	}
	return ""
}

func extractDefault(def string) string {
	if m := reFieldDefault.FindStringSubmatch(def); len(m) == 2 {
		return strings.TrimSpace(strings.TrimSuffix(m[1], ";"))
	}
	return ""
}

func extractValue(def string) string {
	if m := reFieldValueExpr.FindStringSubmatch(def); len(m) == 2 {
		return strings.TrimSpace(strings.TrimSuffix(m[1], ";"))
	}
	return ""
}

func extractReadOnly(def string) bool {
	return reFieldReadOnly.MatchString(def)
}

func extractFlexible(def string) bool {
	return reFieldFlexible.MatchString(def)
}

// -----------------------------------------------------------------------------
// Index extraction helpers
// -----------------------------------------------------------------------------

var (
	reIndexColumns = regexp.MustCompile(`(?i)COLUMNS\s+([^;]+?)(?:\s+UNIQUE\b|\s+FULLTEXT\b|\s+SEARCH\b|\s+HNSW\b|\s+MTREE\b|\s*;|\s*$)`)
	reIndexFields  = regexp.MustCompile(`(?i)FIELDS\s+([^;]+?)(?:\s+UNIQUE\b|\s+FULLTEXT\b|\s+SEARCH\b|\s+HNSW\b|\s+MTREE\b|\s*;|\s*$)`)
	reIndexDim     = regexp.MustCompile(`(?i)DIMENSION\s+(\d+)`)
	reIndexDist    = regexp.MustCompile(`(?i)(?:DIST|DISTANCE)\s+([A-Za-z_]\w*)`)
	reVectorType   = regexp.MustCompile(`(?i)TYPE\s+([A-Za-z_]\w*)`)
	reIndexEFC     = regexp.MustCompile(`(?i)\bEFC\s+(\d+)`)
	reIndexM       = regexp.MustCompile(`(?i)\bM\s+(\d+)`)
	reIndexAnlzr   = regexp.MustCompile(`(?i)ANALYZER\s+(\w+)`)
)

func splitColumns(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func extractIndexColumns(def string) []string {
	if m := reIndexColumns.FindStringSubmatch(def); len(m) == 2 {
		return splitColumns(m[1])
	}
	return nil
}

func extractIndexFields(def string) []string {
	if m := reIndexFields.FindStringSubmatch(def); len(m) == 2 {
		return splitColumns(m[1])
	}
	return nil
}

func extractIndexType(def string) IndexType {
	up := strings.ToUpper(def)
	switch {
	case strings.Contains(up, "UNIQUE"):
		return IndexTypeUnique
	case strings.Contains(up, "HNSW"):
		return IndexTypeHNSW
	case strings.Contains(up, "MTREE"):
		return IndexTypeMTree
	case strings.Contains(up, "FULLTEXT"), strings.Contains(up, "SEARCH"):
		// SurrealDB 3.x renamed the full-text keyword SEARCH -> FULLTEXT;
		// recognise both spellings so v1/v2 and v3 INFO output round-trip.
		return IndexTypeSearch
	default:
		return IndexTypeStandard
	}
}

// extractIndexAnalyzer pulls the `ANALYZER <name>` analyzer name out of a
// full-text DEFINE INDEX statement. The historical `ascii` default (what a
// plain SearchIndex renders) normalises back to "" so a round-trip of the
// default form is an identity, leaving an explicit non-`ascii` analyzer set.
func extractIndexAnalyzer(def string) string {
	if m := reIndexAnlzr.FindStringSubmatch(def); len(m) == 2 {
		if strings.EqualFold(m[1], "ascii") {
			return ""
		}
		return m[1]
	}
	return ""
}

func extractMtreeDimension(def string) int {
	if m := reIndexDim.FindStringSubmatch(def); len(m) == 2 {
		if n, err := strconv.Atoi(m[1]); err == nil {
			return n
		}
	}
	return 0
}

var mtreeDistanceMap = map[string]MTreeDistanceType{
	"COSINE":    MTreeDistanceCosine,
	"EUCLIDEAN": MTreeDistanceEuclidean,
	"MANHATTAN": MTreeDistanceManhattan,
	"MINKOWSKI": MTreeDistanceMinkowski,
}

func extractMtreeDistance(def string) MTreeDistanceType {
	if m := reIndexDist.FindStringSubmatch(def); len(m) == 2 {
		if d, ok := mtreeDistanceMap[strings.ToUpper(m[1])]; ok {
			return d
		}
	}
	return ""
}

var hnswDistanceMap = map[string]HnswDistanceType{
	"CHEBYSHEV": HnswDistanceChebyshev,
	"COSINE":    HnswDistanceCosine,
	"EUCLIDEAN": HnswDistanceEuclidean,
	"HAMMING":   HnswDistanceHamming,
	"JACCARD":   HnswDistanceJaccard,
	"MANHATTAN": HnswDistanceManhattan,
	"MINKOWSKI": HnswDistanceMinkowski,
	"PEARSON":   HnswDistancePearson,
}

func extractHnswDistance(def string) HnswDistanceType {
	if m := reIndexDist.FindStringSubmatch(def); len(m) == 2 {
		if d, ok := hnswDistanceMap[strings.ToUpper(m[1])]; ok {
			return d
		}
	}
	return ""
}

var vectorTypeMap = map[string]MTreeVectorType{
	"F64": MTreeVectorF64,
	"F32": MTreeVectorF32,
	"I64": MTreeVectorI64,
	"I32": MTreeVectorI32,
	"I16": MTreeVectorI16,
}

func extractVectorType(def string) MTreeVectorType {
	if m := reVectorType.FindStringSubmatch(def); len(m) == 2 {
		if v, ok := vectorTypeMap[strings.ToUpper(m[1])]; ok {
			return v
		}
	}
	return ""
}

func extractHnswEFC(def string) int {
	if m := reIndexEFC.FindStringSubmatch(def); len(m) == 2 {
		if n, err := strconv.Atoi(m[1]); err == nil {
			return n
		}
	}
	return 0
}

func extractHnswM(def string) int {
	if m := reIndexM.FindStringSubmatch(def); len(m) == 2 {
		if n, err := strconv.Atoi(m[1]); err == nil {
			return n
		}
	}
	return 0
}

// -----------------------------------------------------------------------------
// Event extraction helpers
// -----------------------------------------------------------------------------

var (
	reEventWhen = regexp.MustCompile(`(?is)WHEN\s+(.+?)\s+THEN\b`)
	reEventThen = regexp.MustCompile(`(?is)THEN\s+(?:\{\s*(.+?)\s*\}|(.+?))(?:\s*;|\s*$)`)
)

func extractEventCondition(def string) string {
	if m := reEventWhen.FindStringSubmatch(def); len(m) == 2 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func extractEventAction(def string) string {
	m := reEventThen.FindStringSubmatch(def)
	if len(m) < 3 {
		return ""
	}
	action := m[1]
	if action == "" {
		action = m[2]
	}
	return strings.TrimSpace(action)
}

// -----------------------------------------------------------------------------
// Access extraction helpers
// -----------------------------------------------------------------------------

var (
	reAccessType    = regexp.MustCompile(`(?i)TYPE\s+(JWT|RECORD)\b`)
	reJwtAlgorithm  = regexp.MustCompile(`(?i)ALGORITHM\s+([A-Za-z0-9]+)`)
	reJwtIssuer     = regexp.MustCompile(`(?i)WITH\s+ISSUER\s+'([^']*)'`)
	reDurationBlock = regexp.MustCompile(`(?is)DURATION\s+(.+?)(?:\s*;|\s*$)`)
	rePerSession    = regexp.MustCompile(`(?i)FOR\s+SESSION\s+([^,;]+?)(?:\s*,|\s*$)`)
	rePerToken      = regexp.MustCompile(`(?i)FOR\s+TOKEN\s+([^,;]+?)(?:\s*,|\s*$)`)
)

func extractAccessType(def string) AccessType {
	if m := reAccessType.FindStringSubmatch(def); len(m) == 2 {
		switch strings.ToUpper(m[1]) {
		case "JWT":
			return AccessTypeJWT
		case "RECORD":
			return AccessTypeRecord
		}
	}
	return ""
}

func extractJwtAlgorithm(def string) string {
	if m := reJwtAlgorithm.FindStringSubmatch(def); len(m) == 2 {
		return strings.ToUpper(m[1])
	}
	return ""
}

func extractJwtIssuer(def string) string {
	if m := reJwtIssuer.FindStringSubmatch(def); len(m) == 2 {
		return m[1]
	}
	return ""
}

// extractQuoted pulls KEYWORD 'value' out of a DEFINE ACCESS body.
func extractQuoted(def, keyword string) string {
	re := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(keyword) + `\s+'([^']*)'`)
	if m := re.FindStringSubmatch(def); len(m) == 2 {
		return m[1]
	}
	return ""
}

// extractParenClause pulls KEYWORD (expression) out of a DEFINE ACCESS body
// handling nested parentheses.
func extractParenClause(def, keyword string) string {
	up := strings.ToUpper(def)
	kw := strings.ToUpper(keyword)
	idx := strings.Index(up, kw)
	if idx < 0 {
		return ""
	}
	rest := def[idx+len(kw):]
	// skip whitespace
	i := 0
	for i < len(rest) && (rest[i] == ' ' || rest[i] == '\t' || rest[i] == '\n' || rest[i] == '\r') {
		i++
	}
	if i >= len(rest) || rest[i] != '(' {
		return ""
	}
	depth := 0
	start := i + 1
	for j := i; j < len(rest); j++ {
		switch rest[j] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return strings.TrimSpace(rest[start:j])
			}
		}
	}
	return ""
}

func extractAccessDuration(def, clause string) string {
	block := reDurationBlock.FindStringSubmatch(def)
	if len(block) != 2 {
		return ""
	}
	body := block[1]
	var re *regexp.Regexp
	switch strings.ToUpper(clause) {
	case "SESSION":
		re = rePerSession
	case "TOKEN":
		re = rePerToken
	default:
		return ""
	}
	// Append trailing comma so the regex's "," terminator matches a lone clause.
	if m := re.FindStringSubmatch(body + ","); len(m) == 2 {
		return strings.TrimSpace(m[1])
	}
	return ""
}
