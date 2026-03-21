package resource

import "strings"

// KindInfo describes a resource type's identity and names.
// Group + Version + Kind form the GVK, matching the Kubernetes convention.
// APIVersion() returns "group/version" (e.g. "walheimlab.github.io/v1alpha1").
type KindInfo struct {
	Group   string   // e.g. "walheimlab.github.io"
	Version string   // e.g. "v1alpha1"
	Kind    string   // e.g. "Namespace" (PascalCase, singular)
	Plural  string   // e.g. "namespaces"
	Aliases []string // e.g. ["ns"]
}

// Singular returns the lowercase singular CLI name derived from Kind.
// "Namespace" → "namespace", "ConfigMap" → "configmap".
// Used as the CLI argument and manifest filename prefix (e.g. ".namespace.yaml").
func (k KindInfo) Singular() string {
	return strings.ToLower(k.Kind)
}

// APIVersion returns the "group/version" string written in manifest apiVersion fields.
func (k KindInfo) APIVersion() string {
	return k.Group + "/" + k.Version
}

// ResourceMeta is what every list/get operation returns per resource.
// Raw holds the typed manifest struct for the resource (e.g. *NamespaceManifest).
// The output layer marshals Raw back to YAML for single-resource display.
type ResourceMeta struct {
	Namespace string            // empty for cluster-scoped resources
	Name      string
	Labels    map[string]string // from metadata.labels
	Summary   map[string]string // computed by the resource package
	Raw       any               // the typed manifest struct
}

// Handler is the minimal interface every resource package satisfies.
// The framework only uses it as a typed token; operations receive it
// and cast to their concrete type.
type Handler interface {
	KindInfo() KindInfo
}
