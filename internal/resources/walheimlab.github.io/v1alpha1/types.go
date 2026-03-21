// Package v1alpha1 contains all walheimlab.github.io/v1alpha1 resource kinds.
package v1alpha1

// ResourceMetadata is the common metadata block for all resource manifests.
type ResourceMetadata struct {
	Name   string            `yaml:"name"`
	Labels map[string]string `yaml:"labels,omitempty"`
}

// ── Namespace ─────────────────────────────────────────────────────────────────

// NamespaceManifest is the typed representation of a .namespace.yaml file.
type NamespaceManifest struct {
	APIVersion string           `yaml:"apiVersion"`
	Kind       string           `yaml:"kind"`
	Metadata   ResourceMetadata `yaml:"metadata"`
	Spec       NamespaceSpec    `yaml:"spec"`
}

// NamespaceSpec holds the Namespace-specific fields.
type NamespaceSpec struct {
	Hostname string `yaml:"hostname"`
	Username string `yaml:"username,omitempty"`
}

func (s NamespaceSpec) sshTarget() string {
	if s.Username != "" {
		return s.Username + "@" + s.Hostname
	}
	return s.Hostname
}

func (s NamespaceSpec) usernameDisplay() string {
	if s.Username != "" {
		return s.Username
	}
	return "(from SSH config)"
}

// ── App ───────────────────────────────────────────────────────────────────────

// AppManifest is the typed representation of a .app.yaml file.
// Full spec fields are added in plan-04.
type AppManifest struct {
	APIVersion string           `yaml:"apiVersion"`
	Kind       string           `yaml:"kind"`
	Metadata   ResourceMetadata `yaml:"metadata"`
}
