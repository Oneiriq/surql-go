package migration

import (
	stdErrors "errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	surqlerrors "github.com/albedosehen/surql-go/pkg/surql/errors"
	"github.com/albedosehen/surql-go/pkg/surql/schema"
)

// withFrozenClock overrides the generator's clock for the duration of a test.
// It returns a cleanup function the caller defers.
func withFrozenClock(t *testing.T, ts time.Time) func() {
	t.Helper()
	prev := now
	now = ts.UTC
	return func() { now = prev }
}

// -----------------------------------------------------------------------------
// Timestamp / filename helpers
// -----------------------------------------------------------------------------

func TestTimestampLayoutFormat(t *testing.T) {
	ts := time.Date(2026, 4, 18, 7, 9, 3, 0, time.UTC)
	got := ts.Format(timestampLayout)
	if got != "20260418_070903" {
		t.Fatalf("timestamp = %q, want %q", got, "20260418_070903")
	}
	if !regexp.MustCompile(`^\d{8}_\d{6}$`).MatchString(got) {
		t.Fatalf("timestamp does not match YYYYMMDD_HHMMSS: %q", got)
	}
}

func TestSanitizeSlug(t *testing.T) {
	cases := map[string]string{
		"Create user table":       "create_user_table",
		"  Create   user  table ": "create_user_table",
		"create-user-table":       "create_user_table",
		"MiXeD Case!@#":           "mixed_case",
		"users/orders":            "usersorders",
		"":                        "",
		"__already__snake__":      "already_snake",
		"drop user":               "drop_user",
		"123 numeric start":       "123_numeric_start",
	}
	for in, want := range cases {
		got := sanitizeSlug(in)
		if got != want {
			t.Errorf("sanitizeSlug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBuildFilename(t *testing.T) {
	got := BuildFilename("20260418_070903", "Create User Table")
	if got != "20260418_070903_create_user_table.surql" {
		t.Fatalf("BuildFilename = %q", got)
	}
	if !ValidateMigrationName(got) {
		t.Fatalf("generated filename %q is not a valid migration name", got)
	}
}

func TestBuildFilename_FallsBackToMigration(t *testing.T) {
	// All characters filtered out -> slug empty -> falls back to "migration".
	got := BuildFilename("20260418_070903", "***")
	if got != "20260418_070903_migration.surql" {
		t.Fatalf("BuildFilename fallback = %q", got)
	}
	if !ValidateMigrationName(got) {
		t.Fatalf("fallback filename %q invalid", got)
	}
}

// -----------------------------------------------------------------------------
// splitScriptStatements
// -----------------------------------------------------------------------------

func TestSplitScriptStatements_Basic(t *testing.T) {
	script := "DEFINE TABLE user SCHEMAFULL;\nDEFINE FIELD email ON user TYPE string;"
	got := splitScriptStatements(script)
	want := []string{
		"DEFINE TABLE user SCHEMAFULL;",
		"DEFINE FIELD email ON user TYPE string;",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestSplitScriptStatements_DropsCommentsAndBlanks(t *testing.T) {
	script := "-- comment\nDEFINE TABLE u SCHEMAFULL;\n\n-- another\nDEFINE FIELD x ON u TYPE int;\n"
	got := splitScriptStatements(script)
	want := []string{"DEFINE TABLE u SCHEMAFULL;", "DEFINE FIELD x ON u TYPE int;"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestSplitScriptStatements_Empty(t *testing.T) {
	if got := splitScriptStatements(""); got != nil {
		t.Fatalf("empty -> %#v", got)
	}
	if got := splitScriptStatements("   \n-- only a comment\n"); got != nil {
		t.Fatalf("only comments -> %#v", got)
	}
}

// -----------------------------------------------------------------------------
// renderMigrationContent
// -----------------------------------------------------------------------------

func TestRenderMigrationContent_FullBody(t *testing.T) {
	body := renderMigrationContent(
		"create user",
		[]string{"20260101_000000_init"},
		[]string{"DEFINE TABLE user SCHEMAFULL;"},
		[]string{"REMOVE TABLE user;"},
	)
	if !strings.Contains(body, "-- @description: create user\n") {
		t.Errorf("description header missing:\n%s", body)
	}
	if !strings.Contains(body, "-- @depends_on: 20260101_000000_init\n") {
		t.Errorf("depends_on header missing:\n%s", body)
	}
	if !strings.Contains(body, "-- @up\nDEFINE TABLE user SCHEMAFULL;\n-- @down\n") {
		t.Errorf("up section wrong:\n%s", body)
	}
	if !strings.HasSuffix(body, "REMOVE TABLE user;\n") {
		t.Errorf("down section wrong:\n%s", body)
	}
}

func TestRenderMigrationContent_OmitsEmptyHeaders(t *testing.T) {
	body := renderMigrationContent("", nil, nil, nil)
	if strings.Contains(body, "-- @description:") {
		t.Errorf("unexpected description header: %q", body)
	}
	if strings.Contains(body, "-- @depends_on:") {
		t.Errorf("unexpected depends_on header: %q", body)
	}
	if !strings.Contains(body, "-- @up\n") || !strings.Contains(body, "-- @down\n") {
		t.Errorf("missing up/down markers: %q", body)
	}
}

// -----------------------------------------------------------------------------
// GenerateMigration
// -----------------------------------------------------------------------------

func TestGenerateMigration_WritesAndRoundTrips(t *testing.T) {
	defer withFrozenClock(t, time.Date(2026, 4, 18, 7, 9, 3, 0, time.UTC))()
	dir := t.TempDir()

	up := []string{"DEFINE TABLE user SCHEMAFULL;", "DEFINE FIELD email ON user TYPE string;"}
	down := []string{"REMOVE TABLE user;"}

	m, mgErr := GenerateMigration("Create user", up, down, dir)
	if mgErr != nil {
		t.Fatalf("generator error: %v", mgErr)
	}

	wantName := "20260418_070903_create_user.surql"
	if filepath.Base(m.Path) != wantName {
		t.Fatalf("filename = %q, want %q", filepath.Base(m.Path), wantName)
	}
	if m.Version != "20260418_070903" {
		t.Errorf("version = %q", m.Version)
	}
	if !reflect.DeepEqual(m.UpStatements, up) {
		t.Errorf("up = %#v", m.UpStatements)
	}
	if !reflect.DeepEqual(m.DownStatements, down) {
		t.Errorf("down = %#v", m.DownStatements)
	}

	// Re-load from disk to verify the round-trip.
	reloaded, err := LoadMigration(m.Path)
	if err != nil {
		t.Fatalf("LoadMigration: %v", err)
	}
	if !reflect.DeepEqual(reloaded.UpStatements, up) {
		t.Errorf("reload up = %#v", reloaded.UpStatements)
	}
	if !reflect.DeepEqual(reloaded.DownStatements, down) {
		t.Errorf("reload down = %#v", reloaded.DownStatements)
	}
	if reloaded.Version != m.Version {
		t.Errorf("reload version = %q", reloaded.Version)
	}
	if reloaded.Checksum != m.Checksum {
		t.Errorf("checksum drifted: %q vs %q", reloaded.Checksum, m.Checksum)
	}
}

func TestGenerateMigration_CreatesDirectory(t *testing.T) {
	defer withFrozenClock(t, time.Date(2026, 4, 18, 7, 9, 3, 0, time.UTC))()
	root := t.TempDir()
	dir := filepath.Join(root, "does", "not", "exist")

	m, err := GenerateMigration("init", []string{"DEFINE TABLE t SCHEMAFULL;"}, nil, dir)
	if err != nil {
		t.Fatalf("generator error: %v", err)
	}

	if _, err := os.Stat(m.Path); err != nil {
		t.Fatalf("expected file, got %v", err)
	}
}

func TestGenerateMigration_RejectsEmptyName(t *testing.T) {
	_, err := GenerateMigration("   ", []string{"X;"}, nil, t.TempDir())
	if err == nil || !stdErrors.Is(err, surqlerrors.ErrMigrationGeneration) {
		t.Fatalf("expected ErrMigrationGeneration, got %v", err)
	}
}

func TestGenerateMigration_RejectsEmptyStatements(t *testing.T) {
	_, err := GenerateMigration("name", nil, nil, t.TempDir())
	if err == nil || !stdErrors.Is(err, surqlerrors.ErrMigrationGeneration) {
		t.Fatalf("expected ErrMigrationGeneration, got %v", err)
	}
}

func TestGenerateMigration_RejectsEmptyDir(t *testing.T) {
	_, err := GenerateMigration("x", []string{"X;"}, nil, "")
	if err == nil || !stdErrors.Is(err, surqlerrors.ErrMigrationGeneration) {
		t.Fatalf("expected ErrMigrationGeneration, got %v", err)
	}
}

func TestGenerateMigration_DirIsFile(t *testing.T) {
	root := t.TempDir()
	filePath := filepath.Join(root, "not-a-dir")
	if err := os.WriteFile(filePath, []byte("x"), 0o600); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	_, err := GenerateMigration("x", []string{"X;"}, nil, filePath)
	if err == nil || !stdErrors.Is(err, surqlerrors.ErrMigrationGeneration) {
		t.Fatalf("expected ErrMigrationGeneration, got %v", err)
	}
}

func TestGenerateMigration_PathIsAbsolute(t *testing.T) {
	defer withFrozenClock(t, time.Date(2026, 4, 18, 7, 9, 3, 0, time.UTC))()
	dir := t.TempDir()
	m, mgErr := GenerateMigration("abs", []string{"X;"}, nil, dir)
	if mgErr != nil {
		t.Fatalf("generator error: %v", mgErr)
	}
	if !filepath.IsAbs(m.Path) {
		t.Fatalf("expected absolute path, got %q", m.Path)
	}
}

// -----------------------------------------------------------------------------
// GenerateInitialMigration
// -----------------------------------------------------------------------------

func TestGenerateInitialMigration_MultiTableRegistry(t *testing.T) {
	defer withFrozenClock(t, time.Date(2026, 4, 18, 8, 0, 0, 0, time.UTC))()
	dir := t.TempDir()

	reg := schema.NewSchemaRegistry()
	if err := reg.RegisterTable(schema.NewTable("user",
		schema.WithMode(schema.TableModeSchemafull),
		schema.WithFields(schema.StringField("email")),
	)); err != nil {
		t.Fatalf("register user: %v", err)
	}
	if err := reg.RegisterTable(schema.NewTable("post",
		schema.WithMode(schema.TableModeSchemafull),
		schema.WithFields(schema.StringField("title")),
	)); err != nil {
		t.Fatalf("register post: %v", err)
	}

	m, mgErr := GenerateInitialMigration(reg, dir)
	if mgErr != nil {
		t.Fatalf("generator error: %v", mgErr)
	}

	if filepath.Base(m.Path) != "20260418_080000_initial_schema.surql" {
		t.Errorf("filename = %q", filepath.Base(m.Path))
	}
	if len(m.UpStatements) == 0 {
		t.Fatalf("expected up statements, got none")
	}
	if len(m.DownStatements) != 0 {
		t.Errorf("expected no down statements, got %d", len(m.DownStatements))
	}

	// All emitted statements must be terminated and contain user + post.
	joined := strings.Join(m.UpStatements, "\n")
	if !strings.Contains(joined, "DEFINE TABLE user") {
		t.Errorf("missing DEFINE TABLE user in:\n%s", joined)
	}
	if !strings.Contains(joined, "DEFINE TABLE post") {
		t.Errorf("missing DEFINE TABLE post in:\n%s", joined)
	}

	// Round-trip through LoadMigration.
	reloaded, err := LoadMigration(m.Path)
	if err != nil {
		t.Fatalf("LoadMigration: %v", err)
	}
	if !reflect.DeepEqual(reloaded.UpStatements, m.UpStatements) {
		t.Errorf("round-trip up statements changed")
	}
}

func TestGenerateInitialMigration_NilRegistry(t *testing.T) {
	_, err := GenerateInitialMigration(nil, t.TempDir())
	if err == nil || !stdErrors.Is(err, surqlerrors.ErrMigrationGeneration) {
		t.Fatalf("expected ErrMigrationGeneration, got %v", err)
	}
}

func TestGenerateInitialMigration_EmptyRegistry(t *testing.T) {
	reg := schema.NewSchemaRegistry()
	_, err := GenerateInitialMigration(reg, t.TempDir())
	if err == nil || !stdErrors.Is(err, surqlerrors.ErrMigrationGeneration) {
		t.Fatalf("expected ErrMigrationGeneration, got %v", err)
	}
}

// -----------------------------------------------------------------------------
// CreateBlankMigration
// -----------------------------------------------------------------------------

func TestCreateBlankMigration_Roundtrip(t *testing.T) {
	defer withFrozenClock(t, time.Date(2026, 4, 18, 9, 0, 0, 0, time.UTC))()
	dir := t.TempDir()

	m, mgErr := CreateBlankMigration("custom_data", "Custom data migration", dir)
	if mgErr != nil {
		t.Fatalf("generator error: %v", mgErr)
	}

	if filepath.Base(m.Path) != "20260418_090000_custom_data.surql" {
		t.Errorf("filename = %q", filepath.Base(m.Path))
	}
	if m.Description != "Custom data migration" {
		t.Errorf("description = %q", m.Description)
	}
	if len(m.UpStatements) != 0 || len(m.DownStatements) != 0 {
		t.Errorf("expected empty statements, got up=%v down=%v",
			m.UpStatements, m.DownStatements)
	}

	reloaded, err := LoadMigration(m.Path)
	if err != nil {
		t.Fatalf("LoadMigration: %v", err)
	}
	if reloaded.Description != "Custom data migration" {
		t.Errorf("reload description = %q", reloaded.Description)
	}
}

func TestCreateBlankMigration_DescriptionDefaultsToName(t *testing.T) {
	defer withFrozenClock(t, time.Date(2026, 4, 18, 9, 0, 0, 0, time.UTC))()
	dir := t.TempDir()

	m, mgErr := CreateBlankMigration("empty_desc", "", dir)
	if mgErr != nil {
		t.Fatalf("generator error: %v", mgErr)
	}
	if m.Description != "empty_desc" {
		t.Errorf("description fallback failed: %q", m.Description)
	}
}

func TestCreateBlankMigration_RejectsEmptyName(t *testing.T) {
	_, err := CreateBlankMigration("", "x", t.TempDir())
	if err == nil || !stdErrors.Is(err, surqlerrors.ErrMigrationGeneration) {
		t.Fatalf("expected ErrMigrationGeneration, got %v", err)
	}
}

// -----------------------------------------------------------------------------
// GenerateMigrationFromDiffs
// -----------------------------------------------------------------------------

func TestGenerateMigrationFromDiffs_OrdersDownInReverse(t *testing.T) {
	defer withFrozenClock(t, time.Date(2026, 4, 18, 10, 0, 0, 0, time.UTC))()
	dir := t.TempDir()

	diffs := []SchemaDiff{
		{
			Operation:   DiffOperationAddTable,
			Table:       "user",
			ForwardSQL:  "DEFINE TABLE user SCHEMAFULL;",
			BackwardSQL: "REMOVE TABLE user;",
		},
		{
			Operation:   DiffOperationAddField,
			Table:       "user",
			Field:       "email",
			ForwardSQL:  "DEFINE FIELD email ON TABLE user TYPE string;",
			BackwardSQL: "REMOVE FIELD email ON TABLE user;",
		},
	}

	m, mgErr := GenerateMigrationFromDiffs("add user", diffs, dir)
	if mgErr != nil {
		t.Fatalf("generator error: %v", mgErr)
	}

	wantUp := []string{
		"DEFINE TABLE user SCHEMAFULL;",
		"DEFINE FIELD email ON TABLE user TYPE string;",
	}
	wantDown := []string{
		"REMOVE FIELD email ON TABLE user;",
		"REMOVE TABLE user;",
	}
	if !reflect.DeepEqual(m.UpStatements, wantUp) {
		t.Errorf("up = %#v want %#v", m.UpStatements, wantUp)
	}
	if !reflect.DeepEqual(m.DownStatements, wantDown) {
		t.Errorf("down = %#v want %#v", m.DownStatements, wantDown)
	}
}

func TestGenerateMigrationFromDiffs_SkipsEmptyStatements(t *testing.T) {
	defer withFrozenClock(t, time.Date(2026, 4, 18, 10, 0, 0, 0, time.UTC))()
	dir := t.TempDir()

	diffs := []SchemaDiff{
		{ForwardSQL: "DEFINE TABLE t SCHEMAFULL;", BackwardSQL: ""},
	}
	m, mgErr := GenerateMigrationFromDiffs("x", diffs, dir)
	if mgErr != nil {
		t.Fatalf("generator error: %v", mgErr)
	}
	if len(m.DownStatements) != 0 {
		t.Errorf("expected empty down, got %#v", m.DownStatements)
	}
}

func TestGenerateMigrationFromDiffs_EmptySlice(t *testing.T) {
	_, err := GenerateMigrationFromDiffs("x", nil, t.TempDir())
	if err == nil || !stdErrors.Is(err, surqlerrors.ErrMigrationGeneration) {
		t.Fatalf("expected ErrMigrationGeneration, got %v", err)
	}
}

func TestGenerateMigrationFromDiffs_RejectsEmptyName(t *testing.T) {
	_, err := GenerateMigrationFromDiffs("", []SchemaDiff{{ForwardSQL: "X;"}}, t.TempDir())
	if err == nil || !stdErrors.Is(err, surqlerrors.ErrMigrationGeneration) {
		t.Fatalf("expected ErrMigrationGeneration, got %v", err)
	}
}

func TestGenerateMigrationFromDiffs_AllEmptySQL(t *testing.T) {
	diffs := []SchemaDiff{{Operation: DiffOperationAddTable, Table: "t"}}
	_, err := GenerateMigrationFromDiffs("x", diffs, t.TempDir())
	if err == nil || !stdErrors.Is(err, surqlerrors.ErrMigrationGeneration) {
		t.Fatalf("expected ErrMigrationGeneration, got %v", err)
	}
}

// -----------------------------------------------------------------------------
// Integration: diff engine -> generator -> discovery
// -----------------------------------------------------------------------------

func TestGeneratorDiscoveryIntegration(t *testing.T) {
	defer withFrozenClock(t, time.Date(2026, 4, 18, 11, 0, 0, 0, time.UTC))()
	dir := t.TempDir()

	newTable := schema.NewTable("user",
		schema.WithMode(schema.TableModeSchemafull),
		schema.WithFields(schema.StringField("email")),
	)
	diffs, err := DiffTables(nil, &newTable)
	if err != nil {
		t.Fatalf("DiffTables: %v", err)
	}

	m, mgErr := GenerateMigrationFromDiffs("add user table", diffs, dir)
	if mgErr != nil {
		t.Fatalf("generator error: %v", mgErr)
	}

	discovered, err := DiscoverMigrations(dir)
	if err != nil {
		t.Fatalf("DiscoverMigrations: %v", err)
	}
	if len(discovered) != 1 {
		t.Fatalf("discovered %d migrations", len(discovered))
	}
	if discovered[0].Version != m.Version {
		t.Errorf("discovered version = %q", discovered[0].Version)
	}
	if !reflect.DeepEqual(discovered[0].UpStatements, m.UpStatements) {
		t.Errorf("discovered up differs")
	}
}

// -----------------------------------------------------------------------------
// Atomic write semantics
// -----------------------------------------------------------------------------

func TestAtomicWriteFile_LeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "20260418_070903_atomic.surql")
	if err := atomicWriteFile(target, []byte("hello")); err != nil {
		t.Fatalf("atomicWriteFile: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("expected exactly one file after atomic write, got %v", names)
	}
	if entries[0].Name() != filepath.Base(target) {
		t.Fatalf("unexpected file %q", entries[0].Name())
	}
}

func TestAtomicWriteFile_Overwrites(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "20260418_070903_overwrite.surql")
	if err := atomicWriteFile(target, []byte("first")); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if err := atomicWriteFile(target, []byte("second")); err != nil {
		t.Fatalf("second write: %v", err)
	}
	body, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(body) != "second" {
		t.Fatalf("file body = %q", string(body))
	}
}

func TestAtomicWriteFile_FailsOnBadDir(t *testing.T) {
	err := atomicWriteFile("/no/such/dir/file.surql", []byte("x"))
	if err == nil {
		t.Fatalf("expected error for bad directory")
	}
}

func TestGenerateMigration_FilenameIsValidForDiscovery(t *testing.T) {
	defer withFrozenClock(t, time.Date(2026, 4, 18, 11, 0, 0, 0, time.UTC))()
	dir := t.TempDir()

	// Many edge-case descriptions that should all normalize to valid filenames.
	cases := []string{
		"Add users",
		"  complex !@# Title ",
		"a-b-c",
		"snake_case_already",
		"UPPERCASE",
	}
	for _, desc := range cases {
		// Different descriptions still share the same timestamp under the
		// frozen clock; clean the directory between runs to avoid collisions.
		if err := os.RemoveAll(dir); err != nil {
			t.Fatalf("clean: %v", err)
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		m, err := GenerateMigration(desc, []string{"X;"}, nil, dir)
		if err != nil {
			t.Fatalf("GenerateMigration(%q): %v", desc, err)
		}
		if !ValidateMigrationName(filepath.Base(m.Path)) {
			t.Errorf("filename %q invalid for discovery", filepath.Base(m.Path))
		}
	}
}

func TestGenerateInitialMigration_Roundtrip(t *testing.T) {
	defer withFrozenClock(t, time.Date(2026, 4, 18, 11, 5, 0, 0, time.UTC))()
	dir := t.TempDir()

	reg := schema.NewSchemaRegistry()
	if err := reg.RegisterTable(schema.NewTable("product",
		schema.WithMode(schema.TableModeSchemafull),
		schema.WithFields(schema.StringField("sku"), schema.IntField("qty")),
	)); err != nil {
		t.Fatalf("register: %v", err)
	}

	m, mgErr := GenerateInitialMigration(reg, dir)
	if mgErr != nil {
		t.Fatalf("generator error: %v", mgErr)
	}
	reloaded, err := LoadMigration(m.Path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !reflect.DeepEqual(reloaded.UpStatements, m.UpStatements) {
		t.Errorf("round-trip up mismatch")
	}
	if reloaded.Description != m.Description {
		t.Errorf("description mismatch: %q vs %q", reloaded.Description, m.Description)
	}
}

func TestGenerateMigration_UpOnlyAndDownOnly(t *testing.T) {
	defer withFrozenClock(t, time.Date(2026, 4, 18, 11, 6, 0, 0, time.UTC))()

	t.Run("up only", func(t *testing.T) {
		dir := t.TempDir()
		m, mgErr := GenerateMigration("up only", []string{"X;"}, nil, dir)
		if mgErr != nil {
			t.Fatalf("generator error: %v", mgErr)
		}
		if len(m.DownStatements) != 0 {
			t.Errorf("down non-empty: %#v", m.DownStatements)
		}
	})

	t.Run("down only", func(t *testing.T) {
		dir := t.TempDir()
		m, mgErr := GenerateMigration("down only", nil, []string{"Y;"}, dir)
		if mgErr != nil {
			t.Fatalf("generator error: %v", mgErr)
		}
		if len(m.UpStatements) != 0 {
			t.Errorf("up non-empty: %#v", m.UpStatements)
		}
	})
}

func TestGenerator_ChecksumMatchesFileContent(t *testing.T) {
	defer withFrozenClock(t, time.Date(2026, 4, 18, 11, 7, 0, 0, time.UTC))()
	dir := t.TempDir()

	m, mgErr := GenerateMigration("checksum", []string{"X;"}, nil, dir)
	if mgErr != nil {
		t.Fatalf("generator error: %v", mgErr)
	}
	raw, err := os.ReadFile(m.Path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if got := sha256Hex(raw); got != m.Checksum {
		t.Fatalf("checksum %q != computed %q", m.Checksum, got)
	}
}
