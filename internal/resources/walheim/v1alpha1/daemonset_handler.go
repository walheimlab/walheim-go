package v1alpha1

import (
	"fmt"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/walheimlab/walheim-go/internal/exitcode"
	"github.com/walheimlab/walheim-go/internal/fs"
	"github.com/walheimlab/walheim-go/internal/output"
	"github.com/walheimlab/walheim-go/internal/resource"
	"github.com/walheimlab/walheim-go/internal/ssh"
	apiv1alpha1 "github.com/walheimlab/walheim-go/pkg/api/walheim/v1alpha1"
)

var daemonSetKind = resource.KindInfo{
	Group:   "walheim",
	Version: "v1alpha1",
	Kind:    "DaemonSet",
	Plural:  "daemonsets",
	Aliases: []string{"ds"},
}

// DaemonSet is the handler for the DaemonSet resource kind.
// It is cluster-scoped: stored at <dataDir>/daemonsets/<name>/.daemonset.yaml
// and deployed to every namespace whose labels match spec.namespaceSelector.
type DaemonSet struct {
	resource.ClusterBase
}

func newDaemonSet(dataDir string, filesystem fs.FS) *DaemonSet {
	return &DaemonSet{
		ClusterBase: resource.ClusterBase{
			DataDir:          dataDir,
			FS:               filesystem,
			Info:             daemonSetKind,
			ManifestFilename: ".daemonset.yaml",
		},
	}
}

func (d *DaemonSet) KindInfo() resource.KindInfo { return daemonSetKind }

// matchingNamespaces returns all namespace manifests whose labels match the
// given selector, along with their names, ordered alphabetically.
func matchingNamespaces(selector *apiv1alpha1.LabelSelector, filesystem fs.FS, dataDir string) ([]*apiv1alpha1.Namespace, []string, error) {
	baseDir := filepath.Join(dataDir, "namespaces")

	entries, err := filesystem.ReadDir(baseDir)
	if err != nil {
		exists, existsErr := filesystem.Exists(baseDir)
		if existsErr != nil {
			return nil, nil, existsErr
		}

		if !exists {
			return nil, nil, nil
		}

		return nil, nil, fmt.Errorf("read namespaces dir: %w", err)
	}

	var (
		manifests []*apiv1alpha1.Namespace
		names     []string
	)

	for _, entry := range entries {
		manifestPath := filepath.Join(baseDir, entry, ".namespace.yaml")

		ok, err := filesystem.Exists(manifestPath)
		if err != nil || !ok {
			continue
		}

		data, err := filesystem.ReadFile(manifestPath)
		if err != nil {
			continue
		}

		var m apiv1alpha1.Namespace
		if err := yaml.Unmarshal(data, &m); err != nil {
			continue
		}

		if selector.Matches(m.Labels) {
			manifests = append(manifests, &m)
			names = append(names, entry)
		}
	}

	return manifests, names, nil
}

// copyDaemonSetManifest returns a deep copy of m via YAML round-trip.
// generateDaemonSetCompose mutates its manifest argument, so each parallel
// goroutine must work on its own copy.
func copyDaemonSetManifest(m *apiv1alpha1.DaemonSet) (*apiv1alpha1.DaemonSet, error) {
	data, err := yaml.Marshal(m)
	if err != nil {
		return nil, err
	}

	var cp apiv1alpha1.DaemonSet
	if err := yaml.Unmarshal(data, &cp); err != nil {
		return nil, err
	}

	return &cp, nil
}

// deployedNamespaces returns the names of namespaces that have a local compose
// directory under <dataDir>/daemonsets/<dsName>/, indicating a prior deployment.
func (d *DaemonSet) deployedNamespaces(dsName string) ([]string, error) {
	dsDir := d.ResourceDir(dsName)

	entries, err := d.FS.ReadDir(dsDir)
	if err != nil {
		if ok, _ := d.FS.Exists(dsDir); !ok {
			return nil, nil
		}

		return nil, fmt.Errorf("read daemonset dir: %w", err)
	}

	var names []string

	for _, entry := range entries {
		composePath := filepath.Join(dsDir, entry, "docker-compose.yml")
		if ok, _ := d.FS.Exists(composePath); ok {
			names = append(names, entry)
		}
	}

	return names, nil
}

// loadNamespace reads a namespace manifest by name without any selector
// filtering. Used during cleanup to reach namespaces that no longer match.
func (d *DaemonSet) loadNamespace(ns string) (*apiv1alpha1.Namespace, error) {
	path := filepath.Join(d.DataDir, "namespaces", ns, ".namespace.yaml")

	data, err := d.FS.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("namespace %q not found", ns)
	}

	var m apiv1alpha1.Namespace
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, err
	}

	return &m, nil
}

// stopInNamespace runs docker compose down, removes remote files, and removes
// the local compose directory for a daemonset in a specific namespace.
func (d *DaemonSet) stopInNamespace(dsName, ns string) error {
	nsMeta, err := d.loadNamespace(ns)
	if err != nil {
		output.Warnf("cannot load namespace %q for cleanup: %v — skipping remote cleanup", ns, err)
	} else {
		target := nsMeta.Spec.SSHTarget()
		remoteDir := nsMeta.Spec.RemoteBaseDir() + "/daemonsets/" + dsName

		sshClient := ssh.NewClient(target)
		if _, checkErr := sshClient.RunOutput("test -d " + remoteDir); checkErr == nil {
			if err := sshClient.Run("cd " + remoteDir + " && docker compose --progress plain down"); err != nil {
				return exitErr(exitcode.Failure, fmt.Errorf("docker compose down in %q: %w", ns, err))
			}

			if err := sshClient.Run("rm -rf " + remoteDir); err != nil {
				return exitErr(exitcode.Failure, fmt.Errorf("remove remote files in %q: %w", ns, err))
			}
		}
	}

	localDir := filepath.Join(d.DataDir, "daemonsets", dsName, ns)
	if err := d.FS.RemoveAll(localDir); err != nil {
		return exitErr(exitcode.Failure, fmt.Errorf("remove local compose dir for %q: %w", ns, err))
	}

	return nil
}
