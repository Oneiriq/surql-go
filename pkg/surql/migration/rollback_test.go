package migration

import (
	stdErrors "errors"
	"os"
	"path/filepath"
	"sort"
	"testing"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

// writeTestMigrationFile writes a migration file with the given version and
// down statements. The up section is a no-op that still satisfies
// parseMigrationContent.
func writeTestMigrationFile(t *testing.T, dir, filename, downSQL string) {
	t.Helper()
	body := "-- @description: test\n-- @up\nDEFINE TABLE noop SCHEMAFULL;\n-- @down\n" + downSQL + "\n"
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", filename, err)
	}
}

// --- RollbackSafety ---

func TestRollbackSafety_IsValid(t *testing.T) {
	for _, s := range []RollbackSafety{RollbackSafetySafe, RollbackSafetyWarning, RollbackSafetyDanger} {
		if !s.IsValid() {
			t.Errorf("expected %q to be valid", s)
		}
	}
	if RollbackSafety("bogus").IsValid() {
		t.Errorf("expected 'bogus' to be invalid")
	}
	if RollbackSafety("").IsValid() {
		t.Errorf("expected '' to be invalid")
	}
}

func TestRollbackSafety_String(t *testing.T) {
	if got := RollbackSafetyWarning.String(); got != "warning" {
		t.Errorf("String()=%q, want %q", got, "warning")
	}
}

