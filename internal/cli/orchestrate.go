package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Oneiriq/surql-go/pkg/surql/migration"
	"github.com/Oneiriq/surql-go/pkg/surql/orchestration"
)

// newOrchestrateCommand wires `surql orchestrate`.
func newOrchestrateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "orchestrate",
		Aliases: []string{"orch"},
		Short:   "Multi-database migration orchestration",
	}
	cmd.AddCommand(
		newOrchestrateDeployCommand(),
		newOrchestrateStatusCommand(),
		newOrchestrateValidateCommand(),
	)
	return cmd
}

// ---------------------------------------------------------------------------
// orchestrate deploy
// ---------------------------------------------------------------------------

func newOrchestrateDeployCommand() *cobra.Command {
	var (
		plan         string
		envCSV       string
		strategyName string
		batchSize    int
		canaryPct    float64
		maxConc      int
		dryRun       bool
		skipHealth   bool
		noRollback   bool
		dir          string
	)
	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Deploy migrations across environments",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			rc := rootFromCmd(c)
			if plan == "" {
				return newUsageError("--plan is required (path to environments registry JSON)")
			}
			if envCSV == "" {
				return newUsageError("--environments is required (comma-separated env names)")
			}
			envs := splitCSVArg(envCSV)
			if len(envs) == 0 {
				return newUsageError("--environments produced zero entries")
			}
			return runOrchestrateDeploy(c.Context(), rc, plan, envs, strategyName,
				batchSize, canaryPct, maxConc, dryRun, skipHealth, noRollback,
				migrationsDir(rc, dir))
		},
	}
	cmd.Flags().StringVar(&plan, "plan", "environments.json", "path to environments registry JSON")
	cmd.Flags().StringVarP(&envCSV, "environments", "e", "", "comma-separated environment names")
	cmd.Flags().StringVar(&strategyName, "strategy", "sequential", "deployment strategy: sequential | parallel | rolling | canary")
	cmd.Flags().IntVar(&batchSize, "batch-size", 1, "batch size for rolling strategy")
	cmd.Flags().Float64Var(&canaryPct, "canary-percent", 10.0, "canary percentage for canary strategy")
	cmd.Flags().IntVar(&maxConc, "max-concurrent", 5, "max concurrent deploys for parallel strategy")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "simulate deployment without executing")
	cmd.Flags().BoolVar(&skipHealth, "skip-health-check", false, "skip pre-deploy health probes")
	cmd.Flags().BoolVar(&noRollback, "no-rollback", false, "disable auto-rollback on partial failure")
	cmd.Flags().StringVarP(&dir, "directory", "d", "", "migrations directory (overrides settings)")
	return cmd
}

// runOrchestrateDeploy discovers migrations, loads the environment
// registry, and runs the selected strategy against the named environments.
func runOrchestrateDeploy(
	ctx context.Context,
	rc *rootContext,
	planPath string,
	envs []string,
	strategyName string,
	batchSize int,
	canaryPct float64,
	maxConc int,
	dryRun bool,
	skipHealth bool,
	noRollback bool,
	migrationsDir string,
) error {
	registry, err := orchestration.LoadRegistryFromFile(planPath)
	if err != nil {
		return fmt.Errorf("load plan %s: %w", planPath, err)
	}
	migrations, err := migration.DiscoverMigrations(migrationsDir)
	if err != nil {
		return err
	}
	if len(migrations) == 0 {
		rc.Printer.Warnf("no migrations found under %s", migrationsDir)
	}

	verify := !skipHealth
	rollback := !noRollback
	opts := &orchestration.DeployOptions{
		Strategy:         orchestration.StrategyName(strategyName),
		BatchSize:        batchSize,
		CanaryPercentage: canaryPct,
		MaxConcurrent:    maxConc,
		DryRun:           dryRun,
		VerifyHealth:     &verify,
		AutoRollback:     &rollback,
	}

	rc.Printer.Infof("deploying %d migration(s) to %d environment(s) via %s",
		len(migrations), len(envs), strategyName)
	if dryRun {
		rc.Printer.Warnf("dry-run mode: no migrations will be applied")
	}

	coordinator := orchestration.NewMigrationCoordinator(registry)
	results, err := coordinator.DeployToEnvironments(ctx, envs, migrations, opts)
	renderDeploymentResults(rc, results)
	if err != nil {
		rc.Printer.Errorf("deploy failed: %v", err)
		return err
	}

	failures := 0
	for _, r := range results {
		if r.Status == orchestration.DeploymentStatusFailed {
			failures++
		}
	}
	if failures > 0 {
		return fmt.Errorf("%d environment(s) failed", failures)
	}
	rc.Printer.Successf("deployed to %d environment(s)", len(results))
	return nil
}

