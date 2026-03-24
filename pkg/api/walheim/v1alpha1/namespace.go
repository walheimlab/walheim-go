package v1alpha1

import metav1 "github.com/walheimlab/walheim-go/pkg/api/meta/v1"

// DefaultRemoteBaseDir is the remote base directory used when spec.baseDir is
// not set.
const DefaultRemoteBaseDir = "/data/walheim"

// Namespace describes a remote host that Walheim manages.
type Namespace struct {
	metav1.TypeMeta   `yaml:",inline"`
	metav1.ObjectMeta `yaml:"metadata"`
	Spec              NamespaceSpec `yaml:"spec"`
	// Status is populated at runtime by describe and never written to storage.
	Status *NamespaceStatus `yaml:"-" json:"status,omitempty"`
}

// NamespaceSpec holds the Namespace-specific fields.
type NamespaceSpec struct {
	Hostname string `yaml:"hostname"`
	Username string `yaml:"username,omitempty"`
	// BaseDir is the root directory on the remote host where Walheim stores its
	// data. Defaults to /data/walheim when empty.
	BaseDir string `yaml:"baseDir,omitempty"`
}

// RemoteBaseDir returns the effective remote base directory.
func (s NamespaceSpec) RemoteBaseDir() string {
	if s.BaseDir != "" {
		return s.BaseDir
	}
	return DefaultRemoteBaseDir
}

// SSHTarget returns the SSH connection target (user@host or host).
func (s NamespaceSpec) SSHTarget() string {
	if s.Username != "" {
		return s.Username + "@" + s.Hostname
	}
	return s.Hostname
}

// UsernameDisplay returns a human-readable username or a fallback message.
func (s NamespaceSpec) UsernameDisplay() string {
	if s.Username != "" {
		return s.Username
	}
	return "(from SSH config)"
}

// BaseDirDisplay returns a human-readable base dir with default annotation.
func (s NamespaceSpec) BaseDirDisplay() string {
	if s.BaseDir != "" {
		return s.BaseDir
	}
	return DefaultRemoteBaseDir + " (default)"
}

// NamespaceStatus holds the runtime (non-persisted) status of a Namespace.
type NamespaceStatus struct {
	Connection   string                     `json:"connection" yaml:"connection"`
	Docker       *NamespaceDockerStatus     `json:"docker,omitempty" yaml:"docker,omitempty"`
	DeployedApps []NamespaceDeployedApp     `json:"deployedApps,omitempty" yaml:"deployedApps,omitempty"`
	Containers   []NamespaceContainerStatus `json:"containers,omitempty" yaml:"containers,omitempty"`
	Resources    NamespaceResourceCounts    `json:"resources" yaml:"resources"`
	Usage        *NamespaceUsage            `json:"usage,omitempty" yaml:"usage,omitempty"`
}

// NamespaceDockerStatus reports whether Docker is available on the host and its version.
type NamespaceDockerStatus struct {
	Available bool   `json:"available" yaml:"available"`
	Version   string `json:"version,omitempty" yaml:"version,omitempty"`
}

// NamespaceDeployedApp summarises containers for a single walheim-managed app.
type NamespaceDeployedApp struct {
	Name    string `json:"name" yaml:"name"`
	State   string `json:"state" yaml:"state"`
	Running int    `json:"running" yaml:"running"`
	Total   int    `json:"total" yaml:"total"`
}

// NamespaceContainerStatus holds the runtime status of a single container on
// the namespace host. Management is one of "managed", "unmanaged", or "orphan".
type NamespaceContainerStatus struct {
	Name         string `json:"name" yaml:"name"`
	App          string `json:"app,omitempty" yaml:"app,omitempty"`
	State        string `json:"state" yaml:"state"`
	DockerStatus string `json:"dockerStatus" yaml:"dockerStatus"`
	Management   string `json:"management" yaml:"management"`
}

// NamespaceResourceCounts counts local context resources for a namespace.
type NamespaceResourceCounts struct {
	Apps       int `json:"apps" yaml:"apps"`
	Secrets    int `json:"secrets" yaml:"secrets"`
	ConfigMaps int `json:"configmaps" yaml:"configmaps"`
}

// NamespaceUsage holds resource usage metrics for the namespace host.
type NamespaceUsage struct {
	Disk       *NamespaceDiskUsage       `json:"disk,omitempty" yaml:"disk,omitempty"`
	Containers *NamespaceContainerCounts `json:"containers,omitempty" yaml:"containers,omitempty"`
}

// NamespaceDiskUsage reports used and total disk space.
type NamespaceDiskUsage struct {
	Used  string `json:"used" yaml:"used"`
	Total string `json:"total" yaml:"total"`
}

// NamespaceContainerCounts reports running and stopped container counts on the host.
type NamespaceContainerCounts struct {
	Running int `json:"running" yaml:"running"`
	Stopped int `json:"stopped" yaml:"stopped"`
}
