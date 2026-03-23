package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/walheimlab/walheim-go/internal/config"
	"github.com/walheimlab/walheim-go/internal/exitcode"
	"github.com/walheimlab/walheim-go/internal/fs"
	"github.com/walheimlab/walheim-go/internal/registry"
	"github.com/walheimlab/walheim-go/internal/resource"
)

// newContextCmd builds the `whctl context` subcommand tree.
// Context commands are NOT in the registry — they operate on the config file itself.
func newContextCmd(gf *GlobalFlags) *cobra.Command {
	ctx := &cobra.Command{
		Use:   "context",
		Short: "Manage whctl contexts",
		Long: `Manage whctl contexts (config entries that point to a homelab data directory).

Each context has a name and a data directory. One context is active at a time.
Use "whctl context new" to add a context or "whctl context use" to switch.`,
		SilenceUsage: true,
	}

	ctx.AddCommand(newContextListCmd(gf))
	ctx.AddCommand(newContextUseCmd(gf))
	ctx.AddCommand(newContextNewCmd(gf))
	ctx.AddCommand(newContextDeleteCmd(gf))
	ctx.AddCommand(newContextCurrentCmd(gf))
	ctx.AddCommand(newContextExportCmd(gf))

	return ctx
}

// newContextListCmd implements `whctl context list`.
func newContextListCmd(gf *GlobalFlags) *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Short:   "List all contexts",
		Aliases: []string{"ls"},
		Example: "  whctl context list\n  whctl context list -o json",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfigForContext(gf.Whconfig)
			if err != nil {
				return err
			}

			views := cfg.ListContexts()
			jsonMode := strings.EqualFold(gf.Output, "json")

			if jsonMode {
				type contextJSON struct {
					Name     string `json:"name"`
					Location string `json:"location"`
					Active   bool   `json:"active"`
				}

				result := make([]contextJSON, len(views))
				for i, v := range views {
					result[i] = contextJSON{Name: v.Name, Location: v.Location, Active: v.Active}
				}

				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")

				return enc.Encode(result)
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
			_, _ = fmt.Fprintln(w, "ACTIVE\tNAME\tLOCATION")

			for _, v := range views {
				active := " "
				if v.Active {
					active = "*"
				}

				_, _ = fmt.Fprintf(w, "%s\t%s\t%s\n", active, v.Name, v.Location)
			}

			return w.Flush()
		},
	}
}

// newContextUseCmd implements `whctl context use <name>`.
func newContextUseCmd(gf *GlobalFlags) *cobra.Command {
	return &cobra.Command{
		Use:     "use <name>",
		Short:   "Switch to a different context",
		Example: "  whctl context use production",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfigForContext(gf.Whconfig)
			if err != nil {
				return err
			}

			if err := cfg.UseContext(args[0]); err != nil {
				return exitErr(exitcode.NotFound, err)
			}

			if err := cfg.Save(); err != nil {
				return exitErr(exitcode.Failure, err)
			}

			fmt.Printf("Switched to context %q\n", args[0])

			return nil
		},
	}
}

