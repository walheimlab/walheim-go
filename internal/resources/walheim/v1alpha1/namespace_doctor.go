package v1alpha1

import (
	"fmt"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/walheimlab/walheim-go/internal/doctor"
	"github.com/walheimlab/walheim-go/internal/exitcode"
	"github.com/walheimlab/walheim-go/internal/output"
	"github.com/walheimlab/walheim-go/internal/registry"
	apiv1alpha1 "github.com/walheimlab/walheim-go/pkg/api/walheim/v1alpha1"
)

func (n *Namespace) runDoctor(opts registry.OperationOpts) error {
	jsonMode := opts.Output == "json"
	rep := &doctor.Report{}

	names, err := n.ListNames()
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	if opts.Name != "" {
		exists, err := n.Exists(opts.Name)
		if err != nil {
			return exitErr(exitcode.Failure, err)
		}

		if !exists {
			msg := fmt.Sprintf("namespace %q not found", opts.Name)
			output.Errorf(jsonMode, "NotFound", msg, "", nil, false)

			return exitErr(exitcode.NotFound, fmt.Errorf("%s", msg))
		}

		names = []string{opts.Name}
	}

	for _, name := range names {
		n.doctorNamespace(rep, name)
	}

	if jsonMode {
		return rep.PrintJSON()
	}

	rep.PrintHuman(opts.Quiet)

	if rep.HasErrors() {
		return exitErr(exitcode.Failure, fmt.Errorf("doctor found errors"))
	}

	return nil
}

// doctorNamespace runs all checks for a single namespace and adds findings to rep.
func (n *Namespace) doctorNamespace(rep *doctor.Report, name string) {
	resourceID := "namespace/" + name

	data, err := n.ReadBytes(name)
	if err != nil {
		rep.Errorf(resourceID, "manifest-unreadable", "cannot read manifest: %v", err)
		return
	}

	var m apiv1alpha1.Namespace
	if err := yaml.Unmarshal(data, &m); err != nil {
		rep.Errorf(resourceID, "manifest-parse", "manifest YAML is invalid: %v", err)
		return
	}

	doctor.CheckAPIVersion(rep, resourceID, m.APIVersion, namespaceKind.APIVersion())
	doctor.CheckKind(rep, resourceID, m.Kind, namespaceKind.Kind)
	doctor.CheckDirNameMatchesMetadataName(rep, resourceID, name, m.Name)

	if m.Spec.Hostname == "" {
		rep.Errorf(resourceID, "missing-hostname", "spec.hostname is required but not set")
	}

	nsDir := n.ResourceDir(name)
	for _, sub := range []string{"apps", "secrets", "configmaps"} {
		subPath := filepath.Join(nsDir, sub)

		exists, err := n.FS.Exists(subPath)
		if err != nil {
			rep.Warnf(resourceID, "subdir-check", "cannot check %s/ directory: %v", sub, err)
			continue
		}

		if !exists {
			rep.Warnf(resourceID, "missing-subdir",
				"%s/ subdirectory is missing (run 'whctl create namespace %s' to repair)", sub, name)
		}
	}
}
