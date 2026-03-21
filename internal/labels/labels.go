package labels

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/walheimlab/walheim-go/internal/fs"
	"github.com/walheimlab/walheim-go/internal/registry"
	"github.com/walheimlab/walheim-go/internal/resource"
)

// validKeyRe matches acceptable label key characters.
var validKeyRe = regexp.MustCompile(`^[a-zA-Z0-9._/\-]+$`)

func validateKey(key string) error {
	if !validKeyRe.MatchString(key) {
		return fmt.Errorf("invalid label key %q: must match ^[a-zA-Z0-9._/-]+$", key)
	}
	if strings.Contains(key, "..") {
		return fmt.Errorf("invalid label key %q: must not contain '..' segments", key)
	}
	return nil
}

// resolveManifestPath returns the path to the resource's manifest file.
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
		cb := &resource.ClusterBase{
			DataDir:          dataDir,
			FS:               filesystem,
			Info:             reg.Info,
			ManifestFilename: "." + reg.Info.Singular() + ".yaml",
		}
		return cb.ManifestPath(name), nil
	}

	if namespace == "" {
		return "", fmt.Errorf("resource kind %q is namespace-scoped; --namespace is required", kind)
	}
	nb := &resource.NamespacedBase{
		DataDir:          dataDir,
		FS:               filesystem,
		Info:             reg.Info,
		ManifestFilename: "." + reg.Info.Singular() + ".yaml",
	}
	return nb.ManifestPath(namespace, name), nil
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

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("failed to parse manifest: %w", err)
	}

	labelsNode, err := ensureLabelsNode(&doc)
	if err != nil {
		return err
	}

	// Read existing labels from the node into a typed map.
	existing := make(map[string]string)
	if err := labelsNode.Decode(&existing); err != nil {
		return fmt.Errorf("failed to decode labels: %w", err)
	}

	for _, spec := range specs {
		if strings.HasSuffix(spec, "-") {
			key := strings.TrimSuffix(spec, "-")
			if err := validateKey(key); err != nil {
				return err
			}
			delete(existing, key)
		} else if idx := strings.IndexByte(spec, '='); idx >= 0 {
			key, value := spec[:idx], spec[idx+1:]
			if err := validateKey(key); err != nil {
				return err
			}
			if !overwrite {
				if _, exists := existing[key]; exists {
					return fmt.Errorf("label %q already exists; use --overwrite to replace", key)
				}
			}
			existing[key] = value
		} else {
			return fmt.Errorf("invalid label spec %q: must be key=value or key-", spec)
		}
	}

	// Write the updated map[string]string back into the node.
	updated, err := yaml.Marshal(existing)
	if err != nil {
		return fmt.Errorf("failed to marshal labels: %w", err)
	}
	var updatedNode yaml.Node
	if err := yaml.Unmarshal(updated, &updatedNode); err != nil {
		return fmt.Errorf("failed to build labels node: %w", err)
	}
	// updatedNode is a document node; its Content[0] is the mapping node.
	*labelsNode = *updatedNode.Content[0]

	encoded, err := yaml.Marshal(&doc)
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

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("failed to parse manifest: %w", err)
	}

	labelsNode := findLabelsNode(&doc)
	if labelsNode == nil {
		return nil // no labels
	}

	var labels map[string]string
	if err := labelsNode.Decode(&labels); err != nil {
		return fmt.Errorf("failed to decode labels: %w", err)
	}

	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		fmt.Printf("%s=%s\n", k, labels[k])
	}
	return nil
}

// ensureLabelsNode navigates to metadata.labels in the YAML document node,
// creating the labels mapping node if it does not exist.
func ensureLabelsNode(doc *yaml.Node) (*yaml.Node, error) {
	root := docRoot(doc)
	if root == nil || root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("manifest is not a YAML mapping")
	}

	metaNode := mappingValue(root, "metadata")
	if metaNode == nil {
		// Create metadata node and append it.
		metaNode = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		root.Content = append(root.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "metadata"},
			metaNode,
		)
	}

	labelsNode := mappingValue(metaNode, "labels")
	if labelsNode == nil {
		labelsNode = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		metaNode.Content = append(metaNode.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "labels"},
			labelsNode,
		)
	}

	return labelsNode, nil
}

// findLabelsNode returns the labels mapping node, or nil if absent.
func findLabelsNode(doc *yaml.Node) *yaml.Node {
	root := docRoot(doc)
	if root == nil {
		return nil
	}
	meta := mappingValue(root, "metadata")
	if meta == nil {
		return nil
	}
	return mappingValue(meta, "labels")
}

// docRoot unwraps a document node to its first content node.
func docRoot(doc *yaml.Node) *yaml.Node {
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		return doc.Content[0]
	}
	return doc
}

// mappingValue returns the value node for key in a YAML mapping node,
// or nil if the key is not present.
func mappingValue(node *yaml.Node, key string) *yaml.Node {
	if node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}
