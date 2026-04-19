package orchestration

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"sync"
	"testing"

	"github.com/Oneiriq/surql-go/pkg/surql/connection"
	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

// testConfig is a valid ConnectionConfig for registry tests. Using a
// memory:// URL means no real connection is ever attempted.
func testConfig() connection.ConnectionConfig {
	cfg := connection.DefaultConfig()
	cfg.DBURL = "memory://test"
	return cfg
}

func TestEnvironmentConfig_Validate_TableDriven(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		input   EnvironmentConfig
		wantErr error
	}{
		{
			name: "valid",
			input: EnvironmentConfig{
				Name:       "prod",
				Connection: testConfig(),
				Priority:   1,
			},
			wantErr: nil,
		},
		{
			name:    "empty name",
			input:   EnvironmentConfig{Connection: testConfig()},
			wantErr: surqlerrors.ErrValidation,
		},
		{
			name: "invalid chars",
			input: EnvironmentConfig{
				Name:       "prod!",
				Connection: testConfig(),
			},
			wantErr: surqlerrors.ErrValidation,
		},
		{
			name: "negative priority",
			input: EnvironmentConfig{
				Name:       "staging",
				Connection: testConfig(),
				Priority:   -1,
			},
			wantErr: surqlerrors.ErrValidation,
		},
		{
			name: "invalid connection",
			input: EnvironmentConfig{
				Name:       "staging",
				Connection: connection.ConnectionConfig{DBURL: "ftp://bogus"},
			},
			wantErr: surqlerrors.ErrValidation,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.input.validate()
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v; want %v", err, tc.wantErr)
			}
		})
	}
}

func TestEnvironmentConfig_TagsHelpers(t *testing.T) {
	t.Parallel()

	env := EnvironmentConfig{
		Name: "prod",
		Tags: map[string]struct{}{"critical": {}, "us-east": {}},
	}
	if !env.HasTag("critical") {
		t.Fatal("HasTag critical = false, want true")
	}
	if env.HasTag("missing") {
		t.Fatal("HasTag missing = true, want false")
	}
	got := env.TagList()
	want := []string{"critical", "us-east"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("TagList = %v, want %v", got, want)
	}
}

func TestEnvironmentConfig_NormalisedFillsDefaults(t *testing.T) {
	t.Parallel()

	env := EnvironmentConfig{Name: "staging"}.normalised()
	if env.Priority != defaultPriority {
		t.Fatalf("Priority = %d, want %d", env.Priority, defaultPriority)
	}
	if env.Tags == nil {
		t.Fatal("Tags should be non-nil after normalisation")
	}
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	t.Parallel()

	r := NewEnvironmentRegistry()
	err := r.Register("prod", testConfig(), &RegisterOptions{Priority: 1, Tags: []string{"critical"}})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	got, err := r.Get("prod")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "prod" || got.Priority != 1 {
		t.Fatalf("got %+v, want Name=prod Priority=1", got)
	}
	if !got.HasTag("critical") {
		t.Fatal("tag critical missing")
	}
	if !got.AllowDestructive {
		t.Fatal("AllowDestructive should default to true")
	}
}

func TestRegistry_Register_DuplicateRejected(t *testing.T) {
	t.Parallel()

	r := NewEnvironmentRegistry()
	if err := r.Register("prod", testConfig(), nil); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	err := r.Register("prod", testConfig(), nil)
	if err == nil {
		t.Fatal("second Register should fail")
	}
	if !errors.Is(err, surqlerrors.ErrRegistry) {
		t.Fatalf("err = %v; want ErrRegistry", err)
	}
}

func TestRegistry_Get_Missing(t *testing.T) {
	t.Parallel()

	r := NewEnvironmentRegistry()
	_, err := r.Get("missing")
	if !errors.Is(err, surqlerrors.ErrRegistry) {
		t.Fatalf("err = %v; want ErrRegistry", err)
	}
}

