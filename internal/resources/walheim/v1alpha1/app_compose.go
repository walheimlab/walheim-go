package v1alpha1

import (
	"encoding/base64"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/walheimlab/walheim-go/internal/fs"
	"github.com/walheimlab/walheim-go/internal/yamlutil"
	apiv1alpha1 "github.com/walheimlab/walheim-go/pkg/api/walheim/v1alpha1"
	corev1 "github.com/walheimlab/walheim-go/pkg/api/core/v1"
)

var varPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

// generateCompose builds the final docker-compose.yml and writes it locally.
// Path: <dataDir>/namespaces/<namespace>/apps/<name>/docker-compose.yml
// NOTE: This function modifies m.Spec.Compose.Services in-place.
func generateCompose(namespace, name string, m *apiv1alpha1.App, filesystem fs.FS, dataDir string) error {
	// Make a working copy of services to avoid mutating the original manifest unexpectedly
	// (we do mutate it, as documented in the plan, but let's be intentional).
	services := m.Spec.Compose.Services
	if services == nil {
		return fmt.Errorf("spec.compose.services is empty")
	}

	// Ensure all service environment and label maps are initialized.
	for svcName, svc := range services {
		if svc.Environment.Values == nil {
			svc.Environment.Values = make(map[string]string)
		}

		if svc.Labels.Values == nil {
			svc.Labels.Values = make(map[string]string)
		}

		services[svcName] = svc
	}

	// Step 1 — Inject Walheim labels into every service.
	for svcName, svc := range services {
		// Strip old walheim.* keys
		for k := range svc.Labels.Values {
			if strings.HasPrefix(k, "walheim.") {
				delete(svc.Labels.Values, k)
			}
		}

		svc.Labels.Values["walheim.managed"] = "true"
		svc.Labels.Values["walheim.namespace"] = namespace
		svc.Labels.Values["walheim.app"] = name
		services[svcName] = svc
	}

	// Audit tracking per service
	// secretInjected[svcName][secretName] = []keys
	secretInjected := map[string]map[string][]string{}
	// configmapInjected[svcName][cmName] = []keys
	configmapInjected := map[string]map[string][]string{}
	// overrideInjected[svcName] = []keys
	overrideInjected := map[string][]string{}

	// Step 2 — Load secrets and configmaps for envFrom (lower precedence — only if key not present).
	for _, entry := range m.Spec.EnvFrom {
		var (
			kvMap      map[string]string
			sourceName string
			isSecret   bool
		)

		if entry.SecretRef != nil {
			var err error

			kvMap, err = loadSecret(namespace, entry.SecretRef.Name, filesystem, dataDir)
			if err != nil {
				return fmt.Errorf("envFrom: %w", err)
			}

			sourceName = entry.SecretRef.Name
			isSecret = true
		} else if entry.ConfigMapRef != nil {
			var err error

			kvMap, err = loadConfigMap(namespace, entry.ConfigMapRef.Name, filesystem, dataDir)
			if err != nil {
				return fmt.Errorf("envFrom: %w", err)
			}

			sourceName = entry.ConfigMapRef.Name
			isSecret = false
		} else {
			continue
		}

		targets := targetServices(services, entry.ServiceNames)
		for _, svcName := range targets {
			svc := services[svcName]
			for k, v := range kvMap {
				if _, exists := svc.Environment.Values[k]; !exists {
					svc.Environment.Values[k] = v
					// Track for audit
					if isSecret {
						if secretInjected[svcName] == nil {
							secretInjected[svcName] = map[string][]string{}
						}

						secretInjected[svcName][sourceName] = append(secretInjected[svcName][sourceName], k)
					} else {
						if configmapInjected[svcName] == nil {
							configmapInjected[svcName] = map[string][]string{}
						}

						configmapInjected[svcName][sourceName] = append(configmapInjected[svcName][sourceName], k)
					}
				}
			}

			services[svcName] = svc
		}
	}

	// Step 3 — Inject env entries (higher precedence — always overwrite).
	for _, entry := range m.Spec.Env {
		targets := targetServices(services, entry.ServiceNames)
		for _, svcName := range targets {
			svc := services[svcName]
			// Substitute ${VAR} in entry.Value using this service's current env
			value := substituteVars(entry.Value, svc.Environment.Values)
			svc.Environment.Values[entry.Name] = value
			overrideInjected[svcName] = append(overrideInjected[svcName], entry.Name)
			services[svcName] = svc
		}
	}

	// Step 4 — Apply audit labels.
	applyAuditLabels(services, overrideInjected, secretInjected, configmapInjected)

	// Write back modified services
	m.Spec.Compose.Services = services

	// Step 5 — Write mount files and inject bind-mount volumes.
	resourceDir := filepath.Join(dataDir, "namespaces", namespace, "apps", name)
	if err := injectComposeMounts(resourceDir, services, m.Spec.Mounts, namespace, filesystem, dataDir); err != nil {
		return err
	}

	// Step 6 — Marshal and write.
	encoded, err := yamlutil.Marshal(m.Spec.Compose)
	if err != nil {
		return fmt.Errorf("marshal docker-compose: %w", err)
	}

	composePath := filepath.Join(resourceDir, "docker-compose.yml")
	if err := filesystem.WriteFile(composePath, encoded); err != nil {
		return fmt.Errorf("write docker-compose.yml: %w", err)
	}

	return nil
}

