package schema

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	surqlerrors "github.com/albedosehen/surql-go/pkg/surql/errors"
)

// ValidationSeverity enumerates the severity of a ValidationResult.
type ValidationSeverity string

// Severity values.
const (
	// SeverityError indicates a breaking issue requiring remediation.
	SeverityError ValidationSeverity = "error"
	// SeverityWarning indicates a non-critical inconsistency.
	SeverityWarning ValidationSeverity = "warning"
	// SeverityInfo indicates an informational finding only.
	SeverityInfo ValidationSeverity = "info"
)

// IsValid reports whether the severity value is recognised.
func (s ValidationSeverity) IsValid() bool {
	switch s {
	case SeverityError, SeverityWarning, SeverityInfo:
		return true
	}
	return false
}

// ValidationResult is a single finding produced by the validator.
type ValidationResult struct {
	Severity  ValidationSeverity
	Table     string
	Field     string
	Message   string
	CodeValue string
	DBValue   string
}

// String returns a human-readable representation of the result.
func (r ValidationResult) String() string {
	var b strings.Builder
	b.WriteString("[")
	b.WriteString(strings.ToUpper(string(r.Severity)))
	b.WriteString("] ")
	b.WriteString(r.Table)
	if r.Field != "" {
		b.WriteString(".")
		b.WriteString(r.Field)
	}
	b.WriteString(": ")
	b.WriteString(r.Message)
	if r.CodeValue != "" || r.DBValue != "" {
		b.WriteString(" (code: ")
		b.WriteString(r.CodeValue)
		b.WriteString(", db: ")
		b.WriteString(r.DBValue)
		b.WriteString(")")
	}
	return b.String()
}

// ValidationReport aggregates ValidationResults and exposes convenience
// accessors for downstream tooling (CLI reporters, migration planners).
type ValidationReport struct {
	Results []ValidationResult
}

// NewValidationReport constructs an empty report.
func NewValidationReport() *ValidationReport {
	return &ValidationReport{}
}

// Add appends a single result to the report.
func (vr *ValidationReport) Add(r ValidationResult) {
	if vr == nil {
		return
	}
	vr.Results = append(vr.Results, r)
}

// AddAll appends multiple results to the report.
func (vr *ValidationReport) AddAll(rs []ValidationResult) {
	if vr == nil {
		return
	}
	vr.Results = append(vr.Results, rs...)
}

// Merge appends every result from other into vr. It is a no-op when either
// report is nil.
func (vr *ValidationReport) Merge(other *ValidationReport) {
	if vr == nil || other == nil {
		return
	}
	vr.Results = append(vr.Results, other.Results...)
}

// Errors returns only ERROR-severity results.
func (vr *ValidationReport) Errors() []ValidationResult {
	if vr == nil {
		return nil
	}
	return FilterBySeverity(vr.Results, SeverityError)
}

// Warnings returns only WARNING-severity results.
func (vr *ValidationReport) Warnings() []ValidationResult {
	if vr == nil {
		return nil
	}
	return FilterBySeverity(vr.Results, SeverityWarning)
}

// Infos returns only INFO-severity results.
func (vr *ValidationReport) Infos() []ValidationResult {
	if vr == nil {
		return nil
	}
	return FilterBySeverity(vr.Results, SeverityInfo)
}

// HasErrors reports whether any ERROR-severity result is present.
func (vr *ValidationReport) HasErrors() bool {
	if vr == nil {
		return false
	}
	return HasErrors(vr.Results)
}

// IsValid is the inverse of HasErrors.
func (vr *ValidationReport) IsValid() bool {
	return !vr.HasErrors()
}

// Len returns the total number of results in the report.
func (vr *ValidationReport) Len() int {
	if vr == nil {
		return 0
	}
	return len(vr.Results)
}

