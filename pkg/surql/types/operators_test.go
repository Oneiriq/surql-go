package types

import "testing"

func TestEq_String(t *testing.T) {
	if got := EqOp("name", "Alice").ToSurql(); got != `name = 'Alice'` {
		t.Errorf("got %q", got)
	}
}

func TestNe_String(t *testing.T) {
	if got := NeOp("status", "deleted").ToSurql(); got != `status != 'deleted'` {
		t.Errorf("got %q", got)
	}
}

func TestGt_Int(t *testing.T) {
	if got := GtOp("age", 18).ToSurql(); got != "age > 18" {
		t.Errorf("got %q", got)
	}
}

func TestLt_Float(t *testing.T) {
	if got := LtOp("price", 50.0).ToSurql(); got != "price < 50" {
		// Go strips trailing zeros via %g-style; accept either 50 or 50.0
		if got != "price < 50.0" {
			t.Errorf("got %q", got)
		}
	}
}

func TestGteAndLte(t *testing.T) {
	if got := GteOp("score", 100).ToSurql(); got != "score >= 100" {
		t.Errorf("gte: %q", got)
	}
	if got := LteOp("quantity", 10).ToSurql(); got != "quantity <= 10" {
		t.Errorf("lte: %q", got)
	}
}

func TestContains(t *testing.T) {
	got := ContainsOp("email", "@example.com").ToSurql()
	if got != `email CONTAINS '@example.com'` {
		t.Errorf("got %q", got)
	}
}

func TestContainsNot(t *testing.T) {
	got := ContainsNotOp("tags", "spam").ToSurql()
	if got != `tags CONTAINSNOT 'spam'` {
		t.Errorf("got %q", got)
	}
}

func TestContainsAll(t *testing.T) {
	got := ContainsAllOp("tags", []any{"python", "database"}).ToSurql()
	if got != `tags CONTAINSALL ['python', 'database']` {
		t.Errorf("got %q", got)
	}
}

func TestContainsAny(t *testing.T) {
	got := ContainsAnyOp("tags", []any{"python", "javascript"}).ToSurql()
	if got != `tags CONTAINSANY ['python', 'javascript']` {
		t.Errorf("got %q", got)
	}
}

func TestInside(t *testing.T) {
	got := InsideOp("status", []any{"active", "pending"}).ToSurql()
	if got != `status INSIDE ['active', 'pending']` {
		t.Errorf("got %q", got)
	}
}

func TestNotInside(t *testing.T) {
	got := NotInsideOp("status", []any{"deleted", "archived"}).ToSurql()
	if got != `status NOTINSIDE ['deleted', 'archived']` {
		t.Errorf("got %q", got)
	}
}

func TestIsNullAndNotNull(t *testing.T) {
	if got := IsNullOp("deleted_at").ToSurql(); got != "deleted_at IS NULL" {
		t.Errorf("is null: %q", got)
	}
	if got := IsNotNullOp("created_at").ToSurql(); got != "created_at IS NOT NULL" {
		t.Errorf("is not null: %q", got)
	}
}

func TestAnd(t *testing.T) {
	got := AndOp(GtOp("age", 18), EqOp("status", "active")).ToSurql()
	if got != `(age > 18) AND (status = 'active')` {
		t.Errorf("got %q", got)
	}
}

func TestOr(t *testing.T) {
	got := OrOp(EqOp("type", "admin"), EqOp("type", "moderator")).ToSurql()
	if got != `(type = 'admin') OR (type = 'moderator')` {
		t.Errorf("got %q", got)
	}
}

func TestNot(t *testing.T) {
	got := NotOp(EqOp("status", "deleted")).ToSurql()
	if got != `NOT (status = 'deleted')` {
		t.Errorf("got %q", got)
	}
}

func TestNullQuotedAsNULL(t *testing.T) {
	got := EqOp("deleted_at", nil).ToSurql()
	if got != "deleted_at = NULL" {
		t.Errorf("got %q", got)
	}
}

func TestBoolQuotedLowercase(t *testing.T) {
	if got := EqOp("active", true).ToSurql(); got != "active = true" {
		t.Errorf("true: %q", got)
	}
	if got := EqOp("active", false).ToSurql(); got != "active = false" {
		t.Errorf("false: %q", got)
	}
}

func TestStringEscapesSingleQuote(t *testing.T) {
	got := EqOp("name", "O'Brien").ToSurql()
	if got != `name = 'O\'Brien'` {
		t.Errorf("got %q", got)
	}
}

func TestStringEscapesBackslash(t *testing.T) {
	got := EqOp("path", `a\b`).ToSurql()
	if got != `path = 'a\\b'` {
		t.Errorf("got %q", got)
	}
}

func TestSurrealFnValueRendersRaw(t *testing.T) {
	got := EqOp("created_at", SurqlFn("time::now")).ToSurql()
	if got != "created_at = time::now()" {
		t.Errorf("got %q", got)
	}
}

func TestRecordRefValueRendersRaw(t *testing.T) {
	got := EqOp("author", StringRecordRef("user", "alice")).ToSurql()
	if got != "author = type::record('user', 'alice')" {
		t.Errorf("got %q", got)
	}
}
