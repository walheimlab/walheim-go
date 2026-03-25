package v1alpha1

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/walheimlab/walheim-go/internal/exitcode"
	"github.com/walheimlab/walheim-go/internal/output"
	"github.com/walheimlab/walheim-go/internal/resource"
	apiv1alpha1 "github.com/walheimlab/walheim-go/pkg/api/walheim/v1alpha1"
)

func validateAppManifest(m *apiv1alpha1.App, namespace, name string) error {
	if m.APIVersion != appKind.APIVersion() {
		return fmt.Errorf("invalid apiVersion: expected %q, got %q", appKind.APIVersion(), m.APIVersion)
	}

	if m.Kind != appKind.Kind {
		return fmt.Errorf("invalid kind: expected %q, got %q", appKind.Kind, m.Kind)
	}

	if m.Name != name {
		return fmt.Errorf("metadata.name %q does not match argument %q", m.Name, name)
	}

	if m.Namespace != namespace {
		return fmt.Errorf("metadata.namespace %q does not match -n %q", m.Namespace, namespace)
	}

	if len(m.Spec.Compose.Services) == 0 {
		return fmt.Errorf("spec.compose.services must define at least one service")
	}

	return nil
}

func (a *App) parseManifest(namespace, name string) (*apiv1alpha1.App, error) {
	data, err := a.ReadBytes(namespace, name)
	if err != nil {
		return nil, err
	}

	var m apiv1alpha1.App
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse app %q in namespace %q: %w", name, namespace, err)
	}

	return &m, nil
}

func appToMeta(namespace, name string, m *apiv1alpha1.App, status, ready string) resource.ResourceMeta {
	img := "N/A"

	for _, svc := range m.Spec.Compose.Services {
		if svc.Image != "" {
			img = svc.Image
		}

		break
	}

	return resource.ResourceMeta{
		Namespace: namespace,
		Name:      name,
		Labels:    m.Labels,
		Summary: map[string]string{
			"IMAGE":  img,
			"READY":  ready,
			"STATUS": status,
		},
		Raw: m,
	}
}

func (a *App) getOne(namespace, name string) (resource.ResourceMeta, *apiv1alpha1.App, error) {
	exists, err := a.Exists(namespace, name)
	if err != nil {
		return resource.ResourceMeta{}, nil, err
	}

	if !exists {
		return resource.ResourceMeta{}, nil,
			exitcode.New(exitcode.NotFound, fmt.Errorf("app %q not found in namespace %q", name, namespace))
	}

	m, err := a.parseManifest(namespace, name)
	if err != nil {
		return resource.ResourceMeta{}, nil, err
	}

	return appToMeta(namespace, name, m, "Configured", "-"), m, nil
}

func (a *App) listNamespace(namespace string) ([]*apiv1alpha1.App, []string, error) {
	names, err := a.ListNames(namespace)
	if err != nil {
		return nil, nil, err
	}

	manifests := make([]*apiv1alpha1.App, 0, len(names))
	validNames := make([]string, 0, len(names))

	for _, name := range names {
		m, err := a.parseManifest(namespace, name)
		if err != nil {
			output.Warnf("skipping app %q in namespace %q: %v", name, namespace, err)
			continue
		}

		manifests = append(manifests, m)
		validNames = append(validNames, name)
	}

	return manifests, validNames, nil
}
