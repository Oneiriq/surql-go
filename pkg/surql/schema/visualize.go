package schema

import (
	"fmt"
	"sort"
	"strings"
)

// VisualizeOptions controls optional inclusion of fields and edges in the
// generated diagram. The zero value includes both.
type VisualizeOptions struct {
	// IncludeFields, when false, omits per-field rows from the diagram.
	// Default: true (represented as the zero value via a nil *bool pattern
	// would be awkward, so callers opt in/out with WithIncludeFields.)
	IncludeFields bool
	// IncludeEdges, when false, omits edge/relationship output.
	IncludeEdges bool
	fieldsSet    bool
	edgesSet     bool
}

// VisualizeOption customises a VisualizeOptions.
type VisualizeOption func(*VisualizeOptions)

// WithIncludeFields toggles inclusion of field definitions.
func WithIncludeFields(include bool) VisualizeOption {
	return func(o *VisualizeOptions) {
		o.IncludeFields = include
		o.fieldsSet = true
	}
}

// WithIncludeEdges toggles inclusion of edge relationships.
func WithIncludeEdges(include bool) VisualizeOption {
	return func(o *VisualizeOptions) {
		o.IncludeEdges = include
		o.edgesSet = true
	}
}

// resolveOptions applies opts and substitutes defaults for flags that were
// never explicitly set.
func resolveOptions(opts []VisualizeOption) VisualizeOptions {
	o := VisualizeOptions{}
	for _, fn := range opts {
		fn(&o)
	}
	if !o.fieldsSet {
		o.IncludeFields = true
	}
	if !o.edgesSet {
		o.IncludeEdges = true
	}
	return o
}

