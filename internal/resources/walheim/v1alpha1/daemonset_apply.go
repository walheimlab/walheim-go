package v1alpha1

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/walheimlab/walheim-go/internal/exitcode"
	"github.com/walheimlab/walheim-go/internal/output"
	"github.com/walheimlab/walheim-go/internal/registry"
	apiv1alpha1 "github.com/walheimlab/walheim-go/pkg/api/walheim/v1alpha1"
)

func (d *DaemonSet) runApply(opts registry.OperationOpts) error {
	jsonMode := opts.Output == "json"
	name := opts.Name

	var data []byte
	if len(opts.RawManifest) > 0 {
		data = opts.RawManifest
	} else {
		filePath := opts.String("file")
		if filePath == "" {
			msg := "--file (-f) is required for 'apply daemonset'"
			output.Errorf(jsonMode, "UsageError", msg,
				"whctl apply daemonset <name> -f <path>", nil, false)

			return exitErr(exitcode.UsageError, fmt.Errorf("%s", msg))
		}

		var err error

		data, err = readInput(filePath, opts.FS)
		if err != nil {
			return exitErr(exitcode.Failure, fmt.Errorf("read %q: %w", filePath, err))
		}
	}

	var m apiv1alpha1.DaemonSet
	if err := yaml.Unmarshal(data, &m); err != nil {
		return exitErr(exitcode.Failure, fmt.Errorf("parse manifest: %w", err))
	}

	if err := validateDaemonSetManifest(&m, name); err != nil {
		output.Errorf(jsonMode, "ValidationError", err.Error(), "", nil, false)
		return exitErr(exitcode.UsageError, err)
	}

	exists, err := d.Exists(name)
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	if opts.DryRun {
		verb := "create"
		if exists {
			verb = "update"
		}

		fmt.Printf("Would %s daemonset %q\n", verb, name)

		return nil
	}

	if !exists {
		if err := d.EnsureDir(name); err != nil {
			return exitErr(exitcode.Failure, err)
		}

		if err := d.WriteManifest(name, &m); err != nil {
			return exitErr(exitcode.Failure, err)
		}

		fmt.Printf("Created daemonset %q\n", name)
	} else {
		if err := d.WriteManifest(name, &m); err != nil {
			return exitErr(exitcode.Failure, err)
		}

		fmt.Printf("Updated daemonset %q\n", name)
	}

	return d.runStart(opts)
}
