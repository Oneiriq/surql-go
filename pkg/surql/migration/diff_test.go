package migration

import (
	"errors"
	"strings"
	"testing"

	surqlerrors "github.com/albedosehen/surql-go/pkg/surql/errors"
	"github.com/albedosehen/surql-go/pkg/surql/schema"
)

// --- helpers -----------------------------------------------------------------

func findByOp(diffs []SchemaDiff, op DiffOperation) []SchemaDiff {
	out := make([]SchemaDiff, 0)
	for _, d := range diffs {
		if d.Operation == op {
			out = append(out, d)
		}
	}
	return out
}

// --- DiffTables --------------------------------------------------------------

func TestDiffTablesAddOnly(t *testing.T) {
	newT := schema.NewTable("user",
		schema.WithMode(schema.TableModeSchemafull),
		schema.WithFields(schema.StringField("email")),
	)
	diffs, err := DiffTables(nil, &newT)

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}
	if len(diffs) != 2 {
		t.Fatalf("expected 2 diffs (table + field), got %d", len(diffs))
	}
	if diffs[0].Operation != DiffOperationAddTable {
		t.Errorf("expected add_table first, got %s", diffs[0].Operation)
	}
	if !strings.Contains(diffs[0].ForwardSQL, "DEFINE TABLE user SCHEMAFULL") {
		t.Errorf("unexpected forward sql: %s", diffs[0].ForwardSQL)
	}
	if diffs[0].BackwardSQL != "REMOVE TABLE user;" {
		t.Errorf("unexpected backward sql: %s", diffs[0].BackwardSQL)
	}
	if diffs[1].Operation != DiffOperationAddField {
		t.Errorf("expected add_field, got %s", diffs[1].Operation)
	}
}

func TestDiffTablesAddWithIndexesEventsPermissions(t *testing.T) {
	newT := schema.NewTable("post",
		schema.WithFields(schema.StringField("title")),
		schema.WithIndexes(schema.UniqueIndex("title_idx", []string{"title"})),
		schema.WithEvents(schema.NewEvent("touch", "$before != $after", "UPDATE post SET updated = time::now()")),
		schema.WithTablePermissions(map[string]string{"select": "true"}),
	)
	diffs, err := DiffTables(nil, &newT)

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}
	if len(diffs) != 5 {
		t.Fatalf("expected 5 diffs, got %d (%v)", len(diffs), diffs)
	}
	got := make([]DiffOperation, len(diffs))
	for i, d := range diffs {
		got[i] = d.Operation
	}
	want := []DiffOperation{
		DiffOperationAddTable, DiffOperationAddField,
		DiffOperationAddIndex, DiffOperationAddEvent,
		DiffOperationModifyPermissions,
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("diffs[%d] = %s, want %s", i, got[i], w)
		}
	}
}

func TestDiffTablesDropOnly(t *testing.T) {
	oldT := schema.NewTable("legacy", schema.WithMode(schema.TableModeSchemaless))
	diffs, err := DiffTables(&oldT, nil)

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}
	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(diffs))
	}
	d := diffs[0]
	if d.Operation != DiffOperationDropTable {
		t.Errorf("op = %s, want drop_table", d.Operation)
	}
	if d.ForwardSQL != "REMOVE TABLE legacy;" {
		t.Errorf("forward sql = %q", d.ForwardSQL)
	}
	if d.BackwardSQL != "DEFINE TABLE legacy SCHEMALESS;" {
		t.Errorf("backward sql = %q", d.BackwardSQL)
	}
}

func TestDiffTablesBothNil(t *testing.T) {
	diffs, err := DiffTables(nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(diffs) != 0 {
		t.Errorf("expected empty diffs, got %d", len(diffs))
	}
}

func TestDiffTablesNoChange(t *testing.T) {
	old := schema.NewTable("user", schema.WithFields(schema.StringField("email")))
	new := schema.NewTable("user", schema.WithFields(schema.StringField("email")))
	diffs, err := DiffTables(&old, &new)

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}
	if len(diffs) != 0 {
		t.Errorf("expected no diffs, got %d: %v", len(diffs), diffs)
	}
}

// --- DiffFields --------------------------------------------------------------

func TestDiffFieldsAdded(t *testing.T) {
	old := schema.NewTable("user")
	new := schema.NewTable("user", schema.WithFields(schema.StringField("email")))
	diffs, err := DiffFields(old, new)

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}
	if len(diffs) != 1 || diffs[0].Operation != DiffOperationAddField {
		t.Fatalf("expected 1 add_field, got %v", diffs)
	}
	if diffs[0].Field != "email" {
		t.Errorf("field = %s", diffs[0].Field)
	}
}

