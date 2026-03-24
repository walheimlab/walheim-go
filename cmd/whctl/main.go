package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/walheimlab/walheim-go/internal/exitcode"
	"github.com/walheimlab/walheim-go/internal/fs"
	"github.com/walheimlab/walheim-go/internal/manifest"
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
	root.PersistentFlags().StringVarP(&gf.Output, "output", "o", "human", "Output format: human|yaml|json")
	root.PersistentFlags().BoolVarP(&gf.Quiet, "quiet", "q", false, "Bare output (one item per line, no headers)")

	root.AddGroup(
		&cobra.Group{ID: "verbs", Title: "Resource commands:"},
		&cobra.Group{ID: "mgmt", Title: "Management:"},
	)

	versionCmd := newVersionCmd()
	versionCmd.GroupID = "mgmt"
	root.AddCommand(versionCmd)

	contextCmd := newContextCmd(gf)
	contextCmd.GroupID = "mgmt"
	root.AddCommand(contextCmd)

	actionsCmd := newActionsCmd()
	actionsCmd.GroupID = "mgmt"
	root.AddCommand(actionsCmd)

	labelCmd := newLabelCmd(gf)
	labelCmd.GroupID = "mgmt"
	root.AddCommand(labelCmd)

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
	for _, verb := range registry.AllOperations() {
		verb := verb // capture
		cmd := buildVerbCommand(verb, gf)
		cmd.GroupID = "verbs"
		root.AddCommand(cmd)
	}
}

// buildVerbCommand builds a cobra command for a verb with one subcommand per
// resource kind that declares it (plural name + singular + aliases).
// Running the verb with no kind lists resources that support it.
func buildVerbCommand(verb string, gf *GlobalFlags) *cobra.Command {
	var declaringEntries []*registry.Entry

	for _, e := range registry.AllEntries() {
		if e.FindOperation(verb) != nil {
			declaringEntries = append(declaringEntries, e)
		}
	}

	verbCmd := &cobra.Command{
		Use:          verb + " <kind> [name]",
		Short:        verbShort(verb),
		Example:      buildVerbExamples(verb, declaringEntries),
		SilenceUsage: true,
		// RunE fires when no kind subcommand matches.
		// If -f/--filename is set, dispatch via manifest file(s).
		// Otherwise list resource kinds that support this verb.
		RunE: func(cmd *cobra.Command, args []string) error {
			filenames, _ := cmd.Flags().GetStringArray("filename")
			if len(filenames) > 0 {
				return runFileDispatch(verb, cmd, gf)
			}

			return runListResourcesForVerb(verb, declaringEntries)
		},
	}

	// Flags for verb-level -f/--filename dispatch (kubectl-style).
	verbCmd.Flags().StringArrayP("filename", "f", nil, "File, directory, or URL containing resource manifest(s)")
	verbCmd.Flags().StringP("namespace", "n", "", "Override namespace from manifest metadata")
	verbCmd.Flags().Bool("dry-run", false, "Print what would change without making any changes")
	verbCmd.Flags().Bool("yes", false, "Skip confirmation prompts")

	for _, e := range declaringEntries {
		op := e.FindOperation(verb)
		info := e.Registration.Info

		// All kind subcommands are hidden from the verb-level help so that
		// `whctl get -h` stays generic and never mentions resource names.
		// They are still fully routable: `whctl get apps -h` works correctly.
		for _, kindName := range append([]string{info.Plural, info.Singular()}, info.Aliases...) {
			sub := newKindCmd(verb, kindName, e, op, gf)
			sub.Hidden = true
			verbCmd.AddCommand(sub)
		}
	}

	return verbCmd
}

