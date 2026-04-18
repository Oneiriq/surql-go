package schema

import (
	stdErrors "errors"
	"strings"
	"testing"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

func TestAccessType_IsValid(t *testing.T) {
	if !AccessTypeJWT.IsValid() {
		t.Error("JWT should be valid")
	}
	if !AccessTypeRecord.IsValid() {
		t.Error("RECORD should be valid")
	}
	if AccessType("bogus").IsValid() {
		t.Error("bogus should not be valid")
	}
}

func TestJwtAccess_ToSurql_KeyOnly(t *testing.T) {
	a := JwtAccess("api", JwtConfig{Algorithm: "HS256", Key: "secret"})
	got := a.ToSurql()
	want := "DEFINE ACCESS api ON DATABASE TYPE JWT ALGORITHM HS256 KEY 'secret';"
	if got != want {
		t.Errorf("ToSurql() = %q, want %q", got, want)
	}
}

func TestJwtAccess_ToSurql_DefaultsAlgorithm(t *testing.T) {
	a := JwtAccess("api", JwtConfig{Key: "secret"})
	got := a.ToSurql()
	if !strings.Contains(got, "ALGORITHM HS256") {
		t.Errorf("default algorithm missing: %q", got)
	}
}

func TestJwtAccess_ToSurql_WithURL(t *testing.T) {
	a := JwtAccess("api", JwtConfig{
		Algorithm: "RS256",
		URL:       "https://example.com/.well-known/jwks.json",
	})
	got := a.ToSurql()
	if !strings.Contains(got, "ALGORITHM RS256") {
		t.Errorf("ToSurql = %q", got)
	}
	if !strings.Contains(got, "URL 'https://example.com/.well-known/jwks.json'") {
		t.Errorf("URL clause missing: %q", got)
	}
}

func TestJwtAccess_ToSurql_WithIssuer(t *testing.T) {
	a := JwtAccess("api", JwtConfig{Algorithm: "HS256", Key: "s", Issuer: "me"})
	got := a.ToSurql()
	if !strings.Contains(got, "WITH ISSUER 'me'") {
		t.Errorf("issuer missing: %q", got)
	}
}

func TestRecordAccess_ToSurql_SignupOnly(t *testing.T) {
	a := RecordAccess("user_auth", RecordAccessConfig{Signup: "CREATE user SET email = $email"})
	got := a.ToSurql()
	if !strings.HasPrefix(got, "DEFINE ACCESS user_auth ON DATABASE TYPE RECORD") {
		t.Errorf("prefix wrong: %q", got)
	}
	if !strings.Contains(got, "SIGNUP (CREATE user SET email = $email)") {
		t.Errorf("signup missing: %q", got)
	}
}

func TestRecordAccess_ToSurql_SigninOnly(t *testing.T) {
	a := RecordAccess("user_auth", RecordAccessConfig{Signin: "SELECT * FROM user"})
	got := a.ToSurql()
	if !strings.Contains(got, "SIGNIN (SELECT * FROM user)") {
		t.Errorf("signin missing: %q", got)
	}
}

func TestRecordAccess_ToSurql_Both(t *testing.T) {
	a := RecordAccess("user_auth", RecordAccessConfig{
		Signup: "CREATE user",
		Signin: "SELECT * FROM user",
	})
	got := a.ToSurql()
	if !strings.Contains(got, "SIGNUP (CREATE user)") {
		t.Errorf("signup missing: %q", got)
	}
	if !strings.Contains(got, "SIGNIN (SELECT * FROM user)") {
		t.Errorf("signin missing: %q", got)
	}
}

func TestAccess_ToSurql_DurationSession(t *testing.T) {
	a := JwtAccess("api", JwtConfig{Key: "s"}, WithDurationSession("24h"))
	got := a.ToSurql()
	if !strings.Contains(got, "DURATION FOR SESSION 24h") {
		t.Errorf("session duration missing: %q", got)
	}
}

func TestAccess_ToSurql_DurationToken(t *testing.T) {
	a := JwtAccess("api", JwtConfig{Key: "s"}, WithDurationToken("15m"))
	got := a.ToSurql()
	if !strings.Contains(got, "DURATION FOR TOKEN 15m") {
		t.Errorf("token duration missing: %q", got)
	}
}

func TestAccess_ToSurql_DurationBoth(t *testing.T) {
	a := JwtAccess("api", JwtConfig{Key: "s"},
		WithDurationSession("24h"), WithDurationToken("15m"))
	got := a.ToSurql()
	if !strings.Contains(got, "FOR SESSION 24h") || !strings.Contains(got, "FOR TOKEN 15m") {
		t.Errorf("durations missing: %q", got)
	}
	// Both should appear after DURATION keyword (single occurrence).
	if strings.Count(got, "DURATION ") != 1 {
		t.Errorf("expected single DURATION keyword: %q", got)
	}
}

func TestAccess_Validate_EmptyName(t *testing.T) {
	a := AccessDefinition{Type: AccessTypeJWT, JWT: &JwtConfig{}}
	if err := a.Validate(); err == nil {
		t.Error("empty name should fail")
	}
}

func TestAccess_Validate_InvalidType(t *testing.T) {
	a := AccessDefinition{Name: "x", Type: AccessType("bogus")}
	err := a.Validate()
	if err == nil {
		t.Fatal("invalid type should fail")
	}
	if !stdErrors.Is(err, surqlerrors.ErrValidation) {
		t.Errorf("err = %v", err)
	}
}

func TestAccess_Validate_JwtRequiresConfig(t *testing.T) {
	a := AccessDefinition{Name: "api", Type: AccessTypeJWT}
	if err := a.Validate(); err == nil {
		t.Error("JWT without config should fail")
	}
}

func TestAccess_Validate_RecordRequiresConfig(t *testing.T) {
	a := AccessDefinition{Name: "api", Type: AccessTypeRecord}
	if err := a.Validate(); err == nil {
		t.Error("RECORD without config should fail")
	}
}

func TestAccess_Validate_ValidJwt(t *testing.T) {
	a := JwtAccess("api", JwtConfig{Key: "s"})
	if err := a.Validate(); err != nil {
		t.Errorf("valid JWT errored: %v", err)
	}
}

func TestAccess_Validate_ValidRecord(t *testing.T) {
	a := RecordAccess("u", RecordAccessConfig{Signin: "SELECT * FROM user"})
	if err := a.Validate(); err != nil {
		t.Errorf("valid RECORD errored: %v", err)
	}
}

func TestNewAccess_Explicit(t *testing.T) {
	a := NewAccess("x", AccessTypeJWT, WithJWT(JwtConfig{Algorithm: "HS512", Key: "k"}))
	if a.Type != AccessTypeJWT {
		t.Errorf("type = %q", string(a.Type))
	}
	if a.JWT == nil || a.JWT.Algorithm != "HS512" {
		t.Errorf("JWT config wrong: %+v", a.JWT)
	}
}

func TestAccessConfigIsolation(t *testing.T) {
	cfg := JwtConfig{Algorithm: "HS256", Key: "k"}
	a := JwtAccess("api", cfg)
	cfg.Key = "MUTATED"
	if a.JWT.Key != "k" {
		t.Errorf("expected isolated JWT config; got %q", a.JWT.Key)
	}
}
