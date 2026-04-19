package migration

import (
	"context"
	stdErrors "errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

// writeMigrationFixture creates a minimal, discoverable migration file
// directly in dir so squash tests can operate on a deterministic set of
// versions without depending on the generator's clock.
func writeMigrationFixture(t *testing.T, dir, version, slug, body string) string {
	t.Helper()
	filename := version + "_" + slug + ".surql"
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write fixture %q: %v", filename, err)
	}
	return path
}

func TestOptimizeStatements_RemovesDefineRemovePair(t *testing.T) {
	input := []string{
		"DEFINE TABLE user SCHEMAFULL;",
		"DEFINE FIELD temp ON TABLE user TYPE string;",
		"REMOVE FIELD temp ON TABLE user;",
	}
	out, count := optimizeStatements(input)
	if count != 2 {
		t.Errorf("optimizations = %d, want 2", count)
	}
	if len(out) != 1 || out[0] != "DEFINE TABLE user SCHEMAFULL;" {
		t.Errorf("out = %v, want [DEFINE TABLE user SCHEMAFULL;]", out)
	}
}

func TestOptimizeStatements_DefineRemoveTablePair(t *testing.T) {
	input := []string{
		"DEFINE TABLE tmp SCHEMAFULL;",
		"DEFINE TABLE keep SCHEMAFULL;",
		"REMOVE TABLE tmp;",
	}
	out, count := optimizeStatements(input)
	if count != 2 {
		t.Errorf("optimizations = %d, want 2", count)
	}
	if len(out) != 1 || out[0] != "DEFINE TABLE keep SCHEMAFULL;" {
		t.Errorf("out = %v, want [DEFINE TABLE keep SCHEMAFULL;]", out)
	}
}

func TestOptimizeStatements_DuplicateDefineKeepsLast(t *testing.T) {
	input := []string{
		"DEFINE FIELD name ON TABLE user TYPE string;",
		"DEFINE FIELD name ON TABLE user TYPE string ASSERT $value != NONE;",
	}
	out, count := optimizeStatements(input)
	if count != 1 {
		t.Errorf("optimizations = %d, want 1", count)
	}
	if len(out) != 1 ||
		out[0] != "DEFINE FIELD name ON TABLE user TYPE string ASSERT $value != NONE;" {
		t.Errorf("out = %v, want last-wins", out)
	}
}

func TestOptimizeStatements_RemovesOrphanedUpdate(t *testing.T) {
	input := []string{
		"DEFINE FIELD temp ON TABLE user TYPE string;",
		"UPDATE user SET temp = 'value' WHERE id = 1;",
		"REMOVE FIELD temp ON TABLE user;",
	}
	out, count := optimizeStatements(input)
	if count < 3 {
		t.Errorf("optimizations = %d, want >=3 (pair + orphan)", count)
	}
	for _, s := range out {
		if strings.Contains(strings.ToUpper(s), "UPDATE USER SET TEMP") {
			t.Errorf("orphaned UPDATE survived optimisation: %q", s)
		}
	}
}

func TestOptimizeStatements_IndexAndEvent(t *testing.T) {
	input := []string{
		"DEFINE INDEX idx_user_email ON TABLE user COLUMNS email UNIQUE;",
		"REMOVE INDEX idx_user_email ON TABLE user;",
		"DEFINE EVENT audit_user ON TABLE user WHEN $event = 'CREATE' THEN ($value);",
		"REMOVE EVENT audit_user ON TABLE user;",
	}
	out, count := optimizeStatements(input)
	if count != 4 {
		t.Errorf("optimizations = %d, want 4", count)
	}
	if len(out) != 0 {
		t.Errorf("out = %v, want []", out)
	}
}

