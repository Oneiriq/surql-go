package migration

import (
	stdErrors "errors"
	"os"
	"path/filepath"
	"testing"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

// writeFile is a small test helper that creates a file with the given body.
func writeFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
	return p
}

const sampleMigration = `-- @description: create user table
-- @depends_on: 20260101_000000_init
-- @up
DEFINE TABLE user SCHEMAFULL;
DEFINE FIELD email ON user TYPE string;
-- @down
REMOVE TABLE user;
`

const sampleNoHeaders = `-- @up
DEFINE TABLE foo SCHEMAFULL;
-- @down
REMOVE TABLE foo;
`

func TestValidateMigrationName_Valid(t *testing.T) {
	cases := []string{
		"20260102_120000_create_user.surql",
		"20260101_000000_init.surql",
		"20991231_235959_create_user_table.surql",
		"20260102_120000_name-with-dash.surql",
	}
	for _, name := range cases {
		if !ValidateMigrationName(name) {
			t.Errorf("expected %q to be valid", name)
		}
	}
}

func TestValidateMigrationName_Invalid(t *testing.T) {
	cases := []string{
		"",
		"invalid.surql",
		"20260102_120000.surql",
		"20260102_120000_.surql",
		"2026010_120000_x.surql",
		"20260102_12000_x.surql",
		"20260102-120000_x.surql",
		"20260102_120000_create_user.py",
		"20260102_120000_create_user",
		"_20260102_120000_create_user.surql",
		"20260102_120000_bad!.surql",
	}
	for _, name := range cases {
		if ValidateMigrationName(name) {
			t.Errorf("expected %q to be invalid", name)
		}
	}
}

func TestGetVersionFromFilename(t *testing.T) {
	v, ok := GetVersionFromFilename("20260102_120000_create_user.surql")
	if !ok || v != "20260102_120000" {
		t.Errorf("version = %q, ok = %v", v, ok)
	}
	if _, ok := GetVersionFromFilename("invalid.surql"); ok {
		t.Errorf("expected ok = false for invalid name")
	}
}

func TestGetDescriptionFromFilename(t *testing.T) {
	d, ok := GetDescriptionFromFilename("20260102_120000_create_user_table.surql")
	if !ok || d != "create_user_table" {
		t.Errorf("description = %q, ok = %v", d, ok)
	}
	if _, ok := GetDescriptionFromFilename("invalid.surql"); ok {
		t.Errorf("expected ok = false for invalid name")
	}
}

func TestLoadMigration_Happy(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "20260102_120000_create_user_table.surql", sampleMigration)

	m, err := LoadMigration(p)
	if err != nil {
		t.Fatalf("LoadMigration: %v", err)
	}
	if m.Version != "20260102_120000" {
		t.Errorf("version = %q", m.Version)
	}
	if m.Description != "create user table" {
		t.Errorf("description = %q", m.Description)
	}
	if m.Path != p {
		t.Errorf("path = %q", m.Path)
	}
	if len(m.UpStatements) != 2 {
		t.Fatalf("up statements = %d, want 2", len(m.UpStatements))
	}
	if m.UpStatements[0] != "DEFINE TABLE user SCHEMAFULL;" {
		t.Errorf("up[0] = %q", m.UpStatements[0])
	}
	if m.UpStatements[1] != "DEFINE FIELD email ON user TYPE string;" {
		t.Errorf("up[1] = %q", m.UpStatements[1])
	}
	if len(m.DownStatements) != 1 || m.DownStatements[0] != "REMOVE TABLE user;" {
		t.Errorf("down = %+v", m.DownStatements)
	}
	if len(m.DependsOn) != 1 || m.DependsOn[0] != "20260101_000000_init" {
		t.Errorf("depends_on = %+v", m.DependsOn)
	}
	if len(m.Checksum) != 64 {
		t.Errorf("checksum len = %d", len(m.Checksum))
	}
}

func TestLoadMigration_DescriptionFallback(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "20260102_120000_create_user_table.surql", sampleNoHeaders)

	m, err := LoadMigration(p)
	if err != nil {
		t.Fatalf("LoadMigration: %v", err)
	}
	// No explicit description header -> derived from filename, humanized.
	// Note that filename description is "create_user_table".
	if m.Description != "create user table" {
		t.Errorf("description fallback = %q", m.Description)
	}
	if len(m.DependsOn) != 0 {
		t.Errorf("depends_on = %+v", m.DependsOn)
	}
}

func TestLoadMigration_FileNotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadMigration(filepath.Join(dir, "missing.surql"))
	if err == nil {
		t.Fatalf("expected error")
	}
	if !stdErrors.Is(err, surqlerrors.ErrMigrationLoad) {
		t.Errorf("error kind = %v", err)
	}
}

