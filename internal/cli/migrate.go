package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Oneiriq/surql-go/pkg/surql/migration"
)

// newMigrateCommand wires the `surql migrate` command group.
func newMigrateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Database migration commands",
	}
	cmd.AddCommand(
		newMigrateUpCommand(),
		newMigrateDownCommand(),
		newMigrateStatusCommand(),
		newMigrateHistoryCommand(),
		newMigrateCreateCommand(),
		newMigrateValidateCommand(),
		newMigrateGenerateCommand(),
		newMigrateSquashCommand(),
	)
	return cmd
}

// ---------------------------------------------------------------------------
// migrate up
// ---------------------------------------------------------------------------

func newMigrateUpCommand() *cobra.Command {
	var (
		target string
		steps  uint32
		dryRun bool
		dir    string
	)
	cmd := &cobra.Command{
		Use:   "up",
		Short: "Apply pending migrations",
		RunE: func(c *cobra.Command, _ []string) error {
			rc := rootFromCmd(c)
			return runMigrateUp(c.Context(), rc, migrationsDir(rc, dir), target, steps, dryRun)
		},
	}
	cmd.Flags().StringVar(&target, "target", "", "stop applying after this version (inclusive)")
	cmd.Flags().Uint32VarP(&steps, "steps", "n", 0, "apply only N migrations (0 = all)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview migrations without applying")
	cmd.Flags().StringVarP(&dir, "directory", "d", "", "migrations directory (overrides settings)")
	return cmd
}

// runMigrateUp executes the up-migration flow: resolve pending, optionally
// clamp to target/steps, and drive the migration executor.
func runMigrateUp(ctx context.Context, rc *rootContext, dir, target string, steps uint32, dryRun bool) error {
	client, cleanup, err := newConnectedClient(ctx, rc)
	if err != nil {
		rc.Printer.Errorf("connection failed: %v", err)
		return err
	}
	defer cleanup()

	pending, err := migration.GetPendingMigrations(ctx, client, dir)
	if err != nil {
		return err
	}
	if target != "" {
		pending = clampPendingToTarget(pending, target)
	}
	if steps > 0 && uint32(len(pending)) > steps {
		pending = pending[:steps]
	}

	if len(pending) == 0 {
		rc.Printer.Successf("no pending migrations")
		return nil
	}

	if dryRun {
		rc.Printer.Infof("dry-run: %d migration(s) would be applied", len(pending))
		renderMigrationList(rc.Printer, pending)
		return nil
	}

	plan := migration.MigrationPlan{Migrations: pending, Direction: migration.MigrationDirectionUp}
	statuses, err := migration.ExecuteMigrationPlan(ctx, client, plan)
	renderMigrationStatuses(rc.Printer, statuses)
	if err != nil {
		rc.Printer.Errorf("migration failed: %v", err)
		return err
	}
	rc.Printer.Successf("applied %d migration(s)", len(statuses))
	return nil
}

// clampPendingToTarget returns the prefix of pending up to (and including)
// the migration whose version matches target. If target is not present the
// whole slice is returned unchanged (caller still sees at least the
// pending set; the executor will abort for invalid targets).
func clampPendingToTarget(pending []migration.Migration, target string) []migration.Migration {
	for i, m := range pending {
		if m.Version == target {
			return pending[:i+1]
		}
	}
	return pending
}

// ---------------------------------------------------------------------------
// migrate down
// ---------------------------------------------------------------------------

func newMigrateDownCommand() *cobra.Command {
	var (
		target string
		steps  uint32
		dryRun bool
		dir    string
	)
	cmd := &cobra.Command{
		Use:   "down",
		Short: "Rollback applied migrations",
		RunE: func(c *cobra.Command, _ []string) error {
			rc := rootFromCmd(c)
			return runMigrateDown(c.Context(), rc, migrationsDir(rc, dir), target, steps, dryRun)
		},
	}
	cmd.Flags().StringVar(&target, "target", "", "rollback down to (but not including) this version")
	cmd.Flags().Uint32VarP(&steps, "steps", "n", 1, "number of migrations to rollback (ignored when --target set)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview rollback without applying")
	cmd.Flags().StringVarP(&dir, "directory", "d", "", "migrations directory (overrides settings)")
	return cmd
}

