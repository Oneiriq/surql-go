package migration

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	surqlerrors "github.com/albedosehen/surql-go/pkg/surql/errors"
)

// DriftSeverity categorises a single DriftIssue. The zero value
// (DriftSeverityInfo) is intentionally the least severe so that a freshly
// allocated DriftIssue with no explicit severity reads as informational.
type DriftSeverity string

// DriftSeverity values, ordered from least to most severe.
const (
	DriftSeverityInfo    DriftSeverity = "info"
	DriftSeverityWarning DriftSeverity = "warning"
	DriftSeverityError   DriftSeverity = "error"
)

// IsValid reports whether the severity is one of the defined constants.
func (s DriftSeverity) IsValid() bool {
	switch s {
	case DriftSeverityInfo, DriftSeverityWarning, DriftSeverityError:
		return true
	}
	return false
}

// String returns the serialised form of the severity.
func (s DriftSeverity) String() string { return string(s) }

// DriftIssue describes a single mismatch between a code-side schema snapshot
// and a recorded (migrated / last-applied) snapshot.
type DriftIssue struct {
	// Severity communicates how urgent the issue is. DROP / MODIFY on existing
	// tables surface as warnings; ADD is info; permissions modifications are
	// warnings (they can affect runtime auth). The executor can promote
	// warnings to errors via its own policy.
	Severity DriftSeverity `json:"severity"`
	// Operation is the DiffOperation that produced this issue.
	Operation DiffOperation `json:"operation"`
	// Table is the table (or edge) affected by the diff.
	Table string `json:"table"`
	// Field is the field affected, when relevant.
	Field string `json:"field,omitempty"`
	// Description is a human-readable summary (mirrors SchemaDiff.Description).
	Description string `json:"description"`
}

// DriftReport is the result of comparing a code-side schema snapshot against
// the most recently recorded snapshot. It is safe to serialise (JSON-friendly).
type DriftReport struct {
	// DriftDetected is true when at least one Issue was produced.
	DriftDetected bool `json:"drift_detected"`
	// Issues holds the per-diff breakdown in the order returned by DiffSchemas.
	Issues []DriftIssue `json:"issues,omitempty"`
	// SuggestedMigration is a CLI-style string the caller can print to guide
	// the user (e.g. `surql-go migrate generate -m "<desc>"`). Empty when no
	// drift was detected.
	SuggestedMigration string `json:"suggested_migration,omitempty"`
}

// CheckSchemaDriftFromSnapshots compares a code-side snapshot to a recorded
// one and produces a DriftReport.
//
// The recorded snapshot is treated as the "current" state (the DB side in
// DiffSchemas terminology) and the code snapshot as the desired state. A
// diff-generation failure is surfaced as ErrMigrationGeneration.
//
// The report's SuggestedMigration is a single-line CLI hint; callers are
// free to replace it with their preferred wording.
func CheckSchemaDriftFromSnapshots(code, recorded SchemaSnapshot) (DriftReport, error) {
	diffs, err := DiffSchemas(code, recorded)
	if err != nil {
		return DriftReport{}, surqlerrors.Wrapf(
			surqlerrors.ErrMigrationGeneration, err,
			"failed to diff snapshots for drift report",
		)
	}

	if len(diffs) == 0 {
		return DriftReport{DriftDetected: false}, nil
	}

	issues := make([]DriftIssue, 0, len(diffs))
	for _, d := range diffs {
		issues = append(issues, DriftIssue{
			Severity:    severityForOperation(d.Operation),
			Operation:   d.Operation,
			Table:       d.Table,
			Field:       d.Field,
			Description: d.Description,
		})
	}

	return DriftReport{
		DriftDetected:      true,
		Issues:             issues,
		SuggestedMigration: "surql-go migrate generate -m '<describe-your-change>'",
	}, nil
}

// severityForOperation maps each DiffOperation to a DriftSeverity.
//
// Additive operations (ADD_TABLE, ADD_FIELD, ADD_INDEX, ADD_EVENT) are info;
// destructive or modifying operations (DROP_*, MODIFY_*) are warnings because
// they may require data migration or backfill. A caller-defined policy can
// promote warnings to errors before failing a pre-commit hook.
func severityForOperation(op DiffOperation) DriftSeverity {
	switch op {
	case DiffOperationAddTable,
		DiffOperationAddField,
		DiffOperationAddIndex,
		DiffOperationAddEvent:
		return DriftSeverityInfo
	case DiffOperationDropTable,
		DiffOperationDropField,
		DiffOperationDropIndex,
		DiffOperationDropEvent,
		DiffOperationModifyField,
		DiffOperationModifyPermissions:
		return DriftSeverityWarning
	}
	return DriftSeverityInfo
}

// gitCommandRunner is the hook used to invoke git. Tests override this to
// avoid requiring a real git binary / temporary repositories; production
// always uses exec.CommandContext through runGitDiffCached.
var gitCommandRunner = runGitDiffCached

// GetStagedSchemaFiles returns paths to files currently staged in git that
// live under schemaDir.
//
// The function runs `git diff --cached --name-only --diff-filter=ACMR` in the
// repository containing schemaDir and filters the results to entries beneath
// schemaDir. Returned paths are repo-relative — the same form git prints —
// to keep them portable between callers. An empty or non-existent schemaDir
// returns an empty slice without invoking git.
//
// Errors are surfaced as ErrMigrationGeneration wrapping the underlying cause
// (missing git binary, non-zero exit, etc.) so the pre-commit caller can log
// a single context-rich message.
func GetStagedSchemaFiles(schemaDir string) ([]string, error) {
	return GetStagedSchemaFilesContext(context.Background(), schemaDir)
}

