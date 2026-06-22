package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/Oneiriq/surql-go/pkg/surql/schema"
)

// newBucketCommand wires `surql bucket`. It mirrors `surql schema` for the
// definition lifecycle (define / list / rm) and `surql db` for the runtime
// file operations (put / get / delete / exists / list).
func newBucketCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bucket",
		Short: "Object-storage bucket and file commands (SurrealDB v3)",
	}
	cmd.AddCommand(
		newBucketDefineCommand(),
		newBucketListCommand(),
		newBucketRemoveCommand(),
		newBucketPutCommand(),
		newBucketGetCommand(),
		newBucketDeleteCommand(),
		newBucketExistsCommand(),
	)
	return cmd
}

// ---------------------------------------------------------------------------
// bucket define
// ---------------------------------------------------------------------------

func newBucketDefineCommand() *cobra.Command {
	var (
		backend     string
		readOnly    bool
		comment     string
		ifNotExists bool
		dryRun      bool
	)
	cmd := &cobra.Command{
		Use:   "define <name>",
		Short: "Define a storage bucket (DEFINE BUCKET)",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			rc := rootFromCmd(c)
			bucket := schema.NewBucket(args[0], backend,
				schema.WithBucketReadOnly(readOnly),
				schema.WithBucketComment(comment),
			)
			if err := bucket.Validate(); err != nil {
				return newUsageError("%v", err)
			}
			stmt := bucket.ToSurql()
			if ifNotExists {
				stmt = bucket.ToSurqlIfNotExists()
			}
			if dryRun {
				rc.Printer.Infof("dry-run: would execute")
				rc.Printer.Plainf("%s", stmt)
				return nil
			}
			client, cleanup, err := newConnectedClient(c.Context(), rc)
			if err != nil {
				rc.Printer.Errorf("connection failed: %v", err)
				return err
			}
			defer cleanup()
			if _, err := client.Query(c.Context(), stmt); err != nil {
				rc.Printer.Errorf("define bucket failed: %v", err)
				return err
			}
			rc.Printer.Successf("bucket %s defined (backend %q)", bucket.Name, bucket.Backend)
			return nil
		},
	}
	cmd.Flags().StringVar(&backend, "backend", "memory", `storage backend: "memory", "file:/path", or "s3://bucket/prefix"`)
	cmd.Flags().BoolVar(&readOnly, "readonly", false, "mark the bucket READONLY")
	cmd.Flags().StringVar(&comment, "comment", "", "attach a COMMENT to the bucket")
	cmd.Flags().BoolVar(&ifNotExists, "if-not-exists", false, "emit DEFINE BUCKET IF NOT EXISTS")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print the SurrealQL without executing")
	return cmd
}

// ---------------------------------------------------------------------------
// bucket list (buckets, or files within a bucket when a name is given)
// ---------------------------------------------------------------------------

func newBucketListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list [bucket]",
		Short: "List buckets, or files within a bucket when a name is given",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			rc := rootFromCmd(c)
			client, cleanup, err := newConnectedClient(c.Context(), rc)
			if err != nil {
				rc.Printer.Errorf("connection failed: %v", err)
				return err
			}
			defer cleanup()

			// With a bucket name: list the files inside it.
			if len(args) == 1 {
				refs, err := client.Bucket(args[0]).List(c.Context())
				if err != nil {
					rc.Printer.Errorf("list files failed: %v", err)
					return err
				}
				if rc.Flags.JSONOut {
					return rc.Printer.JSON(refs)
				}
				if len(refs) == 0 {
					rc.Printer.Infof("bucket %s is empty", args[0])
					return nil
				}
				rows := make([][]string, 0, len(refs))
				for _, ref := range refs {
					rows = append(rows, []string{ref.Key, ref.String()})
				}
				rc.Printer.Table([]string{"Key", "File"}, rows)
				return nil
			}

			// No argument: list bucket definitions from INFO FOR DB.
			names, err := listBuckets(c.Context(), client)
			if err != nil {
				rc.Printer.Errorf("list buckets failed: %v", err)
				return err
			}
			if rc.Flags.JSONOut {
				return rc.Printer.JSON(names)
			}
			if len(names) == 0 {
				rc.Printer.Infof("no buckets in %s/%s", rc.Settings.Database.DBNS, rc.Settings.Database.DB)
				return nil
			}
			rows := make([][]string, 0, len(names))
			for _, n := range names {
				rows = append(rows, []string{n})
			}
			rc.Printer.Table([]string{"Bucket"}, rows)
			return nil
		},
	}
	return cmd
}

// listBuckets reads bucket names from INFO FOR DB via the schema parser.
func listBuckets(ctx context.Context, client interface {
	Query(context.Context, string) (any, error)
}) ([]string, error) {
	res, err := client.Query(ctx, "INFO FOR DB;")
	if err != nil {
		return nil, err
	}
	info, _ := unwrapQueryResult(res).(map[string]any)
	if info == nil {
		return nil, nil
	}
	dbInfo, err := schema.ParseDBInfo(info)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(dbInfo.Buckets))
	for name := range dbInfo.Buckets {
		names = append(names, name)
	}
	return sortStringsStable(names), nil
}

// ---------------------------------------------------------------------------
// bucket rm
// ---------------------------------------------------------------------------

