package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Oneiriq/surql-go/pkg/surql/migration"
	"github.com/Oneiriq/surql-go/pkg/surql/schema"
	"gopkg.in/yaml.v3"
)

// newSchemaCommand wires `surql schema`.
func newSchemaCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schema",
		Short: "Schema inspection and management commands",
	}
	cmd.AddCommand(
		newSchemaShowCommand(),
		newSchemaDiffCommand(),
		newSchemaGenerateCommand(),
		newSchemaSyncCommand(),
		newSchemaExportCommand(),
		newSchemaTablesCommand(),
		newSchemaInspectCommand(),
		newSchemaValidateCommand(),
		newSchemaCheckCommand(),
		newSchemaHookConfigCommand(),
		newSchemaWatchCommand(),
		newSchemaVisualizeCommand(),
	)
	return cmd
}

// ---------------------------------------------------------------------------
// schema show
// ---------------------------------------------------------------------------

func newSchemaShowCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show [table]",
		Short: "Show the current database schema (or one table)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			rc := rootFromCmd(c)
			client, cleanup, err := newConnectedClient(c.Context(), rc)
			if err != nil {
				rc.Printer.Errorf("connection failed: %v", err)
				return err
			}
			defer cleanup()
			var (
				res any
			)
			if len(args) == 1 {
				res, err = client.Query(c.Context(), fmt.Sprintf("INFO FOR TABLE %s;", args[0]))
			} else {
				res, err = client.Query(c.Context(), "INFO FOR DB;")
			}
			if err != nil {
				return err
			}
			return rc.Printer.JSON(res)
		},
	}
	return cmd
}

// ---------------------------------------------------------------------------
// schema diff
// ---------------------------------------------------------------------------

func newSchemaDiffCommand() *cobra.Command {
	var (
		from string
		to   string
		dir  string
	)
	cmd := &cobra.Command{
		Use:   "diff",
		Short: "Compare two stored schema snapshots",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			rc := rootFromCmd(c)
			if from == "" || to == "" {
				return newUsageError("--from and --to are required (both point to stored snapshot files or versions)")
			}
			snapDir := dir
			if snapDir == "" {
				snapDir = filepath.Join(migrationsDir(rc, ""), "snapshots")
			}
			src, err := loadSnapshotFromReference(snapDir, from)
			if err != nil {
				return fmt.Errorf("load --from snapshot: %w", err)
			}
			dst, err := loadSnapshotFromReference(snapDir, to)
			if err != nil {
				return fmt.Errorf("load --to snapshot: %w", err)
			}
			diffs, err := migration.CompareSnapshots(src, dst)
			if err != nil {
				return err
			}
			if rc.Flags.JSONOut {
				return rc.Printer.JSON(diffs)
			}
			if len(diffs) == 0 {
				rc.Printer.Successf("no differences")
				return nil
			}
			rc.Printer.Section(fmt.Sprintf("%d diff(s): %s -> %s", len(diffs), src.Version, dst.Version))
			for _, d := range diffs {
				rc.Printer.Plainf("  %-20s %-24s %s", d.Operation, d.Table, d.Description)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "source snapshot path or version")
	cmd.Flags().StringVar(&to, "to", "", "target snapshot path or version")
	cmd.Flags().StringVar(&dir, "snapshots-dir", "", "directory holding stored snapshots (default: <migrations>/snapshots)")
	return cmd
}

// loadSnapshotFromReference accepts either a direct .snapshot.json path or
// a bare version string (resolved under snapshotsDir).
func loadSnapshotFromReference(snapshotsDir, ref string) (migration.SchemaSnapshot, error) {
	if info, err := os.Stat(ref); err == nil && !info.IsDir() {
		return migration.LoadSnapshot(ref)
	}
	// Treat ref as a version: <snapshotsDir>/<version>.snapshot.json
	candidate := filepath.Join(snapshotsDir, ref+".snapshot.json")
	if _, err := os.Stat(candidate); err == nil {
		return migration.LoadSnapshot(candidate)
	}
	return migration.SchemaSnapshot{}, fmt.Errorf("snapshot %q not found (looked in %s)", ref, snapshotsDir)
}

// ---------------------------------------------------------------------------
// schema generate
// ---------------------------------------------------------------------------

func newSchemaGenerateCommand() *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Render DEFINE statements for every registered schema entity",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			rc := rootFromCmd(c)
			registry := schema.GetRegistry()
			if registry.TableCount() == 0 && registry.EdgeCount() == 0 {
				rc.Printer.Warnf("schema registry is empty (no definitions registered via code)")
				rc.Printer.Infof("register TableDefinition / EdgeDefinition instances via schema.GetRegistry().RegisterTable(...)")
				return nil
			}
			sql, err := schema.GenerateSchemaSQL(registry, false)
			if err != nil {
				return err
			}
			if output == "" {
				rc.Printer.Plainf("%s", sql)
				return nil
			}
			if err := os.WriteFile(output, []byte(sql), 0o644); err != nil {
				return fmt.Errorf("write %s: %w", output, err)
			}
			rc.Printer.Successf("wrote %d bytes to %s", len(sql), output)
			return nil
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "", "write generated SurrealQL to this path instead of stdout")
	return cmd
}

