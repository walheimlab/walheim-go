package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/walheimlab/walheim-go/internal/exitcode"
	"github.com/walheimlab/walheim-go/internal/fs"
	"github.com/walheimlab/walheim-go/internal/registry"
	"github.com/walheimlab/walheim-go/internal/version"

	v1alpha1 "github.com/walheimlab/walheim-go/internal/resources/walheim/v1alpha1"
)

// GlobalFlags holds the persistent flags declared on the root command.
type GlobalFlags struct {
	Context  string
	Whconfig string
	Output   string
	Quiet    bool
}

func main() {
	if err := buildRoot().Execute(); err != nil {
		// Extract exit code if available
		if ee, ok := err.(*exitcode.Error); ok {
			fmt.Fprintln(os.Stderr, "Error:", ee.Err.Error())
			os.Exit(ee.Code)
		}
		fmt.Fprintln(os.Stderr, "Error:", err.Error())
		os.Exit(exitcode.Failure)
	}
}

func buildRoot() *cobra.Command {
	v1alpha1.Register()

	gf := &GlobalFlags{}

	root := &cobra.Command{
		Use:   "whctl",
		Short: "Walheim homelab controller — kubectl-style CLI for self-hosted apps",
		Long: `whctl manages homelab environments: namespaces, apps, secrets, and configmaps.

Global flags apply to every command. Set WHCONFIG env var to override config file path.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.PersistentFlags().StringVar(&gf.Context, "context", "", "Override active context")
	root.PersistentFlags().StringVar(&gf.Whconfig, "whconfig", "", "Alternate config file path (default: ~/.walheim/config or $WHCONFIG)")
	root.PersistentFlags().StringVarP(&gf.Output, "output", "o", "table", "Output format: table|json")
	root.PersistentFlags().BoolVarP(&gf.Quiet, "quiet", "q", false, "Bare output (one item per line, no headers)")

	root.AddCommand(newVersionCmd())
	root.AddCommand(newContextCmd(gf))
	BuildCommandTree(root, gf)

	return root
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show whctl version",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("whctl version %s (commit: %s, built: %s)\n",
				version.Version, version.GitCommit, version.BuildDate)
			return nil
		},
	}
}

// BuildCommandTree generates one cobra command per unique verb across all
// registered resources, then adds it to root.
func BuildCommandTree(root *cobra.Command, gf *GlobalFlags) {
	localFS := fs.NewLocalFS()

	for _, verb := range registry.AllOperations() {
		verb := verb // capture
		cmd := buildVerbCommand(verb, gf, localFS)
		root.AddCommand(cmd)
	}
}

// buildVerbCommand builds a single cobra command for a verb.
// It collects all entries that declare the verb to merge their flags and examples.
func buildVerbCommand(verb string, gf *GlobalFlags, localFS fs.FS) *cobra.Command {
	var declaringEntries []*registry.Entry
	for _, e := range registry.AllEntries() {
		if e.FindOperation(verb) != nil {
			declaringEntries = append(declaringEntries, e)
		}
	}

	cmd := &cobra.Command{
		Use:          verb + " <kind> [name]",
		Short:        buildShort(verb, declaringEntries),
		Example:      buildExamples(verb, declaringEntries),
		Args:         cobra.RangeArgs(1, 2),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			kind := args[0]
			name := ""
			if len(args) > 1 {
				name = args[1]
			}

			if err := validateResourceName(kind); err != nil {
				return exitErr(exitcode.UsageError, err)
			}
			if name != "" {
				if err := validateResourceName(name); err != nil {
					return exitErr(exitcode.UsageError, err)
				}
			}

			entry := registry.Get(kind)
			if entry == nil {
				return exitErr(exitcode.UsageError,
					fmt.Errorf("unknown kind %q — run 'whctl --help' to see available kinds", kind))
			}

			op := entry.FindOperation(verb)
			if op == nil {
				return exitErr(exitcode.UsageError,
					fmt.Errorf("%s is not supported for %s", verb, kind))
			}

			opts, err := collectOpts(cmd, gf, kind, name, entry, op, localFS)
			if err != nil {
				return err
			}

			handler := entry.Registration.Factory(opts.DataDir, localFS)
			return op.Run(handler, opts)
		},
	}

	// Universal flags on every verb command
	cmd.Flags().StringP("namespace", "n", "", "Target namespace")
	cmd.Flags().BoolP("all-namespaces", "A", false, "All namespaces")
	cmd.Flags().Bool("dry-run", false, "Print what would change without making any changes")
	cmd.Flags().Bool("yes", false, "Skip confirmation prompts")

	// Merge operation-specific flags from all resources declaring this verb
	mergeOperationFlags(cmd, verb, declaringEntries)

	return cmd
}

// collectOpts assembles OperationOpts from parsed flags and config.
func collectOpts(cmd *cobra.Command, gf *GlobalFlags,
	kind, name string, entry *registry.Entry, op *registry.OperationDef,
	localFS fs.FS) (registry.OperationOpts, error) {

	dataDir, err := resolveDataDir(gf.Context, gf.Whconfig)
	if err != nil {
		return registry.OperationOpts{}, exitErr(exitcode.Failure, fmt.Errorf("%s", err))
	}

	namespace, _ := cmd.Flags().GetString("namespace")
	allNS, _ := cmd.Flags().GetBool("all-namespaces")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	yes, _ := cmd.Flags().GetBool("yes")

	// Validate namespace requirements for namespaced resources
	if !entry.IsCluster() {
		switch op.NSHandling {
		case registry.NSRequired:
			if namespace == "" {
				return registry.OperationOpts{}, exitErr(exitcode.UsageError,
					fmt.Errorf("-n <namespace> is required\nUsage: whctl %s %s <name> -n <namespace>",
						op.Verb, entry.Registration.Info.Singular()))
			}
		case registry.NSOptionalAll:
			if namespace == "" && !allNS {
				return registry.OperationOpts{}, exitErr(exitcode.UsageError,
					fmt.Errorf("either -n <namespace> or -A is required\nUsage: whctl %s %s [-n <namespace>] [-A]",
						op.Verb, entry.Registration.Info.Plural))
			}
		}
	}

	// Collect operation-specific flags
	flags := make(map[string]any)
	for _, fd := range op.Flags {
		switch fd.Type {
		case "string":
			v, _ := cmd.Flags().GetString(fd.Name)
			flags[fd.Name] = v
		case "bool":
			v, _ := cmd.Flags().GetBool(fd.Name)
			flags[fd.Name] = v
		case "int":
			v, _ := cmd.Flags().GetInt(fd.Name)
			flags[fd.Name] = v
		}
	}

	return registry.OperationOpts{
		DataDir:       dataDir,
		FS:            localFS,
		Kind:          kind,
		Name:          name,
		Namespace:     namespace,
		AllNamespaces: allNS,
		Output:        gf.Output,
		Quiet:         gf.Quiet,
		DryRun:        dryRun,
		Yes:           yes,
		Flags:         flags,
	}, nil
}

// mergeOperationFlags registers operation-specific flags from all resources
// that declare this verb. Flags are deduplicated by name.
func mergeOperationFlags(cmd *cobra.Command, verb string, entries []*registry.Entry) {
	seen := make(map[string]bool)

	for _, e := range entries {
		op := e.FindOperation(verb)
		if op == nil {
			continue
		}
		for _, fd := range op.Flags {
			if seen[fd.Name] {
				continue
			}
			seen[fd.Name] = true

			switch fd.Type {
			case "string":
				defaultStr := ""
				if fd.Default != nil {
					if s, ok := fd.Default.(string); ok {
						defaultStr = s
					}
				}
				if fd.Short != "" {
					cmd.Flags().StringP(fd.Name, fd.Short, defaultStr, fd.Usage)
				} else {
					cmd.Flags().String(fd.Name, defaultStr, fd.Usage)
				}
			case "bool":
				defaultBool := false
				if fd.Default != nil {
					if b, ok := fd.Default.(bool); ok {
						defaultBool = b
					}
				}
				if fd.Short != "" {
					cmd.Flags().BoolP(fd.Name, fd.Short, defaultBool, fd.Usage)
				} else {
					cmd.Flags().Bool(fd.Name, defaultBool, fd.Usage)
				}
			case "int":
				defaultInt := 0
				if fd.Default != nil {
					if i, ok := fd.Default.(int); ok {
						defaultInt = i
					}
				}
				if fd.Short != "" {
					cmd.Flags().IntP(fd.Name, fd.Short, defaultInt, fd.Usage)
				} else {
					cmd.Flags().Int(fd.Name, defaultInt, fd.Usage)
				}
			}
		}
	}
}

// buildShort generates a one-line help string for a verb command.
func buildShort(verb string, entries []*registry.Entry) string {
	if len(entries) == 0 {
		return strings.Title(verb) + " resources" //nolint:staticcheck
	}

	var kinds []string
	for _, e := range entries {
		kinds = append(kinds, e.Registration.Info.Plural)
	}

	// Use first declared short description
	for _, e := range entries {
		op := e.FindOperation(verb)
		if op != nil && op.Short != "" {
			return op.Short
		}
	}

	return fmt.Sprintf("%s %s", verb, strings.Join(kinds, "|"))
}

// buildExamples collects example strings from all resources declaring this verb.
func buildExamples(verb string, entries []*registry.Entry) string {
	var lines []string
	for _, e := range entries {
		op := e.FindOperation(verb)
		if op == nil {
			continue
		}
		for _, ex := range op.Examples {
			lines = append(lines, "  "+ex)
		}
	}
	return strings.Join(lines, "\n")
}