func TestDiffFieldsAddedWithDefaultBackfill(t *testing.T) {
	old := schema.NewTable("user")
	new := schema.NewTable("user", schema.WithFields(
		schema.StringField("created_at", schema.WithDefault("time::now()")),
	))
	diffs, err := DiffFields(old, new)

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}
	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(diffs))
	}
	if !strings.Contains(diffs[0].ForwardSQL, "UPDATE user SET created_at = time::now() WHERE created_at IS NONE") {
		t.Errorf("expected backfill in forward sql, got: %s", diffs[0].ForwardSQL)
	}
}

func TestDiffFieldsAddedWithUnsafeDefault(t *testing.T) {
	new := schema.NewTable("user", schema.WithFields(
		schema.StringField("email", schema.WithDefault("'x'; DROP TABLE user; --")),
	))
	_, err := DiffFields(schema.NewTable("user"), new)
	if err == nil {
		t.Fatal("expected validation error for unsafe default")
	}
	if !errors.Is(err, surqlerrors.ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

func TestDiffFieldsDropped(t *testing.T) {
	old := schema.NewTable("user", schema.WithFields(schema.StringField("email")))
	new := schema.NewTable("user")
	diffs, err := DiffFields(old, new)

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}
	if len(diffs) != 1 || diffs[0].Operation != DiffOperationDropField {
		t.Fatalf("expected 1 drop_field, got %v", diffs)
	}
	if !strings.Contains(diffs[0].BackwardSQL, "DEFINE FIELD email ON TABLE user TYPE string") {
		t.Errorf("unexpected backward sql: %s", diffs[0].BackwardSQL)
	}
}

func TestDiffFieldsModifiedType(t *testing.T) {
	old := schema.NewTable("user", schema.WithFields(schema.StringField("age")))
	new := schema.NewTable("user", schema.WithFields(schema.IntField("age")))
	diffs, err := DiffFields(old, new)

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}
	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(diffs))
	}
	d := diffs[0]
	if d.Operation != DiffOperationModifyField {
		t.Errorf("op = %s", d.Operation)
	}
	if d.Details["old_type"] != "string" || d.Details["new_type"] != "int" {
		t.Errorf("details = %v", d.Details)
	}
}

func TestDiffFieldsModifiedAssertion(t *testing.T) {
	old := schema.NewTable("user", schema.WithFields(
		schema.StringField("email", schema.WithAssertion("string::is::email($value)")),
	))
	new := schema.NewTable("user", schema.WithFields(
		schema.StringField("email", schema.WithAssertion("string::len($value) > 3")),
	))
	diffs, err := DiffFields(old, new)

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}
	if len(diffs) != 1 || diffs[0].Operation != DiffOperationModifyField {
		t.Fatalf("expected modify_field, got %v", diffs)
	}
}

func TestDiffFieldsModifiedReadonlyFlexible(t *testing.T) {
	old := schema.NewTable("user", schema.WithFields(schema.NewField("meta", schema.FieldTypeObject)))
	new := schema.NewTable("user", schema.WithFields(schema.NewField("meta", schema.FieldTypeObject, schema.WithFlexible(true))))
	diffs, err := DiffFields(old, new)

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}
	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(diffs))
	}
}

func TestDiffFieldsNoChange(t *testing.T) {
	f := schema.StringField("email", schema.WithAssertion("string::len($value) > 0"))
	old := schema.NewTable("user", schema.WithFields(f))
	new := schema.NewTable("user", schema.WithFields(f))
	diffs, err := DiffFields(old, new)

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}
	if len(diffs) != 0 {
		t.Errorf("expected no diffs, got %v", diffs)
	}
}

func TestDiffFieldsMixedOperations(t *testing.T) {
	old := schema.NewTable("user", schema.WithFields(
		schema.StringField("email"),
		schema.StringField("legacy"),
		schema.IntField("age"),
	))
	new := schema.NewTable("user", schema.WithFields(
		schema.StringField("email"),
		schema.FloatField("age"),   // modified
		schema.BoolField("active"), // added
	))
	diffs, err := DiffFields(old, new)

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}
	if len(diffs) != 3 {
		t.Fatalf("expected 3 diffs, got %d", len(diffs))
	}
	if len(findByOp(diffs, DiffOperationAddField)) != 1 {
		t.Errorf("expected 1 add_field, got %d", len(findByOp(diffs, DiffOperationAddField)))
	}
	if len(findByOp(diffs, DiffOperationDropField)) != 1 {
		t.Errorf("expected 1 drop_field, got %d", len(findByOp(diffs, DiffOperationDropField)))
	}
	if len(findByOp(diffs, DiffOperationModifyField)) != 1 {
		t.Errorf("expected 1 modify_field, got %d", len(findByOp(diffs, DiffOperationModifyField)))
	}
}

func TestFieldsEqualValueAndReadOnly(t *testing.T) {
	a := schema.ComputedField("full", "string::concat(first, ' ', last)", schema.FieldTypeString)
	b := schema.ComputedField("full", "string::concat(first, ' ', last)", schema.FieldTypeString)
	if !fieldsEqual(a, b) {
		t.Fatal("expected fields equal")
	}
	c := schema.ComputedField("full", "string::concat(first, last)", schema.FieldTypeString)
	if fieldsEqual(a, c) {
		t.Fatal("expected fields unequal when value differs")
	}
}

