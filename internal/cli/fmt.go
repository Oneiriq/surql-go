// Package cli implements the `surql` command-line interface.
//
// The CLI is a thin wrapper over the public packages in
// github.com/Oneiriq/surql-go/pkg/surql: it wires cobra commands to the
// migration / schema / connection / orchestration / settings APIs and
// handles human-facing output formatting (tables, JSON, ANSI colors).
//
// This file provides the shared formatting helpers; each subcommand group
// lives in its own file (root.go, migrate.go, schema.go, db.go, orchestrate.go).
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// Exit codes used consistently across every subcommand.
const (
	// ExitSuccess indicates the command completed successfully.
	ExitSuccess = 0
	// ExitFailure indicates the requested operation failed at runtime
	// (connection error, migration failure, validation drift, ...).
	ExitFailure = 1
	// ExitUsage indicates bad invocation (unknown flag, missing argument).
	ExitUsage = 2
)

// ANSI color escape sequences. Coloring is only emitted when stdout is a
// terminal; Writer methods inspect NO_COLOR and isatty before rendering
// escapes.
const (
	ansiReset  = "\x1b[0m"
	ansiBold   = "\x1b[1m"
	ansiRed    = "\x1b[31m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	ansiBlue   = "\x1b[34m"
	ansiCyan   = "\x1b[36m"
	ansiGray   = "\x1b[90m"
)

// Status markers (ASCII only — see "no emojis" policy).
const (
	markOK   = "[ok]"
	markErr  = "[err]"
	markWarn = "[warn]"
	markInfo = "[info]"
)

// Printer writes human-oriented output. It centralises color handling so
// individual commands never call fmt.Fprintln directly.
//
// The zero value is not usable; construct with NewPrinter.
type Printer struct {
	out    io.Writer
	err    io.Writer
	color  bool
	quiet  bool
	silent bool
}

// NewPrinter returns a Printer wrapping stdout / stderr. color controls
// whether ANSI escapes are emitted; callers typically pass the result of
// colorEnabled(os.Stdout).
func NewPrinter(out, err io.Writer, color bool) *Printer {
	return &Printer{out: out, err: err, color: color}
}

// Out exposes the wrapped stdout writer (used by raw JSON / file writes).
func (p *Printer) Out() io.Writer { return p.out }

// Err exposes the wrapped stderr writer.
func (p *Printer) Err() io.Writer { return p.err }

// SetQuiet suppresses info / success messages but keeps warnings and
// errors. Useful for scripted invocations.
func (p *Printer) SetQuiet(quiet bool) { p.quiet = quiet }

// SetSilent suppresses every status message (info / success / warn /
// err). Errors still exit with the right code; they just do not print.
func (p *Printer) SetSilent(silent bool) { p.silent = silent }

// colorize wraps text in the given ANSI color when color is enabled.
func (p *Printer) colorize(text, color string) string {
	if !p.color || color == "" {
		return text
	}
	return color + text + ansiReset
}

// Infof prints an informational message to stdout. Suppressed in quiet mode.
func (p *Printer) Infof(format string, args ...any) {
	if p.silent || p.quiet {
		return
	}
	marker := p.colorize(markInfo, ansiBlue)
	fmt.Fprintf(p.out, "%s %s\n", marker, fmt.Sprintf(format, args...))
}

// Successf prints a success message to stdout. Suppressed in quiet mode.
func (p *Printer) Successf(format string, args ...any) {
	if p.silent || p.quiet {
		return
	}
	marker := p.colorize(markOK, ansiGreen)
	fmt.Fprintf(p.out, "%s %s\n", marker, fmt.Sprintf(format, args...))
}

// Warnf prints a warning to stderr.
func (p *Printer) Warnf(format string, args ...any) {
	if p.silent {
		return
	}
	marker := p.colorize(markWarn, ansiYellow)
	fmt.Fprintf(p.err, "%s %s\n", marker, fmt.Sprintf(format, args...))
}

// Errorf prints an error to stderr.
func (p *Printer) Errorf(format string, args ...any) {
	if p.silent {
		return
	}
	marker := p.colorize(markErr, ansiRed)
	fmt.Fprintf(p.err, "%s %s\n", marker, fmt.Sprintf(format, args...))
}

// Plainf prints a message with no marker / color to stdout.
func (p *Printer) Plainf(format string, args ...any) {
	fmt.Fprintf(p.out, format+"\n", args...)
}

// Section prints a bold section header to stdout.
func (p *Printer) Section(title string) {
	if p.silent {
		return
	}
	fmt.Fprintln(p.out, p.colorize(title, ansiBold+ansiCyan))
}

// JSON marshals v and writes the indented form to stdout. Returns an
// error if marshal fails; callers propagate it as an ExitFailure.
func (p *Printer) JSON(v any) error {
	enc := json.NewEncoder(p.out)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// Table renders a simple right-padded ASCII table to stdout. Every row
// must have the same number of columns as the header.
func (p *Printer) Table(headers []string, rows [][]string) {
	if len(headers) == 0 {
		return
	}
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i >= len(widths) {
				continue
			}
			if l := len(cell); l > widths[i] {
				widths[i] = l
			}
		}
	}
	renderRow := func(cols []string) {
		parts := make([]string, 0, len(cols))
		for i, c := range cols {
			if i >= len(widths) {
				break
			}
			parts = append(parts, padRight(c, widths[i]))
		}
		fmt.Fprintln(p.out, strings.Join(parts, "  "))
	}
	headerText := make([]string, len(headers))
	for i, h := range headers {
		headerText[i] = p.colorize(padRight(h, widths[i]), ansiBold+ansiCyan)
	}
	fmt.Fprintln(p.out, strings.Join(headerText, "  "))
	// Underline beneath headers — use dashes sized to each column.
	sep := make([]string, len(widths))
	for i, w := range widths {
		sep[i] = strings.Repeat("-", w)
	}
	fmt.Fprintln(p.out, p.colorize(strings.Join(sep, "  "), ansiGray))
	for _, row := range rows {
		renderRow(row)
	}
}

// padRight returns s padded with spaces so its length is at least width.
func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

// colorEnabled reports whether ANSI colors should be emitted given NO_COLOR
// and TERM values. w is inspected when it is an *os.File (so tests can pass
// an io.Discard without colors).
func colorEnabled(w io.Writer) bool {
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return false
	}
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// sortedStringKeys returns keys of m in sorted order. Small utility used
// by commands that render maps as tables.
func sortedStringKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// truncateString shortens s to max (with a trailing "...") for table cells.
// Returns s unchanged if its length is already <= max.
func truncateString(s string, max int) string {
	if max <= 3 || len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
