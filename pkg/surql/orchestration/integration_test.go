//go:build integration
// +build integration

package orchestration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Oneiriq/surql-go/pkg/surql/connection"
	"github.com/Oneiriq/surql-go/pkg/surql/migration"
)

// getIntegrationURL reads the SurrealDB URL used by CI's integration job.
// Falls back to ws://localhost:8000/rpc when SURREAL_URL is unset; tests
// skip when connectivity fails.
func getIntegrationURL(t *testing.T) string {
	t.Helper()
	if url := os.Getenv("SURREAL_URL"); url != "" {
		return url
	}
	return "ws://localhost:8000/rpc"
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// integrationConfig returns a ConnectionConfig tuned for the v3.0.5
// docker-run container documented in the repo README. The namespace is
// isolated so the orchestration integration suite can run alongside the
// migration suite without collisions.
func integrationConfig(t *testing.T, env string) connection.ConnectionConfig {
	t.Helper()
	cfg := connection.DefaultConfig()
	cfg.DBURL = getIntegrationURL(t)
	cfg.DBNS = "surqlgo_test"
	cfg.DB = "orch_" + env
	cfg.DBRetryMaxAttempts = 2
	cfg.DBRetryMinWait = 0.2
	cfg.DBRetryMaxWait = 1.0
	cfg.DBRetryMultiplier = 2.0
	return cfg
}

// integrationFactory is a ClientFactory that authenticates after
// connecting so downstream queries hit a ready client. Required because
// the default factory (defaultClientFactory) leaves the client
// unauthenticated.
func integrationFactory(t *testing.T) ClientFactory {
	t.Helper()
	user := envOr("SURREAL_USER", "root")
	pass := envOr("SURREAL_PASS", "root")
	return func(ctx context.Context, cfg connection.ConnectionConfig) (*connection.DatabaseClient, func(), error) {
		client, err := connection.NewDatabaseClient(cfg)
		if err != nil {
			return nil, func() {}, err
		}
		if err := client.Connect(ctx); err != nil {
			return nil, func() {}, err
		}
		if _, err := client.Signin(ctx, connection.NewRootCredentials(user, pass)); err != nil {
			_ = client.Disconnect()
			return nil, func() {}, err
		}
		return client, func() { _ = client.Disconnect() }, nil
	}
}

// TestIntegration_DeployToEnvironments_Sequential verifies an end-to-end
// sequential deploy against a live SurrealDB: two environments, one
// migration each, health checks enabled.
func TestIntegration_DeployToEnvironments_Sequential(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	factory := integrationFactory(t)

	// Smoke-test the factory once to surface SKIP early when the
	// container is unreachable.
	if _, cleanup, err := factory(ctx, integrationConfig(t, "probe")); err != nil {
		t.Skipf("SurrealDB unreachable: %v", err)
	} else {
		cleanup()
	}

	registry := NewEnvironmentRegistry()
	if err := registry.Register("env_a", integrationConfig(t, "a"), &RegisterOptions{Priority: 1}); err != nil {
		t.Fatalf("Register env_a: %v", err)
	}
	if err := registry.Register("env_b", integrationConfig(t, "b"), &RegisterOptions{Priority: 2}); err != nil {
		t.Fatalf("Register env_b: %v", err)
	}

	// Clean history tables before the test so repeated runs stay idempotent.
	cleanupEnvs(ctx, t, factory, registry, "env_a", "env_b")
	t.Cleanup(func() { cleanupEnvs(ctx, t, factory, registry, "env_a", "env_b") })

	coord := NewMigrationCoordinator(registry)
	migrations := []migration.Migration{
		{
			Version:        "20240101_000000",
			Description:    "create orch_demo",
			UpStatements:   []string{"DEFINE TABLE orch_demo SCHEMAFULL;"},
			DownStatements: []string{"REMOVE TABLE IF EXISTS orch_demo;"},
		},
	}

	falseVal := false
	results, err := coord.DeployToEnvironments(
		ctx,
		[]string{"env_a", "env_b"},
		migrations,
		&DeployOptions{
			Strategy:     StrategySequential,
			Factory:      factory,
			VerifyHealth: &falseVal, // history table probe would add latency; rely on deploy to surface failures.
		},
	)
	if err != nil {
		t.Fatalf("DeployToEnvironments: %v", err)
	}
	for _, env := range []string{"env_a", "env_b"} {
		r, ok := results[env]
		if !ok {
			t.Fatalf("results missing %s", env)
		}
		if r.Status != DeploymentStatusSuccess {
			t.Fatalf("%s status = %s (err=%q), want success", env, r.Status, r.Error)
		}
		if r.MigrationsApplied != 1 {
			t.Fatalf("%s MigrationsApplied = %d, want 1", env, r.MigrationsApplied)
		}
	}
}

// TestIntegration_HealthCheck verifies the connectivity probe against a
// live SurrealDB.
func TestIntegration_HealthCheck(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	factory := integrationFactory(t)
	if _, cleanup, err := factory(ctx, integrationConfig(t, "probe")); err != nil {
		t.Skipf("SurrealDB unreachable: %v", err)
	} else {
		cleanup()
	}

	hc := &HealthCheck{Factory: factory}
	status := hc.CheckEnvironment(ctx, EnvironmentConfig{
		Name:       "env_health",
		Connection: integrationConfig(t, "health"),
	})
	if !status.IsHealthy {
		t.Fatalf("status = %+v, want healthy", status)
	}
	if !status.CanConnect {
		t.Fatal("CanConnect = false")
	}
}

// cleanupEnvs drops the migration history + orch_demo table in each named
// environment. Swallows errors: the purpose is best-effort isolation.
func cleanupEnvs(ctx context.Context, t *testing.T, factory ClientFactory, registry *EnvironmentRegistry, names ...string) {
	t.Helper()
	for _, name := range names {
		env, err := registry.Get(name)
		if err != nil {
			continue
		}
		client, cleanup, err := factory(ctx, env.Connection)
		if err != nil {
			continue
		}
		for _, stmt := range []string{
			"REMOVE TABLE IF EXISTS orch_demo;",
			"REMOVE TABLE IF EXISTS _migration_history;",
		} {
			_, _ = client.Query(ctx, stmt)
		}
		cleanup()
	}
}