func TestLoadMigration_DirectoryRejected(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadMigration(dir)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !stdErrors.Is(err, surqlerrors.ErrMigrationLoad) {
		t.Errorf("error kind = %v", err)
	}
}

func TestLoadMigration_InvalidFilename(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "bad_name.surql", sampleMigration)

	_, err := LoadMigration(p)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !stdErrors.Is(err, surqlerrors.ErrMigrationLoad) {
		t.Errorf("error kind = %v", err)
	}
}

func TestLoadMigration_MissingUpMarker(t *testing.T) {
	body := `-- @description: x
DEFINE TABLE foo;
-- @down
REMOVE TABLE foo;
`
	dir := t.TempDir()
	p := writeFile(t, dir, "20260102_120000_x.surql", body)

	_, err := LoadMigration(p)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !stdErrors.Is(err, surqlerrors.ErrMigrationLoad) {
		t.Errorf("error kind = %v", err)
	}
}

func TestLoadMigration_MissingDownMarker(t *testing.T) {
	body := `-- @up
DEFINE TABLE foo;
`
	dir := t.TempDir()
	p := writeFile(t, dir, "20260102_120000_x.surql", body)

	_, err := LoadMigration(p)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !stdErrors.Is(err, surqlerrors.ErrMigrationLoad) {
		t.Errorf("error kind = %v", err)
	}
}

func TestLoadMigration_DownBeforeUp(t *testing.T) {
	body := `-- @down
REMOVE TABLE foo;
-- @up
DEFINE TABLE foo;
`
	dir := t.TempDir()
	p := writeFile(t, dir, "20260102_120000_x.surql", body)

	_, err := LoadMigration(p)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !stdErrors.Is(err, surqlerrors.ErrMigrationLoad) {
		t.Errorf("error kind = %v", err)
	}
}

func TestLoadMigration_DuplicateUpMarker(t *testing.T) {
	body := `-- @up
DEFINE TABLE foo;
-- @up
DEFINE TABLE bar;
-- @down
REMOVE TABLE foo;
`
	dir := t.TempDir()
	p := writeFile(t, dir, "20260102_120000_x.surql", body)

	_, err := LoadMigration(p)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !stdErrors.Is(err, surqlerrors.ErrMigrationLoad) {
		t.Errorf("error kind = %v", err)
	}
}

func TestLoadMigration_DuplicateDownMarker(t *testing.T) {
	body := `-- @up
DEFINE TABLE foo;
-- @down
REMOVE TABLE foo;
-- @down
REMOVE TABLE bar;
`
	dir := t.TempDir()
	p := writeFile(t, dir, "20260102_120000_x.surql", body)

	_, err := LoadMigration(p)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !stdErrors.Is(err, surqlerrors.ErrMigrationLoad) {
		t.Errorf("error kind = %v", err)
	}
}

func TestLoadMigration_CRLFEndings(t *testing.T) {
	body := "-- @up\r\nDEFINE TABLE foo;\r\n-- @down\r\nREMOVE TABLE foo;\r\n"
	dir := t.TempDir()
	p := writeFile(t, dir, "20260102_120000_crlf_test.surql", body)

	m, err := LoadMigration(p)
	if err != nil {
		t.Fatalf("LoadMigration: %v", err)
	}
	if len(m.UpStatements) != 1 || m.UpStatements[0] != "DEFINE TABLE foo;" {
		t.Errorf("up = %+v", m.UpStatements)
	}
	if len(m.DownStatements) != 1 || m.DownStatements[0] != "REMOVE TABLE foo;" {
		t.Errorf("down = %+v", m.DownStatements)
	}
}

func TestLoadMigration_MultiStatementLine(t *testing.T) {
	body := `-- @up
DEFINE TABLE a; DEFINE TABLE b;
DEFINE TABLE c;
-- @down
REMOVE TABLE c; REMOVE TABLE b; REMOVE TABLE a;
`
	dir := t.TempDir()
	p := writeFile(t, dir, "20260102_120000_multi.surql", body)

	m, err := LoadMigration(p)
	if err != nil {
		t.Fatalf("LoadMigration: %v", err)
	}
	if len(m.UpStatements) != 3 {
		t.Errorf("up len = %d: %+v", len(m.UpStatements), m.UpStatements)
	}
	if len(m.DownStatements) != 3 {
		t.Errorf("down len = %d: %+v", len(m.DownStatements), m.DownStatements)
	}
}