// --- DiffIndexes -------------------------------------------------------------

func TestDiffIndexesAddedStandard(t *testing.T) {
	old := schema.NewTable("post")
	new := schema.NewTable("post", schema.WithIndexes(
		schema.NewIndex("title_idx", []string{"title"}, schema.IndexTypeStandard),
	))
	diffs, err := DiffIndexes(old, new)

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}
	if len(diffs) != 1 || diffs[0].Operation != DiffOperationAddIndex {
		t.Fatalf("expected 1 add_index, got %v", diffs)
	}
	if !strings.HasPrefix(diffs[0].ForwardSQL, "DEFINE INDEX title_idx ON TABLE post COLUMNS title") {
		t.Errorf("unexpected sql: %s", diffs[0].ForwardSQL)
	}
}

func TestDiffIndexesAddedUnique(t *testing.T) {
	new := schema.NewTable("user", schema.WithIndexes(
		schema.UniqueIndex("email_uq", []string{"email"}),
	))
	diffs, err := DiffIndexes(schema.NewTable("user"), new)

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}
	if !strings.Contains(diffs[0].ForwardSQL, "UNIQUE") {
		t.Errorf("expected UNIQUE in sql: %s", diffs[0].ForwardSQL)
	}
}

func TestDiffIndexesAddedMultiColumn(t *testing.T) {
	new := schema.NewTable("event", schema.WithIndexes(
		schema.NewIndex("by_ts_user", []string{"timestamp", "user_id"}, schema.IndexTypeStandard),
	))
	diffs, err := DiffIndexes(schema.NewTable("event"), new)

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}
	if !strings.Contains(diffs[0].ForwardSQL, "COLUMNS timestamp, user_id") {
		t.Errorf("expected multi-column sql, got: %s", diffs[0].ForwardSQL)
	}
}

func TestDiffIndexesDropped(t *testing.T) {
	old := schema.NewTable("post", schema.WithIndexes(
		schema.NewIndex("title_idx", []string{"title"}, schema.IndexTypeStandard),
	))
	new := schema.NewTable("post")
	diffs, err := DiffIndexes(old, new)

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}
	if len(diffs) != 1 || diffs[0].Operation != DiffOperationDropIndex {
		t.Fatalf("expected drop_index, got %v", diffs)
	}
	if diffs[0].ForwardSQL != "REMOVE INDEX title_idx ON TABLE post;" {
		t.Errorf("unexpected sql: %s", diffs[0].ForwardSQL)
	}
}

func TestDiffIndexesNoChange(t *testing.T) {
	idx := schema.UniqueIndex("email_uq", []string{"email"})
	old := schema.NewTable("user", schema.WithIndexes(idx))
	new := schema.NewTable("user", schema.WithIndexes(idx))
	diffs, err := DiffIndexes(old, new)

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}
	if len(diffs) != 0 {
		t.Errorf("expected no diffs, got %v", diffs)
	}
}

// --- MTREE / HNSW ------------------------------------------------------------

func TestDiffIndexesAddMTree(t *testing.T) {
	new := schema.NewTable("documents", schema.WithIndexes(
		schema.MTreeIndex("emb_idx", "embedding", 1536, schema.MTreeIndexOptions{}),
	))
	diffs, err := DiffIndexes(schema.NewTable("documents"), new)

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}
	expect := "DEFINE INDEX emb_idx ON TABLE documents COLUMNS embedding MTREE DIMENSION 1536 DIST EUCLIDEAN TYPE F64;"
	if diffs[0].ForwardSQL != expect {
		t.Errorf("mtree sql mismatch.\n  got:  %s\n  want: %s", diffs[0].ForwardSQL, expect)
	}
}

func TestDiffIndexesAddMTreeCosine(t *testing.T) {
	new := schema.NewTable("documents", schema.WithIndexes(
		schema.MTreeIndex("emb", "embedding", 768, schema.MTreeIndexOptions{
			Distance:   schema.MTreeDistanceCosine,
			VectorType: schema.MTreeVectorF32,
		}),
	))
	diffs, err := DiffIndexes(schema.NewTable("documents"), new)

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}
	if !strings.Contains(diffs[0].ForwardSQL, "DIST COSINE") ||
		!strings.Contains(diffs[0].ForwardSQL, "TYPE F32") {
		t.Errorf("unexpected sql: %s", diffs[0].ForwardSQL)
	}
}

func TestDiffIndexesAddMTreeNoDimension(t *testing.T) {
	new := schema.NewTable("documents", schema.WithIndexes(schema.IndexDefinition{
		Name:    "bad",
		Columns: []string{"embedding"},
		Type:    schema.IndexTypeMTree,
	}))
	_, err := DiffIndexes(schema.NewTable("documents"), new)
	if err == nil || !errors.Is(err, surqlerrors.ErrValidation) {
		t.Fatalf("expected ErrValidation, got %v", err)
	}
}

