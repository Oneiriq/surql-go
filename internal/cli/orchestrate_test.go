package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSplitCSVArg covers common input shapes.
func TestSplitCSVArg(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"   ", nil},
		{"dev", []string{"dev"}},
		{"dev, staging ,prod", []string{"dev", "staging", "prod"}},
		{",, ,", nil},
	}
	for _, tc := range cases {
		got := splitCSVArg(tc.in)
		if len(got) != len(tc.want) {
			t.Fatalf("splitCSVArg(%q) length mismatch: got %v, want %v", tc.in, got, tc.want)
		}
		for i, v := range got {
			if v != tc.want[i] {
				t.Errorf("splitCSVArg(%q)[%d] = %q, want %q", tc.in, i, v, tc.want[i])
			}
		}
	}
}

// TestBoolMarker is a trivial sanity check for the helper used by the
// orchestrate validate table.
func TestBoolMarker(t *testing.T) {
	if boolMarker(true) != "ok" {
		t.Error("true -> ok")
	}
	if boolMarker(false) != "fail" {
		t.Error("false -> fail")
	}
}

// TestOrchestrateDeploy_RequiresFlags asserts both --plan and
// --environments are enforced.
func TestOrchestrateDeploy_RequiresFlags(t *testing.T) {
	cases := [][]string{
		{"orchestrate", "deploy", "--no-color"},
		{"orchestrate", "deploy", "--plan", "plan.json", "--no-color"},
		{"orchestrate", "deploy", "-e", "dev", "--no-color", "--plan", ""},
	}
	tmp := t.TempDir()
	origCwd, _ := os.Getwd()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(origCwd) //nolint:errcheck

	for _, args := range cases {
		code := ExecuteWithArgs(BuildInfo{Version: "test"}, args, nilWriter{}, nilWriter{})
		if code == ExitSuccess {
			t.Errorf("expected failure for %v, got ExitSuccess", args)
		}
	}
}

// TestOrchestrateStatus_MissingPlan returns a usage error when --plan is
// blank.
func TestOrchestrateStatus_MissingPlan(t *testing.T) {
	tmp := t.TempDir()
	origCwd, _ := os.Getwd()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(origCwd) //nolint:errcheck
	args := []string{"orchestrate", "status", "--plan", "", "--no-color"}
	code := ExecuteWithArgs(BuildInfo{Version: "test"}, args, nilWriter{}, nilWriter{})
	if code == ExitSuccess {
		t.Fatal("expected failure when --plan is blank")
	}
}

// TestOrchestrateValidate_MalformedPlan ensures a malformed registry file
// surfaces as a runtime failure (not a panic).
func TestOrchestrateValidate_MalformedPlan(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "envs.json")
	if err := os.WriteFile(path, []byte("{not json}"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	tmp := t.TempDir()
	origCwd, _ := os.Getwd()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(origCwd) //nolint:errcheck

	args := []string{"orchestrate", "validate", "--plan", path, "--no-color"}
	code := ExecuteWithArgs(BuildInfo{Version: "test"}, args, nilWriter{}, nilWriter{})
	if code == ExitSuccess {
		t.Fatal("expected failure for malformed plan")
	}
}
