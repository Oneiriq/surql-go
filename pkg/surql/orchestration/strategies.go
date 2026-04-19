package orchestration

import (
	"context"
	"fmt"
	"sync"
	"time"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
	"github.com/Oneiriq/surql-go/pkg/surql/migration"
)

// DeploymentPlan is the input to DeploymentStrategy.Deploy. It carries the
// target environments, the migrations to apply, and strategy-specific
// knobs (BatchSize, CanaryPercentage, ...).
//
// Strategies read only the fields relevant to their shape; irrelevant
// fields are ignored. Plans are safe to share across strategies because
// they are treated as read-only.
type DeploymentPlan struct {
	// Environments is the ordered list of environments to deploy to. The
	// coordinator resolves names against the registry before building the
	// plan, so this slice must contain ready EnvironmentConfig values.
	Environments []EnvironmentConfig
	// Migrations are the migrations to apply in order (UP direction).
	Migrations []migration.Migration
	// BatchSize applies to RollingStrategy — environments are deployed in
	// batches of this size. Defaults to 1 when zero.
	BatchSize int
	// CanaryPercentage applies to CanaryStrategy — the percentage (1-50)
	// of environments to deploy to first before rolling out the remainder.
	// Defaults to 10.0 when zero.
	CanaryPercentage float64
	// MaxConcurrent applies to ParallelStrategy — caps the number of
	// in-flight deployments. Defaults to 5 when zero.
	MaxConcurrent int
	// DryRun skips migration execution but still reports realistic
	// DeploymentResult shapes.
	DryRun bool
	// Factory overrides the DatabaseClient constructor. When nil the
	// production factory is used (which dials the real database). Tests
	// inject a fake factory to keep runs hermetic.
	Factory ClientFactory
}

// DeploymentStrategy is the interface implemented by Sequential,
// Parallel, Rolling, and Canary strategies. Implementations must be safe
// to call concurrently across different plans but need not protect their
// own state; the coordinator never reuses a strategy instance across
// concurrent deploys.
type DeploymentStrategy interface {
	Deploy(ctx context.Context, plan *DeploymentPlan) (*DeploymentResult, error)
	// DeployAll returns the per-environment results, ordered to match
	// plan.Environments. DeployAll is the primary surface used by
	// MigrationCoordinator; Deploy returns the first failing or last
	// successful result for callers that want a simple single-result view.
	DeployAll(ctx context.Context, plan *DeploymentPlan) ([]DeploymentResult, error)
}

// ---------------------------------------------------------------------------
// Shared deploy helpers
// ---------------------------------------------------------------------------

// executeOnEnvironment runs every migration in plan.Migrations (UP
// direction) against env. Returns a DeploymentResult describing the
// outcome. Errors never escape this function — they are reported via the
// returned result's Status / Error fields.
func executeOnEnvironment(
	ctx context.Context,
	env EnvironmentConfig,
	plan *DeploymentPlan,
) DeploymentResult {
	started := time.Now().UTC()

	if plan.DryRun {
		// Mirror py's simulated 100ms sleep so dry-runs look realistic
		// without doing any I/O.
		select {
		case <-time.After(100 * time.Millisecond):
		case <-ctx.Done():
			completed := time.Now().UTC()
			return DeploymentResult{
				Environment:     env.Name,
				Status:          DeploymentStatusFailed,
				StartedAt:       started,
				CompletedAt:     completed,
				Error:           ctx.Err().Error(),
				ExecutionTimeMs: completed.Sub(started).Milliseconds(),
			}
		}
		completed := time.Now().UTC()
		return DeploymentResult{
			Environment:       env.Name,
			Status:            DeploymentStatusSuccess,
			StartedAt:         started,
			CompletedAt:       completed,
			ExecutionTimeMs:   completed.Sub(started).Milliseconds(),
			MigrationsApplied: len(plan.Migrations),
		}
	}

	factory := plan.Factory
	if factory == nil {
		factory = defaultClientFactory
	}

	client, cleanup, err := factory(ctx, env.Connection)
	if err != nil {
		completed := time.Now().UTC()
		return DeploymentResult{
			Environment: env.Name,
			Status:      DeploymentStatusFailed,
			StartedAt:   started,
			CompletedAt: completed,
			Error:       fmt.Sprintf("connect to %s failed: %v", env.Name, err),
		}
	}
	defer cleanup()

	applied := 0
	for _, m := range plan.Migrations {
		if _, err := migration.ExecuteMigration(ctx, client, m, migration.MigrationDirectionUp); err != nil {
			completed := time.Now().UTC()
			return DeploymentResult{
				Environment:       env.Name,
				Status:            DeploymentStatusFailed,
				StartedAt:         started,
				CompletedAt:       completed,
				Error:             fmt.Sprintf("migration %s failed: %v", m.Version, err),
				MigrationsApplied: applied,
			}
		}
		applied++
	}

	completed := time.Now().UTC()
	return DeploymentResult{
		Environment:       env.Name,
		Status:            DeploymentStatusSuccess,
		StartedAt:         started,
		CompletedAt:       completed,
		ExecutionTimeMs:   completed.Sub(started).Milliseconds(),
		MigrationsApplied: applied,
	}
}