func TestParseStatement_TableFieldIndexEvent(t *testing.T) {
	cases := []struct {
		name       string
		in         string
		op         string
		objectType string
		table      string
		field      string
		index      string
	}{
		{
			name:       "define table",
			in:         "DEFINE TABLE user SCHEMAFULL;",
			op:         "DEFINE",
			objectType: "TABLE",
			table:      "user",
		},
		{
			name:       "remove field",
			in:         "REMOVE FIELD email ON TABLE user;",
			op:         "REMOVE",
			objectType: "FIELD",
			field:      "email",
			table:      "user",
		},
		{
			name:       "define index if not exists",
			in:         "DEFINE INDEX IF NOT EXISTS idx_email ON TABLE user COLUMNS email;",
			op:         "DEFINE",
			objectType: "INDEX",
			index:      "idx_email",
			table:      "user",
		},
		{
			name:       "remove event",
			in:         "REMOVE EVENT audit ON TABLE user;",
			op:         "REMOVE",
			objectType: "EVENT",
			index:      "audit",
			table:      "user",
		},
		{
			name:       "unknown op",
			in:         "RELATE user:1->follows->user:2;",
			op:         "UNKNOWN",
			objectType: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := parseStatement(tc.in)
			if p.operation != tc.op {
				t.Errorf("operation = %q, want %q", p.operation, tc.op)
			}
			if p.objectType != tc.objectType {
				t.Errorf("objectType = %q, want %q", p.objectType, tc.objectType)
			}
			if p.tableName != tc.table {
				t.Errorf("tableName = %q, want %q", p.tableName, tc.table)
			}
			if p.fieldName != tc.field {
				t.Errorf("fieldName = %q, want %q", p.fieldName, tc.field)
			}
			if p.indexName != tc.index {
				t.Errorf("indexName = %q, want %q", p.indexName, tc.index)
			}
		})
	}
}

func TestValidateSquashSafety_FlagsDataStatements(t *testing.T) {
	migrations := []Migration{
		{
			Version: "20260101_000000",
			UpStatements: []string{
				"INSERT INTO user { name: 'alice' };",
				"UPDATE user SET active = true;",
				"UPDATE user SET backfill = 1 WHERE active IS NONE;",
				"DELETE user WHERE archived = true;",
			},
		},
	}

	warnings := validateSquashSafety(migrations)

	var hasInsert, hasUpdate, hasDelete bool
	hasBackfill := true // must remain false
	for _, w := range warnings {
		switch {
		case strings.Contains(w.Message, "INSERT"):
			hasInsert = true
			if w.Severity != "medium" {
				t.Errorf("INSERT severity = %q, want medium", w.Severity)
			}
		case strings.Contains(w.Message, "UPDATE"):
			hasUpdate = true
			// the backfill variant should NOT produce a warning.
			if strings.Contains(w.Message, "IS NONE") {
				hasBackfill = false
			}
		case strings.Contains(w.Message, "DELETE"):
			hasDelete = true
			if w.Severity != "high" {
				t.Errorf("DELETE severity = %q, want high", w.Severity)
			}
		}
	}
	if !hasInsert {
		t.Error("expected an INSERT warning")
	}
	if !hasUpdate {
		t.Error("expected an UPDATE warning")
	}
	if !hasDelete {
		t.Error("expected a DELETE warning")
	}
	if !hasBackfill {
		t.Error("unexpectedly warned about IS NONE backfill UPDATE")
	}
}

func TestFilterMigrationsByVersion(t *testing.T) {
	migrations := []Migration{
		{Version: "20260101_000000"},
		{Version: "20260102_000000"},
		{Version: "20260103_000000"},
		{Version: "20260104_000000"},
	}

	tests := []struct {
		name       string
		from, to   string
		wantCount  int
		wantFirst  string
		wantLast   string
		shouldSort bool
	}{
		{"no bounds", "", "", 4, "20260101_000000", "20260104_000000", true},
		{"from only", "20260102_000000", "", 3, "20260102_000000", "20260104_000000", true},
		{"to only", "", "20260102_000000", 2, "20260101_000000", "20260102_000000", true},
		{"both", "20260102_000000", "20260103_000000", 2, "20260102_000000", "20260103_000000", true},
		{"none match", "20260105_000000", "", 0, "", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := filterMigrationsByVersion(migrations, tc.from, tc.to)
			if len(got) != tc.wantCount {
				t.Fatalf("count = %d, want %d (got=%v)", len(got), tc.wantCount, got)
			}
			if tc.wantCount == 0 {
				return
			}
			if got[0].Version != tc.wantFirst {
				t.Errorf("first = %q, want %q", got[0].Version, tc.wantFirst)
			}
			if got[len(got)-1].Version != tc.wantLast {
				t.Errorf("last = %q, want %q", got[len(got)-1].Version, tc.wantLast)
			}
		})
	}
}

