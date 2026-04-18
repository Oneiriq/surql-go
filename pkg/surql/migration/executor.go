package migration

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/albedosehen/surql-go/pkg/surql/connection"
	surqlerrors "github.com/albedosehen/surql-go/pkg/surql/errors"
)

// ExecuteOptions tunes MigrateUp / MigrateDown behaviour.
//
// Zero-value ExecuteOptions is meaningful: Steps=0 applies every pending
// migration, DryRun=false runs statements, FailOnValidation=true aborts on
// validation errors (mirroring the Python port).
type ExecuteOptions struct {
	// Steps limits the number of migrations applied or rolled back. Zero
	// means "no limit" (apply all pending / roll back every applied).
	Steps uint32
	// DryRun populates the returned status slice without executing
	// statements. Used by CLI `plan` commands.
	DryRun bool
	// FailOnValidation controls whether validation errors abort the run
	// before the first statement executes. Default true.
	FailOnValidation bool
}

// ExecuteOption is a functional option applied to ExecuteOptions.
type ExecuteOption func(*ExecuteOptions)

// WithSteps limits the number of migrations executed.
func WithSteps(steps uint32) ExecuteOption {
	return func(o *ExecuteOptions) { o.Steps = steps }
}

// WithDryRun toggles dry-run mode.
func WithDryRun(dry bool) ExecuteOption {
	return func(o *ExecuteOptions) { o.DryRun = dry }
}

// WithFailOnValidation configures whether validation errors abort execution.
func WithFailOnValidation(fail bool) ExecuteOption {
	return func(o *ExecuteOptions) { o.FailOnValidation = fail }
}

// defaultExecuteOptions returns the defaults used when no option is passed.
func defaultExecuteOptions() ExecuteOptions {
	return ExecuteOptions{FailOnValidation: true}
}

// MigrationStatusReport summarises the state of a directory of migrations
// against the database.
type MigrationStatusReport struct {
	Total   int `json:"total"`
	Applied int `json:"applied"`
	Pending int `json:"pending"`
}

