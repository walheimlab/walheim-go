package v1alpha1

import (
	"fmt"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/walheimlab/walheim-go/internal/fs"
	"github.com/walheimlab/walheim-go/internal/resource"
	apiv1alpha1 "github.com/walheimlab/walheim-go/pkg/api/walheim/v1alpha1"
)

var appKind = resource.KindInfo{
	Group:   "walheim",
	Version: "v1alpha1",
	Kind:    "App",
	Plural:  "apps",
	Aliases: []string{},
}

// App is the handler for the App resource kind.
type App struct {
	resource.NamespacedBase
}

func newApp(dataDir string, filesystem fs.FS) *App {
	return &App{
		NamespacedBase: resource.NamespacedBase{
			DataDir:          dataDir,
			FS:               filesystem,
			Info:             appKind,
			ManifestFilename: ".app.yaml",
		},
	}
}

func (a *App) KindInfo() resource.KindInfo { return appKind }

func (a *App) loadNamespaceManifest(namespace string) (*apiv1alpha1.Namespace, error) {
	path := filepath.Join(a.DataDir, "namespaces", namespace, ".namespace.yaml")

	data, err := a.FS.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("namespace %q not found", namespace)
	}

	var m apiv1alpha1.Namespace
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, err
	}

	return &m, nil
}
