package migration

import (
	"testing"

	"github.com/Oneiriq/surql-go/pkg/surql/schema"
)

func bucketRef(b schema.BucketDefinition) *schema.BucketDefinition { return &b }

func TestDiffOperation_BucketOps_IsValid(t *testing.T) {
	for _, op := range []DiffOperation{
		DiffOperationAddBucket, DiffOperationDropBucket, DiffOperationModifyBucket,
	} {
		if !op.IsValid() {
			t.Errorf("%s should be valid", op)
		}
	}
}

func TestDiffBuckets_Add(t *testing.T) {
	b := schema.NewBucket("assets", "memory")
	diffs, err := DiffBuckets(nil, bucketRef(b))
	if err != nil {
		t.Fatalf("DiffBuckets: %v", err)
	}
	if len(diffs) != 1 {
		t.Fatalf("got %d diffs, want 1", len(diffs))
	}
	d := diffs[0]
	if d.Operation != DiffOperationAddBucket || d.Bucket != "assets" {
		t.Errorf("diff = %+v", d)
	}
	if d.ForwardSQL != `DEFINE BUCKET assets BACKEND "memory";` {
		t.Errorf("ForwardSQL = %q", d.ForwardSQL)
	}
	if d.BackwardSQL != "REMOVE BUCKET assets;" {
		t.Errorf("BackwardSQL = %q", d.BackwardSQL)
	}
}

func TestDiffBuckets_Drop(t *testing.T) {
	b := schema.NewBucket("assets", "memory")
	diffs, err := DiffBuckets(bucketRef(b), nil)
	if err != nil {
		t.Fatalf("DiffBuckets: %v", err)
	}
	d := diffs[0]
	if d.Operation != DiffOperationDropBucket {
		t.Errorf("op = %s", d.Operation)
	}
	if d.ForwardSQL != "REMOVE BUCKET assets;" {
		t.Errorf("ForwardSQL = %q", d.ForwardSQL)
	}
	if d.BackwardSQL != `DEFINE BUCKET assets BACKEND "memory";` {
		t.Errorf("BackwardSQL = %q", d.BackwardSQL)
	}
}

func TestDiffBuckets_NoChange(t *testing.T) {
	b1 := schema.NewBucket("assets", "memory", schema.WithBucketComment("c"))
	b2 := schema.NewBucket("assets", "memory", schema.WithBucketComment("c"))
	diffs, err := DiffBuckets(bucketRef(b1), bucketRef(b2))
	if err != nil {
		t.Fatalf("DiffBuckets: %v", err)
	}
	if len(diffs) != 0 {
		t.Errorf("expected no diffs, got %v", diffs)
	}
}

func TestDiffBuckets_Modify_ReadOnly(t *testing.T) {
	oldB := schema.NewBucket("assets", "memory")
	newB := schema.NewBucket("assets", "memory", schema.WithBucketReadOnly(true))
	diffs, err := DiffBuckets(bucketRef(oldB), bucketRef(newB))
	if err != nil {
		t.Fatalf("DiffBuckets: %v", err)
	}
	d := diffs[0]
	if d.Operation != DiffOperationModifyBucket {
		t.Fatalf("op = %s", d.Operation)
	}
	if d.ForwardSQL != "ALTER BUCKET assets READONLY;" {
		t.Errorf("ForwardSQL = %q", d.ForwardSQL)
	}
	if d.BackwardSQL != "ALTER BUCKET assets DROP READONLY;" {
		t.Errorf("BackwardSQL = %q", d.BackwardSQL)
	}
}

func TestDiffBuckets_Modify_BackendAndComment(t *testing.T) {
	oldB := schema.NewBucket("assets", "memory")
	newB := schema.NewBucket("assets", "file:/data", schema.WithBucketComment("now commented"))
	diffs, err := DiffBuckets(bucketRef(oldB), bucketRef(newB))
	if err != nil {
		t.Fatalf("DiffBuckets: %v", err)
	}
	d := diffs[0]
	wantFwd := `ALTER BUCKET assets BACKEND "file:/data" COMMENT "now commented";`
	if d.ForwardSQL != wantFwd {
		t.Errorf("ForwardSQL = %q, want %q", d.ForwardSQL, wantFwd)
	}
	// Backward restores: backend back to memory, comment dropped.
	wantBwd := `ALTER BUCKET assets BACKEND "memory" DROP COMMENT;`
	if d.BackwardSQL != wantBwd {
		t.Errorf("BackwardSQL = %q, want %q", d.BackwardSQL, wantBwd)
	}
}

