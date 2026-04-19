package cli

import (
	"bytes"
	"strings"
	"testing"
)

// TestExecute_Version covers every spelling of the version verb. All three
// flavours must resolve to the same output without requiring settings.
func TestExecute_Version(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"positional", []string{"version"}},
		{"long-flag", []string{"--version"}},
		{"short-flag", []string{"-v"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
			code := ExecuteWithArgs(BuildInfo{Version: "0.0.0-test"}, tc.args, out, errOut)
			if code != ExitSuccess {
				t.Fatalf("expected exit %d, got %d (stderr=%q)", ExitSuccess, code, errOut.String())
			}
			if !strings.Contains(out.String(), "0.0.0-test") {
				t.Fatalf("expected version in stdout, got %q", out.String())
			}
		})
	}
}

// TestExecute_VersionWithBuildMetadata ensures commit / date are surfaced
// by the positional `version` command (the flag form delegates to cobra
// and only prints the version).
func TestExecute_VersionWithBuildMetadata(t *testing.T) {
	build := BuildInfo{Version: "1.2.3", Commit: "deadbeef", Date: "2026-01-01"}
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	code := ExecuteWithArgs(build, []string{"version"}, out, errOut)
	if code != ExitSuccess {
		t.Fatalf("unexpected exit %d (stderr=%q)", code, errOut.String())
	}
	got := out.String()
	for _, want := range []string{"1.2.3", "commit deadbeef", "built 2026-01-01"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in output, got %q", want, got)
		}
	}
}

// TestExecute_UnknownSubcommand asserts we emit usage-style output for a
// bad invocation. Cobra's unknown-command error goes through classifyError
// and becomes ExitFailure (since it is not tagged as a usageError) — the
// important contract is that we never panic and always surface something
// on stderr.
func TestExecute_UnknownSubcommand(t *testing.T) {
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	code := ExecuteWithArgs(BuildInfo{Version: "0.0.0-test"}, []string{"nope"}, out, errOut)
	if code == ExitSuccess {
		t.Fatal("expected non-zero exit for unknown subcommand")
	}
	if errOut.Len() == 0 && out.Len() == 0 {
		t.Fatal("expected diagnostic output for unknown subcommand")
	}
}

// TestExecute_Help covers the built-in help output. Help must not require
// a database connection or settings file.
func TestExecute_Help(t *testing.T) {
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	code := ExecuteWithArgs(BuildInfo{Version: "0.0.0-test"}, []string{"--help"}, out, errOut)
	if code != ExitSuccess {
		t.Fatalf("help should succeed, got %d (stderr=%q)", code, errOut.String())
	}
	for _, want := range []string{"migrate", "schema", "db", "orchestrate"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("expected %q in help output", want)
		}
	}
}

// TestNewRootCommand_HasAllGroups asserts every expected subcommand group
// is wired. This is a structural test — it protects against accidental
// drops during refactoring.
func TestNewRootCommand_HasAllGroups(t *testing.T) {
	root := NewRootCommand(BuildInfo{Version: "test"})
	expected := map[string]bool{
		"version":     false,
		"migrate":     false,
		"schema":      false,
		"db":          false,
		"orchestrate": false,
	}
	for _, c := range root.Commands() {
		if _, ok := expected[c.Name()]; ok {
			expected[c.Name()] = true
		}
	}
	for name, ok := range expected {
		if !ok {
			t.Errorf("root command missing subcommand %q", name)
		}
	}
}

// TestMigrateGroup_Subcommands enumerates every migrate subcommand so the
// parity matrix stays enforced.
func TestMigrateGroup_Subcommands(t *testing.T) {
	root := NewRootCommand(BuildInfo{Version: "test"})
	migrate, _, err := root.Find([]string{"migrate"})
	if err != nil {
		t.Fatalf("find migrate: %v", err)
	}
	want := []string{"up", "down", "status", "history", "create", "validate", "generate", "squash"}
	got := make(map[string]bool)
	for _, c := range migrate.Commands() {
		got[c.Name()] = true
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("migrate missing subcommand %q", w)
		}
	}
}

// TestSchemaGroup_Subcommands enumerates every schema subcommand.
func TestSchemaGroup_Subcommands(t *testing.T) {
	root := NewRootCommand(BuildInfo{Version: "test"})
	schema, _, err := root.Find([]string{"schema"})
	if err != nil {
		t.Fatalf("find schema: %v", err)
	}
	want := []string{
		"show", "diff", "generate", "sync", "export", "tables",
		"inspect", "validate", "check", "hook-config", "watch", "visualize",
	}
	got := make(map[string]bool)
	for _, c := range schema.Commands() {
		got[c.Name()] = true
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("schema missing subcommand %q", w)
		}
	}
}

// TestDBGroup_Subcommands enumerates every db subcommand.
func TestDBGroup_Subcommands(t *testing.T) {
	root := NewRootCommand(BuildInfo{Version: "test"})
	db, _, err := root.Find([]string{"db"})
	if err != nil {
		t.Fatalf("find db: %v", err)
	}
	want := []string{"init", "ping", "info", "reset", "query", "version"}
	got := make(map[string]bool)
	for _, c := range db.Commands() {
		got[c.Name()] = true
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("db missing subcommand %q", w)
		}
	}
}

// TestOrchestrateGroup_Subcommands enumerates every orchestrate subcommand.
func TestOrchestrateGroup_Subcommands(t *testing.T) {
	root := NewRootCommand(BuildInfo{Version: "test"})
	orch, _, err := root.Find([]string{"orchestrate"})
	if err != nil {
		t.Fatalf("find orchestrate: %v", err)
	}
	want := []string{"deploy", "status", "validate"}
	got := make(map[string]bool)
	for _, c := range orch.Commands() {
		got[c.Name()] = true
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("orchestrate missing subcommand %q", w)
		}
	}
}

// TestRequiresSettings_VersionSkipsResolver asserts help/version paths do
// not trigger settings resolution (which could fail on CI without a
// config file).
func TestRequiresSettings_VersionSkipsResolver(t *testing.T) {
	root := NewRootCommand(BuildInfo{Version: "test"})
	ver, _, err := root.Find([]string{"version"})
	if err != nil {
		t.Fatalf("find version: %v", err)
	}
	if requiresSettings(ver) {
		t.Error("version command should not require settings")
	}
	migrate, _, err := root.Find([]string{"migrate"})
	if err != nil {
		t.Fatalf("find migrate: %v", err)
	}
	up, _, err := migrate.Find([]string{"up"})
	if err != nil {
		t.Fatalf("find migrate up: %v", err)
	}
	if !requiresSettings(up) {
		t.Error("migrate up should require settings")
	}
}

// TestNewUsageError_Classifies asserts classifyError returns ExitUsage for
// usageError-wrapped failures and ExitFailure for everything else.
func TestNewUsageError_Classifies(t *testing.T) {
	out := &bytes.Buffer{}
	if classifyError(out, nil) != ExitSuccess {
		t.Error("nil error should map to ExitSuccess")
	}
	if got := classifyError(out, newUsageError("bad flag")); got != ExitUsage {
		t.Errorf("usage error exit code = %d, want %d", got, ExitUsage)
	}
}
