package orchestration

import (
	"context"
	"strings"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
	"github.com/Oneiriq/surql-go/pkg/surql/migration"
)

// DeployOptions tunes MigrationCoordinator.DeployToEnvironments. The
// zero-value option set matches surql-py's defaults: sequential strategy,
// health verification on, auto-rollback on, dry-run off.
type DeployOptions struct {
	// Strategy is the named strategy to use. Defaults to "sequential".
	Strategy StrategyName
	// BatchSize applies to rolling deployments.
	BatchSize int
	// CanaryPercentage applies to canary deployments.
	CanaryPercentage float64
	// MaxConcurrent applies to parallel deployments.
	MaxConcurrent int
	// VerifyHealth runs the health check against every environment before
	// deploying. Defaults to true.
	VerifyHealth *bool
	// AutoRollback issues DOWN migrations on previously successful
	// environments when any deploy fails. Defaults to true.
	AutoRollback *bool
	// DryRun bypasses migration execution and the health check while
	// still producing realistic DeploymentResult shapes.
	DryRun bool
	// Factory overrides the DatabaseClient constructor. Nil uses the
	// production factory (dials the real database).
	Factory ClientFactory
	// HealthCheck overrides the health prober. Nil uses the default. The
	// factory is propagated from Factory if the supplied HealthCheck
	// carries its own nil factory.
	HealthCheck *HealthCheck
}

// resolveBools applies defaults for the tri-state pointer fields.
func (o DeployOptions) resolveBools() (verify, rollback bool) {
	verify, rollback = true, true
	if o.VerifyHealth != nil {
		verify = *o.VerifyHealth
	}
	if o.AutoRollback != nil {
		rollback = *o.AutoRollback
	}
	return
}

// MigrationCoordinator orchestrates migration deploys across an
// EnvironmentRegistry. Construct with NewMigrationCoordinator.
type MigrationCoordinator struct {
	registry    *EnvironmentRegistry
	healthCheck *HealthCheck
}

// NewMigrationCoordinator builds a coordinator bound to registry. Pass
// GetRegistry() to use the package-level singleton.
func NewMigrationCoordinator(registry *EnvironmentRegistry) *MigrationCoordinator {
	if registry == nil {
		registry = GetRegistry()
	}
	return &MigrationCoordinator{
		registry:    registry,
		healthCheck: NewHealthCheck(),
	}
}

// Registry returns the registry the coordinator was built with.
func (c *MigrationCoordinator) Registry() *EnvironmentRegistry { return c.registry }