// ExecuteMigration runs a single migration in the given direction inside a
// SurrealDB interactive transaction.
//
// On success the history table is updated (record for UP, delete for DOWN)
// and the returned MigrationStatus carries the applied timestamp and
// execution state. On failure the transaction is rolled back and the error
// is surfaced to the caller; the returned status still reports the failure
// for logging purposes.
func ExecuteMigration(
	ctx context.Context,
	client *connection.DatabaseClient,
	m Migration,
	dir MigrationDirection,
) (MigrationStatus, error) {
	if client == nil {
		return MigrationStatus{Migration: m, State: MigrationStateFailed, Error: "client cannot be nil"},
			surqlerrors.New(surqlerrors.ErrValidation, "client cannot be nil")
	}
	if !dir.IsValid() {
		return MigrationStatus{Migration: m, State: MigrationStateFailed, Error: "invalid migration direction"},
			surqlerrors.Newf(surqlerrors.ErrValidation, "invalid migration direction %q", dir)
	}

	statements := m.UpStatements
	if dir == MigrationDirectionDown {
		statements = m.DownStatements
	}

	start := time.Now()

	tx, err := client.Begin(ctx)
	if err != nil {
		return MigrationStatus{
				Migration: m,
				State:     MigrationStateFailed,
				Error:     err.Error(),
			}, surqlerrors.Wrapf(
				surqlerrors.ErrMigrationExecution, err,
				"failed to begin transaction for migration %q", m.Version,
			)
	}

	for i, stmt := range statements {
		if _, execErr := tx.Execute(ctx, stmt); execErr != nil {
			_ = tx.Rollback(ctx)
			return MigrationStatus{
					Migration: m,
					State:     MigrationStateFailed,
					Error:     fmt.Sprintf("statement %d: %v", i, execErr),
				}, surqlerrors.Wrapf(
					surqlerrors.ErrMigrationExecution, execErr,
					"migration %q: failed to execute statement %d", m.Version, i,
				)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return MigrationStatus{
				Migration: m,
				State:     MigrationStateFailed,
				Error:     err.Error(),
			}, surqlerrors.Wrapf(
				surqlerrors.ErrMigrationExecution, err,
				"migration %q: commit failed", m.Version,
			)
	}

	executionMs := time.Since(start).Milliseconds()
	appliedAt := time.Now().UTC()

	switch dir {
	case MigrationDirectionUp:
		entry := MigrationHistory{
			Version:         m.Version,
			Description:     m.Description,
			AppliedAt:       appliedAt,
			Checksum:        m.Checksum,
			ExecutionTimeMs: &executionMs,
		}
		if err := RecordMigration(ctx, client, entry); err != nil {
			return MigrationStatus{
					Migration: m,
					State:     MigrationStateFailed,
					Error:     err.Error(),
				}, surqlerrors.Wrapf(
					surqlerrors.ErrMigrationExecution, err,
					"migration %q applied but history recording failed", m.Version,
				)
		}
	case MigrationDirectionDown:
		if err := RemoveMigrationRecord(ctx, client, m.Version); err != nil {
			return MigrationStatus{
					Migration: m,
					State:     MigrationStateFailed,
					Error:     err.Error(),
				}, surqlerrors.Wrapf(
					surqlerrors.ErrMigrationExecution, err,
					"migration %q rolled back but history removal failed", m.Version,
				)
		}
	}

	return MigrationStatus{
		Migration: m,
		State:     MigrationStateApplied,
		AppliedAt: &appliedAt,
	}, nil
}

// MigrateUp applies pending migrations from migrationsDir.
//
// The migrations are discovered via DiscoverMigrations, filtered against the
// history table, and executed in ascending version order. Returns the
// per-migration MigrationStatus slice; execution halts on the first
// failure.
func MigrateUp(
	ctx context.Context,
	client *connection.DatabaseClient,
	migrationsDir string,
	opts ...ExecuteOption,
) ([]MigrationStatus, error) {
	if client == nil {
		return nil, surqlerrors.New(surqlerrors.ErrValidation, "client cannot be nil")
	}
	options := defaultExecuteOptions()
	for _, opt := range opts {
		opt(&options)
	}

	migrations, err := DiscoverMigrations(migrationsDir)
	if err != nil {
		return nil, err
	}

	if options.FailOnValidation {
		if issues, valErr := validateMigrationSet(migrations); valErr != nil {
			return nil, valErr
		} else if len(issues) > 0 {
			return nil, surqlerrors.Newf(
				surqlerrors.ErrMigrationExecution,
				"migration validation failed: %s", strings.Join(issues, "; "),
			)
		}
	}

	pending, err := pendingMigrations(ctx, client, migrations)
	if err != nil {
		return nil, err
	}
	if options.Steps > 0 && int(options.Steps) < len(pending) {
		pending = pending[:options.Steps]
	}

	statuses := make([]MigrationStatus, 0, len(pending))
	for _, m := range pending {
		if options.DryRun {
			statuses = append(statuses, MigrationStatus{Migration: m, State: MigrationStatePending})
			continue
		}
		status, err := ExecuteMigration(ctx, client, m, MigrationDirectionUp)
		statuses = append(statuses, status)
		if err != nil {
			return statuses, err
		}
	}
	return statuses, nil
}

// MigrateDown rolls back the most recently applied migrations.
//
// steps must be >= 1 (matching surql-py's requirement). The target
// migrations are resolved against both the on-disk directory and the
// history table — a migration file present on disk but absent from history
// is a no-op; one present in history but missing on disk surfaces as
// ErrMigrationExecution with a descriptive reason.
func MigrateDown(
	ctx context.Context,
	client *connection.DatabaseClient,
	migrationsDir string,
	steps uint32,
) ([]MigrationStatus, error) {
	if client == nil {
		return nil, surqlerrors.New(surqlerrors.ErrValidation, "client cannot be nil")
	}
	if steps == 0 {
		return nil, surqlerrors.New(surqlerrors.ErrValidation, "rollback steps must be >= 1")
	}

	migrations, err := DiscoverMigrations(migrationsDir)
	if err != nil {
		return nil, err
	}
	diskByVersion := indexByVersion(migrations)

	applied, err := GetAppliedMigrationsOrdered(ctx, client)
	if err != nil {
		return nil, err
	}
	if len(applied) == 0 {
		return []MigrationStatus{}, nil
	}

	// Take the last `steps` applied, execute in reverse order (newest first).
	start := 0
	if int(steps) < len(applied) {
		start = len(applied) - int(steps)
	}
	target := applied[start:]

	statuses := make([]MigrationStatus, 0, len(target))
	for i := len(target) - 1; i >= 0; i-- {
		entry := target[i]
		disk, ok := diskByVersion[entry.Version]
		if !ok {
			return statuses, surqlerrors.Newf(
				surqlerrors.ErrMigrationExecution,
				"cannot rollback %q: migration file not found in %q",
				entry.Version, migrationsDir,
			)
		}
		status, err := ExecuteMigration(ctx, client, disk, MigrationDirectionDown)
		statuses = append(statuses, status)
		if err != nil {
			return statuses, err
		}
	}
	return statuses, nil
}

// GetPendingMigrations returns the subset of migrations in migrationsDir
// that have not yet been applied, sorted by version.
func GetPendingMigrations(
	ctx context.Context,
	client *connection.DatabaseClient,
	migrationsDir string,
) ([]Migration, error) {
	if client == nil {
		return nil, surqlerrors.New(surqlerrors.ErrValidation, "client cannot be nil")
	}
	migrations, err := DiscoverMigrations(migrationsDir)
	if err != nil {
		return nil, err
	}
	return pendingMigrations(ctx, client, migrations)
}

// GetAppliedMigrationsOrdered returns the history entries sorted by
// AppliedAt ascending. Tie-broken by version for stability.
func GetAppliedMigrationsOrdered(
	ctx context.Context,
	client *connection.DatabaseClient,
) ([]MigrationHistory, error) {
	entries, err := GetAppliedMigrations(ctx, client)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].AppliedAt.Equal(entries[j].AppliedAt) {
			return entries[i].Version < entries[j].Version
		}
		return entries[i].AppliedAt.Before(entries[j].AppliedAt)
	})
	return entries, nil
}

