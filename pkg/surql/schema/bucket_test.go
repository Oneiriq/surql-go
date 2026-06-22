package schema

import (
	stdErrors "errors"
	"testing"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

func TestNewBucket_Defaults(t *testing.T) {
	b := NewBucket("assets", "memory")
	if b.Name != "assets" {
		t.Errorf("Name = %q, want assets", b.Name)
	}
	if b.Backend != "memory" {
		t.Errorf("Backend = %q, want memory", b.Backend)
	}
	if b.ReadOnly {
		t.Error("ReadOnly should default false")
	}
	if b.Comment != "" {
		t.Errorf("Comment = %q, want empty", b.Comment)
	}
}

func TestBucketConstructors(t *testing.T) {
	if got := MemoryBucket("a").Backend; got != "memory" {
		t.Errorf("MemoryBucket backend = %q", got)
	}
	if got := FileBucket("a", "/srv/data").Backend; got != "file:/srv/data" {
		t.Errorf("FileBucket backend = %q", got)
	}
	if got := S3Bucket("a", "my-bucket/prefix").Backend; got != "s3://my-bucket/prefix" {
		t.Errorf("S3Bucket backend = %q", got)
	}
}

func TestBucketDefinition_ToSurql(t *testing.T) {
	tests := []struct {
		name   string
		bucket BucketDefinition
		want   string
	}{
		{
			name:   "minimal",
			bucket: NewBucket("assets", "memory"),
			want:   `DEFINE BUCKET assets BACKEND "memory";`,
		},
		{
			name:   "readonly",
			bucket: NewBucket("assets", "memory", WithBucketReadOnly(true)),
			want:   `DEFINE BUCKET assets BACKEND "memory" READONLY;`,
		},
		{
			name:   "comment",
			bucket: NewBucket("assets", "file:/data", WithBucketComment("user uploads")),
			want:   `DEFINE BUCKET assets BACKEND "file:/data" COMMENT "user uploads";`,
		},
		{
			name: "permissions full",
			bucket: NewBucket("assets", "memory",
				WithBucketPermissions(map[string]string{"*": "FULL"})),
			want: `DEFINE BUCKET assets BACKEND "memory" PERMISSIONS FULL;`,
		},
		{
			name: "permissions none",
			bucket: NewBucket("assets", "memory",
				WithBucketPermissions(map[string]string{"*": "NONE"})),
			want: `DEFINE BUCKET assets BACKEND "memory" PERMISSIONS NONE;`,
		},
		{
			name: "permissions per-action sorted",
			bucket: NewBucket("assets", "memory",
				WithBucketPermissions(map[string]string{
					"select": "true",
					"create": "$auth.admin = true",
				})),
			want: `DEFINE BUCKET assets BACKEND "memory" PERMISSIONS FOR create WHERE $auth.admin = true FOR select WHERE true;`,
		},
		{
			name: "all clauses",
			bucket: NewBucket("assets", "s3://b/p",
				WithBucketReadOnly(true),
				WithBucketPermissions(map[string]string{"*": "FULL"}),
				WithBucketComment("c")),
			want: `DEFINE BUCKET assets BACKEND "s3://b/p" READONLY PERMISSIONS FULL COMMENT "c";`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.bucket.ToSurql(); got != tt.want {
				t.Errorf("ToSurql() =\n  %q\nwant\n  %q", got, tt.want)
			}
		})
	}
}

func TestBucketDefinition_ToSurqlIfNotExists(t *testing.T) {
	b := NewBucket("assets", "memory")
	want := `DEFINE BUCKET IF NOT EXISTS assets BACKEND "memory";`
	if got := b.ToSurqlIfNotExists(); got != want {
		t.Errorf("ToSurqlIfNotExists() = %q, want %q", got, want)
	}
}

func TestBucketDefinition_ToSurqlOverwrite(t *testing.T) {
	b := NewBucket("assets", "memory")
	want := `DEFINE BUCKET OVERWRITE assets BACKEND "memory";`
	if got := b.ToSurqlOverwrite(); got != want {
		t.Errorf("ToSurqlOverwrite() = %q, want %q", got, want)
	}
}

func TestBucketDefinition_ToRemoveSurql(t *testing.T) {
	b := NewBucket("assets", "memory")
	if got := b.ToRemoveSurql(); got != "REMOVE BUCKET assets;" {
		t.Errorf("ToRemoveSurql() = %q", got)
	}
	if got := RemoveBucketSurql("other"); got != "REMOVE BUCKET other;" {
		t.Errorf("RemoveBucketSurql() = %q", got)
	}
}

func TestBucketDefinition_Validate(t *testing.T) {
	tests := []struct {
		name    string
		bucket  BucketDefinition
		wantErr bool
	}{
		{"valid", NewBucket("assets", "memory"), false},
		{"empty name", NewBucket("", "memory"), true},
		{"bad name", NewBucket("1bad", "memory"), true},
		{"name with dash", NewBucket("a-b", "memory"), true},
		{"empty backend", NewBucket("assets", ""), true},
		{"whitespace backend", NewBucket("assets", "   "), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.bucket.Validate()
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if tt.wantErr && err != nil && !stdErrors.Is(err, surqlerrors.ErrValidation) {
				t.Errorf("error kind = %v, want ErrValidation", err)
			}
		})
	}
}