// applyAuditLabels stamps walheim.injected-env.* labels onto each service
// to record which environment keys were injected and from which source.
func applyAuditLabels(
	services map[string]apiv1alpha1.ComposeService,
	overrideInjected map[string][]string,
	secretInjected map[string]map[string][]string,
	configmapInjected map[string]map[string][]string,
) {
	for svcName, svc := range services {
		// walheim.injected-env.override
		if keys, ok := overrideInjected[svcName]; ok && len(keys) > 0 {
			sorted := sortedUnique(keys)
			svc.Labels.Values["walheim.injected-env.override"] = strings.Join(sorted, ",")
		}

		// walheim.injected-env.secret.<name>
		if smap, ok := secretInjected[svcName]; ok {
			for secretName, keys := range smap {
				sorted := sortedUnique(keys)
				svc.Labels.Values["walheim.injected-env.secret."+secretName] = strings.Join(sorted, ",")
			}
		}

		// walheim.injected-env.configmap.<name>
		if cmap, ok := configmapInjected[svcName]; ok {
			for cmName, keys := range cmap {
				sorted := sortedUnique(keys)
				svc.Labels.Values["walheim.injected-env.configmap."+cmName] = strings.Join(sorted, ",")
			}
		}

		services[svcName] = svc
	}
}

// ── Mount helpers ─────────────────────────────────────────────────────────────

// writeMountFiles writes each key in kvMap as a file under
// <resourceDir>/mounts/<sourceType>/<sourceName>/.
// sourceType is "configmaps" or "secrets".
func writeMountFiles(resourceDir, sourceType, sourceName string, kvMap map[string]string, filesystem fs.FS) error {
	dir := filepath.Join(resourceDir, "mounts", sourceType, sourceName)
	if err := filesystem.MkdirAll(dir); err != nil {
		return fmt.Errorf("create mount dir for %s/%s: %w", sourceType, sourceName, err)
	}

	for key, value := range kvMap {
		if err := filesystem.WriteFile(filepath.Join(dir, key), []byte(value)); err != nil {
			return fmt.Errorf("write mount file %q: %w", key, err)
		}
	}

	return nil
}