// lastOrEmpty returns the last result in results (or a zero-value
// DeploymentResult when the slice is empty). Used to supply a sensible
// Deploy return when the caller wants a single-result view.
func lastOrEmpty(results []DeploymentResult) *DeploymentResult {
	if len(results) == 0 {
		return &DeploymentResult{}
	}
	last := results[len(results)-1]
	return &last
}

// firstFailure scans results and returns the first DeploymentStatusFailed
// entry, or nil when every entry succeeded.
func firstFailure(results []DeploymentResult) *DeploymentResult {
	for i := range results {
		if results[i].Status == DeploymentStatusFailed {
			r := results[i]
			return &r
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// SequentialStrategy
// ---------------------------------------------------------------------------

// SequentialStrategy deploys to environments one at a time in order,
// stopping at the first failure. Mirrors surql-py's `SequentialStrategy`.
type SequentialStrategy struct{}

// NewSequentialStrategy constructs a SequentialStrategy.
func NewSequentialStrategy() *SequentialStrategy { return &SequentialStrategy{} }

// Deploy runs the plan and returns the last processed result (failed or
// final).
func (s *SequentialStrategy) Deploy(ctx context.Context, plan *DeploymentPlan) (*DeploymentResult, error) {
	results, err := s.DeployAll(ctx, plan)
	if err != nil {
		return nil, err
	}
	if fail := firstFailure(results); fail != nil {
		return fail, nil
	}
	return lastOrEmpty(results), nil
}

// DeployAll deploys sequentially, stopping on the first failure.
func (s *SequentialStrategy) DeployAll(ctx context.Context, plan *DeploymentPlan) ([]DeploymentResult, error) {
	if plan == nil {
		return nil, surqlerrors.New(surqlerrors.ErrOrchestration, "deployment plan cannot be nil")
	}
	results := make([]DeploymentResult, 0, len(plan.Environments))
	for _, env := range plan.Environments {
		r := executeOnEnvironment(ctx, env, plan)
		results = append(results, r)
		if r.Status == DeploymentStatusFailed {
			break
		}
	}
	return results, nil
}

// ---------------------------------------------------------------------------
// ParallelStrategy
// ---------------------------------------------------------------------------

// ParallelStrategy deploys to every environment concurrently, bounded by
// MaxConcurrent. Mirrors surql-py's `ParallelStrategy`.
type ParallelStrategy struct {
	// MaxConcurrent caps in-flight deployments. Zero defers to
	// plan.MaxConcurrent, which defaults to 5.
	MaxConcurrent int
}

// NewParallelStrategy constructs a ParallelStrategy. Pass maxConcurrent <= 0
// to inherit from the plan.
func NewParallelStrategy(maxConcurrent int) *ParallelStrategy {
	return &ParallelStrategy{MaxConcurrent: maxConcurrent}
}

// Deploy runs the plan and returns the first failing result (when any) or
// the last successful result.
func (p *ParallelStrategy) Deploy(ctx context.Context, plan *DeploymentPlan) (*DeploymentResult, error) {
	results, err := p.DeployAll(ctx, plan)
	if err != nil {
		return nil, err
	}
	if fail := firstFailure(results); fail != nil {
		return fail, nil
	}
	return lastOrEmpty(results), nil
}

// DeployAll fans out to environments with a semaphore-bounded worker pool.
// Results are returned in plan.Environments order regardless of
// completion order.
func (p *ParallelStrategy) DeployAll(ctx context.Context, plan *DeploymentPlan) ([]DeploymentResult, error) {
	if plan == nil {
		return nil, surqlerrors.New(surqlerrors.ErrOrchestration, "deployment plan cannot be nil")
	}
	limit := p.MaxConcurrent
	if limit <= 0 {
		limit = plan.MaxConcurrent
	}
	if limit <= 0 {
		limit = 5
	}

	results := make([]DeploymentResult, len(plan.Environments))
	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup

	for i, env := range plan.Environments {
		i, env := i, env
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i] = executeOnEnvironment(ctx, env, plan)
		}()
	}
	wg.Wait()
	return results, nil
}

// ---------------------------------------------------------------------------
// RollingStrategy
// ---------------------------------------------------------------------------

// RollingStrategy deploys environments in batches. Within each batch the
// deployments run in parallel; batches run sequentially with a short
// stability delay in between. Stops on the first batch that contains a
// failure. Mirrors surql-py's `RollingStrategy`.
type RollingStrategy struct {
	// BatchSize defaults to plan.BatchSize when <= 0; plan default is 1.
	BatchSize int
	// BatchDelay is the pause between successive batches. Defaults to
	// 1s — matching py's `anyio.sleep(1.0)`.
	BatchDelay time.Duration
}

// NewRollingStrategy constructs a RollingStrategy. batchSize <= 0 defers
// to the plan; batchDelay <= 0 defers to the 1s default.
func NewRollingStrategy(batchSize int, batchDelay time.Duration) *RollingStrategy {
	return &RollingStrategy{BatchSize: batchSize, BatchDelay: batchDelay}
}

// Deploy runs the plan and returns the first failing result (when any) or
// the last successful result.
func (r *RollingStrategy) Deploy(ctx context.Context, plan *DeploymentPlan) (*DeploymentResult, error) {
	results, err := r.DeployAll(ctx, plan)
	if err != nil {
		return nil, err
	}
	if fail := firstFailure(results); fail != nil {
		return fail, nil
	}
	return lastOrEmpty(results), nil
}

// DeployAll deploys in batches.
func (r *RollingStrategy) DeployAll(ctx context.Context, plan *DeploymentPlan) ([]DeploymentResult, error) {
	if plan == nil {
		return nil, surqlerrors.New(surqlerrors.ErrOrchestration, "deployment plan cannot be nil")
	}
	batchSize := r.BatchSize
	if batchSize <= 0 {
		batchSize = plan.BatchSize
	}
	if batchSize <= 0 {
		batchSize = 1
	}
	delay := r.BatchDelay
	if delay <= 0 {
		delay = time.Second
	}

	results := make([]DeploymentResult, 0, len(plan.Environments))
	total := len(plan.Environments)

	for i := 0; i < total; i += batchSize {
		end := i + batchSize
		if end > total {
			end = total
		}
		batch := plan.Environments[i:end]
		batchResults := make([]DeploymentResult, len(batch))
		var wg sync.WaitGroup
		for j, env := range batch {
			j, env := j, env
			wg.Add(1)
			go func() {
				defer wg.Done()
				batchResults[j] = executeOnEnvironment(ctx, env, plan)
			}()
		}
		wg.Wait()
		results = append(results, batchResults...)

		// Stop on first batch failure.
		if firstFailure(batchResults) != nil {
			break
		}

		// Stability delay between batches, unless we just finished.
		if end < total {
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return results, nil
			}
		}
	}
	return results, nil
}

