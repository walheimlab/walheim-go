package resource

import (
	"fmt"
	"path/filepath"

	"github.com/walheimlab/walheim-go/internal/fs"
	"github.com/walheimlab/walheim-go/internal/yamlutil"
)

const namespacesPlural = "namespaces"
const namespaceManifest = ".namespace.yaml"

// NamespacedBase provides path resolution and raw I/O for namespaced resources.
// Path formula: <dataDir>/namespaces/<namespace>/<plural>/<name>/<manifestFilename>
//
// It deliberately does NOT parse manifests or compute summaries — that belongs
// in each resource package, using typed structs.
type NamespacedBase struct {
	DataDir          string
	FS               fs.FS
	Info             KindInfo
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

// ReadBytes returns the raw bytes of the manifest file.
func (b *NamespacedBase) ReadBytes(namespace, name string) ([]byte, error) {
	data, err := b.FS.ReadFile(b.ManifestPath(namespace, name))
	if err != nil {
		return nil, fmt.Errorf("read %s/%s in namespace %s: %w", b.Info.Plural, name, namespace, err)
	}

	return data, nil
}

// WriteManifest YAML-encodes v (any typed struct) and writes it atomically.
func (b *NamespacedBase) WriteManifest(namespace, name string, v any) error {
	encoded, err := yamlutil.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal %s/%s in namespace %s: %w", b.Info.Plural, name, namespace, err)
	}

	if err := b.FS.WriteFile(b.ManifestPath(namespace, name), encoded); err != nil {
		return fmt.Errorf("write %s/%s in namespace %s: %w", b.Info.Plural, name, namespace, err)
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

// ListNames returns the names of all resources in namespace that have a manifest file.
func (b *NamespacedBase) ListNames(namespace string) ([]string, error) {
	baseDir := filepath.Join(b.DataDir, namespacesPlural, namespace, b.Info.Plural)

	entries, err := b.FS.ReadDir(baseDir)
	if err != nil {
		exists, existsErr := b.FS.Exists(baseDir)
		if existsErr != nil {
			return nil, existsErr
		}

		if !exists {
			return nil, nil
		}

		return nil, fmt.Errorf("read %s dir in namespace %s: %w", b.Info.Plural, namespace, err)
	}

	var names []string

	for _, entry := range entries {
		manifestPath := filepath.Join(baseDir, entry, b.ManifestFilename)

		exists, err := b.FS.Exists(manifestPath)
		if err != nil {
			return nil, err
		}

		if exists {
			names = append(names, entry)
		}
	}

	return names, nil
}

// ValidNamespaces returns sorted namespace names that have a .namespace.yaml file.
func (b *NamespacedBase) ValidNamespaces() ([]string, error) {
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

		return nil, fmt.Errorf("read namespaces dir: %w", err)
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
