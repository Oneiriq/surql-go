package migration

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Oneiriq/surql-go/pkg/surql/connection"
	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

// RollbackSafety categorises the risk level of a rollback operation.
//
// Safe   — reversible operations (index drops, cosmetic event changes);
// Warning — may cause data loss (field removal, field type changes);
// Danger  — destructive (table removal, schema collapse).
type RollbackSafety string

// RollbackSafety values, ordered from least to most severe.
const (
	RollbackSafetySafe    RollbackSafety = "safe"
	RollbackSafetyWarning RollbackSafety = "warning"
	RollbackSafetyDanger  RollbackSafety = "danger"
)

// IsValid reports whether the receiver is a defined RollbackSafety.
func (s RollbackSafety) IsValid() bool {
	switch s {
	case RollbackSafetySafe, RollbackSafetyWarning, RollbackSafetyDanger:
		return true
	}
	return false
}

// String returns the serialised form of the safety level.
func (s RollbackSafety) String() string { return string(s) }

// rank returns a total order on RollbackSafety (higher is more severe).
func (s RollbackSafety) rank() int {
	switch s {
	case RollbackSafetyDanger:
		return 3
	case RollbackSafetyWarning:
		return 2
	case RollbackSafetySafe:
		return 1
	}
	return 0
}

// RollbackIssue describes a single concern surfaced while planning a
// rollback. Issues are grouped in RollbackPlan.Issues and used to derive
// the plan's overall safety level.
type RollbackIssue struct {
	Safety         RollbackSafety `json:"safety"`
	Migration      string         `json:"migration"`
	Description    string         `json:"description"`
	AffectedData   string         `json:"affected_data,omitempty"`
	Recommendation string         `json:"recommendation,omitempty"`
}

// RollbackPlan captures the migrations that will be reversed, together with
// a safety analysis. The zero value is not useful; build one via
// CreateRollbackPlan / PlanRollbackToVersion.
type RollbackPlan struct {
	FromVersion       string          `json:"from_version"`
	ToVersion         string          `json:"to_version"`
	Migrations        []Migration     `json:"migrations"`
	OverallSafety     RollbackSafety  `json:"overall_safety"`
	Issues            []RollbackIssue `json:"issues,omitempty"`
	RequiresApproval  bool            `json:"requires_approval"`
	EstimatedDuration *time.Duration  `json:"estimated_duration,omitempty"`
}

// MigrationCount returns the number of migrations the plan will reverse.
func (p RollbackPlan) MigrationCount() int { return len(p.Migrations) }

// IsSafe reports whether the plan is entirely safe (no data loss expected).
func (p RollbackPlan) IsSafe() bool { return p.OverallSafety == RollbackSafetySafe }

// HasDataLoss reports whether the plan may cause data loss.
func (p RollbackPlan) HasDataLoss() bool {
	return p.OverallSafety == RollbackSafetyWarning || p.OverallSafety == RollbackSafetyDanger
}

// RollbackResult is the outcome of executing a RollbackPlan.
type RollbackResult struct {
	Plan            RollbackPlan  `json:"plan"`
	Success         bool          `json:"success"`
	ActualDuration  time.Duration `json:"actual_duration"`
	RolledBackCount int           `json:"rolled_back_count"`
	Errors          []string      `json:"errors,omitempty"`
}

// CompletedAll reports whether every planned migration was rolled back.
func (r RollbackResult) CompletedAll() bool {
	return r.RolledBackCount == r.Plan.MigrationCount()
}

// AnalyzeRollbackSafety walks the migrations on disk and returns the safety
// issues that would arise from rolling back to targetVersion.
//
// Unlike CreateRollbackPlan this helper is database-agnostic: it inspects
// the down-SQL of each migration above targetVersion without consulting the
// history table. Useful for CLI `plan` previews.
func AnalyzeRollbackSafety(migrationsDir, targetVersion string) ([]RollbackIssue, error) {
	migrations, err := DiscoverMigrations(migrationsDir)
	if err != nil {
		return nil, err
	}
	if !versionExists(migrations, targetVersion) {
		return nil, surqlerrors.Newf(
			surqlerrors.ErrValidation,
			"target version %q not found in %q", targetVersion, migrationsDir,
		)
	}
	candidates := migrationsNewerThan(migrations, targetVersion)
	issues := make([]RollbackIssue, 0)
	for _, m := range candidates {
		issues = append(issues, analyzeMigrationSafety(m)...)
	}
	return issues, nil
}