// ValidateSchema performs cross-schema validation over every table, edge, and
// access definition currently registered in r. It returns a ValidationReport
// describing every inconsistency. A hard error is returned only when r is nil.
//
// The Access parameter is optional and may be supplied via the variadic accesses
// argument for callers that manage access definitions outside the registry
// (the current SchemaRegistry tracks tables and edges only).
func ValidateSchema(r *SchemaRegistry, accesses ...AccessDefinition) (*ValidationReport, error) {
	if r == nil {
		return nil, surqlerrors.New(surqlerrors.ErrValidation,
			"cannot validate schema: registry is nil")
	}

	report := NewValidationReport()
	tables := r.Tables()
	edges := r.Edges()

	// Per-definition structural validation.
	for _, t := range tables {
		report.Merge(ValidateTable(t))
	}
	for _, e := range edges {
		report.Merge(ValidateEdge(e))
	}
	for _, a := range accesses {
		report.Merge(ValidateAccess(a))
	}

	// Cross-definition checks.
	report.AddAll(detectNameCollisions(tables, edges))
	report.AddAll(detectRecordFieldTargets(tables, edges))
	report.AddAll(detectEdgeTableReferences(tables, edges))
	report.AddAll(detectDuplicateAccessNames(accesses))

	return report, nil
}

// ValidateTable performs internal consistency checks on a single table
// definition. It never returns a hard error; every issue is expressed as a
// ValidationResult so callers can display the full set.
func ValidateTable(t TableDefinition) *ValidationReport {
	report := NewValidationReport()

	if t.Name == "" {
		report.Add(ValidationResult{
			Severity: SeverityError,
			Message:  "table name cannot be empty",
		})
		return report
	}

	if !t.Mode.IsValid() {
		report.Add(ValidationResult{
			Severity:  SeverityError,
			Table:     t.Name,
			Message:   "invalid table mode",
			CodeValue: string(t.Mode),
		})
	}

	report.AddAll(validateFieldList(t.Name, t.Fields, false))
	report.AddAll(validateIndexList(t.Name, t.Indexes, t.Fields))
	report.AddAll(validateEventList(t.Name, t.Events))
	report.AddAll(validatePermissions(t.Name, t.Permissions))

	return report
}

// ValidateEdge performs internal consistency checks on a single edge
// definition, including RELATION-mode requirements for from/to tables.
func ValidateEdge(e EdgeDefinition) *ValidationReport {
	report := NewValidationReport()

	if e.Name == "" {
		report.Add(ValidationResult{
			Severity: SeverityError,
			Message:  "edge name cannot be empty",
		})
		return report
	}

	if !e.Mode.IsValid() {
		report.Add(ValidationResult{
			Severity:  SeverityError,
			Table:     e.Name,
			Message:   "invalid edge mode",
			CodeValue: string(e.Mode),
		})
	}

	if e.Mode == EdgeModeRelation {
		if e.FromTable == "" {
			report.Add(ValidationResult{
				Severity: SeverityError,
				Table:    e.Name,
				Message:  "edge with RELATION mode requires a from_table",
			})
		}
		if e.ToTable == "" {
			report.Add(ValidationResult{
				Severity: SeverityError,
				Table:    e.Name,
				Message:  "edge with RELATION mode requires a to_table",
			})
		}
	}

	// Warn about reserved identifier usage (in/out are permitted on edges).
	report.AddAll(validateFieldList(e.Name, e.Fields, true))
	report.AddAll(validateIndexList(e.Name, e.Indexes, e.Fields))
	report.AddAll(validateEventList(e.Name, e.Events))
	report.AddAll(validatePermissions(e.Name, e.Permissions))

	return report
}

