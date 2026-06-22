package schema

import (
	stdErrors "errors"
	"testing"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

func TestParseBucket(t *testing.T) {
	tests := []struct {
		name         string
		def          string
		wantBackend  string
		wantReadOnly bool
		wantComment  string
		wantPerm     string // "" / "FULL" / "NONE"
	}{
		{
			name:        "double-quoted backend",
			def:         `DEFINE BUCKET assets BACKEND "memory"`,
			wantBackend: "memory",
		},
		{
			name:        "single-quoted backend (INFO form)",
			def:         "DEFINE BUCKET assets BACKEND 'memory'\n      PERMISSIONS FULL",
			wantBackend: "memory",
			wantPerm:    "FULL",
		},
		{
			name:         "readonly",
			def:          `DEFINE BUCKET assets BACKEND 'file:/data' READONLY`,
			wantBackend:  "file:/data",
			wantReadOnly: true,
		},
		{
			name:        "comment",
			def:         `DEFINE BUCKET assets BACKEND 'memory' COMMENT 'hi there'`,
			wantBackend: "memory",
			wantComment: "hi there",
		},
		{
			name:        "permissions none",
			def:         `DEFINE BUCKET assets BACKEND 'memory' PERMISSIONS NONE`,
			wantBackend: "memory",
			wantPerm:    "NONE",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bd, err := ParseBucket("assets", tt.def)
			if err != nil {
				t.Fatalf("ParseBucket: %v", err)
			}
			if bd.Backend != tt.wantBackend {
				t.Errorf("Backend = %q, want %q", bd.Backend, tt.wantBackend)
			}
			if bd.ReadOnly != tt.wantReadOnly {
				t.Errorf("ReadOnly = %v, want %v", bd.ReadOnly, tt.wantReadOnly)
			}
			if bd.Comment != tt.wantComment {
				t.Errorf("Comment = %q, want %q", bd.Comment, tt.wantComment)
			}
			switch tt.wantPerm {
			case "":
				if bd.Permissions != nil {
					t.Errorf("Permissions = %v, want nil", bd.Permissions)
				}
			default:
				if got := bd.Permissions["*"]; got != tt.wantPerm {
					t.Errorf("Permissions[*] = %q, want %q", got, tt.wantPerm)
				}
			}
		})
	}
}

func TestParseBucket_Errors(t *testing.T) {
	if _, err := ParseBucket("", "DEFINE BUCKET x BACKEND 'memory'"); err == nil {
		t.Error("expected error for empty name")
	} else if !stdErrors.Is(err, surqlerrors.ErrSchemaParse) {
		t.Errorf("error kind = %v, want ErrSchemaParse", err)
	}
	if _, err := ParseBucket("x", "   "); err == nil {
		t.Error("expected error for empty definition")
	}
}

func TestParseDBInfo_Buckets(t *testing.T) {
	info := map[string]any{
		"buckets": map[string]any{
			"assets": "DEFINE BUCKET assets BACKEND 'memory'\n      PERMISSIONS FULL",
		},
	}
	dbInfo, err := ParseDBInfo(info)
	if err != nil {
		t.Fatalf("ParseDBInfo: %v", err)
	}
	b, ok := dbInfo.Buckets["assets"]
	if !ok {
		t.Fatal("bucket assets not parsed")
	}
	if b.Backend != "memory" {
		t.Errorf("Backend = %q, want memory", b.Backend)
	}
	if b.Permissions["*"] != "FULL" {
		t.Errorf("Permissions[*] = %q, want FULL", b.Permissions["*"])
	}
}

func TestParseDBInfo_Buckets_ShortKey(t *testing.T) {
	info := map[string]any{
		"bu": map[string]any{
			"cache": `DEFINE BUCKET cache BACKEND "memory"`,
		},
	}
	dbInfo, err := ParseDBInfo(info)
	if err != nil {
		t.Fatalf("ParseDBInfo: %v", err)
	}
	if _, ok := dbInfo.Buckets["cache"]; !ok {
		t.Fatal("bucket cache not parsed from 'bu' key")
	}
}

func TestParseDBInfo_NilHasBucketsMap(t *testing.T) {
	dbInfo, err := ParseDBInfo(nil)
	if err != nil {
		t.Fatalf("ParseDBInfo(nil): %v", err)
	}
	if dbInfo.Buckets == nil {
		t.Error("Buckets map should be non-nil")
	}
}
