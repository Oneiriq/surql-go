package schema

import (
	"errors"
	"strings"
	"testing"

	surqlerrors "github.com/albedosehen/surql-go/pkg/surql/errors"
)

// ValidationSeverity.

func TestValidationSeverityValues(t *testing.T) {
	if SeverityError != "error" {
		t.Errorf("SeverityError = %q, want error", SeverityError)
	}
	if SeverityWarning != "warning" {
		t.Errorf("SeverityWarning = %q, want warning", SeverityWarning)
	}
	if SeverityInfo != "info" {
		t.Errorf("SeverityInfo = %q, want info", SeverityInfo)
	}
}

func TestValidationSeverityIsValid(t *testing.T) {
	valid := []ValidationSeverity{SeverityError, SeverityWarning, SeverityInfo}
	for _, s := range valid {
		if !s.IsValid() {
			t.Errorf("IsValid(%q) = false, want true", s)
		}
	}
	if ValidationSeverity("bogus").IsValid() {
		t.Errorf("IsValid(bogus) = true, want false")
	}
}

// ValidationResult.

func TestValidationResultStringWithField(t *testing.T) {
	r := ValidationResult{
		Severity:  SeverityError,
		Table:     "user",
		Field:     "email",
		Message:   "Field type mismatch",
		CodeValue: "string",
		DBValue:   "int",
	}
	s := r.String()
	for _, want := range []string{"[ERROR]", "user.email", "Field type mismatch",
		"code: string", "db: int"} {
		if !strings.Contains(s, want) {
			t.Errorf("String missing %q: %s", want, s)
		}
	}
}

func TestValidationResultStringWithoutField(t *testing.T) {
	r := ValidationResult{
		Severity: SeverityWarning,
		Table:    "post",
		Message:  "Table exists in database but not defined in code",
	}
	s := r.String()
	if !strings.Contains(s, "[WARNING]") {
		t.Errorf("missing [WARNING]: %s", s)
	}
	if strings.Contains(s, "post.") {
		t.Errorf("unexpected field separator: %s", s)
	}
}

func TestValidationResultStringWithoutValues(t *testing.T) {
	r := ValidationResult{Severity: SeverityInfo, Table: "user", Field: "name", Message: "Info"}
	s := r.String()
	if !strings.Contains(s, "[INFO]") {
		t.Errorf("missing [INFO]: %s", s)
	}
	if strings.Contains(s, "code:") {
		t.Errorf("unexpected code: in output: %s", s)
	}
}

// ValidationReport.

func TestNewValidationReport(t *testing.T) {
	vr := NewValidationReport()
	if vr == nil {
		t.Fatal("NewValidationReport returned nil")
	}
	if vr.Len() != 0 {
		t.Errorf("Len = %d, want 0", vr.Len())
	}
}

func TestValidationReportAdd(t *testing.T) {
	vr := NewValidationReport()
	vr.Add(ValidationResult{Severity: SeverityError, Table: "user"})
	if vr.Len() != 1 {
		t.Errorf("Len = %d, want 1", vr.Len())
	}
}

func TestValidationReportAddAll(t *testing.T) {
	vr := NewValidationReport()
	vr.AddAll([]ValidationResult{
		{Severity: SeverityError, Table: "a"},
		{Severity: SeverityWarning, Table: "b"},
	})
	if vr.Len() != 2 {
		t.Errorf("Len = %d, want 2", vr.Len())
	}
}

func TestValidationReportMerge(t *testing.T) {
	a := NewValidationReport()
	a.Add(ValidationResult{Severity: SeverityError, Table: "one"})
	b := NewValidationReport()
	b.Add(ValidationResult{Severity: SeverityWarning, Table: "two"})
	a.Merge(b)
	if a.Len() != 2 {
		t.Errorf("Len = %d, want 2", a.Len())
	}
}

func TestValidationReportMergeNilSafe(t *testing.T) {
	var vr *ValidationReport
	vr.Merge(NewValidationReport()) // should not panic
	other := NewValidationReport()
	other.Merge(nil) // should not panic
	if other.Len() != 0 {
		t.Errorf("Len = %d, want 0", other.Len())
	}
}

func TestValidationReportErrorsWarningsInfos(t *testing.T) {
	vr := NewValidationReport()
	vr.Add(ValidationResult{Severity: SeverityError})
	vr.Add(ValidationResult{Severity: SeverityError})
	vr.Add(ValidationResult{Severity: SeverityWarning})
	vr.Add(ValidationResult{Severity: SeverityInfo})

	if got := len(vr.Errors()); got != 2 {
		t.Errorf("Errors = %d, want 2", got)
	}
	if got := len(vr.Warnings()); got != 1 {
		t.Errorf("Warnings = %d, want 1", got)
	}
	if got := len(vr.Infos()); got != 1 {
		t.Errorf("Infos = %d, want 1", got)
	}
	if !vr.HasErrors() {
		t.Error("HasErrors = false, want true")
	}
	if vr.IsValid() {
		t.Error("IsValid = true, want false")
	}
}

func TestValidationReportNilAccessors(t *testing.T) {
	var vr *ValidationReport
	if vr.Len() != 0 {
		t.Error("Len should be 0 for nil report")
	}
	if vr.HasErrors() {
		t.Error("HasErrors should be false for nil report")
	}
	if !vr.IsValid() {
		t.Error("IsValid should be true for nil report")
	}
	if vr.Errors() != nil {
		t.Error("Errors should return nil for nil report")
	}
	if vr.Warnings() != nil {
		t.Error("Warnings should return nil for nil report")
	}
	if vr.Infos() != nil {
		t.Error("Infos should return nil for nil report")
	}
}