// ValidateAccess performs internal consistency checks on a single access
// definition.
func ValidateAccess(a AccessDefinition) *ValidationReport {
	report := NewValidationReport()

	if a.Name == "" {
		report.Add(ValidationResult{
			Severity: SeverityError,
			Message:  "access name cannot be empty",
		})
		return report
	}

	if !a.Type.IsValid() {
		report.Add(ValidationResult{
			Severity:  SeverityError,
			Table:     a.Name,
			Message:   "invalid access type",
			CodeValue: string(a.Type),
		})
	}

	switch a.Type {
	case AccessTypeJWT:
		if a.JWT == nil {
			report.Add(ValidationResult{
				Severity: SeverityError,
				Table:    a.Name,
				Message:  "JWT access requires a JWT configuration",
			})
		} else {
			if a.JWT.Key == "" && a.JWT.URL == "" {
				report.Add(ValidationResult{
					Severity: SeverityError,
					Table:    a.Name,
					Message:  "JWT access requires either a KEY or a URL",
				})
			}
			if a.JWT.Key != "" && a.JWT.URL != "" {
				report.Add(ValidationResult{
					Severity: SeverityWarning,
					Table:    a.Name,
					Message:  "JWT access has both KEY and URL; URL takes precedence",
				})
			}
		}
		if a.Record != nil {
			report.Add(ValidationResult{
				Severity: SeverityWarning,
				Table:    a.Name,
				Message:  "JWT access should not carry a record configuration",
			})
		}
	case AccessTypeRecord:
		if a.Record == nil {
			report.Add(ValidationResult{
				Severity: SeverityError,
				Table:    a.Name,
				Message:  "RECORD access requires a record configuration",
			})
		} else if a.Record.Signup == "" && a.Record.Signin == "" {
			report.Add(ValidationResult{
				Severity: SeverityWarning,
				Table:    a.Name,
				Message:  "RECORD access declares neither SIGNUP nor SIGNIN",
			})
		}
		if a.JWT != nil {
			report.Add(ValidationResult{
				Severity: SeverityWarning,
				Table:    a.Name,
				Message:  "RECORD access should not carry a JWT configuration",
			})
		}
	}

	return report
}

// CompareSchemas reports drift between a code-defined schema and the schema
// extracted from a live database. It mirrors the semantics of surql-py's
// validate_schema but consumes pre-fetched TableDefinition / EdgeDefinition
// slices so that the caller owns database I/O.
//
// Missing / extra / mismatched tables, fields, indexes, and events are
// surfaced with the severity levels documented in the Python implementation.
func CompareSchemas(
	codeTables, dbTables []TableDefinition,
	codeEdges, dbEdges []EdgeDefinition,
) *ValidationReport {
	report := NewValidationReport()

	codeByName := tablesByName(codeTables)
	dbByName := tablesByName(dbTables)

	// Missing / extra tables.
	for _, name := range sortedKeys(codeByName) {
		if _, ok := dbByName[name]; !ok {
			report.Add(ValidationResult{
				Severity:  SeverityError,
				Table:     name,
				Message:   "Table defined in code but missing from database",
				CodeValue: "exists",
				DBValue:   "missing",
			})
		}
	}
	for _, name := range sortedKeys(dbByName) {
		if _, ok := codeByName[name]; !ok {
			report.Add(ValidationResult{
				Severity:  SeverityWarning,
				Table:     name,
				Message:   "Table exists in database but not defined in code",
				CodeValue: "missing",
				DBValue:   "exists",
			})
		}
	}

	// Common tables: deep compare.
	for _, name := range sortedKeys(codeByName) {
		db, ok := dbByName[name]
		if !ok {
			continue
		}
		report.AddAll(diffTable(codeByName[name], db))
	}

	// Edges.
	codeEdgeByName := edgesByName(codeEdges)
	dbEdgeByName := edgesByName(dbEdges)

	for _, name := range sortedKeys(codeEdgeByName) {
		if _, ok := dbEdgeByName[name]; !ok {
			report.Add(ValidationResult{
				Severity:  SeverityError,
				Table:     name,
				Message:   "Edge defined in code but missing from database",
				CodeValue: "exists",
				DBValue:   "missing",
			})
		}
	}
	for _, name := range sortedKeys(dbEdgeByName) {
		if _, ok := codeEdgeByName[name]; !ok {
			report.Add(ValidationResult{
				Severity:  SeverityWarning,
				Table:     name,
				Message:   "Edge exists in database but not defined in code",
				CodeValue: "missing",
				DBValue:   "exists",
			})
		}
	}

	for _, name := range sortedKeys(codeEdgeByName) {
		db, ok := dbEdgeByName[name]
		if !ok {
			continue
		}
		report.AddAll(diffEdge(codeEdgeByName[name], db))
	}

	return report
}

