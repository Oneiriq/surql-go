package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Oneiriq/surql-go/pkg/surql/migration"
)

// newDBCommand wires `surql db`.
func newDBCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "db",
		Short: "Database utility commands",
	}
	cmd.AddCommand(
		newDBInitCommand(),
		newDBPingCommand(),
		newDBInfoCommand(),
		newDBResetCommand(),
		newDBQueryCommand(),
		newDBVersionCommand(),
	)
	return cmd
}

// ---------------------------------------------------------------------------
// db init
// ---------------------------------------------------------------------------

func newDBInitCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize the migration history table",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			rc := rootFromCmd(c)
			client, cleanup, err := newConnectedClient(c.Context(), rc)
			if err != nil {
				rc.Printer.Errorf("connection failed: %v", err)
				return err
			}
			defer cleanup()
			if err := migration.CreateMigrationTable(c.Context(), client); err != nil {
				rc.Printer.Errorf("init failed: %v", err)
				return err
			}
			rc.Printer.Successf("migration tracking table ready (%s)", migration.MigrationTableName)
			return nil
		},
	}
}

// ---------------------------------------------------------------------------
// db ping
// ---------------------------------------------------------------------------

func newDBPingCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "ping",
		Short: "Verify database connectivity",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			rc := rootFromCmd(c)
			rc.Printer.Infof("connecting to %s", rc.Settings.Database.DBURL)
			client, cleanup, err := newConnectedClient(c.Context(), rc)
			if err != nil {
				rc.Printer.Errorf("connection failed: %v", err)
				return err
			}
			defer cleanup()
			ok, err := client.Health(c.Context())
			if err != nil || !ok {
				rc.Printer.Errorf("health check failed: %v", err)
				return err
			}
			rc.Printer.Successf("database is reachable (%s/%s)", rc.Settings.Database.DBNS, rc.Settings.Database.DB)
			return nil
		},
	}
}

// ---------------------------------------------------------------------------
// db info
// ---------------------------------------------------------------------------

func newDBInfoCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "info",
		Short: "Show resolved database configuration",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			rc := rootFromCmd(c)
			cfg := rc.Settings.Database
			info := map[string]any{
				"url":                 cfg.DBURL,
				"namespace":           cfg.DBNS,
				"database":            cfg.DB,
				"username":            optionalString(cfg.DBUser),
				"password":            maskOptionalString(cfg.DBPass),
				"timeout_seconds":     cfg.DBTimeout,
				"max_connections":     cfg.DBMaxConnections,
				"retry_max_attempts":  cfg.DBRetryMaxAttempts,
				"retry_min_wait":      cfg.DBRetryMinWait,
				"retry_max_wait":      cfg.DBRetryMaxWait,
				"retry_multiplier":    cfg.DBRetryMultiplier,
				"enable_live_queries": cfg.EnableLiveQueries,
			}
			if rc.Flags.JSONOut {
				return rc.Printer.JSON(info)
			}
			rc.Printer.Section("Database configuration")
			for _, k := range sortedStringKeys(info) {
				rc.Printer.Plainf("  %-22s %v", k, info[k])
			}
			return nil
		},
	}
}

// optionalString unwraps a *string to "(none)" when nil.
func optionalString(s *string) string {
	if s == nil {
		return "(none)"
	}
	return *s
}

// maskOptionalString returns a fixed masked sentinel for any set value so
// passwords never appear in info output.
func maskOptionalString(s *string) string {
	if s == nil || *s == "" {
		return "(none)"
	}
	return "***"
}

// ---------------------------------------------------------------------------
// db reset
// ---------------------------------------------------------------------------

func newDBResetCommand() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "reset",
		Short: "Remove every table in the configured database (destructive)",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			rc := rootFromCmd(c)
			if !yes {
				return newUsageError("refusing to reset without --yes; this drops every table in %s/%s",
					rc.Settings.Database.DBNS, rc.Settings.Database.DB)
			}
			client, cleanup, err := newConnectedClient(c.Context(), rc)
			if err != nil {
				rc.Printer.Errorf("connection failed: %v", err)
				return err
			}
			defer cleanup()
			tables, err := listDatabaseTables(c.Context(), rc)
			if err != nil {
				return err
			}
			if len(tables) == 0 {
				rc.Printer.Successf("database already empty")
				return nil
			}
			rc.Printer.Warnf("removing %d table(s) from %s/%s",
				len(tables), rc.Settings.Database.DBNS, rc.Settings.Database.DB)
			for _, t := range tables {
				if _, err := client.Query(c.Context(), fmt.Sprintf("REMOVE TABLE %s;", t)); err != nil {
					rc.Printer.Errorf("failed to remove %s: %v", t, err)
					return err
				}
				rc.Printer.Plainf("  - removed %s", t)
			}
			rc.Printer.Successf("removed %d table(s)", len(tables))
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation (required for destructive operation)")
	return cmd
}

