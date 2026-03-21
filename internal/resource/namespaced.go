package resource

import (
	"fmt"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/walheimlab/walheim-go/internal/fs"
)

const namespacesPlural = "namespaces"
const namespaceManifest = ".namespace.yaml"

// NamespacedBase provides path resolution and generic CRUD for namespaced resources.
// Path formula: <dataDir>/namespaces/<namespace>/<plural>/<name>/<manifestFilename>
type NamespacedBase struct {
	DataDir          string
	FS               fs.FS
	Info             KindInfo
	Fields           map[string]SummaryField
	ManifestFilename string
}

// ResourceDir returns <dataDir>/namespaces/<namespace>/<plural>/<name>/
func (b *NamespacedBase) ResourceDir(namespace, name string) string {
	return filepath.Join(b.DataDir, namespacesPlural, namespace, b.Info.Plural, name)
}

// ManifestPath returns the full path to the manifest file.
func (b *NamespacedBase) ManifestPath(namespace, name string) string {
	return filepath.Join(b.ResourceDir(namespace, name), b.ManifestFilename)
}

// Exists reports whether the resource's manifest file exists.
func (b *NamespacedBase) Exists(namespace, name string) (bool, error) {
	return b.FS.Exists(b.ManifestPath(namespace, name))
}

// ReadManifest reads and YAML-parses the manifest.
func (b *NamespacedBase) ReadManifest(namespace, name string) (Manifest, error) {
	data, err := b.FS.ReadFile(b.ManifestPath(namespace, name))
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest for %s/%s in namespace %s: %w",
			b.Info.Plural, name, namespace, err)
	}

	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("failed to parse manifest for %s/%s in namespace %s: %w",
			b.Info.Plural, name, namespace, err)
	}

	return m, nil
}

// WriteManifest YAML-encodes data and writes it atomically via fs.WriteFile.
func (b *NamespacedBase) WriteManifest(namespace, name string, data Manifest) error {
	encoded, err := yaml.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal manifest for %s/%s in namespace %s: %w",
			b.Info.Plural, name, namespace, err)
	}

	if err := b.FS.WriteFile(b.ManifestPath(namespace, name), encoded); err != nil {
		return fmt.Errorf("failed to write manifest for %s/%s in namespace %s: %w",
			b.Info.Plural, name, namespace, err)
	}

	return nil
}

// EnsureDir creates the resource directory tree.
func (b *NamespacedBase) EnsureDir(namespace, name string) error {
	return b.FS.MkdirAll(b.ResourceDir(namespace, name))
}

// RemoveDir removes the resource directory tree.
func (b *NamespacedBase) RemoveDir(namespace, name string) error {
	return b.FS.RemoveAll(b.ResourceDir(namespace, name))
}

// Get returns ResourceMeta for a single resource. Errors if not found.
func (b *NamespacedBase) Get(namespace, name string) (ResourceMeta, error) {
	exists, err := b.Exists(namespace, name)
	if err != nil {
		return ResourceMeta{}, err
	}
	if !exists {
		return ResourceMeta{}, fmt.Errorf("%s %q not found in namespace %q", b.Info.Singular, name, namespace)
	}

	m, err := b.ReadManifest(namespace, name)
	if err != nil {
		return ResourceMeta{}, err
	}

	return ResourceMeta{
		Namespace: namespace,
		Name:      name,
		Labels:    extractLabels(m),
		Summary:   b.computeSummary(m),
		Raw:       m,
	}, nil
}

// ListNamespace returns ResourceMeta for all resources in one namespace.
func (b *NamespacedBase) ListNamespace(namespace string) ([]ResourceMeta, error) {
	baseDir := filepath.Join(b.DataDir, namespacesPlural, namespace, b.Info.Plural)

	entries, err := b.FS.ReadDir(baseDir)
	if err != nil {
		// If the directory doesn't exist, return empty list
		exists, existsErr := b.FS.Exists(baseDir)
		if existsErr != nil {
			return nil, existsErr
		}
		if !exists {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read %s directory in namespace %s: %w",
			b.Info.Plural, namespace, err)
	}

	var result []ResourceMeta
	for _, entry := range entries {
		manifestPath := filepath.Join(baseDir, entry, b.ManifestFilename)
		exists, err := b.FS.Exists(manifestPath)
		if err != nil {
			return nil, err
		}
		if !exists {
			continue
		}

		meta, err := b.Get(namespace, entry)
		if err != nil {
			return nil, err
		}
		result = append(result, meta)
	}

	return result, nil
}

// ListAll returns ResourceMeta across all valid namespaces.
// A namespace is valid if <dataDir>/namespaces/<entry>/.namespace.yaml exists.
func (b *NamespacedBase) ListAll() ([]ResourceMeta, error) {
	namespaces, err := b.validNamespaces()
	if err != nil {
		return nil, err
	}

	var result []ResourceMeta
	for _, ns := range namespaces {
		items, err := b.ListNamespace(ns)
		if err != nil {
			return nil, err
		}
		result = append(result, items...)
	}

	return result, nil
}

// validNamespaces returns the sorted list of namespace names that have a
// .namespace.yaml manifest. Internal helper for ListAll.
func (b *NamespacedBase) validNamespaces() ([]string, error) {
	nsDir := filepath.Join(b.DataDir, namespacesPlural)

	entries, err := b.FS.ReadDir(nsDir)
	if err != nil {
		exists, existsErr := b.FS.Exists(nsDir)
		if existsErr != nil {
			return nil, existsErr
		}
		if !exists {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read namespaces directory: %w", err)
	}

	var namespaces []string
	for _, entry := range entries {
		manifestPath := filepath.Join(nsDir, entry, namespaceManifest)
		exists, err := b.FS.Exists(manifestPath)
		if err != nil {
			return nil, err
		}
		if exists {
			namespaces = append(namespaces, entry)
		}
	}

	return namespaces, nil
}

// computeSummary runs all registered SummaryField functions against the manifest.
func (b *NamespacedBase) computeSummary(m Manifest) map[string]string {
	summary := make(map[string]string)
	for key, fn := range b.Fields {
		summary[key] = fn(m)
	}
	return summary
}