func TestRollbackSafety_Rank_Ordering(t *testing.T) {
	cases := []struct {
		a, b RollbackSafety
		want bool
	}{
		{RollbackSafetyDanger, RollbackSafetyWarning, true},
		{RollbackSafetyWarning, RollbackSafetySafe, true},
		{RollbackSafetySafe, RollbackSafetyDanger, false},
	}
	for _, tc := range cases {
		if got := tc.a.rank() > tc.b.rank(); got != tc.want {
			t.Errorf("%s > %s: got %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

// --- analyzeMigrationSafety ---

func TestAnalyzeMigrationSafety_DropTable_Danger(t *testing.T) {
	m := Migration{
		Version:        "20260102_120000",
		DownStatements: []string{"REMOVE TABLE user;"},
	}
	issues := analyzeMigrationSafety(m)
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if issues[0].Safety != RollbackSafetyDanger {
		t.Errorf("safety=%q, want %q", issues[0].Safety, RollbackSafetyDanger)
	}
	if issues[0].AffectedData == "" {
		t.Errorf("expected AffectedData to be populated")
	}
}

func TestAnalyzeMigrationSafety_DropField_Warning(t *testing.T) {
	m := Migration{
		Version:        "20260102_120000",
		DownStatements: []string{"REMOVE FIELD email ON TABLE user;"},
	}
	issues := analyzeMigrationSafety(m)
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if issues[0].Safety != RollbackSafetyWarning {
		t.Errorf("safety=%q, want %q", issues[0].Safety, RollbackSafetyWarning)
	}
}

func TestAnalyzeMigrationSafety_DropIndex_NoIssue(t *testing.T) {
	m := Migration{
		Version:        "20260102_120000",
		DownStatements: []string{"REMOVE INDEX email_idx ON TABLE user;"},
	}
	issues := analyzeMigrationSafety(m)
	if len(issues) != 0 {
		t.Fatalf("expected 0 issues, got %d: %+v", len(issues), issues)
	}
}

func TestAnalyzeMigrationSafety_AlterType_Warning(t *testing.T) {
	m := Migration{
		Version:        "20260102_120000",
		DownStatements: []string{"DEFINE FIELD age ON TABLE user TYPE string;"},
	}
	issues := analyzeMigrationSafety(m)
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if issues[0].Safety != RollbackSafetyWarning {
		t.Errorf("safety=%q, want %q", issues[0].Safety, RollbackSafetyWarning)
	}
}

func TestAnalyzeMigrationSafety_EmptyDown_Danger(t *testing.T) {
	m := Migration{Version: "20260102_120000"}
	issues := analyzeMigrationSafety(m)
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if issues[0].Safety != RollbackSafetyDanger {
		t.Errorf("safety=%q, want %q", issues[0].Safety, RollbackSafetyDanger)
	}
}

func TestAnalyzeMigrationSafety_MultipleStatements(t *testing.T) {
	m := Migration{
		Version: "20260102_120000",
		DownStatements: []string{
			"REMOVE TABLE user;",
			"REMOVE FIELD email ON TABLE customer;",
			"REMOVE INDEX email_idx ON TABLE customer;",
		},
	}
	issues := analyzeMigrationSafety(m)
	if len(issues) != 2 {
		t.Fatalf("expected 2 issues, got %d: %+v", len(issues), issues)
	}

	safetyBag := make(map[RollbackSafety]int, len(issues))
	for _, iss := range issues {
		safetyBag[iss.Safety]++
	}
	if safetyBag[RollbackSafetyDanger] != 1 {
		t.Errorf("expected 1 danger, got %d", safetyBag[RollbackSafetyDanger])
	}
	if safetyBag[RollbackSafetyWarning] != 1 {
		t.Errorf("expected 1 warning, got %d", safetyBag[RollbackSafetyWarning])
	}
}

// --- AnalyzeRollbackSafety ---

func TestAnalyzeRollbackSafety_TargetNotFound(t *testing.T) {
	dir := t.TempDir()
	writeTestMigrationFile(t, dir, "20260102_120000_first.surql", "REMOVE TABLE foo;")

	_, err := AnalyzeRollbackSafety(dir, "99999999_999999")
	if err == nil {
		t.Fatal("expected error for missing target version")
	}
	if !stdErrors.Is(err, surqlerrors.ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

func TestAnalyzeRollbackSafety_OnlyCandidatesAbove(t *testing.T) {
	dir := t.TempDir()
	writeTestMigrationFile(t, dir, "20260101_000000_init.surql", "REMOVE TABLE foo;")
	writeTestMigrationFile(t, dir, "20260102_000000_add_table.surql", "REMOVE TABLE bar;")
	writeTestMigrationFile(t, dir, "20260103_000000_add_field.surql", "REMOVE FIELD x ON TABLE baz;")

	issues, err := AnalyzeRollbackSafety(dir, "20260101_000000")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Expect one danger (drop bar) + one warning (drop field x). The
	// target version itself is not analysed.
	if len(issues) != 2 {
		t.Fatalf("expected 2 issues, got %d: %+v", len(issues), issues)
	}
}

func TestAnalyzeRollbackSafety_TargetIsLatest_NoIssues(t *testing.T) {
	dir := t.TempDir()
	writeTestMigrationFile(t, dir, "20260101_000000_init.surql", "REMOVE TABLE foo;")
	writeTestMigrationFile(t, dir, "20260102_000000_next.surql", "REMOVE TABLE bar;")

	issues, err := AnalyzeRollbackSafety(dir, "20260102_000000")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("expected 0 issues when target is latest, got %d", len(issues))
	}
}

// --- RollbackPlan helpers ---

func TestRollbackPlan_IsSafe_HasDataLoss(t *testing.T) {
	safe := RollbackPlan{OverallSafety: RollbackSafetySafe}
	if !safe.IsSafe() {
		t.Error("safe plan should report IsSafe")
	}
	if safe.HasDataLoss() {
		t.Error("safe plan should not report data loss")
	}

	warn := RollbackPlan{OverallSafety: RollbackSafetyWarning}
	if warn.IsSafe() {
		t.Error("warning plan should not report IsSafe")
	}
	if !warn.HasDataLoss() {
		t.Error("warning plan should report data loss")
	}

	danger := RollbackPlan{OverallSafety: RollbackSafetyDanger}
	if !danger.HasDataLoss() {
		t.Error("danger plan should report data loss")
	}
}

func TestRollbackPlan_MigrationCount(t *testing.T) {
	p := RollbackPlan{
		Migrations: []Migration{
			{Version: "a"}, {Version: "b"}, {Version: "c"},
		},
	}
	if got := p.MigrationCount(); got != 3 {
		t.Errorf("MigrationCount()=%d, want 3", got)
	}
}

// --- migrationsNewerThan ---

func TestMigrationsNewerThan_Ordering(t *testing.T) {
	migrations := []Migration{
		{Version: "20260103_000000"},
		{Version: "20260101_000000"},
		{Version: "20260102_000000"},
		{Version: "20260104_000000"},
	}
	out := migrationsNewerThan(migrations, "20260101_000000")
	if len(out) != 3 {
		t.Fatalf("expected 3 migrations newer than target, got %d", len(out))
	}
	versions := make([]string, len(out))
	for i, m := range out {
		versions[i] = m.Version
	}
	if !sort.StringsAreSorted(versions) {
		t.Errorf("migrations not sorted ascending: %v", versions)
	}
}

// --- RollbackResult ---

func TestRollbackResult_CompletedAll(t *testing.T) {
	plan := RollbackPlan{Migrations: []Migration{{Version: "a"}, {Version: "b"}}}
	if (RollbackResult{Plan: plan, RolledBackCount: 2}).CompletedAll() != true {
		t.Error("expected CompletedAll when count matches plan")
	}
	if (RollbackResult{Plan: plan, RolledBackCount: 1}).CompletedAll() != false {
		t.Error("expected !CompletedAll when count < plan")
	}
}
