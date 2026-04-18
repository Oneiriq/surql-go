package migration

import (
	"context"
	stdErrors "errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	surqlerrors "github.com/albedosehen/surql-go/pkg/surql/errors"
	"github.com/albedosehen/surql-go/pkg/surql/schema"
)

// --- DriftSeverity ---

func TestDriftSeverity_IsValid(t *testing.T) {
	for _, s := range []DriftSeverity{DriftSeverityInfo, DriftSeverityWarning, DriftSeverityError} {
		if !s.IsValid() {
			t.Errorf("expected %q to be valid", s)
		}
	}
}

func TestDriftSeverity_IsValid_Unknown(t *testing.T) {
	if DriftSeverity("bogus").IsValid() {
		t.Errorf("expected 'bogus' to be invalid")
	}
	if DriftSeverity("").IsValid() {
		t.Errorf("expected empty severity to be invalid")
	}
}

func TestDriftSeverity_String(t *testing.T) {
	if got := DriftSeverityWarning.String(); got != "warning" {
		t.Errorf("String()=%q, want %q", got, "warning")
	}
}

// --- severityForOperation ---

func TestSeverityForOperation_AddsAreInfo(t *testing.T) {
	cases := []DiffOperation{
		DiffOperationAddTable,
		DiffOperationAddField,
		DiffOperationAddIndex,
		DiffOperationAddEvent,
	}
	for _, op := range cases {
		if got := severityForOperation(op); got != DriftSeverityInfo {
			t.Errorf("severityForOperation(%q)=%q, want info", op, got)
		}
	}
}

func TestSeverityForOperation_DropsAndModifiesAreWarnings(t *testing.T) {
	cases := []DiffOperation{
		DiffOperationDropTable,
		DiffOperationDropField,
		DiffOperationDropIndex,
		DiffOperationDropEvent,
		DiffOperationModifyField,
		DiffOperationModifyPermissions,
	}
	for _, op := range cases {
		if got := severityForOperation(op); got != DriftSeverityWarning {
			t.Errorf("severityForOperation(%q)=%q, want warning", op, got)
		}
	}
}

func TestSeverityForOperation_UnknownFallsBackToInfo(t *testing.T) {
	if got := severityForOperation(DiffOperation("made_up")); got != DriftSeverityInfo {
		t.Errorf("unknown operation should default to info, got %q", got)
	}
}

// --- CheckSchemaDriftFromSnapshots ---

func snapshotWithTable(t schema.TableDefinition) SchemaSnapshot {
	return SchemaSnapshot{Tables: []schema.TableDefinition{t}}
}