func TestRegistry_Unregister(t *testing.T) {
	t.Parallel()

	r := NewEnvironmentRegistry()
	if err := r.Register("prod", testConfig(), nil); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := r.Unregister("prod"); err != nil {
		t.Fatalf("Unregister: %v", err)
	}
	if _, err := r.Get("prod"); !errors.Is(err, surqlerrors.ErrRegistry) {
		t.Fatalf("post-Unregister Get should ErrRegistry, got %v", err)
	}
	if err := r.Unregister("prod"); !errors.Is(err, surqlerrors.ErrRegistry) {
		t.Fatalf("double Unregister should ErrRegistry, got %v", err)
	}
}

func TestRegistry_List_OrderedByPriority(t *testing.T) {
	t.Parallel()

	r := NewEnvironmentRegistry()
	for _, tc := range []struct {
		name     string
		priority int
	}{
		{"dev", 200},
		{"prod", 1},
		{"staging", 50},
	} {
		if err := r.Register(tc.name, testConfig(), &RegisterOptions{Priority: tc.priority}); err != nil {
			t.Fatalf("Register %s: %v", tc.name, err)
		}
	}
	got := r.List()
	want := []string{"prod", "staging", "dev"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("List = %v, want %v", got, want)
	}
}

func TestRegistry_FindByTag(t *testing.T) {
	t.Parallel()

	r := NewEnvironmentRegistry()
	mustRegister(t, r, "prod", 1, []string{"critical"})
	mustRegister(t, r, "staging", 50, []string{"critical"})
	mustRegister(t, r, "dev", 200, []string{"ephemeral"})

	got := r.FindByTag("critical")
	if len(got) != 2 {
		t.Fatalf("FindByTag critical len = %d, want 2", len(got))
	}
	// Should be priority-ordered.
	if got[0].Name != "prod" || got[1].Name != "staging" {
		names := make([]string, 0, len(got))
		for _, e := range got {
			names = append(names, e.Name)
		}
		t.Fatalf("FindByTag order = %v, want [prod staging]", names)
	}

	if got := r.FindByTag("nonexistent"); len(got) != 0 {
		t.Fatalf("FindByTag nonexistent should be empty, got %v", got)
	}
}

func TestRegistry_ConcurrentRegisterAndGet(t *testing.T) {
	t.Parallel()

	r := NewEnvironmentRegistry()
	var wg sync.WaitGroup
	const N = 50
	for i := 0; i < N; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			name := "env" + itoa(i)
			_ = r.Register(name, testConfig(), nil)
			_, _ = r.Get(name)
		}()
	}
	wg.Wait()
	if r.Len() != N {
		t.Fatalf("Len = %d, want %d", r.Len(), N)
	}
}

func TestRegistry_Clear(t *testing.T) {
	t.Parallel()

	r := NewEnvironmentRegistry()
	mustRegister(t, r, "prod", 1, nil)
	mustRegister(t, r, "staging", 50, nil)
	r.Clear()
	if r.Len() != 0 {
		t.Fatalf("after Clear Len = %d, want 0", r.Len())
	}
}

func TestLoadRegistryFromFile_RoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "environments.json")

	trueVal := true
	fileContents := map[string]any{
		"environments": []map[string]any{
			{
				"name": "prod",
				"connection": map[string]any{
					"db_url":                "memory://prod",
					"db_ns":                 "prod",
					"db":                    "main",
					"db_timeout":            30.0,
					"db_max_connections":    10,
					"db_retry_max_attempts": 3,
					"db_retry_min_wait":     1.0,
					"db_retry_max_wait":     10.0,
					"db_retry_multiplier":   2.0,
					"enable_live_queries":   true,
				},
				"priority":          1,
				"tags":              []string{"critical"},
				"require_approval":  true,
				"allow_destructive": &trueVal,
			},
		},
	}
	raw, err := json.Marshal(fileContents)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	registry, err := LoadRegistryFromFile(path)
	if err != nil {
		t.Fatalf("LoadRegistryFromFile: %v", err)
	}
	env, err := registry.Get("prod")
	if err != nil {
		t.Fatalf("Get prod: %v", err)
	}
	if env.Priority != 1 {
		t.Fatalf("Priority = %d, want 1", env.Priority)
	}
	if !env.RequireApproval {
		t.Fatal("RequireApproval = false, want true")
	}
}

