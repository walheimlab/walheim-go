package v1alpha1

import (
	"fmt"

	"github.com/walheimlab/walheim-go/internal/exitcode"
	"github.com/walheimlab/walheim-go/internal/output"
	"github.com/walheimlab/walheim-go/internal/registry"
	apiv1alpha1 "github.com/walheimlab/walheim-go/pkg/api/walheim/v1alpha1"
)

func (n *Namespace) runCreate(opts registry.OperationOpts) error {
	jsonMode := opts.Output == "json"
	name := opts.Name

	exists, err := n.Exists(name)
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	if exists {
		msg := fmt.Sprintf("namespace %q already exists", name)
		output.Errorf(jsonMode, "Conflict", msg,
			"Use 'whctl apply' to update an existing namespace.", nil, false)

		return exitErr(exitcode.Conflict, fmt.Errorf("%s", msg))
	}

	hostname := opts.String("hostname")
	if hostname == "" {
		hostname = name
	}

	username := opts.String("username")
	baseDir := opts.String("base-dir")

	m := &apiv1alpha1.Namespace{
		Spec: apiv1alpha1.NamespaceSpec{Hostname: hostname, Username: username, BaseDir: baseDir},
	}
	m.APIVersion = namespaceKind.APIVersion()
	m.Kind = namespaceKind.Kind
	m.Name = name

	if opts.DryRun {
		fmt.Printf("Would create namespace %q (hostname: %s, username: %s, base-dir: %s)\n",
			name, hostname, m.Spec.UsernameDisplay(), m.Spec.BaseDirDisplay())

		return nil
	}

	if err := n.EnsureDir(name); err != nil {
		return exitErr(exitcode.Failure, err)
	}

	if err := n.createSubdirs(name); err != nil {
		return exitErr(exitcode.Failure, err)
	}

	if err := n.WriteManifest(name, m); err != nil {
		return exitErr(exitcode.Failure, err)
	}

	fmt.Printf("Created namespace %q (hostname: %s, username: %s, base-dir: %s)\n",
		name, hostname, m.Spec.UsernameDisplay(), m.Spec.BaseDirDisplay())

	return nil
}
