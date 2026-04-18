package schema

import (
	"errors"
	"strings"
	"testing"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

// -------------------- Helpers --------------------

func newRegistryWith(
	t *testing.T,
	tables []TableDefinition,
	edges []EdgeDefinition,
) *SchemaRegistry {
	t.Helper()
	r := NewSchemaRegistry()
	for _, tbl := range tables {
		if err := r.RegisterTable(tbl); err != nil {
			t.Fatalf("RegisterTable(%q): %v", tbl.Name, err)
		}
	}
	for _, e := range edges {
		if err := r.RegisterEdge(e); err != nil {
			t.Fatalf("RegisterEdge(%q): %v", e.Name, err)
		}
	}
	return r
}

func mustEqual(t *testing.T, got, want, label string) {
	t.Helper()
	if got != want {
		t.Errorf("%s mismatch\nwant:\n%q\n\ngot:\n%q", label, want, got)
	}
}

// userTable returns a TableDefinition named "user" with a single string field.
func userTable() TableDefinition {
	return NewTable("user", WithFields(StringField("email")))
}

// -------------------- Theme presets --------------------

func TestModernThemeConstants(t *testing.T) {
	th := ModernTheme()
	if th.Name != "modern" {
		t.Errorf("modern.Name = %q", th.Name)
	}
	if th.Colors.Primary != "#6366f1" {
		t.Errorf("modern.Colors.Primary = %q", th.Colors.Primary)
	}
	if th.GraphViz.NodeColor != "#6366f1" {
		t.Errorf("modern.GraphViz.NodeColor = %q", th.GraphViz.NodeColor)
	}
	if th.Mermaid.ThemeName != "default" {
		t.Errorf("modern.Mermaid.ThemeName = %q", th.Mermaid.ThemeName)
	}
	if th.ASCII.BoxStyle != "rounded" {
		t.Errorf("modern.ASCII.BoxStyle = %q", th.ASCII.BoxStyle)
	}
	if !th.ASCII.UseUnicode || !th.ASCII.UseColors || !th.ASCII.UseIcons {
		t.Errorf("modern.ASCII flags = %+v", th.ASCII)
	}
}

func TestDarkThemeConstants(t *testing.T) {
	th := DarkTheme()
	if th.Name != "dark" {
		t.Errorf("dark.Name = %q", th.Name)
	}
	if th.Colors.Primary != "#8b5cf6" {
		t.Errorf("dark.Colors.Primary = %q", th.Colors.Primary)
	}
	if th.GraphViz.BgColor != "#1e1b4b" {
		t.Errorf("dark.GraphViz.BgColor = %q", th.GraphViz.BgColor)
	}
	if th.Mermaid.ThemeName != "dark" {
		t.Errorf("dark.Mermaid.ThemeName = %q", th.Mermaid.ThemeName)
	}
}

func TestForestThemeConstants(t *testing.T) {
	th := ForestTheme()
	if th.Name != "forest" {
		t.Errorf("forest.Name = %q", th.Name)
	}
	if th.Colors.Primary != "#10b981" {
		t.Errorf("forest.Colors.Primary = %q", th.Colors.Primary)
	}
	if th.GraphViz.NodeColor != "#10b981" {
		t.Errorf("forest.GraphViz.NodeColor = %q", th.GraphViz.NodeColor)
	}
	if th.Mermaid.ThemeName != "forest" {
		t.Errorf("forest.Mermaid.ThemeName = %q", th.Mermaid.ThemeName)
	}
}

func TestMinimalThemeConstants(t *testing.T) {
	th := MinimalTheme()
	if th.Name != "minimal" {
		t.Errorf("minimal.Name = %q", th.Name)
	}
	if th.Colors.Primary != "#6b7280" {
		t.Errorf("minimal.Colors.Primary = %q", th.Colors.Primary)
	}
	if th.Mermaid.ThemeName != "neutral" {
		t.Errorf("minimal.Mermaid.ThemeName = %q", th.Mermaid.ThemeName)
	}
	if th.ASCII.BoxStyle != "single" {
		t.Errorf("minimal.ASCII.BoxStyle = %q", th.ASCII.BoxStyle)
	}
	if th.ASCII.UseColors || th.ASCII.UseIcons {
		t.Errorf("minimal.ASCII flags = %+v", th.ASCII)
	}
	if th.GraphViz.UseGradients {
		t.Errorf("minimal.GraphViz.UseGradients = true, want false")
	}
}

func TestGetThemeKnown(t *testing.T) {
	for _, name := range []string{"modern", "dark", "forest", "minimal"} {
		th, err := GetTheme(name)
		if err != nil {
			t.Fatalf("GetTheme(%q) error: %v", name, err)
		}
		if th.Name != name {
			t.Errorf("GetTheme(%q).Name = %q", name, th.Name)
		}
	}
}

func TestGetThemeUnknown(t *testing.T) {
	_, err := GetTheme("solarized")
	if err == nil {
		t.Fatalf("GetTheme(\"solarized\") error = nil, want non-nil")
	}
	if !errors.Is(err, surqlerrors.ErrValidation) {
		t.Errorf("GetTheme unknown error kind = %v, want ErrValidation", err)
	}
	if !strings.Contains(err.Error(), "solarized") {
		t.Errorf("GetTheme error = %q, want to contain %q", err.Error(), "solarized")
	}
}

func TestListThemes(t *testing.T) {
	names := ListThemes()
	wantSorted := []string{"dark", "forest", "minimal", "modern"}
	if len(names) != len(wantSorted) {
		t.Fatalf("ListThemes length = %d, want %d", len(names), len(wantSorted))
	}
	for i, n := range wantSorted {
		if names[i] != n {
			t.Errorf("ListThemes[%d] = %q, want %q", i, names[i], n)
		}
	}
}

// -------------------- utils --------------------

func TestFieldConstraintID(t *testing.T) {
	tbl := NewTable("x")
	if got := fieldConstraint("id", tbl); got != "PK" {
		t.Errorf("id => %q, want PK", got)
	}
}

func TestFieldConstraintUK(t *testing.T) {
	tbl := NewTable("user",
		WithFields(StringField("email")),
		WithIndexes(UniqueIndex("u_email", []string{"email"})),
	)
	if got := fieldConstraint("email", tbl); got != "UK" {
		t.Errorf("email UK => %q, want UK", got)
	}
}

func TestFieldConstraintFK(t *testing.T) {
	tbl := NewTable("post",
		WithFields(RecordField("author", "user")),
	)
	if got := fieldConstraint("author", tbl); got != "FK" {
		t.Errorf("author record => %q, want FK", got)
	}
}

func TestFieldConstraintNone(t *testing.T) {
	tbl := NewTable("user", WithFields(StringField("email")))
	if got := fieldConstraint("email", tbl); got != "" {
		t.Errorf("email no constraint => %q, want empty", got)
	}
}

func TestDisplayWidthPlain(t *testing.T) {
	if w := displayWidth("hello"); w != 5 {
		t.Errorf("displayWidth hello = %d, want 5", w)
	}
}

func TestDisplayWidthEmoji(t *testing.T) {
	if w := displayWidth("🔑"); w != 2 {
		t.Errorf("displayWidth 🔑 = %d, want 2", w)
	}
	if w := displayWidth("a🔑b"); w != 4 {
		t.Errorf("displayWidth a🔑b = %d, want 4", w)
	}
}

func TestDisplayWidthANSIStripped(t *testing.T) {
	in := "\x1b[91m🔑 (PK)\x1b[0m"
	// 🔑 = 2, " (PK)" = 5 => 7
	if w := displayWidth(in); w != 7 {
		t.Errorf("displayWidth with ANSI = %d, want 7", w)
	}
}

func TestCenterPadPythonCompatible(t *testing.T) {
	// Python str.center puts EXTRA on the left when padding is odd.
	l, r := centerPad("post", 25)
	if l != 11 || r != 10 {
		t.Errorf("centerPad(post, 25) = (%d, %d), want (11, 10)", l, r)
	}
	l, r = centerPad("user", 20)
	if l != 8 || r != 8 {
		t.Errorf("centerPad(user, 20) = (%d, %d), want (8, 8)", l, r)
	}
}

func TestBoxCharsUnicodeRounded(t *testing.T) {
	th := ASCIITheme{BoxStyle: "rounded", UseUnicode: true}
	ch := boxChars(th)
	if ch["tl"] != "╭" || ch["br"] != "╯" {
		t.Errorf("rounded box = %+v", ch)
	}
}

func TestBoxCharsASCIIFallback(t *testing.T) {
	ch := boxChars(ASCIITheme{}) // zero value => UseUnicode=false
	if ch["tl"] != "+" || ch["h"] != "-" || ch["v"] != "|" {
		t.Errorf("ascii fallback = %+v", ch)
	}
}

func TestBoxCharsDouble(t *testing.T) {
	th := ASCIITheme{BoxStyle: "double", UseUnicode: true}
	ch := boxChars(th)
	if ch["tl"] != "╔" || ch["h"] != "═" {
		t.Errorf("double box = %+v", ch)
	}
}

func TestBoxCharsHeavy(t *testing.T) {
	th := ASCIITheme{BoxStyle: "heavy", UseUnicode: true}
	ch := boxChars(th)
	if ch["tl"] != "┏" || ch["h"] != "━" {
		t.Errorf("heavy box = %+v", ch)
	}
}

func TestColorizeDisabled(t *testing.T) {
	th := ASCIITheme{UseColors: false}
	if got := colorize(th, "x", "pk"); got != "x" {
		t.Errorf("colorize off = %q", got)
	}
}

func TestColorizeEnabled(t *testing.T) {
	th := ASCIITheme{UseColors: true}
	got := colorize(th, "(PK)", "pk")
	want := "\x1b[91m(PK)\x1b[0m"
	if got != want {
		t.Errorf("colorize pk = %q, want %q", got, want)
	}
}

func TestConstraintIconEnabled(t *testing.T) {
	th := ASCIITheme{UseIcons: true}
	if got := constraintIcon(th, "PK"); got != "🔑 " {
		t.Errorf("icon PK = %q", got)
	}
	if got := constraintIcon(th, "FK"); got != "🔗 " {
		t.Errorf("icon FK = %q", got)
	}
	if got := constraintIcon(th, "UK"); got != "⭐ " {
		t.Errorf("icon UK = %q", got)
	}
}

func TestConstraintIconDisabled(t *testing.T) {
	th := ASCIITheme{UseIcons: false}
	if got := constraintIcon(th, "PK"); got != "" {
		t.Errorf("icon off PK = %q", got)
	}
}

// -------------------- Mermaid --------------------

func TestMermaidEmpty(t *testing.T) {
	r := NewSchemaRegistry()
	got := GenerateMermaid(r, MermaidTheme{})
	want := "erDiagram\n"
	mustEqual(t, got, want, "mermaid empty")
}

func TestMermaidUserNoTheme(t *testing.T) {
	r := newRegistryWith(t, []TableDefinition{userTable()}, nil)
	got := GenerateMermaid(r, MermaidTheme{})
	want := "erDiagram\n    user {\n        string id PK\n        string email\n    }\n"
	mustEqual(t, got, want, "mermaid user no theme")
}

func TestMermaidUserModern(t *testing.T) {
	r := newRegistryWith(t, []TableDefinition{userTable()}, nil)
	got := GenerateMermaid(r, ModernMermaidTheme())
	want := "%%{init: {'theme':'default'}}%%\nerDiagram\n    user {\n        string id PK\n        string email\n    }\n"
	mustEqual(t, got, want, "mermaid user modern")
}

func TestMermaidUserDark(t *testing.T) {
	r := newRegistryWith(t, []TableDefinition{userTable()}, nil)
	got := GenerateMermaid(r, DarkMermaidTheme())
	want := "%%{init: {'theme':'dark'}}%%\nerDiagram\n    user {\n        string id PK\n        string email\n    }\n"
	mustEqual(t, got, want, "mermaid user dark")
}

func TestMermaidUserForest(t *testing.T) {
	r := newRegistryWith(t, []TableDefinition{userTable()}, nil)
	got := GenerateMermaid(r, ForestMermaidTheme())
	want := "%%{init: {'theme':'forest'}}%%\nerDiagram\n    user {\n        string id PK\n        string email\n    }\n"
	mustEqual(t, got, want, "mermaid user forest")
}

func TestMermaidUserMinimal(t *testing.T) {
	r := newRegistryWith(t, []TableDefinition{userTable()}, nil)
	got := GenerateMermaid(r, MinimalMermaidTheme())
	want := "%%{init: {'theme':'neutral'}}%%\nerDiagram\n    user {\n        string id PK\n        string email\n    }\n"
	mustEqual(t, got, want, "mermaid user minimal")
}

func TestMermaidUserUniqueIndex(t *testing.T) {
	user := NewTable("user",
		WithFields(StringField("email"), StringField("name")),
		WithIndexes(UniqueIndex("u_email", []string{"email"})),
	)
	r := newRegistryWith(t, []TableDefinition{user}, nil)
	got := GenerateMermaid(r, MermaidTheme{})
	want := "erDiagram\n    user {\n        string id PK\n        string email UK\n        string name\n    }\n"
	mustEqual(t, got, want, "mermaid user unique")
}

func TestMermaidEdgesBasic(t *testing.T) {
	user := NewTable("user", WithFields(StringField("email")))
	post := NewTable("post",
		WithFields(StringField("title"), RecordField("author", "user")),
	)
	wrote := TypedEdge("wrote", "user", "post", WithEdgeFields(DatetimeField("at")))
	follows := TypedEdge("follows", "user", "user")
	r := newRegistryWith(t, []TableDefinition{user, post}, []EdgeDefinition{wrote, follows})

	got := GenerateMermaid(r, ModernMermaidTheme())
	want := "%%{init: {'theme':'default'}}%%\n" +
		"erDiagram\n    post {\n        string id PK\n        string title\n        record author FK\n    }\n" +
		"    user {\n        string id PK\n        string email\n    }\n" +
		"    wrote {\n        datetime at\n    }\n" +
		"\n    user }o--o{ user : follows" +
		"\n    user ||--o{ post : wrote"
	mustEqual(t, got, want, "mermaid edges")
}

func TestMermaidIncludeFieldsFalse(t *testing.T) {
	r := newRegistryWith(t, []TableDefinition{userTable()}, nil)
	got := GenerateMermaid(r, MermaidTheme{}, WithIncludeFields(false))
	want := "erDiagram\n    user {\n    }\n"
	mustEqual(t, got, want, "mermaid include_fields=false")
}

func TestMermaidIncludeEdgesFalse(t *testing.T) {
	user := NewTable("user", WithFields(StringField("email")))
	post := NewTable("post", WithFields(StringField("title")))
	e := TypedEdge("wrote", "user", "post")
	r := newRegistryWith(t, []TableDefinition{user, post}, []EdgeDefinition{e})
	got := GenerateMermaid(r, MermaidTheme{}, WithIncludeEdges(false))
	// Tables are emitted in alphabetical order: post, user.
	want := "erDiagram\n    post {\n        string id PK\n        string title\n    }\n" +
		"    user {\n        string id PK\n        string email\n    }"
	// With IncludeEdges=false we do not emit the blank line separator that
	// precedes the edges section (Python: the empty-string list element is
	// appended only when include_edges is true).
	mustEqual(t, got, want, "mermaid include_edges=false")
}

// Skip-if-table-missing test: edge referencing unknown table is dropped.
func TestMermaidEdgeDroppedWhenTableMissing(t *testing.T) {
	user := NewTable("user", WithFields(StringField("email")))
	e := TypedEdge("wrote", "user", "missing")
	r := newRegistryWith(t, []TableDefinition{user}, []EdgeDefinition{e})
	got := GenerateMermaid(r, MermaidTheme{})
	want := "erDiagram\n    user {\n        string id PK\n        string email\n    }\n"
	mustEqual(t, got, want, "mermaid missing table edge dropped")
}

// -------------------- GraphViz --------------------

func TestGraphVizEmpty(t *testing.T) {
	r := NewSchemaRegistry()
	got := GenerateGraphViz(r, GraphVizTheme{})
	want := "digraph schema {\n    rankdir=LR;\n    node [shape=record];\n\n\n}"
	mustEqual(t, got, want, "graphviz empty")
}

func TestGraphVizUserDefault(t *testing.T) {
	r := newRegistryWith(t, []TableDefinition{userTable()}, nil)
	got := GenerateGraphViz(r, GraphVizTheme{})
	want := "digraph schema {\n    rankdir=LR;\n    node [shape=record];\n\n" +
		"    user [label=\"{user|id : string (PK)\\l|email : string\\l}\"];\n\n}"
	mustEqual(t, got, want, "graphviz user default")
}

func TestGraphVizUserModern(t *testing.T) {
	r := newRegistryWith(t, []TableDefinition{userTable()}, nil)
	got := GenerateGraphViz(r, ModernGraphVizTheme())
	want := `digraph schema {
    rankdir=LR;
    fontname="Arial";
    node [shape=record, style="filled,rounded", fontname="Arial", pad="0.5", margin="0.2"];
    edge [color="#64748b", style=solid, fontname="Arial"];

    user [label=<<TABLE BORDER="0" CELLBORDER="1" CELLSPACING="0" CELLPADDING="4"><TR><TD BGCOLOR="#6366f1" COLSPAN="2"><FONT COLOR="#FFFFFF"><B>user</B></FONT></TD></TR><TR><TD ALIGN="LEFT">id</TD><TD ALIGN="LEFT"><FONT COLOR="#94a3b8">string</FONT> <FONT COLOR="#ef4444">PK</FONT></TD></TR><TR><TD ALIGN="LEFT">email</TD><TD ALIGN="LEFT"><FONT COLOR="#10b981">string</FONT></TD></TR></TABLE>>];

}`
	mustEqual(t, got, want, "graphviz user modern")
}

func TestGraphVizUserDark(t *testing.T) {
	user := NewTable("user",
		WithFields(StringField("email"), StringField("name")),
		WithIndexes(UniqueIndex("u_email", []string{"email"})),
	)
	r := newRegistryWith(t, []TableDefinition{user}, nil)
	got := GenerateGraphViz(r, DarkGraphVizTheme())
	want := `digraph schema {
    rankdir=LR;
    bgcolor="#1e1b4b";
    fontname="Arial";
    node [shape=record, style="filled,rounded", fontname="Arial", pad="0.5", margin="0.2"];
    edge [color="#64748b", style=solid, fontname="Arial"];

    user [label=<<TABLE BORDER="0" CELLBORDER="1" CELLSPACING="0" CELLPADDING="4"><TR><TD BGCOLOR="#8b5cf6" COLSPAN="2"><FONT COLOR="#FFFFFF"><B>user</B></FONT></TD></TR><TR><TD ALIGN="LEFT">id</TD><TD ALIGN="LEFT"><FONT COLOR="#94a3b8">string</FONT> <FONT COLOR="#ef4444">PK</FONT></TD></TR><TR><TD ALIGN="LEFT">email</TD><TD ALIGN="LEFT"><FONT COLOR="#10b981">string</FONT> <FONT COLOR="#8b5cf6">UK</FONT></TD></TR><TR><TD ALIGN="LEFT">name</TD><TD ALIGN="LEFT"><FONT COLOR="#10b981">string</FONT></TD></TR></TABLE>>];

}`
	mustEqual(t, got, want, "graphviz user dark unique")
}

func TestGraphVizUserForest(t *testing.T) {
	r := newRegistryWith(t, []TableDefinition{userTable()}, nil)
	got := GenerateGraphViz(r, ForestGraphVizTheme())
	want := `digraph schema {
    rankdir=LR;
    fontname="Arial";
    node [shape=record, style="filled,rounded", fontname="Arial", pad="0.5", margin="0.2"];
    edge [color="#059669", style=solid, fontname="Arial"];

    user [label=<<TABLE BORDER="0" CELLBORDER="1" CELLSPACING="0" CELLPADDING="4"><TR><TD BGCOLOR="#10b981" COLSPAN="2"><FONT COLOR="#FFFFFF"><B>user</B></FONT></TD></TR><TR><TD ALIGN="LEFT">id</TD><TD ALIGN="LEFT"><FONT COLOR="#94a3b8">string</FONT> <FONT COLOR="#ef4444">PK</FONT></TD></TR><TR><TD ALIGN="LEFT">email</TD><TD ALIGN="LEFT"><FONT COLOR="#10b981">string</FONT></TD></TR></TABLE>>];

}`
	mustEqual(t, got, want, "graphviz user forest")
}

func TestGraphVizUserMinimal(t *testing.T) {
	r := newRegistryWith(t, []TableDefinition{userTable()}, nil)
	got := GenerateGraphViz(r, MinimalGraphVizTheme())
	want := `digraph schema {
    rankdir=LR;
    fontname="Arial";
    node [shape=record, style="filled", fontname="Arial", pad="0.5", margin="0.2"];
    edge [color="#9ca3af", style=solid, fontname="Arial"];

    user [label="{user|id : string (PK)\l|email : string\l}"];

}`
	mustEqual(t, got, want, "graphviz user minimal")
}

func TestGraphVizEdgesDefault(t *testing.T) {
	user := NewTable("user", WithFields(StringField("email")))
	post := NewTable("post",
		WithFields(StringField("title"), RecordField("author", "user")),
	)
	wrote := TypedEdge("wrote", "user", "post", WithEdgeFields(DatetimeField("at")))
	follows := TypedEdge("follows", "user", "user")
	r := newRegistryWith(t, []TableDefinition{user, post}, []EdgeDefinition{wrote, follows})
	got := GenerateGraphViz(r, GraphVizTheme{})
	want := "digraph schema {\n    rankdir=LR;\n    node [shape=record];\n\n" +
		"    post [label=\"{post|id : string (PK)\\l|title : string\\l|author : record (FK)\\l}\"];\n" +
		"    user [label=\"{user|id : string (PK)\\l|email : string\\l}\"];\n" +
		"    wrote [label=\"{wrote|at : datetime\\l}\"];\n\n" +
		"    user -> user [label=\"follows\", style=dashed];\n" +
		"    user -> post [label=\"wrote\"];\n}"
	mustEqual(t, got, want, "graphviz edges default")
}

func TestGraphVizEdgesModernGradient(t *testing.T) {
	// Self-referential follows edge picks up dashed + secondary color.
	user := NewTable("user", WithFields(StringField("email")))
	follows := TypedEdge("follows", "user", "user")
	r := newRegistryWith(t, []TableDefinition{user}, []EdgeDefinition{follows})
	got := GenerateGraphViz(r, ModernGraphVizTheme())
	if !strings.Contains(got, `user -> user [label="follows", style=dashed, color="#ec4899"];`) {
		t.Errorf("graphviz modern self-edge = %q", got)
	}
}

func TestGraphVizIncludeFieldsFalse(t *testing.T) {
	r := newRegistryWith(t, []TableDefinition{userTable()}, nil)
	got := GenerateGraphViz(r, GraphVizTheme{}, WithIncludeFields(false))
	want := "digraph schema {\n    rankdir=LR;\n    node [shape=record];\n\n" +
		"    user [label=\"user\"];\n\n}"
	mustEqual(t, got, want, "graphviz no fields")
}

// -------------------- ASCII --------------------

func TestASCIIEmpty(t *testing.T) {
	r := NewSchemaRegistry()
	got := GenerateASCII(r, ASCIITheme{})
	if got != "" {
		t.Errorf("ascii empty = %q, want empty", got)
	}
}

func TestASCIIUserNoTheme(t *testing.T) {
	r := newRegistryWith(t, []TableDefinition{userTable()}, nil)
	got := GenerateASCII(r, ASCIITheme{})
	want := "+--------------------+\n" +
		"|        user        |\n" +
		"+--------------------+\n" +
		"| id : string (PK)   |\n" +
		"| email : string     |\n" +
		"+--------------------+\n"
	mustEqual(t, got, want, "ascii user no theme")
}

func TestASCIIUserMinimal(t *testing.T) {
	user := NewTable("user",
		WithFields(StringField("email"), StringField("name")),
		WithIndexes(UniqueIndex("u_email", []string{"email"})),
	)
	r := newRegistryWith(t, []TableDefinition{user}, nil)
	got := GenerateASCII(r, MinimalASCIITheme())
	want := "┌─────────────────────┐\n" +
		"│         user        │\n" +
		"├─────────────────────┤\n" +
		"│ id : string (PK)    │\n" +
		"│ email : string (UK) │\n" +
		"│ name : string       │\n" +
		"└─────────────────────┘\n"
	mustEqual(t, got, want, "ascii user minimal")
}

func TestASCIIUserModern(t *testing.T) {
	r := newRegistryWith(t, []TableDefinition{userTable()}, nil)
	got := GenerateASCII(r, ModernASCIITheme())
	want := "╭─────────────────────╮\n" +
		"│\x1b[1m         user        \x1b[0m│\n" +
		"├─────────────────────┤\n" +
		"│ id : string \x1b[91m🔑 (PK)\x1b[0m │\n" +
		"│ email : string      │\n" +
		"╰─────────────────────╯\n"
	mustEqual(t, got, want, "ascii user modern")
}

func TestASCIIUserDark(t *testing.T) {
	r := newRegistryWith(t, []TableDefinition{userTable()}, nil)
	got := GenerateASCII(r, DarkASCIITheme())
	want := "╭─────────────────────╮\n" +
		"│\x1b[1m         user        \x1b[0m│\n" +
		"├─────────────────────┤\n" +
		"│ id : string \x1b[91m🔑 (PK)\x1b[0m │\n" +
		"│ email : string      │\n" +
		"╰─────────────────────╯\n"
	mustEqual(t, got, want, "ascii user dark")
}

func TestASCIIUserForest(t *testing.T) {
	r := newRegistryWith(t, []TableDefinition{userTable()}, nil)
	got := GenerateASCII(r, ForestASCIITheme())
	want := "╭─────────────────────╮\n" +
		"│\x1b[1m         user        \x1b[0m│\n" +
		"├─────────────────────┤\n" +
		"│ id : string \x1b[91m🔑 (PK)\x1b[0m │\n" +
		"│ email : string      │\n" +
		"╰─────────────────────╯\n"
	mustEqual(t, got, want, "ascii user forest")
}

func TestASCIIPostModernFKConstraint(t *testing.T) {
	post := NewTable("post",
		WithFields(StringField("title"), RecordField("author", "user")),
	)
	r := newRegistryWith(t, []TableDefinition{post}, nil)
	got := GenerateASCII(r, ModernASCIITheme())
	want := "╭─────────────────────────╮\n" +
		"│\x1b[1m           post          \x1b[0m│\n" +
		"├─────────────────────────┤\n" +
		"│ id : string \x1b[91m🔑 (PK)\x1b[0m     │\n" +
		"│ title : string          │\n" +
		"│ author : record \x1b[94m🔗 (FK)\x1b[0m │\n" +
		"╰─────────────────────────╯\n"
	mustEqual(t, got, want, "ascii post modern")
}

func TestASCIIRelationshipsSection(t *testing.T) {
	user := NewTable("user", WithFields(StringField("email")))
	post := NewTable("post", WithFields(StringField("title")))
	wrote := TypedEdge("wrote", "user", "post")
	r := newRegistryWith(t, []TableDefinition{user, post}, []EdgeDefinition{wrote})
	got := GenerateASCII(r, MinimalASCIITheme())
	if !strings.Contains(got, "Relationships:\n") {
		t.Errorf("ascii missing Relationships header:\n%s", got)
	}
	if !strings.Contains(got, "  user --[wrote]--> post") {
		t.Errorf("ascii missing edge line:\n%s", got)
	}
	if !strings.Contains(got, strings.Repeat("-", 40)) {
		t.Errorf("ascii missing separator")
	}
}

func TestASCIIIncludeFieldsFalse(t *testing.T) {
	r := newRegistryWith(t, []TableDefinition{userTable()}, nil)
	got := GenerateASCII(r, MinimalASCIITheme(), WithIncludeFields(false))
	// Minimal at width 20 (len('user')+4 < 20).
	want := "┌────────────────────┐\n" +
		"│        user        │\n" +
		"└────────────────────┘\n"
	mustEqual(t, got, want, "ascii no fields")
}

func TestASCIIIncludeEdgesFalse(t *testing.T) {
	user := NewTable("user", WithFields(StringField("email")))
	post := NewTable("post", WithFields(StringField("title")))
	wrote := TypedEdge("wrote", "user", "post")
	r := newRegistryWith(t, []TableDefinition{user, post}, []EdgeDefinition{wrote})
	got := GenerateASCII(r, MinimalASCIITheme(), WithIncludeEdges(false))
	if strings.Contains(got, "Relationships:") {
		t.Errorf("ascii should not contain Relationships section:\n%s", got)
	}
}

func TestASCIIManyFields(t *testing.T) {
	// Longer field list to exercise box-width calculation.
	fields := []FieldDefinition{
		StringField("first_name"),
		StringField("last_name"),
		IntField("age"),
		BoolField("active"),
		DatetimeField("created_at"),
	}
	user := NewTable("user", WithFields(fields...))
	r := newRegistryWith(t, []TableDefinition{user}, nil)
	got := GenerateASCII(r, MinimalASCIITheme())
	for _, name := range []string{"first_name", "last_name", "age", "active", "created_at"} {
		if !strings.Contains(got, name) {
			t.Errorf("ascii many fields missing %q:\n%s", name, got)
		}
	}
	if !strings.Contains(got, "id : string (PK)") {
		t.Errorf("ascii many fields missing id PK row:\n%s", got)
	}
}

// -------------------- Registry plumbing --------------------

func TestGenerateFromMapsMatchesRegistry(t *testing.T) {
	user := NewTable("user", WithFields(StringField("email")))
	r := newRegistryWith(t, []TableDefinition{user}, nil)
	fromRegistry := GenerateMermaid(r, ModernMermaidTheme())
	fromMaps := GenerateMermaidFromMaps(
		map[string]TableDefinition{"user": user},
		map[string]EdgeDefinition{},
		ModernMermaidTheme(),
	)
	mustEqual(t, fromMaps, fromRegistry, "registry vs maps parity")
}

func TestWithIncludeFlagsApplyOrder(t *testing.T) {
	// Repeated options should take the last value (standard functional-opt
	// pattern).
	o := resolveOptions([]VisualizeOption{
		WithIncludeFields(true),
		WithIncludeFields(false),
		WithIncludeEdges(false),
		WithIncludeEdges(true),
	})
	if o.IncludeFields != false {
		t.Errorf("IncludeFields = %v, want false", o.IncludeFields)
	}
	if o.IncludeEdges != true {
		t.Errorf("IncludeEdges = %v, want true", o.IncludeEdges)
	}
}

func TestResolveOptionsDefaults(t *testing.T) {
	o := resolveOptions(nil)
	if !o.IncludeFields {
		t.Errorf("default IncludeFields = false, want true")
	}
	if !o.IncludeEdges {
		t.Errorf("default IncludeEdges = false, want true")
	}
}
