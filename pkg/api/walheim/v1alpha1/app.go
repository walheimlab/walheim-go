package v1alpha1

import metav1 "github.com/walheimlab/walheim-go/pkg/api/meta/v1"

// App is the typed representation of an app manifest.
type App struct {
	metav1.TypeMeta   `yaml:",inline"`
	metav1.ObjectMeta `yaml:"metadata"`
	Spec              AppSpec `yaml:"spec"`
	// Status is populated at runtime and never written to storage.
	Status *AppStatus `yaml:"status,omitempty" json:"status,omitempty"`
}

// AppSpec holds the App-specific fields.
type AppSpec struct {
	// Compose holds the raw docker-compose structure.
	Compose ComposeSpec    `yaml:"compose"`
	EnvFrom []EnvFromEntry `yaml:"envFrom,omitempty"`
	Env     []EnvEntry     `yaml:"env,omitempty"`
	Mounts  []MountEntry   `yaml:"mounts,omitempty"`
}

// AppStatus holds the runtime (non-persisted) status of an App.
type AppStatus struct {
	State     string `json:"state" yaml:"state"`
	Ready     string `json:"ready" yaml:"ready"`
	Deployed  bool   `json:"deployed" yaml:"deployed"`
	ComposePS string `json:"composePS,omitempty" yaml:"composePS,omitempty"`
}