// runMigrateDown handles both steps-based and target-based rollbacks.
func runMigrateDown(ctx context.Context, rc *rootContext, dir, target string, steps uint32, dryRun bool) error {
	client, cleanup, err := newConnectedClient(ctx, rc)
	if err != nil {
		rc.Printer.Errorf("connection failed: %v", err)
		return err
	}
	defer cleanup()

	if target != "" {
		plan, err := migration.PlanRollbackToVersion(ctx, client, dir, target)
		if err != nil {
			return err
		}
		if plan.MigrationCount() == 0 {
			rc.Printer.Successf("already at target version %s", target)
			return nil
		}
		if dryRun {
			rc.Printer.Infof("dry-run: %d migration(s) would be rolled back", plan.MigrationCount())
			renderMigrationList(rc.Printer, plan.Migrations)
			if !plan.IsSafe() {
				rc.Printer.Warnf("rollback is not fully safe (%s)", plan.OverallSafety)
			}
			return nil
		}
		res, err := migration.ExecuteRollback(ctx, client, plan)
		if err != nil {
			rc.Printer.Errorf("rollback failed: %v", err)
			return err
		}
		rc.Printer.Successf("rolled back %d migration(s)", res.RolledBackCount)
		return nil
	}

	if steps == 0 {
		return newUsageError("--steps must be >= 1 when --target is not provided")
	}
	if dryRun {
		applied, err := migration.GetAppliedMigrationsOrdered(ctx, client)
		if err != nil {
			return err
		}
		start := 0
		if int(steps) < len(applied) {
			start = len(applied) - int(steps)
		}
		preview := applied[start:]
		rc.Printer.Infof("dry-run: %d migration(s) would be rolled back", len(preview))
		for i := len(preview) - 1; i >= 0; i-- {
			rc.Printer.Plainf("  - %s %s", preview[i].Version, preview[i].Description)
		}
		return nil
	}

	statuses, err := migration.MigrateDown(ctx, client, dir, steps)
	renderMigrationStatuses(rc.Printer, statuses)
	if err != nil {
		rc.Printer.Errorf("rollback failed: %v", err)
		return err
	}
	rc.Printer.Successf("rolled back %d migration(s)", len(statuses))
	return nil
}

// ---------------------------------------------------------------------------
// migrate status
// ---------------------------------------------------------------------------

func newMigrateStatusCommand() *cobra.Command {
	var dir string
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show migration status",
		RunE: func(c *cobra.Command, _ []string) error {
			rc := rootFromCmd(c)
			return runMigrateStatus(c.Context(), rc, migrationsDir(rc, dir))
		},
	}
	cmd.Flags().StringVarP(&dir, "directory", "d", "", "migrations directory (overrides settings)")
	return cmd
}

func runMigrateStatus(ctx context.Context, rc *rootContext, dir string) error {
	client, cleanup, err := newConnectedClient(ctx, rc)
	if err != nil {
		rc.Printer.Errorf("connection failed: %v", err)
		return err
	}
	defer cleanup()

	report, err := migration.GetMigrationStatus(ctx, client, dir)
	if err != nil {
		return err
	}
	if rc.Flags.JSONOut {
		return rc.Printer.JSON(report)
	}
	rc.Printer.Section("Migration status")
	rc.Printer.Plainf("  total:   %d", report.Total)
	rc.Printer.Plainf("  applied: %d", report.Applied)
	rc.Printer.Plainf("  pending: %d", report.Pending)
	return nil
}

// ---------------------------------------------------------------------------
// migrate history
// ---------------------------------------------------------------------------

func newMigrateHistoryCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "history",
		Short: "Show applied migration history",
		RunE: func(c *cobra.Command, _ []string) error {
			rc := rootFromCmd(c)
			return runMigrateHistory(c.Context(), rc)
		},
	}
	return cmd
}

func runMigrateHistory(ctx context.Context, rc *rootContext) error {
	client, cleanup, err := newConnectedClient(ctx, rc)
	if err != nil {
		rc.Printer.Errorf("connection failed: %v", err)
		return err
	}
	defer cleanup()

	entries, err := migration.GetMigrationHistory(ctx, client)
	if err != nil {
		return err
	}
	if rc.Flags.JSONOut {
		return rc.Printer.JSON(entries)
	}
	if len(entries) == 0 {
		rc.Printer.Infof("no migrations applied")
		return nil
	}
	rows := make([][]string, 0, len(entries))
	for _, e := range entries {
		execMs := "-"
		if e.ExecutionTimeMs != nil {
			execMs = fmt.Sprintf("%dms", *e.ExecutionTimeMs)
		}
		rows = append(rows, []string{
			e.Version,
			truncateString(e.Description, 48),
			e.AppliedAt.Format("2006-01-02 15:04:05"),
			execMs,
		})
	}
	rc.Printer.Table([]string{"Version", "Description", "Applied", "Execution"}, rows)
	return nil
}

// ---------------------------------------------------------------------------
// migrate create
// ---------------------------------------------------------------------------

func newMigrateCreateCommand() *cobra.Command {
	var (
		description string
		dir         string
	)
	cmd := &cobra.Command{
		Use:   "create <description>",
		Short: "Create a blank migration file",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			rc := rootFromCmd(c)
			if description == "" {
				description = strings.Join(args, " ")
			}
			m, err := migration.CreateBlankMigration(description, description, migrationsDir(rc, dir))
			if err != nil {
				return err
			}
			rc.Printer.Successf("created %s", m.Path)
			return nil
		},
	}
	cmd.Flags().StringVarP(&dir, "schema-dir", "d", "", "migrations directory (overrides settings)")
	return cmd
}

// ---------------------------------------------------------------------------
// migrate validate
// ---------------------------------------------------------------------------

