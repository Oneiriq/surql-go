package orchestration

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Oneiriq/surql-go/pkg/surql/connection"
	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
	"github.com/Oneiriq/surql-go/pkg/surql/migration"
)

// buildEnvs produces n environments named env0..envN-1 with priority i+1
// using a dummy memory:// connection config that passes validation.
func buildEnvs(n int) []EnvironmentConfig {
	out := make([]EnvironmentConfig, n)
	for i := 0; i < n; i++ {
		out[i] = EnvironmentConfig{
			Name:       "env" + itoa(i),
			Connection: testConfig(),
			Priority:   i + 1,
		}
	}
	return out
}

// buildMigrations produces n dummy migrations.
func buildMigrations(n int) []migration.Migration {
	out := make([]migration.Migration, n)
	for i := 0; i < n; i++ {
		out[i] = migration.Migration{
			Version:      "2024010" + itoa(i) + "_000000",
			Description:  "test",
			UpStatements: []string{"DEFINE TABLE dummy;"},
		}
	}
	return out
}

// failingFactory returns a ClientFactory whose every call returns an
// error. Useful for verifying that strategies report failures without
// requiring a live database.
func failingFactory(counter *atomic.Int32) ClientFactory {
	return func(ctx context.Context, cfg connection.ConnectionConfig) (*connection.DatabaseClient, func(), error) {
		counter.Add(1)
		return nil, func() {}, errors.New("factory disabled for test")
	}
}

// selectiveFactory returns a factory that fails for environments listed in
// failFor; every other environment is returned via the default factory
// (which would attempt to connect — so tests using selectiveFactory must
// arrange to never hit the success path, e.g. by listing every env).
func selectiveFactory(failFor map[string]struct{}) ClientFactory {
	return func(ctx context.Context, cfg connection.ConnectionConfig) (*connection.DatabaseClient, func(), error) {
		if _, ok := failFor[cfg.DBNS]; ok {
			return nil, func() {}, errors.New("selective failure")
		}
		// Should not be invoked in tests; produce an error to keep
		// runs hermetic.
		return nil, func() {}, errors.New("unconfigured environment in test factory")
	}
}

func TestSequentialStrategy_DryRunAllSucceed(t *testing.T) {
	t.Parallel()

	plan := &DeploymentPlan{
		Environments: buildEnvs(3),
		Migrations:   buildMigrations(2),
		DryRun:       true,
	}
	results, err := NewSequentialStrategy().DeployAll(context.Background(), plan)
	if err != nil {
		t.Fatalf("DeployAll: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("len(results) = %d, want 3", len(results))
	}
	for _, r := range results {
		if r.Status != DeploymentStatusSuccess {
			t.Fatalf("env %s status = %s, want success", r.Environment, r.Status)
		}
		if r.MigrationsApplied != 2 {
			t.Fatalf("env %s MigrationsApplied = %d, want 2", r.Environment, r.MigrationsApplied)
		}
	}
}

func TestSequentialStrategy_StopsOnFirstFailure(t *testing.T) {
	t.Parallel()

	var counter atomic.Int32
	plan := &DeploymentPlan{
		Environments: buildEnvs(3),
		Migrations:   buildMigrations(1),
		Factory:      failingFactory(&counter),
	}
	results, err := NewSequentialStrategy().DeployAll(context.Background(), plan)
	if err != nil {
		t.Fatalf("DeployAll: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1 (sequential must stop on failure)", len(results))
	}
	if results[0].Status != DeploymentStatusFailed {
		t.Fatalf("status = %s, want failed", results[0].Status)
	}
	if counter.Load() != 1 {
		t.Fatalf("factory called %d times, want 1", counter.Load())
	}
}

func TestSequentialStrategy_Deploy_ReturnsFirstFailure(t *testing.T) {
	t.Parallel()

	var counter atomic.Int32
	plan := &DeploymentPlan{
		Environments: buildEnvs(3),
		Migrations:   buildMigrations(1),
		Factory:      failingFactory(&counter),
	}
	r, err := NewSequentialStrategy().Deploy(context.Background(), plan)
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if r.Status != DeploymentStatusFailed {
		t.Fatalf("Deploy status = %s, want failed", r.Status)
	}
}

func TestParallelStrategy_DryRunAllSucceed(t *testing.T) {
	t.Parallel()

	plan := &DeploymentPlan{
		Environments:  buildEnvs(5),
		Migrations:    buildMigrations(1),
		DryRun:        true,
		MaxConcurrent: 2,
	}
	start := time.Now()
	results, err := NewParallelStrategy(0).DeployAll(context.Background(), plan)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("DeployAll: %v", err)
	}
	if len(results) != 5 {
		t.Fatalf("len(results) = %d, want 5", len(results))
	}
	for _, r := range results {
		if r.Status != DeploymentStatusSuccess {
			t.Fatalf("env %s status = %s, want success", r.Environment, r.Status)
		}
	}
	// With MaxConcurrent=2 and a 100ms simulated deploy, 5 envs take
	// at least 3 * 100ms. Use a loose upper bound to avoid flakiness.
	if elapsed < 250*time.Millisecond {
		t.Fatalf("elapsed %v too short for MaxConcurrent=2 (want >= 250ms)", elapsed)
	}
}

