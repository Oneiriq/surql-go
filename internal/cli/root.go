package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/Oneiriq/surql-go/pkg/surql"
	"github.com/Oneiriq/surql-go/pkg/surql/connection"
	"github.com/Oneiriq/surql-go/pkg/surql/settings"
)

// BuildInfo is populated at build time via -ldflags so `surql version`
// can surface the commit / date. Exported so cmd/surql/main.go can
// populate it from goreleaser-provided vars.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// defaultBuildInfo resolves to the library version when no ldflags are set.
func defaultBuildInfo() BuildInfo {
	return BuildInfo{Version: surql.Version}
}

// globalFlags captures process-level flags applied to every subcommand.
// Values are populated by cobra in PersistentPreRunE; downstream commands
// consume them via rootContext.
type globalFlags struct {
	ConfigPath string
	Verbose    bool
	Quiet      bool
	NoColor    bool
	JSONOut    bool
}

// rootContext bundles per-invocation context shared by every subcommand:
// resolved settings, a printer, and the global flags. Commands pull it out
// of cobra.Command.Context() via contextKey.
type rootContext struct {
	Flags    globalFlags
	Settings *settings.Settings
	Printer  *Printer
	Build    BuildInfo
}

// contextKey is unexported to prevent collisions with other packages
// stashing values in cobra's context.
type contextKey struct{}

// withRootContext stores rc in ctx.
func withRootContext(ctx context.Context, rc *rootContext) context.Context {
	return context.WithValue(ctx, contextKey{}, rc)
}

// rootFromCmd returns the rootContext stored on cmd (panics if missing —
// PersistentPreRunE always populates it).
func rootFromCmd(cmd *cobra.Command) *rootContext {
	ctx := cmd.Context()
	if ctx == nil {
		return nil
	}
	rc, _ := ctx.Value(contextKey{}).(*rootContext)
	return rc
}

// Execute is the entry point called from cmd/surql/main.go. It constructs
// the root cobra command with the supplied build info, executes it against
// os.Args[1:], and returns the appropriate exit code.
func Execute(build BuildInfo) int {
	return ExecuteWithArgs(build, os.Args[1:], os.Stdout, os.Stderr)
}

// ExecuteWithArgs is the testable form of Execute: args / writers are
// supplied explicitly rather than being pulled from the process. It is the
// single entry point used by every integration test.
func ExecuteWithArgs(build BuildInfo, args []string, out, errOut io.Writer) int {
	root := NewRootCommand(build)
	root.SetOut(out)
	root.SetErr(errOut)
	root.SetArgs(args)

	// Signal handling: translate SIGINT / SIGTERM into context cancellation
	// so long-running commands (watch, migrate up against a slow DB) exit
	// gracefully.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := root.ExecuteContext(ctx); err != nil {
		return classifyError(errOut, err)
	}
	return ExitSuccess
}

// NewRootCommand builds the root `surql` cobra command, attaches global
// flags, and registers every subcommand group.
func NewRootCommand(build BuildInfo) *cobra.Command {
	flags := &globalFlags{}

	cmd := &cobra.Command{
		Use:           "surql",
		Short:         "SurrealDB ORM and migration toolkit",
		Long:          "surql is the SurrealDB ORM, schema, and migration toolkit for Go.",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       build.Version,
	}
	cmd.SetVersionTemplate("surql {{.Version}}\n")

	cmd.PersistentFlags().StringVar(&flags.ConfigPath, "config", "", "path to surql.yaml / surql.toml (overrides auto-discovery)")
	cmd.PersistentFlags().BoolVar(&flags.Verbose, "verbose", false, "enable verbose output")
	cmd.PersistentFlags().BoolVarP(&flags.Quiet, "quiet", "q", false, "suppress informational output")
	cmd.PersistentFlags().BoolVar(&flags.NoColor, "no-color", false, "disable ANSI color output")
	cmd.PersistentFlags().BoolVar(&flags.JSONOut, "json", false, "emit JSON output where supported")
	// The top-level `-v` shorthand matches `surql version` (per the parity
	// spec). Cobra consumes `--version` automatically; we wire `-v` with a
	// custom Lookup on the generated flag so the short form resolves.
	if f := cmd.Flags().Lookup("version"); f != nil {
		f.Shorthand = "v"
	}

	// PersistentPreRunE resolves settings once per invocation so every
	// subcommand sees the same view. Any resolver failure surfaces as a
	// usage-style error (ExitFailure) with a clear message.
	cmd.PersistentPreRunE = func(c *cobra.Command, _ []string) error {
		outW := c.OutOrStdout()
		errW := c.ErrOrStderr()
		useColor := !flags.NoColor && colorEnabled(outW)
		printer := NewPrinter(outW, errW, useColor)
		printer.SetQuiet(flags.Quiet)

		rc := &rootContext{
			Flags:   *flags,
			Printer: printer,
			Build:   build,
		}

		// `version` / help / completion do not need settings; skip the
		// resolver so those commands succeed even without a config file.
		if !requiresSettings(c) {
			c.SetContext(withRootContext(c.Context(), rc))
			return nil
		}

		loadOpts := []any{}
		if flags.ConfigPath != "" {
			loadOpts = append(loadOpts, settings.WithConfigFile(flags.ConfigPath))
		}
		s, err := settings.LoadSettings(loadOpts...)
		if err != nil {
			// Still set rc so the caller has a printer; operation failed.
			c.SetContext(withRootContext(c.Context(), rc))
			return fmt.Errorf("failed to load settings: %w", err)
		}
		rc.Settings = s
		c.SetContext(withRootContext(c.Context(), rc))
		return nil
	}

	cmd.AddCommand(
		newVersionCommand(build),
		newMigrateCommand(),
		newSchemaCommand(),
		newDBCommand(),
		newOrchestrateCommand(),
	)

	return cmd
}