// newKindCmd builds a single cobra subcommand for verb+kind under a specific name.
func newKindCmd(verb, kindName string, e *registry.Entry, op *registry.OperationDef,
	gf *GlobalFlags) *cobra.Command {
	info := e.Registration.Info

	use := kindName
	if op.RequiresName {
		use = kindName + " <name>"
	} else if !op.RequiresName && verb == "get" {
		use = kindName + " [name]"
	}

	cmd := &cobra.Command{
		Use:          use,
		Short:        opShort(op),
		Example:      strings.Join(prependSpaces(op.Examples), "\n"),
		SilenceUsage: true,
		Args:         cobra.RangeArgs(0, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) > 0 {
				name = args[0]
				if err := validateResourceName(name); err != nil {
					return exitErr(exitcode.UsageError, err)
				}
			}

			if op.RequiresName && name == "" {
				return exitErr(exitcode.UsageError,
					fmt.Errorf("<name> is required\nUsage: whctl %s %s", verb, use))
			}

			opts, err := collectOpts(cmd, gf, info.Plural, name, e, op)
			if err != nil {
				return err
			}

			handler := e.Registration.Factory(opts.DataDir, opts.FS)

			return op.Run(handler, opts)
		},
	}

	// Universal flags
	cmd.Flags().StringP("namespace", "n", "", "Target namespace")
	cmd.Flags().BoolP("all-namespaces", "A", false, "All namespaces")
	cmd.Flags().Bool("dry-run", false, "Print what would change without making any changes")
	cmd.Flags().Bool("yes", false, "Skip confirmation prompts")

	// Operation-specific flags for this resource only
	addOperationFlags(cmd, op)

	return cmd
}

// addOperationFlags registers the flags declared by a single OperationDef onto cmd.
func addOperationFlags(cmd *cobra.Command, op *registry.OperationDef) {
	for _, fd := range op.Flags {
		switch fd.Type {
		case "string":
			def, _ := fd.Default.(string)
			if fd.Short != "" {
				cmd.Flags().StringP(fd.Name, fd.Short, def, fd.Usage)
			} else {
				cmd.Flags().String(fd.Name, def, fd.Usage)
			}
		case "bool":
			def, _ := fd.Default.(bool)
			if fd.Short != "" {
				cmd.Flags().BoolP(fd.Name, fd.Short, def, fd.Usage)
			} else {
				cmd.Flags().Bool(fd.Name, def, fd.Usage)
			}
		case "int":
			def, _ := fd.Default.(int)
			if fd.Short != "" {
				cmd.Flags().IntP(fd.Name, fd.Short, def, fd.Usage)
			} else {
				cmd.Flags().Int(fd.Name, def, fd.Usage)
			}
		}
	}
}

func prependSpaces(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = "  " + l
	}

	return out
}

// collectOpts assembles OperationOpts from parsed flags and config.
func collectOpts(cmd *cobra.Command, gf *GlobalFlags,
	kind, name string, entry *registry.Entry, op *registry.OperationDef) (registry.OperationOpts, error) {
	filesystem, dataDir, err := resolveBackend(gf.Context, gf.Whconfig)
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
		FS:            filesystem,
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

// verbDefaults holds the canonical one-line description for each known verb.
// These are shown in the root command help and used as fallbacks wherever a
// resource does not override the short description for that verb.
// A resource's OperationDef.Short should only be non-empty when its behaviour
// for that verb genuinely differs from the generic (e.g. "delete app" also
// removes remote files; "describe namespace" does a live SSH probe).
var verbDefaults = map[string]string{
	"get":      "List or get resources",
	"create":   "Create a resource",
	"apply":    "Create or update a resource from a manifest file",
	"delete":   "Delete a resource",
	"describe": "Show detailed resource information",
	"doctor":   "Check resources for issues",
	"import":   "Import a resource from an external file",
	"start":    "Start a resource",
	"pause":    "Pause a resource",
	"stop":     "Stop a resource",
	"pull":     "Pull the latest images for a resource",
	"logs":     "Print logs for a resource",
	"exec":     "Execute a command in a resource",
	"run":      "Run a job on its target host",
}

// verbShort returns the canonical short description for a verb.
// Falls back to title-casing the verb name if not in verbDefaults.
func verbShort(verb string) string {
	if s, ok := verbDefaults[verb]; ok {
		return s
	}

	return strings.ToUpper(verb[:1]) + verb[1:] + " a resource"
}

// opShort returns the effective short description for one operation on one resource.
// The resource's own Short takes precedence; verbShort is the fallback.
func opShort(op *registry.OperationDef) string {
	if op.Short != "" {
		return op.Short
	}

	return verbShort(op.Verb)
}

// buildVerbExamples generates the Examples block for a verb command.
// It emits one line per resource that declares the verb, using the plural kind
// name so the user can see exactly what to type for each resource.
func buildVerbExamples(verb string, entries []*registry.Entry) string {
	lines := make([]string, 0, len(entries))
	for _, e := range entries {
		lines = append(lines, "  whctl "+verb+" "+e.Registration.Info.Plural)
	}

	return strings.Join(lines, "\n")
}

// newActionsCmd returns the 'actions' command: lists all verbs a resource supports.
func newActionsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "actions <kind>",
		Short: "List all actions (verbs) available for a resource kind",
		Example: `  whctl actions namespaces
  whctl actions apps
  whctl actions ns`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			kind := args[0]
			if err := validateResourceName(kind); err != nil {
				return exitErr(exitcode.UsageError, err)
			}

			entry := registry.Get(kind)
			if entry == nil {
				return exitErr(exitcode.UsageError,
					fmt.Errorf("unknown kind %q — run 'whctl --help' to see available kinds", kind))
			}

			info := entry.Registration.Info
			fmt.Printf("Actions for '%s':\n\n", info.Plural)

			for _, op := range entry.Registration.Operations {
				fmt.Printf("  %-16s  %s\n", op.Verb, opShort(&op))
			}

			fmt.Printf("\nRun 'whctl <action> %s --help' for usage details.\n", info.Plural)

			return nil
		},
	}
}

