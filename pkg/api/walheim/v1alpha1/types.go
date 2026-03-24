package v1alpha1

import (
	"strings"

	"gopkg.in/yaml.v3"
)

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

// NamedRef is a reference to a named resource (Secret or ConfigMap).
type NamedRef struct {
	Name string `yaml:"name"`
}

// EnvFromEntry injects all keys from a Secret or ConfigMap into services.
type EnvFromEntry struct {
	SecretRef    *NamedRef `yaml:"secretRef,omitempty"`
	ConfigMapRef *NamedRef `yaml:"configMapRef,omitempty"`
	// ServiceNames restricts injection to the named services.
	// Empty or absent means all services.
	ServiceNames []string `yaml:"serviceNames,omitempty"`
}

// EnvEntry injects a single environment variable into services.
type EnvEntry struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
	// ServiceNames restricts injection to the named services.
	// Empty or absent means all services.
	ServiceNames []string `yaml:"serviceNames,omitempty"`
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
		var m map[string]string
		if err := value.Decode(&m); err != nil {
			return err
		}

		e.Values = m
	case yaml.SequenceNode:
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
