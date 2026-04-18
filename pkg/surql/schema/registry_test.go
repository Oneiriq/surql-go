package schema

import (
	stdErrors "errors"
	"sync"
	"testing"

	surqlerrors "github.com/albedosehen/surql-go/pkg/surql/errors"
)

func TestNewSchemaRegistry_Empty(t *testing.T) {
	r := NewSchemaRegistry()
	if r.TableCount() != 0 {
		t.Errorf("TableCount() = %d, want 0", r.TableCount())
	}
	if r.EdgeCount() != 0 {
		t.Errorf("EdgeCount() = %d, want 0", r.EdgeCount())
	}
}

func TestRegistry_RegisterAndGetTable(t *testing.T) {
	r := NewSchemaRegistry()
	tbl := NewTable("user", WithMode(TableModeSchemafull))
	if err := r.RegisterTable(tbl); err != nil {
		t.Fatalf("RegisterTable: %v", err)
	}
	got, ok := r.GetTable("user")
	if !ok {
		t.Fatal("GetTable returned not-found")
	}
	if got.Name != "user" {
		t.Errorf("got.Name = %q", got.Name)
	}
	if got.Mode != TableModeSchemafull {
		t.Errorf("got.Mode = %q", got.Mode)
	}
}

func TestRegistry_RegisterAndGetEdge(t *testing.T) {
	r := NewSchemaRegistry()
	e := TypedEdge("likes", "user", "post")
	if err := r.RegisterEdge(e); err != nil {
		t.Fatalf("RegisterEdge: %v", err)
	}
	got, ok := r.GetEdge("likes")
	if !ok {
		t.Fatal("GetEdge returned not-found")
	}
	if got.Name != "likes" {
		t.Errorf("got.Name = %q", got.Name)
	}
	if got.FromTable != "user" || got.ToTable != "post" {
		t.Errorf("from/to mismatch: %q/%q", got.FromTable, got.ToTable)
	}
}

func TestRegistry_GetTable_NotFound(t *testing.T) {
	r := NewSchemaRegistry()
	_, ok := r.GetTable("missing")
	if ok {
		t.Error("expected not-found for missing table")
	}
}

func TestRegistry_GetEdge_NotFound(t *testing.T) {
	r := NewSchemaRegistry()
	_, ok := r.GetEdge("missing")
	if ok {
		t.Error("expected not-found for missing edge")
	}
}

func TestRegistry_RegisterTable_EmptyNameReturnsError(t *testing.T) {
	r := NewSchemaRegistry()
	err := r.RegisterTable(TableDefinition{Mode: TableModeSchemafull})
	if err == nil {
		t.Fatal("expected error for empty name")
	}
	if !stdErrors.Is(err, surqlerrors.ErrValidation) {
		t.Errorf("want ErrValidation, got %v", err)
	}
}

func TestRegistry_RegisterEdge_EmptyNameReturnsError(t *testing.T) {
	r := NewSchemaRegistry()
	err := r.RegisterEdge(EdgeDefinition{Mode: EdgeModeRelation})
	if err == nil {
		t.Fatal("expected error for empty name")
	}
	if !stdErrors.Is(err, surqlerrors.ErrValidation) {
		t.Errorf("want ErrValidation, got %v", err)
	}
}

func TestRegistry_Tables_SortedByName(t *testing.T) {
	r := NewSchemaRegistry()
	for _, name := range []string{"zebra", "apple", "mango"} {
		if err := r.RegisterTable(NewTable(name)); err != nil {
			t.Fatalf("RegisterTable(%q): %v", name, err)
		}
	}
	tables := r.Tables()
	if len(tables) != 3 {
		t.Fatalf("len(tables) = %d, want 3", len(tables))
	}
	want := []string{"apple", "mango", "zebra"}
	for i, tbl := range tables {
		if tbl.Name != want[i] {
			t.Errorf("tables[%d].Name = %q, want %q", i, tbl.Name, want[i])
		}
	}
}

func TestRegistry_Edges_SortedByName(t *testing.T) {
	r := NewSchemaRegistry()
	for _, name := range []string{"follows", "authored", "likes"} {
		if err := r.RegisterEdge(TypedEdge(name, "user", "post")); err != nil {
			t.Fatalf("RegisterEdge(%q): %v", name, err)
		}
	}
	edges := r.Edges()
	want := []string{"authored", "follows", "likes"}
	for i, e := range edges {
		if e.Name != want[i] {
			t.Errorf("edges[%d].Name = %q, want %q", i, e.Name, want[i])
		}
	}
}

func TestRegistry_TableNames_Sorted(t *testing.T) {
	r := NewSchemaRegistry()
	for _, n := range []string{"c", "a", "b"} {
		_ = r.RegisterTable(NewTable(n))
	}
	got := r.TableNames()
	want := []string{"a", "b", "c"}
	for i, n := range got {
		if n != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, n, want[i])
		}
	}
}