// ---------------------------------------------------------------------------
// schema sync
// ---------------------------------------------------------------------------

func newSchemaSyncCommand() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Apply the code registry schema directly to the database",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			rc := rootFromCmd(c)
			registry := schema.GetRegistry()
			if registry.TableCount() == 0 && registry.EdgeCount() == 0 {
				rc.Printer.Warnf("registry is empty; nothing to sync")
				return nil
			}
			sql, err := schema.GenerateSchemaSQL(registry, true)
			if err != nil {
				return err
			}
			if dryRun {
				rc.Printer.Infof("dry-run: would execute the following SurrealQL")
				rc.Printer.Plainf("%s", sql)
				return nil
			}
			rc.Printer.Warnf("schema sync is destructive — prefer `surql migrate generate` to produce a reviewable migration")
			client, cleanup, err := newConnectedClient(c.Context(), rc)
			if err != nil {
				rc.Printer.Errorf("connection failed: %v", err)
				return err
			}
			defer cleanup()
			if _, err := client.Query(c.Context(), sql); err != nil {
				return err
			}
			rc.Printer.Successf("schema synchronized")
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview generated SurrealQL without executing")
	return cmd
}

// ---------------------------------------------------------------------------
// schema export
// ---------------------------------------------------------------------------

func newSchemaExportCommand() *cobra.Command {
	var (
		format string
		output string
	)
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export the database schema to JSON or YAML",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			rc := rootFromCmd(c)
			client, cleanup, err := newConnectedClient(c.Context(), rc)
			if err != nil {
				rc.Printer.Errorf("connection failed: %v", err)
				return err
			}
			defer cleanup()
			res, err := client.Query(c.Context(), "INFO FOR DB;")
			if err != nil {
				return err
			}
			var encoded []byte
			switch strings.ToLower(format) {
			case "", "json":
				encoded, err = json.MarshalIndent(res, "", "  ")
				if err != nil {
					return err
				}
				encoded = append(encoded, '\n')
			case "yaml":
				encoded, err = yaml.Marshal(res)
				if err != nil {
					return err
				}
			default:
				return newUsageError("--format must be one of: json, yaml")
			}
			if output == "" {
				_, err := rc.Printer.Out().Write(encoded)
				return err
			}
			if err := os.WriteFile(output, encoded, 0o644); err != nil {
				return fmt.Errorf("write %s: %w", output, err)
			}
			rc.Printer.Successf("wrote %d bytes to %s", len(encoded), output)
			return nil
		},
	}
	cmd.Flags().StringVarP(&format, "format", "f", "json", "output format: json | yaml")
	cmd.Flags().StringVarP(&output, "output", "o", "", "write to this path instead of stdout")
	return cmd
}

