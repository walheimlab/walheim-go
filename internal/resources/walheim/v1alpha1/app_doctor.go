package v1alpha1

import (
	"fmt"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/walheimlab/walheim-go/internal/doctor"
	"github.com/walheimlab/walheim-go/internal/exitcode"
	"github.com/walheimlab/walheim-go/internal/output"
	"github.com/walheimlab/walheim-go/internal/registry"
	apiv1alpha1 "github.com/walheimlab/walheim-go/pkg/api/walheim/v1alpha1"
)

func (a *App) runDoctor(opts registry.OperationOpts) error {
	jsonMode := opts.Output == "json"
	rep := &doctor.Report{}

	if opts.Name != "" {
		namespace := opts.Namespace

		exists, err := a.Exists(namespace, opts.Name)
		if err != nil {
			return exitErr(exitcode.Failure, err)
		}

		if !exists {
			msg := fmt.Sprintf("app %q not found in namespace %q", opts.Name, namespace)
			output.Errorf(jsonMode, "NotFound", msg, "", nil, false)

			return exitErr(exitcode.NotFound, fmt.Errorf("%s", msg))
		}

		a.doctorApp(rep, namespace, opts.Name)
	} else if opts.AllNamespaces {
		namespaces, err := a.ValidNamespaces()
		if err != nil {
			return exitErr(exitcode.Failure, err)
		}

		for _, ns := range namespaces {
			names, err := a.ListNames(ns)
			if err != nil {
				rep.Warnf("namespace/"+ns, "list-apps", "cannot list apps: %v", err)
				continue
			}

			for _, name := range names {
				a.doctorApp(rep, ns, name)
			}
		}
	} else {
		namespace := opts.Namespace

		names, err := a.ListNames(namespace)
		if err != nil {
			return exitErr(exitcode.Failure, err)
		}

		for _, name := range names {
			a.doctorApp(rep, namespace, name)
		}
	}

	if jsonMode {
		return rep.PrintJSON()
	}

	rep.PrintHuman(opts.Quiet)

	if rep.HasErrors() {
		return exitErr(exitcode.Failure, fmt.Errorf("doctor found errors"))
	}

	return nil
}

// doctorApp runs all checks for a single app and adds findings to rep.
func (a *App) doctorApp(rep *doctor.Report, namespace, name string) {
	resourceID := "app/" + namespace + "/" + name

	data, err := a.ReadBytes(namespace, name)
	if err != nil {
		rep.Errorf(resourceID, "manifest-unreadable", "cannot read manifest: %v", err)
		return
	}

	var m apiv1alpha1.App
	if err := yaml.Unmarshal(data, &m); err != nil {
		rep.Errorf(resourceID, "manifest-parse", "manifest YAML is invalid: %v", err)
		return
	}

	doctor.CheckAPIVersion(rep, resourceID, m.APIVersion, appKind.APIVersion())
	doctor.CheckKind(rep, resourceID, m.Kind, appKind.Kind)
	doctor.CheckDirNameMatchesMetadataName(rep, resourceID, name, m.Name)
	doctor.CheckNamespaceFieldMatchesDir(rep, resourceID, m.Namespace, namespace)

	if len(m.Spec.Compose.Services) == 0 {
		rep.Errorf(resourceID, "no-services", "spec.compose.services must define at least one service")
	}

	for i, entry := range m.Spec.EnvFrom {
		switch {
		case entry.SecretRef != nil:
			secretPath := filepath.Join(a.DataDir, "namespaces", namespace, "secrets",
				entry.SecretRef.Name, ".secret.yaml")

			exists, err := a.FS.Exists(secretPath)
			if err != nil {
				rep.Warnf(resourceID, "envfrom-secret-check",
					"envFrom[%d]: cannot check secret %q: %v", i, entry.SecretRef.Name, err)
			} else if !exists {
				rep.Errorf(resourceID, "envfrom-secret-missing",
					"envFrom[%d]: secretRef %q does not exist in namespace %q",
					i, entry.SecretRef.Name, namespace)
			}

			for _, sn := range entry.ServiceNames {
				if _, ok := m.Spec.Compose.Services[sn]; !ok {
					rep.Warnf(resourceID, "envfrom-unknown-service",
						"envFrom[%d] secretRef %q: serviceNames references unknown service %q",
						i, entry.SecretRef.Name, sn)
				}
			}

		case entry.ConfigMapRef != nil:
			cmPath := filepath.Join(a.DataDir, "namespaces", namespace, "configmaps",
				entry.ConfigMapRef.Name, ".configmap.yaml")

			exists, err := a.FS.Exists(cmPath)
			if err != nil {
				rep.Warnf(resourceID, "envfrom-configmap-check",
					"envFrom[%d]: cannot check configmap %q: %v", i, entry.ConfigMapRef.Name, err)
			} else if !exists {
				rep.Errorf(resourceID, "envfrom-configmap-missing",
					"envFrom[%d]: configMapRef %q does not exist in namespace %q",
					i, entry.ConfigMapRef.Name, namespace)
			}

			for _, sn := range entry.ServiceNames {
				if _, ok := m.Spec.Compose.Services[sn]; !ok {
					rep.Warnf(resourceID, "envfrom-unknown-service",
						"envFrom[%d] configMapRef %q: serviceNames references unknown service %q",
						i, entry.ConfigMapRef.Name, sn)
				}
			}

		default:
			rep.Warnf(resourceID, "envfrom-no-ref",
				"envFrom[%d]: entry has neither secretRef nor configMapRef", i)
		}
	}

	for i, entry := range m.Spec.Env {
		for _, sn := range entry.ServiceNames {
			if _, ok := m.Spec.Compose.Services[sn]; !ok {
				rep.Warnf(resourceID, "env-unknown-service",
					"env[%d] %q: serviceNames references unknown service %q",
					i, entry.Name, sn)
			}
		}
	}
}
