package schema

import (
	"strings"
	"testing"
)

// The rendered shapes here are the ones SurrealDB 3.2 echoes back from
// INFO FOR TABLE. A definition that renders differently from its own echo
// re-applies on every reconcile, so these compare whole strings wherever the
// echo shape is the point.

func TestDiskAnnIndex_SpellsTheDefaults(t *testing.T) {
	idx := DiskAnnIndex("vec_idx", "v", 3, DiskAnnIndexOptions{
		Distance:   DiskAnnDistanceCosine,
		VectorType: MTreeVectorF32,
	})
	got := idx.ToSurql("doc")
	want := "DEFINE INDEX vec_idx ON TABLE doc COLUMNS v DISKANN DIMENSION 3 " +
		"DIST COSINE TYPE F32 DEGREE 64 L_BUILD 100 ALPHA 1.2;"
	if got != want {
		t.Errorf("ToSurql = %q, want %q", got, want)
	}
}

func TestDiskAnnIndex_TunedTailAndHashedVector(t *testing.T) {
	idx := DiskAnnIndex("vec_idx", "v", 3, DiskAnnIndexOptions{
		Distance:     DiskAnnDistanceCosine,
		VectorType:   MTreeVectorF16,
		Degree:       48,
		LBuild:       90,
		Alpha:        "1.5",
		HashedVector: true,
	})
	got := idx.ToSurql("doc")
	want := "DEFINE INDEX vec_idx ON TABLE doc COLUMNS v DISKANN DIMENSION 3 " +
		"DIST COSINE TYPE F16 DEGREE 48 L_BUILD 90 ALPHA 1.5 HASHED_VECTOR;"
	if got != want {
		t.Errorf("ToSurql = %q, want %q", got, want)
	}
}

func TestDiskAnnIndex_Defaults(t *testing.T) {
	idx := DiskAnnIndex("e", "v", 8, DiskAnnIndexOptions{})
	if idx.DiskAnnDistance != DiskAnnDistanceEuclidean {
		t.Errorf("default distance = %q, want EUCLIDEAN", idx.DiskAnnDistance)
	}
	if idx.VectorType != MTreeVectorF32 {
		t.Errorf("default vector type = %q, want F32", idx.VectorType)
	}
	if idx.Degree != DiskAnnDefaultDegree {
		t.Errorf("default degree = %d, want %d", idx.Degree, DiskAnnDefaultDegree)
	}
	if idx.LBuild != DiskAnnDefaultLBuild {
		t.Errorf("default l_build = %d, want %d", idx.LBuild, DiskAnnDefaultLBuild)
	}
	if idx.Alpha != DiskAnnDefaultAlpha {
		t.Errorf("default alpha = %q, want %q", idx.Alpha, DiskAnnDefaultAlpha)
	}
}

func TestDiskAnnIndex_IfNotExists(t *testing.T) {
	idx := DiskAnnIndex("vec_idx", "v", 3, DiskAnnIndexOptions{})
	if got := idx.ToSurqlIfNotExists("doc"); !strings.HasPrefix(got, "DEFINE INDEX IF NOT EXISTS vec_idx") {
		t.Errorf("IF NOT EXISTS missing: %q", got)
	}
}

func TestDiskAnnIndex_EveryMetricRenders(t *testing.T) {
	cases := []struct {
		metric DiskAnnDistanceType
		want   string
	}{
		{DiskAnnDistanceCosine, "DIST COSINE"},
		{DiskAnnDistanceCosineNormalized, "DIST COSINE_NORMALIZED"},
		{DiskAnnDistanceEuclidean, "DIST EUCLIDEAN"},
		{DiskAnnDistanceInnerProduct, "DIST INNER_PRODUCT"},
	}
	for _, tc := range cases {
		idx := DiskAnnIndex("vec_idx", "v", 3, DiskAnnIndexOptions{Distance: tc.metric})
		if got := idx.ToSurql("doc"); !strings.Contains(got, tc.want) {
			t.Errorf("metric %q rendered %q, want it to contain %q", tc.metric, got, tc.want)
		}
	}
}

