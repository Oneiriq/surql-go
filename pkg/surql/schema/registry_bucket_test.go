package schema

import "testing"

func TestSchemaRegistry_Buckets(t *testing.T) {
	r := NewSchemaRegistry()
	if r.BucketCount() != 0 {
		t.Fatalf("fresh registry BucketCount = %d", r.BucketCount())
	}
	if err := r.RegisterBucket(NewBucket("b2", "memory")); err != nil {
		t.Fatalf("RegisterBucket b2: %v", err)
	}
	if err := r.RegisterBucket(NewBucket("b1", "file:/data")); err != nil {
		t.Fatalf("RegisterBucket b1: %v", err)
	}
	if r.BucketCount() != 2 {
		t.Errorf("BucketCount = %d, want 2", r.BucketCount())
	}

	// Sorted order.
	names := r.BucketNames()
	if len(names) != 2 || names[0] != "b1" || names[1] != "b2" {
		t.Errorf("BucketNames = %v, want [b1 b2]", names)
	}
	buckets := r.Buckets()
	if len(buckets) != 2 || buckets[0].Name != "b1" || buckets[1].Name != "b2" {
		t.Errorf("Buckets not sorted: %v", buckets)
	}

	got, ok := r.GetBucket("b1")
	if !ok || got.Backend != "file:/data" {
		t.Errorf("GetBucket(b1) = %+v, ok=%v", got, ok)
	}
	if _, ok := r.GetBucket("missing"); ok {
		t.Error("GetBucket(missing) should be false")
	}

	// Re-register replaces.
	if err := r.RegisterBucket(NewBucket("b1", "memory")); err != nil {
		t.Fatalf("re-register: %v", err)
	}
	got, _ = r.GetBucket("b1")
	if got.Backend != "memory" {
		t.Errorf("re-register did not replace: %q", got.Backend)
	}

	r.Clear()
	if r.BucketCount() != 0 {
		t.Errorf("after Clear BucketCount = %d", r.BucketCount())
	}
}

func TestSchemaRegistry_RegisterBucket_EmptyName(t *testing.T) {
	r := NewSchemaRegistry()
	if err := r.RegisterBucket(NewBucket("", "memory")); err == nil {
		t.Error("expected error registering empty-name bucket")
	}
}

func TestGenerateSchemaSQL_IncludesBuckets(t *testing.T) {
	r := NewSchemaRegistry()
	if err := r.RegisterBucket(NewBucket("assets", "memory")); err != nil {
		t.Fatalf("RegisterBucket: %v", err)
	}
	if err := r.RegisterTable(NewTable("user")); err != nil {
		t.Fatalf("RegisterTable: %v", err)
	}
	sql, err := GenerateSchemaSQL(r, true)
	if err != nil {
		t.Fatalf("GenerateSchemaSQL: %v", err)
	}
	if want := `DEFINE BUCKET IF NOT EXISTS assets BACKEND "memory";`; !containsLine(sql, want) {
		t.Errorf("schema SQL missing bucket DDL:\n%s", sql)
	}
}

func containsLine(s, line string) bool {
	for _, ln := range splitLines(s) {
		if ln == line {
			return true
		}
	}
	return false
}