// sortedTableMap returns a stable iteration order for tables.
func sortedTableMap(tables map[string]TableDefinition) []string {
	names := make([]string, 0, len(tables))
	for k := range tables {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// sortedEdgeMap returns a stable iteration order for edges.
func sortedEdgeMap(edges map[string]EdgeDefinition) []string {
	names := make([]string, 0, len(edges))
	for k := range edges {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// registryTables snapshots the registry tables into a map keyed by name.
func registryTables(r *SchemaRegistry) map[string]TableDefinition {
	out := map[string]TableDefinition{}
	for _, t := range r.Tables() {
		out[t.Name] = t
	}
	return out
}

// registryEdges snapshots the registry edges into a map keyed by name.
func registryEdges(r *SchemaRegistry) map[string]EdgeDefinition {
	out := map[string]EdgeDefinition{}
	for _, e := range r.Edges() {
		out[e.Name] = e
	}
	return out
}

// GenerateMermaid renders a Mermaid ER diagram from registry using theme.
// The diagram contains entity blocks for every table (always showing the
// implicit id PK row) and any relationships derived from edges. Pass an
// empty MermaidTheme to suppress the %%{init}%% directive.
func GenerateMermaid(r *SchemaRegistry, theme MermaidTheme, opts ...VisualizeOption) string {
	return GenerateMermaidFromMaps(registryTables(r), registryEdges(r), theme, opts...)
}

// GenerateMermaidFromMaps is like GenerateMermaid but operates on raw
// table/edge maps, matching the surql-py entry point signature.
func GenerateMermaidFromMaps(
	tables map[string]TableDefinition,
	edges map[string]EdgeDefinition,
	theme MermaidTheme,
	opts ...VisualizeOption,
) string {
	options := resolveOptions(opts)
	var b strings.Builder

	if theme.ThemeName != "" {
		fmt.Fprintf(&b, "%%%%{init: {'theme':'%s'}}%%%%\n", theme.ThemeName)
	}
	b.WriteString("erDiagram")

	for _, name := range sortedTableMap(tables) {
		table := tables[name]
		b.WriteString("\n    ")
		b.WriteString(name)
		b.WriteString(" {")
		if options.IncludeFields {
			b.WriteString("\n        string id PK")
			for _, f := range table.Fields {
				constraint := fieldConstraint(f.Name, table)
				constraintStr := ""
				if constraint != "" {
					constraintStr = " " + constraint
				}
				fmt.Fprintf(&b, "\n        %s %s%s", fieldTypeString(f.Type), f.Name, constraintStr)
			}
		}
		b.WriteString("\n    }")
	}

	for _, name := range sortedEdgeMap(edges) {
		edge := edges[name]
		if !options.IncludeFields || len(edge.Fields) == 0 {
			continue
		}
		b.WriteString("\n    ")
		b.WriteString(name)
		b.WriteString(" {")
		for _, f := range edge.Fields {
			fmt.Fprintf(&b, "\n        %s %s", fieldTypeString(f.Type), f.Name)
		}
		b.WriteString("\n    }")
	}

	if options.IncludeEdges {
		b.WriteString("\n")
		for _, name := range sortedEdgeMap(edges) {
			edge := edges[name]
			from := edge.FromTable
			if from == "" {
				from = "unknown"
			}
			to := edge.ToTable
			if to == "" {
				to = "unknown"
			}
			if _, ok := tables[from]; !ok && from != "unknown" {
				continue
			}
			if _, ok := tables[to]; !ok && to != "unknown" {
				continue
			}
			cardinality := mermaidCardinality(edge)
			fmt.Fprintf(&b, "\n    %s %s %s : %s", from, cardinality, to, name)
		}
	}

	return b.String()
}

// mermaidCardinality picks a Mermaid cardinality symbol based on edge
// semantics. Self-referential edges (from == to) are rendered many-to-many;
// everything else defaults to one-to-many.
func mermaidCardinality(edge EdgeDefinition) string {
	if edge.FromTable != "" && edge.FromTable == edge.ToTable {
		return "}o--o{"
	}
	return "||--o{"
}

// GenerateGraphViz renders a GraphViz DOT diagram from registry using theme.
func GenerateGraphViz(r *SchemaRegistry, theme GraphVizTheme, opts ...VisualizeOption) string {
	return GenerateGraphVizFromMaps(registryTables(r), registryEdges(r), theme, opts...)
}

// GenerateGraphVizFromMaps is like GenerateGraphViz but operates on raw
// table/edge maps.
func GenerateGraphVizFromMaps(
	tables map[string]TableDefinition,
	edges map[string]EdgeDefinition,
	theme GraphVizTheme,
	opts ...VisualizeOption,
) string {
	options := resolveOptions(opts)
	var b strings.Builder
	b.WriteString("digraph schema {\n")
	b.WriteString("    rankdir=LR;\n")

	applyTheme := theme.UseGradients || (theme.NodeStyle != "" && theme.NodeStyle != "filled,rounded")
	if applyTheme {
		if theme.BgColor != "" && theme.BgColor != "transparent" {
			fmt.Fprintf(&b, "    bgcolor=%q;\n", theme.BgColor)
		}
		fmt.Fprintf(&b, "    fontname=%q;\n", theme.FontName)

		nodeAttrs := []string{fmt.Sprintf("shape=%s", theme.NodeShape)}
		if theme.NodeStyle != "" {
			nodeAttrs = append(nodeAttrs, fmt.Sprintf("style=%q", theme.NodeStyle))
		}
		nodeAttrs = append(nodeAttrs, fmt.Sprintf("fontname=%q", theme.FontName))
		nodeAttrs = append(nodeAttrs, `pad="0.5"`, `margin="0.2"`)
		fmt.Fprintf(&b, "    node [%s];\n", strings.Join(nodeAttrs, ", "))

		edgeAttrs := []string{fmt.Sprintf("color=%q", theme.EdgeColor)}
		if theme.EdgeStyle != "" {
			edgeAttrs = append(edgeAttrs, fmt.Sprintf("style=%s", theme.EdgeStyle))
		}
		edgeAttrs = append(edgeAttrs, fmt.Sprintf("fontname=%q", theme.FontName))
		fmt.Fprintf(&b, "    edge [%s];\n", strings.Join(edgeAttrs, ", "))
	} else {
		b.WriteString("    node [shape=record];\n")
	}

	b.WriteString("\n")

	for _, name := range sortedTableMap(tables) {
		table := tables[name]
		label := buildGraphVizTableLabel(name, table, options.IncludeFields, theme)
		fmt.Fprintf(&b, "    %s [label=%s];\n", name, label)
	}
	for _, name := range sortedEdgeMap(edges) {
		edge := edges[name]
		if !options.IncludeFields || len(edge.Fields) == 0 {
			continue
		}
		label := buildGraphVizEdgeLabel(name, edge, theme)
		fmt.Fprintf(&b, "    %s [label=%s];\n", name, label)
	}

	b.WriteString("\n")

	if options.IncludeEdges {
		modern := ModernTheme()
		for _, name := range sortedEdgeMap(edges) {
			edge := edges[name]
			from := edge.FromTable
			to := edge.ToTable
			if from == "" || to == "" {
				continue
			}
			if _, ok := tables[from]; !ok {
				continue
			}
			if _, ok := tables[to]; !ok {
				continue
			}
			style := graphVizEdgeStyle(edge, theme, modern.Colors)
			fmt.Fprintf(&b, "    %s -> %s [label=%q%s];\n", from, to, name, style)
		}
	}

	b.WriteString("}")
	return b.String()
}

func buildGraphVizTableLabel(
	tableName string,
	table TableDefinition,
	includeFields bool,
	theme GraphVizTheme,
) string {
	if !includeFields {
		return fmt.Sprintf("%q", tableName)
	}
	if theme.UseGradients {
		return buildGraphVizHTMLLabel(tableName, table, theme)
	}
	parts := []string{tableName, "id : string (PK)\\l"}
	for _, f := range table.Fields {
		constraint := fieldConstraint(f.Name, table)
		constraintStr := ""
		if constraint != "" {
			constraintStr = " (" + constraint + ")"
		}
		parts = append(parts, fmt.Sprintf("%s : %s%s\\l", f.Name, fieldTypeString(f.Type), constraintStr))
	}
	return `"{` + strings.Join(parts, "|") + `}"`
}

func buildGraphVizHTMLLabel(tableName string, table TableDefinition, theme GraphVizTheme) string {
	modern := ModernTheme()
	var b strings.Builder
	b.WriteString("<")
	b.WriteString(`<TABLE BORDER="0" CELLBORDER="1" CELLSPACING="0" CELLPADDING="4">`)
	fmt.Fprintf(&b, `<TR><TD BGCOLOR="%s" COLSPAN="2">`, theme.NodeColor)
	fmt.Fprintf(&b, `<FONT COLOR="#FFFFFF"><B>%s</B></FONT>`, tableName)
	b.WriteString("</TD></TR>")

	b.WriteString("<TR>")
	b.WriteString(`<TD ALIGN="LEFT">id</TD>`)
	b.WriteString(`<TD ALIGN="LEFT">`)
	fmt.Fprintf(&b, `<FONT COLOR="%s">string</FONT>`, modern.Colors.Muted)
	fmt.Fprintf(&b, ` <FONT COLOR="%s">PK</FONT>`, modern.Colors.Error)
	b.WriteString("</TD></TR>")

	for _, f := range table.Fields {
		constraint := fieldConstraint(f.Name, table)
		fieldColor := fieldTypeColor(f.Type, modern.Colors)

		b.WriteString("<TR>")
		fmt.Fprintf(&b, `<TD ALIGN="LEFT">%s</TD>`, f.Name)
		b.WriteString(`<TD ALIGN="LEFT">`)
		fmt.Fprintf(&b, `<FONT COLOR="%s">%s</FONT>`, fieldColor, fieldTypeString(f.Type))
		if constraint != "" {
			constraintColor := constraintColor(constraint, modern.Colors)
			fmt.Fprintf(&b, ` <FONT COLOR="%s">%s</FONT>`, constraintColor, constraint)
		}
		b.WriteString("</TD></TR>")
	}
	b.WriteString("</TABLE>>")
	return b.String()
}

func buildGraphVizEdgeLabel(edgeName string, edge EdgeDefinition, theme GraphVizTheme) string {
	if theme.UseGradients {
		modern := ModernTheme()
		var b strings.Builder
		b.WriteString("<")
		b.WriteString(`<TABLE BORDER="0" CELLBORDER="1" CELLSPACING="0" CELLPADDING="4">`)
		fmt.Fprintf(&b, `<TR><TD BGCOLOR="%s" COLSPAN="2">`, theme.NodeColor)
		fmt.Fprintf(&b, `<FONT COLOR="#FFFFFF"><B>%s</B></FONT>`, edgeName)
		b.WriteString("</TD></TR>")
		for _, f := range edge.Fields {
			fieldColor := fieldTypeColor(f.Type, modern.Colors)
			b.WriteString("<TR>")
			fmt.Fprintf(&b, `<TD ALIGN="LEFT">%s</TD>`, f.Name)
			fmt.Fprintf(&b, `<TD ALIGN="LEFT"><FONT COLOR="%s">%s</FONT></TD>`, fieldColor, fieldTypeString(f.Type))
			b.WriteString("</TR>")
		}
		b.WriteString("</TABLE>>")
		return b.String()
	}
	parts := []string{edgeName}
	for _, f := range edge.Fields {
		parts = append(parts, fmt.Sprintf("%s : %s\\l", f.Name, fieldTypeString(f.Type)))
	}
	return `"{` + strings.Join(parts, "|") + `}"`
}

func graphVizEdgeStyle(edge EdgeDefinition, theme GraphVizTheme, colors ColorScheme) string {
	if edge.FromTable != "" && edge.FromTable == edge.ToTable {
		if theme.UseGradients {
			return fmt.Sprintf(`, style=dashed, color=%q`, colors.Secondary)
		}
		return ", style=dashed"
	}
	return ""
}

func fieldTypeColor(t FieldType, c ColorScheme) string {
	switch t {
	case FieldTypeString:
		return c.Success
	case FieldTypeInt, FieldTypeFloat, FieldTypeDecimal, FieldTypeNumber:
		return c.Warning
	case FieldTypeBool:
		return c.Accent
	case FieldTypeDatetime, FieldTypeDuration:
		return c.Secondary
	case FieldTypeRecord:
		return c.Primary
	case FieldTypeObject, FieldTypeArray:
		return c.Muted
	}
	return c.Text
}

func constraintColor(constraint string, c ColorScheme) string {
	switch constraint {
	case "PK":
		return c.Error
	case "FK":
		return c.Primary
	case "UK":
		return c.Accent
	}
	return c.Text
}

// GenerateASCII renders a plain-text ASCII diagram for registry using theme.
func GenerateASCII(r *SchemaRegistry, theme ASCIITheme, opts ...VisualizeOption) string {
	return GenerateASCIIFromMaps(registryTables(r), registryEdges(r), theme, opts...)
}

// GenerateASCIIFromMaps is like GenerateASCII but operates on raw
// table/edge maps.
func GenerateASCIIFromMaps(
	tables map[string]TableDefinition,
	edges map[string]EdgeDefinition,
	theme ASCIITheme,
	opts ...VisualizeOption,
) string {
	options := resolveOptions(opts)

	// Resolve the effective theme (zero value means "no unicode, no colors,
	// no icons" which matches surql-py's default behaviour).
	var lines []string
	for _, name := range sortedTableMap(tables) {
		box := buildASCIITableBox(name, tables[name], options.IncludeFields, theme)
		lines = append(lines, box...)
		lines = append(lines, "")
	}

	if options.IncludeEdges && len(edges) > 0 {
		lines = append(lines, "Relationships:")
		lines = append(lines, strings.Repeat("-", 40))
		for _, name := range sortedEdgeMap(edges) {
			edge := edges[name]
			from := edge.FromTable
			if from == "" {
				from = "?"
			}
			to := edge.ToTable
			if to == "" {
				to = "?"
			}
			lines = append(lines, fmt.Sprintf("  %s --[%s]--> %s", from, name, to))
		}
	}

	return strings.Join(lines, "\n")
}

func buildASCIITableBox(
	name string,
	table TableDefinition,
	includeFields bool,
	theme ASCIITheme,
) []string {
	chars := boxChars(theme)

	var fieldLines []string
	if includeFields {
		pkIcon := constraintIcon(theme, "PK")
		pkText := colorize(theme, fmt.Sprintf("%s(PK)", pkIcon), "pk")
		fieldLines = append(fieldLines, fmt.Sprintf("id : string %s", pkText))
		for _, f := range table.Fields {
			constraint := fieldConstraint(f.Name, table)
			constraintStr := ""
			if constraint != "" {
				icon := constraintIcon(theme, constraint)
				colorType := strings.ToLower(constraint)
				label := colorize(theme, fmt.Sprintf("%s(%s)", icon, constraint), colorType)
				constraintStr = " " + label
			}
			fieldLines = append(fieldLines, fmt.Sprintf("%s : %s%s", f.Name, fieldTypeString(f.Type), constraintStr))
		}
	}

	minWidth := len(name) + 4
	if minWidth < 20 {
		minWidth = 20
	}
	contentWidth := 0
	for _, l := range fieldLines {
		w := displayWidth(l)
		if w > contentWidth {
			contentWidth = w
		}
	}
	width := minWidth
	if contentWidth+2 > width {
		width = contentWidth + 2
	}

	top := chars["tl"] + repeatRune(chars["h"], width) + chars["tr"]
	bottom := chars["bl"] + repeatRune(chars["h"], width) + chars["br"]
	out := []string{top}

	styledName := colorize(theme, centerText(name, width), "header")
	leftPad, rightPad := centerPad(styledName, width)
	out = append(out, chars["v"]+strings.Repeat(" ", leftPad)+styledName+strings.Repeat(" ", rightPad)+chars["v"])

	if includeFields {
		separator := chars["ml"] + repeatRune(chars["h"], width) + chars["mr"]
		out = append(out, separator)
		for _, line := range fieldLines {
			visible := displayWidth(line)
			padding := width - visible - 1
			if padding < 0 {
				padding = 0
			}
			out = append(out, fmt.Sprintf("%s %s%s%s", chars["v"], line, strings.Repeat(" ", padding), chars["v"]))
		}
	}
	out = append(out, bottom)
	return out
}

// centerText produces a string whose display-width is >= width, centring
// text within that width using plain space padding. Matches Python
// str.center which places the extra padding char on the LEFT when the
// padding amount is odd.
func centerText(text string, width int) string {
	visible := displayWidth(text)
	if visible >= width {
		return text
	}
	padding := width - visible
	right := padding / 2
	left := padding - right
	return strings.Repeat(" ", left) + text + strings.Repeat(" ", right)
}
