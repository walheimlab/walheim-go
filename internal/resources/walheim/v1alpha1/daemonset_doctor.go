package v1alpha1

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/walheimlab/walheim-go/internal/doctor"
	"github.com/walheimlab/walheim-go/internal/exitcode"
	"github.com/walheimlab/walheim-go/internal/output"
	"github.com/walheimlab/walheim-go/internal/registry"
	apiv1alpha1 "github.com/walheimlab/walheim-go/pkg/api/walheim/v1alpha1"
)

func (d *DaemonSet) runDoctor(opts registry.OperationOpts) error {
	jsonMode := opts.Output == "json"
	rep := &doctor.Report{}

	names, err := d.ListNames()
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	if opts.Name != "" {
		exists, err := d.Exists(opts.Name)
		if err != nil {
			return exitErr(exitcode.Failure, err)
		}

		if !exists {
			msg := fmt.Sprintf("daemonset %q not found", opts.Name)
			output.Errorf(jsonMode, "NotFound", msg, "", nil, false)

			return exitErr(exitcode.NotFound, fmt.Errorf("%s", msg))
		}

		names = []string{opts.Name}
	}

	for _, name := range names {
		d.doctorDaemonSet(rep, name)
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

func (d *DaemonSet) doctorDaemonSet(rep *doctor.Report, name string) {
	resourceID := "daemonset/" + name

	data, err := d.ReadBytes(name)
	if err != nil {
		rep.Errorf(resourceID, "manifest-unreadable", "cannot read manifest: %v", err)
		return
	}

	var m apiv1alpha1.DaemonSet
	if err := yaml.Unmarshal(data, &m); err != nil {
		rep.Errorf(resourceID, "manifest-parse", "manifest YAML is invalid: %v", err)
		return
	}

	doctor.CheckAPIVersion(rep, resourceID, m.APIVersion, daemonSetKind.APIVersion())
	doctor.CheckKind(rep, resourceID, m.Kind, daemonSetKind.Kind)
	doctor.CheckDirNameMatchesMetadataName(rep, resourceID, name, m.Name)

	if len(m.Spec.Compose.Services) == 0 {
		rep.Errorf(resourceID, "missing-services", "spec.compose.services must define at least one service")
	}
}