// ---------------------------------------------------------------------------
// schema tables
// ---------------------------------------------------------------------------

func newSchemaTablesCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tables",
		Short: "List tables in the configured database",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			rc := rootFromCmd(c)
			names, err := listDatabaseTables(c.Context(), rc)
			if err != nil {
				rc.Printer.Errorf("tables failed: %v", err)
				return err
			}
			if rc.Flags.JSONOut {
				return rc.Printer.JSON(names)
			}
			if len(names) == 0 {
				rc.Printer.Infof("no tables in %s/%s", rc.Settings.Database.DBNS, rc.Settings.Database.DB)
				return nil
			}
			rows := make([][]string, 0, len(names))
			for _, n := range names {
				rows = append(rows, []string{n})
			}
			rc.Printer.Table([]string{"Table"}, rows)
			return nil
		},
	}
	return cmd
}

// ---------------------------------------------------------------------------
// schema inspect
// ---------------------------------------------------------------------------

func newSchemaInspectCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inspect <table>",
		Short: "Inspect fields / indexes / events for a table",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			rc := rootFromCmd(c)
			client, cleanup, err := newConnectedClient(c.Context(), rc)
			if err != nil {
				rc.Printer.Errorf("connection failed: %v", err)
				return err
			}
			defer cleanup()
			res, err := client.Query(c.Context(), fmt.Sprintf("INFO FOR TABLE %s;", args[0]))
			if err != nil {
				return err
			}
			return rc.Printer.JSON(res)
		},
	}
	return cmd
}

// ---------------------------------------------------------------------------
// schema validate
// ---------------------------------------------------------------------------

func newSchemaValidateCommand() *cobra.Command {
	var strict bool
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate the code-side schema registry",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			rc := rootFromCmd(c)
			registry := schema.GetRegistry()
			if registry.TableCount() == 0 && registry.EdgeCount() == 0 {
				rc.Printer.Warnf("registry is empty; nothing to validate")
				return nil
			}
			report, err := schema.ValidateSchema(registry)
			if err != nil {
				return err
			}
			if rc.Flags.JSONOut {
				return rc.Printer.JSON(report)
			}
			for _, e := range report.Errors() {
				rc.Printer.Errorf("%s", e.String())
			}
			for _, w := range report.Warnings() {
				rc.Printer.Warnf("%s", w.String())
			}
			for _, i := range report.Infos() {
				rc.Printer.Infof("%s", i.String())
			}
			if report.HasErrors() || (strict && len(report.Warnings()) > 0) {
				return fmt.Errorf("schema validation failed (%d error(s), %d warning(s))",
					len(report.Errors()), len(report.Warnings()))
			}
			rc.Printer.Successf("schema is valid")
			return nil
		},
	}
	cmd.Flags().BoolVar(&strict, "strict", false, "treat warnings as failures")
	return cmd
}

// ---------------------------------------------------------------------------
// schema check
// ---------------------------------------------------------------------------

func newSchemaCheckCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Compare the code schema registry against the database",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			rc := rootFromCmd(c)
			registry := schema.GetRegistry()
			if registry.TableCount() == 0 && registry.EdgeCount() == 0 {
				rc.Printer.Warnf("registry is empty; nothing to check")
				return nil
			}
			client, cleanup, err := newConnectedClient(c.Context(), rc)
			if err != nil {
				rc.Printer.Errorf("connection failed: %v", err)
				return err
			}
			defer cleanup()
			codeSnap := migration.SchemaSnapshot{
				Tables: registry.Tables(),
				Edges:  registry.Edges(),
			}
			dbSnap, err := liveSnapshot(c.Context(), client)
			if err != nil {
				return err
			}
			diffs, err := migration.DiffSchemas(codeSnap, dbSnap)
			if err != nil {
				return err
			}
			if len(diffs) == 0 {
				rc.Printer.Successf("no drift detected")
				return nil
			}
			if rc.Flags.JSONOut {
				return rc.Printer.JSON(diffs)
			}
			rc.Printer.Errorf("drift detected (%d diff(s))", len(diffs))
			for _, d := range diffs {
				rc.Printer.Plainf("  %-20s %-24s %s", d.Operation, d.Table, d.Description)
			}
			return fmt.Errorf("schema drift: %d difference(s)", len(diffs))
		},
	}
	return cmd
}

