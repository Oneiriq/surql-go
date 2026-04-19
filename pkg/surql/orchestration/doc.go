// Package orchestration coordinates SurrealDB migrations across multiple
// environments (staging, production, per-region, ...) with pluggable
// deployment strategies.
//
// The surface mirrors surql-py's `src/surql/orchestration/` so consumers
// written against the Python port transfer one-to-one:
//
//   - EnvironmentConfig + EnvironmentRegistry describe the environments
//     available to the coordinator. The registry is a sync.RWMutex-backed
//     struct with a package-level singleton reachable via GetRegistry.
//   - MigrationCoordinator orchestrates a DeploymentPlan against the
//     registry, optionally verifying health first and auto-rolling-back on
//     failure.
//   - HealthCheck exposes connectivity + migration-table probes used by the
//     coordinator and by CLI `status` commands.
//   - DeploymentStrategy is an interface implemented by SequentialStrategy,
//     ParallelStrategy, RollingStrategy, and CanaryStrategy. Each owns a
//     distinct roll-out shape (one-at-a-time / all-at-once / batched /
//     subset-then-rest).
//   - DeploymentResult + DeploymentStatus capture the outcome per
//     environment.
//
// Errors from this package wrap the sentinel errors.ErrOrchestration so
// callers can match with errors.Is.
package orchestration