func TestDiffIndexesDropMTree(t *testing.T) {
	idx := schema.MTreeIndex("emb_idx", "embedding", 1536, schema.MTreeIndexOptions{})
	old := schema.NewTable("documents", schema.WithIndexes(idx))
	new := schema.NewTable("documents")
	diffs, err := DiffIndexes(old, new)

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}
	if diffs[0].Operation != DiffOperationDropIndex {
		t.Errorf("op = %s", diffs[0].Operation)
	}
	if !strings.Contains(diffs[0].BackwardSQL, "MTREE DIMENSION 1536") {
		t.Errorf("expected MTREE restore in backward sql: %s", diffs[0].BackwardSQL)
	}
}

func TestDiffIndexesAddHNSW(t *testing.T) {
	new := schema.NewTable("vectors", schema.WithIndexes(
		schema.HnswIndex("vec_idx", "vector", 512, schema.HnswIndexOptions{}),
	))
	diffs, err := DiffIndexes(schema.NewTable("vectors"), new)

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}
	if !strings.Contains(diffs[0].ForwardSQL, "HNSW DIMENSION 512") {
		t.Errorf("hnsw sql: %s", diffs[0].ForwardSQL)
	}
}

func TestDiffIndexesAddHNSWWithTuning(t *testing.T) {
	new := schema.NewTable("vectors", schema.WithIndexes(
		schema.HnswIndex("vec_idx", "vector", 128, schema.HnswIndexOptions{
			Distance: schema.HnswDistanceCosine, EFC: 200, M: 32,
		}),
	))
	diffs, err := DiffIndexes(schema.NewTable("vectors"), new)

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}
	sql := diffs[0].ForwardSQL
	if !strings.Contains(sql, "EFC 200") || !strings.Contains(sql, "M 32") ||
		!strings.Contains(sql, "DIST COSINE") {
		t.Errorf("unexpected sql: %s", sql)
	}
}

func TestDiffIndexesAddHNSWNoDimension(t *testing.T) {
	new := schema.NewTable("v", schema.WithIndexes(schema.IndexDefinition{
		Name: "x", Columns: []string{"vec"}, Type: schema.IndexTypeHNSW,
	}))
	_, err := DiffIndexes(schema.NewTable("v"), new)
	if err == nil || !errors.Is(err, surqlerrors.ErrValidation) {
		t.Fatalf("expected ErrValidation, got %v", err)
	}
}

func TestDiffIndexesDropHNSW(t *testing.T) {
	idx := schema.HnswIndex("vec_idx", "vector", 256, schema.HnswIndexOptions{})
	old := schema.NewTable("vectors", schema.WithIndexes(idx))
	new := schema.NewTable("vectors")
	diffs, err := DiffIndexes(old, new)

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}
	if !strings.Contains(diffs[0].BackwardSQL, "HNSW DIMENSION 256") {
		t.Errorf("expected HNSW in backward: %s", diffs[0].BackwardSQL)
	}
}

func TestDiffIndexesMixedTypes(t *testing.T) {
	old := schema.NewTable("v")
	new := schema.NewTable("v", schema.WithIndexes(
		schema.UniqueIndex("name_uq", []string{"name"}),
		schema.MTreeIndex("emb_idx", "embedding", 512, schema.MTreeIndexOptions{}),
		schema.HnswIndex("vec_idx", "vector", 256, schema.HnswIndexOptions{}),
	))
	diffs, err := DiffIndexes(old, new)

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}
	if len(diffs) != 3 {
		t.Fatalf("expected 3 add_index diffs, got %d", len(diffs))
	}
}

// --- DiffEvents --------------------------------------------------------------

func TestDiffEventsAdded(t *testing.T) {
	new := schema.NewTable("user", schema.WithEvents(
		schema.NewEvent("touch", "$before.updated != $after.updated", "UPDATE user SET updated_at = time::now()"),
	))
	diffs, err := DiffEvents(schema.NewTable("user"), new)

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}
	if len(diffs) != 1 || diffs[0].Operation != DiffOperationAddEvent {
		t.Fatalf("expected add_event, got %v", diffs)
	}
	if !strings.Contains(diffs[0].ForwardSQL, "DEFINE EVENT touch ON TABLE user WHEN") {
		t.Errorf("unexpected sql: %s", diffs[0].ForwardSQL)
	}
}

func TestDiffEventsDropped(t *testing.T) {
	old := schema.NewTable("user", schema.WithEvents(
		schema.NewEvent("touch", "true", "UPDATE user SET t = time::now()"),
	))
	diffs, err := DiffEvents(old, schema.NewTable("user"))

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}
	if diffs[0].Operation != DiffOperationDropEvent {
		t.Errorf("op = %s", diffs[0].Operation)
	}
	if diffs[0].ForwardSQL != "REMOVE EVENT touch ON TABLE user;" {
		t.Errorf("forward sql: %s", diffs[0].ForwardSQL)
	}
}

