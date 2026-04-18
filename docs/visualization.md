# Visualization

Render Mermaid / GraphViz / ASCII diagrams straight from a
`SchemaRegistry`.

## Mermaid

```go
import "github.com/Oneiriq/surql-go/pkg/surql/schema"

theme := schema.ModernMermaidTheme()
out   := schema.GenerateMermaid(reg, theme)
fmt.Println(out)
```

## GraphViz DOT

```go
theme := schema.DarkGraphVizTheme()
out   := schema.GenerateGraphViz(reg, theme)
_ = os.WriteFile("schema.dot", []byte(out), 0o644)
```

## ASCII

```go
theme := schema.MinimalASCIITheme()
out   := schema.GenerateASCII(reg, theme)
fmt.Println(out)
```

## Themes

| Preset                       | Mood                                        |
|------------------------------|---------------------------------------------|
| `Modern{Mermaid,GraphViz,ASCII}Theme()` | Bright accents, clean typography, emoji. |
| `Dark…Theme()`               | Dim palette, good on dark terminals.        |
| `Forest…Theme()`             | Earthy greens, muted secondary color.       |
| `Minimal…Theme()`            | Monochrome, no decorative glyphs.           |

`GetTheme(name)` returns an `ErrValidation`-wrapped error for unknown
names. `ListThemes()` returns the sorted preset list.

## What's next

- **[Schema Definition](schema.md)** -- building the registry the
  visualizer consumes.
