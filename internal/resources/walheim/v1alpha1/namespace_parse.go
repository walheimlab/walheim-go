package v1alpha1

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/walheimlab/walheim-go/internal/exitcode"
	"github.com/walheimlab/walheim-go/internal/resource"
	apiv1alpha1 "github.com/walheimlab/walheim-go/pkg/api/walheim/v1alpha1"
)

func validateNamespaceManifest(m *apiv1alpha1.Namespace) error {
	if want := namespaceKind.APIVersion(); m.APIVersion != want {
		return fmt.Errorf("invalid apiVersion: expected %q, got %q", want, m.APIVersion)
	}

	if m.Kind != namespaceKind.Kind {
		return fmt.Errorf("invalid kind: expected %q, got %q", namespaceKind.Kind, m.Kind)
	}

	if m.Spec.Hostname == "" {
		return fmt.Errorf("spec.hostname is required")
	}

	return nil
}

func (n *Namespace) parseManifest(name string) (*apiv1alpha1.Namespace, error) {
	data, err := n.ReadBytes(name)
	if err != nil {
		return nil, err
	}

	var m apiv1alpha1.Namespace
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse namespace %q: %w", name, err)
	}

	return &m, nil
}

func namespaceToMeta(name string, m *apiv1alpha1.Namespace) resource.ResourceMeta {
	h := m.Spec.Hostname
	if h == "" {
		h = "N/A"
	}

	return resource.ResourceMeta{
		Name:   name,
		Labels: m.Labels,
		Summary: map[string]string{
			"HOSTNAME": h,
			"USERNAME": m.Spec.UsernameDisplay(),
			"BASE DIR": m.Spec.BaseDirDisplay(),
		},
		Raw: m,
	}
}

func (n *Namespace) getOne(name string) (resource.ResourceMeta, *apiv1alpha1.Namespace, error) {
	exists, err := n.Exists(name)
	if err != nil {
		return resource.ResourceMeta{}, nil, err
	}

	if !exists {
		return resource.ResourceMeta{}, nil,
			exitcode.New(exitcode.NotFound, fmt.Errorf("namespace %q not found", name))
	}

	m, err := n.parseManifest(name)
	if err != nil {
		return resource.ResourceMeta{}, nil, err
	}

	return namespaceToMeta(name, m), m, nil
}

func (n *Namespace) listAll() ([]resource.ResourceMeta, error) {
	names, err := n.ListNames()
	if err != nil {
		return nil, err
	}

	items := make([]resource.ResourceMeta, 0, len(names))
	for _, name := range names {
		m, err := n.parseManifest(name)
		if err != nil {
			return nil, err
		}

		items = append(items, namespaceToMeta(name, m))
	}

	return items, nil
}
