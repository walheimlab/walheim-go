package labels

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/walheimlab/walheim-go/internal/fs"
	"github.com/walheimlab/walheim-go/internal/registry"
	"github.com/walheimlab/walheim-go/internal/resource"
)

// validKeyRe matches acceptable label key characters.
var validKeyRe = regexp.MustCompile(`^[a-zA-Z0-9._/\-]+$`)

// validateKey returns an error if key contains unsafe characters or segments.
func validateKey(key string) error {
	if !validKeyRe.MatchString(key) {
		return fmt.Errorf("invalid label key %q: must match ^[a-zA-Z0-9._/-]+$", key)
	}
	if strings.Contains(key, "..") {
		return fmt.Errorf("invalid label key %q: must not contain '..' segments", key)
	}
	return nil
}

// resolveManifestPath returns the path to the resource's manifest file
// using the registry to determine scope.
func resolveManifestPath(filesystem fs.FS, dataDir, kind, name, namespace string) (string, error) {
	entry := registry.Get(kind)
	if entry == nil {
		return "", fmt.Errorf("unknown resource kind %q", kind)
	}

	reg := entry.Registration

	if entry.IsCluster() {
		if namespace != "" {
			return "", fmt.Errorf("resource kind %q is cluster-scoped; do not pass --namespace", kind)
		}
		// Use ClusterBase path formula
		cb := &resource.ClusterBase{
			DataDir:          dataDir,
			FS:               filesystem,
			Info:             reg.Info,
			ManifestFilename: manifestFilenameFor(reg.Info.Singular),
		}
		return cb.ManifestPath(name), nil
	}

	// Namespace-scoped
	if namespace == "" {
		return "", fmt.Errorf("resource kind %q is namespace-scoped; --namespace is required", kind)
	}
	nb := &resource.NamespacedBase{
		DataDir:          dataDir,
		FS:               filesystem,
		Info:             reg.Info,
		ManifestFilename: manifestFilenameFor(reg.Info.Singular),
	}
	return nb.ManifestPath(namespace, name), nil
}

// manifestFilenameFor returns the dot-prefixed manifest filename for a singular kind name.
// Convention: .<singular>.yaml
func manifestFilenameFor(singular string) string {
	return "." + singular + ".yaml"
}

// Set applies label specs to a resource manifest file.
// specs: ["key=value", "key2=value2", "removekey-"]
// overwrite: if false, error on existing keys.
func Set(filesystem fs.FS, dataDir, kind, name, namespace string, specs []string, overwrite bool) error {
	manifestPath, err := resolveManifestPath(filesystem, dataDir, kind, name, namespace)
	if err != nil {
		return err
	}

	data, err := filesystem.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("failed to read manifest: %w", err)
	}

	var m resource.Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("failed to parse manifest: %w", err)
	}

	// Ensure metadata map exists
	metadata, ok := m["metadata"].(map[string]any)
	if !ok {
		metadata = make(map[string]any)
		m["metadata"] = metadata
	}

	// Ensure labels map exists
	labelsRaw, ok := metadata["labels"].(map[string]any)
	if !ok {
		labelsRaw = make(map[string]any)
		metadata["labels"] = labelsRaw
	}

	// Apply each spec
	for _, spec := range specs {
		if strings.HasSuffix(spec, "-") {
			// Remove label
			key := strings.TrimSuffix(spec, "-")
			if err := validateKey(key); err != nil {
				return err
			}
			delete(labelsRaw, key)
		} else if idx := strings.IndexByte(spec, '='); idx >= 0 {
			// Set label
			key := spec[:idx]
			value := spec[idx+1:]
			if err := validateKey(key); err != nil {
				return err
			}
			if !overwrite {
				if _, exists := labelsRaw[key]; exists {
					return fmt.Errorf("label %q already exists; use --overwrite to replace", key)
				}
			}
			labelsRaw[key] = value
		} else {
			return fmt.Errorf("invalid label spec %q: must be key=value or key-", spec)
		}
	}

	// Write back
	encoded, err := yaml.Marshal(m)
	if err != nil {
		return fmt.Errorf("failed to marshal manifest: %w", err)
	}

	return filesystem.WriteFile(manifestPath, encoded)
}

// List prints all labels on a resource in "key=value" format, one per line.
func List(filesystem fs.FS, dataDir, kind, name, namespace string) error {
	manifestPath, err := resolveManifestPath(filesystem, dataDir, kind, name, namespace)
	if err != nil {
		return err
	}

	data, err := filesystem.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("failed to read manifest: %w", err)
	}

	var m resource.Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("failed to parse manifest: %w", err)
	}

	metadata, ok := m["metadata"].(map[string]any)
	if !ok {
		return nil
	}
	labelsRaw, ok := metadata["labels"].(map[string]any)
	if !ok {
		return nil
	}

	// Collect and sort keys for deterministic output
	keys := make([]string, 0, len(labelsRaw))
	for k := range labelsRaw {
		keys = append(keys, k)
	}

	// Sort keys
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[i] > keys[j] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}

	for _, k := range keys {
		fmt.Printf("%s=%v\n", k, labelsRaw[k])
	}

	return nil
}
