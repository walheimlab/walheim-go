package resource

// KindInfo describes a resource type's names.
type KindInfo struct {
	Plural   string   // e.g. "namespaces"
	Singular string   // e.g. "namespace"
	Aliases  []string // e.g. ["ns"]
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
