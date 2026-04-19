//go:build integration
// +build integration

package query

import (
	"context"
	"testing"
	"time"
)

// TestIntegration_UpsertMany_RoundTrip upserts two rows, then upserts
// them again with a changed field, and verifies the second call
// updates rather than inserts duplicates.
func TestIntegration_UpsertMany_RoundTrip(t *testing.T) {
	client, cleanup := newIntegrationClient(t)
	defer cleanup()
	cleanupTable(t, client, "surqlgo_batch_upsert")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// First batch.
	items := []map[string]any{
		{"id": "surqlgo_batch_upsert:alice", "name": "Alice", "age": 30},
		{"id": "surqlgo_batch_upsert:bob", "name": "Bob", "age": 25},
	}
	if _, err := UpsertMany(ctx, client, "surqlgo_batch_upsert", items, nil); err != nil {
		t.Fatalf("UpsertMany initial: %v", err)
	}

	n, err := CountRecords(ctx, client, "surqlgo_batch_upsert", nil)
	if err != nil {
		t.Fatalf("CountRecords after insert: %v", err)
	}
	if n != 2 {
		t.Fatalf("want 2 rows after first upsert, got %d", n)
	}

	// Re-upsert with a changed age — row count must stay at 2.
	items[0]["age"] = 31
	if _, err := UpsertMany(ctx, client, "surqlgo_batch_upsert", items, nil); err != nil {
		t.Fatalf("UpsertMany repeat: %v", err)
	}
	n2, err := CountRecords(ctx, client, "surqlgo_batch_upsert", nil)
	if err != nil {
		t.Fatalf("CountRecords after repeat: %v", err)
	}
	if n2 != 2 {
		t.Fatalf("row count diverged after repeat upsert: want 2, got %d", n2)
	}
}
