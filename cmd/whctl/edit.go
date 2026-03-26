package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/walheimlab/walheim-go/internal/exitcode"
	"github.com/walheimlab/walheim-go/internal/fs"
	"github.com/walheimlab/walheim-go/internal/labels"
	"github.com/walheimlab/walheim-go/internal/registry"
)

func newEditCmd(gf *GlobalFlags) *cobra.Command {
	var namespace string

	cmd := &cobra.Command{
		Use:   "edit <kind> <name>",
		Short: "Edit a resource manifest in your editor",
		Long: `Open a resource manifest in $EDITOR and apply it if changed.

The apply is aborted if the file is unchanged, becomes empty, or contains invalid YAML.`,
		Example: `  whctl edit namespace production
  whctl edit app myapp -n production
  whctl edit secret db-creds -n production`,
		Args:         cobra.ExactArgs(2),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			kind := args[0]
			name := args[1]

			if err := validateResourceName(kind); err != nil {
				return exitErr(exitcode.UsageError, err)
			}

			if err := validateResourceName(name); err != nil {
				return exitErr(exitcode.UsageError, err)
			}

			backend, dataDir, err := resolveBackend(gf.Context, gf.Whconfig)
			if err != nil {
				return exitErr(exitcode.Failure, err)
			}

			return runEdit(backend, dataDir, kind, name, namespace, gf)
		},
	}

	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "Target namespace")

	return cmd
}

func runEdit(filesystem fs.FS, dataDir, kind, name, namespace string, gf *GlobalFlags) error {
	// Resolve path — also validates kind exists and scope vs namespace.
	manifestPath, err := labels.ResolvePath(filesystem, dataDir, kind, name, namespace)
	if err != nil {
		return err
	}

	// Read current content from the backend.
	original, err := filesystem.ReadFile(manifestPath)
	if err != nil {
		return exitErr(exitcode.NotFound,
			fmt.Errorf("%s/%s not found — apply it first before editing", kind, name))
	}

	// Write to a local temp file for the editor.
	tmp, err := os.CreateTemp("", "whctl-edit-*.yaml")
	if err != nil {
		return exitErr(exitcode.Failure, fmt.Errorf("create temp file: %w", err))
	}

	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(original); err != nil {
		_ = tmp.Close()
		return exitErr(exitcode.Failure, fmt.Errorf("write temp file: %w", err))
	}

	if err := tmp.Close(); err != nil {
		return exitErr(exitcode.Failure, fmt.Errorf("close temp file: %w", err))
	}

	// Open in editor.
	editorParts := resolveEditor()
	editorCmd := exec.Command(editorParts[0], append(editorParts[1:], tmpPath)...)
	editorCmd.Stdin = os.Stdin
	editorCmd.Stdout = os.Stdout
	editorCmd.Stderr = os.Stderr

	if err := editorCmd.Run(); err != nil {
		return exitErr(exitcode.Failure, fmt.Errorf("editor: %w", err))
	}

	// Read edited content from the local temp file.
	edited, err := os.ReadFile(tmpPath)
	if err != nil {
		return exitErr(exitcode.Failure, fmt.Errorf("read edited file: %w", err))
	}

	// Abort if empty.
	if len(bytes.TrimSpace(edited)) == 0 {
		fmt.Fprintln(os.Stderr, "Edit cancelled: file is empty.")
		return exitErr(exitcode.UsageError, fmt.Errorf("edit cancelled: file is empty"))
	}

	// Abort if unchanged.
	if bytes.Equal(original, edited) {
		fmt.Println("Edit cancelled: no changes.")
		return nil
	}

	// Validate YAML structure.
	var doc yaml.Node
	if err := yaml.Unmarshal(edited, &doc); err != nil {
		return exitErr(exitcode.UsageError, fmt.Errorf("edit cancelled: invalid YAML: %w", err))
	}

	if doc.Kind == 0 {
		fmt.Fprintln(os.Stderr, "Edit cancelled: file contains no YAML content.")
		return exitErr(exitcode.UsageError, fmt.Errorf("edit cancelled: no YAML content"))
	}

	// Dispatch to the resource's apply operation.
	entry := registry.Get(kind)
	if entry == nil {
		return exitErr(exitcode.UsageError, fmt.Errorf("unknown kind %q", kind))
	}

	op := entry.FindOperation("apply")
	if op == nil {
		return exitErr(exitcode.UsageError,
			fmt.Errorf("resource %q does not support apply", entry.Registration.Info.Plural))
	}

	handler := entry.Registration.Factory(dataDir, filesystem)

	return op.Run(handler, registry.OperationOpts{
		DataDir:     dataDir,
		FS:          filesystem,
		Kind:        entry.Registration.Info.Plural,
		Name:        name,
		Namespace:   namespace,
		Output:      gf.Output,
		Quiet:       gf.Quiet,
		RawManifest: edited,
		Flags:       make(map[string]any),
	})
}

// resolveEditor returns the editor command split into parts (cmd + args).
// Reads $VISUAL first, then $EDITOR, then falls back to "vi".
func resolveEditor() []string {
	for _, env := range []string{"VISUAL", "EDITOR"} {
		if v := os.Getenv(env); v != "" {
			return strings.Fields(v)
		}
	}

	return []string{"vi"}
}
