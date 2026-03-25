package v1alpha1

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/walheimlab/walheim-go/internal/exitcode"
	"github.com/walheimlab/walheim-go/internal/output"
	"github.com/walheimlab/walheim-go/internal/resource"
	apiv1alpha1 "github.com/walheimlab/walheim-go/pkg/api/walheim/v1alpha1"
)

func validateDaemonSetManifest(m *apiv1alpha1.DaemonSet, name string) error {
	if m.APIVersion != daemonSetKind.APIVersion() {
		return fmt.Errorf("invalid apiVersion: expected %q, got %q", daemonSetKind.APIVersion(), m.APIVersion)
	}

	if m.Kind != daemonSetKind.Kind {
		return fmt.Errorf("invalid kind: expected %q, got %q", daemonSetKind.Kind, m.Kind)
	}

	if m.Name != name {
		return fmt.Errorf("metadata.name %q does not match argument %q", m.Name, name)
	}

	if len(m.Spec.Compose.Services) == 0 {
		return fmt.Errorf("spec.compose.services must define at least one service")
	}

	return nil
}

func (d *DaemonSet) parseManifest(name string) (*apiv1alpha1.DaemonSet, error) {
	data, err := d.ReadBytes(name)
	if err != nil {
		return nil, err
	}

	var m apiv1alpha1.DaemonSet
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse daemonset %q: %w", name, err)
	}

	return &m, nil
}

func daemonSetToMeta(name string, m *apiv1alpha1.DaemonSet, matchedNS []string) resource.ResourceMeta {
	img := "N/A"

	for _, svc := range m.Spec.Compose.Services {
		if svc.Image != "" {
			img = svc.Image
		}

		break
	}

	nsDisplay := fmt.Sprintf("%d", len(matchedNS))
	if len(matchedNS) > 0 {
		nsDisplay = strings.Join(matchedNS, ",")
	}

	selector := "(all)"

	if m.Spec.NamespaceSelector != nil && len(m.Spec.NamespaceSelector.MatchLabels) != 0 {
		parts := make([]string, 0, len(m.Spec.NamespaceSelector.MatchLabels))
		for k, v := range m.Spec.NamespaceSelector.MatchLabels {
			parts = append(parts, k+"="+v)
		}

		sort.Strings(parts)
		selector = strings.Join(parts, ",")
	}

	return resource.ResourceMeta{
		Name:   name,
		Labels: m.Labels,
		Summary: map[string]string{
			"IMAGE":      img,
			"SELECTOR":   selector,
			"NAMESPACES": nsDisplay,
		},
		Raw: m,
	}
}

func (d *DaemonSet) getOne(name string) (resource.ResourceMeta, *apiv1alpha1.DaemonSet, error) {
	exists, err := d.Exists(name)
	if err != nil {
		return resource.ResourceMeta{}, nil, err
	}

	if !exists {
		return resource.ResourceMeta{}, nil,
			exitcode.New(exitcode.NotFound, fmt.Errorf("daemonset %q not found", name))
	}

	m, err := d.parseManifest(name)
	if err != nil {
		return resource.ResourceMeta{}, nil, err
	}

	_, nsNames, _ := matchingNamespaces(m.Spec.NamespaceSelector, d.FS, d.DataDir)

	return daemonSetToMeta(name, m, nsNames), m, nil
}

func (d *DaemonSet) listAll() ([]resource.ResourceMeta, error) {
	names, err := d.ListNames()
	if err != nil {
		return nil, err
	}

	items := make([]resource.ResourceMeta, 0, len(names))
	for _, name := range names {
		m, err := d.parseManifest(name)
		if err != nil {
			output.Warnf("skipping daemonset %q: %v", name, err)
			continue
		}

		_, nsNames, _ := matchingNamespaces(m.Spec.NamespaceSelector, d.FS, d.DataDir)
		items = append(items, daemonSetToMeta(name, m, nsNames))
	}

	return items, nil
}