func TestParallelStrategy_PreservesOrder(t *testing.T) {
	t.Parallel()

	plan := &DeploymentPlan{
		Environments: buildEnvs(4),
		Migrations:   buildMigrations(1),
		DryRun:       true,
	}
	results, err := NewParallelStrategy(10).DeployAll(context.Background(), plan)
	if err != nil {
		t.Fatalf("DeployAll: %v", err)
	}
	for i, r := range results {
		if r.Environment != "env"+itoa(i) {
			t.Fatalf("results[%d].Environment = %s, want env%d", i, r.Environment, i)
		}
	}
}

func TestParallelStrategy_FactoryFailureReportsAll(t *testing.T) {
	t.Parallel()

	var counter atomic.Int32
	plan := &DeploymentPlan{
		Environments: buildEnvs(3),
		Migrations:   buildMigrations(1),
		Factory:      failingFactory(&counter),
	}
	results, err := NewParallelStrategy(0).DeployAll(context.Background(), plan)
	if err != nil {
		t.Fatalf("DeployAll: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("len(results) = %d, want 3 (parallel deploys all, including failures)", len(results))
	}
	for _, r := range results {
		if r.Status != DeploymentStatusFailed {
			t.Fatalf("env %s status = %s, want failed", r.Environment, r.Status)
		}
	}
	if counter.Load() != 3 {
		t.Fatalf("factory calls = %d, want 3", counter.Load())
	}
}

func TestRollingStrategy_DryRunBatches(t *testing.T) {
	t.Parallel()

	plan := &DeploymentPlan{
		Environments: buildEnvs(4),
		Migrations:   buildMigrations(1),
		DryRun:       true,
		BatchSize:    2,
	}
	results, err := NewRollingStrategy(0, 10*time.Millisecond).DeployAll(context.Background(), plan)
	if err != nil {
		t.Fatalf("DeployAll: %v", err)
	}
	if len(results) != 4 {
		t.Fatalf("len(results) = %d, want 4", len(results))
	}
	for _, r := range results {
		if r.Status != DeploymentStatusSuccess {
			t.Fatalf("env %s status = %s, want success", r.Environment, r.Status)
		}
	}
}

func TestRollingStrategy_StopsOnBatchFailure(t *testing.T) {
	t.Parallel()

	var factoryCalls atomic.Int32
	// Every factory call fails, so the first batch fails wholesale and
	// the rolling strategy must stop before attempting the second batch.
	factory := func(ctx context.Context, cfg connection.ConnectionConfig) (*connection.DatabaseClient, func(), error) {
		factoryCalls.Add(1)
		return nil, func() {}, errors.New("injected failure")
	}

	plan := &DeploymentPlan{
		Environments: buildEnvs(4),
		Migrations:   buildMigrations(1),
		BatchSize:    2,
		Factory:      factory,
	}
	results, err := NewRollingStrategy(0, 10*time.Millisecond).DeployAll(context.Background(), plan)
	if err != nil {
		t.Fatalf("DeployAll: %v", err)
	}
	// First batch's 2 failures are recorded; second batch never runs.
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2 (rolling stops on batch failure)", len(results))
	}
	if factoryCalls.Load() != 2 {
		t.Fatalf("factory calls = %d, want 2 (first batch only)", factoryCalls.Load())
	}
}