// GetMigrationStatus summarises a migrations directory against the database
// as a single tally: total / applied / pending.
func GetMigrationStatus(
	ctx context.Context,
	client *connection.DatabaseClient,
	migrationsDir string,
) (MigrationStatusReport, error) {
	if client == nil {
		return MigrationStatusReport{}, surqlerrors.New(surqlerrors.ErrValidation, "client cannot be nil")
	}
	migrations, err := DiscoverMigrations(migrationsDir)
	if err != nil {
		return MigrationStatusReport{}, err
	}
	applied, err := GetAppliedMigrations(ctx, client)
	if err != nil {
		return MigrationStatusReport{}, err
	}
	appliedSet := make(map[string]struct{}, len(applied))
	for _, a := range applied {
		appliedSet[a.Version] = struct{}{}
	}
	appliedCount := 0
	for _, m := range migrations {
		if _, ok := appliedSet[m.Version]; ok {
			appliedCount++
		}
	}
	return MigrationStatusReport{
		Total:   len(migrations),
		Applied: appliedCount,
		Pending: len(migrations) - appliedCount,
	}, nil
}

// CreateMigrationPlan produces a MigrationPlan describing the forward
// migrations that would run against the database. The returned plan has
// direction = MigrationDirectionUp.
func CreateMigrationPlan(
	ctx context.Context,
	client *connection.DatabaseClient,
	migrationsDir string,
) (MigrationPlan, error) {
	pending, err := GetPendingMigrations(ctx, client, migrationsDir)
	if err != nil {
		return MigrationPlan{}, err
	}
	return MigrationPlan{
		Migrations: pending,
		Direction:  MigrationDirectionUp,
	}, nil
}