// newContextNewCmd implements `whctl context new <name> --data-dir <path>` (local)
// or `whctl context new <name> --backend s3 --s3-bucket <bucket> ...` (S3).
func newContextNewCmd(gf *GlobalFlags) *cobra.Command {
	var (
		dataDirFlag    string
		backendFlag    string
		s3EndpointFlag string
		s3RegionFlag   string
		s3BucketFlag   string
		s3PrefixFlag   string
		s3AccessKeyID  string
		s3SecretKey    string
	)

	cmd := &cobra.Command{
		Use:   "new <name>",
		Short: "Add a new context and activate it",
		Long: `Add a new context pointing to a homelab data store.

For a local directory:
  whctl context new homelab --data-dir ~/homelab

For an S3-compatible store (Cloudflare R2, DigitalOcean Spaces, etc.):
  whctl context new r2-prod --backend s3 \
    --s3-endpoint https://<id>.r2.cloudflarestorage.com \
    --s3-region auto --s3-bucket my-bucket \
    [--s3-prefix walheim] \
    [--s3-access-key-id <key>] [--s3-secret-access-key <secret>]

Credentials for S3 may be omitted to use AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY env vars.`,
		Example: "  whctl context new homelab --data-dir ~/homelab\n  whctl context new r2-prod --backend s3 --s3-region auto --s3-bucket my-bucket",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			if backendFlag == "s3" {
				return runContextNewS3(gf, name, s3EndpointFlag, s3RegionFlag, s3BucketFlag,
					s3PrefixFlag, s3AccessKeyID, s3SecretKey)
			}

			// Local backend (default)
			if dataDirFlag == "" {
				return exitErr(exitcode.UsageError, fmt.Errorf("--data-dir is required (or use --backend s3)"))
			}

			expandedDir := expandHome(dataDirFlag)

			info, err := os.Stat(expandedDir)
			if err != nil {
				if os.IsNotExist(err) {
					return exitErr(exitcode.UsageError,
						fmt.Errorf("data directory %q does not exist (create it first)", expandedDir))
				}

				return exitErr(exitcode.Failure, fmt.Errorf("failed to stat data directory: %w", err))
			}

			if !info.IsDir() {
				return exitErr(exitcode.UsageError,
					fmt.Errorf("%q is not a directory", expandedDir))
			}

			nsDir := filepath.Join(expandedDir, "namespaces")
			if _, err := os.Stat(nsDir); os.IsNotExist(err) {
				fmt.Printf("Notice: creating %s\n", nsDir)

				if err := os.MkdirAll(nsDir, 0755); err != nil {
					return exitErr(exitcode.Failure,
						fmt.Errorf("failed to create namespaces directory: %w", err))
				}
			}

			cfg, err := loadOrInitConfig(gf.Whconfig)
			if err != nil {
				return err
			}

			if err := cfg.AddContext(name, expandedDir, true); err != nil {
				return exitErr(exitcode.Conflict, err)
			}

			if err := cfg.Save(); err != nil {
				return exitErr(exitcode.Failure, err)
			}

			fmt.Printf("Added context %q (data dir: %s) and set as active\n", name, expandedDir)

			return nil
		},
	}

	cmd.Flags().StringVar(&dataDirFlag, "data-dir", "", "Path to homelab data directory (local backend, must exist)")
	cmd.Flags().StringVar(&backendFlag, "backend", "", "Storage backend: local (default) or s3")
	cmd.Flags().StringVar(&s3EndpointFlag, "s3-endpoint", "", "S3-compatible endpoint URL (e.g. https://abc.r2.cloudflarestorage.com)")
	cmd.Flags().StringVar(&s3RegionFlag, "s3-region", "", "S3 region (e.g. auto, us-east-1)")
	cmd.Flags().StringVar(&s3BucketFlag, "s3-bucket", "", "S3 bucket name")
	cmd.Flags().StringVar(&s3PrefixFlag, "s3-prefix", "", "Optional S3 key prefix within the bucket")
	cmd.Flags().StringVar(&s3AccessKeyID, "s3-access-key-id", "", "S3 access key ID (omit to use AWS_ACCESS_KEY_ID env var)")
	cmd.Flags().StringVar(&s3SecretKey, "s3-secret-access-key", "", "S3 secret access key (omit to use AWS_SECRET_ACCESS_KEY env var)")

	return cmd
}

// runContextNewS3 handles `whctl context new --backend s3`.
func runContextNewS3(gf *GlobalFlags, name, endpoint, region, bucket, prefix, accessKeyID, secretKey string) error {
	if bucket == "" {
		return exitErr(exitcode.UsageError, fmt.Errorf("--s3-bucket is required for S3 backend"))
	}

	if region == "" {
		return exitErr(exitcode.UsageError, fmt.Errorf("--s3-region is required for S3 backend"))
	}

	cfg, err := loadOrInitConfig(gf.Whconfig)
	if err != nil {
		return err
	}

	s3cfg := config.S3Config{
		Endpoint:        endpoint,
		Region:          region,
		Bucket:          bucket,
		Prefix:          prefix,
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secretKey,
	}

	// Verify bucket is accessible before saving the context
	s3fs, err := fs.NewS3FS(fs.S3FSConfig{
		Endpoint:        endpoint,
		Region:          region,
		Bucket:          bucket,
		Prefix:          prefix,
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secretKey,
	})
	if err != nil {
		return exitErr(exitcode.Failure, fmt.Errorf("failed to initialise S3 client: %w", err))
	}

	fmt.Printf("Checking bucket %q...\n", bucket)

	if err := s3fs.Ping(); err != nil {
		return exitErr(exitcode.Failure, err)
	}

	if err := cfg.AddS3Context(name, s3cfg, true); err != nil {
		return exitErr(exitcode.Conflict, err)
	}

	if err := cfg.Save(); err != nil {
		return exitErr(exitcode.Failure, err)
	}

	loc := "s3://" + bucket
	if prefix != "" {
		loc += "/" + prefix
	}

	fmt.Printf("Added context %q (location: %s) and set as active\n", name, loc)

	return nil
}