// renderDeploymentResults prints one row per environment with status,
// applied migration count, and duration.
func renderDeploymentResults(rc *rootContext, results map[string]orchestration.DeploymentResult) {
	if len(results) == 0 {
		return
	}
	if rc.Flags.JSONOut {
		_ = rc.Printer.JSON(results)
		return
	}
	rows := make([][]string, 0, len(results))
	for _, name := range sortedStringKeys(results) {
		r := results[name]
		errMsg := truncateString(r.Error, 48)
		rows = append(rows, []string{
			r.Environment,
			string(r.Status),
			fmt.Sprintf("%d", r.MigrationsApplied),
			fmt.Sprintf("%.2fs", r.DurationSeconds()),
			errMsg,
		})
	}
	rc.Printer.Table([]string{"Environment", "Status", "Applied", "Duration", "Error"}, rows)
}

// ---------------------------------------------------------------------------
// orchestrate status
// ---------------------------------------------------------------------------

func newOrchestrateStatusCommand() *cobra.Command {
	var (
		plan   string
		envCSV string
	)
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show health status of registered environments",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			rc := rootFromCmd(c)
			if plan == "" {
				return newUsageError("--plan is required (path to environments registry JSON)")
			}
			registry, err := orchestration.LoadRegistryFromFile(plan)
			if err != nil {
				return fmt.Errorf("load plan %s: %w", plan, err)
			}
			names := splitCSVArg(envCSV)
			if len(names) == 0 {
				names = registry.List()
			}
			coordinator := orchestration.NewMigrationCoordinator(registry)
			statuses := coordinator.GetDeploymentStatus(c.Context(), names)
			if rc.Flags.JSONOut {
				return rc.Printer.JSON(statuses)
			}
			rows := make([][]string, 0, len(statuses))
			for _, name := range sortedStringKeys(statuses) {
				verdict := "healthy"
				if !statuses[name] {
					verdict = "unhealthy"
				}
				rows = append(rows, []string{name, verdict})
			}
			rc.Printer.Table([]string{"Environment", "Status"}, rows)
			return nil
		},
	}
	cmd.Flags().StringVar(&plan, "plan", "environments.json", "path to environments registry JSON")
	cmd.Flags().StringVarP(&envCSV, "environments", "e", "", "comma-separated environment names (default: all)")
	return cmd
}

// ---------------------------------------------------------------------------
// orchestrate validate
// ---------------------------------------------------------------------------

func newOrchestrateValidateCommand() *cobra.Command {
	var plan string
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate environment configuration and connectivity",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			rc := rootFromCmd(c)
			if plan == "" {
				return newUsageError("--plan is required (path to environments registry JSON)")
			}
			registry, err := orchestration.LoadRegistryFromFile(plan)
			if err != nil {
				return fmt.Errorf("load plan %s: %w", plan, err)
			}
			names := registry.List()
			if len(names) == 0 {
				rc.Printer.Warnf("no environments configured")
				return nil
			}
			envs := make([]orchestration.EnvironmentConfig, 0, len(names))
			for _, n := range names {
				cfg, err := registry.Get(n)
				if err != nil {
					return err
				}
				envs = append(envs, cfg)
			}
			hc := orchestration.NewHealthCheck()
			statuses := hc.VerifyAllEnvironments(c.Context(), envs)
			rows := make([][]string, 0, len(statuses))
			allHealthy := true
			for _, name := range sortedStringKeys(statuses) {
				s := statuses[name]
				if !s.IsHealthy {
					allHealthy = false
				}
				rows = append(rows, []string{
					name,
					boolMarker(s.CanConnect),
					boolMarker(s.MigrationTableExists),
					boolMarker(s.IsHealthy),
					truncateString(s.Error, 48),
				})
			}
			rc.Printer.Table([]string{"Environment", "Connect", "MigrationTable", "Healthy", "Error"}, rows)
			if !allHealthy {
				return fmt.Errorf("one or more environments unhealthy")
			}
			rc.Printer.Successf("all environments healthy")
			return nil
		},
	}
	cmd.Flags().StringVar(&plan, "plan", "environments.json", "path to environments registry JSON")
	return cmd
}

// boolMarker renders a healthy/unhealthy tick for table cells.
func boolMarker(ok bool) string {
	if ok {
		return "ok"
	}
	return "fail"
}

// splitCSVArg splits a comma-separated flag value, trimming whitespace and
// dropping empty entries.
func splitCSVArg(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	raw := strings.Split(s, ",")
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		out = append(out, v)
	}
	return out
}