// diffTable computes the drift between two TableDefinition objects.
func diffTable(code, db TableDefinition) []ValidationResult {
	var results []ValidationResult

	if code.Mode != db.Mode {
		results = append(results, ValidationResult{
			Severity:  SeverityError,
			Table:     code.Name,
			Message:   "Table mode mismatch",
			CodeValue: string(code.Mode),
			DBValue:   string(db.Mode),
		})
	}

	results = append(results, diffFields(code.Name, code.Fields, db.Fields)...)
	results = append(results, diffIndexes(code.Name, code.Indexes, db.Indexes)...)
	results = append(results, diffEvents(code.Name, code.Events, db.Events)...)

	return results
}

// diffEdge computes the drift between two EdgeDefinition objects.
func diffEdge(code, db EdgeDefinition) []ValidationResult {
	var results []ValidationResult

	if code.Mode != db.Mode {
		results = append(results, ValidationResult{
			Severity:  SeverityError,
			Table:     code.Name,
			Message:   "Edge mode mismatch",
			CodeValue: string(code.Mode),
			DBValue:   string(db.Mode),
		})
	}
	if code.FromTable != db.FromTable {
		results = append(results, ValidationResult{
			Severity:  SeverityError,
			Table:     code.Name,
			Message:   "Edge from_table mismatch",
			CodeValue: code.FromTable,
			DBValue:   db.FromTable,
		})
	}
	if code.ToTable != db.ToTable {
		results = append(results, ValidationResult{
			Severity:  SeverityError,
			Table:     code.Name,
			Message:   "Edge to_table mismatch",
			CodeValue: code.ToTable,
			DBValue:   db.ToTable,
		})
	}

	results = append(results, diffFields(code.Name, code.Fields, db.Fields)...)
	results = append(results, diffIndexes(code.Name, code.Indexes, db.Indexes)...)
	results = append(results, diffEvents(code.Name, code.Events, db.Events)...)

	return results
}

// diffFields enumerates missing / extra / mismatched fields between code and
// database field slices for a given container (table or edge).
func diffFields(container string, code, db []FieldDefinition) []ValidationResult {
	var results []ValidationResult

	codeByName := fieldsByName(code)
	dbByName := fieldsByName(db)

	for _, name := range sortedKeys(codeByName) {
		if _, ok := dbByName[name]; !ok {
			results = append(results, ValidationResult{
				Severity:  SeverityError,
				Table:     container,
				Field:     name,
				Message:   "Field defined in code but missing from database",
				CodeValue: "exists",
				DBValue:   "missing",
			})
		}
	}
	for _, name := range sortedKeys(dbByName) {
		if _, ok := codeByName[name]; !ok {
			results = append(results, ValidationResult{
				Severity:  SeverityWarning,
				Table:     container,
				Field:     name,
				Message:   "Field exists in database but not defined in code",
				CodeValue: "missing",
				DBValue:   "exists",
			})
		}
	}
	for _, name := range sortedKeys(codeByName) {
		dbField, ok := dbByName[name]
		if !ok {
			continue
		}
		results = append(results, diffField(container, codeByName[name], dbField)...)
	}

	return results
}