// injectComposeMounts writes mount files and appends bind-mount volume entries
// into the target services. Volume paths are relative to the compose file
// location (i.e. resourceDir is the directory that contains docker-compose.yml).
func injectComposeMounts(resourceDir string, services map[string]apiv1alpha1.ComposeService, mounts []apiv1alpha1.MountEntry, namespace string, filesystem fs.FS, dataDir string) error {
	for _, entry := range mounts {
		var (
			kvMap                  map[string]string
			sourceType, sourceName string
			err                    error
		)

		if entry.SecretRef != nil {
			kvMap, err = loadSecret(namespace, entry.SecretRef.Name, filesystem, dataDir)
			if err != nil {
				return fmt.Errorf("mounts: %w", err)
			}

			sourceType, sourceName = "secrets", entry.SecretRef.Name
		} else if entry.ConfigMapRef != nil {
			kvMap, err = loadConfigMap(namespace, entry.ConfigMapRef.Name, filesystem, dataDir)
			if err != nil {
				return fmt.Errorf("mounts: %w", err)
			}

			sourceType, sourceName = "configmaps", entry.ConfigMapRef.Name
		} else {
			continue
		}

		if err := writeMountFiles(resourceDir, sourceType, sourceName, kvMap, filesystem); err != nil {
			return err
		}

		// Relative volume string: ./mounts/<type>/<name>:/container/path:ro
		volumeStr := fmt.Sprintf("./mounts/%s/%s:%s:ro", sourceType, sourceName, entry.MountPath)

		for _, svcName := range targetServices(services, entry.ServiceNames) {
			svc := services[svcName]
			if svc.Extra == nil {
				svc.Extra = make(map[string]any)
			}

			existing, _ := svc.Extra["volumes"].([]any)
			svc.Extra["volumes"] = append(existing, volumeStr)
			services[svcName] = svc
		}
	}

	return nil
}

// targetServices returns the list of service names to inject into.
// If serviceNames is non-empty, only those; otherwise all services.
func targetServices(services map[string]apiv1alpha1.ComposeService, serviceNames []string) []string {
	if len(serviceNames) > 0 {
		return serviceNames
	}

	names := make([]string, 0, len(services))
	for n := range services {
		names = append(names, n)
	}

	sort.Strings(names)

	return names
}

// substituteVars replaces ${VAR} patterns in s with values from env.
// If a variable is not found in env, the literal ${VAR} is kept.
func substituteVars(s string, env map[string]string) string {
	return varPattern.ReplaceAllStringFunc(s, func(match string) string {
		// match is like "${VAR_NAME}"
		varName := match[2 : len(match)-1]
		if val, ok := env[varName]; ok {
			return val
		}

		return match
	})
}

// sortedUnique returns sorted, deduplicated copy of keys.
func sortedUnique(keys []string) []string {
	seen := map[string]bool{}

	var out []string

	for _, k := range keys {
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}

	sort.Strings(out)

	return out
}

// loadSecret reads the .secret.yaml for the named secret in namespace.
// Decodes Data (base64) and merges with StringData (plaintext).
// StringData wins on key collision.
func loadSecret(namespace, name string, filesystem fs.FS, dataDir string) (map[string]string, error) {
	path := filepath.Join(dataDir, "namespaces", namespace, "secrets", name, ".secret.yaml")

	data, err := filesystem.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("secret %q not found in namespace %q", name, namespace)
	}

	var sm corev1.Secret
	if err := yaml.Unmarshal(data, &sm); err != nil {
		return nil, fmt.Errorf("parse secret %q: %w", name, err)
	}

	result := make(map[string]string, len(sm.Data)+len(sm.StringData))
	for k, v := range sm.Data {
		decoded, err := base64.StdEncoding.DecodeString(v)
		if err != nil {
			return nil, fmt.Errorf("secret %q key %q: base64 decode failed: %w", name, k, err)
		}

		result[k] = string(decoded)
	}

	for k, v := range sm.StringData { // stringData wins
		result[k] = v
	}

	return result, nil
}

// loadConfigMap reads the .configmap.yaml for the named configmap in namespace.
func loadConfigMap(namespace, name string, filesystem fs.FS, dataDir string) (map[string]string, error) {
	path := filepath.Join(dataDir, "namespaces", namespace, "configmaps", name, ".configmap.yaml")

	data, err := filesystem.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("configmap %q not found in namespace %q", name, namespace)
	}

	var cm corev1.ConfigMap
	if err := yaml.Unmarshal(data, &cm); err != nil {
		return nil, fmt.Errorf("parse configmap %q: %w", name, err)
	}

	result := make(map[string]string, len(cm.Data))
	for k, v := range cm.Data {
		result[k] = v
	}

	return result, nil
}