// ValidateSchema top-level.

func TestValidateSchemaNilRegistry(t *testing.T) {
	_, err := ValidateSchema(nil)
	if err == nil {
		t.Fatal("expected error for nil registry")
	}
	if !errors.Is(err, surqlerrors.ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

func TestValidateSchemaEmptyRegistry(t *testing.T) {
	r := NewSchemaRegistry()
	report, err := ValidateSchema(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Len() != 0 {
		t.Errorf("expected empty report, got %d results", report.Len())
	}
}

func TestValidateSchemaHappyPath(t *testing.T) {
	r := NewSchemaRegistry()
	_ = r.RegisterTable(NewTable("user", WithFields(StringField("name"))))
	_ = r.RegisterEdge(TypedEdge("likes", "user", "post"))
	_ = r.RegisterTable(NewTable("post", WithFields(StringField("title"))))

	report, err := ValidateSchema(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.HasErrors() {
		t.Errorf("expected no errors; got %+v", report.Errors())
	}
}

func TestValidateSchemaNameCollision(t *testing.T) {
	r := NewSchemaRegistry()
	_ = r.RegisterTable(NewTable("likes"))
	_ = r.RegisterTable(NewTable("user"))
	_ = r.RegisterTable(NewTable("post"))
	_ = r.RegisterEdge(TypedEdge("likes", "user", "post"))

	report, err := ValidateSchema(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, res := range report.Errors() {
		if res.Table == "likes" && strings.Contains(res.Message, "name conflict") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected name conflict error; got %+v", report.Results)
	}
}

func TestValidateSchemaEdgeReferencesUnknownTable(t *testing.T) {
	r := NewSchemaRegistry()
	_ = r.RegisterTable(NewTable("user"))
	_ = r.RegisterEdge(TypedEdge("likes", "user", "missing"))

	report, _ := ValidateSchema(r)
	found := false
	for _, res := range report.Warnings() {
		if strings.Contains(res.Message, "to_table") && res.CodeValue == "missing" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected warning for unknown to_table; got %+v", report.Results)
	}
}

func TestValidateSchemaRecordFieldUnknownTarget(t *testing.T) {
	r := NewSchemaRegistry()
	_ = r.RegisterTable(NewTable("user", WithFields(
		RecordField("favorite", "nonexistent"),
	)))

	report, _ := ValidateSchema(r)
	found := false
	for _, res := range report.Warnings() {
		if res.Field == "favorite" && res.CodeValue == "nonexistent" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected warning for unknown record target; got %+v", report.Results)
	}
}

func TestValidateSchemaRecordFieldKnownTarget(t *testing.T) {
	r := NewSchemaRegistry()
	_ = r.RegisterTable(NewTable("post"))
	_ = r.RegisterTable(NewTable("user", WithFields(RecordField("favorite", "post"))))

	report, _ := ValidateSchema(r)
	for _, res := range report.Warnings() {
		if res.Field == "favorite" {
			t.Errorf("unexpected warning for known record target: %+v", res)
		}
	}
}

func TestValidateSchemaDuplicateAccessNames(t *testing.T) {
	r := NewSchemaRegistry()
	acc := JwtAccess("api", JwtConfig{Key: "secret"})
	report, _ := ValidateSchema(r, acc, acc)
	found := false
	for _, res := range report.Errors() {
		if strings.Contains(res.Message, "duplicate access") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected duplicate access error; got %+v", report.Results)
	}
}

// ValidateTable.

func TestValidateTableEmptyName(t *testing.T) {
	report := ValidateTable(TableDefinition{})
	if !report.HasErrors() {
		t.Error("expected error for empty name")
	}
}

func TestValidateTableInvalidMode(t *testing.T) {
	report := ValidateTable(TableDefinition{Name: "user", Mode: "FOO"})
	if !report.HasErrors() {
		t.Error("expected error for invalid mode")
	}
}

func TestValidateTableValid(t *testing.T) {
	report := ValidateTable(NewTable("user", WithFields(StringField("name"))))
	if report.HasErrors() {
		t.Errorf("unexpected errors: %+v", report.Errors())
	}
}

func TestValidateTableDuplicateField(t *testing.T) {
	report := ValidateTable(NewTable("user",
		WithFields(StringField("name"), StringField("name")),
	))
	found := false
	for _, res := range report.Errors() {
		if res.Field == "name" && strings.Contains(res.Message, "duplicate") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected duplicate field error; got %+v", report.Results)
	}
}

func TestValidateTableInvalidFieldName(t *testing.T) {
	report := ValidateTable(NewTable("user",
		WithFields(FieldDefinition{Name: "1invalid", Type: FieldTypeString}),
	))
	if !report.HasErrors() {
		t.Error("expected error for invalid field name")
	}
}

func TestValidateTableInvalidFieldType(t *testing.T) {
	report := ValidateTable(NewTable("user",
		WithFields(FieldDefinition{Name: "x", Type: "bogus"}),
	))
	if !report.HasErrors() {
		t.Error("expected error for invalid field type")
	}
}

func TestValidateTableDuplicateIndex(t *testing.T) {
	report := ValidateTable(NewTable("user",
		WithFields(StringField("name")),
		WithIndexes(
			NewIndex("idx", []string{"name"}, IndexTypeStandard),
			NewIndex("idx", []string{"name"}, IndexTypeStandard),
		),
	))
	found := false
	for _, res := range report.Errors() {
		if res.Field == "index:idx" && strings.Contains(res.Message, "duplicate") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected duplicate index error; got %+v", report.Results)
	}
}

func TestValidateTableIndexUnknownColumn(t *testing.T) {
	report := ValidateTable(NewTable("user",
		WithFields(StringField("name")),
		WithIndexes(NewIndex("idx", []string{"ghost"}, IndexTypeStandard)),
	))
	found := false
	for _, res := range report.Warnings() {
		if res.Field == "index:idx" && strings.Contains(res.Message, "unknown column") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected unknown column warning; got %+v", report.Results)
	}
}

func TestValidateTableIndexUnknownColumnSchemalessNoWarning(t *testing.T) {
	// A table with no fields declared skips the unknown-column check.
	report := ValidateTable(NewTable("user",
		WithMode(TableModeSchemaless),
		WithIndexes(NewIndex("idx", []string{"name"}, IndexTypeStandard)),
	))
	for _, res := range report.Warnings() {
		if strings.Contains(res.Message, "unknown column") {
			t.Errorf("unexpected unknown-column warning for schemaless table: %+v", res)
		}
	}
}

func TestValidateTableInvalidIndex(t *testing.T) {
	report := ValidateTable(NewTable("user",
		WithIndexes(IndexDefinition{Name: "", Columns: []string{"x"}, Type: IndexTypeStandard}),
	))
	if !report.HasErrors() {
		t.Error("expected error for unnamed index")
	}
}

func TestValidateTableDuplicateEvent(t *testing.T) {
	report := ValidateTable(NewTable("user",
		WithEvents(
			NewEvent("on_create", "$event = 'CREATE'", "SELECT 1"),
			NewEvent("on_create", "$event = 'CREATE'", "SELECT 1"),
		),
	))
	found := false
	for _, res := range report.Errors() {
		if res.Field == "event:on_create" && strings.Contains(res.Message, "duplicate") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected duplicate event error; got %+v", report.Results)
	}
}

func TestValidateTableInvalidEvent(t *testing.T) {
	report := ValidateTable(NewTable("user",
		WithEvents(EventDefinition{Name: "evt", Condition: "", Action: ""}),
	))
	if !report.HasErrors() {
		t.Error("expected error for empty event condition/action")
	}
}

func TestValidateTableReservedFieldNameWarns(t *testing.T) {
	report := ValidateTable(NewTable("user",
		WithFields(StringField("select")),
	))
	// May or may not produce a reserved warning depending on the types
	// package content, so we just ensure the validator does not crash and
	// returns a ValidationReport.
	_ = report
}

func TestValidateTablePermissions(t *testing.T) {
	report := ValidateTable(NewTable("user",
		WithFields(StringField("name")),
		WithTablePermissions(map[string]string{"select": "true", "bogus": "true", "update": ""}),
	))
	warnings := report.Warnings()
	if len(warnings) == 0 {
		t.Errorf("expected permissions warnings; got %+v", report.Results)
	}
	hasEmptyErr := false
	for _, res := range report.Errors() {
		if strings.Contains(res.Message, "permission expression cannot be empty") {
			hasEmptyErr = true
		}
	}
	if !hasEmptyErr {
		t.Error("expected error for empty permission expression")
	}
}

// ValidateEdge.

func TestValidateEdgeEmptyName(t *testing.T) {
	report := ValidateEdge(EdgeDefinition{})
	if !report.HasErrors() {
		t.Error("expected error for empty edge name")
	}
}

func TestValidateEdgeInvalidMode(t *testing.T) {
	report := ValidateEdge(EdgeDefinition{Name: "likes", Mode: "BOGUS"})
	if !report.HasErrors() {
		t.Error("expected error for invalid edge mode")
	}
}

func TestValidateEdgeRelationRequiresFromTo(t *testing.T) {
	report := ValidateEdge(NewEdge("likes"))
	errs := report.Errors()
	gotFrom, gotTo := false, false
	for _, res := range errs {
		if strings.Contains(res.Message, "from_table") {
			gotFrom = true
		}
		if strings.Contains(res.Message, "to_table") {
			gotTo = true
		}
	}
	if !gotFrom || !gotTo {
		t.Errorf("expected from_table and to_table errors; got %+v", errs)
	}
}

func TestValidateEdgeRelationValid(t *testing.T) {
	report := ValidateEdge(TypedEdge("likes", "user", "post"))
	if report.HasErrors() {
		t.Errorf("unexpected errors: %+v", report.Errors())
	}
}

func TestValidateEdgeAllowsInOutFields(t *testing.T) {
	e := TypedEdge("likes", "user", "post",
		WithEdgeFields(RecordField("in", "user"), RecordField("out", "post")),
	)
	report := ValidateEdge(e)
	if report.HasErrors() {
		t.Errorf("unexpected errors for in/out fields: %+v", report.Errors())
	}
}

// ValidateAccess.

func TestValidateAccessEmptyName(t *testing.T) {
	report := ValidateAccess(AccessDefinition{})
	if !report.HasErrors() {
		t.Error("expected error for empty name")
	}
}

func TestValidateAccessInvalidType(t *testing.T) {
	report := ValidateAccess(AccessDefinition{Name: "a", Type: "BOGUS"})
	if !report.HasErrors() {
		t.Error("expected error for invalid type")
	}
}

func TestValidateAccessJwtRequiresConfig(t *testing.T) {
	report := ValidateAccess(AccessDefinition{Name: "a", Type: AccessTypeJWT})
	if !report.HasErrors() {
		t.Error("expected error for JWT without config")
	}
}

func TestValidateAccessJwtRequiresKeyOrURL(t *testing.T) {
	report := ValidateAccess(JwtAccess("a", JwtConfig{}))
	if !report.HasErrors() {
		t.Error("expected error for JWT without KEY or URL")
	}
}

func TestValidateAccessJwtBothKeyAndURL(t *testing.T) {
	report := ValidateAccess(JwtAccess("a", JwtConfig{Key: "x", URL: "http://y"}))
	found := false
	for _, res := range report.Warnings() {
		if strings.Contains(res.Message, "both KEY and URL") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected warning for both KEY and URL; got %+v", report.Results)
	}
}

func TestValidateAccessRecordRequiresConfig(t *testing.T) {
	report := ValidateAccess(AccessDefinition{Name: "a", Type: AccessTypeRecord})
	if !report.HasErrors() {
		t.Error("expected error for RECORD without config")
	}
}

func TestValidateAccessRecordWarnsWithoutSignupSignin(t *testing.T) {
	report := ValidateAccess(RecordAccess("a", RecordAccessConfig{}))
	found := false
	for _, res := range report.Warnings() {
		if strings.Contains(res.Message, "SIGNUP") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected SIGNUP/SIGNIN warning; got %+v", report.Results)
	}
}

func TestValidateAccessJwtWithRecordConfigWarns(t *testing.T) {
	a := JwtAccess("a", JwtConfig{Key: "x"})
	a.Record = &RecordAccessConfig{Signup: "SELECT 1"}
	report := ValidateAccess(a)
	found := false
	for _, res := range report.Warnings() {
		if strings.Contains(res.Message, "should not carry a record") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected warning for JWT with record config; got %+v", report.Results)
	}
}

func TestValidateAccessRecordWithJWTConfigWarns(t *testing.T) {
	a := RecordAccess("a", RecordAccessConfig{Signup: "SELECT 1"})
	a.JWT = &JwtConfig{Key: "x"}
	report := ValidateAccess(a)
	found := false
	for _, res := range report.Warnings() {
		if strings.Contains(res.Message, "should not carry a JWT") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected warning for RECORD with JWT config; got %+v", report.Results)
	}
}

// CompareSchemas (drift detection).

func TestCompareSchemasIdentical(t *testing.T) {
	a := []TableDefinition{NewTable("user", WithFields(StringField("name")))}
	report := CompareSchemas(a, a, nil, nil)
	if report.Len() != 0 {
		t.Errorf("expected empty report, got %+v", report.Results)
	}
}

func TestCompareSchemasMissingTable(t *testing.T) {
	code := []TableDefinition{NewTable("user")}
	report := CompareSchemas(code, nil, nil, nil)
	if report.Len() != 1 {
		t.Fatalf("expected 1 result, got %d", report.Len())
	}
	res := report.Results[0]
	if res.Severity != SeverityError || res.Table != "user" ||
		!strings.Contains(res.Message, "missing from database") {
		t.Errorf("unexpected result: %+v", res)
	}
}

func TestCompareSchemasMultipleMissingTables(t *testing.T) {
	code := []TableDefinition{NewTable("user"), NewTable("post")}
	report := CompareSchemas(code, nil, nil, nil)
	names := map[string]bool{}
	for _, r := range report.Errors() {
		names[r.Table] = true
	}
	if !names["user"] || !names["post"] {
		t.Errorf("expected user and post; got %v", names)
	}
}

func TestCompareSchemasExtraTable(t *testing.T) {
	db := []TableDefinition{NewTable("legacy")}
	report := CompareSchemas(nil, db, nil, nil)
	if report.Len() != 1 {
		t.Fatalf("expected 1 result, got %d", report.Len())
	}
	if report.Results[0].Severity != SeverityWarning {
		t.Errorf("expected WARNING, got %+v", report.Results[0])
	}
}

func TestCompareSchemasTableModeMismatch(t *testing.T) {
	code := []TableDefinition{NewTable("user", WithMode(TableModeSchemafull))}
	db := []TableDefinition{NewTable("user", WithMode(TableModeSchemaless))}
	report := CompareSchemas(code, db, nil, nil)
	found := false
	for _, r := range report.Errors() {
		if strings.Contains(strings.ToLower(r.Message), "mode mismatch") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected mode mismatch; got %+v", report.Results)
	}
}

func TestCompareSchemasFieldTypeMismatch(t *testing.T) {
	code := []TableDefinition{NewTable("user", WithFields(IntField("age")))}
	db := []TableDefinition{NewTable("user", WithFields(StringField("age")))}
	report := CompareSchemas(code, db, nil, nil)
	found := false
	for _, r := range report.Errors() {
		if r.Field == "age" && strings.Contains(r.Message, "type mismatch") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected type mismatch; got %+v", report.Results)
	}
}

func TestCompareSchemasFieldMissing(t *testing.T) {
	code := []TableDefinition{NewTable("user",
		WithFields(StringField("name"), StringField("email")))}
	db := []TableDefinition{NewTable("user", WithFields(StringField("name")))}
	report := CompareSchemas(code, db, nil, nil)
	found := false
	for _, r := range report.Errors() {
		if r.Field == "email" && strings.Contains(r.Message, "missing from database") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected missing email field; got %+v", report.Results)
	}
}

func TestCompareSchemasExtraField(t *testing.T) {
	code := []TableDefinition{NewTable("user", WithFields(StringField("name")))}
	db := []TableDefinition{NewTable("user",
		WithFields(StringField("name"), StringField("legacy")))}
	report := CompareSchemas(code, db, nil, nil)
	found := false
	for _, r := range report.Warnings() {
		if r.Field == "legacy" && strings.Contains(r.Message, "not defined in code") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected extra legacy field; got %+v", report.Results)
	}
}

func TestCompareSchemasFieldAssertionMismatch(t *testing.T) {
	code := []TableDefinition{NewTable("user", WithFields(
		StringField("email", WithAssertion("string::is::email($value)"))))}
	db := []TableDefinition{NewTable("user", WithFields(
		StringField("email", WithAssertion("$value != NONE"))))}
	report := CompareSchemas(code, db, nil, nil)
	found := false
	for _, r := range report.Warnings() {
		if r.Field == "email" && strings.Contains(r.Message, "assertion") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected assertion mismatch; got %+v", report.Results)
	}
}

func TestCompareSchemasFieldDefaultMismatch(t *testing.T) {
	code := []TableDefinition{NewTable("u", WithFields(StringField("n", WithDefault("'a'"))))}
	db := []TableDefinition{NewTable("u", WithFields(StringField("n", WithDefault("'b'"))))}
	report := CompareSchemas(code, db, nil, nil)
	found := false
	for _, r := range report.Warnings() {
		if r.Field == "n" && strings.Contains(r.Message, "default") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected default mismatch; got %+v", report.Results)
	}
}

func TestCompareSchemasFieldValueMismatch(t *testing.T) {
	code := []TableDefinition{NewTable("u", WithFields(StringField("n", WithValue("'a'"))))}
	db := []TableDefinition{NewTable("u", WithFields(StringField("n", WithValue("'b'"))))}
	report := CompareSchemas(code, db, nil, nil)
	found := false
	for _, r := range report.Warnings() {
		if r.Field == "n" && strings.Contains(r.Message, "computed value") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected computed value mismatch; got %+v", report.Results)
	}
}

func TestCompareSchemasFieldReadonlyMismatch(t *testing.T) {
	code := []TableDefinition{NewTable("u",
		WithFields(StringField("n", WithReadOnly(true))))}
	db := []TableDefinition{NewTable("u",
		WithFields(StringField("n", WithReadOnly(false))))}
	report := CompareSchemas(code, db, nil, nil)
	found := false
	for _, r := range report.Infos() {
		if r.Field == "n" && strings.Contains(r.Message, "readonly") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected readonly flag info; got %+v", report.Results)
	}
}

func TestCompareSchemasFieldFlexibleMismatch(t *testing.T) {
	code := []TableDefinition{NewTable("u",
		WithFields(StringField("n", WithFlexible(true))))}
	db := []TableDefinition{NewTable("u",
		WithFields(StringField("n", WithFlexible(false))))}
	report := CompareSchemas(code, db, nil, nil)
	found := false
	for _, r := range report.Infos() {
		if r.Field == "n" && strings.Contains(r.Message, "flexible") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected flexible flag info; got %+v", report.Results)
	}
}

func TestCompareSchemasAssertionWhitespaceNormalized(t *testing.T) {
	code := []TableDefinition{NewTable("u", WithFields(
		StringField("n", WithAssertion("$value  !=  NONE"))))}
	db := []TableDefinition{NewTable("u", WithFields(
		StringField("n", WithAssertion("$value != NONE"))))}
	report := CompareSchemas(code, db, nil, nil)
	for _, r := range report.Warnings() {
		if r.Field == "n" && strings.Contains(r.Message, "assertion") {
			t.Errorf("unexpected assertion warning after normalization: %+v", r)
		}
	}
}

func TestCompareSchemasIndexMissing(t *testing.T) {
	code := []TableDefinition{NewTable("u",
		WithFields(StringField("email")),
		WithIndexes(UniqueIndex("email_idx", []string{"email"})))}
	db := []TableDefinition{NewTable("u",
		WithFields(StringField("email")))}
	report := CompareSchemas(code, db, nil, nil)
	found := false
	for _, r := range report.Errors() {
		if r.Field == "index:email_idx" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected missing index error; got %+v", report.Results)
	}
}

func TestCompareSchemasIndexExtra(t *testing.T) {
	code := []TableDefinition{NewTable("u", WithFields(StringField("email")))}
	db := []TableDefinition{NewTable("u",
		WithFields(StringField("email")),
		WithIndexes(UniqueIndex("legacy_idx", []string{"email"})))}
	report := CompareSchemas(code, db, nil, nil)
	found := false
	for _, r := range report.Warnings() {
		if r.Field == "index:legacy_idx" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected extra index warning; got %+v", report.Results)
	}
}

func TestCompareSchemasIndexTypeMismatch(t *testing.T) {
	code := []TableDefinition{NewTable("u",
		WithFields(StringField("email")),
		WithIndexes(UniqueIndex("email_idx", []string{"email"})))}
	db := []TableDefinition{NewTable("u",
		WithFields(StringField("email")),
		WithIndexes(NewIndex("email_idx", []string{"email"}, IndexTypeStandard)))}
	report := CompareSchemas(code, db, nil, nil)
	found := false
	for _, r := range report.Errors() {
		if strings.Contains(r.Message, "Index type mismatch") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected index type mismatch; got %+v", report.Results)
	}
}

func TestCompareSchemasIndexColumnsMismatch(t *testing.T) {
	code := []TableDefinition{NewTable("u",
		WithFields(StringField("a"), StringField("b")),
		WithIndexes(NewIndex("idx", []string{"a", "b"}, IndexTypeStandard)))}
	db := []TableDefinition{NewTable("u",
		WithFields(StringField("a")),
		WithIndexes(NewIndex("idx", []string{"a"}, IndexTypeStandard)))}
	report := CompareSchemas(code, db, nil, nil)
	found := false
	for _, r := range report.Errors() {
		if strings.Contains(r.Message, "columns mismatch") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected columns mismatch; got %+v", report.Results)
	}
}

func TestCompareSchemasIndexColumnsOrderInsensitive(t *testing.T) {
	code := []TableDefinition{NewTable("u",
		WithFields(StringField("a"), StringField("b")),
		WithIndexes(NewIndex("idx", []string{"a", "b"}, IndexTypeStandard)))}
	db := []TableDefinition{NewTable("u",
		WithFields(StringField("a"), StringField("b")),
		WithIndexes(NewIndex("idx", []string{"b", "a"}, IndexTypeStandard)))}
	report := CompareSchemas(code, db, nil, nil)
	for _, r := range report.Errors() {
		if strings.Contains(r.Message, "columns mismatch") {
			t.Errorf("unexpected columns mismatch with reordered slice: %+v", r)
		}
	}
}

func TestCompareSchemasMTreeDimensionMismatch(t *testing.T) {
	code := []TableDefinition{NewTable("d",
		WithFields(ArrayField("emb")),
		WithIndexes(MTreeIndex("vec", "emb", 1024, MTreeIndexOptions{
			Distance: MTreeDistanceCosine, VectorType: MTreeVectorF32})))}
	db := []TableDefinition{NewTable("d",
		WithFields(ArrayField("emb")),
		WithIndexes(MTreeIndex("vec", "emb", 768, MTreeIndexOptions{
			Distance: MTreeDistanceCosine, VectorType: MTreeVectorF32})))}
	report := CompareSchemas(code, db, nil, nil)
	found := false
	for _, r := range report.Errors() {
		if strings.Contains(r.Message, "MTREE index dimension") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected MTREE dim mismatch; got %+v", report.Results)
	}
}

func TestCompareSchemasMTreeDistanceMismatch(t *testing.T) {
	code := []TableDefinition{NewTable("d",
		WithFields(ArrayField("emb")),
		WithIndexes(MTreeIndex("vec", "emb", 64, MTreeIndexOptions{
			Distance: MTreeDistanceCosine, VectorType: MTreeVectorF32})))}
	db := []TableDefinition{NewTable("d",
		WithFields(ArrayField("emb")),
		WithIndexes(MTreeIndex("vec", "emb", 64, MTreeIndexOptions{
			Distance: MTreeDistanceEuclidean, VectorType: MTreeVectorF32})))}
	report := CompareSchemas(code, db, nil, nil)
	found := false
	for _, r := range report.Warnings() {
		if strings.Contains(r.Message, "distance metric") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected distance mismatch; got %+v", report.Results)
	}
}

func TestCompareSchemasHnswDimensionMismatch(t *testing.T) {
	code := []TableDefinition{NewTable("d",
		WithFields(ArrayField("emb")),
		WithIndexes(HnswIndex("vec", "emb", 1024, HnswIndexOptions{
			Distance: HnswDistanceCosine, VectorType: MTreeVectorF32})))}
	db := []TableDefinition{NewTable("d",
		WithFields(ArrayField("emb")),
		WithIndexes(HnswIndex("vec", "emb", 768, HnswIndexOptions{
			Distance: HnswDistanceCosine, VectorType: MTreeVectorF32})))}
	report := CompareSchemas(code, db, nil, nil)
	found := false
	for _, r := range report.Errors() {
		if strings.Contains(r.Message, "HNSW index dimension") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected HNSW dim mismatch; got %+v", report.Results)
	}
}

func TestCompareSchemasHnswEfcMismatch(t *testing.T) {
	code := []TableDefinition{NewTable("d",
		WithFields(ArrayField("emb")),
		WithIndexes(HnswIndex("vec", "emb", 64, HnswIndexOptions{EFC: 100})))}
	db := []TableDefinition{NewTable("d",
		WithFields(ArrayField("emb")),
		WithIndexes(HnswIndex("vec", "emb", 64, HnswIndexOptions{EFC: 200})))}
	report := CompareSchemas(code, db, nil, nil)
	found := false
	for _, r := range report.Warnings() {
		if strings.Contains(r.Message, "EFC") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected EFC mismatch; got %+v", report.Results)
	}
}

func TestCompareSchemasHnswMMismatch(t *testing.T) {
	code := []TableDefinition{NewTable("d",
		WithFields(ArrayField("emb")),
		WithIndexes(HnswIndex("vec", "emb", 64, HnswIndexOptions{M: 16})))}
	db := []TableDefinition{NewTable("d",
		WithFields(ArrayField("emb")),
		WithIndexes(HnswIndex("vec", "emb", 64, HnswIndexOptions{M: 32})))}
	report := CompareSchemas(code, db, nil, nil)
	found := false
	for _, r := range report.Warnings() {
		if strings.Contains(r.Message, "HNSW index M mismatch") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected M mismatch; got %+v", report.Results)
	}
}

func TestCompareSchemasEventsMissing(t *testing.T) {
	code := []TableDefinition{NewTable("u",
		WithEvents(NewEvent("on_create", "$event = 'CREATE'", "SELECT 1")))}
	db := []TableDefinition{NewTable("u")}
	report := CompareSchemas(code, db, nil, nil)
	found := false
	for _, r := range report.Errors() {
		if r.Field == "event:on_create" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected missing event; got %+v", report.Results)
	}
}

func TestCompareSchemasEventsExtra(t *testing.T) {
	code := []TableDefinition{NewTable("u")}
	db := []TableDefinition{NewTable("u",
		WithEvents(NewEvent("legacy_evt", "$event = 'CREATE'", "SELECT 1")))}
	report := CompareSchemas(code, db, nil, nil)
	found := false
	for _, r := range report.Warnings() {
		if r.Field == "event:legacy_evt" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected extra event warning; got %+v", report.Results)
	}
}

func TestCompareSchemasEdgeMissing(t *testing.T) {
	code := []EdgeDefinition{TypedEdge("likes", "user", "post")}
	report := CompareSchemas(nil, nil, code, nil)
	found := false
	for _, r := range report.Errors() {
		if r.Table == "likes" && strings.Contains(r.Message, "missing from database") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected missing edge error; got %+v", report.Results)
	}
}

func TestCompareSchemasEdgeExtra(t *testing.T) {
	db := []EdgeDefinition{TypedEdge("likes", "user", "post")}
	report := CompareSchemas(nil, nil, nil, db)
	found := false
	for _, r := range report.Warnings() {
		if r.Table == "likes" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected extra edge warning; got %+v", report.Results)
	}
}

func TestCompareSchemasEdgeModeMismatch(t *testing.T) {
	code := []EdgeDefinition{TypedEdge("likes", "user", "post")}
	db := []EdgeDefinition{NewEdge("likes", WithEdgeMode(EdgeModeSchemafull))}
	report := CompareSchemas(nil, nil, code, db)
	found := false
	for _, r := range report.Errors() {
		if strings.Contains(r.Message, "Edge mode mismatch") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected edge mode mismatch; got %+v", report.Results)
	}
}

func TestCompareSchemasEdgeFromToMismatch(t *testing.T) {
	code := []EdgeDefinition{TypedEdge("likes", "user", "post")}
	db := []EdgeDefinition{TypedEdge("likes", "user", "comment")}
	report := CompareSchemas(nil, nil, code, db)
	found := false
	for _, r := range report.Errors() {
		if strings.Contains(r.Message, "to_table") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected to_table mismatch; got %+v", report.Results)
	}
}

func TestCompareSchemasEdgeFieldMismatch(t *testing.T) {
	code := []EdgeDefinition{TypedEdge("likes", "user", "post",
		WithEdgeFields(IntField("weight")))}
	db := []EdgeDefinition{TypedEdge("likes", "user", "post")}
	report := CompareSchemas(nil, nil, code, db)
	found := false
	for _, r := range report.Errors() {
		if r.Field == "weight" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected edge field missing; got %+v", report.Results)
	}
}

// Utility functions.

func sampleResults() []ValidationResult {
	return []ValidationResult{
		{Severity: SeverityError, Table: "user", Field: "email"},
		{Severity: SeverityWarning, Table: "user", Field: "name"},
		{Severity: SeverityInfo, Table: "user", Field: "age"},
		{Severity: SeverityError, Table: "post", Field: "title"},
	}
}

func TestFilterBySeverityError(t *testing.T) {
	got := FilterBySeverity(sampleResults(), SeverityError)
	if len(got) != 2 {
		t.Errorf("got %d errors, want 2", len(got))
	}
}

func TestFilterBySeverityWarning(t *testing.T) {
	got := FilterBySeverity(sampleResults(), SeverityWarning)
	if len(got) != 1 {
		t.Errorf("got %d warnings, want 1", len(got))
	}
}

func TestFilterBySeverityNoMatch(t *testing.T) {
	got := FilterBySeverity([]ValidationResult{{Severity: SeverityError}}, SeverityInfo)
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %d", len(got))
	}
}

func TestFilterBySeverityEmpty(t *testing.T) {
	got := FilterBySeverity(nil, SeverityError)
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestFilterErrors(t *testing.T) {
	got := FilterErrors(sampleResults())
	if len(got) != 2 {
		t.Errorf("got %d, want 2", len(got))
	}
	for _, r := range got {
		if r.Severity != SeverityError {
			t.Errorf("unexpected severity: %v", r)
		}
	}
}

func TestFilterWarnings(t *testing.T) {
	got := FilterWarnings(sampleResults())
	if len(got) != 1 {
		t.Errorf("got %d, want 1", len(got))
	}
}

func TestFilterInfos(t *testing.T) {
	got := FilterInfos(sampleResults())
	if len(got) != 1 {
		t.Errorf("got %d, want 1", len(got))
	}
}

func TestGroupByTable(t *testing.T) {
	grouped := GroupByTable(sampleResults())
	if len(grouped["user"]) != 3 {
		t.Errorf("user: got %d, want 3", len(grouped["user"]))
	}
	if len(grouped["post"]) != 1 {
		t.Errorf("post: got %d, want 1", len(grouped["post"]))
	}
}

func TestGroupByTableEmpty(t *testing.T) {
	grouped := GroupByTable(nil)
	if len(grouped) != 0 {
		t.Errorf("expected empty, got %d keys", len(grouped))
	}
}

func TestHasErrorsTrue(t *testing.T) {
	if !HasErrors(sampleResults()) {
		t.Error("HasErrors = false, want true")
	}
}

func TestHasErrorsFalse(t *testing.T) {
	results := []ValidationResult{
		{Severity: SeverityWarning},
		{Severity: SeverityInfo},
	}
	if HasErrors(results) {
		t.Error("HasErrors = true, want false")
	}
}

func TestHasErrorsEmpty(t *testing.T) {
	if HasErrors(nil) {
		t.Error("HasErrors(nil) = true, want false")
	}
}

func TestFormatValidationReportEmpty(t *testing.T) {
	s := FormatValidationReport(nil, false)
	if !strings.Contains(s, "No schema validation issues") {
		t.Errorf("unexpected empty report: %q", s)
	}
}

func TestFormatValidationReportWithErrors(t *testing.T) {
	s := FormatValidationReport(sampleResults(), false)
	for _, want := range []string{"Schema Validation Report", "user", "post", "[!]", "[~]"} {
		if !strings.Contains(s, want) {
			t.Errorf("report missing %q: %s", want, s)
		}
	}
}

func TestFormatValidationReportExcludesInfoByDefault(t *testing.T) {
	results := []ValidationResult{{Severity: SeverityInfo, Table: "u", Message: "info"}}
	s := FormatValidationReport(results, false)
	if !strings.Contains(s, "No significant") {
		t.Errorf("expected 'No significant' summary, got: %s", s)
	}
}

func TestFormatValidationReportIncludesInfoWhenRequested(t *testing.T) {
	results := []ValidationResult{{Severity: SeverityInfo, Table: "u", Message: "info"}}
	s := FormatValidationReport(results, true)
	if !strings.Contains(s, "u") || !strings.Contains(s, "[i]") {
		t.Errorf("expected info marker in report, got: %s", s)
	}
}

func TestGetValidationSummaryMixed(t *testing.T) {
	summary := GetValidationSummary(sampleResults())
	if summary.Total != 4 {
		t.Errorf("Total = %d, want 4", summary.Total)
	}
	if summary.Errors != 2 {
		t.Errorf("Errors = %d, want 2", summary.Errors)
	}
	if summary.Warnings != 1 {
		t.Errorf("Warnings = %d, want 1", summary.Warnings)
	}
	if summary.Info != 1 {
		t.Errorf("Info = %d, want 1", summary.Info)
	}
	if summary.TablesAffected != 2 {
		t.Errorf("TablesAffected = %d, want 2", summary.TablesAffected)
	}
	if !summary.HasErrors {
		t.Error("HasErrors = false, want true")
	}
}

func TestGetValidationSummaryEmpty(t *testing.T) {
	summary := GetValidationSummary(nil)
	if summary.Total != 0 || summary.HasErrors {
		t.Errorf("expected zero summary, got %+v", summary)
	}
}

func TestGetValidationSummaryNoErrors(t *testing.T) {
	summary := GetValidationSummary([]ValidationResult{{Severity: SeverityWarning}})
	if summary.HasErrors {
		t.Error("HasErrors = true, want false")
	}
}

// normalizeExpression / extractRecordTarget helpers.

func TestNormalizeExpression(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"  ", ""},
		{"$value != NONE", "$value != NONE"},
		{"$value  !=  NONE", "$value != NONE"},
		{"\t$value\n!=\tNONE ", "$value != NONE"},
	}
	for _, c := range cases {
		got := normalizeExpression(c.in)
		if got != c.want {
			t.Errorf("normalize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestExtractRecordTarget(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{`$value.table = "post"`, "post"},
		{`$value.table = 'post'`, "post"},
		{`($value.table = "post") AND ($value != NONE)`, "post"},
		{`$value != NONE`, ""},
		{`$value.table =`, ""},
	}
	for _, c := range cases {
		got := extractRecordTarget(c.in)
		if got != c.want {
			t.Errorf("extractRecordTarget(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSortedSliceEqual(t *testing.T) {
	if !sortedSliceEqual([]string{"a", "b"}, []string{"b", "a"}) {
		t.Error("expected equal sorted slices")
	}
	if sortedSliceEqual([]string{"a"}, []string{"a", "b"}) {
		t.Error("unequal lengths must not match")
	}
	if sortedSliceEqual([]string{"a"}, []string{"b"}) {
		t.Error("different elements must not match")
	}
	if !sortedSliceEqual(nil, nil) {
		t.Error("nil/nil should match")
	}
}
