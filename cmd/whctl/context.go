package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/walheimlab/walheim-go/internal/config"
	"github.com/walheimlab/walheim-go/internal/exitcode"
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
					Name    string `json:"name"`
					DataDir string `json:"dataDir"`
					Active  bool   `json:"active"`
				}
				result := make([]contextJSON, len(views))
				for i, v := range views {
					result[i] = contextJSON{Name: v.Name, DataDir: v.DataDir, Active: v.Active}
				}
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(result)
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
			fmt.Fprintln(w, "ACTIVE\tNAME\tDATA DIR")
			for _, v := range views {
				active := " "
				if v.Active {
					active = "*"
				}
				fmt.Fprintf(w, "%s\t%s\t%s\n", active, v.Name, v.DataDir)
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

// newContextNewCmd implements `whctl context new <name> --data-dir <path>`.
func newContextNewCmd(gf *GlobalFlags) *cobra.Command {
	var dataDirFlag string

	cmd := &cobra.Command{
		Use:   "new <name>",
		Short: "Add a new context and activate it",
		Long: `Add a new context pointing to an existing data directory.
The directory must already exist. The namespaces/ subdirectory is created if missing.`,
		Example: "  whctl context new homelab --data-dir ~/homelab",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			if dataDirFlag == "" {
				return exitErr(exitcode.UsageError, fmt.Errorf("--data-dir is required"))
			}

			// Expand ~ manually since config.expandHome is unexported
			expandedDir := expandHome(dataDirFlag)

			// data directory must pre-exist
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

			// Auto-create namespaces/ subdirectory if missing
			nsDir := filepath.Join(expandedDir, "namespaces")
			if _, err := os.Stat(nsDir); os.IsNotExist(err) {
				fmt.Printf("Notice: creating %s\n", nsDir)
				if err := os.MkdirAll(nsDir, 0755); err != nil {
					return exitErr(exitcode.Failure,
						fmt.Errorf("failed to create namespaces directory: %w", err))
				}
			}

			// Try loading existing config; fall back to creating a fresh one.
			var cfg *config.Config
			if existing, loadErr := config.Load(gf.Whconfig); loadErr == nil {
				cfg = existing
			} else {
				// Config doesn't exist or is invalid — initialise a fresh one.
				fresh, initErr := config.Init(gf.Whconfig)
				if initErr != nil {
					return exitErr(exitcode.Failure,
						fmt.Errorf("failed to initialise config: %w", initErr))
				}
				cfg = fresh
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

	cmd.Flags().StringVar(&dataDirFlag, "data-dir", "", "Path to homelab data directory (must exist)")
	return cmd
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
							Name    string `json:"name"`
							DataDir string `json:"dataDir"`
						}
						enc := json.NewEncoder(os.Stdout)
						enc.SetIndent("", "  ")
						return enc.Encode(currentJSON{Name: v.Name, DataDir: v.DataDir})
					}
					fmt.Printf("%s\t%s\n", v.Name, v.DataDir)
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