// requiresSettings returns false for command paths that never need to read
// settings (version, help, completion). Everything else goes through the
// resolver.
func requiresSettings(c *cobra.Command) bool {
	for cur := c; cur != nil; cur = cur.Parent() {
		switch cur.Name() {
		case "version", "help", "completion":
			return false
		}
	}
	return true
}

// newVersionCommand renders `surql version` identically to `--version`.
func newVersionCommand(build BuildInfo) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show the surql version",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			out := c.OutOrStdout()
			fmt.Fprintf(out, "surql %s", build.Version)
			if build.Commit != "" {
				fmt.Fprintf(out, " (commit %s)", build.Commit)
			}
			if build.Date != "" {
				fmt.Fprintf(out, " built %s", build.Date)
			}
			fmt.Fprintln(out)
			return nil
		},
	}
}

// usageError wraps an error that represents bad CLI invocation (missing
// argument, invalid flag value). classifyError maps it to ExitUsage.
type usageError struct{ err error }

func (e *usageError) Error() string { return e.err.Error() }
func (e *usageError) Unwrap() error { return e.err }

// newUsageError wraps msg as a usage-style error so the root returns
// ExitUsage instead of ExitFailure.
func newUsageError(format string, args ...any) error {
	return &usageError{err: fmt.Errorf(format, args...)}
}

// classifyError maps a command error into an exit code and prints it to
// errOut. Unknown-flag / unknown-command errors from cobra are treated as
// usage errors; everything else is a runtime failure.
func classifyError(errOut io.Writer, err error) int {
	if err == nil {
		return ExitSuccess
	}
	var ue *usageError
	if errors.As(err, &ue) {
		fmt.Fprintln(errOut, "surql: "+ue.Error())
		return ExitUsage
	}
	msg := err.Error()
	fmt.Fprintln(errOut, "surql: "+msg)
	return ExitFailure
}

// newConnectedClient dials the database using the resolved settings and
// returns a connected client together with a cleanup function the caller
// must defer.
func newConnectedClient(ctx context.Context, rc *rootContext) (*connection.DatabaseClient, func(), error) {
	if rc == nil || rc.Settings == nil {
		return nil, func() {}, fmt.Errorf("settings unavailable")
	}
	client, err := connection.NewDatabaseClient(rc.Settings.Database)
	if err != nil {
		return nil, func() {}, err
	}
	if err := client.Connect(ctx); err != nil {
		return nil, func() {}, err
	}
	cleanup := func() { _ = client.Disconnect() }
	return client, cleanup, nil
}

// migrationsDir returns the effective migrations directory, preferring
// rc.Settings.MigrationPath and falling back to the per-command override.
func migrationsDir(rc *rootContext, override string) string {
	if override != "" {
		return override
	}
	if rc != nil && rc.Settings != nil && rc.Settings.MigrationPath != "" {
		return rc.Settings.MigrationPath
	}
	return "migrations"
}
