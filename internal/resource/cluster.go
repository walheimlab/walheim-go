package resource

import (
	"fmt"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/walheimlab/walheim-go/internal/fs"
)

// ClusterBase provides path resolution and raw I/O for cluster-scoped resources.
// Path formula: <dataDir>/<plural>/<name>/<manifestFilename>
//
// It deliberately does NOT parse manifests or compute summaries — that belongs
// in each resource package, using typed structs.
type ClusterBase struct {
	DataDir          string
	FS               fs.FS
	Info             KindInfo
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

// ReadBytes returns the raw bytes of the manifest file.
func (b *ClusterBase) ReadBytes(name string) ([]byte, error) {
	data, err := b.FS.ReadFile(b.ManifestPath(name))
	if err != nil {
		return nil, fmt.Errorf("read %s/%s: %w", b.Info.Plural, name, err)
	}
	return data, nil
}

// WriteManifest YAML-encodes v (any typed struct) and writes it atomically.
func (b *ClusterBase) WriteManifest(name string, v any) error {
	encoded, err := yaml.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal %s/%s: %w", b.Info.Plural, name, err)
	}
	if err := b.FS.WriteFile(b.ManifestPath(name), encoded); err != nil {
		return fmt.Errorf("write %s/%s: %w", b.Info.Plural, name, err)
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

// ListNames returns the names of all resources of this kind that have a
// manifest file. Directories without the manifest are silently skipped.
func (b *ClusterBase) ListNames() ([]string, error) {
	baseDir := filepath.Join(b.DataDir, b.Info.Plural)

	entries, err := b.FS.ReadDir(baseDir)
	if err != nil {
		exists, existsErr := b.FS.Exists(baseDir)
		if existsErr != nil {
			return nil, existsErr
		}
		if !exists {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s dir: %w", b.Info.Plural, err)
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
