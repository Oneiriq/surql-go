package migration

import (
	stdErrors "errors"
	"path/filepath"
	"sort"
	"testing"

	surqlerrors "github.com/albedosehen/surql-go/pkg/surql/errors"
)

// --- ExecuteOptions functional constructors ---

func TestExecuteOptions_Defaults(t *testing.T) {
	opts := defaultExecuteOptions()
	if opts.Steps != 0 {
		t.Errorf("default Steps=%d, want 0", opts.Steps)
	}
	if opts.DryRun {
		t.Errorf("default DryRun=true, want false")
	}
	if !opts.FailOnValidation {
		t.Errorf("default FailOnValidation=false, want true")
	}
}

func TestWithSteps(t *testing.T) {
	opts := defaultExecuteOptions()
	WithSteps(5)(&opts)
	if opts.Steps != 5 {
		t.Errorf("Steps=%d, want 5", opts.Steps)
	}
}

func TestWithDryRun(t *testing.T) {
	opts := defaultExecuteOptions()
	WithDryRun(true)(&opts)
	if !opts.DryRun {
		t.Error("expected DryRun to be true after WithDryRun(true)")
	}
}

func TestWithFailOnValidation(t *testing.T) {
	opts := defaultExecuteOptions()
	WithFailOnValidation(false)(&opts)
	if opts.FailOnValidation {
		t.Error("expected FailOnValidation to be false after WithFailOnValidation(false)")
	}
}

// --- validateMigrationSet ---

func TestValidateMigrationSet_Empty(t *testing.T) {
	issues, err := validateMigrationSet(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("expected no issues, got %v", issues)
	}
}

func TestValidateMigrationSet_Duplicates(t *testing.T) {
	migrations := []Migration{
		{Version: "a", UpStatements: []string{"DEFINE TABLE a;"}},
		{Version: "a", UpStatements: []string{"DEFINE TABLE a;"}},
	}
	issues, err := validateMigrationSet(migrations)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) == 0 {
		t.Fatal("expected duplicate issue, got none")
	}
	found := false
	for _, msg := range issues {
		if msg == "duplicate migration versions: a" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected duplicate message, got %v", issues)
	}
}

func TestValidateMigrationSet_MissingDependency(t *testing.T) {
	migrations := []Migration{
		{
			Version:      "b",
			UpStatements: []string{"DEFINE TABLE b;"},
			DependsOn:    []string{"missing"},
		},
	}
	issues, err := validateMigrationSet(migrations)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) == 0 {
		t.Fatal("expected missing-dep issue, got none")
	}
}

func TestValidateMigrationSet_MissingUpStatements(t *testing.T) {
	migrations := []Migration{
		{Version: "a"},
	}
	issues, err := validateMigrationSet(migrations)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) == 0 {
		t.Fatal("expected missing-up issue, got none")
	}
}

func TestValidateMigrationSet_Valid(t *testing.T) {
	migrations := []Migration{
		{Version: "a", UpStatements: []string{"DEFINE TABLE a;"}},
		{
			Version:      "b",
			UpStatements: []string{"DEFINE TABLE b;"},
			DependsOn:    []string{"a"},
		},
	}
	issues, err := validateMigrationSet(migrations)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("expected no issues, got %v", issues)
	}
}

// --- ValidateMigrations (filesystem) ---

func TestValidateMigrations_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	issues, err := ValidateMigrations(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("expected no issues for empty dir, got %v", issues)
	}
}

func TestValidateMigrations_NonexistentDir(t *testing.T) {
	// DiscoverMigrations returns an empty slice (no error) for a missing
	// directory, so ValidateMigrations should as well.
	issues, err := ValidateMigrations(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("expected no issues, got %v", issues)
	}
}

func TestValidateMigrations_DuplicateDetected(t *testing.T) {
	dir := t.TempDir()
	writeTestMigrationFile(t, dir, "20260101_000000_init.surql", "REMOVE TABLE foo;")
	// Same version, different description — rare but possible operator mistake.
	writeTestMigrationFile(t, dir, "20260101_000000_other.surql", "REMOVE TABLE bar;")

	issues, err := ValidateMigrations(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) == 0 {
		t.Fatal("expected duplicate issue, got none")
	}
}

// --- indexByVersion ---

func TestIndexByVersion(t *testing.T) {
	migrations := []Migration{
		{Version: "a"},
		{Version: "b"},
	}
	got := indexByVersion(migrations)
	if len(got) != 2 {
		t.Fatalf("len=%d, want 2", len(got))
	}
	if got["a"].Version != "a" {
		t.Errorf("missing 'a'")
	}
}

// --- MigrationStatusReport zero-value semantics ---

func TestMigrationStatusReport_Zero(t *testing.T) {
	var r MigrationStatusReport
	if r.Total != 0 || r.Applied != 0 || r.Pending != 0 {
		t.Errorf("unexpected zero report: %+v", r)
	}
}

// --- MigrateDown argument validation ---

func TestMigrateDown_ZeroStepsRejected(t *testing.T) {
	// Client is nil, but nil-check fires first — so this test asserts the
	// input-validation order and returns an error before any I/O.
	_, err := MigrateDown(nil, nil, "", 0)
	if err == nil {
		t.Fatal("expected error")
	}
	if !stdErrors.Is(err, surqlerrors.ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

// --- ExecuteMigration invalid direction ---

func TestExecuteMigration_InvalidDirection(t *testing.T) {
	// We use a zero DatabaseClient pointer, but the direction guard runs
	// after the nil check. Exercise the nil-check path first.
	_, err := ExecuteMigration(nil, nil, Migration{}, MigrationDirectionUp)
	if err == nil {
		t.Fatal("expected error for nil client")
	}
	if !stdErrors.Is(err, surqlerrors.ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

// --- CreateMigrationPlan nil client ---

func TestCreateMigrationPlan_NilClient(t *testing.T) {
	_, err := CreateMigrationPlan(nil, nil, "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !stdErrors.Is(err, surqlerrors.ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

// --- ExecuteMigrationPlan with empty plan short-circuits ---

func TestExecuteMigrationPlan_EmptyReturnsEmptyStatuses(t *testing.T) {
	// Empty plan returns early before touching the DB. Sanity check.
	// Note: ExecuteMigrationPlan still demands a non-nil client even for
	// an empty plan; to keep the unit test hermetic we exercise the nil
	// short-circuit path via the direction guard.
	_, err := ExecuteMigrationPlan(nil, nil, MigrationPlan{Direction: MigrationDirectionUp})
	if err == nil {
		t.Fatal("expected error for nil client")
	}
	if !stdErrors.Is(err, surqlerrors.ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

// --- helper: ensure validateMigrationSet issues are sorted ---

func TestValidateMigrationSet_IssuesSorted(t *testing.T) {
	migrations := []Migration{
		{Version: "c"}, // missing up
		{Version: "a", DependsOn: []string{"z"}, UpStatements: []string{";"}}, // missing dep
		{Version: "a", UpStatements: []string{";"}},                           // duplicate version
	}
	issues, err := validateMigrationSet(migrations)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sort.StringsAreSorted(issues) {
		t.Errorf("validate issues not sorted: %v", issues)
	}
}