func TestCheckSchemaDriftFromSnapshots_NoDrift(t *testing.T) {
	tbl := schema.NewTable("user", schema.WithFields(schema.StringField("email")))
	snap := snapshotWithTable(tbl)

	report, err := CheckSchemaDriftFromSnapshots(snap, snap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.DriftDetected {
		t.Errorf("expected DriftDetected=false, got true")
	}
	if len(report.Issues) != 0 {
		t.Errorf("expected no issues, got %d", len(report.Issues))
	}
	if report.SuggestedMigration != "" {
		t.Errorf("expected empty SuggestedMigration, got %q", report.SuggestedMigration)
	}
}

func TestCheckSchemaDriftFromSnapshots_AddedTable(t *testing.T) {
	code := snapshotWithTable(schema.NewTable("user"))
	recorded := SchemaSnapshot{}

	report, err := CheckSchemaDriftFromSnapshots(code, recorded)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !report.DriftDetected {
		t.Fatalf("expected DriftDetected=true")
	}
	if len(report.Issues) == 0 {
		t.Fatalf("expected at least one issue")
	}
	if report.Issues[0].Operation != DiffOperationAddTable {
		t.Errorf("first operation=%q, want add_table", report.Issues[0].Operation)
	}
	if report.Issues[0].Severity != DriftSeverityInfo {
		t.Errorf("ADD should be info, got %q", report.Issues[0].Severity)
	}
	if report.Issues[0].Table != "user" {
		t.Errorf("Table=%q, want user", report.Issues[0].Table)
	}
	if report.SuggestedMigration == "" {
		t.Errorf("expected non-empty SuggestedMigration")
	}
}

func TestCheckSchemaDriftFromSnapshots_DroppedTable(t *testing.T) {
	code := SchemaSnapshot{}
	recorded := snapshotWithTable(schema.NewTable("legacy"))

	report, err := CheckSchemaDriftFromSnapshots(code, recorded)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !report.DriftDetected {
		t.Fatalf("expected drift detected")
	}
	found := false
	for _, iss := range report.Issues {
		if iss.Operation == DiffOperationDropTable && iss.Table == "legacy" {
			if iss.Severity != DriftSeverityWarning {
				t.Errorf("drop_table severity=%q, want warning", iss.Severity)
			}
			found = true
		}
	}
	if !found {
		t.Errorf("expected drop_table for 'legacy' in issues: %+v", report.Issues)
	}
}

func TestCheckSchemaDriftFromSnapshots_AddedField(t *testing.T) {
	code := snapshotWithTable(schema.NewTable("user",
		schema.WithFields(schema.StringField("email"), schema.IntField("age")),
	))
	recorded := snapshotWithTable(schema.NewTable("user",
		schema.WithFields(schema.StringField("email")),
	))

	report, err := CheckSchemaDriftFromSnapshots(code, recorded)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !report.DriftDetected {
		t.Fatalf("expected drift detected")
	}
	if len(report.Issues) != 1 {
		t.Fatalf("expected exactly 1 issue, got %d: %+v", len(report.Issues), report.Issues)
	}
	iss := report.Issues[0]
	if iss.Operation != DiffOperationAddField {
		t.Errorf("operation=%q, want add_field", iss.Operation)
	}
	if iss.Field != "age" {
		t.Errorf("field=%q, want age", iss.Field)
	}
	if iss.Severity != DriftSeverityInfo {
		t.Errorf("severity=%q, want info", iss.Severity)
	}
}

func TestCheckSchemaDriftFromSnapshots_ModifyField(t *testing.T) {
	code := snapshotWithTable(schema.NewTable("user",
		schema.WithFields(schema.IntField("age")),
	))
	recorded := snapshotWithTable(schema.NewTable("user",
		schema.WithFields(schema.StringField("age")),
	))

	report, err := CheckSchemaDriftFromSnapshots(code, recorded)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !report.DriftDetected {
		t.Fatalf("expected drift detected")
	}
	if len(report.Issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(report.Issues))
	}
	if report.Issues[0].Operation != DiffOperationModifyField {
		t.Errorf("want modify_field, got %q", report.Issues[0].Operation)
	}
	if report.Issues[0].Severity != DriftSeverityWarning {
		t.Errorf("modify_field should be warning, got %q", report.Issues[0].Severity)
	}
}

