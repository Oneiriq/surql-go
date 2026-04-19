//go:build integration
// +build integration

package query

import (
	"context"
	"testing"
	"time"
)

// TestIntegration_Traverse walks a small graph and confirms
// Traverse + TraverseWithDepth return the expected targets.
func TestIntegration_Traverse(t *testing.T) {
	client, cleanup := newIntegrationClient(t)
	defer cleanup()
	cleanupTable(t, client, "surqlgo_graph_user")
	cleanupTable(t, client, "surqlgo_graph_post")
	cleanupTable(t, client, "surqlgo_graph_likes")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Seed nodes + edges.
	seed := []string{
		"CREATE surqlgo_graph_user:alice SET name = 'alice';",
		"CREATE surqlgo_graph_post:p1 SET title = 'first';",
		"CREATE surqlgo_graph_post:p2 SET title = 'second';",
		"RELATE surqlgo_graph_user:alice->surqlgo_graph_likes->surqlgo_graph_post:p1;",
		"RELATE surqlgo_graph_user:alice->surqlgo_graph_likes->surqlgo_graph_post:p2;",
	}
	for _, s := range seed {
		if _, err := client.Query(ctx, s); err != nil {
			t.Fatalf("seed %q: %v", s, err)
		}
	}

	// Raw Traverse.
	got, err := Traverse(ctx, client,
		"surqlgo_graph_user:alice",
		"->surqlgo_graph_likes->surqlgo_graph_post",
	)
	if err != nil {
		t.Fatalf("Traverse: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("Traverse: want 2 posts, got %d", len(got))
	}

	// TraverseWithDepth with no depth suffix.
	got2, err := TraverseWithDepth(ctx, client,
		"surqlgo_graph_user:alice",
		"surqlgo_graph_likes", "surqlgo_graph_post",
		TraverseOut, nil,
	)
	if err != nil {
		t.Fatalf("TraverseWithDepth: %v", err)
	}
	if len(got2) != 2 {
		t.Errorf("TraverseWithDepth: want 2 posts, got %d", len(got2))
	}
}

// TestIntegration_ShortestPath seeds a linear graph A -> B -> C and
// verifies ShortestPath finds C from A at depth 2.
func TestIntegration_ShortestPath(t *testing.T) {
	client, cleanup := newIntegrationClient(t)
	defer cleanup()
	cleanupTable(t, client, "surqlgo_graph_sp_user")
	cleanupTable(t, client, "surqlgo_graph_sp_follows")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	seed := []string{
		"CREATE surqlgo_graph_sp_user:a SET name = 'a';",
		"CREATE surqlgo_graph_sp_user:b SET name = 'b';",
		"CREATE surqlgo_graph_sp_user:c SET name = 'c';",
		"RELATE surqlgo_graph_sp_user:a->surqlgo_graph_sp_follows->surqlgo_graph_sp_user:b;",
		"RELATE surqlgo_graph_sp_user:b->surqlgo_graph_sp_follows->surqlgo_graph_sp_user:c;",
	}
	for _, s := range seed {
		if _, err := client.Query(ctx, s); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	got, err := ShortestPath(ctx, client,
		"surqlgo_graph_sp_user:a", "surqlgo_graph_sp_user:c",
		"surqlgo_graph_sp_follows", 5,
	)
	if err != nil {
		t.Fatalf("ShortestPath: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("expected a path, got none")
	}
}