func TestAlterBucketSurql(t *testing.T) {
	ptrBool := func(b bool) *bool { return &b }
	ptrStr := func(s string) *string { return &s }

	tests := []struct {
		name     string
		bucket   string
		change   AlterBucketChange
		ifExists bool
		want     string
	}{
		{
			name:   "set readonly",
			bucket: "assets",
			change: AlterBucketChange{ReadOnly: ptrBool(true)},
			want:   "ALTER BUCKET assets READONLY;",
		},
		{
			name:   "drop readonly",
			bucket: "assets",
			change: AlterBucketChange{ReadOnly: ptrBool(false)},
			want:   "ALTER BUCKET assets DROP READONLY;",
		},
		{
			name:   "set backend",
			bucket: "assets",
			change: AlterBucketChange{Backend: ptrStr("file:/new")},
			want:   `ALTER BUCKET assets BACKEND "file:/new";`,
		},
		{
			name:   "drop backend",
			bucket: "assets",
			change: AlterBucketChange{DropBackend: true},
			want:   "ALTER BUCKET assets DROP BACKEND;",
		},
		{
			name:   "set comment",
			bucket: "assets",
			change: AlterBucketChange{Comment: ptrStr("hi")},
			want:   `ALTER BUCKET assets COMMENT "hi";`,
		},
		{
			name:   "drop comment",
			bucket: "assets",
			change: AlterBucketChange{DropComment: true},
			want:   "ALTER BUCKET assets DROP COMMENT;",
		},
		{
			name:     "if exists with permissions",
			bucket:   "assets",
			change:   AlterBucketChange{Permissions: map[string]string{"*": "NONE"}},
			ifExists: true,
			want:     "ALTER BUCKET IF EXISTS assets PERMISSIONS NONE;",
		},
		{
			name:   "drop backend takes precedence over set",
			bucket: "assets",
			change: AlterBucketChange{Backend: ptrStr("ignored"), DropBackend: true},
			want:   "ALTER BUCKET assets DROP BACKEND;",
		},
		{
			name:   "all clauses ordered",
			bucket: "assets",
			change: AlterBucketChange{
				ReadOnly:    ptrBool(true),
				Backend:     ptrStr("memory"),
				Permissions: map[string]string{"*": "FULL"},
				Comment:     ptrStr("c"),
			},
			want: `ALTER BUCKET assets READONLY BACKEND "memory" PERMISSIONS FULL COMMENT "c";`,
		},
		{
			name:   "empty change no-op",
			bucket: "assets",
			change: AlterBucketChange{},
			want:   "ALTER BUCKET assets;",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AlterBucketSurql(tt.bucket, tt.change, tt.ifExists); got != tt.want {
				t.Errorf("AlterBucketSurql() =\n  %q\nwant\n  %q", got, tt.want)
			}
		})
	}
}

func TestGenerateBucketSQL(t *testing.T) {
	stmts, err := GenerateBucketSQL(NewBucket("assets", "memory"))
	if err != nil {
		t.Fatalf("GenerateBucketSQL: %v", err)
	}
	if len(stmts) != 1 || stmts[0] != `DEFINE BUCKET assets BACKEND "memory";` {
		t.Errorf("GenerateBucketSQL = %v", stmts)
	}

	if _, err := GenerateBucketSQL(NewBucket("", "memory")); err == nil {
		t.Error("expected validation error for empty name")
	}

	ineStmts, err := GenerateBucketSQLIfNotExists(NewBucket("assets", "memory"))
	if err != nil {
		t.Fatalf("GenerateBucketSQLIfNotExists: %v", err)
	}
	if ineStmts[0] != `DEFINE BUCKET IF NOT EXISTS assets BACKEND "memory";` {
		t.Errorf("GenerateBucketSQLIfNotExists = %v", ineStmts)
	}
}

func TestGenerateSchemaSQLFromSlicesWithBuckets(t *testing.T) {
	buckets := []BucketDefinition{NewBucket("assets", "memory")}
	tables := []TableDefinition{NewTable("user")}
	sql, err := GenerateSchemaSQLFromSlicesWithBuckets(buckets, tables, nil, false)
	if err != nil {
		t.Fatalf("GenerateSchemaSQLFromSlicesWithBuckets: %v", err)
	}
	// Bucket DDL must appear before the table DDL.
	bucketIdx := indexOfLine(sql, `DEFINE BUCKET assets BACKEND "memory";`)
	tableIdx := indexOfLine(sql, "DEFINE TABLE user SCHEMAFULL;")
	if bucketIdx < 0 || tableIdx < 0 {
		t.Fatalf("missing statements in:\n%s", sql)
	}
	if bucketIdx > tableIdx {
		t.Errorf("bucket DDL should precede table DDL:\n%s", sql)
	}
}

func indexOfLine(s, line string) int {
	idx := 0
	for _, ln := range splitLines(s) {
		if ln == line {
			return idx
		}
		idx++
	}
	return -1
}

func splitLines(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == '\n' {
			out = append(out, cur)
			cur = ""
			continue
		}
		cur += string(r)
	}
	out = append(out, cur)
	return out
}