func TestRollingStrategy_BatchSizeDefaultsToOne(t *testing.T) {
	t.Parallel()

	plan := &DeploymentPlan{
		Environments: buildEnvs(3),
		Migrations:   buildMigrations(1),
		DryRun:       true,
	}
	r := NewRollingStrategy(0, 1*time.Millisecond)
	results, err := r.DeployAll(context.Background(), plan)
	if err != nil {
		t.Fatalf("DeployAll: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("len(results) = %d, want 3", len(results))
	}
}

func TestCanaryStrategy_SucceedsThenDeploysRemainder(t *testing.T) {
	t.Parallel()

	plan := &DeploymentPlan{
		Environments:     buildEnvs(10),
		Migrations:       buildMigrations(1),
		DryRun:           true,
		CanaryPercentage: 20.0, // 2 envs canary, 8 remainder
	}
	results, err := NewCanaryStrategy(0).DeployAll(context.Background(), plan)
	if err != nil {
		t.Fatalf("DeployAll: %v", err)
	}
	if len(results) != 10 {
		t.Fatalf("len(results) = %d, want 10", len(results))
	}
	for _, r := range results {
		if r.Status != DeploymentStatusSuccess {
			t.Fatalf("env %s status = %s, want success", r.Environment, r.Status)
		}
	}
}

func TestCanaryStrategy_FailsSkipsRemainder(t *testing.T) {
	t.Parallel()

	var canaryCalls atomic.Int32
	// All factory calls fail — this also asserts that once the canary
	// fails the remainder is never attempted (remainder would add more
	// factory calls beyond the canary count).
	factory := func(ctx context.Context, cfg connection.ConnectionConfig) (*connection.DatabaseClient, func(), error) {
		canaryCalls.Add(1)
		return nil, func() {}, errors.New("canary failure")
	}

	plan := &DeploymentPlan{
		Environments:     buildEnvs(10),
		Migrations:       buildMigrations(1),
		CanaryPercentage: 20.0, // 2 canary envs
		Factory:          factory,
	}
	results, err := NewCanaryStrategy(0).DeployAll(context.Background(), plan)
	if err != nil {
		t.Fatalf("DeployAll: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2 (canary only)", len(results))
	}
	if canaryCalls.Load() != 2 {
		t.Fatalf("factory calls = %d, want 2", canaryCalls.Load())
	}
}

func TestCanaryStrategy_InvalidPercentageErrors(t *testing.T) {
	t.Parallel()

	cases := []float64{0.5, 51.0, -1.0}
	for _, pct := range cases {
		pct := pct
		t.Run("pct="+ftoa(pct), func(t *testing.T) {
			t.Parallel()
			plan := &DeploymentPlan{
				Environments: buildEnvs(5),
				Migrations:   buildMigrations(1),
				DryRun:       true,
			}
			_, err := NewCanaryStrategy(pct).DeployAll(context.Background(), plan)
			if !errors.Is(err, surqlerrors.ErrValidation) {
				t.Fatalf("err = %v; want ErrValidation", err)
			}
		})
	}
}

func TestCanaryStrategy_DefaultPercentage(t *testing.T) {
	t.Parallel()

	plan := &DeploymentPlan{
		Environments: buildEnvs(10),
		Migrations:   buildMigrations(1),
		DryRun:       true,
	}
	results, err := NewCanaryStrategy(0).DeployAll(context.Background(), plan)
	if err != nil {
		t.Fatalf("DeployAll: %v", err)
	}
	if len(results) != 10 {
		t.Fatalf("len(results) = %d, want 10", len(results))
	}
}

func TestNewStrategy_TableDriven(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		input   StrategyName
		wantErr error
	}{
		{"sequential", StrategySequential, nil},
		{"parallel", StrategyParallel, nil},
		{"rolling", StrategyRolling, nil},
		{"canary", StrategyCanary, nil},
		{"unknown", StrategyName("bogus"), surqlerrors.ErrValidation},
	}
	plan := &DeploymentPlan{}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s, err := NewStrategy(tc.input, plan)
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("unexpected err: %v", err)
				}
				if s == nil {
					t.Fatal("nil strategy")
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v; want %v", err, tc.wantErr)
			}
		})
	}
}

func TestStrategy_NilPlan(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cases := []DeploymentStrategy{
		NewSequentialStrategy(),
		NewParallelStrategy(0),
		NewRollingStrategy(0, 0),
		NewCanaryStrategy(0),
	}
	for _, s := range cases {
		if _, err := s.DeployAll(ctx, nil); !errors.Is(err, surqlerrors.ErrOrchestration) {
			t.Fatalf("DeployAll(nil) err = %v; want ErrOrchestration", err)
		}
	}
}

// ftoa formats a float64 without importing strconv/fmt into the test.
func ftoa(f float64) string {
	// Simple non-scientific formatter for tiny test labels.
	neg := f < 0
	if neg {
		f = -f
	}
	intPart := int(f)
	fracPart := int((f - float64(intPart)) * 100)
	s := itoa(intPart) + "." + itoa(fracPart)
	if neg {
		s = "-" + s
	}
	return s
}