// diffField compares a single pair of FieldDefinition objects.
func diffField(container string, code, db FieldDefinition) []ValidationResult {
	var results []ValidationResult

	if code.Type != db.Type {
		results = append(results, ValidationResult{
			Severity:  SeverityError,
			Table:     container,
			Field:     code.Name,
			Message:   "Field type mismatch",
			CodeValue: string(code.Type),
			DBValue:   string(db.Type),
		})
	}
	if normalizeExpression(code.Assertion) != normalizeExpression(db.Assertion) {
		results = append(results, ValidationResult{
			Severity:  SeverityWarning,
			Table:     container,
			Field:     code.Name,
			Message:   "Field assertion mismatch",
			CodeValue: code.Assertion,
			DBValue:   db.Assertion,
		})
	}
	if normalizeExpression(code.Default) != normalizeExpression(db.Default) {
		results = append(results, ValidationResult{
			Severity:  SeverityWarning,
			Table:     container,
			Field:     code.Name,
			Message:   "Field default value mismatch",
			CodeValue: code.Default,
			DBValue:   db.Default,
		})
	}
	if normalizeExpression(code.Value) != normalizeExpression(db.Value) {
		results = append(results, ValidationResult{
			Severity:  SeverityWarning,
			Table:     container,
			Field:     code.Name,
			Message:   "Field computed value mismatch",
			CodeValue: code.Value,
			DBValue:   db.Value,
		})
	}
	if code.ReadOnly != db.ReadOnly {
		results = append(results, ValidationResult{
			Severity:  SeverityInfo,
			Table:     container,
			Field:     code.Name,
			Message:   "Field readonly flag mismatch",
			CodeValue: strconv.FormatBool(code.ReadOnly),
			DBValue:   strconv.FormatBool(db.ReadOnly),
		})
	}
	if code.Flexible != db.Flexible {
		results = append(results, ValidationResult{
			Severity:  SeverityInfo,
			Table:     container,
			Field:     code.Name,
			Message:   "Field flexible flag mismatch",
			CodeValue: strconv.FormatBool(code.Flexible),
			DBValue:   strconv.FormatBool(db.Flexible),
		})
	}

	return results
}

// diffIndexes enumerates missing / extra / mismatched indexes.
func diffIndexes(container string, code, db []IndexDefinition) []ValidationResult {
	var results []ValidationResult

	codeByName := indexesByName(code)
	dbByName := indexesByName(db)

	for _, name := range sortedKeys(codeByName) {
		if _, ok := dbByName[name]; !ok {
			results = append(results, ValidationResult{
				Severity:  SeverityError,
				Table:     container,
				Field:     "index:" + name,
				Message:   "Index defined in code but missing from database",
				CodeValue: "exists",
				DBValue:   "missing",
			})
		}
	}
	for _, name := range sortedKeys(dbByName) {
		if _, ok := codeByName[name]; !ok {
			results = append(results, ValidationResult{
				Severity:  SeverityWarning,
				Table:     container,
				Field:     "index:" + name,
				Message:   "Index exists in database but not defined in code",
				CodeValue: "missing",
				DBValue:   "exists",
			})
		}
	}
	for _, name := range sortedKeys(codeByName) {
		dbIdx, ok := dbByName[name]
		if !ok {
			continue
		}
		results = append(results, diffIndex(container, codeByName[name], dbIdx)...)
	}

	return results
}

// diffIndex compares a single pair of IndexDefinition objects.
func diffIndex(container string, code, db IndexDefinition) []ValidationResult {
	var results []ValidationResult
	indexField := "index:" + code.Name

	if code.Type != db.Type {
		results = append(results, ValidationResult{
			Severity:  SeverityError,
			Table:     container,
			Field:     indexField,
			Message:   "Index type mismatch",
			CodeValue: string(code.Type),
			DBValue:   string(db.Type),
		})
	}
	if !sortedSliceEqual(code.Columns, db.Columns) {
		results = append(results, ValidationResult{
			Severity:  SeverityError,
			Table:     container,
			Field:     indexField,
			Message:   "Index columns mismatch",
			CodeValue: strings.Join(code.Columns, ","),
			DBValue:   strings.Join(db.Columns, ","),
		})
	}

	if code.Type == IndexTypeMTree && db.Type == IndexTypeMTree {
		results = append(results, diffMTreeParams(container, code, db)...)
	}
	if code.Type == IndexTypeHNSW && db.Type == IndexTypeHNSW {
		results = append(results, diffHnswParams(container, code, db)...)
	}

	return results
}

func diffMTreeParams(container string, code, db IndexDefinition) []ValidationResult {
	var results []ValidationResult
	indexField := "index:" + code.Name

	if code.Dimension != db.Dimension {
		results = append(results, ValidationResult{
			Severity:  SeverityError,
			Table:     container,
			Field:     indexField,
			Message:   "MTREE index dimension mismatch",
			CodeValue: strconv.Itoa(code.Dimension),
			DBValue:   strconv.Itoa(db.Dimension),
		})
	}
	if code.Distance != db.Distance {
		results = append(results, ValidationResult{
			Severity:  SeverityWarning,
			Table:     container,
			Field:     indexField,
			Message:   "MTREE index distance metric mismatch",
			CodeValue: string(code.Distance),
			DBValue:   string(db.Distance),
		})
	}
	if code.VectorType != db.VectorType {
		results = append(results, ValidationResult{
			Severity:  SeverityWarning,
			Table:     container,
			Field:     indexField,
			Message:   "MTREE index vector type mismatch",
			CodeValue: string(code.VectorType),
			DBValue:   string(db.VectorType),
		})
	}
	return results
}

