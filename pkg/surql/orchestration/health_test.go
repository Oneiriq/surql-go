package orchestration

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/Oneiriq/surql-go/pkg/surql/connection"
)

func TestHealthCheck_CannotConnect(t *testing.T) {
	t.Parallel()

	hc := &HealthCheck{
		Factory: func(ctx context.Context, cfg connection.ConnectionConfig) (*connection.DatabaseClient, func(), error) {
			return nil, func() {}, errors.New("boom")
		},
	}
	status := hc.CheckEnvironment(context.Background(), EnvironmentConfig{
		Name:       "prod",
		Connection: testConfig(),
	})
	if status.IsHealthy {
		t.Fatal("IsHealthy = true, want false on connection failure")
	}
	if status.CanConnect {
		t.Fatal("CanConnect = true, want false")
	}
	if status.Error == "" {
		t.Fatal("Error should be populated on failure")
	}
}

func TestHealthCheck_CheckConnectivity_Fail(t *testing.T) {
	t.Parallel()

	hc := &HealthCheck{
		Factory: func(ctx context.Context, cfg connection.ConnectionConfig) (*connection.DatabaseClient, func(), error) {
			return nil, func() {}, errors.New("boom")
		},
	}
	if hc.CheckConnectivity(context.Background(), EnvironmentConfig{
		Name:       "prod",
		Connection: testConfig(),
	}) {
		t.Fatal("CheckConnectivity = true, want false")
	}
}

func TestHealthCheck_VerifyAllEnvironments_ParallelFailures(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	hc := &HealthCheck{
		Factory: func(ctx context.Context, cfg connection.ConnectionConfig) (*connection.DatabaseClient, func(), error) {
			calls.Add(1)
			return nil, func() {}, errors.New("boom")
		},
	}
	envs := buildEnvs(5)
	statuses := hc.VerifyAllEnvironments(context.Background(), envs)
	if len(statuses) != 5 {
		t.Fatalf("len(statuses) = %d, want 5", len(statuses))
	}
	if calls.Load() != 5 {
		t.Fatalf("factory calls = %d, want 5", calls.Load())
	}
	for name, s := range statuses {
		if s.IsHealthy {
			t.Fatalf("env %s should be unhealthy", name)
		}
	}
}

func TestHealthCheck_VerifyAllEnvironments_Empty(t *testing.T) {
	t.Parallel()

	hc := NewHealthCheck()
	statuses := hc.VerifyAllEnvironments(context.Background(), nil)
	if len(statuses) != 0 {
		t.Fatalf("len(statuses) = %d, want 0", len(statuses))
	}
}

func TestCheckEnvironmentHealth_PopulatesEnvironment(t *testing.T) {
	t.Parallel()

	// Using an already-cancelled context the default factory fails fast,
	// so we exercise the wrapper shape without waiting on network retries.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	status := CheckEnvironmentHealth(ctx, EnvironmentConfig{
		Name:       "prod",
		Connection: testConfig(),
	})
	if status.Environment != "prod" {
		t.Fatalf("Environment = %q, want prod", status.Environment)
	}
	if status.IsHealthy {
		t.Fatal("expected unhealthy status under cancelled context")
	}
}

func TestVerifyConnectivity_CancelledContextReturnsFalse(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if VerifyConnectivity(ctx, EnvironmentConfig{
		Name:       "prod",
		Connection: testConfig(),
	}) {
		t.Fatal("VerifyConnectivity = true under cancelled ctx, want false")
	}
}
