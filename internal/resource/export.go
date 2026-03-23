package resource

// ClusterLister is implemented by cluster-scoped handlers whose base embeds ClusterBase.
// Used by the export command to enumerate and read raw manifests without knowing the concrete type.
type ClusterLister interface {
	ListNames() ([]string, error)
	ReadBytes(name string) ([]byte, error)
}

// NSLister is implemented by namespace-scoped handlers whose base embeds NamespacedBase.
// Used by the export command to enumerate and read raw manifests without knowing the concrete type.
type NSLister interface {
	ValidNamespaces() ([]string, error)
	ListNames(namespace string) ([]string, error)
	ReadBytes(namespace, name string) ([]byte, error)
}
