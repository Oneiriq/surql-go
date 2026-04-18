package schema

import (
	"sort"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

// ColorScheme is the base color palette used across visualization themes.
// All colors are hex color codes (e.g. "#6366f1").
type ColorScheme struct {
	Primary    string
	Secondary  string
	Background string
	Text       string
	Accent     string
	Success    string
	Warning    string
	Error      string
	Muted      string
}

// GraphVizTheme controls visual aspects of GraphViz DOT output: node and edge
// styling, layout, and advanced features like gradients and clustering.
type GraphVizTheme struct {
	NodeColor    string
	EdgeColor    string
	BgColor      string // use "transparent" for none
	FontName     string
	NodeShape    string // e.g. "record", "box"
	NodeStyle    string // e.g. "filled,rounded"
	EdgeStyle    string // e.g. "solid", "dashed"
	UseGradients bool
	UseClusters  bool
}

// MermaidTheme controls Mermaid ER diagram appearance via Mermaid's theming
// system. Supports both built-in themes and custom CSS variables.
type MermaidTheme struct {
	ThemeName      string // one of "default", "dark", "forest", "neutral", "base"
	PrimaryColor   string
	SecondaryColor string
	UseCustomCSS   bool
}

// ASCIITheme controls ASCII diagram rendering including box drawing
// characters, ANSI colors, and Unicode icons.
type ASCIITheme struct {
	BoxStyle    string // one of "single", "double", "rounded", "heavy"
	UseUnicode  bool
	UseColors   bool
	UseIcons    bool
	ColorScheme string
}

// Theme bundles a color scheme with format-specific configurations into a
// coherent visual design.
type Theme struct {
	Name        string
	Description string
	Colors      ColorScheme
	GraphViz    GraphVizTheme
	Mermaid     MermaidTheme
	ASCII       ASCIITheme
}

// ModernColorScheme returns the clean professional palette used by the
// "modern" preset (indigo primary, pink secondary, slate backgrounds).
func ModernColorScheme() ColorScheme {
	return ColorScheme{
		Primary:    "#6366f1",
		Secondary:  "#ec4899",
		Background: "#f8fafc",
		Text:       "#0f172a",
		Accent:     "#8b5cf6",
		Success:    "#10b981",
		Warning:    "#f59e0b",
		Error:      "#ef4444",
		Muted:      "#94a3b8",
	}
}

// DarkColorScheme returns the dark-mode palette (violet primary, fuchsia
// secondary, indigo-950 background).
func DarkColorScheme() ColorScheme {
	return ColorScheme{
		Primary:    "#8b5cf6",
		Secondary:  "#d946ef",
		Background: "#1e1b4b",
		Text:       "#f1f5f9",
		Accent:     "#a78bfa",
		Success:    "#34d399",
		Warning:    "#fbbf24",
		Error:      "#f87171",
		Muted:      "#64748b",
	}
}

// ForestColorScheme returns the nature-inspired palette (emerald primary,
// teal secondary, green-50 background).
func ForestColorScheme() ColorScheme {
	return ColorScheme{
		Primary:    "#10b981",
		Secondary:  "#14b8a6",
		Background: "#f0fdf4",
		Text:       "#14532d",
		Accent:     "#059669",
		Success:    "#22c55e",
		Warning:    "#f59e0b",
		Error:      "#ef4444",
		Muted:      "#86efac",
	}
}

// MinimalColorScheme returns the grayscale palette used by the "minimal"
// preset.
func MinimalColorScheme() ColorScheme {
	return ColorScheme{
		Primary:    "#6b7280",
		Secondary:  "#64748b",
		Background: "#ffffff",
		Text:       "#1f2937",
		Accent:     "#9ca3af",
		Success:    "#10b981",
		Warning:    "#f59e0b",
		Error:      "#ef4444",
		Muted:      "#d1d5db",
	}
}

// ModernGraphVizTheme returns the GraphViz styling for the "modern" preset.
func ModernGraphVizTheme() GraphVizTheme {
	return GraphVizTheme{
		NodeColor:    "#6366f1",
		EdgeColor:    "#64748b",
		BgColor:      "transparent",
		FontName:     "Arial",
		NodeShape:    "record",
		NodeStyle:    "filled,rounded",
		EdgeStyle:    "solid",
		UseGradients: true,
		UseClusters:  false,
	}
}

// DarkGraphVizTheme returns the GraphViz styling for the "dark" preset.
func DarkGraphVizTheme() GraphVizTheme {
	return GraphVizTheme{
		NodeColor:    "#8b5cf6",
		EdgeColor:    "#64748b",
		BgColor:      "#1e1b4b",
		FontName:     "Arial",
		NodeShape:    "record",
		NodeStyle:    "filled,rounded",
		EdgeStyle:    "solid",
		UseGradients: true,
		UseClusters:  false,
	}
}

// ForestGraphVizTheme returns the GraphViz styling for the "forest" preset.
func ForestGraphVizTheme() GraphVizTheme {
	return GraphVizTheme{
		NodeColor:    "#10b981",
		EdgeColor:    "#059669",
		BgColor:      "transparent",
		FontName:     "Arial",
		NodeShape:    "record",
		NodeStyle:    "filled,rounded",
		EdgeStyle:    "solid",
		UseGradients: true,
		UseClusters:  false,
	}
}

// MinimalGraphVizTheme returns the GraphViz styling for the "minimal" preset.
func MinimalGraphVizTheme() GraphVizTheme {
	return GraphVizTheme{
		NodeColor:    "#6b7280",
		EdgeColor:    "#9ca3af",
		BgColor:      "transparent",
		FontName:     "Arial",
		NodeShape:    "record",
		NodeStyle:    "filled",
		EdgeStyle:    "solid",
		UseGradients: false,
		UseClusters:  false,
	}
}

// ModernMermaidTheme returns the Mermaid styling for the "modern" preset.
func ModernMermaidTheme() MermaidTheme {
	return MermaidTheme{
		ThemeName:      "default",
		PrimaryColor:   "#6366f1",
		SecondaryColor: "#ec4899",
		UseCustomCSS:   true,
	}
}

// DarkMermaidTheme returns the Mermaid styling for the "dark" preset.
func DarkMermaidTheme() MermaidTheme {
	return MermaidTheme{
		ThemeName:      "dark",
		PrimaryColor:   "#8b5cf6",
		SecondaryColor: "#d946ef",
		UseCustomCSS:   true,
	}
}

// ForestMermaidTheme returns the Mermaid styling for the "forest" preset.
func ForestMermaidTheme() MermaidTheme {
	return MermaidTheme{
		ThemeName:      "forest",
		PrimaryColor:   "#10b981",
		SecondaryColor: "#14b8a6",
		UseCustomCSS:   true,
	}
}

// MinimalMermaidTheme returns the Mermaid styling for the "minimal" preset.
func MinimalMermaidTheme() MermaidTheme {
	return MermaidTheme{
		ThemeName:      "neutral",
		PrimaryColor:   "#6b7280",
		SecondaryColor: "#64748b",
		UseCustomCSS:   true,
	}
}

// ModernASCIITheme returns the ASCII styling for the "modern" preset
// (rounded unicode boxes, colors, and icons enabled).
func ModernASCIITheme() ASCIITheme {
	return ASCIITheme{
		BoxStyle:    "rounded",
		UseUnicode:  true,
		UseColors:   true,
		UseIcons:    true,
		ColorScheme: "default",
	}
}

// DarkASCIITheme returns the ASCII styling for the "dark" preset.
func DarkASCIITheme() ASCIITheme {
	return ASCIITheme{
		BoxStyle:    "rounded",
		UseUnicode:  true,
		UseColors:   true,
		UseIcons:    true,
		ColorScheme: "dark",
	}
}

// ForestASCIITheme returns the ASCII styling for the "forest" preset.
func ForestASCIITheme() ASCIITheme {
	return ASCIITheme{
		BoxStyle:    "rounded",
		UseUnicode:  true,
		UseColors:   true,
		UseIcons:    true,
		ColorScheme: "forest",
	}
}

// MinimalASCIITheme returns the ASCII styling for the "minimal" preset
// (single-line boxes, no colors, no icons).
func MinimalASCIITheme() ASCIITheme {
	return ASCIITheme{
		BoxStyle:    "single",
		UseUnicode:  true,
		UseColors:   false,
		UseIcons:    false,
		ColorScheme: "minimal",
	}
}

// ModernTheme returns the "modern" bundled theme.
func ModernTheme() Theme {
	return Theme{
		Name:        "modern",
		Description: "Clean, professional design with indigo and pink accents",
		Colors:      ModernColorScheme(),
		GraphViz:    ModernGraphVizTheme(),
		Mermaid:     ModernMermaidTheme(),
		ASCII:       ModernASCIITheme(),
	}
}

// DarkTheme returns the "dark" bundled theme.
func DarkTheme() Theme {
	return Theme{
		Name:        "dark",
		Description: "Dark background theme with violet and fuchsia for dark mode environments",
		Colors:      DarkColorScheme(),
		GraphViz:    DarkGraphVizTheme(),
		Mermaid:     DarkMermaidTheme(),
		ASCII:       DarkASCIITheme(),
	}
}

// ForestTheme returns the "forest" bundled theme.
func ForestTheme() Theme {
	return Theme{
		Name:        "forest",
		Description: "Nature-inspired theme with emerald and teal on light green background",
		Colors:      ForestColorScheme(),
		GraphViz:    ForestGraphVizTheme(),
		Mermaid:     ForestMermaidTheme(),
		ASCII:       ForestASCIITheme(),
	}
}

// MinimalTheme returns the "minimal" bundled theme.
func MinimalTheme() Theme {
	return Theme{
		Name:        "minimal",
		Description: "Minimalist grayscale theme with subtle styling",
		Colors:      MinimalColorScheme(),
		GraphViz:    MinimalGraphVizTheme(),
		Mermaid:     MinimalMermaidTheme(),
		ASCII:       MinimalASCIITheme(),
	}
}

// GetTheme returns a preset theme by name. Recognised names are "modern",
// "dark", "forest", and "minimal". Unknown names return a wrapped
// ErrValidation.
func GetTheme(name string) (Theme, error) {
	switch name {
	case "modern":
		return ModernTheme(), nil
	case "dark":
		return DarkTheme(), nil
	case "forest":
		return ForestTheme(), nil
	case "minimal":
		return MinimalTheme(), nil
	}
	available := ListThemes()
	return Theme{}, surqlerrors.Newf(surqlerrors.ErrValidation,
		"unknown theme %q. Available themes: %v", name, available)
}

// ListThemes returns the names of every built-in preset, sorted
// alphabetically.
func ListThemes() []string {
	names := []string{"dark", "forest", "minimal", "modern"}
	sort.Strings(names)
	return names
}