func TestDiffEventsNoChange(t *testing.T) {
	ev := schema.NewEvent("touch", "true", "UPDATE user SET t = time::now()")
	old := schema.NewTable("user", schema.WithEvents(ev))
	new := schema.NewTable("user", schema.WithEvents(ev))
	diffs, err := DiffEvents(old, new)

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}
	if len(diffs) != 0 {
		t.Errorf("expected no diffs, got %v", diffs)
	}
}

func TestDiffEventsUnsafeCondition(t *testing.T) {
	new := schema.NewTable("user", schema.WithEvents(
		schema.NewEvent("bad", "true; DROP TABLE user;", "UPDATE user SET t = time::now()"),
	))
	_, err := DiffEvents(schema.NewTable("user"), new)
	if err == nil || !errors.Is(err, surqlerrors.ErrValidation) {
		t.Fatalf("expected ErrValidation, got %v", err)
	}
}

func TestDiffEventsUnsafeAction(t *testing.T) {
	new := schema.NewTable("user", schema.WithEvents(
		schema.NewEvent("bad", "true", "UPDATE user SET t = time::now() -- drop"),
	))
	_, err := DiffEvents(schema.NewTable("user"), new)
	if err == nil || !errors.Is(err, surqlerrors.ErrValidation) {
		t.Fatalf("expected ErrValidation, got %v", err)
	}
}

func TestValidateEventExpressionTrailingSemicolon(t *testing.T) {
	if err := validateEventExpression("true;", "condition"); err == nil {
		t.Error("expected error for trailing semicolon")
	}
}

func TestValidateEventExpressionComment(t *testing.T) {
	if err := validateEventExpression("true -- nope", "condition"); err == nil {
		t.Error("expected error for SQL comment")
	}
}

func TestValidateEventExpressionValid(t *testing.T) {
	if err := validateEventExpression("$before != $after", "condition"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- DiffPermissions ---------------------------------------------------------

func TestDiffPermissionsAdded(t *testing.T) {
	old := schema.NewTable("user")
	new := schema.NewTable("user", schema.WithTablePermissions(map[string]string{"select": "true"}))
	diffs := DiffPermissions(old, new)
	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(diffs))
	}
	if diffs[0].Operation != DiffOperationModifyPermissions {
		t.Errorf("op = %s", diffs[0].Operation)
	}
	if !strings.Contains(diffs[0].ForwardSQL, "DEFINE FIELD PERMISSIONS FOR SELECT ON TABLE user WHERE true") {
		t.Errorf("unexpected forward sql: %s", diffs[0].ForwardSQL)
	}
	if diffs[0].BackwardSQL != "" {
		t.Errorf("expected empty backward sql, got %q", diffs[0].BackwardSQL)
	}
}

func TestDiffPermissionsChanged(t *testing.T) {
	old := schema.NewTable("user", schema.WithTablePermissions(map[string]string{"select": "false"}))
	new := schema.NewTable("user", schema.WithTablePermissions(map[string]string{"select": "true"}))
	diffs := DiffPermissions(old, new)
	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(diffs))
	}
	if !strings.Contains(diffs[0].BackwardSQL, "WHERE false") {
		t.Errorf("expected old perms in backward sql: %s", diffs[0].BackwardSQL)
	}
	if !strings.Contains(diffs[0].ForwardSQL, "WHERE true") {
		t.Errorf("expected new perms in forward sql: %s", diffs[0].ForwardSQL)
	}
}

func TestDiffPermissionsRemoved(t *testing.T) {
	old := schema.NewTable("user", schema.WithTablePermissions(map[string]string{"select": "true"}))
	new := schema.NewTable("user")
	diffs := DiffPermissions(old, new)
	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(diffs))
	}
	if diffs[0].ForwardSQL != "" {
		t.Errorf("expected empty forward sql, got %q", diffs[0].ForwardSQL)
	}
	if !strings.Contains(diffs[0].BackwardSQL, "WHERE true") {
		t.Errorf("unexpected backward sql: %s", diffs[0].BackwardSQL)
	}
}

func TestDiffPermissionsNoChange(t *testing.T) {
	perms := map[string]string{"select": "true"}
	old := schema.NewTable("user", schema.WithTablePermissions(perms))
	new := schema.NewTable("user", schema.WithTablePermissions(perms))
	diffs := DiffPermissions(old, new)
	if len(diffs) != 0 {
		t.Errorf("expected no diffs, got %v", diffs)
	}
}

func TestDiffPermissionsNilVsEmptyNoChange(t *testing.T) {
	old := schema.NewTable("user")
	new := schema.NewTable("user", schema.WithTablePermissions(map[string]string{}))
	diffs := DiffPermissions(old, new)
	if len(diffs) != 0 {
		t.Errorf("expected nil and empty map to match, got %v", diffs)
	}
}