func TestDiffSchemas_BucketsIntegrationOrder(t *testing.T) {
	code := SchemaSnapshot{
		Buckets: []schema.BucketDefinition{
			schema.NewBucket("added", "memory"),
		},
	}
	db := SchemaSnapshot{
		Buckets: []schema.BucketDefinition{
			schema.NewBucket("removed", "memory"),
		},
	}
	diffs, err := DiffSchemas(code, db)
	if err != nil {
		t.Fatalf("DiffSchemas: %v", err)
	}
	var ops []DiffOperation
	for _, d := range diffs {
		ops = append(ops, d.Operation)
	}
	// added bucket -> ADD, removed bucket -> DROP.
	if len(ops) != 2 {
		t.Fatalf("ops = %v", ops)
	}
	if ops[0] != DiffOperationAddBucket || ops[1] != DiffOperationDropBucket {
		t.Errorf("ops = %v, want [add_bucket drop_bucket]", ops)
	}
}

func TestDiffSchemas_BucketModify(t *testing.T) {
	// Same-named bucket on both sides with a changed backend -> MODIFY.
	code := SchemaSnapshot{
		Buckets: []schema.BucketDefinition{schema.NewBucket("assets", "file:/new")},
	}
	db := SchemaSnapshot{
		Buckets: []schema.BucketDefinition{schema.NewBucket("assets", "memory")},
	}
	diffs, err := DiffSchemas(code, db)
	if err != nil {
		t.Fatalf("DiffSchemas: %v", err)
	}
	if len(diffs) != 1 || diffs[0].Operation != DiffOperationModifyBucket {
		t.Fatalf("diffs = %+v", diffs)
	}
	if diffs[0].ForwardSQL != `ALTER BUCKET assets BACKEND "file:/new";` {
		t.Errorf("ForwardSQL = %q", diffs[0].ForwardSQL)
	}
}

func TestDiffBuckets_Modify_DropPermissions(t *testing.T) {
	oldB := schema.NewBucket("assets", "memory",
		schema.WithBucketPermissions(map[string]string{"*": "FULL"}))
	newB := schema.NewBucket("assets", "memory")
	diffs, err := DiffBuckets(bucketRef(oldB), bucketRef(newB))
	if err != nil {
		t.Fatalf("DiffBuckets: %v", err)
	}
	d := diffs[0]
	// Forward clears permissions: an empty PERMISSIONS map renders no clause,
	// so the ALTER is a no-op body for permissions; backward restores FULL.
	if d.ForwardSQL != "ALTER BUCKET assets;" {
		t.Errorf("ForwardSQL = %q", d.ForwardSQL)
	}
	if d.BackwardSQL != "ALTER BUCKET assets PERMISSIONS FULL;" {
		t.Errorf("BackwardSQL = %q", d.BackwardSQL)
	}
}

func TestDiffBuckets_Modify_DropBackend(t *testing.T) {
	oldB := schema.NewBucket("assets", "memory")
	newB := schema.BucketDefinition{Name: "assets"} // empty backend
	diffs, err := DiffBuckets(bucketRef(oldB), bucketRef(newB))
	if err != nil {
		t.Fatalf("DiffBuckets: %v", err)
	}
	if diffs[0].ForwardSQL != "ALTER BUCKET assets DROP BACKEND;" {
		t.Errorf("ForwardSQL = %q", diffs[0].ForwardSQL)
	}
	if diffs[0].BackwardSQL != `ALTER BUCKET assets BACKEND "memory";` {
		t.Errorf("BackwardSQL = %q", diffs[0].BackwardSQL)
	}
}

func TestCompareSnapshots_Buckets(t *testing.T) {
	from := SchemaSnapshot{Version: "1"}
	to := SchemaSnapshot{
		Version: "2",
		Buckets: []schema.BucketDefinition{schema.NewBucket("assets", "memory")},
	}
	diffs, err := CompareSnapshots(from, to)
	if err != nil {
		t.Fatalf("CompareSnapshots: %v", err)
	}
	if len(diffs) != 1 || diffs[0].Operation != DiffOperationAddBucket {
		t.Errorf("diffs = %+v", diffs)
	}
}