func TestCanonicalAlpha(t *testing.T) {
	// The engine echoes an integer ALPHA bare, so 2.0 must not render as 2.0.
	cases := []struct {
		in   float64
		want string
	}{
		{1.2, "1.2"},
		{2.0, "2"},
		{1.5, "1.5"},
	}
	for _, tc := range cases {
		if got := CanonicalAlpha(tc.in); got != tc.want {
			t.Errorf("CanonicalAlpha(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
	if CanonicalAlpha(1.2) != DiskAnnDefaultAlpha {
		t.Error("the default constant and the canonical rendering disagree")
	}
}

func TestParseIndex_DiskAnnRoundTrip(t *testing.T) {
	// Rendering, then parsing the echo, returns the same definition. This is
	// the property that keeps a reconciler from looping.
	idx := DiskAnnIndex("vec_idx", "v", 3, DiskAnnIndexOptions{
		Distance:   DiskAnnDistanceCosine,
		VectorType: MTreeVectorF16,
		Degree:     48,
		LBuild:     90,
		Alpha:      "1.5",
	})
	parsed, err := ParseIndex("vec_idx", idx.ToSurql("doc"))
	if err != nil {
		t.Fatalf("ParseIndex: %v", err)
	}
	if parsed.Type != IndexTypeDiskAnn {
		t.Errorf("type = %q, want DISKANN", parsed.Type)
	}
	if parsed.Dimension != 3 || parsed.VectorType != MTreeVectorF16 {
		t.Errorf("dimension/type = %d/%q, want 3/F16", parsed.Dimension, parsed.VectorType)
	}
	if parsed.DiskAnnDistance != DiskAnnDistanceCosine {
		t.Errorf("distance = %q, want COSINE", parsed.DiskAnnDistance)
	}
	if parsed.Degree != 48 || parsed.LBuild != 90 || parsed.Alpha != "1.5" {
		t.Errorf("tail = %d/%d/%q, want 48/90/1.5", parsed.Degree, parsed.LBuild, parsed.Alpha)
	}
	if parsed.ToSurql("doc") != idx.ToSurql("doc") {
		t.Errorf("re-render differs:\n got %q\nwant %q", parsed.ToSurql("doc"), idx.ToSurql("doc"))
	}
}

func TestParseIndex_StripsTheEngineAlphaSuffix(t *testing.T) {
	// A float ALPHA echoes as 1.2f; reading it as anything but 1.2 makes every
	// reconcile re-apply the index.
	echo := "DEFINE INDEX vec_idx ON TABLE doc COLUMNS v DISKANN DIMENSION 3 " +
		"DIST COSINE TYPE F16 DEGREE 64 L_BUILD 100 ALPHA 1.2f"
	parsed, err := ParseIndex("vec_idx", echo)
	if err != nil {
		t.Fatalf("ParseIndex: %v", err)
	}
	if parsed.Alpha != "1.2" {
		t.Errorf("alpha = %q, want %q", parsed.Alpha, "1.2")
	}
}

func TestParseIndex_IntegerAlphaEchoesBare(t *testing.T) {
	echo := "DEFINE INDEX vec_idx ON TABLE doc COLUMNS v DISKANN DIMENSION 3 " +
		"DIST COSINE TYPE F32 DEGREE 64 L_BUILD 100 ALPHA 2"
	parsed, err := ParseIndex("vec_idx", echo)
	if err != nil {
		t.Fatalf("ParseIndex: %v", err)
	}
	if parsed.Alpha != "2" {
		t.Errorf("alpha = %q, want %q", parsed.Alpha, "2")
	}
}

func TestParseIndex_DiskAnnHashedVectorAndColumns(t *testing.T) {
	idx := DiskAnnIndex("vec_idx", "v", 3, DiskAnnIndexOptions{HashedVector: true})
	parsed, err := ParseIndex("vec_idx", idx.ToSurql("doc"))
	if err != nil {
		t.Fatalf("ParseIndex: %v", err)
	}
	if !parsed.HashedVector {
		t.Error("HASHED_VECTOR not read back")
	}
	if len(parsed.Columns) != 1 || parsed.Columns[0] != "v" {
		t.Errorf("columns = %v, want [v]", parsed.Columns)
	}
}

func TestParseIndex_UnderscoredMetricsSurviveTheCapture(t *testing.T) {
	for _, metric := range []DiskAnnDistanceType{
		DiskAnnDistanceCosineNormalized,
		DiskAnnDistanceInnerProduct,
	} {
		idx := DiskAnnIndex("vec_idx", "v", 3, DiskAnnIndexOptions{Distance: metric})
		parsed, err := ParseIndex("vec_idx", idx.ToSurql("doc"))
		if err != nil {
			t.Fatalf("ParseIndex(%q): %v", metric, err)
		}
		if parsed.DiskAnnDistance != metric {
			t.Errorf("distance = %q, want %q", parsed.DiskAnnDistance, metric)
		}
	}
}

func TestVectorType_NarrowTypesAcrossKinds(t *testing.T) {
	narrow := []MTreeVectorType{MTreeVectorF16, MTreeVectorI8, MTreeVectorU8}

	// HNSW takes every element type.
	for _, vt := range narrow {
		idx := HnswIndex("feat_idx", "features", 3, HnswIndexOptions{VectorType: vt})
		if err := idx.Validate(); err != nil {
			t.Errorf("HNSW rejected %q: %v", vt, err)
		}
	}

	// MTREE parses only its historical five; the engine answers a bare parse
	// error, so the refusal carries the teaching here.
	for _, vt := range narrow {
		idx := MTreeIndex("bad_idx", "v", 3, MTreeIndexOptions{VectorType: vt})
		err := idx.Validate()
		if err == nil {
			t.Errorf("MTREE accepted %q, want a refusal", vt)
			continue
		}
		if !strings.Contains(err.Error(), "F64, F32, I64, I32, or I16") {
			t.Errorf("MTREE refusal for %q does not name the accepted set: %v", vt, err)
		}
	}

	// DISKANN takes only F32 / F16 / I8 / U8.
	for _, vt := range []MTreeVectorType{MTreeVectorF32, MTreeVectorF16, MTreeVectorI8, MTreeVectorU8} {
		idx := DiskAnnIndex("ok_idx", "v", 3, DiskAnnIndexOptions{VectorType: vt})
		if err := idx.Validate(); err != nil {
			t.Errorf("DISKANN rejected %q: %v", vt, err)
		}
	}
	for _, vt := range []MTreeVectorType{MTreeVectorF64, MTreeVectorI64, MTreeVectorI32, MTreeVectorI16} {
		idx := DiskAnnIndex("bad_idx", "v", 3, DiskAnnIndexOptions{VectorType: vt})
		err := idx.Validate()
		if err == nil {
			t.Errorf("DISKANN accepted %q, want a refusal", vt)
			continue
		}
		if !strings.Contains(err.Error(), "F32, F16, I8, or U8") {
			t.Errorf("DISKANN refusal for %q does not name the accepted set: %v", vt, err)
		}
	}
}

func TestDiskAnnIndex_RefusesAMetricFromAnotherKind(t *testing.T) {
	// An MTREE or HNSW metric aimed at DISKANN would be dropped by the
	// renderer, so it is refused rather than silently ignored.
	withMTree := DiskAnnIndex("bad_idx", "v", 3, DiskAnnIndexOptions{})
	withMTree.Distance = MTreeDistanceCosine
	if err := withMTree.Validate(); err == nil {
		t.Error("DISKANN accepted an MTREE metric, want a refusal")
	} else if !strings.Contains(err.Error(), "DiskAnnDistance") {
		t.Errorf("refusal does not name the right member: %v", err)
	}

	withHnsw := DiskAnnIndex("bad_idx", "v", 3, DiskAnnIndexOptions{})
	withHnsw.HnswDistance = HnswDistancePearson
	if err := withHnsw.Validate(); err == nil {
		t.Error("DISKANN accepted an HNSW metric, want a refusal")
	}
}

func TestDiskAnnIndex_RefusesAMissingDimension(t *testing.T) {
	idx := DiskAnnIndex("bad_idx", "v", 0, DiskAnnIndexOptions{})
	if err := idx.Validate(); err == nil {
		t.Error("DISKANN accepted a zero dimension, want a refusal")
	}
}

func TestIndexType_DiskAnnIsValid(t *testing.T) {
	if !IndexTypeDiskAnn.IsValid() {
		t.Error("IndexTypeDiskAnn should be recognised")
	}
	if string(IndexTypeDiskAnn) != "DISKANN" {
		t.Errorf("IndexTypeDiskAnn = %q, want DISKANN", IndexTypeDiskAnn)
	}
}