func TestDiffPermissionsMultipleStableOrder(t *testing.T) {
	new := schema.NewTable("user", schema.WithTablePermissions(map[string]string{
		"select": "true", "update": "false", "create": "true",
	}))
	diffs := DiffPermissions(schema.NewTable("user"), new)
	sql := diffs[0].ForwardSQL
	// Sorted key order: create, select, update
	iCreate := strings.Index(sql, "FOR CREATE")
	iSelect := strings.Index(sql, "FOR SELECT")
	iUpdate := strings.Index(sql, "FOR UPDATE")
	if !(iCreate < iSelect && iSelect < iUpdate) {
		t.Errorf("expected sorted order, got: %s", sql)
	}
}

// --- DiffEdges ---------------------------------------------------------------

func TestDiffEdgesAdded(t *testing.T) {
	e := schema.TypedEdge("likes", "user", "post")
	diffs, err := DiffEdges(nil, &e)

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}
	if len(diffs) != 1 || diffs[0].Operation != DiffOperationAddTable {
		t.Fatalf("expected 1 add_table for edge, got %v", diffs)
	}
	expect := "DEFINE TABLE likes TYPE RELATION FROM user TO post;"
	if diffs[0].ForwardSQL != expect {
		t.Errorf("edge sql = %q, want %q", diffs[0].ForwardSQL, expect)
	}
}

func TestDiffEdgesAddedSchemafull(t *testing.T) {
	e := schema.NewEdge("owns", schema.WithEdgeMode(schema.EdgeModeSchemafull))
	diffs, err := DiffEdges(nil, &e)

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}
	if diffs[0].ForwardSQL != "DEFINE TABLE owns SCHEMAFULL;" {
		t.Errorf("sql = %q", diffs[0].ForwardSQL)
	}
}

func TestDiffEdgesAddedSchemaless(t *testing.T) {
	e := schema.NewEdge("rel", schema.WithEdgeMode(schema.EdgeModeSchemaless))
	diffs, err := DiffEdges(nil, &e)

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}
	if diffs[0].ForwardSQL != "DEFINE TABLE rel SCHEMALESS;" {
		t.Errorf("sql = %q", diffs[0].ForwardSQL)
	}
}

func TestDiffEdgesAddedWithFieldsIndexesEvents(t *testing.T) {
	e := schema.TypedEdge("likes", "user", "post",
		schema.WithEdgeFields(schema.IntField("weight")),
		schema.WithEdgeIndexes(schema.NewIndex("w_idx", []string{"weight"}, schema.IndexTypeStandard)),
		schema.WithEdgeEvents(schema.NewEvent("touch", "true", "UPDATE likes SET t = time::now()")),
	)
	diffs, err := DiffEdges(nil, &e)

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}
	if len(diffs) != 4 {
		t.Fatalf("expected 4 diffs (table + field + index + event), got %d", len(diffs))
	}
}

func TestDiffEdgesDropped(t *testing.T) {
	e := schema.TypedEdge("likes", "user", "post")
	diffs, err := DiffEdges(&e, nil)

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}
	if len(diffs) != 1 || diffs[0].Operation != DiffOperationDropTable {
		t.Fatalf("expected drop_table, got %v", diffs)
	}
	if diffs[0].BackwardSQL != "" {
		t.Errorf("expected empty backward sql for dropped edge, got %q", diffs[0].BackwardSQL)
	}
}

func TestDiffEdgesFieldAdded(t *testing.T) {
	oldE := schema.TypedEdge("likes", "user", "post")
	newE := schema.TypedEdge("likes", "user", "post",
		schema.WithEdgeFields(schema.IntField("weight")))
	diffs, err := DiffEdges(&oldE, &newE)

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}
	if len(diffs) != 1 || diffs[0].Operation != DiffOperationAddField {
		t.Fatalf("expected add_field, got %v", diffs)
	}
	if diffs[0].Field != "weight" {
		t.Errorf("field = %s", diffs[0].Field)
	}
}

func TestDiffEdgesFieldRemoved(t *testing.T) {
	oldE := schema.TypedEdge("likes", "user", "post",
		schema.WithEdgeFields(schema.IntField("weight")))
	newE := schema.TypedEdge("likes", "user", "post")
	diffs, err := DiffEdges(&oldE, &newE)

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}
	if len(diffs) != 1 || diffs[0].Operation != DiffOperationDropField {
		t.Fatalf("expected drop_field, got %v", diffs)
	}
}

func TestDiffEdgesFieldModified(t *testing.T) {
	oldE := schema.TypedEdge("likes", "user", "post",
		schema.WithEdgeFields(schema.IntField("weight")))
	newE := schema.TypedEdge("likes", "user", "post",
		schema.WithEdgeFields(schema.FloatField("weight")))
	diffs, err := DiffEdges(&oldE, &newE)

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}
	if len(diffs) != 1 || diffs[0].Operation != DiffOperationModifyField {
		t.Fatalf("expected modify_field, got %v", diffs)
	}
}