// ---------------------------------------------------------------------------
// CanaryStrategy
// ---------------------------------------------------------------------------

// CanaryStrategy deploys to a small subset (the "canary") first, then
// rolls out to the remaining environments only when the canary succeeds.
// Mirrors surql-py's `CanaryStrategy`, including percentage validation.
type CanaryStrategy struct {
	// Percentage is the canary percentage (1-50). Zero defers to the
	// plan; plan default is 10.
	Percentage float64
}

// NewCanaryStrategy constructs a CanaryStrategy. percentage <= 0 defers
// to the plan. When non-zero the percentage must sit in [1, 50]; an
// invalid value surfaces as ErrValidation on Deploy.
func NewCanaryStrategy(percentage float64) *CanaryStrategy {
	return &CanaryStrategy{Percentage: percentage}
}

// Deploy runs the canary + remainder and returns the first failing
// result (when any) or the last successful result.
func (c *CanaryStrategy) Deploy(ctx context.Context, plan *DeploymentPlan) (*DeploymentResult, error) {
	results, err := c.DeployAll(ctx, plan)
	if err != nil {
		return nil, err
	}
	if fail := firstFailure(results); fail != nil {
		return fail, nil
	}
	return lastOrEmpty(results), nil
}

// DeployAll runs the canary batch first; when any canary environment
// fails the remainder is skipped. When the canary succeeds every
// remaining environment is deployed in parallel.
func (c *CanaryStrategy) DeployAll(ctx context.Context, plan *DeploymentPlan) ([]DeploymentResult, error) {
	if plan == nil {
		return nil, surqlerrors.New(surqlerrors.ErrOrchestration, "deployment plan cannot be nil")
	}
	pct := c.Percentage
	// Only the zero-value percentage defers to the plan; a non-zero but
	// out-of-range percentage surfaces as ErrValidation below so
	// operators see their mistake instead of silently falling back to
	// defaults. Mirrors py's constructor-time check.
	if pct == 0 {
		pct = plan.CanaryPercentage
	}
	if pct == 0 {
		pct = 10.0
	}
	if pct < 1.0 || pct > 50.0 {
		return nil, surqlerrors.Newf(surqlerrors.ErrValidation,
			"canary percentage must be between 1.0 and 50.0, got %.2f", pct)
	}

	envs := plan.Environments
	if len(envs) == 0 {
		return nil, nil
	}
	canaryCount := int(float64(len(envs)) * pct / 100.0)
	if canaryCount < 1 {
		canaryCount = 1
	}
	if canaryCount > len(envs) {
		canaryCount = len(envs)
	}

	canary := envs[:canaryCount]
	remainder := envs[canaryCount:]

	canaryResults := runParallel(ctx, canary, plan)
	if firstFailure(canaryResults) != nil {
		return canaryResults, nil
	}
	if len(remainder) == 0 {
		return canaryResults, nil
	}
	remainderResults := runParallel(ctx, remainder, plan)
	return append(canaryResults, remainderResults...), nil
}