// newContextExportCmd implements `whctl context export`.
func newContextExportCmd(gf *GlobalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "export",
		Short: "Export all manifests from the active context as multi-document YAML",
		Long: `Export every resource across all namespaces from the active (or --context) context.

Output is multi-document YAML (documents separated by ---) written to stdout.
Only stored manifests are included; generated files (e.g. docker-compose.yml) are excluded.`,
		Example: "  whctl context export\n  whctl context export > backup.yaml\n  whctl context export --context prod > prod.yaml",
		RunE: func(cmd *cobra.Command, args []string) error {
			filesystem, dataDir, err := resolveBackend(gf.Context, gf.Whconfig)
			if err != nil {
				return exitErr(exitcode.Failure, err)
			}

			// yaml.v3 Marshal prepends "---\n" to every document it writes.
			// Strip it from stored bytes so we control the separator ourselves,
			// ensuring exactly one "---\n" before each document.
			emit := func(data []byte) error {
				data = bytes.TrimPrefix(data, []byte("---\n"))

				if _, err := fmt.Fprintf(os.Stdout, "---\n%s", data); err != nil {
					return err
				}

				return nil
			}

			for _, entry := range registry.AllEntries() {
				handler := entry.Registration.Factory(dataDir, filesystem)

				if entry.IsCluster() {
					cl, ok := handler.(resource.ClusterLister)
					if !ok {
						continue
					}

					names, err := cl.ListNames()
					if err != nil {
						return exitErr(exitcode.Failure, fmt.Errorf("list %s: %w", entry.Registration.Info.Plural, err))
					}

					for _, name := range names {
						data, err := cl.ReadBytes(name)
						if err != nil {
							return exitErr(exitcode.Failure, err)
						}

						if err := emit(data); err != nil {
							return exitErr(exitcode.Failure, err)
						}
					}
				} else {
					nsl, ok := handler.(resource.NSLister)
					if !ok {
						continue
					}

					namespaces, err := nsl.ValidNamespaces()
					if err != nil {
						return exitErr(exitcode.Failure, fmt.Errorf("list namespaces: %w", err))
					}

					for _, ns := range namespaces {
						names, err := nsl.ListNames(ns)
						if err != nil {
							return exitErr(exitcode.Failure, fmt.Errorf("list %s in %s: %w", entry.Registration.Info.Plural, ns, err))
						}

						for _, name := range names {
							data, err := nsl.ReadBytes(ns, name)
							if err != nil {
								return exitErr(exitcode.Failure, err)
							}

							if err := emit(data); err != nil {
								return exitErr(exitcode.Failure, err)
							}
						}
					}
				}
			}

			return nil
		},
	}
}

// loadOrInitConfig tries to load the existing config; if missing, initialises a fresh one.
func loadOrInitConfig(whconfigFlag string) (*config.Config, error) {
	if existing, loadErr := config.Load(whconfigFlag); loadErr == nil {
		return existing, nil
	}

	fresh, initErr := config.Init(whconfigFlag)
	if initErr != nil {
		return nil, exitErr(exitcode.Failure, fmt.Errorf("failed to initialise config: %w", initErr))
	}

	return fresh, nil
}

// newContextDeleteCmd implements `whctl context delete <name>`.
func newContextDeleteCmd(gf *GlobalFlags) *cobra.Command {
	return &cobra.Command{
		Use:     "delete <name>",
		Short:   "Remove a context",
		Aliases: []string{"rm"},
		Example: "  whctl context delete old-homelab",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfigForContext(gf.Whconfig)
			if err != nil {
				return err
			}

			if err := cfg.DeleteContext(args[0]); err != nil {
				return exitErr(exitcode.NotFound, err)
			}

			if err := cfg.Save(); err != nil {
				return exitErr(exitcode.Failure, err)
			}

			fmt.Printf("Deleted context %q\n", args[0])

			return nil
		},
	}
}

// newContextCurrentCmd implements `whctl context current`.
func newContextCurrentCmd(gf *GlobalFlags) *cobra.Command {
	return &cobra.Command{
		Use:     "current",
		Short:   "Show the active context",
		Example: "  whctl context current\n  whctl context current -o json",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfigForContext(gf.Whconfig)
			if err != nil {
				return err
			}

			views := cfg.ListContexts()
			jsonMode := strings.EqualFold(gf.Output, "json")

			for _, v := range views {
				if v.Active {
					if jsonMode {
						type currentJSON struct {
							Name     string `json:"name"`
							Location string `json:"location"`
						}

						enc := json.NewEncoder(os.Stdout)
						enc.SetIndent("", "  ")

						return enc.Encode(currentJSON{Name: v.Name, Location: v.Location})
					}

					fmt.Printf("%s\t%s\n", v.Name, v.Location)

					return nil
				}
			}

			return exitErr(exitcode.NotFound, fmt.Errorf("no active context — run 'whctl context use <name>'"))
		},
	}
}

// loadConfigForContext loads the config file, translating errors to user-friendly messages.
func loadConfigForContext(whconfigFlag string) (*config.Config, error) {
	cfg, err := config.Load(whconfigFlag)
	if err != nil {
		return nil, exitErr(exitcode.Failure,
			fmt.Errorf("failed to load config: %w\nRun 'whctl context new' to create a config", err))
	}

	return cfg, nil
}

// expandHome expands a leading ~ to the user's home directory.
func expandHome(path string) string {
	if len(path) > 0 && path[0] == '~' {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}

		return filepath.Join(home, path[1:])
	}

	return path
}
