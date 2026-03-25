package v1alpha1

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/walheimlab/walheim-go/internal/exitcode"
	"github.com/walheimlab/walheim-go/internal/output"
	"github.com/walheimlab/walheim-go/internal/resource"
	apiv1alpha1 "github.com/walheimlab/walheim-go/pkg/api/walheim/v1alpha1"
)

func validateJobManifest(m *apiv1alpha1.Job, namespace, name string) error {
	if m.APIVersion != jobKind.APIVersion() {
		return fmt.Errorf("invalid apiVersion: expected %q, got %q", jobKind.APIVersion(), m.APIVersion)
	}

	if m.Kind != jobKind.Kind {
		return fmt.Errorf("invalid kind: expected %q, got %q", jobKind.Kind, m.Kind)
	}

	if m.Name != name {
		return fmt.Errorf("metadata.name %q does not match argument %q", m.Name, name)
	}

	if m.Namespace != namespace {
		return fmt.Errorf("metadata.namespace %q does not match -n %q", m.Namespace, namespace)
	}

	if m.Spec.Image == "" {
		return fmt.Errorf("spec.image is required")
	}

	return nil
}

func (j *Job) parseManifest(namespace, name string) (*apiv1alpha1.Job, error) {
	data, err := j.ReadBytes(namespace, name)
	if err != nil {
		return nil, err
	}

	var m apiv1alpha1.Job
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse job %q in namespace %q: %w", name, namespace, err)
	}

	return &m, nil
}

func jobToMeta(namespace, name string, m *apiv1alpha1.Job, status, lastRun string) resource.ResourceMeta {
	return resource.ResourceMeta{
		Namespace: namespace,
		Name:      name,
		Labels:    m.Labels,
		Summary: map[string]string{
			"IMAGE":    m.Spec.Image,
			"STATUS":   status,
			"LAST RUN": lastRun,
		},
		Raw: m,
	}
}

func (j *Job) getOne(namespace, name string) (resource.ResourceMeta, *apiv1alpha1.Job, error) {
	exists, err := j.Exists(namespace, name)
	if err != nil {
		return resource.ResourceMeta{}, nil, err
	}

	if !exists {
		return resource.ResourceMeta{}, nil,
			exitcode.New(exitcode.NotFound, fmt.Errorf("job %q not found in namespace %q", name, namespace))
	}

	m, err := j.parseManifest(namespace, name)
	if err != nil {
		return resource.ResourceMeta{}, nil, err
	}

	return jobToMeta(namespace, name, m, "Configured", "-"), m, nil
}

func (j *Job) listNamespace(namespace string) ([]*apiv1alpha1.Job, []string, error) {
	names, err := j.ListNames(namespace)
	if err != nil {
		return nil, nil, err
	}

	var (
		manifests  []*apiv1alpha1.Job
		validNames []string
	)

	for _, name := range names {
		m, err := j.parseManifest(namespace, name)
		if err != nil {
			output.Warnf("skipping job %q in namespace %q: %v", name, namespace, err)
			continue
		}

		manifests = append(manifests, m)
		validNames = append(validNames, name)
	}

	return manifests, validNames, nil
}
