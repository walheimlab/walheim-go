package resource

import (
	"fmt"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/walheimlab/walheim-go/internal/fs"
)

// ClusterBase provides path resolution and generic CRUD for cluster-scoped resources.
// Path formula: <dataDir>/<plural>/<name>/<manifestFilename>
type ClusterBase struct {
	DataDir          string
	FS               fs.FS
	Info             KindInfo
	Fields           map[string]SummaryField
	ManifestFilename string // e.g. ".namespace.yaml"
}

// ResourceDir returns <dataDir>/<plural>/<name>/
func (b *ClusterBase) ResourceDir(name string) string {
	return filepath.Join(b.DataDir, b.Info.Plural, name)
}

// ManifestPath returns the full path to the manifest file.
func (b *ClusterBase) ManifestPath(name string) string {
	return filepath.Join(b.ResourceDir(name), b.ManifestFilename)
}

// Exists reports whether the resource's manifest file exists.
func (b *ClusterBase) Exists(name string) (bool, error) {
	return b.FS.Exists(b.ManifestPath(name))
}

// ReadManifest reads and YAML-parses the manifest.
func (b *ClusterBase) ReadManifest(name string) (Manifest, error) {
	data, err := b.FS.ReadFile(b.ManifestPath(name))
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest for %s/%s: %w", b.Info.Plural, name, err)
	}

	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("failed to parse manifest for %s/%s: %w", b.Info.Plural, name, err)
	}

	return m, nil
}

// WriteManifest YAML-encodes data and writes it atomically via fs.WriteFile.
func (b *ClusterBase) WriteManifest(name string, data Manifest) error {
	encoded, err := yaml.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal manifest for %s/%s: %w", b.Info.Plural, name, err)
	}

	if err := b.FS.WriteFile(b.ManifestPath(name), encoded); err != nil {
		return fmt.Errorf("failed to write manifest for %s/%s: %w", b.Info.Plural, name, err)
	}

	return nil
}

// EnsureDir creates the resource directory tree.
func (b *ClusterBase) EnsureDir(name string) error {
	return b.FS.MkdirAll(b.ResourceDir(name))
}

// RemoveDir removes the resource directory tree.
func (b *ClusterBase) RemoveDir(name string) error {
	return b.FS.RemoveAll(b.ResourceDir(name))
}

// Get returns ResourceMeta for a single resource. Errors if not found.
func (b *ClusterBase) Get(name string) (ResourceMeta, error) {
	exists, err := b.Exists(name)
	if err != nil {
		return ResourceMeta{}, err
	}
	if !exists {
		return ResourceMeta{}, fmt.Errorf("%s %q not found", b.Info.Singular, name)
	}

	m, err := b.ReadManifest(name)
	if err != nil {
		return ResourceMeta{}, err
	}

	return ResourceMeta{
		Name:    name,
		Labels:  extractLabels(m),
		Summary: b.computeSummary(m),
		Raw:     m,
	}, nil
}

// ListAll returns ResourceMeta for every resource of this kind.
// Scans <dataDir>/<plural>/ for subdirectories that contain the manifest file.
func (b *ClusterBase) ListAll() ([]ResourceMeta, error) {
	baseDir := filepath.Join(b.DataDir, b.Info.Plural)

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
		return nil, fmt.Errorf("failed to read %s directory: %w", b.Info.Plural, err)
	}

	var result []ResourceMeta
	for _, entry := range entries {
		manifestPath := filepath.Join(baseDir, entry, b.ManifestFilename)
		exists, err := b.FS.Exists(manifestPath)
		if err != nil {
			return nil, err
		}
		if !exists {
			// Skip directories without a manifest (e.g. .git, temp dirs)
			continue
		}

		meta, err := b.Get(entry)
		if err != nil {
			return nil, err
		}
		result = append(result, meta)
	}

	return result, nil
}

// computeSummary runs all registered SummaryField functions against the manifest.
func (b *ClusterBase) computeSummary(m Manifest) map[string]string {
	summary := make(map[string]string)
	for key, fn := range b.Fields {
		summary[key] = fn(m)
	}
	return summary
}

// extractLabels pulls metadata.labels out of a manifest map.
func extractLabels(m Manifest) map[string]string {
	metadata, ok := m["metadata"].(map[string]any)
	if !ok {
		return nil
	}
	labelsRaw, ok := metadata["labels"].(map[string]any)
	if !ok {
		return nil
	}
	labels := make(map[string]string, len(labelsRaw))
	for k, v := range labelsRaw {
		if s, ok := v.(string); ok {
			labels[k] = s
		}
	}
	return labels
}