func TestCheckSchemaDriftFromSnapshots_BothEmpty(t *testing.T) {
	report, err := CheckSchemaDriftFromSnapshots(SchemaSnapshot{}, SchemaSnapshot{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.DriftDetected {
		t.Errorf("expected no drift on two empty snapshots")
	}
}

func TestCheckSchemaDriftFromSnapshots_DescriptionPreserved(t *testing.T) {
	code := snapshotWithTable(schema.NewTable("user"))
	recorded := SchemaSnapshot{}

	report, err := CheckSchemaDriftFromSnapshots(code, recorded)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Issues[0].Description == "" {
		t.Errorf("expected non-empty description on add_table issue")
	}
}

// --- GeneratePreCommitConfig ---

func TestGeneratePreCommitConfig_DefaultsIncludeFailFlag(t *testing.T) {
	out := GeneratePreCommitConfig("schemas/", true)
	if !strings.Contains(out, "--fail-on-drift") {
		t.Errorf("expected --fail-on-drift in output, got:\n%s", out)
	}
	if !strings.Contains(out, "schemas/") {
		t.Errorf("expected schema path in output, got:\n%s", out)
	}
	if !strings.Contains(out, "surql-go migrate check") {
		t.Errorf("expected entry command, got:\n%s", out)
	}
}

func TestGeneratePreCommitConfig_NoFailFlag(t *testing.T) {
	out := GeneratePreCommitConfig("src/schemas", false)
	if strings.Contains(out, "--fail-on-drift") {
		t.Errorf("did not expect --fail-on-drift in output, got:\n%s", out)
	}
	if !strings.Contains(out, "src/schemas") {
		t.Errorf("expected src/schemas in output, got:\n%s", out)
	}
}

func TestGeneratePreCommitConfig_EmptyPathFallsBack(t *testing.T) {
	out := GeneratePreCommitConfig("", true)
	if !strings.Contains(out, "schemas/") {
		t.Errorf("empty schema path should fall back to 'schemas/', got:\n%s", out)
	}
}

func TestGeneratePreCommitConfig_WhitespacePathFallsBack(t *testing.T) {
	out := GeneratePreCommitConfig("   ", false)
	if !strings.Contains(out, "schemas/") {
		t.Errorf("whitespace schema path should fall back to 'schemas/', got:\n%s", out)
	}
}

func TestGeneratePreCommitConfig_StructuralLines(t *testing.T) {
	out := GeneratePreCommitConfig("schemas/", true)
	requiredLines := []string{
		"repos:",
		"- repo: local",
		"hooks:",
		"- id: surql-schema-check",
		"name: Check schema migrations",
		"language: system",
		"pass_filenames: false",
	}
	for _, line := range requiredLines {
		if !strings.Contains(out, line) {
			t.Errorf("expected %q in output, got:\n%s", line, out)
		}
	}
}

func TestGeneratePreCommitConfig_TrailingNewline(t *testing.T) {
	out := GeneratePreCommitConfig("schemas/", true)
	if !strings.HasSuffix(out, "\n") {
		t.Errorf("expected trailing newline")
	}
}

// --- GetStagedSchemaFiles: helpers for git-integration tests ---

// hasGit reports whether a git binary is available on PATH. Several tests
// gate on this to stay runnable on sandboxed build machines.
func hasGit(t *testing.T) bool {
	t.Helper()
	_, err := exec.LookPath("git")
	return err == nil
}

// initGitRepo initialises a git repo rooted at dir and sets a minimal
// user.name / user.email so commits are possible. Returns dir.
func initGitRepo(t *testing.T, dir string) string {
	t.Helper()
	if !hasGit(t) {
		t.Skip("git not available")
	}
	runOrFatal(t, dir, "git", "init", "--initial-branch=main")
	runOrFatal(t, dir, "git", "config", "user.email", "test@example.com")
	runOrFatal(t, dir, "git", "config", "user.name", "Test User")
	runOrFatal(t, dir, "git", "config", "commit.gpgsign", "false")
	return dir
}

func runOrFatal(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s: %v: %s", name, strings.Join(args, " "), err, out)
	}
}

func stageFile(t *testing.T, repo, relPath, body string) {
	t.Helper()
	full := filepath.Join(repo, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	runOrFatal(t, repo, "git", "add", relPath)
}

// --- GetStagedSchemaFiles tests ---

func TestGetStagedSchemaFiles_EmptyDirRejected(t *testing.T) {
	_, err := GetStagedSchemaFiles("")
	if err == nil {
		t.Fatalf("expected error for empty schema dir")
	}
	if !stdErrors.Is(err, surqlerrors.ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

func TestGetStagedSchemaFiles_WhitespaceRejected(t *testing.T) {
	_, err := GetStagedSchemaFiles("   ")
	if err == nil {
		t.Fatalf("expected error for whitespace schema dir")
	}
}

func TestGetStagedSchemaFiles_NoGitRepo(t *testing.T) {
	if !hasGit(t) {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	// Not a git repo — git diff will exit non-zero.
	_, err := GetStagedSchemaFiles(dir)
	if err == nil {
		t.Fatalf("expected error running outside git repo")
	}
	if !stdErrors.Is(err, surqlerrors.ErrMigrationGeneration) {
		t.Errorf("expected ErrMigrationGeneration, got %v", err)
	}
}

func TestGetStagedSchemaFiles_NoStaged(t *testing.T) {
	if !hasGit(t) {
		t.Skip("git not available")
	}
	repo := initGitRepo(t, t.TempDir())
	schemaDir := filepath.Join(repo, "schemas")
	if err := os.MkdirAll(schemaDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	files, err := GetStagedSchemaFiles(schemaDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected no staged files, got %v", files)
	}
}

func TestGetStagedSchemaFiles_StagedUnderSchemaDir(t *testing.T) {
	if !hasGit(t) {
		t.Skip("git not available")
	}
	repo := initGitRepo(t, t.TempDir())
	stageFile(t, repo, "schemas/user.go", "package schemas\n")
	stageFile(t, repo, "schemas/nested/post.go", "package nested\n")
	stageFile(t, repo, "other/unrelated.go", "package other\n")

	schemaDir := filepath.Join(repo, "schemas")
	files, err := GetStagedSchemaFiles(schemaDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sort.Strings(files)
	want := []string{"schemas/nested/post.go", "schemas/user.go"}
	if len(files) != len(want) {
		t.Fatalf("got %v, want %v", files, want)
	}
	for i := range want {
		if files[i] != want[i] {
			t.Errorf("files[%d]=%q, want %q", i, files[i], want[i])
		}
	}
}

func TestGetStagedSchemaFiles_FiltersOutsideSchemaDir(t *testing.T) {
	if !hasGit(t) {
		t.Skip("git not available")
	}
	repo := initGitRepo(t, t.TempDir())
	stageFile(t, repo, "lib/utils.go", "package lib\n")
	stageFile(t, repo, "schemas/only.go", "package schemas\n")

	files, err := GetStagedSchemaFiles(filepath.Join(repo, "schemas"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 1 || files[0] != "schemas/only.go" {
		t.Errorf("want [schemas/only.go], got %v", files)
	}
}

func TestGetStagedSchemaFiles_NonExistentDirReturnsError(t *testing.T) {
	if !hasGit(t) {
		t.Skip("git not available")
	}
	// A non-existent absolute path is still resolvable; git itself will fail
	// because we run it with cwd=absSchema, so expect ErrMigrationGeneration.
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	_, err := GetStagedSchemaFiles(missing)
	if err == nil {
		t.Fatalf("expected error when schema dir does not exist")
	}
	if !stdErrors.Is(err, surqlerrors.ErrMigrationGeneration) {
		t.Errorf("expected ErrMigrationGeneration, got %v", err)
	}
}

func TestGetStagedSchemaFiles_NestedRepoPath(t *testing.T) {
	if !hasGit(t) {
		t.Skip("git not available")
	}
	repo := initGitRepo(t, t.TempDir())
	stageFile(t, repo, "app/schemas/inner/model.go", "package inner\n")

	schemaDir := filepath.Join(repo, "app", "schemas")
	files, err := GetStagedSchemaFiles(schemaDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 1 || files[0] != "app/schemas/inner/model.go" {
		t.Errorf("want [app/schemas/inner/model.go], got %v", files)
	}
}

func TestGetStagedSchemaFiles_RelativePathAccepted(t *testing.T) {
	if !hasGit(t) {
		t.Skip("git not available")
	}
	repo := initGitRepo(t, t.TempDir())
	stageFile(t, repo, "schemas/model.go", "package schemas\n")

	// Use a relative path via chdir to exercise filepath.Abs.
	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
	if err := os.Chdir(repo); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	files, err := GetStagedSchemaFiles("schemas")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 1 || files[0] != "schemas/model.go" {
		t.Errorf("want [schemas/model.go], got %v", files)
	}
}

func TestGetStagedSchemaFiles_ContextCancelled(t *testing.T) {
	if !hasGit(t) {
		t.Skip("git not available")
	}
	repo := initGitRepo(t, t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel so git never runs
	_, err := GetStagedSchemaFilesContext(ctx, filepath.Join(repo, "schemas"))
	if err == nil {
		t.Fatalf("expected error when context is cancelled")
	}
}

func TestGetStagedSchemaFiles_MultipleNestedFilesSorted(t *testing.T) {
	if !hasGit(t) {
		t.Skip("git not available")
	}
	repo := initGitRepo(t, t.TempDir())
	stageFile(t, repo, "schemas/z.go", "package schemas\n")
	stageFile(t, repo, "schemas/a.go", "package schemas\n")
	stageFile(t, repo, "schemas/m.go", "package schemas\n")

	files, err := GetStagedSchemaFiles(filepath.Join(repo, "schemas"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"schemas/a.go", "schemas/m.go", "schemas/z.go"}
	if len(files) != len(want) {
		t.Fatalf("len mismatch: got %v, want %v", files, want)
	}
	for i := range want {
		if files[i] != want[i] {
			t.Errorf("files[%d]=%q, want %q", i, files[i], want[i])
		}
	}
}

// --- isUnderDirectory ---

func TestIsUnderDirectory_Positive(t *testing.T) {
	root := filepath.Join("/tmp", "schemas")
	cand := filepath.Join("/tmp", "schemas", "a", "b.go")
	if !isUnderDirectory(cand, root) {
		t.Errorf("expected %q to be under %q", cand, root)
	}
}

func TestIsUnderDirectory_Equal(t *testing.T) {
	root := filepath.Join("/tmp", "schemas")
	if !isUnderDirectory(root, root) {
		t.Errorf("expected identical paths to report under-directory")
	}
}

func TestIsUnderDirectory_Escape(t *testing.T) {
	root := filepath.Join("/tmp", "schemas")
	cand := filepath.Join("/tmp", "other", "a.go")
	if isUnderDirectory(cand, root) {
		t.Errorf("expected %q not under %q", cand, root)
	}
}

func TestIsUnderDirectory_Sibling(t *testing.T) {
	root := filepath.Join("/a", "b")
	cand := filepath.Join("/a", "bc")
	if isUnderDirectory(cand, root) {
		t.Errorf("expected %q not under %q (prefix-match guard)", cand, root)
	}
}

// --- wrapGitError ---

func TestWrapGitError_PlainError(t *testing.T) {
	base := stdErrors.New("boom")
	wrapped := wrapGitError(base, "git thing failed")
	if !stdErrors.Is(wrapped, surqlerrors.ErrMigrationGeneration) {
		t.Errorf("expected wrapped to be ErrMigrationGeneration, got %v", wrapped)
	}
	if !strings.Contains(wrapped.Error(), "git thing failed") {
		t.Errorf("expected reason in message, got %v", wrapped)
	}
}

// --- GetStagedSchemaFiles: injected runner ---

func TestGetStagedSchemaFiles_CustomRunnerSuccess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("forward-slash path assumptions skip on Windows")
	}
	if !hasGit(t) {
		t.Skip("git not available")
	}
	// Build a repo so the rev-parse step succeeds; then swap the diff runner.
	repo := initGitRepo(t, t.TempDir())
	stageFile(t, repo, "schemas/stub.go", "package schemas\n")

	orig := gitCommandRunner
	t.Cleanup(func() { gitCommandRunner = orig })
	gitCommandRunner = func(ctx context.Context, cwd string) (string, error) {
		return "schemas/injected.go\nschemas/nested/other.go\n", nil
	}

	files, err := GetStagedSchemaFiles(filepath.Join(repo, "schemas"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"schemas/injected.go", "schemas/nested/other.go"}
	if len(files) != len(want) {
		t.Fatalf("len mismatch: got %v, want %v", files, want)
	}
	for i := range want {
		if files[i] != want[i] {
			t.Errorf("files[%d]=%q, want %q", i, files[i], want[i])
		}
	}
}

func TestGetStagedSchemaFiles_RunnerReturnsError(t *testing.T) {
	if !hasGit(t) {
		t.Skip("git not available")
	}
	repo := initGitRepo(t, t.TempDir())

	orig := gitCommandRunner
	t.Cleanup(func() { gitCommandRunner = orig })
	sentinel := stdErrors.New("runner boom")
	gitCommandRunner = func(ctx context.Context, cwd string) (string, error) {
		return "", sentinel
	}

	_, err := GetStagedSchemaFiles(filepath.Join(repo, "schemas"))
	if err == nil {
		t.Fatalf("expected runner error")
	}
	if !stdErrors.Is(err, sentinel) {
		t.Errorf("expected runner sentinel, got %v", err)
	}
}

func TestGetStagedSchemaFiles_BlankLinesIgnored(t *testing.T) {
	if !hasGit(t) {
		t.Skip("git not available")
	}
	repo := initGitRepo(t, t.TempDir())
	stageFile(t, repo, "schemas/stub.go", "package schemas\n")

	orig := gitCommandRunner
	t.Cleanup(func() { gitCommandRunner = orig })
	gitCommandRunner = func(ctx context.Context, cwd string) (string, error) {
		return "\nschemas/real.go\n\n\n", nil
	}

	files, err := GetStagedSchemaFiles(filepath.Join(repo, "schemas"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 1 || files[0] != "schemas/real.go" {
		t.Errorf("want [schemas/real.go], got %v", files)
	}
}