// ---------------------------------------------------------------------------
// db query
// ---------------------------------------------------------------------------

func newDBQueryCommand() *cobra.Command {
	var fromFile string
	cmd := &cobra.Command{
		Use:   "query <surql>",
		Short: "Execute a raw SurrealQL query",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			rc := rootFromCmd(c)
			q, err := resolveQueryText(args, fromFile)
			if err != nil {
				return err
			}
			client, cleanup, err := newConnectedClient(c.Context(), rc)
			if err != nil {
				rc.Printer.Errorf("connection failed: %v", err)
				return err
			}
			defer cleanup()
			res, err := client.Query(c.Context(), q)
			if err != nil {
				rc.Printer.Errorf("query failed: %v", err)
				return err
			}
			if res == nil {
				rc.Printer.Successf("query executed (no results)")
				return nil
			}
			// Query results are semi-structured — JSON is always the right
			// default for machine/human consumption, so emit JSON even
			// without --json. This matches surql-py's default.
			return rc.Printer.JSON(res)
		},
	}
	cmd.Flags().StringVarP(&fromFile, "file", "f", "", "read SurrealQL from a file instead of <surql> argument")
	return cmd
}

// resolveQueryText returns the SurrealQL text from either the inline arg
// or a file. Exactly one must be supplied.
func resolveQueryText(args []string, file string) (string, error) {
	if file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			return "", fmt.Errorf("read %s: %w", file, err)
		}
		text := strings.TrimSpace(string(data))
		if text == "" {
			return "", newUsageError("query file %s is empty", file)
		}
		return text, nil
	}
	if len(args) == 0 {
		return "", newUsageError("a SurrealQL query or --file is required")
	}
	return strings.Join(args, " "), nil
}

// ---------------------------------------------------------------------------
// db version
// ---------------------------------------------------------------------------

func newDBVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the connected SurrealDB server version",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			rc := rootFromCmd(c)
			client, cleanup, err := newConnectedClient(c.Context(), rc)
			if err != nil {
				rc.Printer.Errorf("connection failed: %v", err)
				return err
			}
			defer cleanup()
			db := client.DB()
			if db == nil {
				return fmt.Errorf("underlying connection unavailable")
			}
			ver, err := db.Version(c.Context())
			if err != nil {
				rc.Printer.Errorf("version failed: %v", err)
				return err
			}
			rc.Printer.Successf("server: %s", ver)
			return nil
		},
	}
}

// listDatabaseTables extracts table names from INFO FOR DB. Helper shared
// by reset + schema tables.
func listDatabaseTables(ctx context.Context, rc *rootContext) ([]string, error) {
	client, cleanup, err := newConnectedClient(ctx, rc)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	res, err := client.Query(ctx, "INFO FOR DB;")
	if err != nil {
		return nil, err
	}
	return extractTableNames(res), nil
}

// extractTableNames walks the envelope returned by DatabaseClient.Query
// for INFO FOR DB and returns the "tables" keys in sorted order.
func extractTableNames(res any) []string {
	arr, ok := res.([]any)
	if !ok || len(arr) == 0 {
		return nil
	}
	env, ok := arr[0].(map[string]any)
	if !ok {
		return nil
	}
	result, ok := env["result"].(map[string]any)
	if !ok {
		return nil
	}
	// SurrealDB v3 uses "tables" (previously "tb"); accept both.
	var names []string
	for _, key := range []string{"tables", "tb"} {
		if section, ok := result[key].(map[string]any); ok {
			for name := range section {
				names = append(names, name)
			}
		}
	}
	return sortStringsStable(names)
}

// sortStringsStable is a tiny wrapper so callers do not need to import
// sort just to dedupe / sort the single-use slice.
func sortStringsStable(in []string) []string {
	out := append([]string(nil), in...)
	// insertion sort is fine here; table counts are small and the CLI
	// never runs this in a hot path.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}
