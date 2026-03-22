package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/walheimlab/walheim-go/internal/exitcode"
	"github.com/walheimlab/walheim-go/internal/fs"
	"github.com/walheimlab/walheim-go/internal/labels"
	"github.com/walheimlab/walheim-go/internal/output"
)

func newLabelCmd(gf *GlobalFlags) *cobra.Command {
	var namespace string
	var overwrite bool
	var list bool
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "label <kind> <name> [KEY=VALUE...] [KEY-]",
		Short: "Update labels on a resource",
		Long: `Update labels on a resource manifest.

Use KEY=VALUE to set a label, KEY- (trailing dash) to remove a label.
Use --list to print all current labels without modifying them.`,
		Example: `  whctl label namespace production env=prod team=platform
  whctl label app myapp -n production tier=backend version=v1.2.3
  whctl label app myapp -n production old-label-
  whctl label secret db-creds -n production --overwrite tier=database
  whctl label namespace production env=staging --dry-run
  whctl label namespace production --list
  whctl label namespace production --list -o json`,
		Args:         cobra.MinimumNArgs(2),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			kind := args[0]
			name := args[1]
			specs := args[2:]

			jsonMode := gf.Output == "json"

			// Validate mutual exclusion of --list and specs.
			if list && len(specs) > 0 {
				msg := "--list cannot be combined with label specifications"
				output.Errorf(jsonMode, "UsageError", msg,
					"Use --list alone, or provide KEY=VALUE / KEY- specs without --list.", nil, false)
				return exitErr(exitcode.UsageError, fmt.Errorf("%s", msg))
			}
			if !list && len(specs) == 0 {
				msg := "no label specifications provided"
				output.Errorf(jsonMode, "UsageError", msg,
					"Usage: whctl label KIND NAME KEY=VALUE [KEY=VALUE...] [KEY-]\n"+
						"       whctl label KIND NAME --list", nil, false)
				return exitErr(exitcode.UsageError, fmt.Errorf("%s", msg))
			}

			backend, dataDir, err := resolveBackend(gf.Context, gf.Whconfig)
			if err != nil {
				return exitErr(exitcode.Failure, err)
			}

			if list {
				return runLabelList(backend, dataDir, kind, name, namespace, jsonMode)
			}
			return runLabelSet(backend, dataDir, kind, name, namespace, specs, overwrite, dryRun, jsonMode)
		},
	}

	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "Target namespace")
	cmd.Flags().BoolVar(&overwrite, "overwrite", false, "Replace existing label values")
	cmd.Flags().BoolVar(&list, "list", false, "List labels instead of setting/removing")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print what would change without writing")

	return cmd
}

func runLabelList(localFS fs.FS, dataDir, kind, name, namespace string, jsonMode bool) error {
	lbls, err := labels.Get(localFS, dataDir, kind, name, namespace)
	if err != nil {
		output.Errorf(jsonMode, labelErrorCode(err), err.Error(), "", nil, false)
		return err
	}

	if jsonMode {
		result := map[string]any{
			"kind":      kind,
			"name":      name,
			"namespace": namespace,
			"labels":    lbls,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	if len(lbls) == 0 {
		fmt.Println("No labels.")
		return nil
	}

	keys := make([]string, 0, len(lbls))
	for k := range lbls {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("%s=%s\n", k, lbls[k])
	}
	return nil
}

func runLabelSet(localFS fs.FS, dataDir, kind, name, namespace string,
	specs []string, overwrite, dryRun, jsonMode bool) error {

	if dryRun {
		return printLabelDryRun(kind, name, namespace, specs, jsonMode)
	}

	changed, removed, err := labels.SetTracked(localFS, dataDir, kind, name, namespace, specs, overwrite)
	if err != nil {
		output.Errorf(jsonMode, labelErrorCode(err), err.Error(), labelSetSuggestion(err), nil, false)
		return err
	}

	if jsonMode {
		result := map[string]any{
			"kind":      kind,
			"name":      name,
			"namespace": namespace,
			"changed":   changed,
			"removed":   removed,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	fmt.Printf("%s labeled\n", labelResourceRef(kind, name, namespace))
	return nil
}

func printLabelDryRun(kind, name, namespace string, specs []string, jsonMode bool) error {
	ref := labelResourceRef(kind, name, namespace)
	if jsonMode {
		actions := make([]map[string]string, 0, len(specs))
		for _, spec := range specs {
			if strings.HasSuffix(spec, "-") {
				actions = append(actions, map[string]string{
					"action": "remove",
					"key":    strings.TrimSuffix(spec, "-"),
				})
			} else if idx := strings.IndexByte(spec, '='); idx >= 0 {
				actions = append(actions, map[string]string{
					"action": "set",
					"key":    spec[:idx],
					"value":  spec[idx+1:],
				})
			}
		}
		result := map[string]any{
			"dry_run":   true,
			"kind":      kind,
			"name":      name,
			"namespace": namespace,
			"actions":   actions,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	for _, spec := range specs {
		if strings.HasSuffix(spec, "-") {
			fmt.Printf("Would remove label %s from %s\n", strings.TrimSuffix(spec, "-"), ref)
		} else if idx := strings.IndexByte(spec, '='); idx >= 0 {
			fmt.Printf("Would set label %s on %s\n", spec, ref)
		}
	}
	return nil
}

// labelResourceRef builds the display reference string.
// Cluster-scoped: "namespace/production"
// Namespaced:     "app/myapp -n production"
func labelResourceRef(kind, name, namespace string) string {
	if namespace != "" {
		return fmt.Sprintf("%s/%s -n %s", kind, name, namespace)
	}
	return fmt.Sprintf("%s/%s", kind, name)
}

// labelErrorCode maps a labels error to a string for JSON output's "error" field.
func labelErrorCode(err error) string {
	if ee, ok := err.(*exitcode.Error); ok {
		switch ee.Code {
		case exitcode.NotFound:
			return "NotFound"
		case exitcode.UsageError:
			return "UsageError"
		case exitcode.Conflict:
			return "Conflict"
		}
	}
	return "Failure"
}

// labelSetSuggestion returns a hint string for a label set error.
func labelSetSuggestion(err error) string {
	if strings.Contains(err.Error(), "already exists") {
		return "Use --overwrite to replace existing labels."
	}
	return ""
}