// DeployToEnvironments resolves environments, optionally verifies their
// health, dispatches to the requested strategy, and optionally rolls back
// on failure. Returns per-environment results keyed by environment name.
//
// Errors fall into two buckets:
//   - ErrOrchestration when a precondition fails (unknown environment,
//     unhealthy environment, unknown strategy, ...).
//   - A wrapped strategy error when the strategy implementation itself
//     fails (rare — most failures surface via DeploymentResult.Status).
func (c *MigrationCoordinator) DeployToEnvironments(
	ctx context.Context,
	environments []string,
	migrations []migration.Migration,
	opts *DeployOptions,
) (map[string]DeploymentResult, error) {
	if c.registry == nil {
		return nil, surqlerrors.New(surqlerrors.ErrOrchestration, "coordinator has no registry")
	}
	if len(environments) == 0 {
		return nil, surqlerrors.New(surqlerrors.ErrOrchestration, "at least one environment is required")
	}

	effective := DeployOptions{}
	if opts != nil {
		effective = *opts
	}
	if effective.Strategy == "" {
		effective.Strategy = StrategySequential
	}
	verifyHealth, autoRollback := effective.resolveBools()

	// Resolve environment names -> configs.
	envConfigs := make([]EnvironmentConfig, 0, len(environments))
	for _, name := range environments {
		cfg, err := c.registry.Get(name)
		if err != nil {
			return nil, surqlerrors.Wrapf(surqlerrors.ErrOrchestration, err,
				"environment %q not registered", name)
		}
		envConfigs = append(envConfigs, cfg)
	}

	// Verify health unless dry-running or explicitly disabled.
	if verifyHealth && !effective.DryRun {
		hc := effective.HealthCheck
		if hc == nil {
			hc = c.healthCheck
		}
		// Inject factory override so health checks honour the same
		// ClientFactory as the deploy itself.
		if effective.Factory != nil && hc.Factory == nil {
			hc = &HealthCheck{Factory: effective.Factory}
		}
		statuses := hc.VerifyAllEnvironments(ctx, envConfigs)
		unhealthy := make([]string, 0)
		for _, env := range envConfigs {
			status, ok := statuses[env.Name]
			if !ok || !status.IsHealthy {
				unhealthy = append(unhealthy, env.Name)
			}
		}
		if len(unhealthy) > 0 {
			return nil, surqlerrors.Newf(surqlerrors.ErrOrchestration,
				"unhealthy environments: %s", strings.Join(unhealthy, ", "))
		}
	}

	plan := &DeploymentPlan{
		Environments:     envConfigs,
		Migrations:       migrations,
		BatchSize:        effective.BatchSize,
		CanaryPercentage: effective.CanaryPercentage,
		MaxConcurrent:    effective.MaxConcurrent,
		DryRun:           effective.DryRun,
		Factory:          effective.Factory,
	}

	strategy, err := NewStrategy(effective.Strategy, plan)
	if err != nil {
		return nil, surqlerrors.Wrapf(surqlerrors.ErrOrchestration, err,
			"failed to construct strategy %q", effective.Strategy)
	}

	results, err := strategy.DeployAll(ctx, plan)
	if err != nil {
		return nil, surqlerrors.Wrapf(surqlerrors.ErrOrchestration, err,
			"deployment strategy %q failed", effective.Strategy)
	}

	resultMap := make(map[string]DeploymentResult, len(results))
	for _, r := range results {
		resultMap[r.Environment] = r
	}

	if autoRollback && !effective.DryRun {
		failed := false
		for _, r := range results {
			if r.Status == DeploymentStatusFailed {
				failed = true
				break
			}
		}
		if failed {
			c.rollback(ctx, envConfigs, migrations, resultMap, effective.Factory)
		}
	}

	return resultMap, nil
}

// GetDeploymentStatus returns a healthy/unhealthy verdict per environment.
// Convenience wrapper around HealthCheck; unknown environments are
// silently skipped (mirrors py).
func (c *MigrationCoordinator) GetDeploymentStatus(
	ctx context.Context,
	environments []string,
) map[string]bool {
	out := make(map[string]bool, len(environments))
	envConfigs := make([]EnvironmentConfig, 0, len(environments))
	for _, name := range environments {
		cfg, err := c.registry.Get(name)
		if err != nil {
			continue
		}
		envConfigs = append(envConfigs, cfg)
	}
	statuses := c.healthCheck.VerifyAllEnvironments(ctx, envConfigs)
	for name, s := range statuses {
		out[name] = s.IsHealthy
	}
	return out
}

// rollback issues DOWN migrations against every environment marked
// DeploymentStatusSuccess, mirroring py's `_rollback_deployments`. The
// result map is updated in place to mark rolled-back entries. Errors
// during rollback are swallowed — the best-effort contract matches py.
func (c *MigrationCoordinator) rollback(
	ctx context.Context,
	envs []EnvironmentConfig,
	migrations []migration.Migration,
	results map[string]DeploymentResult,
	factory ClientFactory,
) {
	if factory == nil {
		factory = defaultClientFactory
	}
	for _, env := range envs {
		r, ok := results[env.Name]
		if !ok || r.Status != DeploymentStatusSuccess {
			continue
		}
		client, cleanup, err := factory(ctx, env.Connection)
		if err != nil {
			continue
		}
		// Execute DOWN migrations in reverse order.
		for i := len(migrations) - 1; i >= 0; i-- {
			_, _ = migration.ExecuteMigration(ctx, client, migrations[i], migration.MigrationDirectionDown)
		}
		cleanup()
		r.Status = DeploymentStatusRolledBack
		results[env.Name] = r
	}
}

// DeployToEnvironments is the package-level convenience wrapper mirroring
// py's `deploy_to_environments` helper.
func DeployToEnvironments(
	ctx context.Context,
	registry *EnvironmentRegistry,
	environments []string,
	migrations []migration.Migration,
	opts *DeployOptions,
) (map[string]DeploymentResult, error) {
	return NewMigrationCoordinator(registry).DeployToEnvironments(ctx, environments, migrations, opts)
}