// liveSnapshot materialises a SchemaSnapshot from the connected database by
// re-parsing INFO FOR DB / per-table INFO responses. Falls back to an empty
// snapshot when no tables are present.
func liveSnapshot(ctx context.Context, client interface {
	Query(context.Context, string) (any, error)
}) (migration.SchemaSnapshot, error) {
	res, err := client.Query(ctx, "INFO FOR DB;")
	if err != nil {
		return migration.SchemaSnapshot{}, err
	}
	payload := unwrapQueryResult(res)
	info, _ := payload.(map[string]any)
	if info == nil {
		return migration.SchemaSnapshot{}, nil
	}
	dbInfo, err := schema.ParseDBInfo(info)
	if err != nil {
		return migration.SchemaSnapshot{}, err
	}

	tableNames := extractTableNamesFromInfo(info)
	tables := make([]schema.TableDefinition, 0, len(tableNames))
	edges := make([]schema.EdgeDefinition, 0)
	for _, name := range tableNames {
		tblRes, err := client.Query(ctx, fmt.Sprintf("INFO FOR TABLE %s;", name))
		if err != nil {
			return migration.SchemaSnapshot{}, err
		}
		tblPayload := unwrapQueryResult(tblRes)
		tblInfo, _ := tblPayload.(map[string]any)
		if tblInfo == nil {
			continue
		}
		tbl, err := schema.ParseTableInfo(name, tblInfo)
		if err != nil {
			continue
		}
		tables = append(tables, tbl)
		// edges are returned via ParseEdgeInfo when the table is an edge
		// (IN / OUT fields). Try parsing as edge too; ignore errors.
		if edge, err := schema.ParseEdgeInfo(name, tblInfo); err == nil && edge.Name != "" {
			edges = append(edges, edge)
		}
	}
	_ = dbInfo
	return migration.SchemaSnapshot{Tables: tables, Edges: edges}, nil
}

// unwrapQueryResult pops the first envelope from DatabaseClient.Query and
// returns the `result` field.
func unwrapQueryResult(res any) any {
	arr, ok := res.([]any)
	if !ok || len(arr) == 0 {
		return nil
	}
	env, ok := arr[0].(map[string]any)
	if !ok {
		return nil
	}
	return env["result"]
}

// extractTableNamesFromInfo reads the "tables" / "tb" map off the INFO FOR DB
// payload and returns sorted keys.
func extractTableNamesFromInfo(info map[string]any) []string {
	var names []string
	for _, key := range []string{"tables", "tb"} {
		if m, ok := info[key].(map[string]any); ok {
			for n := range m {
				names = append(names, n)
			}
		}
	}
	return sortStringsStable(names)
}

// ---------------------------------------------------------------------------
// schema hook-config
// ---------------------------------------------------------------------------

func newSchemaHookConfigCommand() *cobra.Command {
	var schemaPath string
	cmd := &cobra.Command{
		Use:   "hook-config",
		Short: "Emit a pre-commit hook YAML snippet for schema drift detection",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			rc := rootFromCmd(c)
			yamlSnippet := fmt.Sprintf(`- repo: local
  hooks:
    - id: surql-schema-check
      name: surql schema check
      entry: surql schema check
      language: system
      files: ^%s.*\.go$
      pass_filenames: false
`, escapeYAMLRegex(schemaPath))
			rc.Printer.Plainf("%s", yamlSnippet)
			return nil
		},
	}
	cmd.Flags().StringVarP(&schemaPath, "schema", "s", "schemas/", "path prefix where schema definitions live")
	return cmd
}