func TestSquashMigrations_WritesConsolidatedFile(t *testing.T) {
	dir := t.TempDir()
	writeMigrationFixture(t, dir, "20260101_000000", "initial",
		"-- @description: initial\n-- @up\nDEFINE TABLE user SCHEMAFULL;\n-- @down\nREMOVE TABLE user;\n")
	writeMigrationFixture(t, dir, "20260102_000000", "add_email",
		"-- @description: add email\n-- @up\nDEFINE FIELD email ON TABLE user TYPE string;\n-- @down\nREMOVE FIELD email ON TABLE user;\n")
	writeMigrationFixture(t, dir, "20260103_000000", "add_index",
		"-- @description: add index\n-- @up\nDEFINE INDEX idx_user_email ON TABLE user COLUMNS email UNIQUE;\n-- @down\nREMOVE INDEX idx_user_email ON TABLE user;\n")

	// Use a fixed clock so the test can assert on the resulting filename.
	orig := now
	now = func() time.Time { return time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { now = orig })

	result, err := SquashMigrations(context.Background(), dir, "", "")
	if err != nil {
		t.Fatalf("SquashMigrations: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}
	if result.OriginalCount != 3 {
		t.Errorf("OriginalCount = %d, want 3", result.OriginalCount)
	}
	if result.Version != "20260601_120000" {
		t.Errorf("Version = %q, want 20260601_120000", result.Version)
	}
	if result.Checksum == "" {
		t.Error("Checksum empty")
	}
	if len(result.OriginalMigrations) != 3 {
		t.Errorf("OriginalMigrations = %v", result.OriginalMigrations)
	}
	if _, err := os.Stat(result.SquashedPath); err != nil {
		t.Errorf("squashed file missing: %v", err)
	}

	// Verify the written file contains the squashed-from marker and merged
	// up/down sections in the correct order.
	body, err := os.ReadFile(result.SquashedPath)
	if err != nil {
		t.Fatalf("read squashed: %v", err)
	}
	text := string(body)
	if !strings.Contains(text, "-- @squashed-from: 20260101_000000..20260103_000000") {
		t.Errorf("squashed-from marker missing in:\n%s", text)
	}
	if !strings.Contains(text, "DEFINE TABLE user SCHEMAFULL;") {
		t.Errorf("missing up statement in:\n%s", text)
	}
	// Down statements should appear in reverse migration order.
	downIdx := strings.Index(text, "-- @down")
	if downIdx == -1 {
		t.Fatalf("no -- @down section:\n%s", text)
	}
	downBody := text[downIdx:]
	removeIndex := strings.Index(downBody, "REMOVE INDEX")
	removeField := strings.Index(downBody, "REMOVE FIELD")
	removeTable := strings.Index(downBody, "REMOVE TABLE")
	if removeIndex == -1 || removeField == -1 || removeTable == -1 {
		t.Fatalf("down section missing statements:\n%s", downBody)
	}
	if !(removeIndex < removeField && removeField < removeTable) {
		t.Errorf("down order wrong: idx=%d field=%d table=%d\n%s",
			removeIndex, removeField, removeTable, downBody)
	}
}

func TestSquashMigrations_DryRunDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	writeMigrationFixture(t, dir, "20260101_000000", "a",
		"-- @up\nDEFINE TABLE a SCHEMAFULL;\n-- @down\nREMOVE TABLE a;\n")
	writeMigrationFixture(t, dir, "20260102_000000", "b",
		"-- @up\nDEFINE TABLE b SCHEMAFULL;\n-- @down\nREMOVE TABLE b;\n")

	result, err := SquashMigrationsWithOptions(
		context.Background(), dir, "", "",
		SquashOptions{DryRun: true},
	)
	if err != nil {
		t.Fatalf("SquashMigrationsWithOptions: %v", err)
	}
	if result.Checksum != "" {
		t.Errorf("Checksum should be empty on dry run, got %q", result.Checksum)
	}
	if _, err := os.Stat(result.SquashedPath); !os.IsNotExist(err) {
		t.Errorf("dry-run wrote a file at %q (err=%v)", result.SquashedPath, err)
	}
}

func TestSquashMigrations_RangeFilters(t *testing.T) {
	dir := t.TempDir()
	writeMigrationFixture(t, dir, "20260101_000000", "a",
		"-- @up\nDEFINE TABLE a SCHEMAFULL;\n-- @down\nREMOVE TABLE a;\n")
	writeMigrationFixture(t, dir, "20260102_000000", "b",
		"-- @up\nDEFINE TABLE b SCHEMAFULL;\n-- @down\nREMOVE TABLE b;\n")
	writeMigrationFixture(t, dir, "20260103_000000", "c",
		"-- @up\nDEFINE TABLE c SCHEMAFULL;\n-- @down\nREMOVE TABLE c;\n")

	result, err := SquashMigrations(context.Background(), dir,
		"20260101_000000", "20260102_000000")
	if err != nil {
		t.Fatalf("SquashMigrations: %v", err)
	}
	if result.OriginalCount != 2 {
		t.Errorf("OriginalCount = %d, want 2", result.OriginalCount)
	}
}

func TestSquashMigrations_ErrorOnEmptyDir(t *testing.T) {
	dir := t.TempDir()
	_, err := SquashMigrations(context.Background(), dir, "", "")
	if err == nil {
		t.Fatal("expected error on empty dir")
	}
	if !stdErrors.Is(err, SquashError) {
		t.Errorf("err not SquashError: %v", err)
	}
	if !stdErrors.Is(err, surqlerrors.ErrMigrationSquash) {
		t.Errorf("err should wrap ErrMigrationSquash: %v", err)
	}
}

func TestSquashMigrations_ErrorOnSingleMigration(t *testing.T) {
	dir := t.TempDir()
	writeMigrationFixture(t, dir, "20260101_000000", "a",
		"-- @up\nDEFINE TABLE a SCHEMAFULL;\n-- @down\nREMOVE TABLE a;\n")
	_, err := SquashMigrations(context.Background(), dir, "", "")
	if err == nil {
		t.Fatal("expected error when fewer than 2 migrations")
	}
	if !strings.Contains(err.Error(), "at least 2") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSquashMigrations_AbortsOnHighSeverity(t *testing.T) {
	dir := t.TempDir()
	writeMigrationFixture(t, dir, "20260101_000000", "a",
		"-- @up\nDEFINE TABLE a SCHEMAFULL;\n-- @down\nREMOVE TABLE a;\n")
	// DELETE is flagged as high severity.
	writeMigrationFixture(t, dir, "20260102_000000", "b",
		"-- @up\nDELETE a WHERE id = 1;\n-- @down\n")

	_, err := SquashMigrations(context.Background(), dir, "", "")
	if err == nil {
		t.Fatal("expected high-severity error")
	}
	if !strings.Contains(err.Error(), "high severity") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSquashMigrations_ContextCancelled(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := SquashMigrations(ctx, dir, "", "")
	if err == nil {
		t.Fatal("expected cancellation error")
	}
}

func TestSquashMigrations_DisableOptimisation(t *testing.T) {
	dir := t.TempDir()
	writeMigrationFixture(t, dir, "20260101_000000", "a",
		"-- @up\nDEFINE FIELD temp ON TABLE user TYPE string;\n-- @down\nREMOVE FIELD temp ON TABLE user;\n")
	writeMigrationFixture(t, dir, "20260102_000000", "b",
		"-- @up\nREMOVE FIELD temp ON TABLE user;\n-- @down\nDEFINE FIELD temp ON TABLE user TYPE string;\n")

	disable := false
	result, err := SquashMigrationsWithOptions(context.Background(), dir, "", "",
		SquashOptions{Optimize: &disable})
	if err != nil {
		t.Fatalf("SquashMigrationsWithOptions: %v", err)
	}
	if result.OptimizationsApplied != 0 {
		t.Errorf("OptimizationsApplied = %d, want 0 when disabled", result.OptimizationsApplied)
	}
}

func TestTruncateForWarning(t *testing.T) {
	short := "SHORT"
	if got := truncateForWarning(short); got != short {
		t.Errorf("truncate short: got %q", got)
	}
	long := strings.Repeat("x", 80)
	got := truncateForWarning(long)
	if !strings.HasSuffix(got, "...") || len(got) != 53 {
		t.Errorf("truncate long: %q (len=%d)", got, len(got))
	}
}

func TestSquashError_IsMigrationSquash(t *testing.T) {
	if !stdErrors.Is(SquashError, surqlerrors.ErrMigrationSquash) {
		t.Error("SquashError should equal ErrMigrationSquash")
	}
}

func TestBuildSquashedPath(t *testing.T) {
	got := buildSquashedPath("/tmp/migrations", "20260101_000000", "squashed_a_to_b")
	want := filepath.Join("/tmp/migrations", "20260101_000000_squashed_a_to_b.surql")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
