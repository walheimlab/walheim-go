// Package v1alpha1 contains all walheim/v1alpha1 resource kinds.
package v1alpha1

import (
	"strings"

	"gopkg.in/yaml.v3"
)

// ResourceMetadata is the common metadata block for all resource manifests.
type ResourceMetadata struct {
	Name      string            `yaml:"name"`
	Namespace string            `yaml:"namespace,omitempty"`
	Labels    map[string]string `yaml:"labels,omitempty"`
}

// ── Namespace ─────────────────────────────────────────────────────────────────

// NamespaceManifest is the typed representation of a .namespace.yaml file.
type NamespaceManifest struct {
	APIVersion string           `yaml:"apiVersion"`
	Kind       string           `yaml:"kind"`
	Metadata   ResourceMetadata `yaml:"metadata"`
	Spec       NamespaceSpec    `yaml:"spec"`
	// Status is populated at runtime by describe and never written to storage.
	Status *NamespaceStatus `yaml:"-" json:"status,omitempty"`
}

// NamespaceStatus holds the runtime (non-persisted) status of a Namespace.
// It is populated during describe and never written to storage.
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

// NamespaceSpec holds the Namespace-specific fields.
type NamespaceSpec struct {
	Hostname string `yaml:"hostname"`
	Username string `yaml:"username,omitempty"`
	// BaseDir is the root directory on the remote host where Walheim stores its
	// data. Defaults to /data/walheim when empty. Changing this after initial
	// deployment requires manually migrating remote files.
	BaseDir string `yaml:"baseDir,omitempty"`
}

// DefaultRemoteBaseDir is the remote base directory used when spec.baseDir is
// not set. It matches the Ruby implementation's hardcoded path.
const DefaultRemoteBaseDir = "/data/walheim"

// remoteBaseDir returns the effective remote base directory, falling back to
// DefaultRemoteBaseDir when the field is not set in the manifest.
func (s NamespaceSpec) remoteBaseDir() string {
	if s.BaseDir != "" {
		return s.BaseDir
	}

	return DefaultRemoteBaseDir
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

func (s NamespaceSpec) baseDirDisplay() string {
	if s.BaseDir != "" {
		return s.BaseDir
	}

	return DefaultRemoteBaseDir + " (default)"
}

// ── App ───────────────────────────────────────────────────────────────────────

// AppManifest is the typed representation of a .app.yaml file.
type AppManifest struct {
	APIVersion string           `yaml:"apiVersion"`
	Kind       string           `yaml:"kind"`
	Metadata   ResourceMetadata `yaml:"metadata"`
	Spec       AppSpec          `yaml:"spec"`
	// Status is populated at runtime and never written to storage.
	Status *AppStatus `yaml:"status,omitempty" json:"status,omitempty"`
}

// AppStatus holds the runtime (non-persisted) status of an App.
// It is populated during describe and never written to storage.
type AppStatus struct {
	State     string `json:"state" yaml:"state"`
	Ready     string `json:"ready" yaml:"ready"`
	Deployed  bool   `json:"deployed" yaml:"deployed"`
	ComposePS string `json:"composePS,omitempty" yaml:"composePS,omitempty"`
}

// AppSpec holds the App-specific fields.
type AppSpec struct {
	// Compose holds the raw docker-compose structure.
	// It is intentionally map[string]any because it mirrors an arbitrary
	// user-authored docker-compose file whose service definitions, volume
	// mounts, network configs, etc. are open-ended. Structured fields that
	// Walheim cares about (envFrom, env) are separate typed fields below.
	Compose ComposeSpec    `yaml:"compose"`
	EnvFrom []EnvFromEntry `yaml:"envFrom,omitempty"`
	Env     []EnvEntry     `yaml:"env,omitempty"`
	Mounts  []MountEntry   `yaml:"mounts,omitempty"`
}

// ComposeSpec is the raw docker-compose document.
// Services is the only field Walheim reads programmatically; everything else
// (volumes, networks, configs, etc.) is passed through verbatim.
type ComposeSpec struct {
	Services map[string]ComposeService `yaml:"services,omitempty"`
	// Extra holds all other top-level compose keys (volumes, networks, …)
	// preserved verbatim on round-trip.
	Extra map[string]any `yaml:",inline"`
}

// ComposeService is a single service entry in a docker-compose file.
// Walheim reads Image, Environment, and Labels; everything else is passed
// through verbatim via Extra.
type ComposeService struct {
	Image       string        `yaml:"image,omitempty"`
	Environment ServiceEnv    `yaml:"environment,omitempty"`
	Labels      ServiceLabels `yaml:"labels,omitempty"`
	// Extra holds all other service keys (ports, volumes, depends_on, …)
	// preserved verbatim on round-trip.
	Extra map[string]any `yaml:",inline"`
}

// ServiceEnv represents the docker-compose environment field.
// Docker Compose accepts both list form (["KEY=val"]) and map form ({KEY: val}).
// yaml.v3 unmarshals whichever form the user wrote; we normalise to map on use.
type ServiceEnv struct {
	Values map[string]string
}

func (e *ServiceEnv) UnmarshalYAML(value *yaml.Node) error {
	e.Values = make(map[string]string)

	switch value.Kind {
	case yaml.MappingNode:
		// map form: {KEY: value}
		var m map[string]string
		if err := value.Decode(&m); err != nil {
			return err
		}

		e.Values = m
	case yaml.SequenceNode:
		// list form: ["KEY=value", "KEY2=value2"]
		var list []string
		if err := value.Decode(&list); err != nil {
			return err
		}

		for _, item := range list {
			if idx := strings.IndexByte(item, '='); idx >= 0 {
				e.Values[item[:idx]] = item[idx+1:]
			} else {
				e.Values[item] = ""
			}
		}
	}

	return nil
}

func (e ServiceEnv) MarshalYAML() (interface{}, error) {
	// Always marshal back as map form for consistency.
	return e.Values, nil
}

// ServiceLabels represents the docker-compose labels field.
// Accepts both list form (["key=val"]) and map form ({key: val}).
type ServiceLabels struct {
	Values map[string]string
}

func (l *ServiceLabels) UnmarshalYAML(value *yaml.Node) error {
	l.Values = make(map[string]string)

	switch value.Kind {
	case yaml.MappingNode:
		var m map[string]string
		if err := value.Decode(&m); err != nil {
			return err
		}

		l.Values = m
	case yaml.SequenceNode:
		var list []string
		if err := value.Decode(&list); err != nil {
			return err
		}

		for _, item := range list {
			if idx := strings.IndexByte(item, '='); idx >= 0 {
				l.Values[item[:idx]] = item[idx+1:]
			} else {
				l.Values[item] = ""
			}
		}
	}

	return nil
}

func (l ServiceLabels) MarshalYAML() (interface{}, error) {
	return l.Values, nil
}

// MountEntry mounts all keys from a Secret or ConfigMap as individual files
// into a directory inside the container.
type MountEntry struct {
	SecretRef    *NamedRef `yaml:"secretRef,omitempty"`
	ConfigMapRef *NamedRef `yaml:"configMapRef,omitempty"`
	// MountPath is the directory inside the container where each key becomes a file.
	MountPath string `yaml:"mountPath"`
	// ServiceNames restricts mounting to named services. Empty means all services.
	ServiceNames []string `yaml:"serviceNames,omitempty"`
}

// EnvFromEntry injects all keys from a Secret or ConfigMap into services.
type EnvFromEntry struct {
	SecretRef    *NamedRef `yaml:"secretRef,omitempty"`
	ConfigMapRef *NamedRef `yaml:"configMapRef,omitempty"`
	// ServiceNames restricts injection to the named services.
	// Empty or absent means all services.
	ServiceNames []string `yaml:"serviceNames,omitempty"`
}

// NamedRef is a reference to a named resource (Secret or ConfigMap).
type NamedRef struct {
	Name string `yaml:"name"`
}

// EnvEntry injects a single environment variable into services.
type EnvEntry struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
	// ServiceNames restricts injection to the named services.
	// Empty or absent means all services.
	ServiceNames []string `yaml:"serviceNames,omitempty"`
}