func TestRegistry_EdgeNames_Sorted(t *testing.T) {
	r := NewSchemaRegistry()
	for _, n := range []string{"c", "a", "b"} {
		_ = r.RegisterEdge(TypedEdge(n, "x", "y"))
	}
	got := r.EdgeNames()
	want := []string{"a", "b", "c"}
	for i, n := range got {
		if n != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, n, want[i])
		}
	}
}

func TestRegistry_ReRegisterReplaces(t *testing.T) {
	r := NewSchemaRegistry()
	_ = r.RegisterTable(NewTable("user", WithMode(TableModeSchemafull)))
	_ = r.RegisterTable(NewTable("user", WithMode(TableModeSchemaless)))
	got, ok := r.GetTable("user")
	if !ok {
		t.Fatal("missing table after re-register")
	}
	if got.Mode != TableModeSchemaless {
		t.Errorf("got.Mode = %q, want SCHEMALESS after re-register", got.Mode)
	}
	if r.TableCount() != 1 {
		t.Errorf("TableCount() = %d, want 1", r.TableCount())
	}
}

func TestRegistry_Clear(t *testing.T) {
	r := NewSchemaRegistry()
	_ = r.RegisterTable(NewTable("user"))
	_ = r.RegisterEdge(TypedEdge("likes", "user", "post"))
	r.Clear()
	if r.TableCount() != 0 {
		t.Errorf("TableCount() = %d, want 0 after Clear", r.TableCount())
	}
	if r.EdgeCount() != 0 {
		t.Errorf("EdgeCount() = %d, want 0 after Clear", r.EdgeCount())
	}
}

func TestRegistry_TablesReturnsCopy(t *testing.T) {
	r := NewSchemaRegistry()
	_ = r.RegisterTable(NewTable("user"))
	tables := r.Tables()
	tables[0] = NewTable("other") // should not affect registry
	got, ok := r.GetTable("user")
	if !ok || got.Name != "user" {
		t.Errorf("registry state mutated by caller; got %q / %v", got.Name, ok)
	}
}

func TestRegistry_EdgesReturnsCopy(t *testing.T) {
	r := NewSchemaRegistry()
	_ = r.RegisterEdge(TypedEdge("likes", "user", "post"))
	edges := r.Edges()
	edges[0] = TypedEdge("other", "a", "b")
	got, ok := r.GetEdge("likes")
	if !ok || got.Name != "likes" {
		t.Errorf("registry state mutated by caller; got %q / %v", got.Name, ok)
	}
}

func TestGetRegistry_ReturnsSingleton(t *testing.T) {
	r1 := GetRegistry()
	r2 := GetRegistry()
	if r1 != r2 {
		t.Error("GetRegistry returned distinct instances")
	}
}

func TestGetRegistry_SingletonClearIsolation(t *testing.T) {
	r := GetRegistry()
	r.Clear()
	if err := r.RegisterTable(NewTable("singleton_user")); err != nil {
		t.Fatalf("RegisterTable: %v", err)
	}
	other := GetRegistry()
	if _, ok := other.GetTable("singleton_user"); !ok {
		t.Error("expected singleton to share state across GetRegistry calls")
	}
	// cleanup so we don't leak across tests
	r.Clear()
}

func TestRegistry_ConcurrentRegister(t *testing.T) {
	r := NewSchemaRegistry()
	var wg sync.WaitGroup
	const goroutines = 32
	const perGoroutine = 16

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				name := tableNameFor(gid, i)
				_ = r.RegisterTable(NewTable(name))
			}
		}(g)
	}
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				name := edgeNameFor(gid, i)
				_ = r.RegisterEdge(TypedEdge(name, "user", "post"))
			}
		}(g)
	}
	// readers in parallel
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				_ = r.Tables()
				_ = r.Edges()
				_ = r.TableCount()
				_ = r.EdgeCount()
			}
		}()
	}
	wg.Wait()

	if r.TableCount() != goroutines*perGoroutine {
		t.Errorf("TableCount() = %d, want %d",
			r.TableCount(), goroutines*perGoroutine)
	}
	if r.EdgeCount() != goroutines*perGoroutine {
		t.Errorf("EdgeCount() = %d, want %d",
			r.EdgeCount(), goroutines*perGoroutine)
	}
}

func tableNameFor(g, i int) string {
	return "t_" + itoa(g) + "_" + itoa(i)
}

func edgeNameFor(g, i int) string {
	return "e_" + itoa(g) + "_" + itoa(i)
}

// itoa is a tiny allocation-free integer->string formatter used by the
// concurrent-registration test; avoids pulling strconv into a test helper.
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