func TestDiffEdgesIndexAdded(t *testing.T) {
	oldE := schema.TypedEdge("likes", "user", "post")
	newE := schema.TypedEdge("likes", "user", "post",
		schema.WithEdgeIndexes(schema.NewIndex("w_idx", []string{"weight"}, schema.IndexTypeStandard)))
	diffs, err := DiffEdges(&oldE, &newE)

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}
	if diffs[0].Operation != DiffOperationAddIndex {
		t.Errorf("op = %s", diffs[0].Operation)
	}
}

func TestDiffEdgesEventAdded(t *testing.T) {
	oldE := schema.TypedEdge("likes", "user", "post")
	newE := schema.TypedEdge("likes", "user", "post",
		schema.WithEdgeEvents(schema.NewEvent("touch", "true", "UPDATE likes SET t = time::now()")))
	diffs, err := DiffEdges(&oldE, &newE)

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}
	if diffs[0].Operation != DiffOperationAddEvent {
		t.Errorf("op = %s", diffs[0].Operation)
	}
}

func TestDiffEdgesPermissionsChanged(t *testing.T) {
	oldE := schema.TypedEdge("likes", "user", "post",
		schema.WithEdgePermissions(map[string]string{"select": "false"}))
	newE := schema.TypedEdge("likes", "user", "post",
		schema.WithEdgePermissions(map[string]string{"select": "true"}))
	diffs, err := DiffEdges(&oldE, &newE)

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}
	if len(diffs) != 1 || diffs[0].Operation != DiffOperationModifyPermissions {
		t.Fatalf("expected modify_permissions, got %v", diffs)
	}
}

func TestDiffEdgesNoChange(t *testing.T) {
	e := schema.TypedEdge("likes", "user", "post",
		schema.WithEdgeFields(schema.IntField("weight")))
	diffs, err := DiffEdges(&e, &e)

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}
	if len(diffs) != 0 {
		t.Errorf("expected no diffs, got %v", diffs)
	}
}

func TestDiffEdgesBothNil(t *testing.T) {
	diffs, err := DiffEdges(nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if diffs != nil && len(diffs) != 0 {
		t.Errorf("expected empty, got %v", diffs)
	}
}

// --- DiffSchemas -------------------------------------------------------------

func TestDiffSchemasAddTable(t *testing.T) {
	code := SchemaSnapshot{Tables: []schema.TableDefinition{schema.NewTable("user")}}
	db := SchemaSnapshot{}
	diffs, err := DiffSchemas(code, db)

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}
	if len(diffs) != 1 || diffs[0].Operation != DiffOperationAddTable {
		t.Fatalf("expected add_table, got %v", diffs)
	}
}

func TestDiffSchemasDropTable(t *testing.T) {
	code := SchemaSnapshot{}
	db := SchemaSnapshot{Tables: []schema.TableDefinition{schema.NewTable("legacy")}}
	diffs, err := DiffSchemas(code, db)

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}
	if len(diffs) != 1 || diffs[0].Operation != DiffOperationDropTable {
		t.Fatalf("expected drop_table, got %v", diffs)
	}
}

func TestDiffSchemasModifyTable(t *testing.T) {
	code := SchemaSnapshot{Tables: []schema.TableDefinition{
		schema.NewTable("user", schema.WithFields(schema.StringField("email"), schema.IntField("age"))),
	}}
	db := SchemaSnapshot{Tables: []schema.TableDefinition{
		schema.NewTable("user", schema.WithFields(schema.StringField("email"))),
	}}
	diffs, err := DiffSchemas(code, db)

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}
	if len(diffs) != 1 || diffs[0].Operation != DiffOperationAddField {
		t.Fatalf("expected add_field, got %v", diffs)
	}
}

func TestDiffSchemasAddEdge(t *testing.T) {
	code := SchemaSnapshot{Edges: []schema.EdgeDefinition{schema.TypedEdge("likes", "user", "post")}}
	db := SchemaSnapshot{}
	diffs, err := DiffSchemas(code, db)

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}
	if len(diffs) != 1 || diffs[0].Operation != DiffOperationAddTable {
		t.Fatalf("expected edge add_table, got %v", diffs)
	}
}

func TestDiffSchemasDropEdge(t *testing.T) {
	code := SchemaSnapshot{}
	db := SchemaSnapshot{Edges: []schema.EdgeDefinition{schema.TypedEdge("likes", "user", "post")}}
	diffs, err := DiffSchemas(code, db)

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}
	if len(diffs) != 1 || diffs[0].Operation != DiffOperationDropTable {
		t.Fatalf("expected drop_table, got %v", diffs)
	}
}

func TestDiffSchemasEmptyBoth(t *testing.T) {
	diffs, err := DiffSchemas(SchemaSnapshot{}, SchemaSnapshot{})

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}
	if len(diffs) != 0 {
		t.Errorf("expected empty, got %v", diffs)
	}
}