// ── DaemonSet ─────────────────────────────────────────────────────────────────

// DaemonSetManifest is the typed representation of a .daemonset.yaml file.
type DaemonSetManifest struct {
	APIVersion string           `yaml:"apiVersion"`
	Kind       string           `yaml:"kind"`
	Metadata   ResourceMetadata `yaml:"metadata"`
	Spec       DaemonSetSpec    `yaml:"spec"`
	// Status is populated at runtime and never written to storage.
	Status *DaemonSetStatus `yaml:"status,omitempty" json:"status,omitempty"`
}

// DaemonSetStatus holds the runtime (non-persisted) status of a DaemonSet across namespaces.
// It is populated during describe and never written to storage.
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

// LabelSelector selects resources by matching labels.
// All entries in MatchLabels must be present on the target resource.
type LabelSelector struct {
	MatchLabels map[string]string `yaml:"matchLabels,omitempty"`
}

// Matches reports whether the given labels satisfy this selector.
// A nil or empty selector matches everything.
func (s *LabelSelector) Matches(labels map[string]string) bool {
	if s == nil || len(s.MatchLabels) == 0 {
		return true
	}

	for k, v := range s.MatchLabels {
		if labels[k] != v {
			return false
		}
	}

	return true
}

// ── Job ───────────────────────────────────────────────────────────────────────

// JobManifest is the typed representation of a .job.yaml file.
type JobManifest struct {
	APIVersion string           `yaml:"apiVersion"`
	Kind       string           `yaml:"kind"`
	Metadata   ResourceMetadata `yaml:"metadata"`
	Spec       JobSpec          `yaml:"spec"`
}

// JobSpec holds the Job-specific fields.
type JobSpec struct {
	// Image is the container image to run.
	Image string `yaml:"image"`
	// Command overrides the container entrypoint + args.
	// Empty means the image's default CMD is used.
	Command []string       `yaml:"command,omitempty"`
	EnvFrom []EnvFromEntry `yaml:"envFrom,omitempty"`
	Env     []EnvEntry     `yaml:"env,omitempty"`
	Mounts  []MountEntry   `yaml:"mounts,omitempty"`
}

// ── Secret & ConfigMap (on-disk manifest structs) ─────────────────────────────

// SecretManifest is the typed representation of a .secret.yaml file.
type SecretManifest struct {
	APIVersion string           `yaml:"apiVersion"`
	Kind       string           `yaml:"kind"`
	Metadata   ResourceMetadata `yaml:"metadata"`
	// Data holds base64-encoded key/value pairs.
	Data map[string]string `yaml:"data,omitempty"`
	// StringData holds plaintext key/value pairs.
	// On merge, StringData takes precedence over Data for the same key.
	StringData map[string]string `yaml:"stringData,omitempty"`
}

// ConfigMapManifest is the typed representation of a .configmap.yaml file.
type ConfigMapManifest struct {
	APIVersion string            `yaml:"apiVersion"`
	Kind       string            `yaml:"kind"`
	Metadata   ResourceMetadata  `yaml:"metadata"`
	Data       map[string]string `yaml:"data,omitempty"`
}
