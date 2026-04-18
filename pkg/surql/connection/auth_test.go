package connection

import (
	"encoding/json"
	"testing"
)

func TestAuthType_Values(t *testing.T) {
	cases := map[AuthType]string{
		AuthRoot:      "root",
		AuthNamespace: "namespace",
		AuthDatabase:  "database",
		AuthScope:     "scope",
	}
	for at, want := range cases {
		if string(at) != want {
			t.Errorf("%v: got %q, want %q", at, string(at), want)
		}
	}
}

func TestRootCredentials_Payload(t *testing.T) {
	c := NewRootCredentials("root", "secret")
	p := c.ToSigninPayload()
	if p["username"] != "root" || p["password"] != "secret" {
		t.Errorf("payload: %+v", p)
	}
	if c.AuthType() != AuthRoot {
		t.Errorf("authtype: %v", c.AuthType())
	}
}

func TestNamespaceCredentials_Payload(t *testing.T) {
	c := NewNamespaceCredentials("prod", "u", "p")
	p := c.ToSigninPayload()
	if p["namespace"] != "prod" || p["username"] != "u" || p["password"] != "p" {
		t.Errorf("payload: %+v", p)
	}
	if c.AuthType() != AuthNamespace {
		t.Error("authtype mismatch")
	}
}

func TestDatabaseCredentials_Payload(t *testing.T) {
	c := NewDatabaseCredentials("prod", "app", "u", "p")
	p := c.ToSigninPayload()
	if p["namespace"] != "prod" || p["database"] != "app" {
		t.Errorf("ns/db: %+v", p)
	}
	if p["username"] != "u" || p["password"] != "p" {
		t.Errorf("user/pass: %+v", p)
	}
	if c.AuthType() != AuthDatabase {
		t.Error("authtype mismatch")
	}
}

func TestScopeCredentials_PayloadFlattensVariables(t *testing.T) {
	c := NewScopeCredentials("prod", "app", "user").
		With("email", "a@example.com").
		With("password", "secret")
	p := c.ToSigninPayload()
	if p["namespace"] != "prod" || p["database"] != "app" || p["access"] != "user" {
		t.Errorf("base: %+v", p)
	}
	if p["email"] != "a@example.com" || p["password"] != "secret" {
		t.Errorf("vars: %+v", p)
	}
	if c.AuthType() != AuthScope {
		t.Error("authtype mismatch")
	}
}

func TestScopeCredentials_WithDoesNotMutate(t *testing.T) {
	a := NewScopeCredentials("prod", "app", "user")
	b := a.With("email", "a@example.com")
	if len(a.Variables) != 0 {
		t.Errorf("original should be untouched, got %+v", a.Variables)
	}
	if b.Variables["email"] != "a@example.com" {
		t.Errorf("derived missing email: %+v", b.Variables)
	}
}

func TestRootCredentials_OmitsEmptyPassword(t *testing.T) {
	c := RootCredentials{Username: "root"}
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if containsString(string(data), "password") {
		t.Errorf("should omit empty password: %s", data)
	}
}

func TestCredentialsInterface(t *testing.T) {
	var _ Credentials = NewRootCredentials("r", "p")
	var _ Credentials = NewNamespaceCredentials("n", "u", "p")
	var _ Credentials = NewDatabaseCredentials("n", "d", "u", "p")
	var _ Credentials = NewScopeCredentials("n", "d", "a")
}

func TestTokenAuth(t *testing.T) {
	t1 := NewTokenAuth("abc")
	if t1.Token != "abc" {
		t.Errorf("token: %q", t1.Token)
	}
}

func containsString(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
