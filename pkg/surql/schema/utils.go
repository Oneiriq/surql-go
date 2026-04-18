package schema

import (
	"regexp"
	"strings"
)

// ansiEscapePattern matches ANSI CSI escape sequences so that display-width
// calculations ignore color codes.
var ansiEscapePattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// displayWidth returns the approximate terminal column width of text,
// stripping ANSI escape codes and counting East Asian Wide / Fullwidth
// characters as two columns. Emoji fall into the Wide category on most
// terminals.
func displayWidth(text string) int {
	stripped := ansiEscapePattern.ReplaceAllString(text, "")
	width := 0
	for _, r := range stripped {
		if r < 0x20 {
			continue
		}
		if isWideRune(r) {
			width += 2
			continue
		}
		width++
	}
	return width
}

// isWideRune reports whether r occupies two terminal columns. Implemented as
// a conservative range table covering the CJK, Fullwidth, emoji, and
// Miscellaneous Symbols and Pictographs blocks used by the emoji icons
// produced by the visualizer.
func isWideRune(r rune) bool {
	if r < 0x1100 {
		return false
	}
	switch {
	case r >= 0x1100 && r <= 0x115F: // Hangul Jamo
		return true
	case r >= 0x2E80 && r <= 0x303E: // CJK Radicals Supplement, Kangxi
		return true
	case r >= 0x3041 && r <= 0x33FF: // Hiragana through CJK Compatibility
		return true
	case r >= 0x3400 && r <= 0x4DBF: // CJK Unified Ideographs Extension A
		return true
	case r >= 0x4E00 && r <= 0x9FFF: // CJK Unified Ideographs
		return true
	case r >= 0xA000 && r <= 0xA4CF: // Yi Syllables
		return true
	case r >= 0xAC00 && r <= 0xD7A3: // Hangul Syllables
		return true
	case r >= 0xF900 && r <= 0xFAFF: // CJK Compatibility Ideographs
		return true
	case r >= 0xFE30 && r <= 0xFE4F: // CJK Compatibility Forms
		return true
	case r >= 0xFF00 && r <= 0xFF60: // Fullwidth Forms
		return true
	case r >= 0xFFE0 && r <= 0xFFE6: // Fullwidth Signs
		return true
	case r >= 0x1F300 && r <= 0x1F64F: // Misc Symbols and Pictographs + Emoticons
		return true
	case r >= 0x1F680 && r <= 0x1F6FF: // Transport and Map Symbols
		return true
	case r >= 0x1F900 && r <= 0x1F9FF: // Supplemental Symbols and Pictographs
		return true
	case r >= 0x1FA70 && r <= 0x1FAFF: // Symbols and Pictographs Extended-A
		return true
	case r >= 0x20000 && r <= 0x2FFFD: // CJK Unified Ideographs Extension B-F
		return true
	case r >= 0x30000 && r <= 0x3FFFD: // CJK Unified Ideographs Extension G
		return true
	}
	return false
}

// fieldConstraint returns the constraint suffix (PK, FK, UK) that should be
// displayed next to a field in diagrams. An empty string means the field has
// no constraint.
//
// The rules mirror surql-py: "id" is always PK; a field that appears alone in
// a unique index is UK; a FieldTypeRecord field is FK.
func fieldConstraint(fieldName string, table TableDefinition) string {
	if fieldName == "id" {
		return "PK"
	}
	for _, idx := range table.Indexes {
		if idx.Type != IndexTypeUnique {
			continue
		}
		for _, col := range idx.Columns {
			if col == fieldName {
				return "UK"
			}
		}
	}
	for _, f := range table.Fields {
		if f.Name == fieldName && f.Type == FieldTypeRecord {
			return "FK"
		}
	}
	return ""
}

// fieldTypeString returns the user-facing string form of a FieldType
// (currently the enum value as a lowercase keyword).
func fieldTypeString(t FieldType) string {
	return string(t)
}

// centerPad returns text padded with spaces so the visible display-width is
// width. Padding is split to match Python's str.center, which places the
// extra space on the LEFT when padding is odd. Text wider than width is
// returned unchanged.
func centerPad(text string, width int) (left, right int) {
	visible := displayWidth(text)
	if visible >= width {
		return 0, 0
	}
	padding := width - visible
	right = padding / 2
	left = padding - right
	return left, right
}

// boxChars returns the box drawing characters to use for the supplied ASCII
// theme. When the theme is the zero value or has UseUnicode disabled, it
// returns plain ASCII characters.
func boxChars(theme ASCIITheme) map[string]string {
	if !theme.UseUnicode {
		return map[string]string{
			"tl": "+", "tr": "+", "bl": "+", "br": "+",
			"h": "-", "v": "|", "ml": "+", "mr": "+",
		}
	}
	switch theme.BoxStyle {
	case "double":
		return map[string]string{
			"tl": "╔", "tr": "╗", "bl": "╚", "br": "╝",
			"h": "═", "v": "║", "ml": "╠", "mr": "╣",
		}
	case "rounded":
		return map[string]string{
			"tl": "╭", "tr": "╮", "bl": "╰", "br": "╯",
			"h": "─", "v": "│", "ml": "├", "mr": "┤",
		}
	case "heavy":
		return map[string]string{
			"tl": "┏", "tr": "┓", "bl": "┗", "br": "┛",
			"h": "━", "v": "┃", "ml": "┣", "mr": "┫",
		}
	default: // "single" (and anything unknown)
		return map[string]string{
			"tl": "┌", "tr": "┐", "bl": "└", "br": "┘",
			"h": "─", "v": "│", "ml": "├", "mr": "┤",
		}
	}
}

// ansi color codes used when ASCIITheme.UseColors is true.
const (
	ansiPK     = "\x1b[91m"
	ansiFK     = "\x1b[94m"
	ansiUK     = "\x1b[95m"
	ansiHeader = "\x1b[1m"
	ansiReset  = "\x1b[0m"
)

// colorize wraps text in an ANSI color sequence based on color_type. If the
// theme disables colors, text is returned unchanged.
func colorize(theme ASCIITheme, text, colorType string) string {
	if !theme.UseColors {
		return text
	}
	var code string
	switch strings.ToLower(colorType) {
	case "pk":
		code = ansiPK
	case "fk":
		code = ansiFK
	case "uk":
		code = ansiUK
	case "header":
		code = ansiHeader
	case "field", "":
		return text
	default:
		return text
	}
	return code + text + ansiReset
}

// constraintIcon returns the unicode icon used next to a constraint label in
// ASCII diagrams, or an empty string when the theme disables icons.
func constraintIcon(theme ASCIITheme, constraint string) string {
	if !theme.UseIcons {
		return ""
	}
	switch constraint {
	case "PK":
		return "🔑 "
	case "FK":
		return "🔗 "
	case "UK":
		return "⭐ "
	}
	return ""
}

// repeatRune returns a string containing n copies of s. It is a convenience
// helper for building box top/bottom/middle lines.
func repeatRune(s string, n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat(s, n)
}
