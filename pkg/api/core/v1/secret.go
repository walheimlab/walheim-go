package v1

import metav1 "github.com/walheimlab/walheim-go/pkg/api/meta/v1"

// Secret holds key-value pairs used to inject sensitive configuration.
type Secret struct {
	metav1.TypeMeta   `yaml:",inline"`
	metav1.ObjectMeta `yaml:"metadata"`
	// Data holds base64-encoded key/value pairs.
	Data map[string]string `yaml:"data,omitempty"`
	// StringData holds plaintext key/value pairs.
	// On merge, StringData takes precedence over Data for the same key.
	StringData map[string]string `yaml:"stringData,omitempty"`
}
