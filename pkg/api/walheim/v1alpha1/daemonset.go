package v1alpha1

import metav1 "github.com/walheimlab/walheim-go/pkg/api/meta/v1"

// DaemonSet is the typed representation of a daemonset manifest.
type DaemonSet struct {
	metav1.TypeMeta   `yaml:",inline"`
	metav1.ObjectMeta `yaml:"metadata"`
	Spec              DaemonSetSpec `yaml:"spec"`
	// Status is populated at runtime and never written to storage.
	Status *DaemonSetStatus `yaml:"status,omitempty" json:"status,omitempty"`
}

// DaemonSetSpec holds the DaemonSet-specific fields.
type DaemonSetSpec struct {
	// NamespaceSelector selects which namespaces the DaemonSet runs in by
	// matching labels. A nil or empty selector matches all namespaces.
	NamespaceSelector *LabelSelector `yaml:"namespaceSelector,omitempty"`
	Compose           ComposeSpec    `yaml:"compose"`
	EnvFrom           []EnvFromEntry `yaml:"envFrom,omitempty"`
	Env               []EnvEntry     `yaml:"env,omitempty"`
	Mounts            []MountEntry   `yaml:"mounts,omitempty"`
}

// DaemonSetStatus holds the runtime (non-persisted) status of a DaemonSet across namespaces.
type DaemonSetStatus struct {
	Namespaces []DaemonSetNamespaceStatus `json:"namespaces,omitempty" yaml:"namespaces,omitempty"`
}

// DaemonSetNamespaceStatus holds the status of a DaemonSet in one namespace.
type DaemonSetNamespaceStatus struct {
	Namespace string `json:"namespace" yaml:"namespace"`
	State     string `json:"state" yaml:"state"`
	Ready     string `json:"ready" yaml:"ready"`
	Deployed  bool   `json:"deployed" yaml:"deployed"`
}
