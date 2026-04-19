// Command surql is the CLI entry point for the surql-go toolkit.
//
// The actual subcommand tree (migrate / schema / db / orchestrate) lives
// in internal/cli so that main() stays a thin shim wired to
// goreleaser-provided -ldflags for version metadata.
package main

import (
	"os"

	"github.com/Oneiriq/surql-go/internal/cli"
	"github.com/Oneiriq/surql-go/pkg/surql"
)

// Build-time populated by goreleaser via -ldflags.
var (
	version = surql.Version
	commit  = ""
	date    = ""
)

func main() {
	os.Exit(cli.Execute(cli.BuildInfo{
		Version: version,
		Commit:  commit,
		Date:    date,
	}))
}
