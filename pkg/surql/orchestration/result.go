package orchestration

import "time"

// DeploymentStatus captures the lifecycle state of a single environment's
// deployment. Mirrors surql-py's `DeploymentStatus` enum.
type DeploymentStatus string

// DeploymentStatus values.
const (
	// DeploymentStatusPending indicates the deployment has not started.
	DeploymentStatusPending DeploymentStatus = "pending"
	// DeploymentStatusInProgress indicates the deployment is running.
	DeploymentStatusInProgress DeploymentStatus = "in_progress"
	// DeploymentStatusSuccess indicates every migration applied cleanly.
	DeploymentStatusSuccess DeploymentStatus = "success"
	// DeploymentStatusFailed indicates at least one migration failed.
	DeploymentStatusFailed DeploymentStatus = "failed"
	// DeploymentStatusRolledBack indicates a previously successful
	// deployment has been reverted via auto-rollback.
	DeploymentStatusRolledBack DeploymentStatus = "rolled_back"
)

// IsValid reports whether s is one of the defined DeploymentStatus values.
func (s DeploymentStatus) IsValid() bool {
	switch s {
	case DeploymentStatusPending, DeploymentStatusInProgress,
		DeploymentStatusSuccess, DeploymentStatusFailed,
		DeploymentStatusRolledBack:
		return true
	}
	return false
}

// String returns the serialized form of the status.
func (s DeploymentStatus) String() string { return string(s) }

// DeploymentResult is the outcome of running a strategy against a single
// environment. All timing fields are in UTC.
//
// Zero-value DeploymentResult is meaningful only as a placeholder while a
// deployment is in flight (CompletedAt will be the zero Time). Callers
// should inspect Status for a definitive verdict.
type DeploymentResult struct {
	// Environment is the name of the target environment.
	Environment string `json:"environment"`
	// Status is the final lifecycle state.
	Status DeploymentStatus `json:"status"`
	// StartedAt is when the strategy began deploying to this environment.
	StartedAt time.Time `json:"started_at"`
	// CompletedAt is when the strategy finished (success or failure). Zero
	// value when still in progress.
	CompletedAt time.Time `json:"completed_at,omitempty"`
	// Error is the human-readable failure reason; empty when Status ==
	// DeploymentStatusSuccess.
	Error string `json:"error,omitempty"`
	// ExecutionTimeMs is the wall-clock duration of the deploy in
	// milliseconds. Zero when still in progress or when the deploy failed
	// before timing could be captured.
	ExecutionTimeMs int64 `json:"execution_time_ms,omitempty"`
	// MigrationsApplied is how many migrations were executed successfully.
	MigrationsApplied int `json:"migrations_applied"`
}

// DurationSeconds returns the wall-clock duration of the deploy in seconds,
// or 0 when CompletedAt is the zero Time.
func (r DeploymentResult) DurationSeconds() float64 {
	if r.CompletedAt.IsZero() {
		return 0
	}
	return r.CompletedAt.Sub(r.StartedAt).Seconds()
}