func newBucketRemoveCommand() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "rm <name>",
		Short: "Remove a storage bucket (REMOVE BUCKET, destructive)",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			rc := rootFromCmd(c)
			if !yes {
				return newUsageError("refusing to remove bucket %q without --yes; this deletes the bucket and its files", args[0])
			}
			client, cleanup, err := newConnectedClient(c.Context(), rc)
			if err != nil {
				rc.Printer.Errorf("connection failed: %v", err)
				return err
			}
			defer cleanup()
			if _, err := client.Query(c.Context(), schema.RemoveBucketSurql(args[0])); err != nil {
				rc.Printer.Errorf("remove bucket failed: %v", err)
				return err
			}
			rc.Printer.Successf("bucket %s removed", args[0])
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation (required for destructive operation)")
	return cmd
}

// ---------------------------------------------------------------------------
// bucket put
// ---------------------------------------------------------------------------

func newBucketPutCommand() *cobra.Command {
	var (
		data        string
		fromFile    string
		ifNotExists bool
	)
	cmd := &cobra.Command{
		Use:   "put <bucket> <key>",
		Short: "Upload a file to a bucket",
		Args:  cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			rc := rootFromCmd(c)
			payload, err := resolveBucketPayload(data, fromFile)
			if err != nil {
				return err
			}
			client, cleanup, err := newConnectedClient(c.Context(), rc)
			if err != nil {
				rc.Printer.Errorf("connection failed: %v", err)
				return err
			}
			defer cleanup()
			handle := client.Bucket(args[0])
			if ifNotExists {
				err = handle.PutIfNotExists(c.Context(), args[1], payload)
			} else {
				err = handle.Put(c.Context(), args[1], payload)
			}
			if err != nil {
				rc.Printer.Errorf("put failed: %v", err)
				return err
			}
			rc.Printer.Successf("wrote %d byte(s) to %s:/%s", len(payload), args[0], args[1])
			return nil
		},
	}
	cmd.Flags().StringVarP(&data, "data", "d", "", "inline file content")
	cmd.Flags().StringVarP(&fromFile, "file", "f", "", "read file content from this path")
	cmd.Flags().BoolVar(&ifNotExists, "if-not-exists", false, "only write when the key does not already exist")
	return cmd
}

// resolveBucketPayload returns the bytes to upload from either inline --data
// or a --file path. Exactly one must be supplied.
func resolveBucketPayload(data, file string) ([]byte, error) {
	switch {
	case data != "" && file != "":
		return nil, newUsageError("specify only one of --data or --file")
	case file != "":
		b, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", file, err)
		}
		return b, nil
	case data != "":
		return []byte(data), nil
	default:
		return nil, newUsageError("a payload is required: pass --data or --file")
	}
}

// ---------------------------------------------------------------------------
// bucket get
// ---------------------------------------------------------------------------

func newBucketGetCommand() *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "get <bucket> <key>",
		Short: "Download a file from a bucket",
		Args:  cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			rc := rootFromCmd(c)
			client, cleanup, err := newConnectedClient(c.Context(), rc)
			if err != nil {
				rc.Printer.Errorf("connection failed: %v", err)
				return err
			}
			defer cleanup()
			content, err := client.Bucket(args[0]).Get(c.Context(), args[1])
			if err != nil {
				rc.Printer.Errorf("get failed: %v", err)
				return err
			}
			if output != "" {
				// Downloaded files are intentionally user-readable; 0644
				// matches the other CLI export commands.
				if err := os.WriteFile(output, content, 0o644); err != nil { //nolint:gosec // G306: downloaded file is intentionally world-readable
					return fmt.Errorf("write %s: %w", output, err)
				}
				rc.Printer.Successf("wrote %d byte(s) to %s", len(content), output)
				return nil
			}
			_, err = rc.Printer.Out().Write(content)
			return err
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "", "write the file content to this path instead of stdout")
	return cmd
}

// ---------------------------------------------------------------------------
// bucket delete
// ---------------------------------------------------------------------------

func newBucketDeleteCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <bucket> <key>",
		Short: "Delete a file from a bucket",
		Args:  cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			rc := rootFromCmd(c)
			client, cleanup, err := newConnectedClient(c.Context(), rc)
			if err != nil {
				rc.Printer.Errorf("connection failed: %v", err)
				return err
			}
			defer cleanup()
			if err := client.Bucket(args[0]).Delete(c.Context(), args[1]); err != nil {
				rc.Printer.Errorf("delete failed: %v", err)
				return err
			}
			rc.Printer.Successf("deleted %s:/%s", args[0], args[1])
			return nil
		},
	}
	return cmd
}

// ---------------------------------------------------------------------------
// bucket exists
// ---------------------------------------------------------------------------

func newBucketExistsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "exists <bucket> <key>",
		Short: "Report whether a file exists in a bucket",
		Args:  cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			rc := rootFromCmd(c)
			client, cleanup, err := newConnectedClient(c.Context(), rc)
			if err != nil {
				rc.Printer.Errorf("connection failed: %v", err)
				return err
			}
			defer cleanup()
			exists, err := client.Bucket(args[0]).Exists(c.Context(), args[1])
			if err != nil {
				rc.Printer.Errorf("exists failed: %v", err)
				return err
			}
			if rc.Flags.JSONOut {
				return rc.Printer.JSON(map[string]any{"exists": exists})
			}
			if exists {
				rc.Printer.Successf("%s:/%s exists", args[0], args[1])
			} else {
				rc.Printer.Infof("%s:/%s does not exist", args[0], args[1])
			}
			return nil
		},
	}
	return cmd
}
