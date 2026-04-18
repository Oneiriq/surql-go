package types

import "testing"

func TestIsReservedWord_CaseInsensitive(t *testing.T) {
	cases := []string{"SELECT", "select", "Where", "FROM"}
	for _, c := range cases {
		if !IsReservedWord(c) {
			t.Errorf("expected %q to be reserved", c)
		}
	}
}

func TestIsReservedWord_SafeNames(t *testing.T) {
	safe := []string{"username", "user_name", "email", "created_at"}
	for _, c := range safe {
		if IsReservedWord(c) {
			t.Errorf("expected %q to be safe", c)
		}
	}
}

func TestCheckReservedWord_DotNotationLeaf(t *testing.T) {
	if got := CheckReservedWord("user.name", false); got != "" {
		t.Errorf("expected safe for user.name, got %q", got)
	}
	if got := CheckReservedWord("user.select", false); got == "" {
		t.Error("expected warning for user.select")
	}
}

func TestCheckReservedWord_EdgeAllowed(t *testing.T) {
	if got := CheckReservedWord("in", true); got != "" {
		t.Errorf("expected `in` safe with allowEdgeFields, got %q", got)
	}
	if got := CheckReservedWord("out", true); got != "" {
		t.Errorf("expected `out` safe with allowEdgeFields, got %q", got)
	}
	if got := CheckReservedWord("in", false); got == "" {
		t.Error("expected warning for `in` without allowEdgeFields")
	}
}

func TestCheckReservedWord_MessageContainsName(t *testing.T) {
	msg := CheckReservedWord("select", false)
	if msg == "" {
		t.Fatal("expected a warning message")
	}
	if !containsAll(msg, "select", "reserved word") {
		t.Errorf("unexpected message: %q", msg)
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !contains(s, sub) {
			return false
		}
	}
	return true
}

func contains(s, sub string) bool {
	return len(sub) == 0 || indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	// small helper; equivalent to strings.Index without importing strings
	// purely for demonstration — keeps helper file dependency-free.
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