// CreateRollbackPlan builds a safety-analysed rollback plan that takes the
// database from its current version down to targetVersion.
//
// The "current version" is the most recent entry in the history table. When
// no migrations have been applied, the call fails with ErrMigrationHistory.
// The target version must exist on disk.
func CreateRollbackPlan(
	ctx context.Context,
	client *connection.DatabaseClient,
	migrationsDir string,
	targetVersion string,
) (RollbackPlan, error) {
	if client == nil {
		return RollbackPlan{}, surqlerrors.New(surqlerrors.ErrValidation, "client cannot be nil")
	}
	if targetVersion == "" {
		return RollbackPlan{}, surqlerrors.New(surqlerrors.ErrValidation, "target version cannot be empty")
	}

	migrations, err := DiscoverMigrations(migrationsDir)
	if err != nil {
		return RollbackPlan{}, err
	}
	if !versionExists(migrations, targetVersion) {
		return RollbackPlan{}, surqlerrors.Newf(
			surqlerrors.ErrValidation,
			"target version %q not found in %q", targetVersion, migrationsDir,
		)
	}

	applied, err := GetAppliedMigrationsOrdered(ctx, client)
	if err != nil {
		return RollbackPlan{}, err
	}
	if len(applied) == 0 {
		return RollbackPlan{}, surqlerrors.New(
			surqlerrors.ErrMigrationHistory,
			"no migrations have been applied",
		)
	}
	currentVersion := applied[len(applied)-1].Version

	// Collect files strictly greater than targetVersion and <= currentVersion.
	// The plan reverses the newest-first to match SurrealDB rollback semantics.
	toRollback := make([]Migration, 0)
	for _, m := range migrations {
		if m.Version > targetVersion && m.Version <= currentVersion {
			toRollback = append(toRollback, m)
		}
	}
	sort.SliceStable(toRollback, func(i, j int) bool {
		return toRollback[i].Version > toRollback[j].Version
	})

	issues := make([]RollbackIssue, 0)
	overall := RollbackSafetySafe
	for _, m := range toRollback {
		migrationIssues := analyzeMigrationSafety(m)
		issues = append(issues, migrationIssues...)
		for _, iss := range migrationIssues {
			if iss.Safety.rank() > overall.rank() {
				overall = iss.Safety
			}
		}
	}

	return RollbackPlan{
		FromVersion:      currentVersion,
		ToVersion:        targetVersion,
		Migrations:       toRollback,
		OverallSafety:    overall,
		Issues:           issues,
		RequiresApproval: overall != RollbackSafetySafe,
	}, nil
}

// ExecuteRollback runs every migration in plan in reverse-version order,
// using ExecuteMigration per step. A failure short-circuits the run; the
// returned RollbackResult reflects the partial progress.
//
// Unsafe plans are not blocked here — CLI callers are responsible for
// prompting the operator. Downstream tooling that wishes to enforce the
// RequiresApproval flag should do so before invoking this function.
func ExecuteRollback(
	ctx context.Context,
	client *connection.DatabaseClient,
	plan RollbackPlan,
) (RollbackResult, error) {
	if client == nil {
		return RollbackResult{Plan: plan}, surqlerrors.New(surqlerrors.ErrValidation, "client cannot be nil")
	}

	start := time.Now()
	errs := make([]string, 0)
	rolledBack := 0

	for _, m := range plan.Migrations {
		if _, err := ExecuteMigration(ctx, client, m, MigrationDirectionDown); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", m.Version, err))
			return RollbackResult{
				Plan:            plan,
				Success:         false,
				ActualDuration:  time.Since(start),
				RolledBackCount: rolledBack,
				Errors:          errs,
			}, err
		}
		rolledBack++
	}

	return RollbackResult{
		Plan:            plan,
		Success:         true,
		ActualDuration:  time.Since(start),
		RolledBackCount: rolledBack,
	}, nil
}

// PlanRollbackToVersion is a convenience alias for CreateRollbackPlan. The
// API separates "build the plan" and "execute the plan" so callers can
// display safety issues between the two steps.
func PlanRollbackToVersion(
	ctx context.Context,
	client *connection.DatabaseClient,
	migrationsDir string,
	targetVersion string,
) (RollbackPlan, error) {
	return CreateRollbackPlan(ctx, client, migrationsDir, targetVersion)
}