func diffHnswParams(container string, code, db IndexDefinition) []ValidationResult {
	var results []ValidationResult
	indexField := "index:" + code.Name

	if code.Dimension != db.Dimension {
		results = append(results, ValidationResult{
			Severity:  SeverityError,
			Table:     container,
			Field:     indexField,
			Message:   "HNSW index dimension mismatch",
			CodeValue: strconv.Itoa(code.Dimension),
			DBValue:   strconv.Itoa(db.Dimension),
		})
	}
	if code.HnswDistance != db.HnswDistance {
		results = append(results, ValidationResult{
			Severity:  SeverityWarning,
			Table:     container,
			Field:     indexField,
			Message:   "HNSW index distance metric mismatch",
			CodeValue: string(code.HnswDistance),
			DBValue:   string(db.HnswDistance),
		})
	}
	if code.VectorType != db.VectorType {
		results = append(results, ValidationResult{
			Severity:  SeverityWarning,
			Table:     container,
			Field:     indexField,
			Message:   "HNSW index vector type mismatch",
			CodeValue: string(code.VectorType),
			DBValue:   string(db.VectorType),
		})
	}
	if code.EFC != db.EFC {
		results = append(results, ValidationResult{
			Severity:  SeverityWarning,
			Table:     container,
			Field:     indexField,
			Message:   "HNSW index EFC mismatch",
			CodeValue: strconv.Itoa(code.EFC),
			DBValue:   strconv.Itoa(db.EFC),
		})
	}
	if code.M != db.M {
		results = append(results, ValidationResult{
			Severity:  SeverityWarning,
			Table:     container,
			Field:     indexField,
			Message:   "HNSW index M mismatch",
			CodeValue: strconv.Itoa(code.M),
			DBValue:   strconv.Itoa(db.M),
		})
	}
	return results
}

// diffEvents enumerates missing / extra events between two event slices.
func diffEvents(container string, code, db []EventDefinition) []ValidationResult {
	var results []ValidationResult

	codeByName := eventsByName(code)
	dbByName := eventsByName(db)

	for _, name := range sortedKeys(codeByName) {
		if _, ok := dbByName[name]; !ok {
			results = append(results, ValidationResult{
				Severity:  SeverityError,
				Table:     container,
				Field:     "event:" + name,
				Message:   "Event defined in code but missing from database",
				CodeValue: "exists",
				DBValue:   "missing",
			})
		}
	}
	for _, name := range sortedKeys(dbByName) {
		if _, ok := codeByName[name]; !ok {
			results = append(results, ValidationResult{
				Severity:  SeverityWarning,
				Table:     container,
				Field:     "event:" + name,
				Message:   "Event exists in database but not defined in code",
				CodeValue: "missing",
				DBValue:   "exists",
			})
		}
	}

	return results
}

// Structural helpers.

// validateFieldList checks per-field invariants plus duplicate-name detection.
func validateFieldList(container string, fields []FieldDefinition, allowEdgeReserved bool) []ValidationResult {
	var results []ValidationResult
	seen := make(map[string]struct{}, len(fields))

	for _, f := range fields {
		if _, dup := seen[f.Name]; dup && f.Name != "" {
			results = append(results, ValidationResult{
				Severity: SeverityError,
				Table:    container,
				Field:    f.Name,
				Message:  "duplicate field name",
			})
			continue
		}
		seen[f.Name] = struct{}{}

		if err := f.Validate(); err != nil {
			results = append(results, ValidationResult{
				Severity: SeverityError,
				Table:    container,
				Field:    f.Name,
				Message:  err.Error(),
			})
			continue
		}

		if warn := f.ReservedWarning(allowEdgeReserved); warn != "" {
			results = append(results, ValidationResult{
				Severity: SeverityWarning,
				Table:    container,
				Field:    f.Name,
				Message:  warn,
			})
		}
	}

	return results
}

