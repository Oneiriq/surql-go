//go:build integration
// +build integration

package query

import (
	"context"
	"testing"
	"time"
)

// TestIntegration_GraphQuery_CountExists seeds alice -> {bob, carol}
// via `follows` and checks Count + Exists return the expected numbers.
func TestIntegration_GraphQuery_CountExists(t *testing.T) {
	client, cleanup := newIntegrationClient(t)
	defer cleanup()
	cleanupTable(t, client, "surqlgo_gq_user")
	cleanupTable(t, client, "surqlgo_gq_follows")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	seed := []string{
		"CREATE surqlgo_gq_user:alice SET name = 'alice';",
		"CREATE surqlgo_gq_user:bob   SET name = 'bob';",
		"CREATE surqlgo_gq_user:carol SET name = 'carol';",
		"RELATE surqlgo_gq_user:alice->surqlgo_gq_follows->surqlgo_gq_user:bob;",
		"RELATE surqlgo_gq_user:alice->surqlgo_gq_follows->surqlgo_gq_user:carol;",
	}
	for _, s := range seed {
		if _, err := client.Query(ctx, s); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	q := NewGraphQuery("surqlgo_gq_user:alice").Out("surqlgo_gq_follows", nil)

	n, err := q.Count(ctx, client)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if n != 2 {
		t.Errorf("want 2 followers, got %d", n)
	}

	ok, err := q.Exists(ctx, client)
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if !ok {
		t.Error("want Exists=true")
	}

	// Exists must report false for an empty traversal.
	q2 := NewGraphQuery("surqlgo_gq_user:bob").Out("surqlgo_gq_follows", nil)
	has, err := q2.Exists(ctx, client)
	if err != nil {
		t.Fatalf("Exists empty: %v", err)
	}
	if has {
		t.Error("want Exists=false for bob's (empty) followers")
	}
}