func TestLoadMigration_CommentsAndBlankLinesDropped(t *testing.T) {
	body := `-- @up
-- this is a comment

DEFINE TABLE a;

-- another comment
DEFINE TABLE b;
-- @down
REMOVE TABLE b;
REMOVE TABLE a;
`
	dir := t.TempDir()
	p := writeFile(t, dir, "20260102_120000_comments.surql", body)

	m, err := LoadMigration(p)
	if err != nil {
		t.Fatalf("LoadMigration: %v", err)
	}
	if len(m.UpStatements) != 2 {
		t.Errorf("up = %+v", m.UpStatements)
	}
	if m.UpStatements[0] != "DEFINE TABLE a;" || m.UpStatements[1] != "DEFINE TABLE b;" {
		t.Errorf("up contents = %+v", m.UpStatements)
	}
}

func TestLoadMigration_ExplicitDescriptionOverridesFilename(t *testing.T) {
	body := `-- @description: Shiny user table
-- @up
DEFINE TABLE user;
-- @down
REMOVE TABLE user;
`
	dir := t.TempDir()
	p := writeFile(t, dir, "20260102_120000_create_user.surql", body)

	m, err := LoadMigration(p)
	if err != nil {
		t.Fatalf("LoadMigration: %v", err)
	}
	if m.Description != "Shiny user table" {
		t.Errorf("description = %q", m.Description)
	}
}

func TestLoadMigration_DependsOnMultipleValues(t *testing.T) {
	body := `-- @depends_on: 20260101_000000_init,  20260101_010000_seed
-- @up
SELECT 1;
-- @down
SELECT 0;
`
	dir := t.TempDir()
	p := writeFile(t, dir, "20260102_120000_multi_dep.surql", body)

	m, err := LoadMigration(p)
	if err != nil {
		t.Fatalf("LoadMigration: %v", err)
	}
	if len(m.DependsOn) != 2 {
		t.Fatalf("depends_on len = %d: %+v", len(m.DependsOn), m.DependsOn)
	}
	if m.DependsOn[0] != "20260101_000000_init" || m.DependsOn[1] != "20260101_010000_seed" {
		t.Errorf("depends_on = %+v", m.DependsOn)
	}
}

func TestLoadMigration_DependsOnEmptyValue(t *testing.T) {
	body := `-- @depends_on:
-- @up
SELECT 1;
-- @down
SELECT 0;
`
	dir := t.TempDir()
	p := writeFile(t, dir, "20260102_120000_empty_dep.surql", body)

	m, err := LoadMigration(p)
	if err != nil {
		t.Fatalf("LoadMigration: %v", err)
	}
	if len(m.DependsOn) != 0 {
		t.Errorf("depends_on = %+v", m.DependsOn)
	}
}

func TestLoadMigration_ChecksumStableAcrossReads(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "20260102_120000_stable.surql", sampleMigration)

	m1, err := LoadMigration(p)
	if err != nil {
		t.Fatalf("first load: %v", err)
	}
	m2, err := LoadMigration(p)
	if err != nil {
		t.Fatalf("second load: %v", err)
	}
	if m1.Checksum != m2.Checksum {
		t.Errorf("checksum mismatch: %q vs %q", m1.Checksum, m2.Checksum)
	}
}

func TestLoadMigration_ChecksumChangesWithContent(t *testing.T) {
	dir := t.TempDir()
	p1 := writeFile(t, dir, "20260102_120000_one.surql", sampleMigration)
	p2 := writeFile(t, dir, "20260102_120001_two.surql", sampleNoHeaders)

	m1, err := LoadMigration(p1)
	if err != nil {
		t.Fatalf("load p1: %v", err)
	}
	m2, err := LoadMigration(p2)
	if err != nil {
		t.Fatalf("load p2: %v", err)
	}
	if m1.Checksum == m2.Checksum {
		t.Errorf("different content produced identical checksum")
	}
}

func TestDiscoverMigrations_EmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	ms, err := DiscoverMigrations(dir)
	if err != nil {
		t.Fatalf("DiscoverMigrations: %v", err)
	}
	if len(ms) != 0 {
		t.Errorf("expected empty, got %d", len(ms))
	}
}

func TestDiscoverMigrations_NonexistentDirectory(t *testing.T) {
	ms, err := DiscoverMigrations(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("DiscoverMigrations: %v", err)
	}
	if len(ms) != 0 {
		t.Errorf("expected empty, got %d", len(ms))
	}
}

func TestDiscoverMigrations_PathIsFileRejected(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "20260102_120000_x.surql", sampleMigration)

	_, err := DiscoverMigrations(p)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !stdErrors.Is(err, surqlerrors.ErrMigrationDiscovery) {
		t.Errorf("error kind = %v", err)
	}
}

func TestDiscoverMigrations_SortedByVersion(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "20260103_120000_c.surql", sampleNoHeaders)
	writeFile(t, dir, "20260101_120000_a.surql", sampleNoHeaders)
	writeFile(t, dir, "20260102_120000_b.surql", sampleNoHeaders)

	ms, err := DiscoverMigrations(dir)
	if err != nil {
		t.Fatalf("DiscoverMigrations: %v", err)
	}
	if len(ms) != 3 {
		t.Fatalf("len = %d", len(ms))
	}
	wantOrder := []string{"20260101_120000", "20260102_120000", "20260103_120000"}
	for i, want := range wantOrder {
		if ms[i].Version != want {
			t.Errorf("ms[%d].Version = %q, want %q", i, ms[i].Version, want)
		}
	}
}

