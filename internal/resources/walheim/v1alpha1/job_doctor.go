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

func (j *Job) runDoctor(opts registry.OperationOpts) error {
	jsonMode := opts.Output == "json"
	rep := &doctor.Report{}

	names, err := j.ListNames(opts.Namespace)
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	if opts.Name != "" {
		exists, err := j.Exists(opts.Namespace, opts.Name)
		if err != nil {
			return exitErr(exitcode.Failure, err)
		}

		if !exists {
			msg := fmt.Sprintf("job %q not found in namespace %q", opts.Name, opts.Namespace)
			output.Errorf(jsonMode, "NotFound", msg, "", nil, false)

			return exitErr(exitcode.NotFound, fmt.Errorf("%s", msg))
		}

		names = []string{opts.Name}
	}

	for _, name := range names {
		j.doctorJob(rep, opts.Namespace, name)
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

func (j *Job) doctorJob(rep *doctor.Report, namespace, name string) {
	resourceID := fmt.Sprintf("job/%s/%s", namespace, name)

	data, err := j.ReadBytes(namespace, name)
	if err != nil {
		rep.Errorf(resourceID, "manifest-unreadable", "cannot read manifest: %v", err)
		return
	}

	var m apiv1alpha1.Job
	if err := yaml.Unmarshal(data, &m); err != nil {
		rep.Errorf(resourceID, "manifest-parse", "manifest YAML is invalid: %v", err)
		return
	}

	doctor.CheckAPIVersion(rep, resourceID, m.APIVersion, jobKind.APIVersion())
	doctor.CheckKind(rep, resourceID, m.Kind, jobKind.Kind)
	doctor.CheckDirNameMatchesMetadataName(rep, resourceID, name, m.Name)

	if m.Spec.Image == "" {
		rep.Errorf(resourceID, "missing-image", "spec.image is required but not set")
	}
}