// runParallel is the unbounded-parallel helper used by canary batches.
// It preserves input order in the returned slice.
func runParallel(ctx context.Context, envs []EnvironmentConfig, plan *DeploymentPlan) []DeploymentResult {
	results := make([]DeploymentResult, len(envs))
	var wg sync.WaitGroup
	for i, env := range envs {
		i, env := i, env
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = executeOnEnvironment(ctx, env, plan)
		}()
	}
	wg.Wait()
	return results
}

// ---------------------------------------------------------------------------
// Strategy naming helpers
// ---------------------------------------------------------------------------

// StrategyName is a string alias used by DeploymentPlan-style entry
// points that accept a named strategy (mirrors py's `strategy='rolling'`).
type StrategyName string

// Recognised strategy names.
const (
	StrategySequential StrategyName = "sequential"
	StrategyParallel   StrategyName = "parallel"
	StrategyRolling    StrategyName = "rolling"
	StrategyCanary     StrategyName = "canary"
)

// IsValid reports whether s is a recognised strategy name.
func (s StrategyName) IsValid() bool {
	switch s {
	case StrategySequential, StrategyParallel, StrategyRolling, StrategyCanary:
		return true
	}
	return false
}

// NewStrategy constructs a DeploymentStrategy from a named strategy and a
// reference plan. Unknown names return ErrValidation. Parameters come
// from the plan (BatchSize, CanaryPercentage, MaxConcurrent) so strategies
// produced by this helper stay in sync with the plan's knobs.
func NewStrategy(name StrategyName, plan *DeploymentPlan) (DeploymentStrategy, error) {
	switch name {
	case StrategySequential:
		return NewSequentialStrategy(), nil
	case StrategyParallel:
		max := 0
		if plan != nil {
			max = plan.MaxConcurrent
		}
		return NewParallelStrategy(max), nil
	case StrategyRolling:
		batch := 0
		if plan != nil {
			batch = plan.BatchSize
		}
		return NewRollingStrategy(batch, 0), nil
	case StrategyCanary:
		pct := 0.0
		if plan != nil {
			pct = plan.CanaryPercentage
		}
		return NewCanaryStrategy(pct), nil
	default:
		return nil, surqlerrors.Newf(surqlerrors.ErrValidation,
			"unknown deployment strategy %q (want sequential, parallel, rolling, or canary)", name)
	}
}
