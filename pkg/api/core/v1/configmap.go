package v1

import metav1 "github.com/walheimlab/walheim-go/pkg/api/meta/v1"

// ConfigMap holds non-sensitive key-value configuration.
type ConfigMap struct {
	metav1.TypeMeta   `yaml:",inline"`
	metav1.ObjectMeta `yaml:"metadata"`
	Data              map[string]string `yaml:"data,omitempty"`
}
