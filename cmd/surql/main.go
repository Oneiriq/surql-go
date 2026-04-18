// Command surql is the CLI entry point for the surql-go toolkit.
//
// Subcommands (migrate, schema, db, orchestrate) are added incrementally
// as the library port progresses. See README.md for the target surface.
package main

import (
	"fmt"
	"os"

	"github.com/albedosehen/surql-go/pkg/surql"
)

// build-time populated by goreleaser via -ldflags.
var (
	version = surql.Version
	commit  = ""
	date    = ""
)

func main() {
	if len(os.Args) >= 2 && (os.Args[1] == "-v" || os.Args[1] == "--version" || os.Args[1] == "version") {
		fmt.Printf("surql %s", version)
		if commit != "" {
			fmt.Printf(" (commit %s)", commit)
		}
		if date != "" {
			fmt.Printf(" built %s", date)
		}
		fmt.Println()
		return
	}

	fmt.Fprintln(os.Stderr, "surql-go CLI is under construction; subcommands will land with each release.")
	fmt.Fprintln(os.Stderr, "Use `surql --version` to inspect the current build.")
	os.Exit(0)
}
