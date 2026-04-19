package orchestration

import (
	"context"
	"sync"

	"github.com/Oneiriq/surql-go/pkg/surql/connection"
)

// HealthStatus records the observed health of a single environment.
// Mirrors surql-py's `HealthStatus` model.
type HealthStatus struct {
	// Environment is the environment name.
	Environment string `json:"environment"`
	// IsHealthy is the aggregate verdict. True iff CanConnect is true and
	// no probe reported an error.
	IsHealthy bool `json:"is_healthy"`
	// CanConnect reports whether the client was able to reach the
	// database and issue a trivial query.
	CanConnect bool `json:"can_connect"`
	// MigrationTableExists reports whether the migration history table
	// was queryable.
	MigrationTableExists bool `json:"migration_table_exists"`
	// Error is a human-readable failure reason; empty when IsHealthy.
	Error string `json:"error,omitempty"`
}

// ClientFactory builds a connected *DatabaseClient for an environment.
// Injectable so tests can supply an in-memory fake without touching the
// network. When non-nil it is preferred over the default factory.
type ClientFactory func(ctx context.Context, cfg connection.ConnectionConfig) (*connection.DatabaseClient, func(), error)

// defaultClientFactory constructs a DatabaseClient and connects it. The
// returned cleanup disconnects the client; callers must invoke it.
func defaultClientFactory(ctx context.Context, cfg connection.ConnectionConfig) (*connection.DatabaseClient, func(), error) {
	client, err := connection.NewDatabaseClient(cfg)
	if err != nil {
		return nil, func() {}, err
	}
	if err := client.Connect(ctx); err != nil {
		return nil, func() {}, err
	}
	cleanup := func() { _ = client.Disconnect() }
	return client, cleanup, nil
}

// HealthCheck performs connectivity + migration-table probes against
// EnvironmentConfig entries. Construct with NewHealthCheck; the zero value
// uses the default (real-network) factory.
type HealthCheck struct {
	// Factory overrides the client constructor. Leave nil for production.
	Factory ClientFactory
}

// NewHealthCheck returns a HealthCheck using the default factory (which
// dials the real database).
func NewHealthCheck() *HealthCheck { return &HealthCheck{} }

// clientFactory returns h.Factory or the default.
func (h *HealthCheck) clientFactory() ClientFactory {
	if h.Factory != nil {
		return h.Factory
	}
	return defaultClientFactory
}

// CheckEnvironment performs a full health probe against env. Connectivity
// is verified first; when it fails the returned HealthStatus short-circuits
// with IsHealthy=false and the migration-table check is skipped.
func (h *HealthCheck) CheckEnvironment(ctx context.Context, env EnvironmentConfig) HealthStatus {
	client, cleanup, err := h.clientFactory()(ctx, env.Connection)
	if err != nil {
		return HealthStatus{
			Environment: env.Name,
			IsHealthy:   false,
			CanConnect:  false,
			Error:       "cannot connect to database: " + err.Error(),
		}
	}
	defer cleanup()

	// Trivial liveness query — mirrors py's `RETURN 1`.
	if _, err := client.Query(ctx, "RETURN 1"); err != nil {
		return HealthStatus{
			Environment: env.Name,
			IsHealthy:   false,
			CanConnect:  false,
			Error:       "connectivity probe failed: " + err.Error(),
		}
	}

	tableExists := checkMigrationTable(ctx, client)

	return HealthStatus{
		Environment:          env.Name,
		IsHealthy:            true,
		CanConnect:           true,
		MigrationTableExists: tableExists,
	}
}

// CheckConnectivity is a stand-alone connectivity probe. Returns true iff
// a connection could be opened and a trivial query succeeded.
func (h *HealthCheck) CheckConnectivity(ctx context.Context, env EnvironmentConfig) bool {
	client, cleanup, err := h.clientFactory()(ctx, env.Connection)
	if err != nil {
		return false
	}
	defer cleanup()
	_, err = client.Query(ctx, "RETURN 1")
	return err == nil
}

// VerifyAllEnvironments runs CheckEnvironment against each env in
// parallel and returns a map keyed by environment name. The order of
// probes is unspecified; callers that need deterministic ordering should
// sort their result set themselves.
func (h *HealthCheck) VerifyAllEnvironments(
	ctx context.Context,
	envs []EnvironmentConfig,
) map[string]HealthStatus {
	results := make(map[string]HealthStatus, len(envs))
	if len(envs) == 0 {
		return results
	}

	var (
		mu sync.Mutex
		wg sync.WaitGroup
	)
	wg.Add(len(envs))
	for _, env := range envs {
		env := env
		go func() {
			defer wg.Done()
			status := h.CheckEnvironment(ctx, env)
			mu.Lock()
			results[env.Name] = status
			mu.Unlock()
		}()
	}
	wg.Wait()
	return results
}

// checkMigrationTable probes the migration history table via a minimal
// SELECT. Any error (including "table not found") is treated as a
// negative signal — matching py's behaviour.
//
// Note: we query a literal table name here instead of importing the
// migration package to avoid an import cycle between orchestration and
// migration (coordinator already depends on migration for executor.go).
// The migration package defines this table as `_migration_history`; the
// constant is duplicated in migrationTableName below.
func checkMigrationTable(ctx context.Context, client *connection.DatabaseClient) bool {
	if client == nil {
		return false
	}
	_, err := client.Query(ctx, "SELECT * FROM "+migrationTableName+" LIMIT 1")
	return err == nil
}

// migrationTableName mirrors migration.MigrationTableName. Duplicated to
// keep the orchestration/migration dependency one-way (migration does not
// import orchestration, and this package only consumes migration in
// coordinator.go).
const migrationTableName = "_migration_history"

// CheckEnvironmentHealth is a convenience wrapper around
// HealthCheck.CheckEnvironment using a fresh HealthCheck.
func CheckEnvironmentHealth(ctx context.Context, env EnvironmentConfig) HealthStatus {
	return NewHealthCheck().CheckEnvironment(ctx, env)
}

// VerifyConnectivity is a convenience wrapper around
// HealthCheck.CheckConnectivity using a fresh HealthCheck.
func VerifyConnectivity(ctx context.Context, env EnvironmentConfig) bool {
	return NewHealthCheck().CheckConnectivity(ctx, env)
}