// runFileDispatch loads manifests from the -f/--filename sources, infers the
// GVK of each document, and delegates to the matching resource operation handler.
// name and namespace are taken from manifest metadata; -n overrides namespace.
func runFileDispatch(verb string, cmd *cobra.Command, gf *GlobalFlags) error {
	filenames, _ := cmd.Flags().GetStringArray("filename")
	nsOverride, _ := cmd.Flags().GetString("namespace")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	yes, _ := cmd.Flags().GetBool("yes")

	filesystem, dataDir, err := resolveBackend(gf.Context, gf.Whconfig)
	if err != nil {
		return exitErr(exitcode.Failure, fmt.Errorf("%s", err))
	}

	envelopes, err := manifest.LoadSources(filenames, fs.NewLocalFS())
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	for _, env := range envelopes {
		// Kind in manifests is PascalCase; registry keys are lowercase.
		entry := registry.Get(strings.ToLower(env.Kind))
		if entry == nil {
			return exitErr(exitcode.UsageError,
				fmt.Errorf("unknown kind %q (name: %q) — is this a walheim/v1alpha1 resource?",
					env.Kind, env.Name))
		}

		op := entry.FindOperation(verb)
		if op == nil {
			return exitErr(exitcode.UsageError,
				fmt.Errorf("resource %q does not support %q",
					entry.Registration.Info.Plural, verb))
		}

		ns := env.Namespace
		if nsOverride != "" {
			ns = nsOverride
		}

		handler := entry.Registration.Factory(dataDir, filesystem)
		opts := registry.OperationOpts{
			DataDir:     dataDir,
			FS:          filesystem,
			Kind:        entry.Registration.Info.Plural,
			Name:        env.Name,
			Namespace:   ns,
			Output:      gf.Output,
			Quiet:       gf.Quiet,
			DryRun:      dryRun,
			Yes:         yes,
			RawManifest: env.Raw,
			Flags:       make(map[string]any),
		}

		if err := op.Run(handler, opts); err != nil {
			return err
		}
	}

	return nil
}

// runListResourcesForVerb prints which resource kinds support a given verb,
// with the per-kind short description of that operation.
func runListResourcesForVerb(verb string, entries []*registry.Entry) error {
	fmt.Printf("Resources that support '%s':\n\n", verb)

	for _, e := range entries {
		op := e.FindOperation(verb)
		if op == nil {
			continue
		}

		fmt.Printf("  %-16s  %s\n", e.Registration.Info.Plural, opShort(op))
	}

	fmt.Printf("\nRun 'whctl %s <kind> --help' for usage details.\n", verb)

	return nil
}