func newMigrateValidateCommand() *cobra.Command {
	var dir string
	cmd := &cobra.Command{
		Use:   "validate [version]",
		Short: "Validate migration files",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			rc := rootFromCmd(c)
			issues, err := migration.ValidateMigrations(migrationsDir(rc, dir))
			if err != nil {
				return err
			}
			if len(args) > 0 {
				filter := args[0]
				filtered := make([]string, 0, len(issues))
				for _, iss := range issues {
					if strings.Contains(iss, filter) {
						filtered = append(filtered, iss)
					}
				}
				issues = filtered
			}
			if len(issues) == 0 {
				rc.Printer.Successf("migrations are valid")
				return nil
			}
			rc.Printer.Errorf("validation failed")
			for _, iss := range issues {
				rc.Printer.Plainf("  - %s", iss)
			}
			return fmt.Errorf("migration validation failed (%d issue(s))", len(issues))
		},
	}
	cmd.Flags().StringVarP(&dir, "directory", "d", "", "migrations directory (overrides settings)")
	return cmd
}

// ---------------------------------------------------------------------------
// migrate generate
// ---------------------------------------------------------------------------

func newMigrateGenerateCommand() *cobra.Command {
	var (
		from string
		to   string
		dir  string
	)
	cmd := &cobra.Command{
		Use:   "generate <description>",
		Short: "Generate a blank migration (schema diff not yet wired up)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			rc := rootFromCmd(c)
			description := strings.Join(args, " ")
			if from != "" || to != "" {
				rc.Printer.Warnf("--from / --to ignored: schema-diff generation requires a code registry (not yet wired into the CLI)")
			}
			m, err := migration.CreateBlankMigration(description, description, migrationsDir(rc, dir))
			if err != nil {
				return err
			}
			rc.Printer.Successf("generated %s", m.Path)
			rc.Printer.Infof("edit the file to populate the -- @up / -- @down sections")
			return nil
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "snapshot version to diff from (placeholder)")
	cmd.Flags().StringVar(&to, "to", "", "snapshot version to diff to (placeholder)")
	cmd.Flags().StringVarP(&dir, "directory", "d", "", "migrations directory (overrides settings)")
	return cmd
}

// ---------------------------------------------------------------------------
// migrate squash
// ---------------------------------------------------------------------------

func newMigrateSquashCommand() *cobra.Command {
	var (
		output string
		dryRun bool
		dir    string
	)
	cmd := &cobra.Command{
		Use:   "squash <from> <to>",
		Short: "Squash migrations in [from,to] into a single file",
		Args:  cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			rc := rootFromCmd(c)
			from, to := args[0], args[1]
			opts := migration.SquashOptions{OutputPath: output, DryRun: dryRun}
			res, err := migration.SquashMigrationsWithOptions(c.Context(), migrationsDir(rc, dir), from, to, opts)
			if err != nil {
				return err
			}
			if rc.Flags.JSONOut {
				return rc.Printer.JSON(res)
			}
			if dryRun {
				rc.Printer.Infof("dry-run: would squash %d migration(s) -> %s", res.OriginalCount, res.SquashedPath)
			} else {
				rc.Printer.Successf("squashed %d migration(s) into %s", res.OriginalCount, res.SquashedPath)
			}
			rc.Printer.Plainf("  statements:      %d", res.StatementCount)
			rc.Printer.Plainf("  optimizations:   %d", res.OptimizationsApplied)
			rc.Printer.Plainf("  warnings:        %d", len(res.Warnings))
			for _, w := range res.Warnings {
				rc.Printer.Warnf("%s [%s]: %s", w.Migration, w.Severity, w.Message)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "", "explicit output file (auto-named under directory if unset)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview without writing")
	cmd.Flags().StringVarP(&dir, "directory", "d", "", "migrations directory (overrides settings)")
	return cmd
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// renderMigrationList prints a bullet list of migrations. Used by dry-run
// previews where the caller has not yet executed anything.
func renderMigrationList(p *Printer, migrations []migration.Migration) {
	for _, m := range migrations {
		p.Plainf("  - %s %s", m.Version, m.Description)
	}
}

// renderMigrationStatuses prints one line per status using the state as a
// marker. Applied -> [ok], Failed -> [err], anything else -> [info].
func renderMigrationStatuses(p *Printer, statuses []migration.MigrationStatus) {
	for _, s := range statuses {
		switch s.State {
		case migration.MigrationStateApplied:
			p.Successf("%s %s", s.Migration.Version, s.Migration.Description)
		case migration.MigrationStateFailed:
			p.Errorf("%s %s: %s", s.Migration.Version, s.Migration.Description, s.Error)
		default:
			p.Infof("%s %s (%s)", s.Migration.Version, s.Migration.Description, s.State)
		}
	}
}

// parseUint32 parses s as an unsigned 32-bit integer. The helper is used
// by a handful of command-line flags that prefer explicit parsing over
// cobra's Uint32Var (so we can produce a usage-style error instead of a
// generic cobra error).
func parseUint32(s string) (uint32, error) {
	n, err := strconv.ParseUint(strings.TrimSpace(s), 10, 32)
	if err != nil {
		return 0, err
	}
	return uint32(n), nil
}