func TestLoadRegistryFromFile_MissingReturnsEmpty(t *testing.T) {
	t.Parallel()

	registry, err := LoadRegistryFromFile(filepath.Join(t.TempDir(), "does-not-exist.json"))
	if err != nil {
		t.Fatalf("expected nil err for missing file, got %v", err)
	}
	if registry.Len() != 0 {
		t.Fatalf("empty registry expected, got %d entries", registry.Len())
	}
}

func TestLoadRegistryFromFile_InvalidJSON(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := LoadRegistryFromFile(path)
	if !errors.Is(err, surqlerrors.ErrSerialization) {
		t.Fatalf("err = %v; want ErrSerialization", err)
	}
}

func TestGlobalRegistry_SetAndGet(t *testing.T) {
	// Not parallel: mutates package-level state.
	original := GetRegistry()
	t.Cleanup(func() { SetRegistry(original) })

	fresh := NewEnvironmentRegistry()
	SetRegistry(fresh)
	if GetRegistry() != fresh {
		t.Fatal("SetRegistry did not replace the singleton")
	}

	// SetRegistry(nil) should install a fresh empty registry rather than panicking.
	SetRegistry(nil)
	if GetRegistry() == nil {
		t.Fatal("SetRegistry(nil) left a nil registry")
	}
	if GetRegistry().Len() != 0 {
		t.Fatal("SetRegistry(nil) did not reset to empty")
	}
}

func TestRegisterEnvironment_HitsGlobalRegistry(t *testing.T) {
	// Not parallel: mutates package-level state.
	original := GetRegistry()
	t.Cleanup(func() { SetRegistry(original) })

	SetRegistry(NewEnvironmentRegistry())
	if err := RegisterEnvironment("prod", testConfig(), nil); err != nil {
		t.Fatalf("RegisterEnvironment: %v", err)
	}
	if _, err := GetRegistry().Get("prod"); err != nil {
		t.Fatalf("global registry missing prod: %v", err)
	}
}

func TestConfigureEnvironments_ReplacesGlobal(t *testing.T) {
	// Not parallel: mutates package-level state.
	original := GetRegistry()
	t.Cleanup(func() { SetRegistry(original) })

	dir := t.TempDir()
	path := filepath.Join(dir, "env.json")
	raw := []byte(`{"environments":[{"name":"prod","connection":{"db_url":"memory://prod","db_ns":"prod","db":"main","db_timeout":30,"db_max_connections":10,"db_retry_max_attempts":3,"db_retry_min_wait":1,"db_retry_max_wait":10,"db_retry_multiplier":2,"enable_live_queries":true}}]}`)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := ConfigureEnvironments(path); err != nil {
		t.Fatalf("ConfigureEnvironments: %v", err)
	}
	names := GetRegistry().List()
	sort.Strings(names)
	if !reflect.DeepEqual(names, []string{"prod"}) {
		t.Fatalf("List = %v, want [prod]", names)
	}
}

// mustRegister registers env in r and fails the test on error.
func mustRegister(t *testing.T, r *EnvironmentRegistry, name string, priority int, tags []string) {
	t.Helper()
	if err := r.Register(name, testConfig(), &RegisterOptions{Priority: priority, Tags: tags}); err != nil {
		t.Fatalf("Register %s: %v", name, err)
	}
}

// itoa is a tiny stdlib-free int->string helper used by concurrency tests.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
