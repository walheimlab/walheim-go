package v1alpha1

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/walheimlab/walheim-go/internal/exitcode"
	"github.com/walheimlab/walheim-go/internal/output"
	"github.com/walheimlab/walheim-go/internal/registry"
	"github.com/walheimlab/walheim-go/internal/yamlutil"
	apiv1alpha1 "github.com/walheimlab/walheim-go/pkg/api/walheim/v1alpha1"
)

func (a *App) runImport(opts registry.OperationOpts) error {
	jsonMode := opts.Output == "json"
	namespace := opts.Namespace
	name := opts.Name

	filePath := opts.String("file")
	if filePath == "" {
		msg := "--file (-f) is required for 'import app'"
		output.Errorf(jsonMode, "UsageError", msg,
			"whctl import app <name> -n <namespace> -f <path>", nil, false)

		return exitErr(exitcode.UsageError, fmt.Errorf("%s", msg))
	}

	data, err := readInput(filePath, opts.FS)
	if err != nil {
		return exitErr(exitcode.Failure, fmt.Errorf("read %q: %w", filePath, err))
	}

	var composeSpec apiv1alpha1.ComposeSpec
	if err := yaml.Unmarshal(data, &composeSpec); err != nil {
		return exitErr(exitcode.Failure, fmt.Errorf("parse compose file: %w", err))
	}

	m := &apiv1alpha1.App{
		Spec: apiv1alpha1.AppSpec{Compose: composeSpec},
	}
	m.APIVersion = appKind.APIVersion()
	m.Kind = appKind.Kind
	m.Name = name
	m.Namespace = namespace

	if opts.DryRun {
		encoded, err := yamlutil.Marshal(m)
		if err != nil {
			return exitErr(exitcode.Failure, err)
		}

		fmt.Print(string(encoded))

		return nil
	}

	exists, err := a.Exists(namespace, name)
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	if !exists {
		if err := a.EnsureDir(namespace, name); err != nil {
			return exitErr(exitcode.Failure, err)
		}
	}

	if err := a.WriteManifest(namespace, name, m); err != nil {
		return exitErr(exitcode.Failure, err)
	}

	fmt.Printf("Imported app %q (no deploy — run 'whctl start app %s -n %s')\n", name, name, namespace)

	return nil
}