func TestDiffSchemasNoChange(t *testing.T) {
	snap := SchemaSnapshot{
		Tables: []schema.TableDefinition{schema.NewTable("user", schema.WithFields(schema.StringField("email")))},
		Edges:  []schema.EdgeDefinition{schema.TypedEdge("likes", "user", "post")},
	}
	diffs, err := DiffSchemas(snap, snap)

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}
	if len(diffs) != 0 {
		t.Errorf("expected no diffs, got %v", diffs)
	}
}

func TestDiffSchemasComplex(t *testing.T) {
	code := SchemaSnapshot{
		Tables: []schema.TableDefinition{
			schema.NewTable("user", schema.WithFields(schema.StringField("email"), schema.StringField("name"))),
			schema.NewTable("post", schema.WithFields(schema.StringField("title"))),
		},
		Edges: []schema.EdgeDefinition{
			schema.TypedEdge("likes", "user", "post"),
		},
	}
	db := SchemaSnapshot{
		Tables: []schema.TableDefinition{
			schema.NewTable("user", schema.WithFields(schema.StringField("email"))),
			schema.NewTable("legacy"),
		},
	}
	diffs, err := DiffSchemas(code, db)

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}
	// Expected: add post (table + title field), drop legacy, add user.name, add likes edge.
	opsSeen := map[DiffOperation]int{}
	for _, d := range diffs {
		opsSeen[d.Operation]++
	}
	if opsSeen[DiffOperationAddTable] != 2 {
		t.Errorf("add_table count = %d, want 2 (post + likes)", opsSeen[DiffOperationAddTable])
	}
	if opsSeen[DiffOperationDropTable] != 1 {
		t.Errorf("drop_table count = %d, want 1 (legacy)", opsSeen[DiffOperationDropTable])
	}
	if opsSeen[DiffOperationAddField] != 2 {
		t.Errorf("add_field count = %d, want 2 (post.title, user.name)", opsSeen[DiffOperationAddField])
	}
}

func TestDiffSchemasModifyTableSortedOrder(t *testing.T) {
	// Modified tables should be emitted in sorted name order for determinism.
	code := SchemaSnapshot{Tables: []schema.TableDefinition{
		schema.NewTable("zebra", schema.WithFields(schema.StringField("stripes"))),
		schema.NewTable("alpha", schema.WithFields(schema.StringField("name"))),
	}}
	db := SchemaSnapshot{Tables: []schema.TableDefinition{
		schema.NewTable("zebra"),
		schema.NewTable("alpha"),
	}}
	diffs, err := DiffSchemas(code, db)

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}
	if len(diffs) != 2 {
		t.Fatalf("expected 2 diffs, got %d", len(diffs))
	}
	if diffs[0].Table != "alpha" || diffs[1].Table != "zebra" {
		t.Errorf("expected sorted table order, got %s, %s", diffs[0].Table, diffs[1].Table)
	}
}

// --- Default-value validation ------------------------------------------------

func TestValidateDefaultValueFunctionCall(t *testing.T) {
	if err := validateDefaultValue("time::now()"); err != nil {
		t.Errorf("unexpected: %v", err)
	}
	if err := validateDefaultValue("rand::uuid()"); err != nil {
		t.Errorf("unexpected: %v", err)
	}
}

func TestValidateDefaultValueLiterals(t *testing.T) {
	cases := []string{"42", "-1", "3.14", "true", "false", "NONE", "NULL", "'hello'", "$session"}
	for _, c := range cases {
		if err := validateDefaultValue(c); err != nil {
			t.Errorf("expected %q safe, got %v", c, err)
		}
	}
}

func TestValidateDefaultValueUnsafe(t *testing.T) {
	cases := []string{
		"'x'; DROP TABLE user; --",
		"1; SELECT",
		"foo + 1", // not a pure function call
	}
	for _, c := range cases {
		if err := validateDefaultValue(c); err == nil {
			t.Errorf("expected %q unsafe", c)
		}
	}
}

// --- Sanity: modify table routes all child diffs -----------------------------

func TestDiffTablesModifyFieldAndIndex(t *testing.T) {
	old := schema.NewTable("user",
		schema.WithFields(schema.StringField("email")),
		schema.WithIndexes(schema.UniqueIndex("email_uq", []string{"email"})),
	)
	new := schema.NewTable("user",
		schema.WithFields(schema.StringField("email"), schema.IntField("age")),
		schema.WithIndexes(
			schema.UniqueIndex("email_uq", []string{"email"}),
			schema.NewIndex("age_idx", []string{"age"}, schema.IndexTypeStandard),
		),
	)
	diffs, err := DiffTables(&old, &new)

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}
	if len(findByOp(diffs, DiffOperationAddField)) != 1 {
		t.Errorf("expected 1 add_field, got %d", len(findByOp(diffs, DiffOperationAddField)))
	}
	if len(findByOp(diffs, DiffOperationAddIndex)) != 1 {
		t.Errorf("expected 1 add_index, got %d", len(findByOp(diffs, DiffOperationAddIndex)))
	}
}
