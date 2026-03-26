package v1alpha1

import (
	"fmt"
	"path/filepath"

	"github.com/walheimlab/walheim-go/internal/fs"
	"github.com/walheimlab/walheim-go/internal/resource"
	apiv1alpha1 "github.com/walheimlab/walheim-go/pkg/api/walheim/v1alpha1"
)

var namespaceKind = resource.KindInfo{
	Group:   "walheim",
	Version: "v1alpha1",
	Kind:    "Namespace",
	Plural:  "namespaces",
	Aliases: []string{"ns"},
}

// Namespace is the handler for the Namespace resource kind.
type Namespace struct {
	resource.ClusterBase
}

func newNamespace(dataDir string, filesystem fs.FS) *Namespace {
	return &Namespace{
		ClusterBase: resource.ClusterBase{
			DataDir:          dataDir,
			FS:               filesystem,
			Info:             namespaceKind,
			ManifestFilename: ".namespace.yaml",
		},
	}
}

func (n *Namespace) KindInfo() resource.KindInfo { return namespaceKind }

func (n *Namespace) createSubdirs(name string) error {
	nsDir := n.ResourceDir(name)
	for _, sub := range []string{"apps", "secrets", "configmaps"} {
		if err := n.FS.MkdirAll(filepath.Join(nsDir, sub)); err != nil {
			return fmt.Errorf("create %s subdir: %w", sub, err)
		}
	}

	return nil
}

func (n *Namespace) countLocalResources(nsName string) apiv1alpha1.NamespaceResourceCounts {
	nsDir := n.ResourceDir(nsName)
	count := func(sub string) int {
		entries, err := n.FS.ReadDir(filepath.Join(nsDir, sub))
		if err != nil {
			return 0
		}

		return len(entries)
	}

	return apiv1alpha1.NamespaceResourceCounts{
		Apps:       count("apps"),
		Secrets:    count("secrets"),
		ConfigMaps: count("configmaps"),
	}
}

// localAppNames returns the set of app names in the local context for nsName.
func (n *Namespace) localAppNames(nsName string) map[string]struct{} {
	nsDir := n.ResourceDir(nsName)

	entries, err := n.FS.ReadDir(filepath.Join(nsDir, "apps"))
	if err != nil {
		return nil
	}

	set := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		set[entry] = struct{}{}
	}

	return set
}

// localDaemonSetNames returns the set of daemonset names that have been
// deployed to nsName (i.e. have a per-namespace directory under daemonsets/).
func (n *Namespace) localDaemonSetNames(nsName string) map[string]struct{} {
	entries, err := n.FS.ReadDir(filepath.Join(n.DataDir, "daemonsets"))
	if err != nil {
		return nil
	}

	set := make(map[string]struct{})

	for _, dsName := range entries {
		nsDir := filepath.Join(n.DataDir, "daemonsets", dsName, nsName)
		if isDir, err := n.FS.IsDir(nsDir); err == nil && isDir {
			set[dsName] = struct{}{}
		}
	}

	return set
}
