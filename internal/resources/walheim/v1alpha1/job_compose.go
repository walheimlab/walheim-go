package v1alpha1

import (
	"fmt"
	"path/filepath"

	"github.com/walheimlab/walheim-go/internal/fs"
	"github.com/walheimlab/walheim-go/internal/yamlutil"
	apiv1alpha1 "github.com/walheimlab/walheim-go/pkg/api/walheim/v1alpha1"
)

// generateJobCompose builds a docker-compose.yml for the job and writes it to
// <localResourceDir>/docker-compose.yml. Mount files are also written under
// <localResourceDir>/mounts/ so they can be rsynced together.
func generateJobCompose(localResourceDir, ns, name string, spec apiv1alpha1.JobSpec, filesystem fs.FS, dataDir string) error {
	// Resolve env (envFrom + env overrides).
	env := make(map[string]string)

	for _, entry := range spec.EnvFrom {
		var (
			kvMap map[string]string
			err   error
		)

		if entry.SecretRef != nil {
			kvMap, err = loadSecret(ns, entry.SecretRef.Name, filesystem, dataDir)
		} else if entry.ConfigMapRef != nil {
			kvMap, err = loadConfigMap(ns, entry.ConfigMapRef.Name, filesystem, dataDir)
		} else {
			continue
		}

		if err != nil {
			return fmt.Errorf("envFrom: %w", err)
		}

		for k, v := range kvMap {
			if _, exists := env[k]; !exists {
				env[k] = v
			}
		}
	}

	for _, entry := range spec.Env {
		env[entry.Name] = substituteVars(entry.Value, env)
	}

	svc := apiv1alpha1.ComposeService{
		Image:       spec.Image,
		Environment: apiv1alpha1.ServiceEnv{Values: env},
		Labels: apiv1alpha1.ServiceLabels{Values: map[string]string{
			"walheim.managed":   "true",
			"walheim.namespace": ns,
			"walheim.job":       name,
		}},
		Extra: map[string]any{
			"restart": "no",
		},
	}

	// Write mount files and add volumes.
	for _, entry := range spec.Mounts {
		var (
			kvMap                  map[string]string
			sourceType, sourceName string
			err                    error
		)

		if entry.SecretRef != nil {
			kvMap, err = loadSecret(ns, entry.SecretRef.Name, filesystem, dataDir)
			if err != nil {
				return fmt.Errorf("mounts: %w", err)
			}

			sourceType, sourceName = "secrets", entry.SecretRef.Name
		} else if entry.ConfigMapRef != nil {
			kvMap, err = loadConfigMap(ns, entry.ConfigMapRef.Name, filesystem, dataDir)
			if err != nil {
				return fmt.Errorf("mounts: %w", err)
			}

			sourceType, sourceName = "configmaps", entry.ConfigMapRef.Name
		} else {
			continue
		}

		if err := writeMountFiles(localResourceDir, sourceType, sourceName, kvMap, filesystem); err != nil {
			return err
		}

		existing, _ := svc.Extra["volumes"].([]any)
		svc.Extra["volumes"] = append(existing, fmt.Sprintf("./mounts/%s/%s:%s:ro", sourceType, sourceName, entry.MountPath))
	}

	if len(spec.Command) > 0 {
		svc.Extra["command"] = spec.Command
	}

	compose := apiv1alpha1.ComposeSpec{
		Services: map[string]apiv1alpha1.ComposeService{"job": svc},
	}

	encoded, err := yamlutil.Marshal(compose)
	if err != nil {
		return fmt.Errorf("marshal docker-compose: %w", err)
	}

	composePath := filepath.Join(localResourceDir, "docker-compose.yml")

	return filesystem.WriteFile(composePath, encoded)
}