func TestDiscoverMigrations_SkipsInvalidNames(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "20260102_120000_ok.surql", sampleNoHeaders)
	writeFile(t, dir, "not-a-migration.surql", "random body")
	writeFile(t, dir, "README.md", "hi")
	writeFile(t, dir, ".hidden.surql", "SELECT 1;")

	ms, err := DiscoverMigrations(dir)
	if err != nil {
		t.Fatalf("DiscoverMigrations: %v", err)
	}
	if len(ms) != 1 || ms[0].Version != "20260102_120000" {
		t.Errorf("unexpected result: %+v", ms)
	}
}

func TestDiscoverMigrations_SkipsSubdirectories(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "20260102_120000_looks_like_migration.surql")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeFile(t, dir, "20260103_120000_real.surql", sampleNoHeaders)

	ms, err := DiscoverMigrations(dir)
	if err != nil {
		t.Fatalf("DiscoverMigrations: %v", err)
	}
	if len(ms) != 1 || ms[0].Version != "20260103_120000" {
		t.Errorf("unexpected result: %+v", ms)
	}
}

func TestDiscoverMigrations_PropagatesLoadError(t *testing.T) {
	dir := t.TempDir()
	// Valid filename but no markers -> load error.
	writeFile(t, dir, "20260102_120000_broken.surql", "SELECT 1;\n")

	_, err := DiscoverMigrations(dir)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !stdErrors.Is(err, surqlerrors.ErrMigrationDiscovery) {
		t.Errorf("discovery kind not found: %v", err)
	}
	if !stdErrors.Is(err, surqlerrors.ErrMigrationLoad) {
		t.Errorf("load kind not found in chain: %v", err)
	}
}

func TestDiscoverMigrations_MultipleValidFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "20260101_000000_init.surql", sampleNoHeaders)
	writeFile(t, dir, "20260102_120000_create_user_table.surql", sampleMigration)

	ms, err := DiscoverMigrations(dir)
	if err != nil {
		t.Fatalf("DiscoverMigrations: %v", err)
	}
	if len(ms) != 2 {
		t.Fatalf("len = %d", len(ms))
	}
	if ms[0].Version != "20260101_000000" || ms[1].Version != "20260102_120000" {
		t.Errorf("unexpected versions: %+v", ms)
	}
	if ms[1].Description != "create user table" {
		t.Errorf("description = %q", ms[1].Description)
	}
}

func TestLoadMigration_EmptyUpSectionAllowed(t *testing.T) {
	// An empty up section is unusual but not structurally illegal; it just
	// yields nil statements. We assert the parser doesn't crash and caller
	// gets back an empty slice, not an error.
	body := `-- @up
-- @down
REMOVE TABLE foo;
`
	dir := t.TempDir()
	p := writeFile(t, dir, "20260102_120000_empty_up.surql", body)

	m, err := LoadMigration(p)
	if err != nil {
		t.Fatalf("LoadMigration: %v", err)
	}
	if len(m.UpStatements) != 0 {
		t.Errorf("expected empty up, got %+v", m.UpStatements)
	}
	if len(m.DownStatements) != 1 {
		t.Errorf("down = %+v", m.DownStatements)
	}
}

func TestLoadMigration_TrailingWhitespaceInStatements(t *testing.T) {
	body := "-- @up\n   DEFINE TABLE foo ;   \n-- @down\nREMOVE TABLE foo;\n"
	dir := t.TempDir()
	p := writeFile(t, dir, "20260102_120000_ws.surql", body)

	m, err := LoadMigration(p)
	if err != nil {
		t.Fatalf("LoadMigration: %v", err)
	}
	if len(m.UpStatements) != 1 || m.UpStatements[0] != "DEFINE TABLE foo;" {
		t.Errorf("up = %+v", m.UpStatements)
	}
}

func TestHumanizeDescriptionFallback_Empty(t *testing.T) {
	// If somehow the filename regex succeeds with an empty description (it
	// cannot today), the helper should still behave sensibly. We test the
	// exported path via a filename whose description is a single word.
	dir := t.TempDir()
	body := `-- @up
SELECT 1;
-- @down
SELECT 0;
`
	p := writeFile(t, dir, "20260102_120000_single.surql", body)
	m, err := LoadMigration(p)
	if err != nil {
		t.Fatalf("LoadMigration: %v", err)
	}
	if m.Description != "single" {
		t.Errorf("description = %q", m.Description)
	}
}
