package v1alpha1

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/walheimlab/walheim-go/internal/exitcode"
	"github.com/walheimlab/walheim-go/internal/output"
	"github.com/walheimlab/walheim-go/internal/registry"
	apiv1alpha1 "github.com/walheimlab/walheim-go/pkg/api/walheim/v1alpha1"
)

func (n *Namespace) runApply(opts registry.OperationOpts) error {
	jsonMode := opts.Output == "json"
	name := opts.Name

	var data []byte
	if len(opts.RawManifest) > 0 {
		data = opts.RawManifest
	} else {
		filePath := opts.String("file")
		if filePath == "" {
			msg := "--file (-f) is required for 'apply namespace'"
			output.Errorf(jsonMode, "UsageError", msg,
				"whctl apply namespace <name> -f <path>", nil, false)

			return exitErr(exitcode.UsageError, fmt.Errorf("%s", msg))
		}

		var err error

		data, err = readInput(filePath, opts.FS)
		if err != nil {
			return exitErr(exitcode.Failure, fmt.Errorf("read %q: %w", filePath, err))
		}
	}

	var m apiv1alpha1.Namespace
	if err := yaml.Unmarshal(data, &m); err != nil {
		return exitErr(exitcode.Failure, fmt.Errorf("parse manifest: %w", err))
	}

	if err := validateNamespaceManifest(&m); err != nil {
		output.Errorf(jsonMode, "ValidationError", err.Error(), "", nil, false)
		return exitErr(exitcode.UsageError, err)
	}

	if m.Name != name {
		msg := fmt.Sprintf("manifest metadata.name %q does not match argument %q",
			m.Name, name)
		output.Errorf(jsonMode, "ValidationError", msg, "", nil, false)

		return exitErr(exitcode.UsageError, fmt.Errorf("%s", msg))
	}

	exists, err := n.Exists(name)
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	if opts.DryRun {
		verb := "create"
		if exists {
			verb = "update"
		}

		fmt.Printf("Would %s namespace %q\n", verb, name)

		return nil
	}

	if !exists {
		if err := n.EnsureDir(name); err != nil {
			return exitErr(exitcode.Failure, err)
		}

		if err := n.createSubdirs(name); err != nil {
			return exitErr(exitcode.Failure, err)
		}

		if err := n.WriteManifest(name, &m); err != nil {
			return exitErr(exitcode.Failure, err)
		}

		fmt.Printf("Created namespace %q\n", name)
	} else {
		if err := n.WriteManifest(name, &m); err != nil {
			return exitErr(exitcode.Failure, err)
		}

		fmt.Printf("Updated namespace %q\n", name)
	}

	return nil
}