// escapeYAMLRegex escapes dots in a path prefix so the emitted regex
// matches the literal directory.
func escapeYAMLRegex(path string) string {
	return strings.ReplaceAll(strings.ReplaceAll(path, `\`, `\\`), `.`, `\.`)
}

// ---------------------------------------------------------------------------
// schema watch
// ---------------------------------------------------------------------------

func newSchemaWatchCommand() *cobra.Command {
	var (
		dir      string
		debounce time.Duration
	)
	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Watch a directory for schema changes",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			rc := rootFromCmd(c)
			watchDir := dir
			if watchDir == "" {
				watchDir = migrationsDir(rc, "")
			}
			// The DriftChecker is where watch consumers would call
			// CheckSchemaDriftFromSnapshots against the current code
			// registry; for CLI use we log each batch and emit a synthetic
			// report so callers see every change.
			checker := func(_ context.Context, events []migration.SchemaChangeEvent) (*migration.DriftReport, error) {
				for _, ev := range events {
					rc.Printer.Infof("%s %s", ev.ChangeType, ev.Path)
				}
				return &migration.DriftReport{}, nil
			}
			watcher, err := migration.NewSchemaWatcher(watchDir, checker, migration.WatcherOptions{
				Debounce: debounce,
				OnError: func(err error) {
					rc.Printer.Warnf("watcher error: %v", err)
				},
			})
			if err != nil {
				return err
			}
			ctx := c.Context()
			if err := watcher.Start(ctx); err != nil {
				return err
			}
			rc.Printer.Infof("watching %s (Ctrl+C to stop)", watchDir)
			for {
				select {
				case <-ctx.Done():
					_ = watcher.Stop()
					rc.Printer.Successf("watcher stopped")
					return nil
				case _, ok := <-watcher.Reports():
					if !ok {
						return nil
					}
					// Events are logged inside the checker above; the
					// report channel is only used for the closing sentinel.
				}
			}
		},
	}
	cmd.Flags().StringVarP(&dir, "directory", "d", "", "directory to watch")
	cmd.Flags().DurationVar(&debounce, "debounce", 500*time.Millisecond, "debounce interval between change batches")
	return cmd
}

// ---------------------------------------------------------------------------
// schema visualize
// ---------------------------------------------------------------------------

func newSchemaVisualizeCommand() *cobra.Command {
	var (
		themeName string
		format    string
		output    string
	)
	cmd := &cobra.Command{
		Use:   "visualize",
		Short: "Render the code schema registry as a diagram",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			rc := rootFromCmd(c)
			registry := schema.GetRegistry()
			if registry.TableCount() == 0 && registry.EdgeCount() == 0 {
				rc.Printer.Warnf("registry is empty; nothing to visualize")
				return nil
			}
			theme, err := schema.GetTheme(themeName)
			if err != nil {
				return newUsageError("%v", err)
			}
			var out string
			switch strings.ToLower(format) {
			case "mermaid", "":
				out = schema.GenerateMermaid(registry, theme.Mermaid)
			case "graphviz", "dot":
				out = schema.GenerateGraphViz(registry, theme.GraphViz)
			case "ascii":
				out = schema.GenerateASCII(registry, theme.ASCII)
			default:
				return newUsageError("--format must be one of: mermaid, graphviz, ascii")
			}
			if output == "" {
				rc.Printer.Plainf("%s", out)
				return nil
			}
			if err := os.WriteFile(output, []byte(out), 0o644); err != nil {
				return fmt.Errorf("write %s: %w", output, err)
			}
			rc.Printer.Successf("wrote %d bytes to %s", len(out), output)
			return nil
		},
	}
	cmd.Flags().StringVar(&themeName, "theme", "modern", "theme: modern | dark | forest | minimal")
	cmd.Flags().StringVarP(&format, "format", "f", "mermaid", "format: mermaid | graphviz | ascii")
	cmd.Flags().StringVarP(&output, "output", "o", "", "write diagram to this path instead of stdout")
	return cmd
}
