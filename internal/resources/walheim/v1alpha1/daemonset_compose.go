package v1alpha1

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/walheimlab/walheim-go/internal/fs"
	"github.com/walheimlab/walheim-go/internal/yamlutil"
	apiv1alpha1 "github.com/walheimlab/walheim-go/pkg/api/walheim/v1alpha1"
)

// generateDaemonSetCompose builds docker-compose.yml for a daemonset in a
// specific namespace, written to:
//
//	<dataDir>/daemonsets/<dsName>/<namespace>/docker-compose.yml
//
// NOTE: This function modifies m.Spec.Compose.Services in-place.
func generateDaemonSetCompose(namespace, dsName string, m *apiv1alpha1.DaemonSet, filesystem fs.FS, dataDir string) error {
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

	// Inject Walheim labels into every service.
	for svcName, svc := range services {
		for k := range svc.Labels.Values {
			if strings.HasPrefix(k, "walheim.") {
				delete(svc.Labels.Values, k)
			}
		}

		svc.Labels.Values["walheim.managed"] = "true"
		svc.Labels.Values["walheim.namespace"] = namespace
		svc.Labels.Values["walheim.owner"] = dsName
		svc.Labels.Values["walheim.kind"] = "DaemonSet"
		services[svcName] = svc
	}

	// Load and inject envFrom (lower precedence — only if key not present).
	for _, entry := range m.Spec.EnvFrom {
		var (
			kvMap map[string]string
			err   error
		)

		if entry.SecretRef != nil {
			kvMap, err = loadSecret(namespace, entry.SecretRef.Name, filesystem, dataDir)
			if err != nil {
				return fmt.Errorf("envFrom: %w", err)
			}
		} else if entry.ConfigMapRef != nil {
			kvMap, err = loadConfigMap(namespace, entry.ConfigMapRef.Name, filesystem, dataDir)
			if err != nil {
				return fmt.Errorf("envFrom: %w", err)
			}
		} else {
			continue
		}

		targets := targetServices(services, entry.ServiceNames)
		for _, svcName := range targets {
			svc := services[svcName]
			for k, v := range kvMap {
				if _, exists := svc.Environment.Values[k]; !exists {
					svc.Environment.Values[k] = v
				}
			}

			services[svcName] = svc
		}
	}

	// Inject env entries (higher precedence — always overwrite).
	for _, entry := range m.Spec.Env {
		targets := targetServices(services, entry.ServiceNames)
		for _, svcName := range targets {
			svc := services[svcName]
			value := substituteVars(entry.Value, svc.Environment.Values)
			svc.Environment.Values[entry.Name] = value
			services[svcName] = svc
		}
	}

	m.Spec.Compose.Services = services

	resourceDir := filepath.Join(dataDir, "daemonsets", dsName, namespace)
	if err := filesystem.MkdirAll(resourceDir); err != nil {
		return fmt.Errorf("create compose dir: %w", err)
	}

	if err := injectComposeMounts(resourceDir, services, m.Spec.Mounts, namespace, filesystem, dataDir); err != nil {
		return err
	}

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
