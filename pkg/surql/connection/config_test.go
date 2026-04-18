package connection

import (
	"errors"
	"math"
	"testing"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

func TestDefaultsAreValid(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if cfg.URL() != "ws://localhost:8000/rpc" {
		t.Errorf("url: %q", cfg.URL())
	}
	if cfg.Namespace() != "development" || cfg.Database() != "main" {
		t.Errorf("ns=%q db=%q", cfg.Namespace(), cfg.Database())
	}
	if !cfg.EnableLiveQueries {
		t.Error("live queries should default to enabled")
	}
}

func TestValidateRejectsEmptyURL(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DBURL = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error")
	} else if !errors.Is(err, surqlerrors.ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

func TestValidateRejectsUnsupportedProtocol(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DBURL = "ftp://localhost"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateAcceptsEmbeddedProtocols(t *testing.T) {
	for _, url := range []string{
		"mem://",
		"memory://",
		"file:///tmp/db.sdb",
		"surrealkv:///tmp/db.skv",
	} {
		cfg := DefaultConfig()
		cfg.DBURL = url
		if err := cfg.Validate(); err != nil {
			t.Errorf("%s: %v", url, err)
		}
	}
}

func TestLiveQueriesRejectedOverHTTP(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DBURL = "https://db.example.com/rpc"
	cfg.EnableLiveQueries = true
	if err := cfg.Validate(); err == nil {
		t.Error("expected error")
	}
	cfg.EnableLiveQueries = false
	if err := cfg.Validate(); err != nil {
		t.Errorf("https with live queries off should be valid: %v", err)
	}
}

func TestInvalidIdentifiers(t *testing.T) {
	for _, bad := range []string{"", "has space", "has/slash", "has!bang"} {
		cfg := DefaultConfig()
		cfg.DBNS = bad
		if err := cfg.Validate(); err == nil {
			t.Errorf("ns %q should be invalid", bad)
		}
	}
}

func TestRetryMaxMustExceedMin(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DBRetryMinWait = 5.0
	cfg.DBRetryMaxWait = 3.0
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error")
	}
}

func TestProtocolDetection(t *testing.T) {
	cases := []struct {
		url      string
		protocol Protocol
	}{
		{"ws://localhost:8000", ProtocolWebSocket},
		{"wss://host/rpc", ProtocolWebSocketSecure},
		{"http://host", ProtocolHTTP},
		{"https://host", ProtocolHTTPS},
		{"mem://", ProtocolMemory},
		{"memory://", ProtocolMemory},
		{"file:///tmp/db", ProtocolFile},
		{"surrealkv:///tmp/db", ProtocolSurrealKV},
	}
	for _, tc := range cases {
		cfg := DefaultConfig()
		cfg.DBURL = tc.url
		cfg.EnableLiveQueries = tc.protocol.SupportsLiveQueries()
		if err := cfg.Validate(); err != nil {
			t.Fatalf("%s: validate: %v", tc.url, err)
		}
		p, err := cfg.Protocol()
		if err != nil {
			t.Fatalf("%s: protocol: %v", tc.url, err)
		}
		if p != tc.protocol {
			t.Errorf("%s: got %v, want %v", tc.url, p, tc.protocol)
		}
	}
}

func TestProtocolHelpers(t *testing.T) {
	if !ProtocolWebSocket.SupportsLiveQueries() || !ProtocolMemory.SupportsLiveQueries() {
		t.Error("ws/memory should support live queries")
	}
	if ProtocolHTTP.SupportsLiveQueries() || ProtocolHTTPS.SupportsLiveQueries() {
		t.Error("http(s) should not support live queries")
	}
	if !ProtocolMemory.IsEmbedded() {
		t.Error("memory should be embedded")
	}
	if ProtocolWebSocket.IsEmbedded() {
		t.Error("ws should not be embedded")
	}
}

func TestLoadConfigFromMap(t *testing.T) {
	prefix := "SURQL_TEST_CFG_"
	env := map[string]string{
		prefix + "URL":                 "wss://env.example/rpc",
		prefix + "NAMESPACE":           "envns",
		prefix + "DATABASE":            "envdb",
		prefix + "USERNAME":            "envuser",
		prefix + "TIMEOUT":             "45.5",
		prefix + "ENABLE_LIVE_QUERIES": "false",
	}
	cfg, err := LoadConfigFromMap(prefix, env)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.URL() != "wss://env.example/rpc" {
		t.Errorf("url: %q", cfg.URL())
	}
	if cfg.Namespace() != "envns" || cfg.Database() != "envdb" {
		t.Errorf("ns/db: %q/%q", cfg.Namespace(), cfg.Database())
	}
	if cfg.Username() == nil || *cfg.Username() != "envuser" {
		t.Errorf("username: %v", cfg.Username())
	}
	if math.Abs(cfg.Timeout()-45.5) > 1e-9 {
		t.Errorf("timeout: %v", cfg.Timeout())
	}
	if cfg.EnableLiveQueries {
		t.Error("enable_live_queries should be false")
	}
}

func TestLoadConfigAcceptsLegacyAliases(t *testing.T) {
	prefix := "SURQL_LEGACY_"
	env := map[string]string{
		prefix + "DB_URL":  "ws://legacy.example/rpc",
		prefix + "DB_NS":   "legns",
		prefix + "DB":      "legdb",
		prefix + "DB_USER": "leguser",
		prefix + "DB_PASS": "legpass",
	}
	cfg, err := LoadConfigFromMap(prefix, env)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.URL() != "ws://legacy.example/rpc" {
		t.Errorf("url: %q", cfg.URL())
	}
	if cfg.Namespace() != "legns" || cfg.Database() != "legdb" {
		t.Errorf("ns/db: %q/%q", cfg.Namespace(), cfg.Database())
	}
	if cfg.Password() == nil || *cfg.Password() != "legpass" {
		t.Errorf("password: %v", cfg.Password())
	}
}

func TestLoadNamedConfigFromSource(t *testing.T) {
	prefix := "SURQL_PRIMARY_"
	env := map[string]string{
		prefix + "URL":       "ws://primary.example/rpc",
		prefix + "NAMESPACE": "pns",
		prefix + "DATABASE":  "pdb",
	}
	named, err := LoadNamedConfigFromSource("primary", func(k string) (string, bool) {
		v, ok := env[k]
		return v, ok
	})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if named.Name != "primary" {
		t.Errorf("name: %q", named.Name)
	}
	if named.Config.URL() != "ws://primary.example/rpc" {
		t.Errorf("url: %q", named.Config.URL())
	}
}

func TestLoadConfigInvalidTimeout(t *testing.T) {
	_, err := LoadConfigFromMap("X_", map[string]string{"X_TIMEOUT": "not-a-float"})
	if err == nil {
		t.Fatal("expected error")
	}
}