// validateIndexList checks per-index invariants, duplicate-name detection, and
// that every index column references a known field (when the table is
// SCHEMAFULL and fields are supplied).
func validateIndexList(container string, indexes []IndexDefinition, fields []FieldDefinition) []ValidationResult {
	var results []ValidationResult
	seen := make(map[string]struct{}, len(indexes))
	fieldSet := make(map[string]struct{}, len(fields))
	for _, f := range fields {
		fieldSet[f.Name] = struct{}{}
	}

	for _, idx := range indexes {
		if _, dup := seen[idx.Name]; dup && idx.Name != "" {
			results = append(results, ValidationResult{
				Severity: SeverityError,
				Table:    container,
				Field:    "index:" + idx.Name,
				Message:  "duplicate index name",
			})
			continue
		}
		seen[idx.Name] = struct{}{}

		if err := idx.Validate(); err != nil {
			results = append(results, ValidationResult{
				Severity: SeverityError,
				Table:    container,
				Field:    "index:" + idx.Name,
				Message:  err.Error(),
			})
			continue
		}

		// Warn when an index references a column absent from the field list
		// (fields may be empty for SCHEMALESS tables, so only warn when we
		// have a non-empty field set to compare against).
		if len(fieldSet) == 0 {
			continue
		}
		for _, col := range idx.Columns {
			if _, ok := fieldSet[col]; !ok {
				results = append(results, ValidationResult{
					Severity:  SeverityWarning,
					Table:     container,
					Field:     "index:" + idx.Name,
					Message:   fmt.Sprintf("index references unknown column %q", col),
					CodeValue: col,
				})
			}
		}
	}

	return results
}

// validateEventList checks per-event invariants plus duplicate-name detection.
func validateEventList(container string, events []EventDefinition) []ValidationResult {
	var results []ValidationResult
	seen := make(map[string]struct{}, len(events))

	for _, e := range events {
		if _, dup := seen[e.Name]; dup && e.Name != "" {
			results = append(results, ValidationResult{
				Severity: SeverityError,
				Table:    container,
				Field:    "event:" + e.Name,
				Message:  "duplicate event name",
			})
			continue
		}
		seen[e.Name] = struct{}{}

		if err := e.Validate(); err != nil {
			results = append(results, ValidationResult{
				Severity: SeverityError,
				Table:    container,
				Field:    "event:" + e.Name,
				Message:  err.Error(),
			})
		}
	}

	return results
}

// validatePermissions surfaces malformed permission entries.
func validatePermissions(container string, perms map[string]string) []ValidationResult {
	var results []ValidationResult
	if len(perms) == 0 {
		return nil
	}
	allowed := map[string]struct{}{
		"select": {},
		"create": {},
		"update": {},
		"delete": {},
	}
	for _, action := range sortedKeysString(perms) {
		expr := perms[action]
		lower := strings.ToLower(action)
		if _, ok := allowed[lower]; !ok {
			results = append(results, ValidationResult{
				Severity:  SeverityWarning,
				Table:     container,
				Field:     "permissions:" + action,
				Message:   "unknown permission action",
				CodeValue: action,
			})
		}
		if strings.TrimSpace(expr) == "" {
			results = append(results, ValidationResult{
				Severity: SeverityError,
				Table:    container,
				Field:    "permissions:" + action,
				Message:  "permission expression cannot be empty",
			})
		}
	}
	return results
}

// detectNameCollisions reports tables and edges that share a name.
func detectNameCollisions(tables []TableDefinition, edges []EdgeDefinition) []ValidationResult {
	var results []ValidationResult
	tableNames := make(map[string]struct{}, len(tables))
	for _, t := range tables {
		tableNames[t.Name] = struct{}{}
	}
	for _, e := range edges {
		if _, clash := tableNames[e.Name]; clash {
			results = append(results, ValidationResult{
				Severity: SeverityError,
				Table:    e.Name,
				Message:  "name conflict: edge and table share the same name",
			})
		}
	}
	return results
}

