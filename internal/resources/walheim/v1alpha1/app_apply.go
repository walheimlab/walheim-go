package v1alpha1

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/walheimlab/walheim-go/internal/exitcode"
	"github.com/walheimlab/walheim-go/internal/output"
	"github.com/walheimlab/walheim-go/internal/registry"
	apiv1alpha1 "github.com/walheimlab/walheim-go/pkg/api/walheim/v1alpha1"
)

func (a *App) runApply(opts registry.OperationOpts) error {
	jsonMode := opts.Output == "json"
	namespace := opts.Namespace
	name := opts.Name

	var data []byte
	if len(opts.RawManifest) > 0 {
		data = opts.RawManifest
	} else {
		filePath := opts.String("file")
		if filePath == "" {
			msg := "--file (-f) is required for 'apply app'"
			output.Errorf(jsonMode, "UsageError", msg,
				"whctl apply app <name> -n <namespace> -f <path>", nil, false)

			return exitErr(exitcode.UsageError, fmt.Errorf("%s", msg))
		}

		var err error

		data, err = readInput(filePath, opts.FS)
		if err != nil {
			return exitErr(exitcode.Failure, fmt.Errorf("read %q: %w", filePath, err))
		}
	}

	var m apiv1alpha1.App
	if err := yaml.Unmarshal(data, &m); err != nil {
		return exitErr(exitcode.Failure, fmt.Errorf("parse manifest: %w", err))
	}

	if err := validateAppManifest(&m, namespace, name); err != nil {
		output.Errorf(jsonMode, "ValidationError", err.Error(), "", nil, false)
		return exitErr(exitcode.UsageError, err)
	}

	exists, err := a.Exists(namespace, name)
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	if opts.DryRun {
		verb := "create"
		if exists {
			verb = "update"
		}

		fmt.Printf("Would %s app %q in namespace %q\n", verb, name, namespace)

		return nil
	}

	if !exists {
		if err := a.EnsureDir(namespace, name); err != nil {
			return exitErr(exitcode.Failure, err)
		}

		if err := a.WriteManifest(namespace, name, &m); err != nil {
			return exitErr(exitcode.Failure, err)
		}

		fmt.Printf("Created app %q in namespace %q\n", name, namespace)
	} else {
		if err := a.WriteManifest(namespace, name, &m); err != nil {
			return exitErr(exitcode.Failure, err)
		}

		fmt.Printf("Updated app %q in namespace %q\n", name, namespace)
	}

	// apply auto-starts (post-create and post-update hook)
	return a.runStart(opts)
}
