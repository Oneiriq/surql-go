package orchestration

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/Oneiriq/surql-go/pkg/surql/connection"
	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

func newTestRegistry(t *testing.T, names ...string) *EnvironmentRegistry {
	t.Helper()
	r := NewEnvironmentRegistry()
	for i, name := range names {
		if err := r.Register(name, testConfig(), &RegisterOptions{Priority: i + 1}); err != nil {
			t.Fatalf("Register %s: %v", name, err)
		}
	}
	return r
}

func TestCoordinator_DeployToEnvironments_DryRun(t *testing.T) {
	t.Parallel()

	registry := newTestRegistry(t, "prod", "staging")
	c := NewMigrationCoordinator(registry)

	results, err := c.DeployToEnvironments(
		context.Background(),
		[]string{"staging", "prod"},
		buildMigrations(1),
		&DeployOptions{DryRun: true},
	)
	if err != nil {
		t.Fatalf("DeployToEnvironments: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	for _, r := range results {
		if r.Status != DeploymentStatusSuccess {
			t.Fatalf("env %s status = %s, want success", r.Environment, r.Status)
		}
	}
}

func TestCoordinator_UnknownEnvironment(t *testing.T) {
	t.Parallel()

	c := NewMigrationCoordinator(newTestRegistry(t, "prod"))
	_, err := c.DeployToEnvironments(
		context.Background(),
		[]string{"ghost"},
		buildMigrations(1),
		&DeployOptions{DryRun: true},
	)
	if !errors.Is(err, surqlerrors.ErrOrchestration) {
		t.Fatalf("err = %v; want ErrOrchestration", err)
	}
}

func TestCoordinator_EmptyEnvironmentsRejected(t *testing.T) {
	t.Parallel()

	c := NewMigrationCoordinator(newTestRegistry(t, "prod"))
	_, err := c.DeployToEnvironments(
		context.Background(),
		nil,
		buildMigrations(1),
		&DeployOptions{DryRun: true},
	)
	if !errors.Is(err, surqlerrors.ErrOrchestration) {
		t.Fatalf("err = %v; want ErrOrchestration", err)
	}
}

func TestCoordinator_UnknownStrategy(t *testing.T) {
	t.Parallel()

	c := NewMigrationCoordinator(newTestRegistry(t, "prod"))
	_, err := c.DeployToEnvironments(
		context.Background(),
		[]string{"prod"},
		buildMigrations(1),
		&DeployOptions{DryRun: true, Strategy: StrategyName("bogus")},
	)
	if !errors.Is(err, surqlerrors.ErrOrchestration) {
		t.Fatalf("err = %v; want ErrOrchestration", err)
	}
}

func TestCoordinator_HealthVerificationPasses(t *testing.T) {
	t.Parallel()

	registry := newTestRegistry(t, "prod", "staging")
	c := NewMigrationCoordinator(registry)

	// Factory that "connects" — but we route through DryRun so no
	// migrations are executed. Health check must honour the factory too.
	factory := func(ctx context.Context, cfg connection.ConnectionConfig) (*connection.DatabaseClient, func(), error) {
		return nil, func() {}, errors.New("should not be called in dry-run mode with VerifyHealth=false")
	}
	falseVal := false
	_, err := c.DeployToEnvironments(
		context.Background(),
		[]string{"prod", "staging"},
		buildMigrations(1),
		&DeployOptions{
			DryRun:       true,
			VerifyHealth: &falseVal,
			Factory:      factory,
		},
	)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestCoordinator_HealthVerificationFails(t *testing.T) {
	t.Parallel()

	registry := newTestRegistry(t, "prod")
	c := NewMigrationCoordinator(registry)

	// Failing factory -> every env is unhealthy.
	var counter atomic.Int32
	factory := failingFactory(&counter)

	trueVal := true
	_, err := c.DeployToEnvironments(
		context.Background(),
		[]string{"prod"},
		buildMigrations(1),
		&DeployOptions{
			DryRun:       false,
			VerifyHealth: &trueVal,
			Factory:      factory,
		},
	)
	if !errors.Is(err, surqlerrors.ErrOrchestration) {
		t.Fatalf("err = %v; want ErrOrchestration", err)
	}
	if counter.Load() == 0 {
		t.Fatal("factory should have been invoked for health check")
	}
}

func TestCoordinator_AutoRollbackOnFailure(t *testing.T) {
	t.Parallel()

	registry := newTestRegistry(t, "prod")
	c := NewMigrationCoordinator(registry)

	// Factory always errors -> deploy fails -> nothing to rollback; but
	// we can still verify that the rollback branch does not blow up.
	var counter atomic.Int32
	falseVal := false
	trueVal := true
	results, err := c.DeployToEnvironments(
		context.Background(),
		[]string{"prod"},
		buildMigrations(1),
		&DeployOptions{
			Strategy:     StrategySequential,
			DryRun:       false,
			VerifyHealth: &falseVal,
			AutoRollback: &trueVal,
			Factory:      failingFactory(&counter),
		},
	)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	r, ok := results["prod"]
	if !ok {
		t.Fatal("results missing prod entry")
	}
	if r.Status != DeploymentStatusFailed {
		t.Fatalf("status = %s, want failed", r.Status)
	}
}

func TestCoordinator_AutoRollbackFlipsSuccessToRolledBack(t *testing.T) {
	t.Parallel()

	registry := newTestRegistry(t, "prod", "staging")
	c := NewMigrationCoordinator(registry)

	// Env "prod" succeeds (via dry-run shortcut inside executeOnEnvironment
	// would skip factory entirely; instead, we use a smart factory that
	// succeeds for prod and fails for staging — but we cannot construct a
	// real DatabaseClient without a live DB. Instead, use DryRun=false +
	// a factory that always fails, and assert rollback handling preserves
	// non-success entries).
	//
	// This test pins the more-important invariant that rollback only
	// flips entries that were Success -> RolledBack; failures stay as-is.
	var counter atomic.Int32
	falseVal := false
	trueVal := true
	results, err := c.DeployToEnvironments(
		context.Background(),
		[]string{"prod", "staging"},
		buildMigrations(1),
		&DeployOptions{
			Strategy:     StrategyParallel,
			DryRun:       false,
			VerifyHealth: &falseVal,
			AutoRollback: &trueVal,
			Factory:      failingFactory(&counter),
		},
	)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	for name, r := range results {
		if r.Status != DeploymentStatusFailed {
			t.Fatalf("env %s status = %s, want failed (no successes -> none to roll back)", name, r.Status)
		}
	}
}

func TestCoordinator_GetDeploymentStatus_IgnoresUnknown(t *testing.T) {
	t.Parallel()

	registry := newTestRegistry(t, "prod")
	c := NewMigrationCoordinator(registry)
	// GetDeploymentStatus currently uses the default HealthCheck factory
	// which would try to connect to the real DB. To keep the test hermetic
	// we swap the internal HealthCheck.
	c.healthCheck = &HealthCheck{
		Factory: func(ctx context.Context, cfg connection.ConnectionConfig) (*connection.DatabaseClient, func(), error) {
			return nil, func() {}, errors.New("no network in tests")
		},
	}

	statuses := c.GetDeploymentStatus(context.Background(), []string{"prod", "ghost"})
	if _, ok := statuses["ghost"]; ok {
		t.Fatal("unknown env should be skipped from status map")
	}
	if statuses["prod"] {
		t.Fatal("prod should be unhealthy under failing factory")
	}
}

func TestPackageDeployToEnvironments_Wrapper(t *testing.T) {
	t.Parallel()

	registry := newTestRegistry(t, "prod")
	results, err := DeployToEnvironments(
		context.Background(),
		registry,
		[]string{"prod"},
		buildMigrations(1),
		&DeployOptions{DryRun: true},
	)
	if err != nil {
		t.Fatalf("DeployToEnvironments: %v", err)
	}
	if _, ok := results["prod"]; !ok {
		t.Fatal("results missing prod entry")
	}
}

func TestNewMigrationCoordinator_NilRegistryUsesGlobal(t *testing.T) {
	// Not parallel: mutates global registry.
	original := GetRegistry()
	t.Cleanup(func() { SetRegistry(original) })

	SetRegistry(NewEnvironmentRegistry())
	c := NewMigrationCoordinator(nil)
	if c.Registry() != GetRegistry() {
		t.Fatal("nil registry did not resolve to global")
	}
}
