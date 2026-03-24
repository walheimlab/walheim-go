package v1alpha1

import metav1 "github.com/walheimlab/walheim-go/pkg/api/meta/v1"

// Job is the typed representation of a job manifest.
type Job struct {
	metav1.TypeMeta   `yaml:",inline"`
	metav1.ObjectMeta `yaml:"metadata"`
	Spec              JobSpec `yaml:"spec"`
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