// ---------------------------------------------------------------------------
// Safety analysis
// ---------------------------------------------------------------------------

// dropTableRe matches DROP TABLE or REMOVE TABLE statements (case
// insensitive). We capture the table identifier to surface it in the issue.
var dropTableRe = regexp.MustCompile(`(?i)\b(?:DROP|REMOVE)\s+TABLE(?:\s+IF\s+EXISTS)?\s+([A-Za-z_][A-Za-z0-9_]*)`)

// dropFieldRe matches DROP FIELD / REMOVE FIELD statements. Captures the
// field identifier, which may be in `table.field` form or qualified via
// `ON TABLE` later in the statement.
var dropFieldRe = regexp.MustCompile(`(?i)\b(?:DROP|REMOVE)\s+FIELD(?:\s+IF\s+EXISTS)?\s+([A-Za-z_][A-Za-z0-9_\.]*)`)

// dropIndexRe matches DROP INDEX / REMOVE INDEX statements.
var dropIndexRe = regexp.MustCompile(`(?i)\b(?:DROP|REMOVE)\s+INDEX`)

// alterTypeRe matches ALTER ... TYPE statements (best-effort). Surreal's
// actual syntax uses DEFINE FIELD … TYPE to redefine, so we also detect
// DEFINE FIELD … TYPE emitted as part of a down migration.
var alterTypeRe = regexp.MustCompile(`(?i)\bALTER\b.*\bTYPE\b`)

// redefineFieldRe matches DEFINE FIELD … TYPE statements (which, emitted
// as a down migration, signal a field type change being reverted).
var redefineFieldRe = regexp.MustCompile(`(?i)\bDEFINE\s+FIELD\b.*\bTYPE\b`)

// analyzeMigrationSafety inspects the down SQL of a migration and returns
// the safety issues it produces.
func analyzeMigrationSafety(m Migration) []RollbackIssue {
	issues := make([]RollbackIssue, 0)
	if len(m.DownStatements) == 0 {
		issues = append(issues, RollbackIssue{
			Safety:         RollbackSafetyDanger,
			Migration:      m.Version,
			Description:    "migration has no rollback statements",
			Recommendation: "provide a '-- @down' section to enable safe rollback",
		})
		return issues
	}

	for _, stmt := range m.DownStatements {
		normalised := strings.TrimSpace(stmt)
		if normalised == "" {
			continue
		}
		// Table drops: destructive.
		if match := dropTableRe.FindStringSubmatch(normalised); match != nil {
			table := match[1]
			issues = append(issues, RollbackIssue{
				Safety:         RollbackSafetyDanger,
				Migration:      m.Version,
				Description:    fmt.Sprintf("dropping table %q", table),
				AffectedData:   fmt.Sprintf("all records in table %s", table),
				Recommendation: "export table data before rollback",
			})
			continue
		}
		// Field drops: data loss.
		if match := dropFieldRe.FindStringSubmatch(normalised); match != nil {
			field := match[1]
			issues = append(issues, RollbackIssue{
				Safety:         RollbackSafetyWarning,
				Migration:      m.Version,
				Description:    fmt.Sprintf("dropping field %q", field),
				AffectedData:   fmt.Sprintf("field data in %s", field),
				Recommendation: "backup affected field data",
			})
			continue
		}
		// Type changes (ALTER … TYPE or re-DEFINE FIELD … TYPE): data loss.
		if alterTypeRe.MatchString(normalised) || redefineFieldRe.MatchString(normalised) {
			issues = append(issues, RollbackIssue{
				Safety:         RollbackSafetyWarning,
				Migration:      m.Version,
				Description:    "altering field type may cause data conversion issues",
				Recommendation: "review data compatibility before rollback",
			})
			continue
		}
		// Index drops are safe but noteworthy.
		if dropIndexRe.MatchString(normalised) {
			continue
		}
	}
	return issues
}

// versionExists reports whether migrations contains the given version.
func versionExists(migrations []Migration, version string) bool {
	for _, m := range migrations {
		if m.Version == version {
			return true
		}
	}
	return false
}

// migrationsNewerThan returns the subset of migrations with Version >
// targetVersion, sorted ascending.
func migrationsNewerThan(migrations []Migration, targetVersion string) []Migration {
	out := make([]Migration, 0)
	for _, m := range migrations {
		if m.Version > targetVersion {
			out = append(out, m)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Version < out[j].Version
	})
	return out
}