// detectRecordFieldTargets warns when a record-typed field references an
// unknown table. The Python implementation does not perform this check, but
// it is a natural extension of cross-schema validation.
func detectRecordFieldTargets(tables []TableDefinition, edges []EdgeDefinition) []ValidationResult {
	var results []ValidationResult
	known := make(map[string]struct{}, len(tables)+len(edges))
	for _, t := range tables {
		known[t.Name] = struct{}{}
	}
	for _, e := range edges {
		known[e.Name] = struct{}{}
	}

	walk := func(container string, fields []FieldDefinition) {
		for _, f := range fields {
			if f.Type != FieldTypeRecord {
				continue
			}
			target := extractRecordTarget(f.Assertion)
			if target == "" {
				continue
			}
			if _, ok := known[target]; !ok {
				results = append(results, ValidationResult{
					Severity:  SeverityWarning,
					Table:     container,
					Field:     f.Name,
					Message:   "record field references unknown table",
					CodeValue: target,
				})
			}
		}
	}

	for _, t := range tables {
		walk(t.Name, t.Fields)
	}
	for _, e := range edges {
		walk(e.Name, e.Fields)
	}
	return results
}

// detectEdgeTableReferences warns when a RELATION edge references a table
// that is not registered with the schema.
func detectEdgeTableReferences(tables []TableDefinition, edges []EdgeDefinition) []ValidationResult {
	var results []ValidationResult
	known := make(map[string]struct{}, len(tables))
	for _, t := range tables {
		known[t.Name] = struct{}{}
	}
	for _, e := range edges {
		if e.Mode != EdgeModeRelation {
			continue
		}
		if e.FromTable != "" {
			if _, ok := known[e.FromTable]; !ok {
				results = append(results, ValidationResult{
					Severity:  SeverityWarning,
					Table:     e.Name,
					Message:   "edge from_table is not a registered table",
					CodeValue: e.FromTable,
				})
			}
		}
		if e.ToTable != "" {
			if _, ok := known[e.ToTable]; !ok {
				results = append(results, ValidationResult{
					Severity:  SeverityWarning,
					Table:     e.Name,
					Message:   "edge to_table is not a registered table",
					CodeValue: e.ToTable,
				})
			}
		}
	}
	return results
}

// detectDuplicateAccessNames reports access definitions that share a name.
func detectDuplicateAccessNames(accesses []AccessDefinition) []ValidationResult {
	var results []ValidationResult
	seen := make(map[string]struct{}, len(accesses))
	for _, a := range accesses {
		if a.Name == "" {
			continue
		}
		if _, dup := seen[a.Name]; dup {
			results = append(results, ValidationResult{
				Severity: SeverityError,
				Table:    a.Name,
				Message:  "duplicate access definition",
			})
			continue
		}
		seen[a.Name] = struct{}{}
	}
	return results
}

// Lookup helpers.

func tablesByName(tables []TableDefinition) map[string]TableDefinition {
	out := make(map[string]TableDefinition, len(tables))
	for _, t := range tables {
		out[t.Name] = t
	}
	return out
}

func edgesByName(edges []EdgeDefinition) map[string]EdgeDefinition {
	out := make(map[string]EdgeDefinition, len(edges))
	for _, e := range edges {
		out[e.Name] = e
	}
	return out
}

func fieldsByName(fields []FieldDefinition) map[string]FieldDefinition {
	out := make(map[string]FieldDefinition, len(fields))
	for _, f := range fields {
		out[f.Name] = f
	}
	return out
}

func indexesByName(indexes []IndexDefinition) map[string]IndexDefinition {
	out := make(map[string]IndexDefinition, len(indexes))
	for _, i := range indexes {
		out[i.Name] = i
	}
	return out
}

func eventsByName(events []EventDefinition) map[string]EventDefinition {
	out := make(map[string]EventDefinition, len(events))
	for _, e := range events {
		out[e.Name] = e
	}
	return out
}

// sortedKeys is generic over maps keyed by string.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedKeysString(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
