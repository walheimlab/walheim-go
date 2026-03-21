package resource

// KindInfo describes a resource type's names.
type KindInfo struct {
	Plural   string   // e.g. "namespaces"
	Singular string   // e.g. "namespace"
	Aliases  []string // e.g. ["ns"]
}

// Manifest is a parsed YAML document.
type Manifest map[string]any

// SummaryField extracts a single display value from a parsed manifest.
// Used to populate the extra columns in table output.
type SummaryField func(m Manifest) string

// ResourceMeta is what every list/get operation returns per resource.
type ResourceMeta struct {
	Namespace string            // empty for cluster-scoped resources
	Name      string
	Labels    map[string]string // from metadata.labels
	Summary   map[string]string // computed by SummaryField functions
	Raw       Manifest          // full parsed manifest
}

// Handler is the minimal interface every resource package satisfies.
// The framework only uses it as a typed token; operations receive it
// and cast to their concrete type.
type Handler interface {
	KindInfo() KindInfo
}