// ExecuteMigrationPlan runs every migration in plan in the plan's
// direction. Down plans are iterated in reverse order to match the LIFO
// rollback semantics of MigrateDown.
func ExecuteMigrationPlan(
	ctx context.Context,
	client *connection.DatabaseClient,
	plan MigrationPlan,
) ([]MigrationStatus, error) {
	if client == nil {
		return nil, surqlerrors.New(surqlerrors.ErrValidation, "client cannot be nil")
	}
	if !plan.Direction.IsValid() {
		return nil, surqlerrors.Newf(surqlerrors.ErrValidation, "invalid plan direction %q", plan.Direction)
	}
	if plan.IsEmpty() {
		return []MigrationStatus{}, nil
	}

	ordered := append([]Migration(nil), plan.Migrations...)
	if plan.Direction == MigrationDirectionDown {
		// Reverse the slice in-place; rollback must execute newest-first.
		for i, j := 0, len(ordered)-1; i < j; i, j = i+1, j-1 {
			ordered[i], ordered[j] = ordered[j], ordered[i]
		}
	}

	statuses := make([]MigrationStatus, 0, len(ordered))
	for _, m := range ordered {
		status, err := ExecuteMigration(ctx, client, m, plan.Direction)
		statuses = append(statuses, status)
		if err != nil {
			return statuses, err
		}
	}
	return statuses, nil
}

// ValidateMigrations loads every migration in migrationsDir and returns a
// list of human-readable validation errors (duplicate versions, missing
// dependencies, missing up/down blocks). An empty slice means the set is
// valid. Discovery failures are surfaced as errors.
func ValidateMigrations(migrationsDir string) ([]string, error) {
	migrations, err := DiscoverMigrations(migrationsDir)
	if err != nil {
		return nil, err
	}
	issues, err := validateMigrationSet(migrations)
	if err != nil {
		return nil, err
	}
	return issues, nil
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// pendingMigrations returns every migration whose version is not in the
// history table, sorted by version.
func pendingMigrations(
	ctx context.Context,
	client *connection.DatabaseClient,
	migrations []Migration,
) ([]Migration, error) {
	applied, err := GetAppliedMigrations(ctx, client)
	if err != nil {
		return nil, err
	}
	appliedSet := make(map[string]struct{}, len(applied))
	for _, a := range applied {
		appliedSet[a.Version] = struct{}{}
	}
	pending := make([]Migration, 0, len(migrations))
	for _, m := range migrations {
		if _, ok := appliedSet[m.Version]; ok {
			continue
		}
		pending = append(pending, m)
	}
	sort.SliceStable(pending, func(i, j int) bool {
		return pending[i].Version < pending[j].Version
	})
	return pending, nil
}

// indexByVersion returns the migrations keyed by their Version string.
func indexByVersion(migrations []Migration) map[string]Migration {
	out := make(map[string]Migration, len(migrations))
	for _, m := range migrations {
		out[m.Version] = m
	}
	return out
}

// validateMigrationSet mirrors surql-py validate_migrations. It never returns
// a non-nil error today, but keeps the signature symmetric with other
// validators in the package in case future checks require IO.
func validateMigrationSet(migrations []Migration) ([]string, error) {
	var issues []string

	counts := make(map[string]int, len(migrations))
	for _, m := range migrations {
		counts[m.Version]++
	}
	duplicates := make([]string, 0)
	for v, c := range counts {
		if c > 1 {
			duplicates = append(duplicates, v)
		}
	}
	if len(duplicates) > 0 {
		sort.Strings(duplicates)
		issues = append(issues,
			fmt.Sprintf("duplicate migration versions: %s", strings.Join(duplicates, ", ")),
		)
	}

	versionSet := make(map[string]struct{}, len(migrations))
	for _, m := range migrations {
		versionSet[m.Version] = struct{}{}
	}
	for _, m := range migrations {
		for _, dep := range m.DependsOn {
			if _, ok := versionSet[dep]; !ok {
				issues = append(issues,
					fmt.Sprintf("migration %s depends on missing migration %s", m.Version, dep),
				)
			}
		}
		if len(m.UpStatements) == 0 {
			issues = append(issues,
				fmt.Sprintf("migration %s has no up statements", m.Version),
			)
		}
	}

	sort.Strings(issues)
	return issues, nil
}