// GetStagedSchemaFilesContext is the context-aware variant of
// GetStagedSchemaFiles. It cancels the underlying git invocation when ctx is
// done.
func GetStagedSchemaFilesContext(ctx context.Context, schemaDir string) ([]string, error) {
	if strings.TrimSpace(schemaDir) == "" {
		return nil, surqlerrors.New(surqlerrors.ErrValidation,
			"schema directory path must not be empty")
	}

	absSchema, err := filepath.Abs(schemaDir)
	if err != nil {
		return nil, surqlerrors.Wrapf(surqlerrors.ErrValidation, err,
			"failed to resolve absolute path for %q", schemaDir)
	}
	absSchema = filepath.Clean(absSchema)

	out, err := gitCommandRunner(ctx, absSchema)
	if err != nil {
		return nil, err
	}

	// `git` emits repo-relative paths, newline-separated, using `/` on all
	// platforms. We determine the repo root from `git rev-parse` so we can
	// translate those back into absolute paths for the directory filter.
	repoRoot, err := runGitRevParseTopLevel(ctx, absSchema)
	if err != nil {
		return nil, err
	}
	repoRoot = filepath.Clean(repoRoot)

	// Reconcile symlinked temp-dir paths (e.g. macOS `/var` -> `/private/var`).
	// `git rev-parse --show-toplevel` always emits the resolved path; if the
	// caller handed us a pre-symlink path, `filepath.Rel` from absSchema to
	// repo-joined paths will disagree on the common prefix. Resolve both to
	// their canonical forms before filtering.
	if resolvedSchema, err := filepath.EvalSymlinks(absSchema); err == nil {
		absSchema = filepath.Clean(resolvedSchema)
	}
	if resolvedRoot, err := filepath.EvalSymlinks(repoRoot); err == nil {
		repoRoot = filepath.Clean(resolvedRoot)
	}

	staged := make([]string, 0)
	for _, raw := range strings.Split(strings.TrimSpace(out), "\n") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		// git always prints forward slashes; convert before joining.
		native := filepath.FromSlash(raw)
		abs := filepath.Join(repoRoot, native)
		if !isUnderDirectory(abs, absSchema) {
			continue
		}
		staged = append(staged, raw)
	}

	sort.Strings(staged)
	return staged, nil
}

// isUnderDirectory reports whether candidate sits at or beneath root. Both
// arguments are expected to be absolute and cleaned.
func isUnderDirectory(candidate, root string) bool {
	if candidate == root {
		return true
	}
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	// filepath.Rel returns a `..` prefix when candidate escapes root.
	if strings.HasPrefix(rel, "..") {
		return false
	}
	return true
}

// runGitDiffCached invokes `git diff --cached --name-only --diff-filter=ACMR`
// in cwd and returns stdout as a string. A non-zero exit code is wrapped as
// ErrMigrationGeneration so callers do not need to import os/exec.
func runGitDiffCached(ctx context.Context, cwd string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "diff", "--cached", "--name-only", "--diff-filter=ACMR")
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		return "", wrapGitError(err, "git diff --cached failed")
	}
	return string(out), nil
}

// runGitRevParseTopLevel resolves the repository root for cwd.
func runGitRevParseTopLevel(ctx context.Context, cwd string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel")
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		return "", wrapGitError(err, "git rev-parse --show-toplevel failed")
	}
	return strings.TrimSpace(string(out)), nil
}

// wrapGitError normalises an os/exec error into a SurqlError. ExitError
// carries stderr on its own struct; we surface it verbatim so callers get a
// single actionable message.
func wrapGitError(err error, reason string) error {
	if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
		return surqlerrors.Wrapf(surqlerrors.ErrMigrationGeneration, err,
			"%s: %s", reason, strings.TrimSpace(string(ee.Stderr)))
	}
	return surqlerrors.Wrapf(surqlerrors.ErrMigrationGeneration, err, "%s", reason)
}

// GeneratePreCommitConfig returns a .pre-commit-config.yaml snippet that
// invokes `surql-go migrate check` against schemaPath.
//
// When failOnDrift is true the emitted entry command passes
// `--fail-on-drift`. The returned string is a multi-line YAML fragment
// terminated with a newline so it can be concatenated into an existing
// configuration file.
func GeneratePreCommitConfig(schemaPath string, failOnDrift bool) string {
	// Mirror surql-py defaults: empty path falls back to 'schemas/' and the
	// hook name is stable across runs.
	if strings.TrimSpace(schemaPath) == "" {
		schemaPath = "schemas/"
	}

	failFlag := ""
	if failOnDrift {
		failFlag = " --fail-on-drift"
	}

	return fmt.Sprintf(
		"repos:\n"+
			"  - repo: local\n"+
			"    hooks:\n"+
			"      - id: surql-schema-check\n"+
			"        name: Check schema migrations\n"+
			"        entry: surql-go migrate check --schema %s%s\n"+
			"        language: system\n"+
			"        pass_filenames: false\n",
		schemaPath, failFlag,
	)
}
